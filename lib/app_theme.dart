import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

/// 影院编辑风（Cinematic Editorial）设计令牌。
///
/// 全部使用系统字体与纯色值，零外部资源、零网络依赖，可完全离线使用。
abstract final class CinemaTokens {
  // ---------- DramaX 设计稿色板（与 design_tokens.dart 的 DramaX 一致） ----------
  /// 五级背景分层。
  static const dxBg0 = Color(0xFF000000);
  static const dxBg1 = Color(0xFF0C0C0E);
  static const dxBg2 = Color(0xFF151517);
  static const dxBg3 = Color(0xFF1D1D20);
  static const dxCard = Color(0xFF1A1A1D);
  static const dxLine = Color(0xFF2B2B30);

  /// 文本三级。
  static const dxT1 = Color(0xFFFFFFFF);
  static const dxT2 = Color(0xFF98989F);
  static const dxT3 = Color(0xFF66666D);

  /// 品牌红（当前状态）与 CTA 黄（下一步动作）。
  static const dxBrand = Color(0xFFFF2D4B);
  static const dxBrand2 = Color(0xFFFF5F79);
  static const dxCta = Color(0xFFFFC21A);
  static const dxCtaText = Color(0xFF1A1A1A);
  static const dxOk = Color(0xFF34C759);
  static const dxWarn = Color(0xFFFF9F0A);
  static const dxInfo = Color(0xFF5FA8FF);

  /// 落日琥珀金 —— 全局唯一强调色（60-30-10 中的 10，因稀有而有力）。
  static const amber = Color(0xFFF6C86B);
  static const amberBright = Color(0xFFFFDA8C);
  static const amberDim = Color(0xFFB98F3E);

  /// 暖橙红 —— 延续项目既有品牌主色，避免品牌断层。
  static const ember = Color(0xFFFF664F);

  /// 深炭底 —— 带暖色相，非纯黑（纯黑会丢失层次）。
  static const ink = Color(0xFF101114);
  static const inkRaised = Color(0xFF1A1A20);

  /// 衬线标题字体族：用通用 serif 族（Android/iOS/桌面均能映射），
  /// 比写死 'Noto Serif SC' 这类多数设备不存在的具体名字可靠得多。
  /// 仅用于标题，正文保持无衬线以保证可读性。
  static const displayFamily = 'serif';
  static const displayFallback = <String>[
    'Songti SC',
    'STSong',
    'SimSun',
  ];
}

abstract final class AppTheme {
  static final light = _theme(Brightness.light);
  static final dark = _theme(Brightness.dark);

  static ThemeMode mode(String preference) => switch (preference) {
    'light' => ThemeMode.light,
    'dark' => ThemeMode.dark,
    _ => ThemeMode.system,
  };

  static String label(String preference) => switch (preference) {
    'light' => '浅色',
    'dark' => '深色',
    _ => '跟随系统',
  };

  static SystemUiOverlayStyle systemBars(Brightness brightness) {
    final icons = brightness == Brightness.dark
        ? Brightness.light
        : Brightness.dark;
    return SystemUiOverlayStyle(
      statusBarColor: Colors.transparent,
      statusBarIconBrightness: icons,
      statusBarBrightness: brightness,
      systemStatusBarContrastEnforced: false,
      systemNavigationBarColor: Colors.transparent,
      systemNavigationBarDividerColor: Colors.transparent,
      systemNavigationBarIconBrightness: icons,
      systemNavigationBarContrastEnforced: false,
    );
  }

  static ThemeData _theme(Brightness brightness) {
    final dark = brightness == Brightness.dark;
    final background = dark ? CinemaTokens.dxBg1 : const Color(0xFFFAF8F6);
    // DramaX 色板：品牌红只用于「当前状态」，CTA 黄只用于「下一步动作」。
    final scheme = ColorScheme.fromSeed(
      seedColor: dark ? CinemaTokens.dxBrand : CinemaTokens.ember,
      brightness: brightness,
      primary: dark ? CinemaTokens.dxBrand : const Color(0xFFAD3826),
      onPrimary: Colors.white,
      primaryContainer: dark
          ? const Color(0xFF4A1220)
          : const Color(0xFFFFE0D8),
      onPrimaryContainer: dark
          ? const Color(0xFFFFD9E0)
          : const Color(0xFF4B160D),
      // 黄只给 CTA，不给选中态。
      tertiary: dark ? CinemaTokens.dxCta : const Color(0xFF8A6100),
      surface: dark ? CinemaTokens.dxBg2 : Colors.white,
      onSurface: dark ? CinemaTokens.dxT1 : const Color(0xFF252328),
      onSurfaceVariant: dark ? CinemaTokens.dxT2 : const Color(0xFF68636C),
      outline: dark ? const Color(0xFF4A4A52) : const Color(0xFF7C757D),
      outlineVariant: dark ? CinemaTokens.dxLine : const Color(0xFFE3DFDC),
      surfaceContainerLowest: dark ? CinemaTokens.dxBg0 : Colors.white,
      surfaceContainerLow: dark ? CinemaTokens.dxBg1 : const Color(0xFFF7F3F0),
      surfaceContainer: dark ? CinemaTokens.dxCard : const Color(0xFFF2EEEB),
      surfaceContainerHigh: dark
          ? CinemaTokens.dxBg3
          : const Color(0xFFEDE8E5),
      surfaceContainerHighest: dark
          ? const Color(0xFF232327)
          : const Color(0xFFE6E0DC),
      surfaceTint: Colors.transparent,
    );

    // 以 Material 3 默认主题为底，仅覆盖标题层级 —— 正文保持无衬线，
    // 标题改用衬线（宋体系）营造电影海报 / 杂志排版质感。
    final base = ThemeData(
      useMaterial3: true,
      brightness: brightness,
      colorScheme: scheme,
    );
    TextStyle display(
      TextStyle? src, {
      required double size,
      required FontWeight weight,
      double height = 1.12,
      double spacing = -0.4,
    }) =>
        (src ?? const TextStyle()).copyWith(
          fontFamily: CinemaTokens.displayFamily,
          fontFamilyFallback: CinemaTokens.displayFallback,
          fontSize: size,
          fontWeight: weight,
          height: height,
          letterSpacing: spacing,
        );

    final t = base.textTheme;
    final textTheme = t.copyWith(
      displayLarge: display(t.displayLarge, size: 57, weight: FontWeight.w700),
      displayMedium: display(t.displayMedium, size: 45, weight: FontWeight.w700),
      displaySmall: display(t.displaySmall, size: 36, weight: FontWeight.w600),
      headlineLarge: display(t.headlineLarge, size: 32, weight: FontWeight.w700),
      headlineMedium: display(t.headlineMedium, size: 28, weight: FontWeight.w700),
      headlineSmall: display(t.headlineSmall, size: 24, weight: FontWeight.w600),
      titleLarge: display(
        t.titleLarge,
        size: 22,
        weight: FontWeight.w700,
        spacing: 0,
      ),
    );

    return base.copyWith(
      scaffoldBackgroundColor: background,
      textTheme: textTheme,
      appBarTheme: AppBarTheme(
        backgroundColor: dark ? CinemaTokens.dxBg1 : background,
        foregroundColor: scheme.onSurface,
        titleTextStyle: textTheme.titleLarge?.copyWith(
          color: scheme.onSurface,
          letterSpacing: -.3,
        ),
        scrolledUnderElevation: 0,
        elevation: 0,
        centerTitle: false,
        // 设计稿不用描边分栏，靠背景明度差分层。
        systemOverlayStyle: systemBars(brightness),
      ),
      dividerTheme: DividerThemeData(color: scheme.outlineVariant),
      navigationRailTheme: NavigationRailThemeData(
        backgroundColor: background,
        useIndicator: false,
        selectedIconTheme: IconThemeData(color: scheme.tertiary),
        unselectedIconTheme: IconThemeData(color: scheme.onSurfaceVariant),
        selectedLabelTextStyle: TextStyle(
          color: scheme.tertiary,
          fontWeight: FontWeight.w700,
        ),
        unselectedLabelTextStyle: TextStyle(color: scheme.onSurfaceVariant),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: scheme.surfaceContainer,
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(14),
          borderSide: BorderSide.none,
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(14),
          borderSide: BorderSide(color: CinemaTokens.amber, width: 1.5),
        ),
      ),
      filledButtonTheme: FilledButtonThemeData(
        style: ButtonStyle(
          backgroundColor: WidgetStateProperty.resolveWith((states) =>
              states.contains(WidgetState.disabled)
                  ? scheme.surfaceContainerHighest
                  : scheme.primary),
          foregroundColor: WidgetStateProperty.all(scheme.onPrimary),
          padding: WidgetStateProperty.all(
            const EdgeInsets.symmetric(horizontal: 22, vertical: 14),
          ),
          shape: WidgetStateProperty.all(
            RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
          ),
          textStyle: WidgetStateProperty.all(
            const TextStyle(fontSize: 15, fontWeight: FontWeight.w700),
          ),
          elevation: WidgetStateProperty.all(0),
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: ButtonStyle(
          foregroundColor: WidgetStateProperty.all(scheme.onSurface),
          side: WidgetStateProperty.all(
            BorderSide(color: scheme.outlineVariant),
          ),
          padding: WidgetStateProperty.all(
            const EdgeInsets.symmetric(horizontal: 20, vertical: 13),
          ),
          shape: WidgetStateProperty.all(
            RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
          ),
          textStyle: WidgetStateProperty.all(
            const TextStyle(fontSize: 15, fontWeight: FontWeight.w600),
          ),
        ),
      ),
      iconButtonTheme: IconButtonThemeData(
        style: ButtonStyle(
          foregroundColor: WidgetStateProperty.all(scheme.onSurfaceVariant),
        ),
      ),
    );
  }
}
