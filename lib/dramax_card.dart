import 'package:flutter/material.dart';

import 'core_bridge.dart';
import 'design_tokens.dart';
import 'models.dart';
import 'widgets.dart';

/// DramaX 风格剧集卡片：3:4 封面 + 左上集数角标 + 右上热度 + 居中播放键。
///
/// 与旧 `DramaTile` 的差别：
/// - 封面比例 3:4（旧 2:3），信息密度更高，一屏能多放半行
/// - 集数/热度做成封面上浮的胶囊，不占正文高度
/// - 正文只留标题 + 分类 + 源数，两行封顶
class DramaXCard extends StatelessWidget {
  const DramaXCard({
    super.key,
    required this.drama,
    required this.repository,
    required this.onTap,
    this.onLongPress,
    this.onMore,
    this.badge,
    this.selected,
    this.sourceCount = 0,
    this.actions,
  });

  final Drama drama;
  final AppRepository repository;
  final VoidCallback onTap;
  final VoidCallback? onLongPress;
  final VoidCallback? onMore;
  final String? badge;
  final bool? selected;

  /// 该剧可用的站源数量，>0 时在卡片上显示「N源可选」。
  final int sourceCount;
  final Widget? actions;

  /// 海报解码宽度：按卡片实际宽度 × 设备像素比，封顶 440。
  /// 原来所有卡片硬编码 440，两列手机上解码出的位图是显示尺寸的 2~3 倍，
  /// 滑一屏要占几十 MB 纹理内存，掉帧就来自这里的解码与上传。
  static int _coverCacheWidth(BuildContext context) {
    final width = MediaQuery.sizeOf(context).width;
    final columns = width < 600 ? 2 : (width / 220).floor().clamp(3, 8);
    final spacing = width < 600 ? 12.0 : 18.0;
    final tile = (width - (columns - 1) * spacing) / columns;
    final dpr = MediaQuery.devicePixelRatioOf(context);
    return (tile * dpr).round().clamp(120, 440);
  }

  static double extentFor(BuildContext context, double width) {
    final scaler = MediaQuery.textScalerOf(context);
    return (width * 4 / 3 + 8 + scaler.scale(13) * 1.35 + 6 + scaler.scale(10) * 1.4)
        .ceilToDouble();
  }

  @override
  Widget build(BuildContext context) {
    final gradient = DramaX.posterFor(drama.id);
    // 每张卡片独立重绘边界：滑动时一处的脏区不扩散到整屏卡片。
    return RepaintBoundary(
      child: Material(
      color: DramaX.card,
      borderRadius: BorderRadius.circular(DramaX.rLg - 2),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap,
        onLongPress: onLongPress,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            AspectRatio(
              aspectRatio: 3 / 4,
              child: Stack(
                fit: StackFit.expand,
                children: [
                  // 海报底与底部渐变遮罩都由 DramaCover 内部提供，这里不再
                  // 各铺一层 —— 两层渐变叠在一起既看不出差别，又让每张卡
                  // 多两个 RenderObject，一屏六张就是十二层多余的合成。
                  DramaCover(
                    drama: drama,
                    repository: repository,
                    radius: 0,
                    background: gradient,
                    cacheWidth: _coverCacheWidth(context),
                  ),
                  if (drama.episodes > 0)
                    Positioned(
                      top: 8,
                      left: 8,
                      child: _Pill(
                        text: '${drama.episodes}集',
                        background: const Color(0x8C000000),
                        border: const Color(0x33FFFFFF),
                      ),
                    ),
                  if (_hotLabel.isNotEmpty)
                    Positioned(
                      top: 8,
                      right: 8,
                      child: _Pill(
                        text: _hotLabel,
                        background: DramaX.brand.withValues(alpha: .85),
                      ),
                    ),
                  if (badge != null && badge!.isNotEmpty)
                    Positioned(
                      left: 8,
                      right: 8,
                      bottom: 8,
                      child: _Pill(
                        text: badge!,
                        background: const Color(0xB8000000),
                        maxLines: 2,
                      ),
                    ),
                  if (actions != null && selected == null)
                    Positioned(top: 0, right: 0, child: actions!),
                  if (selected != null)
                    Positioned(
                      top: 8,
                      right: 8,
                      child: CircleAvatar(
                        radius: 15,
                        backgroundColor: selected!
                            ? DramaX.brand
                            : const Color(0xA3000000),
                        child: Icon(
                          selected! ? Icons.check_rounded : Icons.circle_outlined,
                          size: 19,
                          color: Colors.white,
                        ),
                      ),
                    )
                  else
                    const Center(child: _PlayBadge()),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(10, 8, 10, 10),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    drama.title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w700,
                      color: DramaX.t1,
                      height: 1.3,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Row(
                    children: [
                      Expanded(
                        child: Text(
                          drama.category.isEmpty ? '短剧' : drama.category,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(fontSize: 10, color: DramaX.t3),
                        ),
                      ),
                      if (sourceCount > 1)
                        Container(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 5,
                            vertical: 2,
                          ),
                          decoration: BoxDecoration(
                            color: DramaX.ok.withValues(alpha: .12),
                            borderRadius: BorderRadius.circular(4),
                          ),
                          child: Text(
                            '$sourceCount源可选',
                            style: const TextStyle(
                              fontSize: 9,
                              color: DramaX.ok,
                              fontWeight: FontWeight.w600,
                            ),
                          ),
                        ),
                    ],
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    ),
    );
  }

  String get _hotLabel {
    if (drama.heat.isNotEmpty) {
      return drama.heat.startsWith('热') ? drama.heat : '热 ${drama.heat}';
    }
    if (drama.releaseStatus == 'finished' || drama.releaseStatus == 'completed') {
      return '完结';
    }
    if (drama.vip) return 'VIP';
    return '';
  }
}

class _Pill extends StatelessWidget {
  const _Pill({
    required this.text,
    required this.background,
    this.border,
    this.maxLines = 1,
  });

  final String text;
  final Color background;
  final Color? border;
  final int maxLines;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 3),
    decoration: BoxDecoration(
      color: background,
      borderRadius: BorderRadius.circular(5),
      border: border == null ? null : Border.all(color: border!),
    ),
    child: Text(
      text,
      maxLines: maxLines,
      overflow: TextOverflow.ellipsis,
      style: const TextStyle(
        fontSize: 9,
        fontWeight: FontWeight.w600,
        color: Colors.white,
        height: 1.35,
      ),
    ),
  );
}

class _PlayBadge extends StatelessWidget {
  const _PlayBadge();

  @override
  Widget build(BuildContext context) => Container(
    width: 44,
    height: 44,
    decoration: BoxDecoration(
      shape: BoxShape.circle,
      color: const Color(0x6B08080C),
      border: Border.all(color: const Color(0xBFFFFFFF), width: 1.5),
    ),
    child: const Icon(Icons.play_arrow_rounded, size: 24, color: Colors.white),
  );
}

/// DramaX 网格参数：手机 2 列（设计稿），宽屏按宽度自适应。
SliverGridDelegate dramaXGridDelegate(BuildContext context, double width) {
  final columns = width < 600 ? 2 : (width / 220).floor().clamp(3, 8);
  final spacing = width < 600 ? 12.0 : 18.0;
  final tileWidth = (width - (columns - 1) * spacing) / columns;
  return SliverGridDelegateWithFixedCrossAxisCount(
    crossAxisCount: columns,
    mainAxisSpacing: width < 600 ? 12 : 20,
    crossAxisSpacing: spacing,
    mainAxisExtent: DramaXCard.extentFor(context, tileWidth),
  );
}
