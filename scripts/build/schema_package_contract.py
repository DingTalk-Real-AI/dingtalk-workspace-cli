#!/usr/bin/env python3
"""Shared enabled-target identity and root-help sealing contract."""

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


FIELDS = {
    'schemaCacheEdition': 'edition',
    'schemaCacheSourceSHA256': 'source_sha256',
    'schemaCacheSurfaceSHA256': 'surface_sha256',
    'schemaCacheBuildID': 'build_id',
    'schemaCacheMetaLength': 'meta_length',
    'schemaCacheMetaSHA256': 'meta_sha256',
    'schemaCacheRegistryLength': 'registry_length',
    'schemaCacheRegistrySHA256': 'registry_sha256',
}


def sha256(path):
    value = hashlib.sha256()
    with Path(path).open('rb') as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b''):
            value.update(chunk)
    return value.hexdigest()


def validate_identity(proof):
    for key in FIELDS.values():
        value = proof.get(key)
        if not isinstance(value, (str, int)) or not str(value) or any(character.isspace() for character in str(value)):
            raise RuntimeError(f'invalid Schema identity field: {key}')
    if proof['edition'] != 'open' or proof.get('go_runtime_version') != 'go1.25.9':
        raise RuntimeError('Schema identity must use open edition and Go 1.25.9')
    for key in ('source_sha256', 'surface_sha256', 'build_id', 'meta_sha256', 'registry_sha256'):
        if not re.fullmatch(r'[0-9a-f]{64}', str(proof[key])):
            raise RuntimeError(f'invalid Schema identity digest: {key}')
    for key in ('meta_length', 'registry_length'):
        if not str(proof[key]).isdigit() or int(proof[key]) <= 0:
            raise RuntimeError(f'invalid Schema identity length: {key}')
    return proof


def core_ldflags(proof, version, commit, build_time):
    validate_identity(proof)
    package = 'github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app'
    flags = ['-s', '-w', '-X', f'{package}.version={version}',
             '-X', f'{package}.gitCommit={commit}', '-X', f'{package}.buildTime={build_time}']
    for field, key in FIELDS.items():
        flags += ['-X', f'{package}.{field}={proof[key]}']
    return ' '.join(flags)


def launcher_ldflags(proof, version, commit, build_time, core_digest, core_size, help_snapshot):
    validate_identity(proof)
    values = [version, commit, build_time, core_digest, str(core_size), help_snapshot]
    if any(not value or any(character.isspace() for character in value) for value in values):
        raise RuntimeError('launcher identity values must be nonempty and contain no whitespace')
    if (not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?', version)
            or not re.fullmatch(r'[0-9a-f]{40}', commit)
            or not re.fullmatch(r'[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z', build_time)
            or not re.fullmatch(r'[0-9a-f]{64}', core_digest)
            or not str(core_size).isdigit() or int(core_size) <= 0
            or not re.fullmatch(r'[A-Za-z0-9+/]+', help_snapshot)):
        raise RuntimeError('invalid launcher release binding')
    flags = (f'-s -w -X main.version={version} -X main.commit={commit} '
             f'-X main.buildTime={build_time} -X main.edition=open '
             f'-X main.coreSHA256={core_digest} -X main.coreSize={core_size} '
             f'-X main.helpSnapshot={help_snapshot}')
    for field, key in FIELDS.items():
        flags += f' -X main.{field}={proof[key]}'
    return flags


def failure_output(data):
    limit = 1024 * 1024
    return {'bytes': len(data), 'sha256': hashlib.sha256(data).hexdigest(),
            'base64': base64.b64encode(data[:limit]).decode(), 'truncated': len(data) > limit}


def seal_root_help(core, help_generator, core_digest, commit, proof_path, compare_core=True):
    proof_path = Path(proof_path)
    proof = {'passed': False, 'release_eligible': False, 'core_sha256': core_digest,
             'source_commit': commit, 'generator_sha256': sha256(help_generator), 'locales': {}}
    try:
        with tempfile.TemporaryDirectory(prefix='.dws-help-proof-', dir=Path.home()) as directory:
            home = Path(directory)
            env = {'PATH': os.environ.get('PATH', '/usr/bin:/bin'), 'HOME': str(home),
                   'DWS_CONFIG_DIR': str(home / '.dws'), 'DO_NOT_TRACK': '1',
                   'NO_COLOR': '1', 'LANG': 'en', 'LC_ALL': 'C',
                   'GOMEMLIMIT': '1GiB', 'GOGC': '50'}
            generated = subprocess.run([str(help_generator), '-commit', commit, '-core-sha256', core_digest],
                                       env=env, cwd=home, capture_output=True, timeout=30, check=True)
            if generated.stderr:
                raise RuntimeError('help generator emitted diagnostics')
            projection = json.loads(generated.stdout)
            for locale in ('en', 'zh'):
                expected = base64.b64decode(projection['References'][locale], validate=True)
                proof['locales'][locale] = {
                    'stdout_sha256': hashlib.sha256(expected).hexdigest(), 'stdout_bytes': len(expected),
                }
                if compare_core:
                    result = subprocess.run([str(core), '--help'], env={**env, 'LANG': locale}, cwd=home,
                                            capture_output=True, timeout=30)
                    equal = result.returncode == 0 and not result.stderr and result.stdout == expected
                    proof['locales'][locale].update(
                        actual_stdout_sha256=hashlib.sha256(result.stdout).hexdigest(),
                        returncode=result.returncode, equal=bool(equal))
                    if not equal:
                        proof['locales'][locale].update(expected_stdout=failure_output(expected),
                                                        actual_stdout=failure_output(result.stdout),
                                                        stderr=failure_output(result.stderr))
                        raise RuntimeError(f'{locale} help projection differs from finalized core')
                else:
                    proof['locales'][locale]['native_comparison'] = 'deferred to final-artifact native runner'
            if sha256(core) != core_digest:
                raise RuntimeError('core changed during help projection proof')
            snapshot = projection['Snapshot']
            if not isinstance(snapshot, str) or not snapshot or any(character.isspace() for character in snapshot):
                raise RuntimeError('help generator emitted an invalid snapshot')
            proof.update(
                passed=compare_core,
                native_core_compared=compare_core,
                release_eligible=compare_core,
                status='passed' if compare_core else 'pending_native_comparison',
                snapshot_sha256=hashlib.sha256(snapshot.encode()).hexdigest(),
            )
            return snapshot
    except Exception as error:
        proof['error'] = str(error)
        if isinstance(error, (subprocess.CalledProcessError, subprocess.TimeoutExpired)):
            proof['failed_process'] = {
                'timed_out': isinstance(error, subprocess.TimeoutExpired),
                'returncode': getattr(error, 'returncode', None),
                'stdout': failure_output(error.stdout or b''), 'stderr': failure_output(error.stderr or b''),
            }
        raise
    finally:
        proof_path.parent.mkdir(parents=True, exist_ok=True)
        proof_path.write_text(json.dumps(proof, indent=2) + '\n')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest='command', required=True)
    ldflags = subparsers.add_parser('ldflags')
    ldflags.add_argument('--scope', choices=('core', 'launcher'), required=True)
    ldflags.add_argument('--identity', type=Path, required=True)
    ldflags.add_argument('--version', required=True)
    ldflags.add_argument('--commit', required=True)
    ldflags.add_argument('--build-time', required=True)
    ldflags.add_argument('--core-sha256')
    ldflags.add_argument('--core-size')
    ldflags.add_argument('--help-snapshot')
    seal = subparsers.add_parser('seal-help')
    seal.add_argument('--core', type=Path, required=True)
    seal.add_argument('--generator', type=Path, required=True)
    seal.add_argument('--core-sha256', required=True)
    seal.add_argument('--commit', required=True)
    seal.add_argument('--proof', type=Path, required=True)
    seal.add_argument('--snapshot-output', type=Path, required=True)
    seal.add_argument('--defer-native-comparison', action='store_true')
    args = parser.parse_args()
    if args.command == 'seal-help':
        snapshot = seal_root_help(args.core, args.generator, args.core_sha256, args.commit, args.proof,
                                  compare_core=not args.defer_native_comparison)
        args.snapshot_output.write_text(snapshot)
        return
    proof = validate_identity(json.loads(args.identity.read_text()))
    if args.scope == 'core':
        print(core_ldflags(proof, args.version, args.commit, args.build_time))
        return
    if not args.core_sha256 or not args.core_size or not args.help_snapshot:
        parser.error('launcher ldflags require core SHA-256, size, and help snapshot')
    print(launcher_ldflags(proof, args.version, args.commit, args.build_time,
                           args.core_sha256, args.core_size, args.help_snapshot))


if __name__ == '__main__':
    main()
