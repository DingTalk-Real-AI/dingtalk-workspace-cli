#!/usr/bin/env python3
"""Measure default-tracker Schema and canonical package entry overhead.

Uses the exact candidate and its packaged core with a fresh HOME and no auth
profile. Help/version are measured both with the default tracker and with the
explicit DO_NOT_TRACK opt-out. Network conditions are not controlled. An
optional pinned pre-PR binary adds whole-PR help/version latency gates. The
same-package launcher/core comparison is diagnostic only. This is not
competitive acceptance or cache-I/O absence proof.
"""

import argparse
import importlib.util
import hashlib
import json
import os
from pathlib import Path
import platform
import random
import shutil
import subprocess
import sys
import tempfile
import time

spec = importlib.util.spec_from_file_location('candidate_measure',
    Path(__file__).with_name('verify-schema-cache-binary.py'))
measure = importlib.util.module_from_spec(spec)
spec.loader.exec_module(measure)
baseline_spec = importlib.util.spec_from_file_location('entry_baseline',
    Path(__file__).with_name('build-schema-entry-baseline.py'))
baseline_build = importlib.util.module_from_spec(baseline_spec)
baseline_spec.loader.exec_module(baseline_build)


def validate_baseline(binary, proof_path):
    proof = json.loads(proof_path.read_text())
    if proof.get('source_commit') != baseline_build.BASE_COMMIT:
        raise RuntimeError('baseline must use the immutable pre-PR RFC commit')
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
            raise RuntimeError(f'{name}: default-tracker output differs from authoritative core')
        if (index + 1) % 30 == 0:
            print(f'default entry: {index + 1}/{len(order)} measured processes', file=sys.stderr, flush=True)
    report['summary'] = {name: measure.summarize(values) for name, values in raw.items()}
    summary = report['summary']
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
    diagnostics = {}
    for entry in ('help', 'version'):
        for mode, suffix in (('default', ''), ('opt_out', '-opt-out')):
            for percentile in ('p50', 'p95'):
                diagnostics[f'{entry}_same_package_{mode}_wall_{percentile}_overhead_at_most_5_percent'] = (
                    summary[f'{entry}-launcher{suffix}']['wall_ms'][percentile] <=
                    1.05 * summary[f'{entry}-core{suffix}']['wall_ms'][percentile])
                if f'{entry}-baseline{suffix}' in summary:
                    gates[f'{entry}_pre_pr_{mode}_wall_{percentile}_regression_at_most_5_percent'] = (
                        summary[f'{entry}-launcher{suffix}']['wall_ms'][percentile] <=
                        1.05 * summary[f'{entry}-baseline{suffix}']['wall_ms'][percentile])
    baseline_gates = [value for key, value in gates.items() if '_pre_pr_' in key]
    report['pre_pr_help_version_latency_proven'] = len(baseline_gates) == 8 and all(baseline_gates)
    report['diagnostics'] = diagnostics
    report['gates'] = gates
    report['passed'] = all(gates.values()) and report['pre_pr_help_version_latency_proven']


def measure_core_hash_diagnostic(core, expected_sha256, samples):
    """Measure the mandatory full-core digest separately from entry gates."""
    raw = []
    for _ in range(samples):
        started = time.perf_counter()
        actual = measure.digest(core)
        raw.append({'wall_ms': (time.perf_counter() - started) * 1000})
        if actual != expected_sha256:
            raise RuntimeError('core changed during SHA-256 diagnostic')
    return {
        'method': 'Python SHA-256 over every finalized core byte; excluded from gates',
        'raw_samples': raw,
        'summary': measure.summarize(raw),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--proof', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--baseline', type=Path, help='exact native pre-PR executable')
    parser.add_argument('--baseline-proof', type=Path, help='its baseline-build.json')
    parser.add_argument('--samples', type=int, default=30)
    parser.add_argument('--seed', type=int, default=20260906)
    args = parser.parse_args()
    if sys.platform not in ('darwin', 'linux') or not hasattr(os, 'wait4'):
        parser.error('requires native Darwin/Linux process accounting')
    if args.samples < 30:
        parser.error('at least 30 samples per mode are required')
    if bool(args.baseline) != bool(args.baseline_proof):
        parser.error('--baseline and --baseline-proof must be supplied together')
    report = {'scope': __doc__.strip(), 'passed': False, 'release_eligible': False,
              'pre_pr_baseline_proven': False, 'competitive_acceptance_proven': False,
              'pre_pr_help_version_latency_proven': False, 'cache_io_absence_proven': False,
              'platform': platform.platform(), 'samples_per_mode': args.samples, 'seed': args.seed,
              'telemetry': 'help/version cover default tracking and explicit DO_NOT_TRACK opt-out',
              'network_sandboxed': False,
              'measurement': 'wait4 in a fresh sampler; sampler startup excluded from child wall time'}
    try:
        binary = args.binary.resolve(strict=True)
        manifest = json.loads((binary.parent.parent / 'package-manifest.json').read_text())
        if manifest.get('capabilities') != {
                'schema_cache': 'enabled', 'root_help': 'enabled', 'disabled_reason': ''}:
            raise RuntimeError('candidate manifest does not declare enabled Schema/help capabilities')
        core = (binary.parent.parent / manifest['core']['path']).resolve(strict=True)
        binary_sha, core_sha = measure.digest(binary), measure.digest(core)
        if binary_sha != manifest['launcher']['sha256'] or core_sha != manifest['core']['sha256']:
            raise RuntimeError('candidate bytes differ from finalized package manifest')
        proof = json.loads(args.proof.read_text())
        if proof.get('go_runtime_version') != 'go1.25.9':
            raise RuntimeError('proof must use the pinned Go 1.25.9 toolchain')
        for executable in (binary, core):
            build_info = subprocess.check_output(['go', 'version', '-m', str(executable)], text=True)
            if build_info.splitlines()[0].split()[-1] != 'go1.25.9':
                raise RuntimeError('launcher and core must use the pinned Go 1.25.9 toolchain')
        if manifest['release']['edition'] != proof['edition']:
            raise RuntimeError('candidate edition differs from native identity proof')
        report.update(binary_sha256=binary_sha, core_sha256=core_sha,
                      proof_sha256=measure.digest(args.proof), build_id=proof['build_id'],
                      release=manifest['release'])
        baseline = args.baseline.resolve(strict=True) if args.baseline else None
        if baseline:
            baseline_proof = validate_baseline(baseline, args.baseline_proof)
            report['baseline_build'] = baseline_proof
            report['baseline_proof_sha256'] = measure.digest(args.baseline_proof)
        with tempfile.TemporaryDirectory(prefix='.dws-default-entry-', dir=Path.home()) as directory:
            home = Path(directory)
            cache = home / ('Library/Caches' if sys.platform == 'darwin' else '.cache')
            cache.mkdir(parents=True, mode=0o700)
            env = {'HOME': str(home), 'PATH': os.environ.get('PATH', '/usr/bin:/bin'),
                   'XDG_CACHE_HOME': str(cache), 'XDG_CONFIG_HOME': str(home / '.config'),
                   'DWS_CONFIG_DIR': str(home / '.dws'), 'LANG': 'C', 'LC_ALL': 'C'}
            live = {**env, 'DWS_SCHEMA_CACHE_DISABLE': '1'}
            # Exercise the default tracked version in an identical launcher
            # with no core available, before any Schema artifacts are created.
            # This proves execution ownership, not absence of attempted I/O or
            # successful delivery to the external telemetry service.
            version_output, _ = measure.invoke(core, ['--version'], {**env, 'DO_NOT_TRACK': '1'}, home)
            isolated = home / 'version-only' / 'bin' / 'dws'
            isolated.parent.mkdir(parents=True)
            shutil.copy2(binary, isolated)
            if measure.digest(isolated) != binary_sha:
                raise RuntimeError('isolated version launcher differs from the finalized candidate')
            isolated_output, _ = measure.invoke(isolated, ['--version'], env, home)
            if isolated_output != version_output or measure.digest(isolated) != binary_sha:
                raise RuntimeError('default core-free version changed output or executable bytes')
            report['default_version_core_free'] = {
                'passed': True, 'launcher_sha256': binary_sha,
                'stdout_sha256': hashlib.sha256(isolated_output).hexdigest(),
                'do_not_track_present': False, 'core_present': False,
                'telemetry_delivery_proven': False,
            }
            report['default_help_core_free'] = {}
            for locale in ('en', 'zh'):
                help_env = {**env, 'LANG': locale}
                expected_help, _ = measure.invoke(core, ['--help'], {**help_env, 'DO_NOT_TRACK': '1'}, home)
                actual_help, _ = measure.invoke(isolated, ['--help'], help_env, home)
                if actual_help != expected_help or measure.digest(isolated) != binary_sha:
                    raise RuntimeError(f'{locale}: default core-free help changed output or executable bytes')
                report['default_help_core_free'][locale] = {
                    'passed': True, 'launcher_sha256': binary_sha,
                    'stdout_sha256': hashlib.sha256(actual_help).hexdigest(),
                    'do_not_track_present': False, 'core_present': False,
                    'telemetry_delivery_proven': False,
                }
            leaf = ['schema', 'calendar.create_calendar_event', '--compact', '-f', 'json']
            # Preparatory calls are excluded from measurements. Get the oracle
            # through full declaration assembly, then authenticate warmed bytes.
            expected, _ = measure.invoke(core, leaf, {**live, 'DO_NOT_TRACK': '1'}, home)
            warmed, _ = measure.invoke(binary, leaf, {**env, 'DO_NOT_TRACK': '1'}, home)
            if warmed != expected:
                raise RuntimeError('cache warmup differs from authoritative core')
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
                expected, _ = measure.invoke(core, argv, {**env, 'DO_NOT_TRACK': '1'}, home)
                for mode, suffix, mode_env in (
                        ('default', '', env),
                        ('opt_out', '-opt-out', {**env, 'DO_NOT_TRACK': '1'})):
                    cases[name + '-launcher' + suffix] = (binary, argv, mode_env, expected)
                    cases[name + '-core' + suffix] = (core, argv, mode_env, expected)
                    if baseline:
                        baseline_output, _ = measure.invoke(baseline, argv, mode_env, home)
                        if not baseline_output:
                            raise RuntimeError('baseline entry produced empty output')
                        if name == 'version':
                            expected_version = (f"dws version {baseline_proof['version']} "
                                                f"({baseline_proof['source_commit']}, {baseline_proof['build_time']})\n").encode()
                            if baseline_output != expected_version:
                                raise RuntimeError('baseline version differs from its sealed build metadata')
                        # Help and version can legitimately change across source
                        # commits; each baseline invocation must match its own oracle.
                        key = f'{name}_{mode}'
                        report.setdefault('baseline_output_sha256', {})[key] = hashlib.sha256(baseline_output).hexdigest()
                        cases[name + '-baseline' + suffix] = (baseline, argv, mode_env, baseline_output)
            report['core_sha256_diagnostic'] = measure_core_hash_diagnostic(core, core_sha, args.samples)
            measure_cases(cases, args.samples, args.seed, home, report)
        if measure.digest(binary) != binary_sha or measure.digest(core) != core_sha:
            raise RuntimeError('finalized candidate bytes changed during measurement')
        if baseline and measure.digest(baseline) != baseline_proof['binary_sha256']:
            raise RuntimeError('baseline executable changed during measurement')
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
