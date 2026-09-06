#!/usr/bin/env python3

import importlib.util
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('default_entry', Path(__file__).with_name('measure-schema-default-entry.py'))
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)


@unittest.skipIf(os.name == 'nt', 'native process accounting requires POSIX')
class DefaultEntryTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='dws-default-entry-test-')
        self.addCleanup(self.temporary.cleanup)
        self.home = Path(self.temporary.name).resolve()
        self.binary = self.home / 'entry'
        self.binary.write_text('#!/bin/sh\nprintf "exact output\\n"\n')
        self.binary.chmod(0o755)
        env = {'HOME': str(self.home), 'PATH': '/usr/bin:/bin'}
        self.cases = {}
        for name in ('schema-cache', 'schema-live', 'schema-cache-opt-out', 'schema-live-opt-out',
                     'help-candidate', 'help-baseline', 'version-candidate', 'version-baseline',
                     'help-candidate-opt-out', 'help-baseline-opt-out',
                     'version-candidate-opt-out', 'version-baseline-opt-out'):
            child_env = dict(env)
            if name.startswith('schema-live'):
                child_env['DWS_SCHEMA_CACHE_DISABLE'] = '1'
            if name.endswith('-opt-out'):
                child_env['DO_NOT_TRACK'] = '1'
            self.cases[name] = (self.binary, [name], child_env, b'exact output\n')

    def test_actual_children_ignore_parent_opt_out(self):
        report = {'passed': False}
        with patch.dict(os.environ, {'DO_NOT_TRACK': '1'}):
            check.measure_cases(self.cases, 1, 42, self.home, report)
        self.assertEqual(set(report['sample_order']), set(self.cases))
        self.assertEqual(len(report['gates']), 12)

    def test_output_drift_fails_with_partial_evidence(self):
        binary, argv, env, _ = self.cases['schema-cache']
        report = {'passed': False}
        with self.assertRaisesRegex(RuntimeError, 'output differs'):
            check.measure_cases({'schema-cache': (binary, argv, env, b'wrong\n')}, 1, 42, self.home, report)
        self.assertEqual(len(report['raw_samples']['schema-cache']), 1)

    def test_opt_out_presence_must_match_classification(self):
        binary, argv, env, expected = self.cases['schema-cache']
        with self.assertRaisesRegex(RuntimeError, 'does not match case classification'):
            check.measure_cases({'schema-cache': (binary, argv, {**env, 'DO_NOT_TRACK': '1'}, expected)},
                                1, 42, self.home, {'passed': False})

    def run_synthetic(self, candidate_wall):
        def invoke(binary, argv, env, home):
            name = argv[0]
            wall = 10 if '-baseline' in name else candidate_wall
            return b'exact output\n', {'wall_ms': wall,
                'user_ms': 100 if name.startswith('schema-live') else 1,
                'system_ms': 1, 'max_rss_bytes': 1024}
        report = {'passed': False}
        with patch.object(check.measure, 'invoke', side_effect=invoke):
            check.measure_cases(self.cases, 30, 42, self.home, report)
        return report

    def test_candidate_regression_against_main_fails(self):
        report = self.run_synthetic(11)
        self.assertFalse(report['gates']['help_pre_pr_default_wall_p95_regression_at_most_5_percent'])
        self.assertFalse(report['pre_pr_help_version_latency_proven'])
        self.assertFalse(report['passed'])

    def test_candidate_within_main_budget_passes(self):
        report = self.run_synthetic(10.4)
        self.assertTrue(report['pre_pr_help_version_latency_proven'])
        self.assertTrue(report['passed'])


if __name__ == '__main__':
    unittest.main()
