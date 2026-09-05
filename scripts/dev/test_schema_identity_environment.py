#!/usr/bin/env python3
"""Exercise the actual native sandbox controls before trusting generator results."""

import importlib.util
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('identity_environment',
    Path(__file__).with_name('check-schema-identity-environment.py'))
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)


class IdentityEnvironmentTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='dws-proof-control-')
        self.addCleanup(self.temporary.cleanup)
        self.work = Path(self.temporary.name).resolve()
        self.fixture, self.home = self.work / 'fixture', self.work / 'home'
        self.fixture.mkdir()
        self.home.mkdir(mode=0o700)
        (self.fixture / 'read-control.txt').write_text('allowed fixture\n')
        self.forbidden = self.work / 'forbidden.txt'
        self.forbidden.write_text('harmless host sentinel\n')
        self.cat = Path('/bin/cat' if platform.system() == 'Darwin' else '/usr/bin/cat')

    def test_native_file_and_network_isolation_controls(self):
        if platform.system() not in ('Darwin', 'Linux'):
            self.skipTest('native sandbox controls require Darwin/Linux')
        if platform.system() == 'Linux' and not shutil.which('bwrap'):
            self.skipTest('install bubblewrap to run native controls; production checker never skips')
        # Paths remain literal policy arguments even with whitespace, quotes and
        # non-ASCII characters; they must not become broader SBPL expressions.
        original = self.fixture
        self.fixture = self.work / 'fixture with spaces "中文"'
        original.rename(self.fixture)
        sandbox = check.Sandbox(self.cat, self.fixture, self.home)
        for hostile in (False, True):
            result = check.check_controls(sandbox, check.environment(self.home, hostile), self.forbidden)
            self.assertTrue(all(result.values()))

    def test_broken_positive_control_cannot_count_as_isolation(self):
        class BrokenSandbox:
            fixture = self.fixture

            def run(self, *args, **kwargs):
                return subprocess.CompletedProcess([], -6, b'', b'')

        broken = BrokenSandbox()
        broken.cat = self.cat
        with self.assertRaisesRegex(RuntimeError, 'cannot read its allowed fixture'):
            check.check_controls(broken, check.environment(self.home), self.forbidden)

    def test_missing_isolator_cannot_use_unrestricted_process(self):
        with patch.object(check.platform, 'system', return_value='Linux'), \
             patch.object(check.shutil, 'which', return_value=None):
            with self.assertRaisesRegex(RuntimeError, 'no unrestricted fallback'):
                check.Sandbox(self.cat, self.fixture, self.home)


if __name__ == '__main__':
    unittest.main()
