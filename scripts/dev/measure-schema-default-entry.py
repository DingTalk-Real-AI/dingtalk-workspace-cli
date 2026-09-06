#!/usr/bin/env python3
"""Measure the single DWS entry against a fixed pre-PR baseline.

Schema cache/live runs cover CPU and RSS. Help/version runs cover both default
telemetry and the explicit DO_NOT_TRACK opt-out. The fixed baseline is the
release gate; competing CLIs remain diagnostic in the five-dimension report.
"""

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import random
import subprocess
import sys
import tempfile

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('candidate_measure', HERE / 'verify-schema-cache-binary.py')
measure = importlib.util.module_from_spec(spec)
spec.loader.exec_module(measure)
baseline_spec = importlib.util.spec_from_file_location('entry_baseline', HERE / 'build-schema-entry-baseline.py')
baseline_build = importlib.util.module_from_spec(baseline_spec)
baseline_spec.loader.exec_module(baseline_build)


def validate_baseline(binary, proof_path):
    proof = json.loads(proof_path.read_text())
    if proof.get('source_commit') != baseline_build.BASE_COMMIT:
        raise RuntimeError('baseline must use the immutable PR-base main commit')
    if proof.get('go_version') != 'go1.25.9' or proof.get('binary_sha256') != measure.digest(binary):
        raise RuntimeError('baseline bytes or toolchain differ from its build report')
    target = ('darwin', 'arm64') if sys.platform == 'darwin' else ('linux', 'amd64')
    if (proof.get('goos'), proof.get('goarch')) != target:
        raise RuntimeError('baseline must be built for the same native target')
    info = subprocess.check_output(['go', 'version', '-m', str(binary)], text=True)
    if info.splitlines()[0].split()[-1] != 'go1.25.9':
        raise RuntimeError('baseline binary must use Go 1.25.9')
    return proof


def measure_cases(cases, samples, seed, home, report):
    order = list(cases) * samples
    random.Random(seed).shuffle(order)
    report['sample_order'] = order
    raw = report['raw_samples'] = {name: [] for name in cases}
    for index, name in enumerate(order):
        binary, argv, env, expected = cases[name]
        opt_out = name.endswith('-opt-out')
        if ('DO_NOT_TRACK' in env) != opt_out:
            raise RuntimeError(f'{name}: DO_NOT_TRACK presence does not match case classification')
        output, usage = measure.invoke(binary, argv, env, home)
        raw[name].append(usage)
        if output != expected:
            raise RuntimeError(f'{name}: output differs from its sealed oracle')
        if (index + 1) % 30 == 0:
            print(f'default entry: {index + 1}/{len(order)} measured processes', file=sys.stderr, flush=True)
    summary = report['summary'] = {name: measure.summarize(values) for name, values in raw.items()}
    gates = {
        'schema_default_user_cpu_reduction_at_least_80_percent':
            summary['schema-cache']['user_ms']['p50'] <= .2 * summary['schema-live']['user_ms']['p50'],
        'schema_default_peak_rss_at_most_100_mib':
            max(s['max_rss_bytes'] for s in raw['schema-cache']) <= 100 * 1024 * 1024,
        'schema_opt_out_user_cpu_reduction_at_least_80_percent':
            summary['schema-cache-opt-out']['user_ms']['p50'] <= .2 * summary['schema-live-opt-out']['user_ms']['p50'],
        'schema_opt_out_peak_rss_at_most_100_mib':
            max(s['max_rss_bytes'] for s in raw['schema-cache-opt-out']) <= 100 * 1024 * 1024,
    }
    for entry in ('help', 'version'):
        for mode, suffix in (('default', ''), ('opt_out', '-opt-out')):
            for percentile in ('p50', 'p95'):
                gates[f'{entry}_pre_pr_{mode}_wall_{percentile}_regression_at_most_5_percent'] = (
                    summary[f'{entry}-candidate{suffix}']['wall_ms'][percentile] <=
                    1.05 * summary[f'{entry}-baseline{suffix}']['wall_ms'][percentile])
    entry_gates = [value for key, value in gates.items() if '_pre_pr_' in key]
    report['pre_pr_help_version_latency_proven'] = len(entry_gates) == 8 and all(entry_gates)
    report['gates'] = gates
    report['passed'] = all(gates.values()) and report['pre_pr_help_version_latency_proven']


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--proof', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--baseline', type=Path, required=True, help='exact native PR-base executable')
    parser.add_argument('--baseline-proof', type=Path, required=True, help='its baseline-build.json')
    parser.add_argument('--samples', type=int, default=30)
    parser.add_argument('--seed', type=int, default=20260906)
    args = parser.parse_args()
    if sys.platform not in ('darwin', 'linux') or not hasattr(os, 'wait4'):
        parser.error('requires native Darwin/Linux process accounting')
    if args.samples < 30:
        parser.error('at least 30 samples per mode are required')
    report = {'scope': __doc__.strip(), 'passed': False, 'release_eligible': False,
              'pre_pr_baseline_proven': False, 'competitive_acceptance_proven': False,
              'platform': platform.platform(), 'samples_per_mode': args.samples, 'seed': args.seed,
              'telemetry': 'help/version cover default tracking and explicit DO_NOT_TRACK opt-out',
              'network_sandboxed': False,
              'measurement': 'wait4 in a fresh sampler; sampler startup excluded from child wall time'}
    try:
        binary = args.binary.resolve(strict=True)
        build_path = binary.parent.parent / 'candidate-build.json'
        build = json.loads(build_path.read_text())
        binary_sha = measure.digest(binary)
        if build.get('binary_sha256') != binary_sha:
            raise RuntimeError('candidate differs from its finalized single-binary build record')
        proof = json.loads(args.proof.read_text())
        if proof.get('go_runtime_version') != 'go1.25.9' or build.get('identity_sha256') != measure.digest(args.proof):
            raise RuntimeError('candidate and native identity proof differ')
        info = subprocess.check_output(['go', 'version', '-m', str(binary)], text=True)
        if info.splitlines()[0].split()[-1] != 'go1.25.9':
            raise RuntimeError('candidate must use the pinned Go 1.25.9 toolchain')
        baseline = args.baseline.resolve(strict=True)
        baseline_proof = validate_baseline(baseline, args.baseline_proof)
        report.update(binary_sha256=binary_sha, proof_sha256=measure.digest(args.proof),
                      build_id=proof['build_id'], release={'version': build['version'],
                      'commit': build['source_commit'], 'edition': proof['edition']},
                      baseline_build=baseline_proof, pre_pr_baseline_proven=True)
        with tempfile.TemporaryDirectory(prefix='.dws-default-entry-', dir=Path.home()) as directory:
            home = Path(directory)
            cache = home / ('Library/Caches' if sys.platform == 'darwin' else '.cache')
            cache.mkdir(parents=True, mode=0o700)
            env = {'HOME': str(home), 'PATH': os.environ.get('PATH', '/usr/bin:/bin'),
                   'XDG_CACHE_HOME': str(cache), 'XDG_CONFIG_HOME': str(home / '.config'),
                   'DWS_CONFIG_DIR': str(home / '.dws'), 'LANG': 'C', 'LC_ALL': 'C'}
            live = {**env, 'DWS_SCHEMA_CACHE_DISABLE': '1'}
            leaf = ['schema', 'calendar.create_calendar_event', '--compact', '-f', 'json']
            expected, _ = measure.invoke(binary, leaf, {**live, 'DO_NOT_TRACK': '1'}, home)
            warmed, _ = measure.invoke(binary, leaf, {**env, 'DO_NOT_TRACK': '1'}, home)
            if warmed != expected:
                raise RuntimeError('cache warmup differs from authoritative assembly')
            artifact_root = cache / 'dws/schema' / hashlib.sha256(proof['edition'].encode()).hexdigest() / 'v1'
            for name, prefix in (('meta.cache', 'meta'), ('registry.shards.cache', 'registry')):
                data = (artifact_root / name).read_bytes()
                if len(data) != 208 + proof[prefix + '_length'] or hashlib.sha256(data[208:]).hexdigest() != proof[prefix + '_sha256']:
                    raise RuntimeError('warmed artifact differs from native identity proof')
            cases = {'schema-cache': (binary, leaf, env, expected),
                     'schema-live': (binary, leaf, live, expected),
                     'schema-cache-opt-out': (binary, leaf, {**env, 'DO_NOT_TRACK': '1'}, expected),
                     'schema-live-opt-out': (binary, leaf, {**live, 'DO_NOT_TRACK': '1'}, expected)}
            for name, argv in (('help', ['--help']), ('version', ['--version'])):
                candidate_output, _ = measure.invoke(binary, argv, {**env, 'DO_NOT_TRACK': '1'}, home)
                baseline_output, _ = measure.invoke(baseline, argv, {**env, 'DO_NOT_TRACK': '1'}, home)
                if not candidate_output or not baseline_output:
                    raise RuntimeError(f'{name}: empty output')
                for suffix, mode_env in (('', env), ('-opt-out', {**env, 'DO_NOT_TRACK': '1'})):
                    cases[name + '-candidate' + suffix] = (binary, argv, mode_env, candidate_output)
                    cases[name + '-baseline' + suffix] = (baseline, argv, mode_env, baseline_output)
            measure_cases(cases, args.samples, args.seed, home, report)
        if measure.digest(binary) != binary_sha or measure.digest(baseline) != baseline_proof['binary_sha256']:
            raise RuntimeError('measured executable bytes changed')
    except Exception as error:
        report.update(passed=False, error=str(error))
        raise
    finally:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({'report': str(args.output), 'gates': report['gates']}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
