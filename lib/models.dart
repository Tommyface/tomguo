import 'dart:convert';

class SourceSite {
  const SourceSite(this.id, this.name, this.description);
  final String id;
  final String name;
  final String description;
  bool get onlineSearch =>
      id == 'hongguo' ||
      pagedSearch ||
      // 参考 zhenguojianTV v0.8.31 新增站源的在线搜索能力。
      id == '51duanju' ||
      id == 'ezdmw' ||
      id == 'hema' ||
      id == 'huaguo' ||
      id == 'faguo' ||
      id == 'wuguo' ||
      id == 'mxdm' ||
      id == 'jcydm';
  bool get pagedSearch =>
      id == 'huangju' ||
      id == 'chengguo' ||
      id == 'yeguo' ||
      id == 'dsd' ||
      id == 'sorani' ||
      id == 'guipian' ||
      id == 'hanxiaoquan' ||
      // 参考 zhenguojianTV v0.8.31：以下新源搜索支持翻页。
      id == '51duanju' ||
      id == 'ezdmw' ||
      id == 'hema' ||
      id == 'faguo' ||
      id == 'wuguo' ||
      id == 'jcydm';
  bool get searchSuggestions => id == 'hongguo';
  String get groupId => switch (id) {
    'huangguo-video' || 'huangguoai' || 'cloudfront' => 'huangguo',
    _ => id,
  };
  String get groupName => groupId == 'huangguo' ? '黄果' : name;
  String get entryName => switch (id) {
    'huangguo-video' => '视频',
    'huangguoai' => 'AI',
    'cloudfront' => '旧版',
    _ => name,
  };

  static const hongguo = SourceSite('hongguo', '红果', '短剧 · 漫剧 · AI 剧');
  static const dsd = SourceSite('dsd', '帝果', '分类视频 · 在线搜索');
  static const sorani = SourceSite('sorani', '青空', '番剧 · 剧场动画 · 特摄');
  static const guipian = SourceSite('guipian', '鬼片', '鬼片 · 电视剧 · 动漫');
  static const hanxiaoquan = SourceSite(
    'hanxiaoquan',
    '韩小圈',
    '韩剧 · 韩国电影 · 综艺动漫',
  );

  /// 参考 zhenguojianTV v0.8.31 新增站源。
  static const duanju51 = SourceSite('51duanju', '51', '短剧 · 首页 · 榜单');
  static const hema = SourceSite('hema', '河马', '短剧 · 全集合集');
  static const ezdmw = SourceSite('ezdmw', 'E站', '动漫 · 新番 · 完结');
  static const huaguo = SourceSite('huaguo', '花果', '短剧 · 花生短剧');
  static const wangguo = SourceSite('wangguo', '网果', '短剧 · 短剧网');
  static const faguo = SourceSite('faguo', '发果', '短剧 · 168 影院');
  static const wuguo = SourceSite('wuguo', '伍果', '短剧 · 五五短剧');
  static const mxdm = SourceSite('mxdm', 'MX动漫', '日漫 · 国漫 · 剧场版');
  static const jcydm = SourceSite('jcydm', '囧次元', '日漫 · 剧场 · 热搜榜');

  /// 主程序不预置任何站源：全部站源都由用户从「站源模块」导入 zip 后出现。
  /// 这里保留空清单，是为了让 [values]/[visibleValues] 的调用方无需判空。
  static const primaryValues = <SourceSite>[];

  /// 敏感站源同样不预置；导入的模块若带 adult 标记，由模块清单决定是否解锁。
  static const restrictedValues = <SourceSite>[];

  static const knownValues = [...primaryValues, ...restrictedValues];
  static const allValues = [...primaryValues, ...restrictedValues];
  static const values = knownValues;

  /// 运行时注册的自定义站源（由用户在「添加自定义源」入口配置）。
  /// 与内置源共用同一套 UI 与解析链路，仅来源不同。
  static final Map<String, SourceSite> _custom = {};

  /// 用配置注入一组自定义源；id 需以 custom_ 前缀开头。
  static void registerCustom(Iterable<SourceSite> sites) {
    _custom.clear();
    for (final site in sites) {
      if (site.id.startsWith('custom_')) _custom[site.id] = site;
    }
  }

  static Iterable<SourceSite> get customValues => _custom.values;

  /// 运行时安装的站源模块（JS 插件），由设置页在启动与安装后注入。
  static final Map<String, SourceSite> _modules = {};

  /// 用模块清单注入一组站源；id 直接用模块 id。
  static void registerModules(Iterable<SourceSite> sites) {
    _modules.clear();
    for (final site in sites) {
      if (site.id.isNotEmpty) _modules[site.id] = site;
    }
  }

  static Iterable<SourceSite> get moduleValues => _modules.values;

  static bool isAvailable(String id) =>
      values.any((site) => site.id == id) ||
      _custom.containsKey(id) ||
      _modules.containsKey(id);
  static bool isKnown(String id) =>
      allValues.any((site) => site.id == id) ||
      _custom.containsKey(id) ||
      _modules.containsKey(id);
  static bool isPrimary(String id) =>
      primaryValues.any((site) => site.id == id);
  static bool isRestricted(String id) =>
      restrictedValues.any((site) => site.id == id);

  /// 运行时可见站源：内置清单 + 已安装模块，按 [enabled] 过滤。
  /// [enabled] 为 null 表示未设置过，此时全部显示。
  static List<SourceSite> visibleValues({
    required bool unlocked,
    Set<String>? enabled,
  }) {
    final all = <String, SourceSite>{};
    for (final site in values) {
      all[site.id] = site;
    }
    for (final site in _modules.values) {
      all[site.id] = site;
    }
    for (final site in _custom.values) {
      all[site.id] = site;
    }
    return [
      for (final site in all.values)
        if (enabled == null || enabled.contains(site.id)) site,
    ];
  }
  static SourceSite byId(String id) {
    for (final site in allValues) {
      if (site.id == id) return site;
    }
    final custom = _custom[id];
    if (custom != null) return custom;
    final module = _modules[id];
    if (module != null) return module;
    if (_modules.isNotEmpty) return _modules.values.first;
    if (_custom.isNotEmpty) return _custom.values.first;
    return hongguo;
  }
}

class SourceGroup {
  const SourceGroup(this.id, this.name, this.sources);
  final String id;
  final String name;
  final List<SourceSite> sources;

  static List<SourceGroup> fromSources(Iterable<SourceSite> sources) {
    final groups = <String, List<SourceSite>>{};
    for (final source in sources) {
      (groups[source.groupId] ??= []).add(source);
    }
    return [
      for (final group in groups.entries)
        SourceGroup(group.key, group.value.first.groupName, group.value),
    ];
  }
}

class CatalogCategory {
  const CatalogCategory(this.id, this.name, {this.local = false});
  static const all = CatalogCategory('', '全部');
  final String id;
  final String name;
  final bool local;
  factory CatalogCategory.fromJson(Map<String, dynamic> json) =>
      CatalogCategory(
        json['id'] as String? ?? '',
        json['name'] as String? ?? '全部',
      );
}

int intValue(Object? value) =>
    value is num ? value.toInt() : int.tryParse('$value') ?? 0;

class Drama {
  const Drama({
    required this.id,
    required this.source,
    required this.title,
    this.sourceId = '',
    this.description = '',
    this.cover = '',
    this.episodes = 0,
    this.category = '',
    bool? vip,
    this.heat = '',
    this.views = '',
    this.onlineDate = '',
    this.tags = const [],
    this.releaseStatus = '',
  }) : vipStatus = vip;
  final String id;
  final String source;
  final String sourceId;
  final String title;
  final String description;
  final String cover;
  final int episodes;
  final String category;
  final bool? vipStatus;
  bool get vip => vipStatus == true;
  final String heat;
  final String views;
  final String onlineDate;
  final List<String> tags;
  final String releaseStatus;
  String get releaseLabel => switch (releaseStatus) {
    'finished' || 'completed' => '已完结',
    'ongoing' => '连载中',
    _ => '状态未知',
  };

  factory Drama.fromJson(Map<String, dynamic> json) => Drama(
    id: json['id'] as String? ?? '',
    source: json['source'] as String? ?? 'hongguo',
    sourceId: json['sourceId'] as String? ?? '',
    title: json['title'] as String? ?? '短剧',
    description: json['description'] as String? ?? '',
    cover: json['cover'] as String? ?? '',
    episodes: intValue(json['episodes']),
    category: json['category'] as String? ?? '',
    vip: json['vip'] == true
        ? true
        : json['vip'] == false &&
              (json['source'] != 'huangdou' ||
                  intValue(json['metadataSchema']) >= 1)
        ? false
        : null,
    heat: json['heat']?.toString() ?? '',
    views: json['views']?.toString() ?? '',
    onlineDate: json['onlineDate'] as String? ?? '',
    tags: (json['tags'] as List? ?? const []).whereType<String>().toList(),
    releaseStatus: json['releaseStatus'] as String? ?? '',
  );
  Map<String, dynamic> toJson() => {
    'metadataSchema': 1,
    'id': id,
    'source': source,
    'sourceId': sourceId,
    'title': title,
    'description': description,
    'cover': cover,
    'episodes': episodes,
    'category': category,
    'vip': vipStatus,
    'heat': heat,
    'views': views,
    'onlineDate': onlineDate,
    'tags': tags,
    'releaseStatus': releaseStatus,
  };

  Drama merge(Drama fresh) {
    if (id != fresh.id) return fresh;
    const genericCategories = {
      '短剧',
      '真人剧',
      '漫剧',
      'AI剧',
      'AI 剧',
      'AI短剧',
      'AI 短剧',
      'AI漫剧',
      'AI 漫剧',
    };
    return Drama(
      id: id,
      source: fresh.source.isEmpty ? source : fresh.source,
      sourceId: fresh.sourceId.isEmpty ? sourceId : fresh.sourceId,
      title: fresh.title.isEmpty || fresh.title == '短剧' ? title : fresh.title,
      description: fresh.description.isEmpty ? description : fresh.description,
      cover: fresh.cover.isEmpty ? cover : fresh.cover,
      episodes: fresh.episodes > 0 ? fresh.episodes : episodes,
      category:
          fresh.category.isEmpty ||
              genericCategories.contains(fresh.category) &&
                  category.isNotEmpty &&
                  !genericCategories.contains(category)
          ? category
          : fresh.category,
      vip: fresh.vipStatus ?? vipStatus,
      heat: fresh.heat.isEmpty ? heat : fresh.heat,
      views: fresh.views.isEmpty ? views : fresh.views,
      onlineDate: fresh.onlineDate.isEmpty ? onlineDate : fresh.onlineDate,
      tags: fresh.tags.isEmpty ? tags : fresh.tags,
      releaseStatus:
          fresh.releaseStatus.isEmpty || fresh.releaseStatus == 'unknown'
          ? releaseStatus
          : fresh.releaseStatus,
    );
  }
}

class Episode {
  Episode(this.raw, int fallback)
    : id = raw['id'] as String? ?? '',
      title = raw['title'] as String? ?? '第$fallback集',
      number = intValue(raw['currentEpisode']) > 0
          ? intValue(raw['currentEpisode'])
          : fallback,
      vip = raw['vip'] == true;
  final Map<String, dynamic> raw;
  final String id;
  final String title;
  final int number;
  final bool vip;
}

class DramaDetail {
  DramaDetail(this.drama, this.episodes, {this.warning = ''});
  final Drama drama;
  final List<Episode> episodes;
  final String warning;
  factory DramaDetail.fromJson(Map<String, dynamic> json) {
    final rows = json['chapters'] as List? ?? const [];
    return DramaDetail(
      Drama.fromJson(Map<String, dynamic>.from(json['drama'] as Map)),
      [
        for (var i = 0; i < rows.length; i++)
          Episode(Map<String, dynamic>.from(rows[i] as Map), i + 1),
      ],
      warning: json['warning'] as String? ?? '',
    );
  }
}

class CatalogPage {
  CatalogPage(
    this.items, {
    this.hasMore = false,
    this.warning = '',
    this.page = 1,
    this.fresh = false,
  });
  final List<Drama> items;
  final bool hasMore;
  final String warning;
  final int page;
  final bool fresh;
  factory CatalogPage.fromJson(Map<String, dynamic> json) => CatalogPage(
    [
      for (final row in json['items'] as List? ?? const [])
        Drama.fromJson(Map<String, dynamic>.from(row as Map)),
    ],
    hasMore: json['hasMore'] == true,
    warning: json['warning'] as String? ?? '',
    page: intValue(json['page']) > 0 ? intValue(json['page']) : 1,
    fresh: json['fresh'] == true,
  );
}

class PlaybackPlan {
  const PlaybackPlan({
    required this.url,
    this.headers = const {},
    this.decryptionKey = '',
    this.quality = 0,
    this.qualities = const [],
    this.session = '',
    this.danmakuId = '',
    this.prefetchedBytes = 0,
    this.expiresAt = 0,
    this.routeIndex = 0,
    this.routeCount = 1,
    this.local = false,
  });
  final String url;
  final Map<String, String> headers;
  final String decryptionKey;
  final int quality;
  final List<int> qualities;
  final String session;
  final String danmakuId;
  final int prefetchedBytes;
  final int expiresAt;
  final int routeIndex;
  final int routeCount;
  final bool local;
  bool get hasAlternative => session.isNotEmpty && routeIndex + 1 < routeCount;
  factory PlaybackPlan.fromJson(Map<String, dynamic> json) => PlaybackPlan(
    url: json['url'] as String? ?? '',
    local: json['local'] == true,
    headers: (json['headers'] as Map? ?? {}).map(
      (key, value) => MapEntry(key.toString(), value.toString()),
    ),
    decryptionKey: json['decryptionKey'] as String? ?? '',
    quality: intValue(json['quality']),
    qualities: (json['qualities'] as List? ?? []).map(intValue).toSet().toList()
      ..sort((a, b) => b.compareTo(a)),
    session: json['session'] as String? ?? '',
    danmakuId: json['danmakuId'] as String? ?? '',
    prefetchedBytes: intValue(json['prefetchedBytes']),
    expiresAt: intValue(json['expiresAt']),
    routeIndex: intValue(json['routeIndex']),
    routeCount: intValue(json['routeCount']) > 0
        ? intValue(json['routeCount'])
        : 1,
  );
}

class WatchEntry {
  WatchEntry({
    required this.drama,
    required this.episode,
    required this.position,
    required this.duration,
    required this.updatedAt,
  });
  final Drama drama;
  final int episode;
  final double position;
  final double duration;
  final DateTime updatedAt;
  bool get finished => duration > 1 && position >= duration - 1;
  Map<String, dynamic> toJson() => {
    'drama': drama.toJson(),
    'episode': episode,
    'position': position,
    'duration': duration,
    'updatedAt': updatedAt.toIso8601String(),
  };
  factory WatchEntry.fromJson(Map<String, dynamic> json) => WatchEntry(
    drama: Drama.fromJson(Map<String, dynamic>.from(json['drama'] as Map)),
    episode: intValue(json['episode']),
    position: (json['position'] as num?)?.toDouble() ?? 0,
    duration: (json['duration'] as num?)?.toDouble() ?? 0,
    updatedAt:
        DateTime.tryParse(json['updatedAt'].toString()) ?? DateTime(2000),
  );
}

class DownloadJob {
  const DownloadJob({
    required this.id,
    required this.drama,
    required this.episode,
    required this.state,
    this.bytes = 0,
    this.total = 0,
    this.progress = 0,
    this.quality = 0,
    this.actualQuality = 0,
    this.error = '',
    this.created = 0,
    this.revision = 0,
    this.archived = false,
  });
  final int created;
  final int revision;
  final bool archived;
  final String id;
  final Drama drama;
  final Episode episode;
  final String state;
  final int bytes;
  final int total;
  final double progress;
  final int quality;
  final int actualQuality;
  final String error;
  bool get completed => state == 'completed';
  bool get active => state == 'queued' || state == 'downloading';
  bool get resumable => state == 'paused' || state == 'failed';
  String get stateLabel => switch (state) {
    'queued' => '等待下载',
    'downloading' => '正在下载',
    'paused' => '已暂停',
    'failed' => '下载失败',
    'completed' => '已下载',
    'removing' => '正在取消',
    _ => '等待更新',
  };
  factory DownloadJob.fromJson(Map<String, dynamic> value) => DownloadJob(
    id: value['id'] as String? ?? '',
    drama: Drama.fromJson(Map<String, dynamic>.from(value['drama'] as Map)),
    episode: Episode(
      Map<String, dynamic>.from(value['chapter'] as Map),
      intValue(value['index']),
    ),
    state: value['state'] as String? ?? 'failed',
    bytes: intValue(value['bytes']),
    total: intValue(value['total']),
    progress: ((value['progress'] as num?)?.toDouble() ?? 0).clamp(0, 1),
    quality: intValue(value['quality']),
    actualQuality: intValue(value['actualQuality']),
    error: value['error'] as String? ?? '',
    created: intValue(value['created']),
    revision: intValue(value['revision']),
    archived: value['archived'] == true,
  );
}

List<Map<String, dynamic>> readJsonList(String? value) {
  try {
    return (jsonDecode(value ?? '[]') as List)
        .whereType<Map>()
        .map((row) => Map<String, dynamic>.from(row))
        .toList();
  } catch (_) {
    return [];
  }
}

/// 站源整合：同名作品在不同站源的一条来源。
class AggregatedSource {
  const AggregatedSource({
    required this.source,
    required this.dramaId,
    required this.title,
    this.sourceId = '',
    this.cover = '',
    this.episodes = 0,
    this.vip = false,
    this.playable = false,
    this.adult = false,
    this.category = '',
  });

  final String source;
  final String sourceId;
  final String dramaId;
  final String title;
  final String cover;
  final int episodes;
  final bool vip;
  final bool playable;
  final bool adult;
  final String category;

  factory AggregatedSource.fromJson(Map<String, dynamic> json) =>
      AggregatedSource(
        source: json['source'] as String? ?? '',
        sourceId: json['sourceId'] as String? ?? '',
        dramaId: json['dramaId'] as String? ?? '',
        title: json['title'] as String? ?? '',
        cover: json['cover'] as String? ?? '',
        episodes: intValue(json['episodes']),
        vip: json['vip'] == true,
        playable: json['playable'] == true,
        adult: json['adult'] == true,
        category: json['category'] as String? ?? '',
      );

  /// 转回普通 Drama，供详情页与播放链路复用。
  Drama toDrama(String fallbackTitle) => Drama(
    id: dramaId,
    source: source,
    sourceId: sourceId,
    title: title.isEmpty ? fallbackTitle : title,
    cover: cover,
    episodes: episodes,
    category: category,
    vip: vip,
  );
}

/// 站源整合：聚合后的一部作品。
class AggregatedDrama {
  const AggregatedDrama({
    required this.key,
    required this.title,
    required this.sources,
    this.cover = '',
    this.contentKind = 'short_drama',
    this.episodes = 0,
    this.sourceCount = 0,
  });

  final String key;
  final String title;
  final String cover;
  final String contentKind;
  final int episodes;
  final int sourceCount;
  final List<AggregatedSource> sources;

  bool get adult => sources.isNotEmpty && sources.every((item) => item.adult);

  factory AggregatedDrama.fromJson(Map<String, dynamic> json) =>
      AggregatedDrama(
        key: json['key'] as String? ?? '',
        title: json['title'] as String? ?? '短剧',
        cover: json['cover'] as String? ?? '',
        contentKind: json['contentKind'] as String? ?? 'short_drama',
        episodes: intValue(json['episodes']),
        sourceCount: intValue(json['sourceCount']),
        sources: [
          for (final row in json['sources'] as List? ?? const [])
            AggregatedSource.fromJson(Map<String, dynamic>.from(row as Map)),
        ],
      );
}

/// 站源整合：聚合首页一页数据。
class AggregatePage {
  AggregatePage(
    this.items, {
    this.hasMore = false,
    this.page = 1,
    this.totalSources = 0,
    this.failedSources = const {},
    this.partial = false,
    this.groupId = '',
  });

  final List<AggregatedDrama> items;
  final bool hasMore;
  final int page;
  final int totalSources;
  final Map<String, String> failedSources;
  final bool partial;
  final String groupId;

  factory AggregatePage.fromJson(Map<String, dynamic> json) => AggregatePage(
    [
      for (final row in json['items'] as List? ?? const [])
        AggregatedDrama.fromJson(Map<String, dynamic>.from(row as Map)),
    ],
    hasMore: json['hasMore'] == true,
    page: intValue(json['page']) > 0 ? intValue(json['page']) : 1,
    totalSources: intValue(json['totalSources']),
    failedSources:
        (json['failedSources'] as Map?)?.map(
          (key, value) => MapEntry('$key', '$value'),
        ) ??
        const {},
    partial: json['partial'] == true,
    groupId: json['groupId'] as String? ?? '',
  );
}

/// 内容类型：与 Go 侧 contentKind 一一对应。
abstract final class ContentKind {
  static const all = '';
  static const shortDrama = 'short_drama';
  static const liveAction = 'live_action';
  static const video = 'video';
  static const aiDuanju = 'ai_duanju';
  static const aiManhua = 'ai_manhua';
  static const aiMogai = 'ai_mogai';
  static const aiHuanlian = 'ai_huanlian';

  static const labels = <String, String>{
    all: '全部',
    shortDrama: '短剧',
    liveAction: '真人剧',
    video: '视频',
    aiDuanju: 'AI 短剧',
    aiManhua: 'AI 漫剧',
    aiMogai: 'AI 魔改',
    aiHuanlian: 'AI 换脸',
  };
}
