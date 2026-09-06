#!/usr/bin/env python3

import importlib.util
from pathlib import Path
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

    def test_single_binary_receives_every_identity_field(self):
        proof = self.identity()
        binary = contract.core_ldflags(proof, 'v1.2.3', 'b' * 40, '2026-09-06T00:00:00Z')
        for field, key in contract.FIELDS.items():
            self.assertIn(f'{field}={proof[key]}', binary)

    def test_partial_or_malformed_identity_is_rejected_before_build(self):
        for proof in ({'edition': 'open'}, {**self.identity(), 'build_id': 'BAD'}):
            with self.assertRaises(RuntimeError):
                contract.core_ldflags(proof, 'v1.2.3', 'b' * 40, '2026-09-06T00:00:00Z')

if __name__ == '__main__':
    unittest.main()
