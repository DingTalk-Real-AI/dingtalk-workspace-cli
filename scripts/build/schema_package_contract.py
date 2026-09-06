#!/usr/bin/env python3
"""Validate a Schema identity proof and emit single-binary linker flags."""

import argparse
import json
from pathlib import Path
import re


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


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest='command', required=True)
    ldflags = subparsers.add_parser('ldflags')
    ldflags.add_argument('--scope', choices=('core',), required=True)
    ldflags.add_argument('--identity', type=Path, required=True)
    ldflags.add_argument('--version', required=True)
    ldflags.add_argument('--commit', required=True)
    ldflags.add_argument('--build-time', required=True)
    args = parser.parse_args()
    proof = validate_identity(json.loads(args.identity.read_text()))
    print(core_ldflags(proof, args.version, args.commit, args.build_time))


if __name__ == '__main__':
    main()
