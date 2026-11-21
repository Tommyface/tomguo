import 'models.dart';

/// 运行时自定义站源配置。
///
/// 字段与 native/core/custom_sources.go 的 customSourceConfig 一一对应，
/// 通过 JSON 在 Dart 与 Go 之间传递。
class CustomSource {
  const CustomSource({
    required this.name,
    required this.baseUrl,
    required this.type,
    this.id = '',
    this.description = '',
    this.detailPathPattern = '',
    this.playPathPattern = '',
    this.detailUrlTemplate = '',
    this.listUrlTemplate = '',
    this.searchUrlTemplate = '',
    this.apiUrl = '',
    this.aesKey = '',
    this.aesIv = '',
    this.keyIsHex = false,
    this.signKey = '',
    this.signTemplate = '',
    this.catalogPath = '',
    this.detailApiPath = '',
    this.playApiPath = '',
  });

  final String id;
  final String name;
  final String description;
  final String baseUrl;
  final String type; // maccms | aes

  // 苹果CMS
  final String detailPathPattern;
  final String playPathPattern;
  final String detailUrlTemplate;
  final String listUrlTemplate;
  final String searchUrlTemplate;

  // AES
  final String apiUrl;
  final String aesKey;
  final String aesIv;
  final bool keyIsHex;
  final String signKey;
  final String signTemplate;
  final String catalogPath;
  final String detailApiPath;
  final String playApiPath;

  String get effectiveId =>
      id.isEmpty ? 'custom_${_slug(name)}' : id;

  bool get isMaccms => type == 'maccms';

  /// 转成 SourceSite 供 UI 展示。
  SourceSite toSourceSite() => SourceSite(
    effectiveId,
    name,
    description.isEmpty
        ? (isMaccms ? '自定义 · 苹果CMS' : '自定义 · AES 接口')
        : description,
  );

  Map<String, dynamic> toJson() => {
    'id': effectiveId,
    'name': name,
    'description': description,
    'baseUrl': baseUrl,
    'type': type,
    if (isMaccms) ...{
      'detailPathPattern': detailPathPattern,
      'playPathPattern': playPathPattern,
      'detailUrlTemplate': detailUrlTemplate,
      'listUrlTemplate': listUrlTemplate,
      'searchUrlTemplate': searchUrlTemplate,
    } else ...{
      'apiUrl': apiUrl,
      'aesKey': aesKey,
      'aesIv': aesIv,
      'keyIsHex': keyIsHex,
      'signKey': signKey,
      'signTemplate': signTemplate,
      'catalogPath': catalogPath,
      'detailPath': detailApiPath,
      'playApiPath': playApiPath,
    },
  };

  factory CustomSource.fromJson(Map<String, dynamic> json) => CustomSource(
    id: json['id'] as String? ?? '',
    name: json['name'] as String? ?? '',
    description: json['description'] as String? ?? '',
    baseUrl: json['baseUrl'] as String? ?? '',
    type: json['type'] as String? ?? 'maccms',
    detailPathPattern: json['detailPathPattern'] as String? ?? '',
    playPathPattern: json['playPathPattern'] as String? ?? '',
    detailUrlTemplate: json['detailUrlTemplate'] as String? ?? '',
    listUrlTemplate: json['listUrlTemplate'] as String? ?? '',
    searchUrlTemplate: json['searchUrlTemplate'] as String? ?? '',
    apiUrl: json['apiUrl'] as String? ?? '',
    aesKey: json['aesKey'] as String? ?? '',
    aesIv: json['aesIv'] as String? ?? '',
    keyIsHex: json['keyIsHex'] as bool? ?? false,
    signKey: json['signKey'] as String? ?? '',
    signTemplate: json['signTemplate'] as String? ?? '',
    catalogPath: json['catalogPath'] as String? ?? '',
    detailApiPath: json['detailPath'] as String? ?? '',
    playApiPath: json['playApiPath'] as String? ?? '',
  );

  CustomSource copyWith({String? id}) => CustomSource(
    id: id ?? this.id,
    name: name,
    description: description,
    baseUrl: baseUrl,
    type: type,
    detailPathPattern: detailPathPattern,
    playPathPattern: playPathPattern,
    detailUrlTemplate: detailUrlTemplate,
    listUrlTemplate: listUrlTemplate,
    searchUrlTemplate: searchUrlTemplate,
    apiUrl: apiUrl,
    aesKey: aesKey,
    aesIv: aesIv,
    keyIsHex: keyIsHex,
    signKey: signKey,
    signTemplate: signTemplate,
    catalogPath: catalogPath,
    detailApiPath: detailApiPath,
    playApiPath: playApiPath,
  );

  static String _slug(String name) {
    final buffer = StringBuffer();
    for (final rune in name.toLowerCase().runes) {
      if ((rune >= 0x61 && rune <= 0x7a) ||
          (rune >= 0x30 && rune <= 0x39)) {
        buffer.writeCharCode(rune);
      } else {
        buffer.write('_');
      }
    }
    final s = buffer.toString().replaceAll(RegExp(r'^_+|_+$'), '');
    return s.isEmpty ? 'site' : s;
  }

  /// 简单校验，返回错误信息（null 表示通过）。
  String? validate() {
    if (name.trim().isEmpty) return '名称不能为空';
    final uri = Uri.tryParse(baseUrl);
    if (uri == null ||
        !uri.hasScheme ||
        (uri.scheme != 'http' && uri.scheme != 'https') ||
        uri.host.isEmpty) {
      return '站点地址必须是完整的 http(s) URL';
    }
    if (type == 'maccms') {
      if (detailPathPattern.trim().isEmpty) return '请填写详情路由正则';
      try {
        RegExp(detailPathPattern);
      } catch (_) {
        return '详情路由正则无效';
      }
    } else if (type == 'aes') {
      if (aesKey.trim().isEmpty) return '请填写 AES 密钥';
    } else {
      return '未知站源类型';
    }
    return null;
  }
}
