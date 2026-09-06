#!/usr/bin/env python3

import base64
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location(
    'schema_package_contract', Path(__file__).with_name('schema_package_contract.py'))
contract = importlib.util.module_from_spec(spec)
spec.loader.exec_module(contract)


class SchemaPackageContractTests(unittest.TestCase):
    def identity(self):
        return ({key: ('12' if key.endswith('length') else 'a' * 64)
                 for key in contract.FIELDS.values()} |
                {'edition': 'open', 'go_runtime_version': 'go1.25.9'})

    def test_core_and_launcher_receive_every_identity_field(self):
        proof = self.identity()
        core = contract.core_ldflags(proof, 'v1.2.3', 'b' * 40, '2026-09-06T00:00:00Z')
        launcher = contract.launcher_ldflags(
            proof, 'v1.2.3', 'b' * 40, '2026-09-06T00:00:00Z', 'c' * 64, '42', 'c25hcHNob3Q')
        for field, key in contract.FIELDS.items():
            self.assertIn(f'{field}={proof[key]}', core)
            self.assertIn(f'{field}={proof[key]}', launcher)

    def test_partial_or_malformed_identity_is_rejected_before_build(self):
        for proof in ({'edition': 'open'}, {**self.identity(), 'build_id': 'BAD'}):
            with self.assertRaises(RuntimeError):
                contract.core_ldflags(proof, 'v1.2.3', 'b' * 40, '2026-09-06T00:00:00Z')

    def test_help_seal_compares_both_locales_with_final_core(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            core = root / 'core'
            core.write_text('#!/bin/sh\nprintf "sealed help\\n"\n')
            core.chmod(0o755)
            generator = root / 'generator'
            reference = base64.b64encode(b'sealed help\n').decode()
            payload = json.dumps({'Snapshot': 'snapshot', 'References': {'en': reference, 'zh': reference}})
            generator.write_text(f'#!/bin/sh\nprintf \'%s\\n\' \'{payload}\'\n')
            generator.chmod(0o755)
            proof = root / 'proof.json'
            snapshot = contract.seal_root_help(core, generator, contract.sha256(core), 'd' * 40, proof)
            self.assertEqual(snapshot, 'snapshot')
            self.assertTrue(json.loads(proof.read_text())['passed'])
            core.write_text('#!/bin/sh\nprintf "drifted help\\n"\n')
            with self.assertRaisesRegex(RuntimeError, 'help projection differs'):
                contract.seal_root_help(core, generator, contract.sha256(core), 'd' * 40, proof)
            self.assertFalse(json.loads(proof.read_text())['passed'])

    def test_cross_compiled_help_seal_records_native_comparison_as_pending(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            core = root / 'foreign-core'
            core.write_bytes(b'not executable on the build host')
            generator = root / 'generator'
            reference = base64.b64encode(b'sealed help\n').decode()
            payload = json.dumps({'Snapshot': 'snapshot', 'References': {'en': reference, 'zh': reference}})
            generator.write_text(f'#!/bin/sh\nprintf \'%s\\n\' \'{payload}\'\n')
            generator.chmod(0o755)
            proof_path = root / 'proof.json'

            snapshot = contract.seal_root_help(
                core, generator, contract.sha256(core), 'd' * 40, proof_path, compare_core=False)
            proof = json.loads(proof_path.read_text())
            self.assertEqual(snapshot, 'snapshot')
            self.assertFalse(proof['passed'])
            self.assertFalse(proof['native_core_compared'])
            self.assertEqual(proof['status'], 'pending_native_comparison')
            self.assertFalse(proof['release_eligible'])
            self.assertEqual(
                {entry['native_comparison'] for entry in proof['locales'].values()},
                {'deferred to final-artifact native runner'})


if __name__ == '__main__':
    unittest.main()
