import base64
import os
from pathlib import Path
import plistlib
import runpy
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

from app_build import BuildVariant
from configure_ios_branding import configure


def dart_defines(*values):
    return ','.join(base64.b64encode(value.encode()).decode() for value in values)


class AppBuildTests(unittest.TestCase):
    def test_variant_is_always_the_full_source_edition(self):
        for encoded in ['', dart_defines('ALL_SOURCES=false'), dart_defines('OTHER=true'),
                        dart_defines('ALL_SOURCES=true')]:
            variant = BuildVariant.from_dart_defines(encoded)
            self.assertTrue(variant.all_sources)
            self.assertEqual(variant.name, '聚合短剧')
            self.assertEqual(variant.slug, 'juheduanju')
            self.assertEqual(variant.arguments, [])
            self.assertEqual(variant.flutter_arguments, [])
            self.assertEqual(variant.linker_flags, '-s -w')

    def test_default_variant_matches_full_source_edition(self):
        variant = BuildVariant()
        self.assertEqual(variant.name, '聚合短剧')
        self.assertEqual(variant.slug, 'juheduanju')

    def test_ios_branding_keeps_bundle_identity(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / 'Info.plist'
            original = {'CFBundleIdentifier': 'com.tomguo.ti', 'CFBundleVersion': '9'}
            path.write_bytes(plistlib.dumps(original))
            for flags in [[], ['ALL_SOURCES=false'], ['OTHER=true'], ['ALL_SOURCES=true']]:
                configure(path, dart_defines(*flags))
                actual = plistlib.loads(path.read_bytes())
                self.assertEqual(actual['CFBundleDisplayName'], '聚合短剧')
                self.assertEqual(actual['CFBundleName'], 'juheduanju')
                for key, value in original.items():
                    self.assertEqual(actual[key], value)

    @unittest.skipUnless(shutil.which('cmake'), 'CMake is unavailable')
    def test_windows_branding_is_fixed(self):
        branding = Path(__file__).resolve().parents[1] / 'windows/runner/app_branding.cmake'
        for flags in [[], ['ALL_SOURCES=false'], ['OTHER=true'], ['ALL_SOURCES=true']]:
            with self.subTest(flags=flags), tempfile.TemporaryDirectory() as temporary:
                script = Path(temporary) / 'check.cmake'
                result = Path(temporary) / 'name.txt'
                script.write_text(
                    'list(APPEND FLUTTER_TOOL_ENVIRONMENT "OTHER=1" "DART_DEFINES=' + dart_defines(*flags) + '")\n'
                    'include("' + branding.as_posix() + '")\n'
                    'file(WRITE "' + result.as_posix() + '" "${APP_DISPLAY_NAME}:${APP_EXECUTABLE_NAME}")\n',
                    encoding='utf-8',
                )
                subprocess.run(['cmake', '-P', str(script)], check=True, capture_output=True)
                self.assertEqual(result.read_text(encoding='utf-8'), '聚合短剧:juheduanju.exe')

    def test_android_and_windows_build_the_single_edition(self):
        root = Path(__file__).resolve().parent
        for target in ['android', 'windows']:
            with self.subTest(target=target):
                script = root / f'build_{target}.py'
                with mock.patch.object(sys, 'argv', [str(script)]), \
                        mock.patch.dict(os.environ, {'PATH': '/tools'}, clear=True), \
                        mock.patch('shutil.which', return_value='/tools/flutter'), \
                        mock.patch('subprocess.run') as run:
                    runpy.run_path(str(script), run_name='__main__')
                calls = [call.args[0] for call in run.call_args_list]
                native = next(call for call in calls if any(str(arg).endswith('build_native.py') for arg in call))
                flutter = next(call for call in calls if 'build' in call)
                package = next(call for call in calls if any(str(arg).endswith('package_release.py') for arg in call))
                self.assertNotIn('--all-sources', native)
                self.assertNotIn('--all-sources', package)
                self.assertNotIn('--dart-define=ALL_SOURCES', flutter)
                self.assertNotIn('buildAllSources', BuildVariant().linker_flags)


if __name__ == '__main__':
    unittest.main()
