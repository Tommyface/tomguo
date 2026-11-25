import 'package:flutter/material.dart';

import 'core_bridge.dart';
import 'design_tokens.dart';
import 'models.dart';

/// 智能选源面板：设计稿的底部弹层。
///
/// 数据来自现有 `crossSource` 接口（同名换源），但呈现方式改成"源列表 + 评分环"：
/// - 每行一个候选源，左侧排名、中间延迟/稳定性进度条、右侧评分环
/// - 顶部「智能选源」开关（默认开）
/// - 底部「添加播放源 / 管理播放源」
///
/// 与旧版 `showCrossSourceSheet` 的区别：旧版是 ListTile 列表，看不出源质量差异。
class SourceCandidate {
  const SourceCandidate({
    required this.id,
    required this.source,
    required this.sourceId,
    required this.title,
    required this.episodes,
    required this.vipEpisodes,
    required this.playable,
    required this.lastPlayable,
    required this.score,
    required this.error,
    required this.elapsedMs,
  });

  factory SourceCandidate.fromJson(Map<String, dynamic> json, int elapsed) =>
      SourceCandidate(
        id: json['id'] as String? ?? '',
        source: json['source'] as String? ?? '',
        sourceId: json['sourceId'] as String? ?? '',
        title: json['title'] as String? ?? '',
        episodes: _int(json['episodes']),
        vipEpisodes: _int(json['vipEpisodes']),
        playable: json['playable'] == true,
        lastPlayable: json['lastPlayable'] == true,
        score: _int(json['score']),
        error: json['error'] as String? ?? '',
        elapsedMs: elapsed,
      );

  final String id;
  final String source;
  final String sourceId;
  final String title;
  final int episodes;
  final int vipEpisodes;
  final bool playable;
  final bool lastPlayable;
  final int score;
  final String error;
  final int elapsedMs;

  bool get complete => playable && lastPlayable && vipEpisodes == 0;

  /// 归一化到 0–100，用于进度条与评分环。
  int get health {
    if (!playable) return 20;
    var value = 60;
    if (lastPlayable) value += 20;
    if (vipEpisodes == 0) value += 20;
    return value.clamp(0, 100);
  }

  String get tag {
    if (complete) return '全集';
    if (playable && lastPlayable) return '可播';
    if (playable) return '部分';
    return '不可用';
  }

  Color get tagColor => switch (tag) {
    '全集' => DramaX.ok,
    '可播' => DramaX.info,
    '部分' => DramaX.warn,
    _ => DramaX.t3,
  };

  String get sourceLabel => switch (source) {
    'huangguo-official' => '黄果官网',
    'huangguoai' => '黄果 AI',
    'huangdou' => '黄豆',
    'huanggua' => '黄瓜',
    'heitao' => '黑桃',
    'chengguo' => '橙果',
    'huangju' => '剧果',
    'guipian' => '鬼片',
    'hanxiaoquan' => '韩小圈',
    '51duanju' => '51短剧',
    _ => source,
  };

  Drama toDrama() => Drama(
    id: id,
    source: source,
    sourceId: sourceId,
    title: title,
    episodes: episodes,
    vip: vipEpisodes == 0 ? false : null,
  );

  static int _int(Object? value) =>
      value is int ? value : int.tryParse('${value ?? ''}') ?? 0;
}

/// 打开智能选源面板。选中返回对应 Drama，取消返回 null。
Future<Drama?> showSourceSheet(
  BuildContext context, {
  required AppRepository repository,
  required Drama drama,
  VoidCallback? onManage,
  /// 聚合视图已知的同名候选（来自首页聚合结果）。
  /// 传进来就先铺出来，用户不用等一轮跨源搜索。
  List<Drama> preset = const [],
}) => showModalBottomSheet<Drama>(
  context: context,
  isScrollControlled: true,
  backgroundColor: Colors.transparent,
  barrierColor: Colors.black.withValues(alpha: .66),
  builder: (_) => _SourceSheet(
    repository: repository,
    drama: drama,
    onManage: onManage,
    preset: preset,
  ),
);

class _SourceSheet extends StatefulWidget {
  const _SourceSheet({
    required this.repository,
    required this.drama,
    this.onManage,
    this.preset = const [],
  });

  final AppRepository repository;
  final Drama drama;
  final List<Drama> preset;
  final VoidCallback? onManage;

  @override
  State<_SourceSheet> createState() => _SourceSheetState();
}

class _SourceSheetState extends State<_SourceSheet> {
  List<SourceCandidate> _candidates = const [];
  Map<String, String> _errors = const {};
  String? _error;
  bool _loading = true;
  bool _auto = true;
  int _selected = 0;

  @override
  void initState() {
    super.initState();
    // 聚合视图已经把同名候选算好了，直接铺出来，后台再补一次完整搜索。
    if (widget.preset.isNotEmpty) {
      _candidates = [
        for (final drama in widget.preset)
          SourceCandidate(
            id: drama.id,
            source: drama.source,
            sourceId: drama.sourceId,
            title: drama.title,
            episodes: drama.episodes,
            vipEpisodes: 0,
            playable: drama.episodes > 0,
            lastPlayable: drama.episodes > 0,
            score: drama.episodes,
            error: '',
            elapsedMs: 0,
          ),
      ]..sort((a, b) => b.health.compareTo(a.health));
      _loading = false;
      _auto = true;
      return;   // 不自动搜索，等用户点「重新搜索」或首次下滑
    }
    _search();
  }

  Future<void> _search() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    final started = DateTime.now();
    try {
      final data = await widget.repository.crossSource(widget.drama);
      if (!mounted) return;
      final elapsed = DateTime.now().difference(started).inMilliseconds;
      final list = (data['candidates'] as List? ?? const [])
          .whereType<Map>()
          .map((item) => SourceCandidate.fromJson(
                Map<String, dynamic>.from(item),
                elapsed,
              ))
          .toList()
        ..sort((a, b) => b.health.compareTo(a.health));
      setState(() {
        _candidates = list;
        _errors = (data['errors'] as Map?)?.map(
              (key, value) => MapEntry('$key', '$value'),
            ) ??
            const {};
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

  @override
  Widget build(BuildContext context) {
    final height = MediaQuery.sizeOf(context).height;
    return Container(
      constraints: BoxConstraints(maxHeight: height * 0.82),
      decoration: const BoxDecoration(
        color: Color(0xFF17171B),
        borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
      ),
      child: SafeArea(
        top: false,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const SizedBox(height: 10),
            Container(
              width: 36,
              height: 4,
              decoration: BoxDecoration(
                color: const Color(0xFF3A3A40),
                borderRadius: BorderRadius.circular(2),
              ),
            ),
            const SizedBox(height: 14),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 18),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.baseline,
                textBaseline: TextBaseline.alphabetic,
                children: [
                  const Text(
                    '选择播放源',
                    style: TextStyle(
                      fontSize: 18,
                      fontWeight: FontWeight.w700,
                      color: DramaX.t1,
                    ),
                  ),
                  const Spacer(),
                  Text(
                    _loading ? '正在检测…' : '共 ${_candidates.length} 个可用源',
                    style: const TextStyle(fontSize: 12, color: DramaX.t2),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 12),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 18),
              child: _smartRow(),
            ),
            const SizedBox(height: 12),
            Flexible(child: _list()),
            const SizedBox(height: 14),
            Padding(
              padding: const EdgeInsets.fromLTRB(18, 0, 18, 20),
              child: _footer(),
            ),
          ],
        ),
      ),
    );
  }

  Widget _smartRow() => Container(
    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 11),
    decoration: BoxDecoration(
      gradient: const LinearGradient(
        colors: [Color(0xFF1F2A24), Color(0xFF1A1F1E)],
      ),
      borderRadius: BorderRadius.circular(13),
      border: Border.all(color: DramaX.ok.withValues(alpha: .28)),
    ),
    child: Row(
      children: [
        Container(
          width: 34,
          height: 34,
          decoration: BoxDecoration(
            color: DramaX.ok.withValues(alpha: .16),
            borderRadius: BorderRadius.circular(10),
          ),
          child: const Icon(Icons.auto_awesome, size: 18, color: DramaX.ok),
        ),
        const SizedBox(width: 11),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  const Text(
                    '智能选源',
                    style: TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w700,
                      color: DramaX.t1,
                    ),
                  ),
                  const SizedBox(width: 6),
                  Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 6,
                      vertical: 2,
                    ),
                    decoration: BoxDecoration(
                      color: DramaX.ok.withValues(alpha: .18),
                      borderRadius: BorderRadius.circular(5),
                    ),
                    child: Text(
                      _auto ? '已开启' : '已关闭',
                      style: const TextStyle(
                        fontSize: 9,
                        fontWeight: FontWeight.w700,
                        color: DramaX.ok,
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 3),
              const Text(
                '按完整性 / 可播性实时优选，异常自动切换',
                style: TextStyle(fontSize: 10, color: DramaX.t2),
              ),
            ],
          ),
        ),
        Switch(
          value: _auto,
          onChanged: (value) => setState(() => _auto = value),
          activeThumbColor: Colors.white,
          activeTrackColor: DramaX.ok,
        ),
      ],
    ),
  );

  Widget _list() {
    if (_loading) {
      return const Padding(
        padding: EdgeInsets.symmetric(vertical: 40),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            CircularProgressIndicator(color: DramaX.brand),
            SizedBox(height: 14),
            Text(
              '正在逐个站源搜索，约 20–40 秒',
              style: TextStyle(fontSize: 12, color: DramaX.t2),
            ),
          ],
        ),
      );
    }
    if (_error != null) {
      return Padding(
        padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              '换源搜索失败：$_error',
              textAlign: TextAlign.center,
              style: const TextStyle(fontSize: 12, color: DramaX.t2),
            ),
            const SizedBox(height: 12),
            FilledButton(onPressed: _search, child: const Text('重试')),
          ],
        ),
      );
    }
    if (_candidates.isEmpty) {
      return Padding(
        padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Text(
              '没有找到同名条目',
              style: TextStyle(fontSize: 13, color: DramaX.t1),
            ),
            const SizedBox(height: 8),
            Text(
              _errors.isEmpty
                  ? '可以换个更短的剧名再试。'
                  : '失败站源：${_errors.keys.join('、')}',
              textAlign: TextAlign.center,
              style: const TextStyle(fontSize: 11, color: DramaX.t3),
            ),
          ],
        ),
      );
    }
    return ListView.separated(
      padding: const EdgeInsets.symmetric(horizontal: 18),
      shrinkWrap: true,
      itemCount: _candidates.length,
      separatorBuilder: (_, _) => const SizedBox(height: 8),
      itemBuilder: (context, index) => _row(_candidates[index], index),
    );
  }

  Widget _row(SourceCandidate item, int index) {
    final selected = _selected == index;
    return GestureDetector(
      onTap: item.playable
          ? () {
              setState(() => _selected = index);
              Navigator.of(context).pop(item.toDrama());
            }
          : null,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 150),
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        decoration: BoxDecoration(
          color: selected ? const Color(0xFF231D24) : const Color(0xFF202025),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: selected ? DramaX.brand : const Color(0xFF2C2C32),
          ),
          boxShadow: selected
              ? [
                  BoxShadow(
                    color: DramaX.brand.withValues(alpha: .15),
                    blurRadius: 18,
                  ),
                ]
              : null,
        ),
        child: Row(
          children: [
            Container(
              width: 24,
              height: 24,
              decoration: BoxDecoration(
                color: selected ? DramaX.brand : const Color(0xFF2E2E34),
                borderRadius: BorderRadius.circular(7),
              ),
              alignment: Alignment.center,
              child: Text(
                '${index + 1}',
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w700,
                  color: selected ? Colors.white : DramaX.t2,
                ),
              ),
            ),
            const SizedBox(width: 11),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Flexible(
                        child: Text(
                          item.sourceLabel,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(
                            fontSize: 13,
                            fontWeight: FontWeight.w600,
                            color: DramaX.t1,
                          ),
                        ),
                      ),
                      const SizedBox(width: 7),
                      Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 6,
                          vertical: 2,
                        ),
                        decoration: BoxDecoration(
                          color: item.tagColor.withValues(alpha: .16),
                          borderRadius: BorderRadius.circular(5),
                        ),
                        child: Text(
                          item.tag,
                          style: TextStyle(
                            fontSize: 9,
                            fontWeight: FontWeight.w700,
                            color: item.tagColor,
                          ),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 7),
                  ClipRRect(
                    borderRadius: BorderRadius.circular(2),
                    child: LinearProgressIndicator(
                      value: item.health / 100,
                      minHeight: 4,
                      backgroundColor: const Color(0xFF313138),
                      valueColor: AlwaysStoppedAnimation(item.tagColor),
                    ),
                  ),
                  const SizedBox(height: 5),
                  Row(
                    children: [
                      Text(
                        '${item.episodes} 集',
                        style: const TextStyle(fontSize: 10, color: DramaX.t3),
                      ),
                      const SizedBox(width: 10),
                      Text(
                        item.vipEpisodes > 0
                            ? 'VIP ${item.vipEpisodes} 集'
                            : '无 VIP',
                        style: const TextStyle(fontSize: 10, color: DramaX.t3),
                      ),
                      const Spacer(),
                      Text(
                        '${item.elapsedMs}ms',
                        style: const TextStyle(fontSize: 10, color: DramaX.t3),
                      ),
                    ],
                  ),
                ],
              ),
            ),
            const SizedBox(width: 10),
            ScoreRing(score: item.health, color: item.tagColor),
            const SizedBox(width: 8),
            Container(
              width: 19,
              height: 19,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: selected ? DramaX.brand : Colors.transparent,
                border: Border.all(
                  color: selected ? DramaX.brand : const Color(0xFF4A4A52),
                  width: 1.5,
                ),
              ),
              child: selected
                  ? const Icon(Icons.check, size: 12, color: Colors.white)
                  : null,
            ),
          ],
        ),
      ),
    );
  }

  Widget _footer() => Row(
    children: [
      Expanded(
        child: OutlinedButton.icon(
          onPressed: widget.onManage,
          icon: const Icon(Icons.add, size: 16),
          label: const Text('添加播放源'),
          style: OutlinedButton.styleFrom(
            foregroundColor: const Color(0xFFDDDDDD),
            side: const BorderSide(color: Color(0xFF34343A)),
            backgroundColor: const Color(0xFF202025),
            minimumSize: const Size(0, 40),
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(12),
            ),
          ),
        ),
      ),
      const SizedBox(width: 10),
      Expanded(
        child: FilledButton.icon(
          onPressed: widget.onManage,
          icon: const Icon(Icons.settings, size: 16),
          label: const Text('管理播放源'),
          style: FilledButton.styleFrom(
            backgroundColor: DramaX.brand.withValues(alpha: .12),
            foregroundColor: DramaX.brand2,
            minimumSize: const Size(0, 40),
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(12),
            ),
          ),
        ),
      ),
    ],
  );
}
