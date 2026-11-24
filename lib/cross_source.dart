import 'dart:async';

import 'package:flutter/material.dart';

import 'core_bridge.dart';
import 'models.dart';

/// 同名换源：某站源把正片锁在服务端时，用其他站源的同名条目拿到全集。
class CrossSourceCandidate {
  const CrossSourceCandidate({
    required this.id,
    required this.source,
    required this.sourceId,
    required this.title,
    required this.cover,
    required this.episodes,
    required this.vipEpisodes,
    required this.playable,
    required this.lastPlayable,
    required this.score,
    required this.error,
  });

  factory CrossSourceCandidate.fromJson(Map<String, dynamic> json) =>
      CrossSourceCandidate(
        id: json['id'] as String? ?? '',
        source: json['source'] as String? ?? '',
        sourceId: json['sourceId'] as String? ?? '',
        title: json['title'] as String? ?? '',
        cover: json['cover'] as String? ?? '',
        episodes: intValue(json['episodes']),
        vipEpisodes: intValue(json['vipEpisodes']),
        playable: json['playable'] == true,
        lastPlayable: json['lastPlayable'] == true,
        score: intValue(json['score']),
        error: json['error'] as String? ?? '',
      );

  final String id;
  final String source;
  final String sourceId;
  final String title;
  final String cover;
  final int episodes;
  final int vipEpisodes;
  final bool playable;
  final bool lastPlayable;
  final int score;
  final String error;

  bool get isComplete => playable && lastPlayable && vipEpisodes == 0;

  String get badge {
    if (isComplete) return '全集';
    if (playable && lastPlayable) return '可播';
    if (playable) return '部分';
    return '不可用';
  }

  Drama toDrama() => Drama(
    id: id,
    source: source,
    sourceId: sourceId,
    title: title,
    cover: cover,
    episodes: episodes,
    vip: vipEpisodes == 0 ? false : null,
  );
}

class CrossSourceResult {
  const CrossSourceResult({
    required this.query,
    required this.candidates,
    required this.errors,
    required this.elapsedMs,
  });

  factory CrossSourceResult.fromJson(Map<String, dynamic> json) =>
      CrossSourceResult(
        query: json['query'] as String? ?? '',
        candidates: (json['candidates'] as List? ?? const [])
            .whereType<Map<String, dynamic>>()
            .map(CrossSourceCandidate.fromJson)
            .toList(),
        errors: (json['errors'] as Map?)?.map(
              (key, value) => MapEntry('$key', '$value'),
            ) ??
            const {},
        elapsedMs: intValue(json['elapsedMs']),
      );

  final String query;
  final List<CrossSourceCandidate> candidates;
  final Map<String, String> errors;
  final int elapsedMs;

  bool get hasComplete => candidates.any((item) => item.isComplete);
}

/// 打开换源面板。用户选中一条后返回该站源的 Drama，取消返回 null。
Future<Drama?> showCrossSourceSheet(
  BuildContext context, {
  required AppRepository repository,
  required Drama drama,
}) => showModalBottomSheet<Drama>(
  context: context,
  isScrollControlled: true,
  showDragHandle: true,
  builder: (_) => _CrossSourceSheet(repository: repository, drama: drama),
);

class _CrossSourceSheet extends StatefulWidget {
  const _CrossSourceSheet({required this.repository, required this.drama});

  final AppRepository repository;
  final Drama drama;

  @override
  State<_CrossSourceSheet> createState() => _CrossSourceSheetState();
}

class _CrossSourceSheetState extends State<_CrossSourceSheet> {
  CrossSourceResult? _result;
  String? _error;
  bool _loading = true;
  bool _refining = false;

  @override
  void initState() {
    super.initState();
    _search();
  }

  /// 先用缓存里的同名条目秒出候选，再后台跑完整搜索替换。
  Future<void> _search() async {
    setState(() {
      _loading = true;
      _refining = false;
      _error = null;
    });
    try {
      final cached = await widget.repository.crossSource(
        widget.drama,
        cachedOnly: true,
      );
      if (!mounted) return;
      final quick = CrossSourceResult.fromJson(cached);
      setState(() {
        if (quick.candidates.isNotEmpty) {
          _result = quick;
          _loading = false;
          _refining = true;
        }
      });
    } catch (_) {
      // 缓存不可用就直接走完整搜索。
    }
    await _searchFull();
  }

  Future<void> _searchFull() async {
    try {
      final data = await widget.repository.crossSource(widget.drama);
      if (!mounted) return;
      setState(() {
        _result = CrossSourceResult.fromJson(data);
        _loading = false;
        _refining = false;
      });
    } catch (error) {
      if (!mounted) return;
      setState(() {
        _error = _result == null ? error.toString() : null;
        _loading = false;
        _refining = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return SafeArea(
      child: ConstrainedBox(
        constraints: BoxConstraints(
          maxHeight: MediaQuery.of(context).size.height * 0.82,
        ),
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 4, 20, 20),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Row(
                children: [
                  Expanded(
                    child: Text('同名换源', style: theme.textTheme.titleLarge),
                  ),
                  IconButton(
                    tooltip: '重新搜索',
                    onPressed: _loading ? null : _search,
                    icon: const Icon(Icons.refresh),
                  ),
                ],
              ),
              Text(
                '在其他站源里找《${widget.drama.title}》，优先无 VIP 且集数完整的版本。',
                style: theme.textTheme.bodySmall,
              ),
              if (_refining)
                const Padding(
                  padding: EdgeInsets.only(top: 8),
                  child: LinearProgressIndicator(minHeight: 2),
                ),
              const SizedBox(height: 12),
              Flexible(child: _body(context)),
            ],
          ),
        ),
      ),
    );
  }

  Widget _body(BuildContext context) {
    if (_loading && (_result?.candidates.isEmpty ?? true)) {
      return const Center(
        child: Padding(
          padding: EdgeInsets.all(32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              CircularProgressIndicator(),
              SizedBox(height: 16),
              Text('正在逐个站源搜索，约 20–40 秒'),
            ],
          ),
        ),
      );
    }
    if (_error != null) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text('换源搜索失败：$_error', textAlign: TextAlign.center),
            const SizedBox(height: 12),
            FilledButton(onPressed: _search, child: const Text('重试')),
          ],
        ),
      );
    }
    final result = _result;
    if (result == null || result.candidates.isEmpty) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Text('没有找到同名条目'),
            const SizedBox(height: 8),
            Text(
              result == null || result.errors.isEmpty
                  ? '可以换个更短的剧名再试。'
                  : '失败站源：${result.errors.keys.join('、')}',
              style: Theme.of(context).textTheme.bodySmall,
              textAlign: TextAlign.center,
            ),
          ],
        ),
      );
    }
    return ListView.separated(
      itemCount: result.candidates.length,
      separatorBuilder: (_, __) => const SizedBox(height: 8),
      itemBuilder: (context, index) {
        final item = result.candidates[index];
        final theme = Theme.of(context);
        return Card(
          margin: EdgeInsets.zero,
          child: ListTile(
            enabled: item.playable,
            leading: CircleAvatar(
              backgroundColor: item.isComplete
                  ? theme.colorScheme.primaryContainer
                  : theme.colorScheme.surfaceContainerHighest,
              child: Text('${item.episodes}'),
            ),
            title: Text(item.title, overflow: TextOverflow.ellipsis),
            subtitle: Text(
              '${item.source} · ${item.badge}'
              '${item.vipEpisodes > 0 ? ' · VIP ${item.vipEpisodes} 集' : ''}'
              '${item.error.isEmpty ? '' : ' · ${item.error}'}',
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
            ),
            trailing: item.playable
                ? const Icon(Icons.chevron_right)
                : const Icon(Icons.block),
            onTap: item.playable
                ? () => Navigator.of(context).pop(item.toDrama())
                : null,
          ),
        );
      },
    );
  }
}
