#!/usr/bin/env python3

import importlib.util
import os
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('package_version', Path(__file__).with_name('verify-package-version.py'))
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)


@unittest.skipIf(os.name == 'nt', 'shell fixtures require POSIX; real native binaries are checked in CI')
class PackageVersionTests(unittest.TestCase):
    version, commit, build_time = 'v1.2.3', 'a' * 40, '2026-09-06T01:02:03Z'

    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix='dws-version-contract-')
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name).resolve()
        self.launcher, self.core = root / 'bin/dws', root / 'libexec/dws-core'
        self.expected = f'dws version {self.version} ({self.commit}, {self.build_time})'
        for path in (self.launcher, self.core):
            path.parent.mkdir()
            self.write_binary(path, self.expected)

    def write_binary(self, path, output, suffix=''):
        quoted = "'" + output.replace("'", "'\"'\"'") + "'"
        path.write_text("#!/bin/sh\nprintf '%s\\n' " + quoted + '\n' + suffix)
        path.chmod(0o755)

    def verify(self):
        report = {'passed': False}
        check.verify(self.launcher, self.core, self.version, self.commit, self.build_time, report,
                     verify_hot_paths=False)
        return report

    def test_exact_metadata_and_unchanged_binaries_pass(self):
        report = self.verify()
        self.assertTrue(report['passed'])
        self.assertEqual(len(report['runs']), 3)
        self.assertEqual(report['binaries']['core'], check.sha256(self.core))

    def test_consistently_wrong_time_does_not_pass_by_parity_alone(self):
        for path in (self.launcher, self.core):
            self.write_binary(path, self.expected.replace('01:02:03Z', '01:02:04Z'))
        with self.assertRaisesRegex(RuntimeError, 'version contract failed'):
            self.verify()

    def test_bare_launcher_version_is_rejected(self):
        self.write_binary(self.launcher, f'dws version {self.version}')
        with self.assertRaisesRegex(RuntimeError, 'launcher version contract failed'):
            self.verify()

    def test_always_delegating_launcher_fails_core_free_probe(self):
        self.launcher.write_text('#!/bin/sh\nexec "$(dirname "$0")/../libexec/dws-core" "$@"\n')
        with self.assertRaisesRegex(RuntimeError, 'core-free-launcher version contract failed'):
            self.verify()

    def test_version_cannot_modify_the_finalized_binary(self):
        self.write_binary(self.core, self.expected, "printf '\\n# mutation\\n' >> \"$0\"\n")
        with self.assertRaisesRegex(RuntimeError, 'core changed during'):
            self.verify()


if __name__ == '__main__':
    unittest.main()
