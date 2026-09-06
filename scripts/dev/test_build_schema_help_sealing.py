import base64
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('candidate_build', Path(__file__).with_name('build-schema-cache-candidate.py'))
build = importlib.util.module_from_spec(spec)
spec.loader.exec_module(build)

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
        return build.seal_root_help(self.core, self.generator, build.sha256(self.core), 'a' * 40, self.output)

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

    def test_core_changed_during_verification_cannot_seal(self):
        self.write_core('printf "%s help\\n" "$LANG"\necho "# mutation" >> "$0"')
        with self.assertRaisesRegex(RuntimeError, 'core changed'):
            self.seal()
        self.assertFalse(json.loads((self.output / 'root-help-proof.json').read_text())['passed'])

    def test_missing_snapshot_cannot_seal(self):
        self.write_generator(snapshot='')
        with self.assertRaisesRegex(RuntimeError, 'empty snapshot'):
            self.seal()
