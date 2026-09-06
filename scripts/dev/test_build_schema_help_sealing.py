import base64
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location(
    'schema_package_contract', Path(__file__).resolve().parents[1] / 'build/schema_package_contract.py')
contract = importlib.util.module_from_spec(spec)
spec.loader.exec_module(contract)

@unittest.skipIf(os.name == 'nt', 'native candidate proof uses POSIX controls')
class HelpSealingTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='dws-help-sealing-')
        self.addCleanup(self.temporary.cleanup)
        self.output = Path(self.temporary.name).resolve()
        self.core = self.output / 'core'
        self.generator = self.output / 'generator'
        self.write_core('printf "%s help\\n" "$LANG"')
        self.write_generator()

    def write_core(self, action):
        self.core.write_text('#!/bin/sh\n[ "$NO_COLOR" = 1 ] && [ "$DO_NOT_TRACK" = 1 ] || exit 11\n' + action + '\n')
        self.core.chmod(0o755)

    def write_generator(self, snapshot='sealed-snapshot'):
        result = {'Snapshot': snapshot, 'References': {lang: base64.b64encode((lang + ' help\n').encode()).decode() for lang in ('en', 'zh')}}
        self.generator.write_text("#!/bin/sh\ncat <<'PROJECTION'\n" + json.dumps(result) + '\nPROJECTION\n')
        self.generator.chmod(0o755)

    def seal(self):
        return contract.seal_root_help(
            self.core, self.generator, contract.sha256(self.core), 'a' * 40,
            self.output / 'root-help-proof.json', release_eligible=False)

    def test_real_children_match_both_locales_before_sealing(self):
        self.assertEqual(self.seal(), 'sealed-snapshot')
        proof = json.loads((self.output / 'root-help-proof.json').read_text())
        self.assertTrue(proof['passed'])
        self.assertEqual(set(proof['locales']), {'en', 'zh'})
        self.assertFalse(proof['release_eligible'])

    def test_drift_diagnostics_and_nonzero_cannot_seal(self):
        for action in ('printf "different help\\n"', 'printf "%s help\\n" "$LANG"; echo warning >&2', 'exit 7'):
            with self.subTest(action=action):
                self.write_core(action)
                with self.assertRaises(Exception):
                    self.seal()
                proof = json.loads((self.output / 'root-help-proof.json').read_text())
                self.assertFalse(proof['passed'])
                self.assertIn('error', proof)
                detail = proof['locales']['en']
                self.assertFalse(detail['equal'])
                self.assertEqual(base64.b64decode(detail['expected_stdout']['base64']), b'en help\n')
                if 'different' in action:
                    self.assertEqual(base64.b64decode(detail['actual_stdout']['base64']), b'different help\n')
                elif 'warning' in action:
                    self.assertEqual(base64.b64decode(detail['stderr']['base64']), b'warning\n')
                else:
                    self.assertEqual(detail['returncode'], 7)

    def test_generator_failure_retains_output_before_proof_directory_cleanup(self):
        for code in (0, 6):
            with self.subTest(code=code):
                self.generator.write_text('#!/bin/sh\nprintf "partial projection"\nprintf "generator failure" >&2\nexit ' + str(code) + '\n')
                with self.assertRaises(Exception):
                    self.seal()
                proof = json.loads((self.output / 'root-help-proof.json').read_text())
                self.assertFalse(proof['passed'])
                failed = proof['failed_process']
                self.assertEqual(failed['returncode'], code)
                self.assertEqual(base64.b64decode(failed['stdout']['base64']), b'partial projection')
                self.assertEqual(base64.b64decode(failed['stderr']['base64']), b'generator failure')

    def test_core_changed_during_verification_cannot_seal(self):
        self.write_core('printf "%s help\\n" "$LANG"\necho "# mutation" >> "$0"')
        with self.assertRaisesRegex(RuntimeError, 'core changed'):
            self.seal()
        self.assertFalse(json.loads((self.output / 'root-help-proof.json').read_text())['passed'])

    def test_missing_snapshot_cannot_seal(self):
        self.write_generator(snapshot='')
        with self.assertRaisesRegex(RuntimeError, 'invalid snapshot'):
            self.seal()
