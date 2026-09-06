#!/usr/bin/env python3

import importlib.util
import json
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
case "$1" in
  *-opt-out) [ "$DO_NOT_TRACK" = 1 ] || exit 21 ;;
  *) [ "${DO_NOT_TRACK+x}" != x ] || exit 24 ;;
esac
case "$1" in schema-live*)
  [ "$DWS_SCHEMA_CACHE_DISABLE" = 1 ] || exit 22
  ;;
*)
  [ "${DWS_SCHEMA_CACHE_DISABLE+x}" != x ] || exit 23
  ;;
esac
printf 'exact output\\n'
''')
        self.binary.chmod(0o755)
        env = {'HOME': str(self.home), 'PATH': '/usr/bin:/bin'}
        self.cases = {}
        for name in ('schema-cache', 'schema-live', 'schema-cache-opt-out', 'schema-live-opt-out',
                     'help-launcher', 'help-core',
                     'version-launcher', 'version-core', 'help-launcher-opt-out',
                     'help-core-opt-out', 'version-launcher-opt-out', 'version-core-opt-out'):
            child_env = dict(env)
            if name.startswith('schema-live'):
                child_env['DWS_SCHEMA_CACHE_DISABLE'] = '1'
            if name.endswith('-opt-out'):
                child_env['DO_NOT_TRACK'] = '1'
            self.cases[name] = (self.binary, [name], child_env, b'exact output\n')

    def test_actual_children_ignore_parent_opt_out_and_keep_live_control(self):
        report = {'passed': False}
        with patch.dict(os.environ, {'DO_NOT_TRACK': '1'}):
            check.measure_cases(self.cases, 1, 42, self.home, report)
        self.assertEqual(set(report['sample_order']), set(self.cases))
        self.assertTrue(all(len(values) == 1 for values in report['raw_samples'].values()))
        self.assertEqual(len(report['gates']), 4)
        self.assertEqual(len(report['diagnostics']), 8)
        # No latency acceptance claim is made from one shell-fixture sample.

    def test_output_drift_fails_with_partial_evidence(self):
        name = 'schema-cache'
        binary, argv, env, _ = self.cases[name]
        report = {'passed': False}
        with self.assertRaisesRegex(RuntimeError, 'output differs'):
            check.measure_cases({name: (binary, argv, env, b'wrong\n')}, 1, 42, self.home, report)
        self.assertFalse(report['passed'])
        self.assertEqual(len(report['raw_samples'][name]), 1)

    def test_opt_out_presence_must_match_case_classification(self):
        binary, argv, env, expected = self.cases['schema-cache']
        with self.assertRaisesRegex(RuntimeError, 'does not match case classification'):
            check.measure_cases({'schema-cache': (binary, argv, {**env, 'DO_NOT_TRACK': '1'}, expected)},
                                1, 42, self.home, {'passed': False})
        with self.assertRaisesRegex(RuntimeError, 'does not match case classification'):
            check.measure_cases({'help-launcher-opt-out': (binary, argv, env, expected)},
                                1, 42, self.home, {'passed': False})

    def test_good_package_comparison_cannot_hide_pre_pr_regression(self):
        cases = dict(self.cases)
        for entry in ('help', 'version'):
            for suffix in ('', '-opt-out'):
                cases[entry + '-baseline' + suffix] = (
                    self.binary, [entry + '-baseline' + suffix],
                    cases[entry + '-core' + suffix][2], b'exact output\n')

        def invoke(binary, argv, env, home):
            name = argv[0]
            # Package core and launcher agree, but help regresses 10% from
            # the pre-PR executable. This must fail the independent gate.
            wall = 10 if name.startswith('help-baseline') else 11
            return b'exact output\n', {'wall_ms': wall,
                'user_ms': 100 if name.startswith('schema-live') else 1,
                'system_ms': 1, 'max_rss_bytes': 1024}

        report = {'passed': False}
        with patch.object(check.measure, 'invoke', side_effect=invoke):
            check.measure_cases(cases, 30, 42, self.home, report)
        self.assertTrue(report['diagnostics']['help_same_package_default_wall_p95_overhead_at_most_5_percent'])
        self.assertFalse(report['gates']['help_pre_pr_default_wall_p95_regression_at_most_5_percent'])
        self.assertFalse(report['gates']['help_pre_pr_opt_out_wall_p95_regression_at_most_5_percent'])
        self.assertFalse(report['pre_pr_help_version_latency_proven'])
        self.assertFalse(report['passed'])

    def test_same_package_regression_is_diagnostic_only(self):
        cases = dict(self.cases)
        for entry in ('help', 'version'):
            for suffix in ('', '-opt-out'):
                cases[entry + '-baseline' + suffix] = (
                    self.binary, [entry + '-baseline' + suffix],
                    cases[entry + '-core' + suffix][2], b'exact output\n')

        def invoke(binary, argv, env, home):
            name = argv[0]
            wall = 9 if '-core' in name else 10
            return b'exact output\n', {'wall_ms': wall,
                'user_ms': 100 if name.startswith('schema-live') else 1,
                'system_ms': 1, 'max_rss_bytes': 1024}

        report = {'passed': False}
        with patch.object(check.measure, 'invoke', side_effect=invoke):
            check.measure_cases(cases, 30, 42, self.home, report)
        self.assertTrue(report['pre_pr_help_version_latency_proven'])
        self.assertTrue(report['passed'])
        self.assertFalse(report['diagnostics']['help_same_package_default_wall_p95_overhead_at_most_5_percent'])

    def test_wrong_source_or_modified_baseline_cannot_supply_comparison(self):
        proof = self.home / 'baseline-build.json'
        for record, error in (({'source_commit': 'a' * 40}, 'immutable pre-PR'),
                              ({'source_commit': check.baseline_build.BASE_COMMIT,
                                'go_version': 'go1.25.9', 'binary_sha256': 'b' * 64}, 'baseline bytes')):
            proof.write_text(json.dumps(record))
            with self.assertRaisesRegex(RuntimeError, error):
                check.validate_baseline(self.binary, proof)

    def test_core_hash_diagnostic_detects_mutation(self):
        expected = check.measure.digest(self.binary)
        result = check.measure_core_hash_diagnostic(self.binary, expected, 2)
        self.assertEqual(len(result['raw_samples']), 2)
        with self.assertRaisesRegex(RuntimeError, 'core changed'):
            check.measure_core_hash_diagnostic(self.binary, '0' * 64, 1)


if __name__ == '__main__':
    unittest.main()
