import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('lark_check', Path(__file__).with_name('check-cli-lark-performance.py'))
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)


def fixture():
    digest = 'a' * 64
    report = {'complete': True, 'failures': {}, 'samples_per_case_per_phase': 100,
              'comparison_package_lock_sha256': digest, 'comparison_package_lock_final_sha256': digest,
              'executables': {}, 'artifact_trees': {}, 'cases': {}, 'raw_samples': {}, 'raw_memory': {}}
    for name in ('dws-candidate-package', 'dws-public-package', 'comparison-install'):
        report['artifact_trees'][name] = {'sha256': digest, 'final_sha256': digest}
    for entry in ('native', 'public'):
        metric = 'kernel_process_peak_rss_bytes' if entry == 'native' else 'sampled_tree_peak_rss_bytes'
        method = 'kernel_process_peak' if entry == 'native' else 'sampled_process_tree'
        for product in ('dws', 'lark'):
            report['executables'][f'{product}-{entry}'] = {'sha256': digest, 'final_sha256': digest}
            for workload in ('schema', 'help', 'version', 'leaf-help', 'dry-run'):
                key = f'{product}-{entry}/{workload}'
                value = 1 if product == 'dws' else 2
                report['cases'][key] = {'env': {}, 'cache': 'warm', 'memory_method': method, 'stdout_bytes': 10, 'stdout_sha256': digest}
                report['raw_samples'][key] = [{'wall_ms': value} for _ in range(100)]
                report['raw_memory'][key] = [{metric: value, 'max_simultaneous_processes': 2} for _ in range(100)]
    return report


class LarkComparisonTests(unittest.TestCase):
    def test_all_forty_metrics_are_required(self):
        result = check.evaluate(fixture())
        self.assertTrue(result['passed'], result['failures'])
        self.assertEqual(len(result['comparisons']), 40)

    def test_equal_is_not_exceeding(self):
        report = fixture()
        report['raw_samples']['dws-native/help'] = [{'wall_ms': 2} for _ in range(100)]
        self.assertFalse(check.evaluate(report)['passed'])

    def test_tail_failure_cannot_hide_behind_median_or_supplied_summary(self):
        report = fixture()
        report['raw_samples']['dws-native/help'][-10:] = [{'wall_ms': 20} for _ in range(10)]
        report['raw_samples_summary'] = {'dws-native/help': {'wall_ms': {'p50': .1, 'p95': .1}}}
        result = check.evaluate(report)
        self.assertTrue(result['comparisons']['native/help/wall_ms/p50']['passed'])
        self.assertFalse(result['comparisons']['native/help/wall_ms/p95']['passed'])

    def test_incomplete_or_incomparable_evidence_never_passes(self):
        mutations = [
            lambda r: r.update(complete=False),
            lambda r: r.update(failures={'dry-run': 'exit 1'}),
            lambda r: r.update(samples_per_case_per_phase=30),
            lambda r: r['cases']['dws-native/help']['env'].update(DO_NOT_TRACK='1'),
            lambda r: r['cases']['dws-native/help'].update(memory_method='sampled_process_tree'),
            lambda r: r['raw_samples']['lark-native/help'].pop(),
            lambda r: r['raw_samples']['dws-native/help'][0].update(wall_ms=float('nan')),
            lambda r: r['raw_samples']['lark-native/help'][0].update(wall_ms=float('inf')),
            lambda r: r['raw_memory']['dws-public/help'][0].update(max_simultaneous_processes=1),
            lambda r: r['raw_memory'].pop('lark-public/schema'),
            lambda r: r['executables'].pop('dws-native'),
            lambda r: r['artifact_trees']['comparison-install'].update(final_sha256='b' * 64),
            lambda r: r.pop('comparison_package_lock_final_sha256'),
        ]
        for index, mutate in enumerate(mutations):
            with self.subTest(index=index):
                report = fixture()
                mutate(report)
                self.assertFalse(check.evaluate(report)['passed'])


if __name__ == '__main__':
    unittest.main()
