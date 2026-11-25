import 'dart:async';

import 'package:flutter/material.dart';

import 'search_input.dart';

/// 发现页顶部的搜索面板：搜索框 + 最近搜索。
///
/// 单独成文件不是为了好看，是为了重建范围：首页 setState 时会重建整棵树，
/// 搜索面板一旦内联在首页 build 里，输入法焦点、光标位置、suggestions
/// 下拉都跟着重建。抽成独立 widget 之后，只要参数没变，这一块不重建。
class CatalogSearchPanel extends StatelessWidget {
  const CatalogSearchPanel({
    super.key,
    required this.controller,
    required this.groupKey,
    required this.hint,
    required this.visible,
    required this.autofocus,
    required this.onChanged,
    required this.onCancel,
    required this.onSearch,
    required this.recentSearches,
    required this.onClearRecent,
    this.suggestions,
  });

  final TextEditingController controller;

  /// 站源分组标识：换源时搜索框要跟着重置，用 key 强制重建。
  final String groupKey;
  final String hint;
  final bool visible;
  final bool autofocus;
  final ValueChanged<String> onChanged;
  final VoidCallback onCancel;
  final ValueChanged<String> onSearch;
  final List<String> recentSearches;
  final Future<void> Function() onClearRecent;
  final Future<List<String>> Function(String)? suggestions;

  @override
  Widget build(BuildContext context) {
    if (!visible) return const SizedBox.shrink();
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 4, 16, 4),
          child: SearchInput(
            key: ValueKey('search-$groupKey'),
            controller: controller,
            autofocus: autofocus,
            hint: hint,
            suggestions: suggestions,
            onChanged: onChanged,
            onCancel: onCancel,
            onSearch: onSearch,
          ),
        ),
        if (controller.text.trim().isEmpty && recentSearches.isNotEmpty)
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
            child: Row(
              children: [
                Expanded(
                  child: SingleChildScrollView(
                    scrollDirection: Axis.horizontal,
                    child: Row(
                      children: [
                        for (final query in recentSearches)
                          Padding(
                            padding: const EdgeInsets.only(right: 8),
                            child: ActionChip(
                              avatar: const Icon(
                                Icons.history_rounded,
                                size: 16,
                              ),
                              label: Text(query),
                              onPressed: () => onSearch(query),
                            ),
                          ),
                      ],
                    ),
                  ),
                ),
                IconButton(
                  tooltip: '清空最近搜索',
                  onPressed: () => unawaited(onClearRecent()),
                  icon: const Icon(Icons.delete_outline_rounded, size: 20),
                ),
              ],
            ),
          ),
      ],
    );
  }
}
