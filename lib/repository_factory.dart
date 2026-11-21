/// 仓库工厂 + 双轨仓库（双轨骨架 · 第 1 步）
///
/// 目的：把「用哪个站源实现」的决策集中到一个工厂里，让 main.dart 只依赖
/// 工厂函数，而不再直接 new NativeRepository()。这样后续切换到 SDK 路径
/// 时，业务代码零改动。
///
/// 定位（适配品牌重构后的「聚合短剧」全站源版本）：
///   本项目现为单版本、含 11 个站源（红果 / 韩小圈 / 鬼片 / 青空 +
///   黄豆 / 剧果 / 野果 / 帝果 / 黄果视频 / 黄果 AI / 黄果旧版）。
///   openhongguo SDK 只覆盖「红果」这一个站源，其余 10 个仍走原生 Go core。
///   因此本工厂是「红果单源可切换 + 其余源恒定原生」的双轨，而非整体替换。
///
/// 设计要点：
///   1. 采用「继承 NativeRepository + 选择性覆写」而非组合，天然对齐全部
///      AppRepository 接口，未覆写的方法行为完全不变。
///   2. 覆写的红果方法内部先判断 SDK 是否可用；骨架阶段 SDK 默认禁用，
///      因此全部走 super（原生 core）——与改造前行为逐字一致。
///   3. 桌面端（Windows）无论 flag 如何都强制走原生 core（SDK 不支持桌面）。
library repository_factory;

import 'dart:io';

import 'core_bridge.dart';
import 'hongguo_sdk.dart';
import 'models.dart';
import 'danmaku_models.dart';

/// 双轨仓库：红果站源可走 SDK，其余能力全部沿用原生 core。
///
/// 骨架阶段所有方法都委托 super，行为与原生 core 完全一致。
/// 第 2 步接入 SDK 后，只需在 `_hongguo` 可用时把红果调用改为走适配层。
class DualTrackRepository extends NativeRepository {
  DualTrackRepository({super.background, HongguoSource? hongguo})
    : _hongguo = hongguo ?? const DisabledHongguoSource();

  /// 红果 SDK 适配层。默认禁用实现，保证骨架阶段零行为变化。
  final HongguoSource _hongguo;

  /// 红果 SDK 路径是否真正启用（需要适配层报告 available）。
  bool get _useSdk => _hongguo.available;

  // ---- 以下为红果相关方法的分流点。骨架阶段统一走 super。----

  @override
  Future<CatalogPage> catalog(
    String source, {
    int page = 1,
    String query = '',
    String category = '',
    bool force = false,
  }) async {
    if (_useSdk && _isHongguo(source)) {
      // TODO(step2): return _hongguo.catalog(page: page, query: query, category: category);
    }
    return super.catalog(
      source,
      page: page,
      query: query,
      category: category,
      force: force,
    );
  }

  @override
  Future<List<String>> suggestions(String query) async {
    if (_useSdk) {
      // TODO(step2): return _hongguo.suggestions(query);
    }
    return super.suggestions(query);
  }

  @override
  Future<DramaDetail> detail(Drama drama) async {
    if (_useSdk && _isHongguo(drama.source)) {
      // TODO(step2): return _hongguo.detail(drama);
    }
    return super.detail(drama);
  }

  @override
  Future<PlaybackPlan> resolve(
    Drama drama,
    Episode episode, {
    int quality = 0,
  }) async {
    if (_useSdk && _isHongguo(drama.source)) {
      // TODO(step2): return _hongguo.resolve(drama, episode, quality: quality);
    }
    return super.resolve(drama, episode, quality: quality);
  }

  @override
  Future<CatalogPage> recommendations(
    String genre, {
    bool more = false,
    bool force = false,
  }) async {
    if (_useSdk) {
      // TODO(step2): return _hongguo.recommendations(genre as Drama) ——需确认 genre 语义
    }
    return super.recommendations(genre, more: more, force: force);
  }

  @override
  Future<DanmakuPage> danmaku(
    PlaybackPlan plan, {
    required int startMs,
    required int durationMs,
  }) async {
    if (_useSdk && plan.danmakuId.isNotEmpty) {
      // TODO(step2): return _hongguo.danmaku(plan.danmakuId);
    }
    return super.danmaku(plan, startMs: startMs, durationMs: durationMs);
  }

  @override
  Future<void> release(String session) async {
    // 先释放 SDK 客户端（若启用），再释放原生 session。
    if (_useSdk) {
      await _hongguo.release();
    }
    return super.release(session);
  }

  /// 是否红果站源。站源标识以 models.dart / core 中的约定为准。
  bool _isHongguo(String source) => source == 'hongguo' || source.isEmpty;

  /// 平台是否支持 SDK（仅移动端）。
  static bool get platformSupportsSdk => Platform.isAndroid || Platform.isIOS;
}

/// 运行时开关：是否启用红果 SDK 双轨。
///
/// 骨架阶段恒为 false。第 2 步完成 SDK 接入与真机验证后改为 true，
/// 或改为从 ResourceSettings / dart-define 读取。
const bool kEnableHongguoSdk = false;

/// 创建应用仓库。
///
/// [background] 用于后台下载/隔离区实例（与 main.dart 现有用法一致）。
/// [forceNative] 供测试或降级场景强制使用原生 core。
NativeRepository createRepository({
  bool background = false,
  bool forceNative = false,
}) {
  final canUseSdk =
      kEnableHongguoSdk &&
      !forceNative &&
      DualTrackRepository.platformSupportsSdk;

  if (!canUseSdk) {
    // 骨架阶段与降级路径：直接返回原生 core 实例（零行为变化）。
    return NativeRepository(background: background);
  }

  // 第 2 步启用后：构造 SDK 适配器并注入双轨仓库。
  // final adapter = OpenhongguoAdapter();
  // await adapter.initialize();  // 工厂需改为 async 或改为惰性初始化
  // return DualTrackRepository(background: background, hongguo: adapter);
  return DualTrackRepository(background: background);
}
