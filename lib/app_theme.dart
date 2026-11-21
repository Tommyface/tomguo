import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'design_tokens.dart';

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
    final background = dark ? DramaX.bg1 : const Color(0xFFFAF8F6);
    // DramaX 色板：品牌红只用于「当前状态」，CTA 黄只用于「下一步动作」。
    final scheme = ColorScheme.fromSeed(
      seedColor: DramaX.brand,
      brightness: brightness,
      primary: dark ? DramaX.brand : const Color(0xFFAD3826),
      onPrimary: Colors.white,
      primaryContainer: dark
          ? const Color(0xFF4A1220)
          : const Color(0xFFFFE0D8),
      onPrimaryContainer: dark
          ? const Color(0xFFFFD9E0)
          : const Color(0xFF4B160D),
      // 黄只给 CTA，不给选中态。
      tertiary: dark ? DramaX.cta : const Color(0xFF8A6100),
      surface: dark ? DramaX.bg2 : Colors.white,
      onSurface: dark ? DramaX.t1 : const Color(0xFF252328),
      onSurfaceVariant: dark ? DramaX.t2 : const Color(0xFF68636C),
      outline: dark ? const Color(0xFF4A4A52) : const Color(0xFF7C757D),
      outlineVariant: dark ? DramaX.line : const Color(0xFFE3DFDC),
      surfaceContainerLowest: dark ? DramaX.bg0 : Colors.white,
      surfaceContainerLow: dark ? DramaX.bg1 : const Color(0xFFF7F3F0),
      surfaceContainer: dark ? DramaX.card : const Color(0xFFF2EEEB),
      surfaceContainerHigh: dark
          ? DramaX.bg3
          : const Color(0xFFEDE8E5),
      surfaceContainerHighest: dark
          ? const Color(0xFF232327)
          : const Color(0xFFE6E0DC),
      surfaceTint: Colors.transparent,
    );

    // 以 Material 3 默认主题为底，仅覆盖标题层级的字号 / 字重 / 行高。
    // 字体沿用系统无衬线（设计稿语言），正文与标题一致，不引入衬线族。
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
        backgroundColor: dark ? DramaX.bg1 : background,
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
          borderSide: BorderSide(color: DramaX.brand, width: 1.5),
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
