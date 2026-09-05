#!/usr/bin/env python3

import importlib.util
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

path = Path(__file__).with_name('record-go-test-events.py')
spec = importlib.util.spec_from_file_location('record_go_events', path)
recorder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(recorder)


class RecordGoEventsTests(unittest.TestCase):
    def test_repeated_output_is_retained_without_console_flood(self):
        events = [{'Action': 'run', 'Package': 'example/pkg', 'Test': 'TestExample'}]
        events += [{'Action': 'output', 'Package': 'example/pkg', 'Output': 'large repeated payload\n'}] * 10000
        events += [{'Action': 'output', 'Package': 'example/pkg', 'Output': 'specific assertion failed\n'},
                   {'Action': 'fail', 'Package': 'example/pkg', 'Test': 'TestExample'},
                   {'Action': 'fail', 'Package': 'example/pkg', 'Elapsed': 3}]
        source = b''.join(json.dumps(event).encode() + b'\n' for event in events)
        raw, console = io.BytesIO(), io.StringIO()
        recorder.record(io.BytesIO(source), raw, console)
        self.assertEqual(raw.getvalue(), source)
        self.assertIn('[FAIL] example/pkg TestExample', console.getvalue())
        self.assertIn('specific assertion failed', console.getvalue())
        self.assertLess(len(console.getvalue()), 2048)

    def test_compiler_diagnostics_survive_non_json_input(self):
        source = b'# example/pkg\nfile.go:2: undefined: missing\n'
        raw, console = io.BytesIO(), io.StringIO()
        recorder.record(io.BytesIO(source), raw, console)
        self.assertEqual(raw.getvalue(), source)
        self.assertIn('undefined: missing', console.getvalue())

    def test_go_125_build_events_remain_visible(self):
        source = b''.join(json.dumps(event).encode() + b'\n' for event in [
            {'Action': 'build-output', 'ImportPath': 'example/pkg', 'Output': 'file.go:2: undefined: missing\n'},
            {'Action': 'build-fail', 'ImportPath': 'example/pkg'}])
        raw, console = io.BytesIO(), io.StringIO()
        recorder.record(io.BytesIO(source), raw, console)
        self.assertEqual(raw.getvalue(), source)
        self.assertIn('undefined: missing', console.getvalue())
        self.assertIn('[BUILD FAIL] example/pkg', console.getvalue())

    def test_pipefail_preserves_upstream_failure(self):
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / 'raw.jsonl'
            # Positional arguments avoid shell interpolation of paths.
            result = subprocess.run(['bash', '-c',
                'set -o pipefail; (printf "compiler failed\\n"; exit 7) | "$1" "$2" --output "$3"',
                'recorder-test', sys.executable, str(path), str(output)], capture_output=True)
            self.assertEqual(result.returncode, 7, result.stderr)
            self.assertEqual(output.read_bytes(), b'compiler failed\n')


if __name__ == '__main__':
    unittest.main()
