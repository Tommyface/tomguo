import 'dart:ui' show ImageFilter;

import 'package:flutter/material.dart';

/// DramaX 设计令牌：与设计稿 `tokens.css` 一一对应。
///
/// 设计语言三条主线：
/// 1. **深色分层** —— 纯黑到 #1d1d20 五级背景，靠明度差分层，不靠描边。
/// 2. **品牌红 + CTA 黄** —— 红色只用于"当前状态"（选中、角标、主按钮），
///    黄色只用于"下一步动作"（看全集）。两者不混用。
/// 3. **分层磨砂** —— 面板/浮层用不同强度的 backdrop blur，越靠前的层越模糊。
abstract final class DramaX {
  // ---------- 背景分层 ----------
  static const bg0 = Color(0xFF000000);
  static const bg1 = Color(0xFF0C0C0E);
  static const bg2 = Color(0xFF151517);
  static const bg3 = Color(0xFF1D1D20);
  static const card = Color(0xFF1A1A1D);
  static const line = Color(0xFF2B2B30);

  // ---------- 文本 ----------
  static const t1 = Color(0xFFFFFFFF);
  static const t2 = Color(0xFF98989F);
  static const t3 = Color(0xFF66666D);

  // ---------- 品牌与语义 ----------
  static const brand = Color(0xFFFF2D4B);
  static const brand2 = Color(0xFFFF5F79);
  static const cta = Color(0xFFFFC21A);
  static const ctaText = Color(0xFF1A1A1A);
  static const ok = Color(0xFF34C759);
  static const warn = Color(0xFFFF9F0A);
  static const info = Color(0xFF5FA8FF);
  static const purple = Color(0xFFA78BFA);

  // ---------- 圆角 ----------
  static const rSm = 6.0;
  static const rMd = 10.0;
  static const rLg = 14.0;
  static const rXl = 18.0;

  // ---------- 间距（4pt 栅格） ----------
  static const sp1 = 4.0;
  static const sp2 = 8.0;
  static const sp3 = 12.0;
  static const sp4 = 16.0;
  static const sp6 = 24.0;

  // ---------- 磨砂强度（sigma 值） ----------
  /// HUD 小条：源信息、音量。
  static const frostHud = 8.0;

  /// Toast：自动切换提示。
  static const frostToast = 10.0;

  /// 面板遮罩：底部弹层的背景暗化。
  static const frostDim = 4.0;

  /// 设置页卡片：静态磨砂。
  static const frostStatic = 18.0;

  // ---------- 阴影 ----------
  static const shadowSm = [
    BoxShadow(color: Color(0x59000000), blurRadius: 8, offset: Offset(0, 2)),
  ];
  static const shadowMd = [
    BoxShadow(color: Color(0x73000000), blurRadius: 24, offset: Offset(0, 8)),
  ];

  /// 卡片封面底：设计稿用径向渐变模拟海报，真实封面加载前先铺一层。
  static const posterGradients = <List<Color>>[
    [Color(0xFF7C3AED), Color(0xFF3B1D63), Color(0xFF150E28)],
    [Color(0xFF0EA5A4), Color(0xFF134E5E), Color(0xFF0B2029)],
    [Color(0xFFE11D48), Color(0xFF7F1D3A), Color(0xFF2A0D1C)],
    [Color(0xFF2563EB), Color(0xFF1E3A8A), Color(0xFF0D1930)],
    [Color(0xFFEA580C), Color(0xFF7C2D12), Color(0xFF2A1208)],
    [Color(0xFFDB2777), Color(0xFF831843), Color(0xFF2A0D20)],
  ];

  /// 按剧集 id 稳定取一个海报渐变，避免刷新时颜色跳变。
  static List<Color> posterFor(String id) {
    var hash = 0;
    for (final unit in id.codeUnits) {
      hash = (hash * 31 + unit) & 0x7FFFFFFF;
    }
    return posterGradients[hash % posterGradients.length];
  }
}

/// 磨砂容器：设计稿里所有浮层都用它。
class FrostBox extends StatelessWidget {
  const FrostBox({
    super.key,
    required this.child,
    this.sigma = DramaX.frostStatic,
    this.color = const Color(0xE61A1A1D),
    this.borderColor = const Color(0x14FFFFFF),
    this.radius = DramaX.rLg,
    this.padding = EdgeInsets.zero,
  });

  final Widget child;
  final double sigma;
  final Color color;
  final Color borderColor;
  final double radius;
  final EdgeInsets padding;

  @override
  Widget build(BuildContext context) => ClipRRect(
    borderRadius: BorderRadius.circular(radius),
    child: BackdropFilter(
      filter: ImageFilter.blur(sigmaX: sigma, sigmaY: sigma),
      child: DecoratedBox(
        decoration: BoxDecoration(
          color: color,
          borderRadius: BorderRadius.circular(radius),
          border: Border.all(color: borderColor),
        ),
        child: Padding(padding: padding, child: child),
      ),
    ),
  );
}

/// 评分环：设计稿源列表右侧的圆锥渐变圆环。
class ScoreRing extends StatelessWidget {
  const ScoreRing({
    super.key,
    required this.score,
    required this.color,
    this.size = 34,
  });

  final int score;
  final Color color;
  final double size;

  @override
  Widget build(BuildContext context) => SizedBox.square(
    dimension: size,
    child: CustomPaint(
      painter: _RingPainter(progress: (score / 100).clamp(0, 1), color: color),
      child: Center(
        child: Text(
          '$score',
          style: TextStyle(
            fontSize: 9,
            fontWeight: FontWeight.w700,
            color: DramaX.t2.withValues(alpha: .92),
          ),
        ),
      ),
    ),
  );
}

class _RingPainter extends CustomPainter {
  const _RingPainter({required this.progress, required this.color});

  final double progress;
  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final center = size.center(Offset.zero);
    final radius = size.width / 2;
    final track = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 3
      ..color = const Color(0xFF313138);
    final arc = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 3
      ..strokeCap = StrokeCap.round
      ..color = color;
    canvas.drawCircle(center, radius - 1.5, track);
    canvas.drawArc(
      Rect.fromCircle(center: center, radius: radius - 1.5),
      -1.5707963267948966,
      6.283185307179586 * progress,
      false,
      arc,
    );
  }

  @override
  bool shouldRepaint(_RingPainter oldDelegate) =>
      oldDelegate.progress != progress || oldDelegate.color != color;
}

/// 信号格：设计稿 HUD 里的四格信号。
class SignalBars extends StatelessWidget {
  const SignalBars({super.key, required this.active, this.color = DramaX.ok});

  final int active;
  final Color color;

  @override
  Widget build(BuildContext context) => SizedBox(
    height: 13,
    child: Row(
      crossAxisAlignment: CrossAxisAlignment.end,
      children: [
        for (var index = 0; index < 4; index++)
          Padding(
            padding: EdgeInsets.only(right: index == 3 ? 0 : 2),
            child: Container(
              width: 3,
              height: 4.0 + index * 3,
              decoration: BoxDecoration(
                color: index < active ? color : const Color(0xFF4A4A52),
                borderRadius: BorderRadius.circular(1),
              ),
            ),
          ),
      ],
    ),
  );
}
