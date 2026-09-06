#!/usr/bin/env python3
"""Native/public CLI comparison and ordinary-command paired process measurements.

Warm metadata, private HOME, default telemetry, no real credentials or business
requests. Memory trials are separate: native entries use kernel wait4 peak RSS
from a small C parent; public wrappers use psutil concurrent process-tree samples
with a requested 1 ms interval (an observed lower bound, not exact peak RSS).
Latency uses fresh blocking-wait4 samplers with no psutil polling interference.
"""

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import random
import shutil
import stat
import subprocess
import sys
import tempfile

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('entry', HERE / 'measure-schema-default-entry.py')
entry = importlib.util.module_from_spec(spec)
spec.loader.exec_module(entry)
measure = entry.measure

EXPECTED_CASE_COUNT = 44


def tree_digest(root):
    """Hash a dependency tree without following symlinks or trusting mtimes."""
    root = root.resolve(strict=True)
    digest = hashlib.sha256()

    def field(value):
        digest.update(len(value).to_bytes(8, 'big'))
        digest.update(value)

    field(b'dws-measurement-tree-v1')
    for path in sorted(root.rglob('*'), key=lambda item: item.relative_to(root).as_posix()):
        relative = path.relative_to(root).as_posix().encode()
        status = path.lstat()
        field(relative)
        field(str(stat.S_IMODE(status.st_mode)).encode())
        if stat.S_ISDIR(status.st_mode):
            field(b'directory')
        elif stat.S_ISLNK(status.st_mode):
            field(b'symlink')
            field(os.readlink(path).encode())
        elif stat.S_ISREG(status.st_mode):
            field(b'file')
            field(status.st_size.to_bytes(8, 'big'))
            with path.open('rb') as source:
                for chunk in iter(lambda: source.read(1024 * 1024), b''):
                    digest.update(chunk)
        else:
            raise RuntimeError(f'unsupported entry in measured dependency tree: {path}')
    return digest.hexdigest()


def verify_final_bindings(report, executables, artifact_roots, lock):
    """Reject any executable or dependency mutation during the complete run."""
    for name, executable in executables.items():
        final = measure.digest(executable)
        report['executables'][name]['final_sha256'] = final
        if final != report['executables'][name]['sha256']:
            raise RuntimeError(f'{name}: executable changed during measurement')
    for name, path in artifact_roots.items():
        final = tree_digest(path)
        report['artifact_trees'][name]['final_sha256'] = final
        if final != report['artifact_trees'][name]['sha256']:
            raise RuntimeError(f'{name}: dependency tree changed during measurement')
    final_lock = measure.digest(lock)
    report['comparison_package_lock_final_sha256'] = final_lock
    if final_lock != report['comparison_package_lock_sha256']:
        raise RuntimeError('comparison package lock changed during measurement')


def invoke(binary, argv, env, home, memory=False, native_sampler=None):
    with tempfile.TemporaryDirectory(prefix='sample-', dir=home) as directory:
        stdout, stderr = Path(directory) / 'stdout', Path(directory) / 'stderr'
        request = {'argv': [str(binary), *argv], 'env': env, 'cwd': str(home),
                   'stdout': str(stdout), 'stderr': str(stderr), 'timeout_seconds': 180}
        sampler = 'cli-process-tree-measure.py' if memory else 'schema-cache-process-measure.py'
        if memory and native_sampler:
            child = subprocess.run([str(native_sampler), str(stdout), str(stderr), str(home), '180',
                                    str(binary), *argv], env=env, stdin=subprocess.DEVNULL,
                                   text=True, capture_output=True, timeout=200)
        else:
            child = subprocess.run([sys.executable, str(HERE / sampler)], input=json.dumps(request),
                                   text=True, capture_output=True, timeout=200)
        if child.returncode:
            raise RuntimeError(f'sampler failed (exit {child.returncode}): {child.stderr}')
        result = json.loads(child.stdout)
        out, err = stdout.read_bytes(), stderr.read_bytes()
        if result['timed_out'] or result['returncode']:
            raise RuntimeError(f'{argv}: exit={result["returncode"]}, timeout={result["timed_out"]}: '
                               f'{out.decode(errors="replace")[:1000]} {err.decode(errors="replace")[:1000]}')
        memory_key = 'kernel_process_peak_rss_bytes' if native_sampler else 'sampled_tree_peak_rss_bytes'
        if memory and not result['measurement'][memory_key]:
            raise RuntimeError('memory sampler did not observe the command')
        return out, err, result['measurement']


def has_true(value, key):
    if isinstance(value, dict):
        return value.get(key) is True or any(has_true(item, key) for item in value.values())
    return isinstance(value, list) and any(has_true(item, key) for item in value)


def validate_output(workload, out):
    if not out.strip():
        raise RuntimeError(f'{workload}: empty successful output')
    if workload in ('schema', 'dry-run', 'mock', 'config'):
        parsed = json.loads(out)
        if not isinstance(parsed, (dict, list)) or not parsed:
            raise RuntimeError(f'{workload}: expected structured nonempty output')
        marker = {'dry-run': 'dry_run', 'mock': '_mock'}.get(workload)
        if marker and not has_true(parsed, marker):
            raise RuntimeError(f'{workload}: missing explicit {marker} marker')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--baseline', type=Path, required=True)
    parser.add_argument('--baseline-proof', type=Path, required=True)
    parser.add_argument('--tools-prefix', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--samples', type=int, default=30)
    args = parser.parse_args()
    if importlib.util.find_spec('psutil') is None:
        parser.error('psutil is required for public-wrapper process-tree memory measurements')
    if args.samples < 30:
        parser.error('at least 30 trials per case are required')
    report = {'complete': False, 'release_eligible': False, 'scope': __doc__,
              'platform': platform.platform(), 'machine': platform.machine(),
              'cpu_count': os.cpu_count(), 'load_start': os.getloadavg(),
              'node_version': subprocess.check_output(['node', '--version'], text=True).strip(),
              'samples_per_case_per_phase': args.samples, 'seed': 20260906,
              'telemetry': 'default; DO_NOT_TRACK absent; network not sandboxed',
              'cache': 'warm metadata; per-product private HOME, no real credentials',
              'memory': 'separate trials, sampled simultaneous process-tree RSS, 1 ms requested sleep; '
                        'public wrappers only, lower bound, shared pages counted, not PSS; native entries use '
                        'kernel wait4 peak RSS from a small C parent after exec, avoiding Python pre-exec RSS; '
                        'native cases are single-process workloads; no latency claims from memory trials',
              'business_scope': 'calendar list request preview only; DWS local config and built-in mock response; '
                                'no authenticated remote-service or persistent-event throughput claim',
              'cases': {}, 'failures': {}, 'raw_samples': {}, 'raw_memory': {}}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    try:
        binary = args.binary.resolve(strict=True)
        package = binary.parent
        build = json.loads((package.parent / 'candidate-build.json').read_text())
        if measure.digest(binary) != build['binary_sha256']:
            raise RuntimeError('candidate differs from finalized single-binary build record')
        baseline = args.baseline.resolve(strict=True)
        report['baseline_build'] = entry.validate_baseline(baseline, args.baseline_proof)
        report['release'] = {'version': build['version'], 'commit': build['source_commit'], 'edition': 'open'}
        modules = args.tools_prefix.resolve(strict=True) / 'node_modules'
        competitor_roots = {'lark': modules / '@larksuite/cli', 'gws': modules / '@googleworkspace/cli'}
        report['executables'] = {}
        with tempfile.TemporaryDirectory(prefix='.dws-five-dimensions-', dir=Path.home()) as directory:
            root = Path(directory)
            native_sampler = root / 'native-memory-sampler'
            subprocess.run(['cc', '-O2', '-std=c11', '-Wall', '-Wextra', '-Werror',
                            str(HERE / 'cli-native-memory-measure.c'), '-o', str(native_sampler)], check=True)
            report['native_memory_sampler_sha256'] = measure.digest(native_sampler)
            report['native_memory_sampler_source_sha256'] = measure.digest(HERE / 'cli-native-memory-measure.c')
            report['native_memory_compiler'] = subprocess.check_output(['cc', '--version'], text=True).splitlines()[0]
            # Use the repository's actual npm wrapper and byte-identical
            # single binary. Staging is outside the measured interval.
            npm = root / 'npm'
            (npm / 'bin').mkdir(parents=True)
            shutil.copy2(HERE.parents[1] / 'build/npm/bin/dws.js', npm / 'bin/dws.js')
            (npm / 'vendor').mkdir()
            shutil.copy2(binary, npm / 'vendor' / 'dws')
            executables = {'dws-native': binary, 'dws-public': npm / 'bin/dws.js',
                           'dws-baseline': baseline}
            for product, path in competitor_roots.items():
                metadata = json.loads((path / 'package.json').read_text())
                version = {'lark': '1.0.85', 'gws': '0.22.5'}[product]
                if metadata['version'] != version:
                    raise RuntimeError(f'{product}: expected pinned version {version}')
                name = 'lark-cli' if product == 'lark' else 'gws'
                executables[product + '-public'] = (modules / '.bin' / name).absolute()
                executables[product + '-native'] = path / 'bin' / name
            for name, executable in executables.items():
                report['executables'][name] = {'path': str(executable), 'sha256': measure.digest(executable),
                                              'version': build['version'] if name.startswith('dws') else
                                                         {'lark': '1.0.85', 'gws': '0.22.5'}[name.split('-')[0]]}
            report['executables']['dws-baseline']['version'] = report['baseline_build'].get('version', 'see baseline_build')
            artifact_roots = {'dws-candidate-package': package, 'dws-public-package': npm,
                              'comparison-install': args.tools_prefix.resolve(strict=True)}
            report['artifact_trees'] = {name: {'path': str(path), 'sha256': tree_digest(path)}
                                        for name, path in artifact_roots.items()}
            lock = args.tools_prefix.resolve(strict=True) / 'package-lock.json'
            report['comparison_package_lock_sha256'] = measure.digest(lock)
            argv = {
                'dws': {'schema': ['schema', 'calendar.list_calendars', '--compact', '-f', 'json'],
                        'help': ['--help'], 'version': ['--version'],
                        'leaf-help': ['calendar', 'book', 'list', '--help'],
                        'dry-run': ['calendar', 'book', 'list', '--dry-run', '-f', 'json'],
                        'config': ['config', 'list', '--json'],
                        'mock': ['calendar', 'book', 'list', '--mock', '-f', 'json']},
                'lark': {'schema': ['schema', 'calendar.calendars.list'],
                         'help': ['--help'], 'version': ['--version'],
                         'leaf-help': ['calendar', 'calendars', 'list', '--help'],
                         'dry-run': ['calendar', 'calendars', 'list', '--dry-run']},
                'gws': {'schema': ['schema', 'calendar.calendarList.list'],
                        'help': ['--help'], 'version': ['--version'],
                        'leaf-help': ['calendar', 'calendarList', 'list', '--help'],
                        'dry-run': ['calendar', 'calendarList', 'list', '--dry-run']}}
            cases = {}
            for name, executable in executables.items():
                product = name.split('-')[0]
                home = root / product
                home.mkdir(mode=0o700, exist_ok=True)
                cache = home / ('Library/Caches' if sys.platform == 'darwin' else '.cache')
                cache.mkdir(mode=0o700, parents=True, exist_ok=True)
                env = {'HOME': str(home), 'PATH': os.environ['PATH'], 'LANG': 'en', 'LC_ALL': 'C',
                       'NO_COLOR': '1', 'XDG_CONFIG_HOME': str(home / '.config'),
                       'XDG_CACHE_HOME': str(cache), 'DWS_CONFIG_DIR': str(home / '.dws')}
                for workload, arguments in argv[product].items():
                    case_env = dict(env)
                    if product == 'lark' and workload == 'dry-run':
                        # The official dry-run requires an account context;
                        # synthetic env-only credentials never access keychain.
                        case_env.update(LARKSUITE_CLI_APP_ID='cli_benchmark_fixture',
                                        LARKSUITE_CLI_TENANT_ACCESS_TOKEN='benchmark-placeholder-not-a-credential')
                    key = name + '/' + workload
                    cases[key] = (executable, arguments, case_env, home)
                    if name == 'dws-native' and workload in ('schema', 'dry-run', 'mock'):
                        cases['dws-live/' + workload] = (executable, arguments,
                            {**case_env, 'DWS_SCHEMA_CACHE_DISABLE': '1'}, home)
            if len(cases) != EXPECTED_CASE_COUNT:
                raise RuntimeError(f'fixed workload inventory changed: got {len(cases)}, want {EXPECTED_CASE_COUNT}')
            expected = {}
            # Every trial must retain this exact stdout AND stderr. Successful
            # dry-run previews legitimately use stderr; do not suppress it.
            for key, case in cases.items():
                report['cases'][key] = {'argv': [str(case[0]), *case[1]],
                                        'env': case[2], 'cache': 'warm',
                                        'memory_method': 'sampled_process_tree' if '-public/' in key else 'kernel_process_peak'}
                try:
                    out, err, _ = invoke(*case)
                    validate_output(key.split('/')[1], out)
                    expected[key] = (out, err)
                    report['cases'][key].update(stdout_bytes=len(out), stderr_bytes=len(err),
                        stdout_sha256=hashlib.sha256(out).hexdigest(), stderr_sha256=hashlib.sha256(err).hexdigest())
                except Exception as error:
                    report['failures'][key] = str(error)
                print('prepared ' + key, file=sys.stderr, flush=True)
            for workload in ('schema', 'dry-run', 'mock'):
                hit, live = 'dws-native/' + workload, 'dws-live/' + workload
                if hit in expected and live in expected and expected[hit] != expected[live]:
                    raise RuntimeError(f'{workload}: cache output differs from authoritative live path')
            for product, workloads in argv.items():
                for workload in workloads:
                    native, public = product + '-native/' + workload, product + '-public/' + workload
                    if native in expected and public in expected and expected[native] != expected[public]:
                        raise RuntimeError(f'{product}/{workload}: public wrapper changes output')
            for phase, field in ((False, 'raw_samples'), (True, 'raw_memory')):
                order = list(expected) * args.samples
                random.Random(report['seed'] + int(phase)).shuffle(order)
                report[field + '_order'] = order
                for key in expected:
                    report[field][key] = []
                for index, key in enumerate(order):
                    if key in report['failures']:
                        continue
                    try:
                        out, err, usage = invoke(*cases[key], memory=phase,
                            native_sampler=native_sampler if phase and '-public/' not in key else None)
                        if (out, err) != expected[key]:
                            raise RuntimeError('stdout/stderr drift after warmup')
                        report[field][key].append(usage)
                    except Exception as error:
                        report['failures'][key] = str(error)
                    if (index + 1) % 30 == 0:
                        print(f'{field}: {index + 1}/{len(order)}', file=sys.stderr, flush=True)
                report[field + '_summary'] = {key: measure.summarize(values)
                    for key, values in report[field].items() if values}
                args.output.write_text(json.dumps(report, indent=2) + '\n')
            verify_final_bindings(report, executables, artifact_roots, lock)
            report['complete'] = not report['failures'] and all(
                len(report[field].get(key, [])) == args.samples
                for field in ('raw_samples', 'raw_memory') for key in cases)
    except Exception as error:
        report['fatal_error'] = str(error)
    finally:
        report['load_end'] = os.getloadavg()
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, indent=2) + '\n')
    return 0 if report['complete'] else 1


if __name__ == '__main__':
    sys.exit(main())
