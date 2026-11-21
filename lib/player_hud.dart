import 'package:flutter/material.dart';

import 'design_tokens.dart';

/// 播放器顶部 HUD：源名称 + 延迟 + 信号格 + 线路点。
///
/// 设计稿里这一条是关键信息位：用户随时知道"现在播的是哪个源、质量如何"。
class PlayerSourceHud extends StatelessWidget {
  const PlayerSourceHud({
    super.key,
    required this.sourceLabel,
    required this.latencyMs,
    this.activeBars = 4,
    this.totalLines = 3,
    this.activeLine = 1,
  });

  final String sourceLabel;
  final int latencyMs;
  final int activeBars;
  final int totalLines;
  final int activeLine;

  @override
  Widget build(BuildContext context) => FrostBox(
    sigma: DramaX.frostHud,
    radius: 17,
    color: const Color(0xA80A0A0E),
    borderColor: const Color(0x33FFFFFF),
    padding: const EdgeInsets.symmetric(horizontal: 11),
    child: SizedBox(
      height: 34,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          SignalBars(active: activeBars),
          const SizedBox(width: 8),
          Text(
            sourceLabel,
            style: const TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w600,
              color: Colors.white,
            ),
          ),
          const SizedBox(width: 6),
          Text(
            '${latencyMs}ms',
            style: const TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w600,
              color: DramaX.ok,
              fontFeatures: [FontFeature.tabularFigures()],
            ),
          ),
          if (totalLines > 1) ...[
            const SizedBox(width: 8),
            Container(width: 1, height: 12, color: const Color(0x40FFFFFF)),
            const SizedBox(width: 8),
            Row(
              children: [
                for (var index = 0; index < totalLines; index++)
                  Padding(
                    padding: EdgeInsets.only(right: index == totalLines - 1 ? 0 : 3),
                    child: Container(
                      width: 5,
                      height: 5,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: index < activeLine
                            ? DramaX.ok
                            : const Color(0xFF4A4A52),
                      ),
                    ),
                  ),
              ],
            ),
          ],
        ],
      ),
    ),
  );
}

/// 自动切换提示条：源超时切到备用源时浮出，4 秒后淡出。
class PlayerSwitchToast extends StatelessWidget {
  const PlayerSwitchToast({
    super.key,
    required this.fromLabel,
    required this.toLabel,
  });

  final String fromLabel;
  final String toLabel;

  @override
  Widget build(BuildContext context) => FrostBox(
    sigma: DramaX.frostToast,
    radius: 11,
    color: const Color(0xEB121A14),
    borderColor: DramaX.ok.withValues(alpha: .4),
    padding: const EdgeInsets.symmetric(horizontal: 11, vertical: 8),
    child: Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(
          width: 20,
          height: 20,
          decoration: BoxDecoration(
            color: DramaX.ok.withValues(alpha: .2),
            borderRadius: BorderRadius.circular(6),
          ),
          child: const Icon(Icons.check, size: 13, color: DramaX.ok),
        ),
        const SizedBox(width: 9),
        Flexible(
          child: RichText(
            text: TextSpan(
              style: const TextStyle(fontSize: 11, color: Color(0xFFD8F5DF)),
              children: [
                TextSpan(text: '$fromLabel 响应超时，已自动切换至 '),
                TextSpan(
                  text: toLabel,
                  style: const TextStyle(
                    color: DramaX.ok,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ],
            ),
          ),
        ),
      ],
    ),
  );
}

/// 主按钮：设计稿的黄色 CTA（看全集）。
class DramaXCta extends StatelessWidget {
  const DramaXCta({
    super.key,
    required this.label,
    required this.onPressed,
    this.icon = Icons.play_arrow_rounded,
  });

  final String label;
  final VoidCallback? onPressed;
  final IconData icon;

  @override
  Widget build(BuildContext context) => SizedBox(
    height: 42,
    child: FilledButton.icon(
      onPressed: onPressed,
      icon: Icon(icon, size: 18),
      label: Text(
        label,
        style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w700),
      ),
      style: FilledButton.styleFrom(
        backgroundColor: DramaX.cta,
        foregroundColor: DramaX.ctaText,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(21),
        ),
      ),
    ),
  );
}

/// 次级按钮：切换源。
class DramaXGhostButton extends StatelessWidget {
  const DramaXGhostButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.icon = Icons.swap_horiz_rounded,
  });

  final String label;
  final VoidCallback? onPressed;
  final IconData icon;

  @override
  Widget build(BuildContext context) => SizedBox(
    height: 42,
    child: OutlinedButton.icon(
      onPressed: onPressed,
      icon: Icon(icon, size: 16),
      label: Text(
        label,
        style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w600),
      ),
      style: OutlinedButton.styleFrom(
        foregroundColor: Colors.white,
        backgroundColor: const Color(0x8C14141A),
        side: const BorderSide(color: Color(0x33FFFFFF)),
        padding: const EdgeInsets.symmetric(horizontal: 12),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(21),
        ),
      ),
    ),
  );
}
