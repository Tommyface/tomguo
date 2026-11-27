import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';

import 'core_bridge.dart';
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

class _ModulesScreenState extends State<ModulesScreen> {
  List<SourceModule> _modules = const [];
  String? _error;
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

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
      });
    } catch (error) {
      if (!mounted) return;
      setState(() {
        _error = error.toString();
      });
    } finally {
      if (mounted) {
        setState(() {
          _loading = false;
        });
      }
    }
  }

  Future<void> _installFromUrl() async {
    final controller = TextEditingController(
      text: 'https://tommyfac.ccwu.cc/tomguo/registry.json',
    );
    final formKey = GlobalKey<FormState>();
    final result = await showDialog<_RegistryInput>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('从清单地址安装'),
        content: Form(
          key: formKey,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextFormField(
                controller: controller,
                autofocus: true,
                keyboardType: TextInputType.url,
                decoration: const InputDecoration(
                  labelText: '模块清单地址（HTTPS）',
                  hintText: 'https://example.com/tomguo/registry.json',
                ),
                validator: (value) {
                  final text = value?.trim() ?? '';
                  if (!text.startsWith('https://')) return '必须使用 HTTPS 地址';
                  if (Uri.tryParse(text)?.host.isEmpty ?? true) return '地址无效';
                  return null;
                },
              ),
              const SizedBox(height: 8),
              Text(
                '可用地址：tommyfac / tommyface / tommyfa .ccwu.cc/tomguo/registry.json',
                style: Theme.of(context).textTheme.bodySmall,
              ),
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () {
              if (formKey.currentState?.validate() ?? false) {
                Navigator.pop(
                  context,
                  _RegistryInput(
                    controller.text.trim(),
                    widget.store.sourcesUnlocked,
                  ),
                );
              }
            },
            child: const Text('读取并安装'),
          ),
        ],
      ),
    );
    if (result == null) return;
    try {
      final registry = await widget.repository.fetchRegistry(result.url);
      if (!mounted) return;
      final picked = await showDialog<String>(
        context: context,
        builder: (context) => AlertDialog(
          title: const Text('选择要安装的模块'),
          content: SingleChildScrollView(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                for (final entry in registry.entries)
                  ListTile(
                    title: Text(entry.name),
                    subtitle: Text(
                      '${entry.id} · ${entry.version}'
                      '${entry.adult ? ' · 成人' : ''}'
                      '${entry.stateLabel.isEmpty ? '' : ' · ${entry.stateLabel}'}',
                    ),
                    enabled: entry.canInstall,
                    onTap: entry.canInstall
                        ? () => Navigator.pop(context, entry.id)
                        : null,
                  ),
              ],
            ),
          ),
        ),
      );
      if (picked == null) return;
      final module = await widget.repository.installFromRegistry(
        registry.url,
        picked,
        includeAdult: result.includeAdult,
      );
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
      widget.store.notifySourcesChanged();
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
            tooltip: '从清单地址安装',
            onPressed: _installFromUrl,
            icon: const Icon(Icons.link),
          ),
          IconButton(
            tooltip: '安装模块',
            onPressed: _install,
            icon: const Icon(Icons.add),
          ),
        ],
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
              onPressed: _installFromUrl,
              icon: const Icon(Icons.link),
              label: const Text('从清单地址安装'),
            ),
          ),
          const SizedBox(height: 8),
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
    final visible = _modules.where((module) => !module.disabled).toList();
    final hidden = _modules.where((module) => module.disabled).toList();
    return ListView.separated(
      itemCount: visible.length + (hidden.isEmpty ? 0 : 1),
      separatorBuilder: (_, __) => const Divider(height: 1),
      itemBuilder: (context, index) {
        if (index == visible.length) {
          return ListTile(
            leading: const Icon(Icons.visibility_off_outlined),
            title: Text('已停用 ${hidden.length} 个站源'),
            subtitle: Text(
              hidden
                  .map((m) => '${m.name}：${m.disabledReason.isEmpty ? '长期不可用' : m.disabledReason}')
                  .join('\n'),
              maxLines: 6,
              overflow: TextOverflow.ellipsis,
            ),
          );
        }
        final module = visible[index];
        final enabled = widget.store.enabledModules?.contains(module.id) ?? true;
        return ListTile(
          leading: const Icon(Icons.extension_outlined),
          title: Text(module.name),
          subtitle: Text(
            [
              if (module.builtin) '内置',
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

class _RegistryInput {
  const _RegistryInput(this.url, this.includeAdult);
  final String url;
  final bool includeAdult;
}
