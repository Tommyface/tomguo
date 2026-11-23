/// 站源模块：运行时加载的 JS 站源。
class SourceModule {
  const SourceModule({
    required this.id,
    required this.name,
    this.version = '',
    this.description = '',
    this.hosts = const [],
    this.capabilities = const [],
    this.adult = false,
    this.installed = false,
    this.builtin = false,
    this.error = '',
  });

  factory SourceModule.fromJson(Map<String, dynamic> json) => SourceModule(
    id: json['id'] as String? ?? '',
    name: json['name'] as String? ?? '',
    version: json['version'] as String? ?? '',
    description: json['description'] as String? ?? '',
    hosts: (json['hosts'] as List? ?? const []).whereType<String>().toList(),
    capabilities: (json['capabilities'] as List? ?? const [])
        .whereType<String>()
        .toList(),
    adult: json['adult'] == true,
    installed: json['installed'] == true,
    builtin: json['builtin'] == true,
    error: json['error'] as String? ?? '',
  );

  final String id;
  final String name;
  final String version;
  final String description;
  final List<String> hosts;
  final List<String> capabilities;
  final bool adult;
  final bool installed;
  final bool builtin;
  final String error;
}

const moduleCapabilityNames = <String, String>{
  'search': '搜索',
  'suggestions': '搜索联想',
  'recommendations': '推荐',
  'danmaku': '弹幕',
  'rankings': '榜单',
  'cover': '封面',
  'metadata': '补充信息',
  'catalogState': '目录状态',
};

String moduleCapabilityLabel(String name) =>
    moduleCapabilityNames[name] ?? name;

/// 三段版本比较，用于提示模块有新版本。
bool isNewerModuleVersion(String latest, String installed) {
  List<int>? parse(String version) {
    final parts = version.split('.').map(int.tryParse).toList();
    return parts.length == 3 && !parts.contains(null)
        ? parts.cast<int>()
        : null;
  }

  final next = parse(latest);
  final current = parse(installed);
  if (next == null) return false;
  if (current == null) return latest != installed;
  for (var index = 0; index < 3; index++) {
    if (next[index] != current[index]) return next[index] > current[index];
  }
  return false;
}

/// 有内容来自未安装的站源时，提示用户去安装而不是静默丢失。
String? missingSourceHint(Map<String, int> counts, String unit) {
  final total = counts.values.fold(0, (sum, count) => sum + count);
  return total == 0 ? null : '有 $total $unit来自未安装的站源，在 设置 › 站源模块 安装后恢复显示';
}

/// 模块清单里的一条可安装记录（只用于展示，安装由原生层做完整校验）。
class RegistryEntry {
  const RegistryEntry({
    required this.id,
    required this.name,
    required this.version,
    required this.description,
    required this.adult,
  });
  final String id;
  final String name;
  final String version;
  final String description;
  final bool adult;

  factory RegistryEntry.fromJson(Map<String, dynamic> json) => RegistryEntry(
    id: (json['id'] ?? '').toString(),
    name: (json['name'] ?? json['id'] ?? '').toString(),
    version: (json['version'] ?? '').toString(),
    description: (json['description'] ?? '').toString(),
    adult: json['adult'] == true,
  );
}
