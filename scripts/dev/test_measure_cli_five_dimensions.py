#!/usr/bin/env python3

import importlib.util
import os
from pathlib import Path
import sys
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('five',
    Path(__file__).with_name('measure-cli-five-dimensions.py'))
five = importlib.util.module_from_spec(spec)
spec.loader.exec_module(five)


class MeasurementTests(unittest.TestCase):
    def test_tree_digest_binds_files_modes_and_symlinks(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'file').write_text('first')
            (root / 'link').symlink_to('file')
            before = five.tree_digest(root)
            (root / 'file').write_text('other')
            self.assertNotEqual(before, five.tree_digest(root))
            (root / 'file').write_text('first')
            self.assertEqual(before, five.tree_digest(root))

    def test_final_binding_rejects_dependency_mutation(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            executable = root / 'entry'
            executable.write_text('entry')
            dependency = root / 'package'
            dependency.mkdir()
            (dependency / 'data').write_text('before')
            lock = root / 'package-lock.json'
            lock.write_text('{}')
            report = {
                'executables': {'entry': {'sha256': five.measure.digest(executable)}},
                'artifact_trees': {'package': {'sha256': five.tree_digest(dependency)}},
                'comparison_package_lock_sha256': five.measure.digest(lock),
            }
            (dependency / 'data').write_text('after')
            with self.assertRaisesRegex(RuntimeError, 'dependency tree changed'):
                five.verify_final_bindings(report, {'entry': executable}, {'package': dependency}, lock)

    def test_native_kernel_peak_captures_short_process_without_polling(self):
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory)
            sampler = home / 'native-sampler'
            subprocess.run(['cc', '-O2', '-std=c11', '-Wall', '-Wextra', '-Werror',
                            str(Path(__file__).with_name('cli-native-memory-measure.c')),
                            '-o', str(sampler)], check=True)
            # Touch actual pages; calloc alone can leave them demand-zero.
            script = 'import sys; x=bytearray(32*1024*1024); x[::4096]=bytes(len(x[::4096])); print("ok"); print("preview",file=sys.stderr)'
            for _ in range(3):
                out, err, usage = five.invoke(sys.executable, ['-c', script], {}, home,
                                              memory=True, native_sampler=sampler)
                self.assertEqual((out, err), (b'ok\n', b'preview\n'))
                self.assertGreater(usage['kernel_process_peak_rss_bytes'], 32 * 1024 * 1024)
                self.assertNotIn('sampled_tree_peak_rss_bytes', usage)
            with self.assertRaisesRegex(RuntimeError, 'exit=3'):
                five.invoke(sys.executable, ['-c', 'raise SystemExit(3)'], {}, home,
                            memory=True, native_sampler=sampler)

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

    @unittest.skipIf(importlib.util.find_spec('psutil') is None, 'psutil is an explicit public-wrapper dependency')
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
