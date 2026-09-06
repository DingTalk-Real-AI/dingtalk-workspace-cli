#!/usr/bin/env python3
"""Measure default-tracker Schema and canonical package entry overhead.

Uses the exact candidate and its packaged core with a fresh HOME and no auth
profile. DO_NOT_TRACK is absent from every measured child: identity resolution,
tracking and the normal flush budget remain active. Network conditions are not
controlled. This is not a pre-PR baseline or competitive acceptance report.
"""

import argparse
import importlib.util
import hashlib
import json
import os
from pathlib import Path
import platform
import random
import subprocess
import sys
import tempfile

spec = importlib.util.spec_from_file_location('candidate_measure',
    Path(__file__).with_name('verify-schema-cache-binary.py'))
measure = importlib.util.module_from_spec(spec)
spec.loader.exec_module(measure)


def measure_cases(cases, samples, seed, home, report):
    order = list(cases) * samples
    random.Random(seed).shuffle(order)
    report['sample_order'] = order
    raw = report['raw_samples'] = {name: [] for name in cases}
    for index, name in enumerate(order):
        binary, argv, env, expected = cases[name]
        if 'DO_NOT_TRACK' in env:
            raise RuntimeError('default-entry measurement must not set DO_NOT_TRACK')
        output, usage = measure.invoke(binary, argv, env, home)
        raw[name].append(usage)
        if output != expected:
            raise RuntimeError(f'{name}: default-tracker output differs from authoritative core')
        if (index + 1) % 30 == 0:
            print(f'default entry: {index + 1}/{len(order)} measured processes', file=sys.stderr, flush=True)
    report['summary'] = {name: measure.summarize(values) for name, values in raw.items()}
    summary = report['summary']
    gates = {
        'schema_user_cpu_reduction_at_least_80_percent':
            summary['schema-cache']['user_ms']['p50'] <= .2 * summary['schema-live']['user_ms']['p50'],
        'schema_peak_rss_at_most_100_mib':
            max(s['max_rss_bytes'] for s in raw['schema-cache']) <= 100 * 1024 * 1024,
    }
    for entry in ('help', 'version'):
        for percentile in ('p50', 'p95'):
            gates[f'{entry}_package_wall_{percentile}_overhead_at_most_5_percent'] = (
                summary[f'{entry}-launcher']['wall_ms'][percentile] <=
                1.05 * summary[f'{entry}-core']['wall_ms'][percentile])
    report['gates'] = gates
    report['passed'] = all(gates.values())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--proof', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
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
              'telemetry': 'DO_NOT_TRACK absent in every measured child; default tracker unchanged',
              'network_sandboxed': False,
              'measurement': 'wait4 in a fresh sampler; sampler startup excluded from child wall time'}
    try:
        binary = args.binary.resolve(strict=True)
        manifest = json.loads((binary.parent.parent / 'package-manifest.json').read_text())
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
        with tempfile.TemporaryDirectory(prefix='.dws-default-entry-', dir=Path.home()) as directory:
            home = Path(directory)
            cache = home / ('Library/Caches' if sys.platform == 'darwin' else '.cache')
            cache.mkdir(parents=True, mode=0o700)
            env = {'HOME': str(home), 'PATH': os.environ.get('PATH', '/usr/bin:/bin'),
                   'XDG_CACHE_HOME': str(cache), 'XDG_CONFIG_HOME': str(home / '.config'),
                   'DWS_CONFIG_DIR': str(home / '.dws'), 'LANG': 'C', 'LC_ALL': 'C'}
            live = {**env, 'DWS_SCHEMA_CACHE_DISABLE': '1'}
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
                     'schema-live': (binary, leaf, live, expected)}
            for name, argv in (('help', ['--help']), ('version', ['--version'])):
                expected, _ = measure.invoke(core, argv, {**env, 'DO_NOT_TRACK': '1'}, home)
                cases[name + '-launcher'] = (binary, argv, env, expected)
                cases[name + '-core'] = (core, argv, env, expected)
            measure_cases(cases, args.samples, args.seed, home, report)
        if measure.digest(binary) != binary_sha or measure.digest(core) != core_sha:
            raise RuntimeError('finalized candidate bytes changed during measurement')
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
