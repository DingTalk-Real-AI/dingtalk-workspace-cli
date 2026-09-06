#!/usr/bin/env python3
"""Check native identity generation with isolated files and no host network.

Development evidence only: wall-clock independence and final signed-artifact
proof remain separate requirements. This report never enables release caches.
"""

import argparse
import csv
import io
import os
import re
import uuid
import hashlib
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile
import threading


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


class Sandbox:
    def __init__(self, generator, fixture, home):
        self.generator, self.fixture, self.home = generator, fixture, home
        self.system = platform.system()
        self.cat = Path('/bin/cat' if self.system == 'Darwin' else '/usr/bin/cat')
        self.curl = Path('/usr/bin/curl')
        self.executables = list(dict.fromkeys([generator, self.cat, self.curl]))
        if self.system == 'Darwin':
            literal = lambda path: '(literal ' + json.dumps(str(path), ensure_ascii=False) + ')'
            # dyld-support only supplies OS loader rules. Do not import system.sb:
            # it also grants service lookup and other ambient capabilities.
            self.profile = '\n'.join([
                '(version 1)', '(deny default)', '(import "dyld-support.sb")',
                '(allow sysctl-read)', '(allow process-info* (target self))',
                '(allow signal (target self))',
                '(allow file-write-data (subpath "/dev/fd") (literal "/dev/null"))',
                '(allow file-read* file-map-executable (subpath "/System/Library") (subpath "/usr/lib"))',
                '(allow file-read* (literal "/dev/urandom") (literal "/dev/random") (literal "/dev/null"))',
                # Apple's curl initializes LibreSSL even for --version. This is
                # one OS configuration file, not the user config/credential tree.
                '(allow file-read* (literal "/private/etc/ssl/openssl.cnf"))',
                '(allow file-read* (subpath ' + json.dumps(str(fixture), ensure_ascii=False) + ') (subpath ' + json.dumps(str(home), ensure_ascii=False) + '))',
                '(allow file-read* file-map-executable ' + ' '.join(map(literal, self.executables)) + ')',
                '(allow process-exec ' + ' '.join(map(literal, self.executables)) + ')',
            ])
            self.prefix = ['/usr/bin/sandbox-exec', '-p', self.profile]
        elif self.system == 'Linux':
            docker = shutil.which('docker')
            if not docker:
                raise RuntimeError('Docker is required; no unrestricted fallback is allowed')
            self.image = os.environ.get('DWS_SCHEMA_PROOF_IMAGE', '')
            if not re.fullmatch(r'sha256:[0-9a-f]{64}', self.image):
                raise RuntimeError('an exact locally imported empty image ID is required')
            if os.getuid() == 0:
                raise RuntimeError('the proof generator must run as the unprivileged native runner user')
            self.docker = [docker, '--host', 'unix:///var/run/docker.sock']
            server = subprocess.check_output(self.docker + ['info', '--format', '{{.OSType}}/{{.Architecture}}'],
                                             text=True, timeout=30).strip()
            if server not in ('linux/amd64', 'linux/x86_64'):
                raise RuntimeError('Docker must use the local native Linux amd64 daemon')
            details = json.loads(subprocess.check_output(self.docker + ['image', 'inspect', self.image], timeout=30))[0]
            config = details.get('Config') or {}
            if details['Id'] != self.image or any(config.get(key) for key in ('Env', 'Entrypoint', 'Cmd', 'Volumes')):
                raise RuntimeError('proof image must have no inherited environment, command or volumes')
            rootfs = details.get('RootFS') or {}
            empty_layer = 'sha256:' + hashlib.sha256(bytes(10240)).hexdigest()
            if rootfs.get('Type') != 'layers' or rootfs.get('Layers') != [empty_layer]:
                raise RuntimeError('proof image rootfs must be the exact empty tar imported by the workflow')
            self.image_metadata = {'id': self.image, 'rootfs': rootfs, 'server': server}
            self.prefix = self.docker + ['run', '--rm', '--pull=never', '--network=none', '--read-only',
                '--cap-drop=ALL', '--security-opt=no-new-privileges', '--pids-limit=128',
                '--user', f'{os.getuid()}:{os.getgid()}', '--hostname', 'schema-proof']
            # An empty imported rootfs contains no host HOME or credentials.
            # Bind only the exact native executables, libraries and fixtures;
            # the same cat/curl bytes are used by outside and inside controls.
            paths = [Path(p) for p in ('/usr/lib', '/lib', '/lib64') if Path(p).exists()]
            paths += self.executables + [fixture, home]
            for path in paths:
                value = io.StringIO()
                csv.writer(value, lineterminator='').writerow(['type=bind', 'source=' + str(path),
                                                             'target=' + str(path), 'readonly'])
                self.prefix += ['--mount', value.getvalue()]

        else:
            raise RuntimeError('identity environment checks require native Linux or macOS')

    def run(self, command, env, timeout=120):
        prefix = self.prefix.copy()
        if self.system == 'Linux':
            name = 'dws-schema-proof-' + uuid.uuid4().hex
            prefix += ['--name', name]
            for key, value in sorted(env.items()):
                prefix += ['--env', key + '=' + value]
            prefix += ['--workdir', str(self.home), '--entrypoint', str(command[0]), self.image]
            try:
                return subprocess.run(prefix + list(map(str, command[1:])), cwd=self.home, env=env,
                                      stdin=subprocess.DEVNULL, capture_output=True, timeout=timeout,
                                      close_fds=True)
            finally:
                # A timeout kills the client, not necessarily its container.
                # Remove only this invocation's unique container, even on error.
                subprocess.run(self.docker + ['rm', '--force', name], env=env,
                               stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                               stderr=subprocess.DEVNULL, timeout=30, check=False)
        return subprocess.run(prefix + list(map(str, command)), cwd=self.home, env=env,
                              stdin=subprocess.DEVNULL, capture_output=True, timeout=timeout,
                              close_fds=True)


def environment(home, hostile=False):
    result = {'HOME': str(home), 'TMPDIR': str(home), 'XDG_CONFIG_HOME': str(home / 'config'),
              'XDG_CACHE_HOME': str(home / 'cache'), 'DO_NOT_TRACK': '1', 'LANG': 'C',
              'LC_ALL': 'C', 'TZ': 'UTC', 'PATH': '/usr/bin:/bin'}
    if hostile:
        result.update(PATH='/nonexistent-schema-proof-path', LANG='tr_TR.UTF-8', LC_ALL='tr_TR.UTF-8',
                      TZ='Pacific/Honolulu', HTTP_PROXY='http://127.0.0.1:1',
                      HTTPS_PROXY='http://127.0.0.1:1', ALL_PROXY='http://127.0.0.1:1',
                      DWS_PROFILE='schema-proof-hostile', DWS_CONFIG_DIR=str(home / 'unconfigured'),
                      DWS_SCHEMA_CACHE_DISABLE='1', DWS_AGENT_HOST='invalid-proof-host',
                      DWS_AGENT_PRODUCT='invalid-proof-product', SOURCE_DATE_EPOCH='1')
    return result


def check_controls(sandbox, env, forbidden):
    readable = sandbox.fixture / 'read-control.txt'
    positive = sandbox.run([sandbox.cat, readable], env)
    if positive.returncode != 0 or positive.stdout != readable.read_bytes():
        raise RuntimeError(f'sandbox cannot read its allowed fixture: {positive.returncode}, {positive.stderr!r}')
    # The same executable must succeed outside the sandbox on our harmless
    # sentinel, then fail inside. A broken executable is not isolation evidence.
    outside = subprocess.run([sandbox.cat, forbidden], env=env, capture_output=True, check=True)
    inside = sandbox.run([sandbox.cat, forbidden], env)
    if not outside.stdout or inside.returncode == 0 or outside.stdout in inside.stdout:
        raise RuntimeError('sandbox exposed a non-fixture user file')
    version = sandbox.run([sandbox.curl, '-q', '--version'], env)
    if version.returncode != 0 or not version.stdout.startswith(b'curl '):
        raise RuntimeError(f'sandbox curl control cannot execute: {version.returncode}, {version.stderr!r}')

    class Handler(BaseHTTPRequestHandler):
        timeout = 3

        def setup(self):
            self.server.accepted_connections += 1
            super().setup()

        def do_GET(self):
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b'schema-proof-host-network-control')

        def log_message(self, *_):
            pass

    server = HTTPServer(('127.0.0.1', 0), Handler)
    server.accepted_connections = 0
    worker = threading.Thread(target=server.serve_forever, daemon=True)
    worker.start()
    try:
        command = [sandbox.curl, '-q', '--silent', '--show-error', '--noproxy', '*',
                   '--connect-timeout', '2', '--max-time', '3', f'http://127.0.0.1:{server.server_port}/']
        reachable = subprocess.run(command, env=env, capture_output=True, check=True, timeout=5)
        blocked = sandbox.run(command, env, timeout=5)
        if (reachable.stdout != b'schema-proof-host-network-control' or blocked.returncode == 0
                or reachable.stdout in blocked.stdout or server.accepted_connections != 1):
            raise RuntimeError('sandbox can access the host network')
    finally:
        server.shutdown()
        server.server_close()
        worker.join()
    return {'allowed_fixture_read': True, 'non_fixture_read_blocked': True, 'host_network_blocked': True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--generator', type=Path, required=True)
    parser.add_argument('--root', type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument('--expected', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    generator = args.generator.resolve(strict=True)
    expected = args.expected.read_bytes()
    expected_identity = json.loads(expected)
    if expected_identity.get('edition') != 'open' or expected_identity.get('go_runtime_version') != 'go1.25.9':
        parser.error('expected identity must describe the open edition with Go 1.25.9')
    if (platform.system(), platform.machine()) not in (('Darwin', 'arm64'), ('Linux', 'x86_64')):
        parser.error('identity checks require native darwin/arm64 or linux/amd64')
    report = {'scope': __doc__.strip(), 'platform': platform.platform(),
              'source_commit': subprocess.check_output(['git', '-C', str(args.root), 'rev-parse', 'HEAD'], text=True).strip(),
              'source_dirty': bool(subprocess.check_output(['git', '-C', str(args.root), 'status', '--porcelain'])),
              'generator_sha256': digest(generator), 'identity_sha256': hashlib.sha256(expected).hexdigest(),
              'wall_clock_independence_proven': False, 'forbidden_access_attempts_audited': False,
              'final_artifact_proven': False,
              'release_eligible': False, 'passed': False, 'runs': []}
    if platform.system() == 'Darwin':
        report['os_policy_inputs_sha256'] = {
            name: digest(Path(name)) if Path(name).exists() else None for name in (
                '/System/Library/Sandbox/Profiles/dyld-support.sb', '/private/etc/ssl/openssl.cnf')}
    try:
        with tempfile.TemporaryDirectory(prefix='dws-identity-environment-') as temporary:
            work = Path(temporary).resolve()
            fixture = work / 'fixture'
            protobuf = fixture / 'internal/cli/schemacachepb'
            protobuf.mkdir(parents=True)
            for name in ('schema_cache.proto', 'schema_cache.pb.go'):
                shutil.copyfile(args.root / 'internal/cli/schemacachepb' / name, protobuf / name)
            (fixture / 'read-control.txt').write_text('readable fixture\n')
            forbidden = work / 'non-fixture-user-file.txt'
            forbidden.write_text('isolated harmless user-file sentinel\n')
            before = {str(p.relative_to(fixture)): digest(p) for p in fixture.rglob('*') if p.is_file()}
            for name, hostile in [('clean-first', False), ('clean-repeat', False), ('hostile', True)]:
                home = work / name
                home.mkdir(mode=0o700)
                sandbox = Sandbox(generator, fixture, home)
                env = environment(home, hostile)
                if hasattr(sandbox, 'image_metadata'):
                    report['linux_image'] = sandbox.image_metadata
                controls = check_controls(sandbox, env, forbidden)
                entry = {'name': name, 'controls': controls, 'identity_byte_equal': False,
                         'sandbox_argv': sandbox.prefix, 'environment': env,
                         'sandbox_argv_sha256': hashlib.sha256(json.dumps(sandbox.prefix).encode()).hexdigest()}
                report['runs'].append(entry)
                result = sandbox.run([generator, '-root', fixture], env)
                entry['exit_code'] = result.returncode
                entry['generator_argv'] = result.args
                if result.returncode != 0 or result.stdout != expected:
                    raise RuntimeError(f'{name}: identity differs or generator failed: {result.returncode}, {result.stderr!r}')
                if list(home.rglob('*')):
                    raise RuntimeError(f'{name}: generator populated empty HOME/config')
                entry['identity_byte_equal'] = True
            after = {str(p.relative_to(fixture)): digest(p) for p in fixture.rglob('*') if p.is_file()}
            if before != after or digest(generator) != report['generator_sha256']:
                raise RuntimeError('proof executable or fixture changed during verification')
            report['fixture_sha256'] = before
            report['passed'] = True
    except Exception as error:
        report['error'] = str(error)
        raise
    finally:
        args.output.write_text(json.dumps(report, indent=2) + '\n')


if __name__ == '__main__':
    main()
