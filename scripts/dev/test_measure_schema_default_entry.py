#!/usr/bin/env python3

import importlib.util
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('default_entry',
    Path(__file__).with_name('measure-schema-default-entry.py'))
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)


@unittest.skipIf(os.name == 'nt', 'native process accounting requires POSIX')
class DefaultEntryTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='dws-default-entry-test-')
        self.addCleanup(self.temporary.cleanup)
        self.home = Path(self.temporary.name).resolve()
        self.binary = self.home / 'entry'
        self.binary.write_text('''#!/bin/sh
[ "${DO_NOT_TRACK+x}" != x ] || exit 21
if [ "$1" = schema-live ]; then
  [ "$DWS_SCHEMA_CACHE_DISABLE" = 1 ] || exit 22
else
  [ "${DWS_SCHEMA_CACHE_DISABLE+x}" != x ] || exit 23
fi
printf 'exact output\\n'
''')
        self.binary.chmod(0o755)
        env = {'HOME': str(self.home), 'PATH': '/usr/bin:/bin'}
        self.cases = {}
        for name in ('schema-cache', 'schema-live', 'help-launcher', 'help-core', 'version-launcher', 'version-core'):
            child_env = dict(env)
            if name == 'schema-live':
                child_env['DWS_SCHEMA_CACHE_DISABLE'] = '1'
            self.cases[name] = (self.binary, [name], child_env, b'exact output\n')

    def test_actual_children_ignore_parent_opt_out_and_keep_live_control(self):
        report = {'passed': False}
        with patch.dict(os.environ, {'DO_NOT_TRACK': '1'}):
            check.measure_cases(self.cases, 1, 42, self.home, report)
        self.assertEqual(set(report['sample_order']), set(self.cases))
        self.assertTrue(all(len(values) == 1 for values in report['raw_samples'].values()))
        self.assertEqual(len(report['gates']), 6)
        # No latency acceptance claim is made from one shell-fixture sample.

    def test_output_drift_fails_with_partial_evidence(self):
        name = 'schema-cache'
        binary, argv, env, _ = self.cases[name]
        report = {'passed': False}
        with self.assertRaisesRegex(RuntimeError, 'output differs'):
            check.measure_cases({name: (binary, argv, env, b'wrong\n')}, 1, 42, self.home, report)
        self.assertFalse(report['passed'])
        self.assertEqual(len(report['raw_samples'][name]), 1)

    def test_explicit_opt_out_cannot_be_labeled_default(self):
        binary, argv, env, expected = self.cases['schema-cache']
        with self.assertRaisesRegex(RuntimeError, 'must not set DO_NOT_TRACK'):
            check.measure_cases({'schema-cache': (binary, argv, {**env, 'DO_NOT_TRACK': '1'}, expected)},
                                1, 42, self.home, {'passed': False})


if __name__ == '__main__':
    unittest.main()
