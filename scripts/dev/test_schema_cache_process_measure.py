#!/usr/bin/env python3

import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time
import unittest


@unittest.skipIf(os.name == 'nt' or shutil.which('node') is None,
                 'real npm-wrapper timeout proof requires POSIX and Node.js')
class ProcessTimeoutTests(unittest.TestCase):
    def test_timeout_stops_detached_vendor_process(self):
        with tempfile.TemporaryDirectory(prefix='dws-wrapper-timeout-') as directory:
            root = Path(directory)
            wrapper = root / 'npm' / 'bin' / 'dws.js'
            wrapper.parent.mkdir(parents=True)
            shutil.copy2(Path(__file__).resolve().parents[2] / 'build/npm/bin/dws.js', wrapper)
            package = root / 'npm' / 'vendor' / 'dws-v0.0.0-linux-amd64'
            binary = package / 'bin' / 'dws'
            binary.parent.mkdir(parents=True)
            pid_file = root / 'vendor.pid'
            binary.write_text('#!/bin/sh\nprintf "%s\\n" "$$" > "$PID_FILE"\nsleep 30\n')
            binary.chmod(0o755)
            stdout, stderr = root / 'stdout', root / 'stderr'
            request = {
                'argv': [str(wrapper), '--version'],
                'env': {**os.environ, 'PID_FILE': str(pid_file)},
                'cwd': str(root), 'stdout': str(stdout), 'stderr': str(stderr),
                'timeout_seconds': 5,
            }
            sampler = Path(__file__).with_name('schema-cache-process-measure.py')
            result = subprocess.run([sys.executable, str(sampler)], input=json.dumps(request),
                                    text=True, capture_output=True, timeout=10, check=True)
            report = json.loads(result.stdout)
            self.assertTrue(report['timed_out'])
            self.assertIsNone(report['measurement'])
            if not pid_file.exists():
                self.fail(f'vendor did not start before timeout: {stderr.read_text(errors="replace")}')
            pid = int(pid_file.read_text())
            for _ in range(100):
                try:
                    os.kill(pid, 0)
                except ProcessLookupError:
                    break
                time.sleep(.01)
            else:
                self.fail(f'detached vendor process {pid} survived sampler timeout')


if __name__ == '__main__':
    unittest.main()
