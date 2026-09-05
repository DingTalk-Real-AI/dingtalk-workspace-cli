#!/usr/bin/env python3
"""Exercise the multi-process verifier itself before trusting candidate evidence."""

import importlib.util
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

spec = importlib.util.spec_from_file_location(
    "verify_schema_cache_binary", Path(__file__).with_name("verify-schema-cache-binary.py"))
verifier = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verifier)


class ConcurrentInvocationTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.binary = self.root / "child"
        self.binary.write_text(f"#!{sys.executable}\n" + """
import json, os, pathlib, sys, time
root = pathlib.Path.cwd()
mode, index, total = sys.argv[1:]
(root / (index + '.pid')).write_text(str(os.getpid()))
while len(list(root.glob('*.pid'))) < int(total):
    time.sleep(.01)
if mode == 'fail' and index == '0':
    print('fixture failure', file=sys.stderr)
    sys.exit(7)
if mode in ('timeout', 'fail'):
    time.sleep(60)
print(json.dumps({'index': index}))
""")
        self.binary.chmod(0o700)

    def invoke(self, mode, expected=None, timeout=10):
        return verifier.invoke_concurrently(
            self.binary, [[mode, str(i), "4"] for i in range(4)],
            expected or [{"index": str(i)} for i in range(4)],
            os.environ.copy(), self.root, timeout=timeout)

    def assert_reaped(self):
        pids = list(self.root.glob("*.pid"))
        self.assertEqual(len(pids), 4)
        for path in pids:
            with self.assertRaises(ProcessLookupError):
                os.kill(int(path.read_text()), 0)
        self.assertEqual(list(self.root.glob("concurrent-*")), [])

    def test_children_overlap_and_outputs_match(self):
        result = self.invoke("success")
        self.assertEqual(result["processes"], 4)
        self.assertTrue(result["wire_parity"])
        self.assert_reaped()

    def test_mismatch_is_rejected(self):
        with self.assertRaisesRegex(RuntimeError, "differs from authoritative"):
            self.invoke("success", [{"index": "wrong"}] * 4)
        self.assert_reaped()

    def test_failure_reaps_other_live_children(self):
        with self.assertRaisesRegex(RuntimeError, "exit 7: fixture failure"):
            self.invoke("fail")
        self.assert_reaped()

    def test_timeout_reaps_all_children(self):
        with self.assertRaises(subprocess.TimeoutExpired):
            self.invoke("timeout", timeout=1)
        self.assert_reaped()


class ProcessAccountingTest(unittest.TestCase):
    def test_retained_parent_json_does_not_become_candidate_rss(self):
        with tempfile.TemporaryDirectory() as directory:
            def sample():
                output, usage = verifier.invoke(Path(sys.executable), ["-c", "print('ok')"],
                                                os.environ.copy(), Path(directory))
                self.assertEqual(output, b"ok\n")
                self.assertGreater(usage["wall_ms"], 0)
                return usage["max_rss_bytes"]
            baseline = sample()
            # Keep resident pages alive while the second candidate is spawned.
            retained = bytearray(b"x" * (128 * 1024 * 1024))
            loaded = sample()
            self.assertEqual(retained[-1], ord("x"))
            self.assertLess(loaded, baseline + 32 * 1024 * 1024,
                            "candidate RSS includes the coordinator's retained heap")


if __name__ == "__main__":
    unittest.main()
