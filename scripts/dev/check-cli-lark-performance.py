#!/usr/bin/env python3
"""Evaluate one five-dimensions report against Lark, using raw default-entry samples.

This is one platform/run's diagnostic acceptance, not three-run/two-platform
or release approval. Historical 30-sample reports produce observed comparisons
but cannot pass the plan's 100-sample requirement. No old release gate changes.
"""

import argparse
import hashlib
import json
import math
from pathlib import Path
import re
import statistics


def evaluate(report):
    result = {'passed': False, 'scope': __doc__, 'comparisons': {}, 'failures': []}
    failures = result['failures']
    if report.get('complete') is not True or report.get('failures') or report.get('fatal_error'):
        failures.append('source measurement is incomplete or contains failures')
    count = report.get('samples_per_case_per_phase')
    if not isinstance(count, int) or isinstance(count, bool) or count < 100:
        failures.append('at least 100 samples per case per phase are required')
    for group in ('executables', 'artifact_trees'):
        entries = report.get(group, {})
        required = ('dws-native', 'dws-public', 'lark-native', 'lark-public') if group == 'executables' else ('dws-candidate-package', 'dws-public-package', 'comparison-install')
        for name in required:
            if name not in entries:
                failures.append(f'missing {group}/{name} binding')
        for name, entry in entries.items():
            digest = entry.get('sha256', '')
            if not re.fullmatch(r'[0-9a-f]{64}', digest) or digest != entry.get('final_sha256'):
                failures.append(f'{group}/{name}: missing or changed final binding')
    lock = report.get('comparison_package_lock_sha256', '')
    if not re.fullmatch(r'[0-9a-f]{64}', lock) or lock != report.get('comparison_package_lock_final_sha256'):
        failures.append('comparison package lock is missing or changed')
    for entry in ('native', 'public'):
        memory_metric = 'kernel_process_peak_rss_bytes' if entry == 'native' else 'sampled_tree_peak_rss_bytes'
        memory_method = 'kernel_process_peak' if entry == 'native' else 'sampled_process_tree'
        for workload in ('schema', 'help', 'version', 'leaf-help', 'dry-run'):
            keys = [f'{product}-{entry}/{workload}' for product in ('dws', 'lark')]
            for key in keys:
                case = report.get('cases', {}).get(key, {})
                if 'env' not in case or 'DO_NOT_TRACK' in case.get('env', {}):
                    failures.append(f'{key}: default environment is missing or opted out')
                if case.get('cache') != 'warm' or case.get('memory_method') != memory_method:
                    failures.append(f'{key}: cache or memory method differs from contract')
                if not case.get('stdout_bytes') or len(case.get('stdout_sha256', '')) != 64:
                    failures.append(f'{key}: missing output proof')
            for phase, metric in (('raw_samples', 'wall_ms'), ('raw_memory', memory_metric)):
                samples = []
                for key in keys:
                    rows = report.get(phase, {}).get(key, [])
                    if not rows or len(rows) != count:
                        failures.append(f'{key}/{phase}: missing samples')
                    if phase == 'raw_memory' and entry == 'public' and any(row.get('max_simultaneous_processes', 0) < 2 for row in rows):
                        failures.append(f'{key}/{phase}: wrapper and child were not observed together')
                    values = [row.get(metric) for row in rows]
                    if not values or any(isinstance(x, bool) or not isinstance(x, (int, float)) or not math.isfinite(x) or x <= 0 for x in values):
                        failures.append(f'{key}/{phase}: missing, nonfinite or nonpositive values')
                        samples.append(None)
                    else:
                        samples.append(sorted(values))
                if any(values is None for values in samples):
                    continue
                for percentile in ('p50', 'p95'):
                    values = [statistics.median(xs) if percentile == 'p50' else xs[math.ceil(len(xs) * .95) - 1] for xs in samples]
                    key = f'{entry}/{workload}/{metric}/{percentile}'
                    passed = values[0] < values[1]
                    result['comparisons'][key] = {'dws': values[0], 'lark': values[1], 'ratio': values[0] / values[1], 'passed': passed}
                    if not passed:
                        failures.append(f'{key}: DWS is not faster/smaller than Lark')
    result['passed'] = not failures and len(result['comparisons']) == 40
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--report', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    raw = args.report.read_bytes()
    result = evaluate(json.loads(raw))
    result['source_report_sha256'] = hashlib.sha256(raw).hexdigest()
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + '\n')
    print(f"Lark comparison: {sum(row['passed'] for row in result['comparisons'].values())}/40 observed metrics pass; accepted={result['passed']}")
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
