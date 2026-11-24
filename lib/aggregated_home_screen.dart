import 'dart:async';

import 'package:flutter/material.dart';

import 'aggregated_detail_screen.dart';
import 'core_bridge.dart';
import 'dramax_card.dart';
import 'local_store.dart';
import 'models.dart';

/// 聚合视图的共享状态：由宿主页面持有，喂给顶栏按钮与内容区。
///
/// 只存本页视图状态（普通/成人、当前类型），不落盘、不带解锁语义；
/// 杀掉 App 重进就回到普通视图。
class AggregateController extends ChangeNotifier {
  AggregateController({bool adultView = false, String kind = ContentKind.all})
    : _adultView = adultView,
      _kind = kind;

  bool _adultView;
  String _kind;

  bool get adultView => _adultView;
  String get kind => _kind;

  void setKind(String value) {
    if (_kind == value) return;
    _kind = value;
    notifyListeners();
  }

  void toggleView() {
    _adultView = !_adultView;
    notifyListeners();
  }
}

/// 可嵌入的聚合内容区：类型 Tab + 作品网格。
///
/// 不自带 Scaffold，供首页「发现」标签页直接使用，保留底部导航；
/// 切换按钮由宿主放在顶栏（见 [AggregateController]）。
class AggregatedHomeView extends StatefulWidget {
  const AggregatedHomeView({
    super.key,
    required this.repository,
    required this.store,
    required this.controller,
    this.sources = const [],
    this.catalogRevision = 0,
  });

  final AppRepository repository;
  final LocalStore store;
  final AggregateController controller;
  final List<SourceSite> sources;

  /// 剧库更新计数：外部「更新剧库」完成后 +1，这里先用新缓存重铺一遍。
  final int catalogRevision;

  @override
  State<AggregatedHomeView> createState() => _AggregatedHomeViewState();
}

class _AggregatedHomeViewState extends State<AggregatedHomeView> {
  final _scroll = ScrollController();
  bool _loading = true;
  bool _loadingMore = false;
  bool _hasMore = false;
  int _page = 1;
  String _error = '';
  String _warning = '';
  List<AggregatedDrama> _items = const [];
  Set<String> _adultSources = const {};
  String _groupId = '';
  int _loadSeq = 0;

  AggregateController get _ctl => widget.controller;

  @override
  void initState() {
    super.initState();
    _ctl.addListener(_onControllerChanged);
    widget.store.addListener(_onStoreChanged);
    _bootstrap();
  }

  @override
  void dispose() {
    _ctl.removeListener(_onControllerChanged);
    widget.store.removeListener(_onStoreChanged);
    _scroll.dispose();
    super.dispose();
  }

  @override
  void didUpdateWidget(covariant AggregatedHomeView oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.catalogRevision != widget.catalogRevision) {
      unawaited(_reloadFromCache());
    }
  }

  /// 剧库刚更新完：先用缓存重铺（瞬时），再后台拉新数据。
  Future<void> _reloadFromCache() async {
    final seq = ++_loadSeq;
    final cachedItems = await _cachedItems();
    if (!mounted || seq != _loadSeq) return;
    if (cachedItems.isNotEmpty) {
      setState(() => _items = cachedItems);
    }
    if (mounted && seq == _loadSeq) await _load();
  }

  /// 站源分类标签变化后，成人/普通视图的源集合要跟着变。
  void _onStoreChanged() {
    final next = {
      for (final source in widget.sources)
        if (widget.store.sourceTaggedAdult(source.id)) source.id,
    };
    if (next.length == _adultSources.length && next.containsAll(_adultSources)) {
      return;
    }
    _adultSources = next;
    _groupId = '';
    _loadSeq++;
    setState(() => _items = const []);
    _load();
  }

  void _onControllerChanged() {
    _groupId = '';
    _loadSeq++;
    setState(() => _items = const []);
    _load();
  }

  /// 成人源标记由「站源管理」手动打标（LocalStore.sourceTags），
  /// 不再依赖模块 manifest，用户想怎么分就怎么分。
  Future<void> _loadAdultSources() async {
    _adultSources = {
      for (final source in widget.sources)
        if (widget.store.sourceTaggedAdult(source.id)) source.id,
    };
  }

  Future<void> _bootstrap() async {
    await _loadAdultSources();
    if (!mounted) return;
    await _load();
  }

  List<String> get _sourceIds {
    final wantAdult = _ctl.adultView;
    final ids = <String>[];
    for (final source in widget.sources) {
      if (_adultSources.contains(source.id) == wantAdult) ids.add(source.id);
    }
    return ids;
  }

  /// 用 Go 侧各源已有目录缓存拼首帧，单次 IPC、不联网。
  Future<List<AggregatedDrama>> _cachedItems() async {
    try {
      final page = await widget.repository.aggregateCached(
        sources: _sourceIds,
        kind: _ctl.kind,
        adultView: _ctl.adultView,
        sourcesTagged: true,
      );
      return page.items;
    } catch (_) {
      return const [];
    }
  }

  Future<void> _load({bool more = false}) async {
    if (more ? _loadingMore : _loading) return;
    if (more && !_hasMore) return;
    final seq = ++_loadSeq;
    final groupId = more || _groupId.isNotEmpty
        ? _groupId
        : 'agg-${DateTime.now().microsecondsSinceEpoch}';
    if (!more) _groupId = groupId;
    setState(() {
      if (more) {
        _loadingMore = true;
      } else {
        _loading = true;
        _error = '';
        _page = 1;
      }
    });
    final page = more ? _page + 1 : 1;
    // 首帧先铺已缓存的各源目录，避免整屏转圈等待全部站源。
    if (!more && page == 1 && _items.isEmpty) {
      final cachedItems = await _cachedItems();
      if (cachedItems.isNotEmpty && mounted && seq == _loadSeq) {
        setState(() => _items = cachedItems);
      }
    }
    try {
      final result = await widget.repository.aggregateCatalog(
        sources: _sourceIds,
        page: page,
        kind: _ctl.kind,
        adultView: _ctl.adultView,
        groupId: groupId,
        sourcesTagged: true,
      );
      if (!mounted || seq != _loadSeq) return;
      setState(() {
        _items = more ? [..._items, ...result.items] : result.items;
        _page = result.page;
        _hasMore = result.hasMore;
        _loading = false;
        _loadingMore = false;
        _warning = result.failedSources.isEmpty
            ? ''
            : '${result.failedSources.length} 个站源本轮未响应';
      });
      // 首屏只等了部分源；同一 groupId 再取一次拿完整结果。
      if (result.partial && !more && mounted) {
        unawaited(_loadRefinement(groupId, seq));
      }
    } catch (error) {
      if (!mounted || seq != _loadSeq) return;
      setState(() {
        _error = error is AppFailure ? error.message : '$error';
        _loading = false;
        _loadingMore = false;
      });
    }
  }

  /// 后台补拉剩余站源；同组聚合只替换当前结果，不重复分页。
  Future<void> _loadRefinement(String groupId, int seq) async {
    try {
      final result = await widget.repository.aggregateCatalog(
        sources: _sourceIds,
        page: 1,
        kind: _ctl.kind,
        adultView: _ctl.adultView,
        groupId: groupId,
        refine: true,
        sourcesTagged: true,
      );
      if (!mounted || seq != _loadSeq || result.partial) return;
      setState(() {
        _items = result.items;
        _hasMore = result.hasMore;
        _warning = result.failedSources.isEmpty
            ? ''
            : '${result.failedSources.length} 个站源本轮未响应';
      });
    } catch (_) {}
  }

  void _openItem(AggregatedDrama item) {
    if (item.sources.isEmpty) return;
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => AggregatedDetailScreen(
          item: item,
          repository: widget.repository,
          store: widget.store,
          adultView: _ctl.adultView,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Column(
      children: [
        SizedBox(
          height: 46,
          child: ListView.separated(
            scrollDirection: Axis.horizontal,
            padding: const EdgeInsets.fromLTRB(12, 0, 12, 8),
            itemCount: ContentKind.labels.length,
            separatorBuilder: (_, __) => const SizedBox(width: 8),
            itemBuilder: (context, index) {
              final entry = ContentKind.labels.entries.elementAt(index);
              return ChoiceChip(
                key: ValueKey('aggregate-kind-${entry.key}'),
                label: Text(entry.value),
                selected: entry.key == _ctl.kind,
                onSelected: (_) => _ctl.setKind(entry.key),
              );
            },
          ),
        ),
        if (_loading && _items.isNotEmpty)
          const LinearProgressIndicator(minHeight: 2),
        Expanded(
          child: RefreshIndicator(onRefresh: () => _load(), child: _body(theme)),
        ),
      ],
    );
  }

  Widget _body(ThemeData theme) {
    // 有缓存卡就直接铺出来，加载指示放顶部细条，不再整屏转圈。
    if (_loading && _items.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_error.isNotEmpty && _items.isEmpty) {
      return ListView(
        padding: const EdgeInsets.all(24),
        children: [
          const SizedBox(height: 60),
          Text('聚合加载失败：$_error', textAlign: TextAlign.center),
          const SizedBox(height: 12),
          Center(
            child: FilledButton(
              onPressed: () => _load(),
              child: const Text('重试'),
            ),
          ),
        ],
      );
    }
    if (_items.isEmpty) {
      return ListView(
        padding: const EdgeInsets.all(24),
        children: [
          const SizedBox(height: 60),
          Text(
            _ctl.adultView
                ? '成人视图下暂无内容，检查成人站源是否已安装启用。'
                : '当前分类下暂无内容，换一个分类或下拉刷新。',
            textAlign: TextAlign.center,
            style: theme.textTheme.bodyMedium,
          ),
        ],
      );
    }
    return LayoutBuilder(
      builder: (context, constraints) {
        final padding = constraints.maxWidth >= 900 ? 24.0 : 14.0;
        return CustomScrollView(
          controller: _scroll,
          physics: const AlwaysScrollableScrollPhysics(),
          slivers: [
            if (_warning.isNotEmpty)
              SliverToBoxAdapter(
                child: Padding(
                  padding: EdgeInsets.fromLTRB(padding, 8, padding, 0),
                  child: Text(
                    _warning,
                    style: theme.textTheme.bodySmall?.copyWith(
                      color: theme.colorScheme.onSurfaceVariant,
                    ),
                  ),
                ),
              ),
            SliverPadding(
              padding: EdgeInsets.fromLTRB(padding, 12, padding, 16),
              sliver: SliverGrid(
                gridDelegate: dramaXGridDelegate(
                  context,
                  constraints.maxWidth - 2 * padding,
                ),
                delegate: SliverChildBuilderDelegate((context, index) {
                  final item = _items[index];
                  if (item.sources.isEmpty) return const SizedBox.shrink();
                  return DramaXCard(
                    key: ValueKey('aggregate-card-${item.key}'),
                    drama: item.sources.first.toDrama(item.title),
                    repository: widget.repository,
                    onTap: () => _openItem(item),
                    sourceCount: item.sourceCount,
                    badge: ContentKind.labels[item.contentKind],
                  );
                }, childCount: _items.length),
              ),
            ),
            SliverToBoxAdapter(
              child: Padding(
                padding: const EdgeInsets.only(bottom: 28),
                child: Center(
                  child: _loadingMore
                      ? const CircularProgressIndicator()
                      : _hasMore
                      ? OutlinedButton.icon(
                          onPressed: () => _load(more: true),
                          icon: const Icon(Icons.expand_more),
                          label: const Text('加载更多'),
                        )
                      : const SizedBox.shrink(),
                ),
              ),
            ),
          ],
        );
      },
    );
  }
}

/// 独立打开用的聚合页（自带 Scaffold 与顶栏「切换」按钮）。
class AggregatedHomeScreen extends StatefulWidget {
  const AggregatedHomeScreen({
    super.key,
    required this.repository,
    required this.store,
    this.sources = const [],
  });

  final AppRepository repository;
  final LocalStore store;
  final List<SourceSite> sources;

  @override
  State<AggregatedHomeScreen> createState() => _AggregatedHomeScreenState();
}

class _AggregatedHomeScreenState extends State<AggregatedHomeScreen> {
  final _controller = AggregateController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  Future<void> _toggle() async {
    if (!_controller.adultView) {
      final confirmed = await showDialog<bool>(
        context: context,
        builder: (context) => AlertDialog(
          title: const Text('切换到成人视图？'),
          content: const Text('成人视图只聚合标记为成人的站源，可随时切回普通视图。'),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('取消'),
            ),
            FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('切换'),
            ),
          ],
        ),
      );
      if (confirmed != true) return;
    }
    _controller.toggleView();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: _controller,
    builder: (context, _) => Scaffold(
      appBar: AppBar(
        title: Text(_controller.adultView ? '聚合 · 成人' : '聚合'),
        actions: [
          TextButton.icon(
            key: const ValueKey('aggregate-toggle-view'),
            onPressed: _toggle,
            icon: Icon(
              _controller.adultView ? Icons.visibility_off_outlined : Icons.swap_horiz,
              size: 18,
            ),
            label: Text(_controller.adultView ? '切回' : '切换'),
          ),
          const SizedBox(width: 8),
        ],
      ),
      body: AggregatedHomeView(
        repository: widget.repository,
        store: widget.store,
        controller: _controller,
        sources: widget.sources,
      ),
    ),
  );
}

/// 顶栏「切换」按钮：普通 ⇄ 成人视图，进入前需要一次确认。
class AggregateToggleButton extends StatelessWidget {
  const AggregateToggleButton({super.key, required this.controller});

  final AggregateController controller;

  Future<void> _toggle(BuildContext context) async {
    if (!controller.adultView) {
      final confirmed = await showDialog<bool>(
        context: context,
        builder: (context) => AlertDialog(
          title: const Text('切换到成人视图？'),
          content: const Text('成人视图只聚合标记为成人的站源，可随时切回普通视图。'),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('取消'),
            ),
            FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('切换'),
            ),
          ],
        ),
      );
      if (confirmed != true) return;
    }
    controller.toggleView();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: controller,
    builder: (context, _) => TextButton.icon(
      key: const ValueKey('aggregate-toggle-view'),
      onPressed: () => _toggle(context),
      icon: Icon(
        controller.adultView ? Icons.visibility_off_outlined : Icons.swap_horiz,
        size: 18,
      ),
      label: Text(controller.adultView ? '切回' : '切换'),
    ),
  );
}
