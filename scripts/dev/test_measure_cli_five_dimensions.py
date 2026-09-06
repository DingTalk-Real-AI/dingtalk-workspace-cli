#!/usr/bin/env python3

import importlib.util
import os
from pathlib import Path
import sys
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('five',
    Path(__file__).with_name('measure-cli-five-dimensions.py'))
five = importlib.util.module_from_spec(spec)
spec.loader.exec_module(five)


class MeasurementTests(unittest.TestCase):
    def test_real_child_preserves_both_output_streams_and_default_environment(self):
        with tempfile.TemporaryDirectory() as directory:
            env = {'PATH': os.environ['PATH']}
            script = 'import os,sys; assert "DO_NOT_TRACK" not in os.environ; print("ok"); print("preview",file=sys.stderr)'
            out, err, usage = five.invoke(sys.executable, ['-c', script], env, Path(directory))
            self.assertEqual(out, b'ok\n')
            self.assertEqual(err, b'preview\n')
            self.assertGreater(usage['wall_ms'], 0)

    def test_error_exit_is_not_a_fast_success(self):
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaisesRegex(RuntimeError, 'exit=3'):
                five.invoke(sys.executable, ['-c', 'raise SystemExit(3)'], {}, Path(directory))

    def test_memory_includes_live_wrapper_and_native_child(self):
        with tempfile.TemporaryDirectory() as directory:
            script = ('import subprocess,sys,time; x=bytearray(8*1024*1024); '
                      'p=subprocess.Popen([sys.executable,"-c",'
                      '"import time; x=bytearray(32*1024*1024); time.sleep(.3)"]); p.wait(); print("ok")')
            out, err, usage = five.invoke(sys.executable, ['-c', script], {}, Path(directory), memory=True)
            self.assertEqual((out, err), (b'ok\n', b''))
            self.assertGreaterEqual(usage['max_simultaneous_processes'], 2)
            self.assertGreater(usage['sampled_tree_peak_rss_bytes'], 40 * 1024 * 1024)
            self.assertNotIn('wall_ms', usage)

    def test_preview_and_mock_require_explicit_success_markers(self):
        for workload in ('dry-run', 'mock'):
            with self.assertRaisesRegex(RuntimeError, 'missing explicit'):
                five.validate_output(workload, b'{"error":"not configured"}')
        five.validate_output('dry-run', b'{"result":{"dry_run":true}}')
        five.validate_output('mock', b'{"content":{"_mock":true}}')


if __name__ == '__main__':
    unittest.main()
