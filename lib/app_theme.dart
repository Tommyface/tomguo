import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

/// 影院编辑风（Cinematic Editorial）设计令牌。
///
/// 全部使用系统字体与纯色值，零外部资源、零网络依赖，可完全离线使用。
abstract final class CinemaTokens {
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
    final background = dark ? CinemaTokens.ink : const Color(0xFFFAF8F6);
    final scheme = ColorScheme.fromSeed(
      seedColor: CinemaTokens.ember,
      brightness: brightness,
      primary: dark ? const Color(0xFFFF765F) : const Color(0xFFAD3826),
      onPrimary: dark ? const Color(0xFF3B0E07) : Colors.white,
      primaryContainer: dark
          ? const Color(0xFF5C3027)
          : const Color(0xFFFFE0D8),
      onPrimaryContainer: dark
          ? const Color(0xFFFFE0D8)
          : const Color(0xFF4B160D),
      // 琥珀金作为第三强调色：徽章、选中态、评分、CTA 高亮。
      tertiary: dark ? CinemaTokens.amber : const Color(0xFF8A6100),
      surface: dark ? const Color(0xFF16171B) : Colors.white,
      onSurface: dark ? const Color(0xFFF2F0F4) : const Color(0xFF252328),
      onSurfaceVariant: dark
          ? const Color(0xFFB9B6C2)
          : const Color(0xFF68636C),
      outline: dark ? const Color(0xFF85818B) : const Color(0xFF7C757D),
      outlineVariant: dark ? const Color(0xFF34343D) : const Color(0xFFE3DFDC),
      surfaceContainerLowest: dark ? const Color(0xFF0D0E11) : Colors.white,
      surfaceContainerLow: dark
          ? const Color(0xFF191A20)
          : const Color(0xFFF7F3F0),
      surfaceContainer: dark
          ? const Color(0xFF1E2026)
          : const Color(0xFFF2EEEB),
      surfaceContainerHigh: dark
          ? const Color(0xFF25262D)
          : const Color(0xFFEDE8E5),
      surfaceContainerHighest: dark
          ? const Color(0xFF30313A)
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
        backgroundColor: background,
        foregroundColor: scheme.onSurface,
        // 深色底用亮金、浅色底用深琥珀，保证两种主题下标题对比度都达标。
        titleTextStyle: textTheme.titleLarge?.copyWith(
          color: dark ? CinemaTokens.amber : scheme.tertiary,
          letterSpacing: .3,
        ),
        scrolledUnderElevation: 0,
        elevation: 0,
        centerTitle: false,
        shape: Border(
          bottom: BorderSide(
            color: dark
                ? CinemaTokens.amber.withValues(alpha: .28)
                : CinemaTokens.amberDim.withValues(alpha: .5),
            width: 1,
          ),
        ),
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
