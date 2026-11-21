import 'package:flutter/material.dart';

import 'app_theme.dart';

class AppBottomNavigation extends StatelessWidget {
  const AppBottomNavigation({
    super.key,
    required this.selectedIndex,
    required this.onDestinationSelected,
    required this.destinations,
  });

  final int selectedIndex;
  final ValueChanged<int> onDestinationSelected;
  final List<NavigationDestination> destinations;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final colors = theme.colorScheme;
    // 影院编辑风：选中态用琥珀金，与主色暖橙红形成品牌双色。
    final accent = CinemaTokens.amber;
    return Material(
      color: theme.scaffoldBackgroundColor,
      child: SafeArea(
        top: false,
        child: DecoratedBox(
          decoration: BoxDecoration(
            border: Border(
              top: BorderSide(color: colors.outlineVariant, width: .5),
            ),
          ),
          child: Row(
            children: [
              for (final (index, destination) in destinations.indexed)
                Expanded(
                  child: Semantics(
                    container: true,
                    button: true,
                    selected: index == selectedIndex,
                    child: Tooltip(
                      message: destination.label,
                      excludeFromSemantics: true,
                      child: InkWell(
                        key: ValueKey('bottom-nav-$index'),
                        onTap: () => onDestinationSelected(index),
                        child: ConstrainedBox(
                          constraints: const BoxConstraints(
                            minHeight: 68,
                            minWidth: 48,
                          ),
                          child: Padding(
                            padding: const EdgeInsets.symmetric(
                              horizontal: 4,
                              vertical: 9,
                            ),
                            child: Column(
                              mainAxisSize: MainAxisSize.min,
                              children: [
                                // 顶部指示条：选中时由暖橙红渐变到琥珀金，并轻微加宽。
                                AnimatedContainer(
                                  duration: const Duration(milliseconds: 220),
                                  curve: Curves.easeOutCubic,
                                  width: index == selectedIndex ? 26 : 0,
                                  height: 3,
                                  decoration: BoxDecoration(
                                    gradient: index == selectedIndex
                                        ? const LinearGradient(colors: [
                                            CinemaTokens.ember,
                                            CinemaTokens.amber,
                                          ])
                                        : null,
                                    color: index == selectedIndex
                                        ? null
                                        : Colors.transparent,
                                    borderRadius: BorderRadius.circular(1.5),
                                  ),
                                ),
                                const SizedBox(height: 6),
                                // 图标：选中时轻微放大，给出克制的触感反馈。
                                AnimatedScale(
                                  scale: index == selectedIndex ? 1.08 : 1.0,
                                  duration: const Duration(milliseconds: 200),
                                  curve: Curves.easeOutCubic,
                                  child: IconTheme(
                                    data: IconThemeData(
                                      size: 24,
                                      color: index == selectedIndex
                                          ? accent
                                          : colors.onSurfaceVariant,
                                    ),
                                    child: index == selectedIndex
                                        ? destination.selectedIcon ??
                                              destination.icon
                                        : destination.icon,
                                  ),
                                ),
                                const SizedBox(height: 4),
                                Text(
                                  destination.label,
                                  textAlign: TextAlign.center,
                                  maxLines: 2,
                                  overflow: TextOverflow.ellipsis,
                                  style: TextStyle(
                                    fontSize: 12,
                                    height: 1.2,
                                    fontWeight: index == selectedIndex
                                        ? FontWeight.w700
                                        : FontWeight.w500,
                                    color: index == selectedIndex
                                        ? accent
                                        : colors.onSurfaceVariant,
                                  ),
                                ),
                              ],
                            ),
                          ),
                        ),
                      ),
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}
