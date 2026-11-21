/// 红果站源 SDK 适配层（双轨骨架 · 第 1 步）
///
/// 设计目标：把「红果站源能力」抽象成独立接口 [HongguoSource]，使上层
/// [AppRepository] 可以在「自研 Go core」与「openhongguo Flutter SDK」之间
/// 切换，而不触碰任何现有业务逻辑。
///
/// 当前状态（骨架）：
///   - 默认使用 [DisabledHongguoSource]，所有方法抛出 [UnsupportedError]，
///     工厂会据此始终回退到原生 core —— 行为与改造前完全一致。
///   - [OpenhongguoAdapter] 是待填充的真实实现骨架，标有 TODO 的地方在
///     第 2 步执行 `flutter pub add openhongguo` 后替换为 SDK 调用。
///
/// 为什么骨架阶段不 `import 'package:openhongguo/...'`：
///   SDK 尚未加入 pubspec.yaml。提前 import 会导致整个项目无法编译。
///   依赖倒置让骨架保持可编译，同时锁定接口边界。
library hongguo_sdk;

import 'dart:io';

import 'models.dart';
import 'danmaku_models.dart';

/// 红果站源能力接口。
///
/// 方法签名与 openhongguo SDK 0.3.0 的 API 一一对应：
///   discoverAll → catalog, searchAll → search, getSeries → detail,
///   listEpisodes → (并入 detail), resolvePlayback → resolve,
///   listDanmaku → danmaku, suggest → suggestions, seriesRecommend → recommendations.
abstract class HongguoSource {
  /// 是否可用。工厂据此决定是否启用 SDK 路径。
  bool get available;

  /// 加载红果目录一页。
  Future<CatalogPage> catalog({
    required int page,
    String query = '',
    String category = '',
  });

  /// 搜索红果短剧。
  Future<CatalogPage> search(String query, {int page = 1});

  /// 搜索联想词。
  Future<List<String>> suggestions(String prefix);

  /// 短剧详情 + 分集。
  Future<DramaDetail> detail(Drama drama);

  /// 解析某一集的播放地址与内容密钥。
  Future<PlaybackPlan> resolve(Drama drama, Episode episode, {int quality = 0});

  /// 加载一集弹幕。
  Future<DanmakuPage> danmaku(String danmakuId, {int page = 1});

  /// 相关推荐。
  Future<CatalogPage> recommendations(Drama drama);

  /// 释放底层客户端。
  Future<void> release();
}

/// 默认（禁用）实现：所有方法抛出不支持错误。
///
/// 工厂在 [HongguoSource.available == false] 时不会调用这些方法，
/// 因此它仅作为类型占位，保证骨架可编译。
class DisabledHongguoSource implements HongguoSource {
  const DisabledHongguoSource();

  @override
  bool get available => false;

  UnsupportedError _disabled(String method) => UnsupportedError(
    '红果 SDK 尚未启用（$method）。请在 repository_factory.dart 打开 feature flag '
    '并完成第 2 步 SDK 接入。',
  );

  @override
  Future<CatalogPage> catalog({
    required int page,
    String query = '',
    String category = '',
  }) => Future.error(_disabled('catalog'));

  @override
  Future<CatalogPage> search(String query, {int page = 1}) =>
      Future.error(_disabled('search'));

  @override
  Future<List<String>> suggestions(String prefix) =>
      Future.error(_disabled('suggestions'));

  @override
  Future<DramaDetail> detail(Drama drama) => Future.error(_disabled('detail'));

  @override
  Future<PlaybackPlan> resolve(
    Drama drama,
    Episode episode, {
    int quality = 0,
  }) => Future.error(_disabled('resolve'));

  @override
  Future<DanmakuPage> danmaku(String danmakuId, {int page = 1}) =>
      Future.error(_disabled('danmaku'));

  @override
  Future<CatalogPage> recommendations(Drama drama) =>
      Future.error(_disabled('recommendations'));

  @override
  Future<void> release() async {}
}

/// openhongguo SDK 真实实现的骨架。
///
/// 平台约束：SDK 是 gomobile 原生库，仅支持 Android / iOS，不支持桌面端。
/// 工厂层会保证本实现只在移动端被实例化。
///
/// 第 2 步待办（pub add openhongguo 后）：
///   1. import 'package:openhongguo/openhongguo.dart';
///   2. 持有 final OpenhongguoClient _client;
///   3. 用 OpenhongguoClient.create() 初始化，配置 profilePath / retryAttempts / log
///   4. 把每个方法体替换为 SDK 调用，并把返回的 SDK 类型转换为本项目的
///      Drama / DramaDetail / PlaybackPlan / DanmakuPage（见 models.dart 的 fromJson）
///   5. 处理 OpenhongguoException：RATE_LIMITED 用 retryAfterMs 退避，
///      DEVICE_REJECTED 调 resetDevice() 后重试
class OpenhongguoAdapter implements HongguoSource {
  OpenhongguoAdapter({this.enableLogging = false});

  /// 是否输出 SDK 诊断日志（URL 已由 SDK 脱敏）。
  final bool enableLogging;

  // TODO(step2): 改为 `late final OpenhongguoClient _client;`
  // 并在 initialize() 中 `OpenhongguoClient.create(...)`。
  Object? _client;

  @override
  bool get available =>
      (Platform.isAndroid || Platform.isIOS) && _client != null;

  /// 初始化 SDK 客户端。骨架阶段仅校验平台，不真正创建客户端。
  Future<void> initialize() async {
    if (!Platform.isAndroid && !Platform.isIOS) {
      throw UnsupportedError('openhongguo SDK 仅支持 Android / iOS，桌面端请用原生 core');
    }
    // TODO(step2): _client = await OpenhongguoClient.create(
    //   ClientOptions(retryAttempts: 3, deviceResets: 2, log: enableLogging ? print : null),
    // );
  }

  @override
  Future<CatalogPage> catalog({
    required int page,
    String query = '',
    String category = '',
  }) {
    _assertReady();
    // TODO(step2): final r = await _client.discover(...); => _toCatalogPage(r)
    throw UnimplementedError('catalog 待第 2 步接入 SDK');
  }

  @override
  Future<CatalogPage> search(String query, {int page = 1}) {
    _assertReady();
    // TODO(step2): final r = await _client.searchAll(query, ...); => _toCatalogPage(r)
    throw UnimplementedError('search 待第 2 步接入 SDK');
  }

  @override
  Future<List<String>> suggestions(String prefix) {
    _assertReady();
    // TODO(step2): return _client.suggest(prefix);
    throw UnimplementedError('suggestions 待第 2 步接入 SDK');
  }

  @override
  Future<DramaDetail> detail(Drama drama) {
    _assertReady();
    // TODO(step2): getSeries + listEpisodes => DramaDetail
    throw UnimplementedError('detail 待第 2 步接入 SDK');
  }

  @override
  Future<PlaybackPlan> resolve(
    Drama drama,
    Episode episode, {
    int quality = 0,
  }) {
    _assertReady();
    // TODO(step2): resolvePlayback => PlaybackPlan（含 contentKey => decryptionKey）
    throw UnimplementedError('resolve 待第 2 步接入 SDK');
  }

  @override
  Future<DanmakuPage> danmaku(String danmakuId, {int page = 1}) {
    _assertReady();
    // TODO(step2): listDanmaku => DanmakuPage
    throw UnimplementedError('danmaku 待第 2 步接入 SDK');
  }

  @override
  Future<CatalogPage> recommendations(Drama drama) {
    _assertReady();
    // TODO(step2): seriesRecommend => _toCatalogPage
    throw UnimplementedError('recommendations 待第 2 步接入 SDK');
  }

  @override
  Future<void> release() async {
    // TODO(step2): await _client?.release();
    _client = null;
  }

  void _assertReady() {
    if (_client == null) {
      throw StateError('OpenhongguoAdapter 未初始化，请先调用 initialize()');
    }
  }
}

/// SDK 类型 → 项目类型的转换助手占位。
///
/// 第 2 步在此集中实现，避免转换逻辑散落在各方法里。
class HongguoAdapterConverters {
  // TODO(step2): static Drama toDrama(dynamic sdkSeries) { ... }
  // TODO(step2): static PlaybackPlan toPlaybackPlan(dynamic playback, dynamic contentKey) { ... }
  // TODO(step2): static DanmakuPage toDanmakuPage(dynamic danmakuPage) { ... }
}
