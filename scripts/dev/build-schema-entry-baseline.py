#!/usr/bin/env python3
"""Build the immutable PR-base main entry on the native host.

The old source and runtime payload come exclusively from the pinned Git tree.
Only version metadata and release build flags are supplied by this driver.
This produces development evidence, not a signed release.
"""

import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import subprocess
import tarfile

BASE_COMMIT = '6f71222b9b07c760cdb5f376b24dab9155e62094'


def digest(path):
    value = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            value.update(chunk)
    return value.hexdigest()


def extract_source(archive_path, source):
    # This particular sealed tree contains only directories and regular files.
    # Refuse links and special entries before extracting any archive member.
    with tarfile.open(archive_path) as archive:
        for member in archive.getmembers():
            path = PurePosixPath(member.name)
            if path.is_absolute() or '..' in path.parts or not (member.isdir() or member.isfile()):
                raise RuntimeError('baseline archive contains a non-regular or escaping source path')
        archive.extractall(source)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True, help='new output directory')
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[2]
    env = {**os.environ, 'GOTOOLCHAIN': 'go1.25.9', 'CGO_ENABLED': '0', 'GOFLAGS': '',
           'GOWORK': 'off', 'GOEXPERIMENT': '', 'GOAMD64': 'v1', 'GOARM64': 'v8.0'}

    def run(command, cwd=root, capture=False):
        return subprocess.run(command, cwd=cwd, env=env, check=True, text=True,
                              stdout=subprocess.PIPE if capture else None).stdout

    host = json.loads(run(['go', 'env', '-json', 'GOHOSTOS', 'GOHOSTARCH', 'GOVERSION'], capture=True))
    goos, goarch = host['GOHOSTOS'], host['GOHOSTARCH']
    if host['GOVERSION'] != 'go1.25.9' or (goos, goarch) not in (('darwin', 'arm64'), ('linux', 'amd64')):
        parser.error('requires Go 1.25.9 on native darwin/arm64 or linux/amd64')
    env.update(GOOS=goos, GOARCH=goarch)
    commit = run(['git', 'rev-parse', BASE_COMMIT + '^{commit}'], capture=True).strip()
    if commit != BASE_COMMIT:
        raise RuntimeError('baseline must be the exact PR-base main commit')
    tree = run(['git', 'rev-parse', BASE_COMMIT + '^{tree}'], capture=True).strip()
    stamp = int(run(['git', 'show', '-s', '--format=%ct', commit], capture=True))
    build_time = datetime.datetime.fromtimestamp(stamp, datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    archive = output / 'source.tar'
    run(['git', 'archive', '--format=tar', '--output', str(archive), commit])
    source = output / 'source'
    source.mkdir()
    extract_source(archive, source)
    binary = output / 'dws-baseline'
    package = 'github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app'
    version = 'v0.0.0-schema-cache-candidate'
    flags = f'-s -w -X {package}.version={version} -X {package}.gitCommit={commit} -X {package}.buildTime={build_time}'
    run(['go', 'build', '-trimpath', '-buildmode=pie', '-ldflags', flags, '-o', str(binary), './cmd'], cwd=source)
    staging = output / 'runtime-staging'
    run(['sh', 'scripts/build/prepare-runtime-payload.sh', goos, goarch, str(staging)], cwd=source)
    payload = staging / '.dws-runtime/20260825'
    if goos == 'darwin':
        path = payload / 'manifest.json'
        manifest = json.loads(path.read_text())
        library = payload / manifest['library']
        run(['codesign', '--force', '--sign', '-', str(library)])
        run(['codesign', '--verify', '--strict', str(library)])
        manifest['library_sha256'] = digest(library)
        path.write_text(json.dumps(manifest, indent=2) + '\n')
    run(['go', 'run', './scripts/build/runtime-payload', 'inject', str(binary), str(payload)], cwd=source)
    if goos == 'darwin':
        run(['codesign', '--force', '--sign', '-', str(binary)])
        run(['codesign', '--verify', '--strict', str(binary)])
    report = {'scope': __doc__.strip(), 'source_commit': commit, 'source_tree': tree,
              'source_archive_sha256': digest(archive), 'binary_sha256': digest(binary),
              'go_version': host['GOVERSION'], 'goos': goos, 'goarch': goarch,
              'cgo_enabled': '0', 'goexperiment': '', 'gowork': 'off', 'build_tags': [],
              'build_flags': ['-trimpath', '-buildmode=pie'], 'ldflags': flags,
              'version': version, 'build_time': build_time,
              'runtime_manifest_sha256': digest(payload / 'manifest.json'),
              'signing': 'ad-hoc' if goos == 'darwin' else 'none', 'release_eligible': False}
    (output / 'baseline-build.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({'binary': str(binary), 'report': str(output / 'baseline-build.json')}))


if __name__ == '__main__':
    main()
