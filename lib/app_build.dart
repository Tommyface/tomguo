const appName = '聚合短剧';
const appSlug = 'juheduanju';

/// 构建时是否默认展示全部站源（含默认隐藏的敏感站源）。
///
/// 为 true 时：无需输入站源解锁密码，所有站源默认可见；用户仍可在设置里
/// 自行启用密码锁（届时以用户配置为准）。
/// 为 false 时：恢复原有行为——红果 / 韩小圈 / 鬼片 / 青空默认可见，
/// 其余站源需密码解锁。
///
/// 通过 dart-define 覆盖：
///   flutter build apk --dart-define=SHOW_ALL_SOURCES_BY_DEFAULT=false
/// 遵循项目既有的 `bool.fromEnvironment` 约定（见 media_exports.dart）。
const bool kShowAllSourcesByDefault = bool.fromEnvironment(
  'SHOW_ALL_SOURCES_BY_DEFAULT',
  defaultValue: true,
);
