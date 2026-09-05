#!/usr/bin/env python3
"""Verify exact finalized launcher/core version behavior without modifying them.

Run after archive/manifest/signature verification on the matching native host.
This is version-contract evidence, not complete Schema release enablement proof.
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


def verify(launcher, core, version, commit, build_time, report):
    expected = f'dws version {version} ({commit}, {build_time})\n'.encode()
    originals = {'launcher': launcher, 'core': core}
    for path in originals.values():
        if not stat.S_ISREG(path.lstat().st_mode):
            raise RuntimeError(f'version verification requires a regular binary: {path}')
    digests = {name: sha256(path) for name, path in originals.items()}
    report.update(version=version, commit=commit, build_time=build_time, binaries=digests, runs=[])
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
