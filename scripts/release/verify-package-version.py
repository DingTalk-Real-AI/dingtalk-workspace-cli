#!/usr/bin/env python3
"""Verify finalized launcher/core identity and enabled native hot paths.

Run after archive/manifest/signature verification on the matching native host.
The core-free help and warmed Schema probes prove that release packaging sealed
the same capabilities exercised by native candidates.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import tempfile


def sha256(path):
    value = hashlib.sha256()
    with path.open('rb') as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b''):
            value.update(chunk)
    return value.hexdigest()


def invoke(binary, argv, env, home, output_limit=32 * 1024 * 1024):
    with tempfile.TemporaryFile() as stdout, tempfile.TemporaryFile() as stderr:
        result = subprocess.run([str(binary), *argv], cwd=home, env=env,
                                stdin=subprocess.DEVNULL, stdout=stdout, stderr=stderr,
                                timeout=180, close_fds=True)
        stdout.seek(0)
        output = stdout.read(output_limit + 1)
        stderr.seek(0)
        diagnostic = stderr.read(8192)
    if len(output) > output_limit:
        raise RuntimeError(f'{argv}: output exceeds {output_limit} bytes')
    if result.returncode or diagnostic:
        raise RuntimeError(f'{argv}: exit={result.returncode}, stderr={diagnostic!r}')
    return output


def verify(launcher, core, version, commit, build_time, report, verify_hot_paths=True):
    expected = f'dws version {version} ({commit}, {build_time})\n'.encode()
    originals = {'launcher': launcher, 'core': core}
    for path in originals.values():
        if not stat.S_ISREG(path.lstat().st_mode):
            raise RuntimeError(f'version verification requires a regular binary: {path}')
    digests = {name: sha256(path) for name, path in originals.items()}
    report.update(version=version, commit=commit, build_time=build_time, binaries=digests, runs=[])
    if verify_hot_paths:
        manifest = json.loads((launcher.parent.parent / 'package-manifest.json').read_text())
        expected_capabilities = {'schema_cache': 'enabled', 'root_help': 'enabled', 'disabled_reason': ''}
        if manifest.get('capabilities') != expected_capabilities:
            raise RuntimeError(f'enabled native package capability matrix mismatch: {manifest.get("capabilities")!r}')
        report['capabilities'] = manifest['capabilities']
    with tempfile.TemporaryDirectory(prefix='dws-package-version-') as temporary:
        home = Path(temporary).resolve()
        env = {name: os.environ[name] for name in ('SystemRoot', 'SYSTEMROOT', 'WINDIR') if name in os.environ}
        env.update(HOME=str(home), USERPROFILE=str(home), TMPDIR=str(home), TEMP=str(home), TMP=str(home),
                   XDG_CONFIG_HOME=str(home / 'config'), XDG_CACHE_HOME=str(home / 'cache'),
                   APPDATA=str(home / 'appdata'), LOCALAPPDATA=str(home / 'localappdata'),
                   DO_NOT_TRACK='1', PATH=os.defpath, LANG='C', LC_ALL='C')
        isolated = home / 'core-free' / 'bin' / launcher.name
        isolated.parent.mkdir(parents=True)
        shutil.copy2(launcher, isolated)
        if sha256(isolated) != digests['launcher']:
            raise RuntimeError('core-free copy differs from the finalized launcher')
        for name, binary in [('core', core), ('launcher', launcher), ('core-free-launcher', isolated)]:
            # Spool output to files so an erroneous --version implementation
            # cannot allocate an unbounded capture buffer in the verifier.
            with tempfile.TemporaryFile() as stdout, tempfile.TemporaryFile() as stderr:
                result = subprocess.run([str(binary), '--version'], cwd=home, env=env,
                                        stdin=subprocess.DEVNULL, stdout=stdout, stderr=stderr,
                                        timeout=30, close_fds=True)
                stdout.seek(0)
                actual = stdout.read(len(expected) + 1)
                stderr.seek(0)
                diagnostic = stderr.read(8192)
            entry = {'binary': name, 'exit_code': result.returncode, 'stdout_exact': actual == expected,
                     'stderr_empty': not diagnostic}
            report['runs'].append(entry)
            if result.returncode != 0 or actual != expected or diagnostic:
                raise RuntimeError(f'{name} version contract failed: exit={result.returncode}, '
                                   f'stdout={actual!r}, stderr={diagnostic!r}')
        if verify_hot_paths:
            report['hot_paths'] = {'help': {}, 'schema': {}}
            for locale in ('en', 'zh'):
                help_env = {**env, 'LANG': locale}
                expected_help = invoke(core, ['--help'], help_env, home)
                actual_help = invoke(isolated, ['--help'], help_env, home)
                if actual_help != expected_help:
                    raise RuntimeError(f'{locale}: core-free sealed help differs from finalized core')
                report['hot_paths']['help'][locale] = {
                    'stdout_sha256': hashlib.sha256(actual_help).hexdigest(),
                    'stdout_bytes': len(actual_help), 'core_free': True,
                }
            leaf = ['schema', 'calendar.create_calendar_event', '--compact', '-f', 'json']
            expected_schema = invoke(core, leaf, {**env, 'DWS_SCHEMA_CACHE_DISABLE': '1'}, home)
            warmed_schema = invoke(launcher, leaf, env, home)
            cached_schema = invoke(isolated, leaf, env, home)
            if warmed_schema != expected_schema or cached_schema != expected_schema:
                raise RuntimeError('finalized release Schema cache differs from authoritative core')
            report['hot_paths']['schema'] = {
                'stdout_sha256': hashlib.sha256(cached_schema).hexdigest(),
                'stdout_bytes': len(cached_schema), 'core_free_cache_hit': True,
            }
        for name, path in originals.items():
            if sha256(path) != digests[name]:
                raise RuntimeError(f'finalized {name} changed during version verification')
        if sha256(isolated) != digests['launcher']:
            raise RuntimeError('core-free launcher changed during verification')
    report['passed'] = True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--launcher', type=Path, required=True)
    parser.add_argument('--core', type=Path, required=True)
    parser.add_argument('--version', required=True)
    parser.add_argument('--commit', required=True)
    parser.add_argument('--build-time', required=True)
    parser.add_argument('--report', type=Path, required=True)
    args = parser.parse_args()
    if not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?', args.version):
        parser.error('version must be a v-prefixed semantic version')
    if not re.fullmatch(r'[0-9a-f]{40}', args.commit):
        parser.error('commit must be a full lowercase SHA')
    if not re.fullmatch(r'[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z', args.build_time):
        parser.error('build time must be the UTC RFC3339 committer date')
    report = {'scope': __doc__.strip(), 'passed': False}
    try:
        verify(args.launcher.absolute(), args.core.absolute(), args.version, args.commit, args.build_time, report)
    except Exception as error:
        report['error'] = str(error)
        raise
    finally:
        args.report.write_text(json.dumps(report, indent=2) + '\n')


if __name__ == '__main__':
    main()
