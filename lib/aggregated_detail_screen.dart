import 'dart:async';

import 'package:flutter/material.dart';

import 'core_bridge.dart';
import 'design_tokens.dart';
import 'detail_screen.dart';
import 'local_store.dart';
import 'models.dart';

/// 一部聚合作品在某个站源上的加载状态。
class _SourceState {
  _SourceState(this.source) : episodes = source.episodes;

  final AggregatedSource source;
  /// 初值直接取聚合结果里的集数。
  ///
  /// 聚合阶段 Go 侧已经按源算过 episodes，进详情页却又把这里置 0 重新
  /// 拉一遍，用户看到的就是「所有行一起转圈」。现在先用现成数字渲染，
  /// 后台校准到新值再替换；loading 只表示「正在校准」。
  int episodes;
  String warning = '';
  String error = '';
  bool loading = false;

  Drama toDrama(String fallbackTitle) => Drama(
    id: source.dramaId,
    source: source.source,
    sourceId: source.sourceId,
    title: source.title.isEmpty ? fallbackTitle : source.title,
    cover: source.cover,
    episodes: episodes > 0 ? episodes : source.episodes,
    category: source.category,
    vip: source.vip,
  );
}

/// 聚合作品详情：同一部剧在不同站源的集数与可用性逐源列出。
///
/// 点某一源后进入原来的 [DetailScreen]，播放链路完全不变。
class AggregatedDetailScreen extends StatefulWidget {
  const AggregatedDetailScreen({
    super.key,
    required this.item,
    required this.repository,
    required this.store,
    this.adultView = false,
  });

  final AggregatedDrama item;
  final AppRepository repository;
  final LocalStore store;
  final bool adultView;

  @override
  State<AggregatedDetailScreen> createState() => _AggregatedDetailScreenState();
}

class _AggregatedDetailScreenState extends State<AggregatedDetailScreen> {
  late final List<_SourceState> _states = [
    for (final source in widget.item.sources) _SourceState(source),
  ];
  bool _refreshing = false;

  @override
  void initState() {
    super.initState();
    unawaited(_refreshAll());
  }

  /// 后台校准各源集数：3 并发，单源失败只标该行，不影响其他源。
  ///
  /// 只在数字真的变了才 setState，避免每个源完成都整页重建一次。
  Future<void> _refreshAll() async {
    if (mounted) setState(() => _refreshing = true);
    var index = 0;
    Future<void> worker() async {
      while (index < _states.length) {
        final state = _states[index++];
        try {
          final detail = await widget.repository.detail(
            state.toDrama(widget.item.title),
          );
          if (!mounted) return;
          final next = detail.episodes.length > 0
              ? detail.episodes.length
              : state.source.episodes;
          if (next == state.episodes &&
              state.warning == detail.warning &&
              state.error.isEmpty &&
              !state.loading) {
            continue;   // 没有变化就不重建
          }
          setState(() {
            state.episodes = next;
            state.warning = detail.warning;
            state.error = '';
            state.loading = false;
          });
        } catch (error) {
          if (!mounted) return;
          setState(() {
            state.error = error is AppFailure ? error.message : '$error';
            state.loading = false;
          });
        }
      }
    }

    await Future.wait(List.generate(_states.length.clamp(1, 3), (_) => worker()));
    if (!mounted) return;
    setState(() => _refreshing = false);
  }

  String _sourceName(String id) {
    for (final site in widget.store.sources) {
      if (site.id == id) return site.name;
    }
    return id;
  }

  void _openSource(_SourceState state) {
    if (state.loading || state.error.isNotEmpty) return;
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => DetailScreen(
          drama: state.toDrama(widget.item.title),
          repository: widget.repository,
          store: widget.store,
        ),
      ),
    );
  }

  int get _bestEpisodes => _states.fold<int>(
    0,
    (best, state) => state.episodes > best ? state.episodes : best,
  );

  int get _playableCount => _states.where((state) => state.error.isEmpty).length;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final item = widget.item;
    return Scaffold(
      appBar: AppBar(
        title: Text(
          item.title,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
        ),
        actions: [
          IconButton(
            key: const ValueKey('aggregate-detail-refresh'),
            tooltip: '重新检测各站源',
            onPressed: _refreshing ? null : _refreshAll,
            icon: _refreshing
                ? const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.refresh),
          ),
          const SizedBox(width: 4),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(16, 12, 16, 28),
        children: [
          _header(theme),
          const SizedBox(height: 20),
          Row(
            children: [
              Text('来源剧集', style: theme.textTheme.titleMedium),
              const SizedBox(width: 8),
              Text(
                '${_playableCount}/${_states.length} 可用',
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
              const Spacer(),
              if (_bestEpisodes > 0)
                Text(
                  '最多 $_bestEpisodes 集',
                  style: theme.textTheme.bodySmall?.copyWith(
                    color: theme.colorScheme.onSurfaceVariant,
                  ),
                ),
            ],
          ),
          const SizedBox(height: 10),
          for (final state in _states) ...[
            _sourceTile(theme, state),
            const SizedBox(height: 10),
          ],
        ],
      ),
    );
  }

  Widget _header(ThemeData theme) {
    final item = widget.item;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        ClipRRect(
          borderRadius: BorderRadius.circular(DramaX.rMd),
          child: SizedBox(
            width: 96,
            height: 128,
            child: item.cover.isEmpty
                ? ColoredBox(
                    color: theme.colorScheme.surfaceContainerHigh,
                    child: const Icon(Icons.movie_outlined),
                  )
                : Image.network(
                    item.cover,
                    fit: BoxFit.cover,
                    errorBuilder: (_, __, ___) => ColoredBox(
                      color: theme.colorScheme.surfaceContainerHigh,
                      child: const Icon(Icons.movie_outlined),
                    ),
                  ),
          ),
        ),
        const SizedBox(width: 14),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                item.title,
                style: theme.textTheme.titleLarge,
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
              ),
              const SizedBox(height: 8),
              Wrap(
                spacing: 8,
                runSpacing: 6,
                children: [
                  _chip(
                    theme,
                    ContentKind.labels[item.contentKind] ?? '短剧',
                    tone: theme.colorScheme.primary,
                  ),
                  _chip(theme, '${item.sourceCount} 个站源'),
                  if (_bestEpisodes > 0) _chip(theme, '共 $_bestEpisodes 集'),
                  if (widget.adultView || item.adult) _chip(theme, '成人'),
                ],
              ),
              const SizedBox(height: 10),
              Text(
                '同一个作品在不同站源的集数与可用性不同，选一个能播的来源进去。',
                style: theme.textTheme.bodySmall?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _chip(ThemeData theme, String label, {Color? tone}) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
    decoration: BoxDecoration(
      color: (tone ?? theme.colorScheme.onSurfaceVariant).withValues(
        alpha: 0.12,
      ),
      borderRadius: BorderRadius.circular(DramaX.rSm),
    ),
    child: Text(
      label,
      style: theme.textTheme.labelSmall?.copyWith(
        color: tone ?? theme.colorScheme.onSurfaceVariant,
      ),
    ),
  );

  Widget _sourceTile(ThemeData theme, _SourceState state) {
    final source = state.source;
    final disabled = state.loading || state.error.isNotEmpty;
    final episodes = state.episodes > 0 ? state.episodes : source.episodes;
    return Material(
      color: theme.colorScheme.surfaceContainerLow,
      borderRadius: BorderRadius.circular(DramaX.rLg - 4),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        key: ValueKey('aggregate-source-${source.source}'),
        onTap: disabled ? null : () => _openSource(state),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
          child: Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Text(
                          _sourceName(source.source),
                          style: theme.textTheme.titleSmall,
                        ),
                        if (source.vip) ...[
                          const SizedBox(width: 6),
                          _chip(theme, 'VIP', tone: theme.colorScheme.tertiary),
                        ],
                      ],
                    ),
                    const SizedBox(height: 4),
                    Text(
                      state.loading
                          ? '正在检测…'
                          : state.error.isNotEmpty
                          ? '不可用：${state.error}'
                          : episodes > 0
                          ? '$episodes 集'
                          : '集数未知',
                      style: theme.textTheme.bodySmall?.copyWith(
                        color: state.error.isNotEmpty
                            ? theme.colorScheme.error
                            : theme.colorScheme.onSurfaceVariant,
                      ),
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                    if (state.warning.isNotEmpty) ...[
                      const SizedBox(height: 2),
                      Text(
                        state.warning,
                        style: theme.textTheme.bodySmall?.copyWith(
                          color: theme.colorScheme.tertiary,
                        ),
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ],
                  ],
                ),
              ),
              const SizedBox(width: 12),
              if (state.loading)
                const SizedBox(
                  width: 18,
                  height: 18,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              else if (state.error.isNotEmpty)
                Icon(
                  Icons.error_outline,
                  size: 20,
                  color: theme.colorScheme.error,
                )
              else
                Icon(
                  Icons.chevron_right,
                  color: theme.colorScheme.onSurfaceVariant,
                ),
            ],
          ),
        ),
      ),
    );
  }
}
