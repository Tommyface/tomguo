import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';

import 'core_bridge.dart';
import 'design_tokens.dart';
import 'local_store.dart';
import 'models.dart';
import 'source_module.dart';
import 'widgets.dart';

/// 站源模块管理：运行时加载的 JS 站源，安装 / 卸载后无需重装应用。
class ModulesScreen extends StatefulWidget {
  const ModulesScreen({
    super.key,
    required this.repository,
    required this.store,
  });
  final AppRepository repository;
  final LocalStore store;

  @override
  State<ModulesScreen> createState() => _ModulesScreenState();
}

class _ModulesScreenState extends State<ModulesScreen>
    with SingleTickerProviderStateMixin {
  List<SourceModule> _modules = const [];
  String? _error;
  bool _loading = true;

  /// 分类：0=全部，1=内置，2=导入。
  int _category = 0;
  late final TabController _tabController;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 3, vsync: this);
    _tabController.addListener(() {
      if (!_tabController.indexIsChanging && mounted) {
        setState(() => _category = _tabController.index);
      }
    });
    _load();
  }

  @override
  void dispose() {
    _tabController.dispose();
    super.dispose();
  }

  List<SourceModule> get _visibleModules => switch (_category) {
    1 => _modules.where((module) => module.builtin).toList(),
    2 => _modules.where((module) => !module.builtin).toList(),
    _ => _modules,
  };

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final modules = await widget.repository.modules();
      if (!mounted) return;
      setState(() {
        _modules = modules;
        _loading = false;
      });
    } catch (error) {
      if (!mounted) return;
      setState(() {
        _error = error.toString();
        _loading = false;
      });
    }
  }

  Future<void> _install() async {
    final picked = await FilePicker.pickFile(
      type: FileType.custom,
      allowedExtensions: const ['zip'],
    );
    final path = picked?.path;
    if (path == null) return;
    try {
      final module = await widget.repository.installModule(path);
      if (!mounted) return;
      await _syncSourceSites();
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text('已安装 ${module.name}')));
      await _load();
    } catch (error) {
      if (!mounted) return;
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text('安装失败：$error')));
    }
  }

  /// 安装或卸载后同步到 Dart 侧可见站源列表。
  Future<void> _syncSourceSites() async {
    try {
      final modules = await widget.repository.modules();
      SourceSite.registerModules(
        modules
            .where((module) => module.installed)
            .map(
              (module) => SourceSite(
                module.id,
                module.name,
                module.description.isEmpty ? '站源模块' : module.description,
              ),
            ),
      );
    } catch (_) {}
  }

  /// 启用/停用某个站源；停用后它不出现在站源列表、搜索与换源里。
  Future<void> _toggle(SourceModule module, bool value) async {
    final current = widget.store.enabledModules ?? await _allIds();
    final next = Set<String>.from(current);
    if (value) {
      next.add(module.id);
    } else {
      next.remove(module.id);
    }
    await widget.store.saveEnabledModules(next);
    await _syncSourceSites();
    if (mounted) setState(() {});
  }

  /// 首次操作时把所有已安装模块视为启用。
  Future<Set<String>> _allIds() async {
    final modules = await widget.repository.modules();
    return {
      for (final module in modules)
        if (module.installed) module.id,
    };
  }

  Future<void> _remove(SourceModule module) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text('卸载 ${module.name}？'),
        content: const Text('卸载后该站源的剧集不会出现在剧库中，已下载的视频保留。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('卸载'),
          ),
        ],
      ),
    );
    if (confirmed != true) return;
    try {
      await widget.repository.removeModule(module.id);
      await _syncSourceSites();
      await _load();
    } catch (error) {
      if (!mounted) return;
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text('卸载失败：$error')));
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Scaffold(
      appBar: AppBar(
        title: const Text('站源模块'),
        actions: [
          IconButton(
            tooltip: '安装模块',
            onPressed: _install,
            icon: const Icon(Icons.add),
          ),
        ],
        bottom: TabBar(
          controller: _tabController,
          tabs: [
            Tab(text: '全部 (${_modules.length})'),
            Tab(
              text: '内置 (${_modules.where((m) => m.builtin).length})',
            ),
            Tab(
              text: '导入 (${_modules.where((m) => !m.builtin).length})',
            ),
          ],
        ),
      ),
      body: RefreshIndicator(
        onRefresh: _load,
        child: _body(theme),
      ),
    );
  }

  Widget _body(ThemeData theme) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_error != null) {
      return ListView(
        padding: const EdgeInsets.all(24),
        children: [
          Text('读取模块失败：$_error', textAlign: TextAlign.center),
          const SizedBox(height: 16),
          Center(child: FilledButton(onPressed: _load, child: const Text('重试'))),
        ],
      );
    }
    if (_modules.isEmpty) {
      return ListView(
        padding: const EdgeInsets.all(24),
        children: [
          Icon(
            Icons.extension_outlined,
            size: 56,
            color: theme.colorScheme.outline,
          ),
          const SizedBox(height: 16),
          Text(
            '还没有安装站源模块',
            textAlign: TextAlign.center,
            style: theme.textTheme.titleMedium,
          ),
          const SizedBox(height: 8),
          Text(
            '站源模块是运行时加载的 JS 脚本，安装后立即生效，不需要重新安装应用。\n'
            '模块压缩包内需包含 manifest.json 与 main.js。',
            textAlign: TextAlign.center,
            style: theme.textTheme.bodySmall,
          ),
          const SizedBox(height: 20),
          Center(
            child: FilledButton.icon(
              onPressed: _install,
              icon: const Icon(Icons.add),
              label: const Text('选择模块压缩包'),
            ),
          ),
        ],
      );
    }
    final visible = _visibleModules;
    if (visible.isEmpty) {
      return ListView(
        padding: const EdgeInsets.all(32),
        children: [
          Icon(
            Icons.extension_off_outlined,
            size: 48,
            color: theme.colorScheme.outline,
          ),
          const SizedBox(height: 14),
          Text(
            _category == 1 ? '没有内置站源' : '还没有导入的站源',
            textAlign: TextAlign.center,
            style: theme.textTheme.titleMedium,
          ),
          const SizedBox(height: 8),
          Text(
            _category == 1
                ? '内置站源随应用分发，无需安装。'
                : '在「设置 › 站源模块」用「+」选择模块压缩包即可导入。',
            textAlign: TextAlign.center,
            style: theme.textTheme.bodySmall,
          ),
        ],
      );
    }
    return ListView.separated(
      itemCount: visible.length,
      separatorBuilder: (_, __) => const Divider(height: 1),
      itemBuilder: (context, index) {
        final module = visible[index];
        final enabled = widget.store.enabledModules?.contains(module.id) ?? true;
        return ListTile(
          leading: const Icon(Icons.extension_outlined),
          title: Row(
            children: [
              Flexible(child: Text(module.name, overflow: TextOverflow.ellipsis)),
              const SizedBox(width: 8),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
                decoration: BoxDecoration(
                  color: module.builtin
                      ? DramaX.ok.withValues(alpha: .16)
                      : DramaX.purple.withValues(alpha: .16),
                  borderRadius: BorderRadius.circular(6),
                ),
                child: Text(
                  module.builtin ? '内置' : '导入',
                  style: TextStyle(
                    fontSize: 10,
                    fontWeight: FontWeight.w800,
                    color: module.builtin ? DramaX.ok : DramaX.purple,
                  ),
                ),
              ),
            ],
          ),
          subtitle: Text(
            [
              module.id,
              if (module.version.isNotEmpty) 'v${module.version}',
              if (module.capabilities.isNotEmpty)
                module.capabilities.map(moduleCapabilityLabel).join('、'),
              if (module.description.isNotEmpty) module.description,
            ].join(' · '),
            maxLines: 3,
            overflow: TextOverflow.ellipsis,
          ),
          trailing: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Switch(
                value: enabled,
                onChanged: (value) => _toggle(module, value),
              ),
              if (!module.builtin)
                IconButton(
                  tooltip: '卸载',
                  onPressed: () => _remove(module),
                  icon: const Icon(Icons.delete_outline),
                ),
            ],
          ),
        );
      },
    );
  }
}
