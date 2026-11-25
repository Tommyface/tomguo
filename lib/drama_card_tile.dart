import 'package:flutter/material.dart';

import 'core_bridge.dart';
import 'drama_actions.dart';
import 'dramax_card.dart';
import 'models.dart';

/// 剧集网格里的单张卡片。
///
/// 从首页抽出来的原因同样是重建范围：首页 setState 时整棵 build 重跑，
/// 卡片数量通常几十张，每张都要重新算跟随状态、选择态和角标。独立成
/// widget 之后，卡片自身的构造只依赖传入的几个值，父级重建时 Dart 能
/// 按引用比较直接跳过没变的子树。
class DramaCardTile extends StatelessWidget {
  const DramaCardTile({
    super.key,
    required this.drama,
    required this.repository,
    required this.onTap,
    required this.onMore,
    this.onLongPress,
    this.selected,
    this.followBadge,
  });

  final Drama drama;
  final AppRepository repository;
  final VoidCallback onTap;
  final VoidCallback onMore;
  final VoidCallback? onLongPress;

  /// null 表示不在选择模式，卡片不显示选中态。
  final bool? selected;

  /// 追剧角标：状态 + 更新提示，没有追这部剧时为 null。
  final String? followBadge;

  @override
  Widget build(BuildContext context) => DramaXCard(
    key: ValueKey(drama.id),
    drama: drama,
    repository: repository,
    onTap: onTap,
    onLongPress: onLongPress,
    onMore: onMore,
    actions: DramaActionButton(drama: drama, onPressed: onMore),
    selected: selected,
    badge: followBadge,
  );
}
