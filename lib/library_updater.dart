import 'dart:async';

import 'package:flutter/foundation.dart';

import 'core_bridge.dart';
import 'local_store.dart';
import 'models.dart';
import 'source_status.dart';

class LibraryUpdater extends ChangeNotifier {
  LibraryUpdater(this.repository, this.store, {this.onCatalogChanged})
    : _epoch = store.profileEpoch;

  final AppRepository repository;
  final LocalStore store;
  final ValueChanged<String>? onCatalogChanged;
  final int _epoch;
  final _statuses = <String, SourceStatus>{};
  final _errors = <String, String>{};
  final _pending = <String>{};
  final _stopRequested = <String>{};
  final _revisions = <String, int>{};
  final _delivered = <String, String>{};

  /// 「更新」完成后自动继续拉的历史页数，用来把剧库快照做厚。
  /// 聚合首页直接读这份快照，所以页数越多首屏卡片越全（不占首屏时间）。
  static const int autoExtendPages = 3;
  final _extendRemaining = <String, int>{};
  Timer? _timer;
  bool _polling = false;
  bool _disposed = false;
  int _ticks = 0;

  bool get _valid =>
      !_disposed && !store.locked && _epoch == store.profileEpoch;
  SourceStatus? status(String source) => _statuses[source];
  String error(String source) => _errors[source] ?? '';
  bool busy(String source) =>
      _pending.contains(source) || _statuses[source]?.running == true;

  void startWatching() {
    if (_timer != null || !repository.supportsSourceManagement || !_valid) {
      return;
    }
    unawaited(refresh());
    _timer = Timer.periodic(const Duration(seconds: 2), (_) {
      _ticks++;
      if (_statuses.values.any((status) => status.running) || _ticks % 5 == 0) {
        unawaited(refresh());
      }
    });
  }

  void _notify() {
    if (_valid) notifyListeners();
  }

  void _accept(SourceStatus status, {bool submitted = false}) {
    if (!_valid || !store.allowsSource(status.source)) return;
    _statuses[status.source] = status;
    _errors.remove(status.source);
    final revision =
        '${status.operation}|${status.startedAt}|${status.finishedAt}|'
        '${status.updatedAt}|${status.count}|${status.storageError}';
    if (!status.running &&
        (status.finishedAt != null || submitted) &&
        {
          'update',
          'more',
          'metadata',
          'vipMetadata',
          'retrySave',
        }.contains(status.operation) &&
        _delivered[status.source] != revision) {
      _delivered[status.source] = revision;
      onCatalogChanged?.call(status.source);
    }
  }

  Future<void> refresh() async {
    if (_polling || !_valid || !repository.supportsSourceManagement) return;
    _polling = true;
    try {
      final ids = store.sources.map((source) => source.id).toList();
      if (ids.isEmpty) return;
      // 单次 IPC 取回全部源状态。原先是每源一次 sourceStatus：10 个源
      // 每 10 秒就有 10 次 isolate 冷启，主列表会一顿一顿的。
      final statuses = await repository.sourceStatuses(ids);
      for (final id in ids) {
        if (!_valid) return;
        final revision = _revisions[id] ?? 0;
        final status = statuses[id];
        if (status != null && revision == (_revisions[id] ?? 0)) {
          _accept(status);
          if (_valid && !status.running) {
            unawaited(_extendCatalog(id));
          }
        } else if (status == null && revision == (_revisions[id] ?? 0)) {
          _errors[id] = '未取到站源状态';
        }
      }
    } catch (error) {
      if (_valid) {
        for (final id in store.sources.map((s) => s.id)) {
          _errors[id] = error.toString();
        }
      }
    } finally {
      _polling = false;
      _notify();
    }
  }

  Future<void> update(Iterable<SourceSite> sources) async {
    if (!_valid || !repository.supportsSourceManagement) return;
    final selected = sources
        .map((source) => source.id)
        .toSet()
        .where((source) => store.allowsSource(source) && !busy(source))
        .toList();
    for (final source in selected) {
      _stopRequested.remove(source);
      _pending.add(source);
      _errors.remove(source);
      _revisions[source] = (_revisions[source] ?? 0) + 1;
    }
    _notify();
    try {
      for (final source in selected) {
        if (!_valid || !store.allowsSource(source)) break;
        try {
          if (_stopRequested.contains(source)) continue;
          final status = await repository.startSourceJob(source, 'update');
          _extendRemaining[source] = autoExtendPages;
          _accept(status, submitted: true);
          if (_valid && _stopRequested.contains(source) && status.running) {
            _accept(await repository.cancelSourceJob(source));
          }
        } catch (error) {
          if (_valid) _errors[source] = error.toString();
        } finally {
          _pending.remove(source);
          _notify();
        }
      }
    } finally {
      _pending.removeAll(selected);
      _notify();
    }
  }

  /// 更新完成后自动继续翻页，把已加载历史做进缓存快照。
  Future<void> _extendCatalog(String source) async {
    final remaining = _extendRemaining[source] ?? 0;
    if (remaining <= 0 || !_valid || !store.allowsSource(source)) return;
    if (_pending.contains(source) || _statuses[source]?.running == true) return;
    if (_statuses[source]?.hasMore == false) {
      _extendRemaining.remove(source);
      return;
    }
    _extendRemaining[source] = remaining - 1;
    _pending.add(source);
    _notify();
    try {
      final status = await repository.startSourceJob(source, 'more');
      _accept(status, submitted: true);
    } catch (error) {
      if (_valid) _errors[source] = error.toString();
    } finally {
      _pending.remove(source);
      _notify();
    }
  }

  Future<void> stop(Iterable<SourceSite> sources) async {
    final selected = sources.toList();
    if (!_valid) return;
    for (final source in selected) {
      if (store.allowsSource(source.id)) _stopRequested.add(source.id);
    }
    for (final source in selected) {
      if (!_valid) return;
      if (!store.allowsSource(source.id) ||
          _pending.contains(source.id) ||
          _statuses[source.id]?.running != true) {
        continue;
      }
      _pending.add(source.id);
      _revisions[source.id] = (_revisions[source.id] ?? 0) + 1;
      _notify();
      try {
        _accept(await repository.cancelSourceJob(source.id));
      } catch (error) {
        if (_valid) _errors[source.id] = error.toString();
      } finally {
        _pending.remove(source.id);
        _notify();
      }
    }
  }

  @override
  void dispose() {
    _disposed = true;
    _timer?.cancel();
    super.dispose();
  }
}
