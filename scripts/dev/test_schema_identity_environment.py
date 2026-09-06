#!/usr/bin/env python3
"""Exercise the actual native sandbox controls before trusting generator results."""

import importlib.util
import json
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
        if platform.system() == 'Linux' and not shutil.which('docker'):
            self.skipTest('Docker is required for native controls; production checker never skips')
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

    def linux_sandbox(self, layers=None):
        image = 'sha256:' + 'a' * 64
        if layers is None:
            layers = ['sha256:' + check.hashlib.sha256(bytes(10240)).hexdigest()]
        details = [{'Id': image, 'RootFS': {'Type': 'layers', 'Layers': layers}, 'Config': {}}]
        with patch.object(check.platform, 'system', return_value='Linux'), \
             patch.object(check.shutil, 'which', return_value='/usr/bin/docker'), \
             patch.object(check.os, 'getuid', return_value=1001), \
             patch.dict(check.os.environ, {'DWS_SCHEMA_PROOF_IMAGE': image}), \
             patch.object(check.subprocess, 'check_output', side_effect=['linux/amd64\n', json.dumps(details).encode()]):
            return check.Sandbox(self.cat, self.fixture, self.home)

    def test_nonempty_image_cannot_supply_hidden_inputs(self):
        with self.assertRaisesRegex(RuntimeError, 'exact empty tar'):
            self.linux_sandbox(['sha256:' + 'b' * 64])

    def test_linux_container_has_no_host_network_or_privileges(self):
        sandbox = self.linux_sandbox()
        prefix = sandbox.prefix
        for flag in ('--network=none', '--read-only', '--cap-drop=ALL', '--security-opt=no-new-privileges', '--pull=never'):
            self.assertIn(flag, prefix)
        self.assertEqual(prefix[:3], ['/usr/bin/docker', '--host', 'unix:///var/run/docker.sock'])
        mounts = [prefix[i + 1] for i, item in enumerate(prefix) if item == '--mount']
        self.assertTrue(all(value.endswith(',readonly') for value in mounts))
        self.assertFalse(any('docker.sock' in value for value in mounts))

    def test_timeout_removes_only_the_owned_container(self):
        sandbox = self.linux_sandbox()
        with patch.object(check.subprocess, 'run', side_effect=[subprocess.TimeoutExpired('docker', 1),
                                                              subprocess.CompletedProcess([], 0)]) as run:
            with self.assertRaises(subprocess.TimeoutExpired):
                sandbox.run([self.cat, self.fixture / 'read-control.txt'], check.environment(self.home), timeout=1)
        start, cleanup = [call.args[0] for call in run.call_args_list]
        name = start[start.index('--name') + 1]
        self.assertTrue(name.startswith('dws-schema-proof-'))
        self.assertEqual(cleanup, sandbox.docker + ['rm', '--force', name])

    def test_mutable_image_reference_cannot_be_used_for_proof(self):
        with patch.object(check.platform, 'system', return_value='Linux'), \
             patch.object(check.shutil, 'which', return_value='/usr/bin/docker'), \
             patch.dict(check.os.environ, {'DWS_SCHEMA_PROOF_IMAGE': 'ubuntu:latest'}):
            with self.assertRaisesRegex(RuntimeError, 'exact locally imported empty image ID'):
                check.Sandbox(self.cat, self.fixture, self.home)

    def test_missing_isolator_cannot_use_unrestricted_process(self):
        with patch.object(check.platform, 'system', return_value='Linux'), \
             patch.object(check.shutil, 'which', return_value=None):
            with self.assertRaisesRegex(RuntimeError, 'no unrestricted fallback'):
                check.Sandbox(self.cat, self.fixture, self.home)


if __name__ == '__main__':
    unittest.main()
