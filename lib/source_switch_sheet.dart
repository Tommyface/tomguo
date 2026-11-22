import 'package:flutter/material.dart';

import 'design_tokens.dart';
import 'models.dart';

/// 切换站源底部弹层（对齐设计稿「全部站源」网格）。
///
/// 与 [showSourceSheet]（播放页换同名播放源）不同：这里切换的是「站源组」，
/// 决定首页目录/搜索用哪个站，结构为「内置站源 / 导入的站源」两组网格胶囊。
/// - 内置组：随应用分发的站源（SourceSite.allValues）。
/// - 导入组：用户安装的 JS 模块 / 自定义源（moduleValues / customValues）。
/// 点击胶囊即切换并关闭；底部「管理站源」进入站源管理页。
Future<SourceGroup?> showSourceSwitchSheet(
  BuildContext context, {
  required List<SourceGroup> groups,
  required String currentGroupId,
  required Set<String> builtinIds,
  required VoidCallback onManage,
}) {
  final builtin = <SourceGroup>[];
  final imported = <SourceGroup>[];
  for (final group in groups) {
    if (group.id == 'all') continue;
    final isImported = group.sources.any(
      (site) => !builtinIds.contains(site.id),
    );
    (isImported ? imported : builtin).add(group);
  }
  return showModalBottomSheet<SourceGroup>(
    context: context,
    isScrollControlled: true,
    backgroundColor: Colors.transparent,
    barrierColor: Colors.black.withValues(alpha: .66),
    builder: (_) => _SourceSwitchSheet(
      groups: groups,
      builtin: builtin,
      imported: imported,
      currentGroupId: currentGroupId,
      onManage: onManage,
    ),
  );
}

class _SourceSwitchSheet extends StatelessWidget {
  const _SourceSwitchSheet({
    required this.groups,
    required this.builtin,
    required this.imported,
    required this.currentGroupId,
    required this.onManage,
  });

  final List<SourceGroup> groups;
  final List<SourceGroup> builtin;
  final List<SourceGroup> imported;
  final String currentGroupId;
  final VoidCallback onManage;

  @override
  Widget build(BuildContext context) {
    final height = MediaQuery.sizeOf(context).height;
    final allGroup = groups.where((group) => group.id == 'all').firstOrNull;
    return Container(
      constraints: BoxConstraints(maxHeight: height * 0.82),
      decoration: const BoxDecoration(
        color: Color(0xFF17171B),
        borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
      ),
      child: SafeArea(
        top: false,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const SizedBox(height: 10),
            Container(
              width: 36,
              height: 4,
              decoration: BoxDecoration(
                color: const Color(0xFF3A3A40),
                borderRadius: BorderRadius.circular(2),
              ),
            ),
            const SizedBox(height: 14),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 18),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.baseline,
                textBaseline: TextBaseline.alphabetic,
                children: [
                  const Text(
                    '全部站源',
                    style: TextStyle(
                      fontSize: 20,
                      fontWeight: FontWeight.w800,
                      color: DramaX.t1,
                    ),
                  ),
                  const SizedBox(width: 10),
                  Text(
                    '共 ${groups.length - (allGroup == null ? 0 : 1)} 个',
                    style: const TextStyle(fontSize: 12, color: DramaX.t2),
                  ),
                  const Spacer(),
                  IconButton(
                    tooltip: '关闭',
                    onPressed: () => Navigator.pop(context),
                    icon: const Icon(Icons.close_rounded, color: DramaX.t2),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 6),
            Flexible(
              child: SingleChildScrollView(
                padding: const EdgeInsets.fromLTRB(18, 4, 18, 0),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (allGroup != null) ...[
                      _sectionTitle('快速切换', DramaX.brand),
                      const SizedBox(height: 8),
                      _widePill(allGroup, context),
                      const SizedBox(height: 16),
                    ],
                    if (builtin.isNotEmpty) ...[
                      _sectionTitle('内置站源', DramaX.ok),
                      const SizedBox(height: 8),
                      _grid(builtin, builtin: true),
                    ],
                    if (imported.isNotEmpty) ...[
                      const SizedBox(height: 16),
                      _sectionTitle('导入的站源', DramaX.purple),
                      const SizedBox(height: 8),
                      _grid(imported, builtin: false),
                    ],
                  ],
                ),
              ),
            ),
            const SizedBox(height: 12),
            Padding(
              padding: const EdgeInsets.fromLTRB(18, 12, 18, 20),
              child: Row(
                children: [
                  Expanded(
                    child: OutlinedButton.icon(
                      key: const ValueKey('source-switch-manage'),
                      onPressed: onManage,
                      icon: const Icon(Icons.tune_rounded, size: 16),
                      label: const Text('管理站源'),
                      style: OutlinedButton.styleFrom(
                        foregroundColor: DramaX.brand2,
                        side: BorderSide(
                          color: DramaX.brand.withValues(alpha: .4),
                        ),
                        padding: const EdgeInsets.symmetric(vertical: 12),
                        shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(DramaX.rLg),
                        ),
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _sectionTitle(String text, Color color) => Row(
    children: [
      Container(
        width: 3,
        height: 13,
        decoration: BoxDecoration(
          color: color,
          borderRadius: BorderRadius.circular(2),
        ),
      ),
      const SizedBox(width: 7),
      Text(
        text,
        style: const TextStyle(
          fontSize: 12,
          fontWeight: FontWeight.w700,
          color: DramaX.t2,
        ),
      ),
    ],
  );

  Widget _grid(List<SourceGroup> items, {required bool builtin}) =>
      LayoutBuilder(
        builder: (context, constraints) {
          const spacing = 10.0;
          final width = (constraints.maxWidth - spacing * 2) / 3;
          return Wrap(
            spacing: spacing,
            runSpacing: spacing,
            children: [
              for (final group in items)
                SizedBox(
                  width: width,
                  child: _pill(group, context, builtin: builtin),
                ),
            ],
          );
        },
      );

  Widget _widePill(SourceGroup group, BuildContext context) {
    final selected = group.id == currentGroupId;
    return SizedBox(
      height: 46,
      child: _pillStyle(
        context: context,
        selected: selected,
        onTap: () => Navigator.pop(context, group),
        child: Text(
          group.name,
          textAlign: TextAlign.center,
          style: TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w700,
            color: selected ? Colors.white : DramaX.t1,
          ),
        ),
      ),
    );
  }

  Widget _pill(SourceGroup group, BuildContext context, {required bool builtin}) {
    final selected = group.id == currentGroupId;
    return SizedBox(
      height: 48,
      child: _pillStyle(
        context: context,
        selected: selected,
        onTap: () => Navigator.pop(context, group),
        child: Stack(
          alignment: Alignment.center,
          children: [
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 20),
              child: Text(
                group.name,
                textAlign: TextAlign.center,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                  color: selected ? Colors.white : DramaX.t1,
                ),
              ),
            ),
            Positioned(
              right: 6,
              bottom: 4,
              child: _typeBadge(builtin),
            ),
            if (selected)
              const Positioned(
                left: 5,
                top: 5,
                child: Icon(
                  Icons.check_circle_rounded,
                  size: 13,
                  color: Colors.white,
                ),
              ),
          ],
        ),
      ),
    );
  }

  Widget _typeBadge(bool builtin) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 1),
    decoration: BoxDecoration(
      color: builtin
          ? DramaX.ok.withValues(alpha: .18)
          : DramaX.purple.withValues(alpha: .18),
      borderRadius: BorderRadius.circular(4),
    ),
    child: Text(
      builtin ? '内置' : '导入',
      style: TextStyle(
        fontSize: 8,
        fontWeight: FontWeight.w800,
        color: builtin ? DramaX.ok : DramaX.purple,
      ),
    ),
  );

  Widget _pillStyle({
    required BuildContext context,
    required bool selected,
    required VoidCallback onTap,
    required Widget child,
  }) =>
      Material(
        color: Colors.transparent,
        child: InkWell(
          onTap: onTap,
          borderRadius: BorderRadius.circular(DramaX.rLg),
          child: AnimatedContainer(
            duration: const Duration(milliseconds: 150),
            decoration: BoxDecoration(
              color: selected ? null : const Color(0xFF202025),
              gradient: selected
                  ? const LinearGradient(
                      colors: [DramaX.brand, Color(0xFFFF5F79)],
                    )
                  : null,
              border: Border.all(
                color: selected ? DramaX.brand : const Color(0xFF303038),
              ),
              borderRadius: BorderRadius.circular(DramaX.rLg),
              boxShadow: selected
                  ? [BoxShadow(color: DramaX.brand.withValues(alpha: .3), blurRadius: 16)]
                  : null,
            ),
            child: child,
          ),
        ),
      );
}
