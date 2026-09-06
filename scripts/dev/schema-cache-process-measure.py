#!/usr/bin/env python3
"""Measure exactly one command from a fresh, small parent process.

Linux preserves pre-exec peak RSS. Forking the candidate directly from the
wire-parity verifier can include hundreds of MB of the verifier's JSON heap.
The sampler's own inherited high-water mark is never reported; wait4 measures
its child, forked after this fresh interpreter has discarded that address space.
"""

import json
import math
import os
import signal
import subprocess
import sys
import time


class CommandTimedOut(Exception):
    pass


def timeout_handler(signum, frame):
    raise CommandTimedOut()


def terminate_process_tree(child, grace_seconds=2):
    """Stop a wrapper and descendants, including children in another group."""
    descendants = []
    try:
        import psutil
    except ImportError:
        psutil = None
    if psutil is not None:
        try:
            root = psutil.Process(child.pid)
            descendants = root.children(recursive=True)
        except psutil.NoSuchProcess:
            pass
    if child.poll() is None:
        try:
            child.terminate()
        except ProcessLookupError:
            pass
    if descendants:
        for process in descendants:
            try:
                process.terminate()
            except psutil.NoSuchProcess:
                pass
        _, alive = psutil.wait_procs(descendants, timeout=grace_seconds)
        for process in alive:
            try:
                process.kill()
            except psutil.NoSuchProcess:
                pass
    try:
        child.wait(timeout=grace_seconds)
    except subprocess.TimeoutExpired:
        try:
            child.kill()
        except ProcessLookupError:
            pass
        child.wait()


def main():
    request = json.load(sys.stdin)
    timeout = float(request['timeout_seconds'])
    if not math.isfinite(timeout) or timeout <= 0:
        raise ValueError('measurement timeout must be finite and positive')
    signal.signal(signal.SIGALRM, timeout_handler)
    timed_out = False
    with open(request["stdout"], "wb") as stdout, open(request["stderr"], "wb") as stderr:
        started = time.perf_counter()
        child = subprocess.Popen(request["argv"], env=request["env"], cwd=request["cwd"],
                                 stdin=subprocess.DEVNULL, stdout=stdout, stderr=stderr)
        # Keep blocking wait4 accounting on successful samples. Polling with
        # sleeps would quantize short CLI timings and corrupt the latency gate.
        try:
            signal.setitimer(signal.ITIMER_REAL, max(.000001, timeout - (time.perf_counter() - started)))
            _, status, usage = os.wait4(child.pid, 0)
            signal.setitimer(signal.ITIMER_REAL, 0)
        except CommandTimedOut:
            timed_out = True
            signal.setitimer(signal.ITIMER_REAL, 0)
            # The real npm wrapper creates a separate vendor process group.
            # SIGTERM lets it forward shutdown; psutil (when installed by the
            # process-tree job) also closes descendants if the wrapper cannot.
            terminate_process_tree(child)
        finally:
            signal.setitimer(signal.ITIMER_REAL, 0)
        elapsed = (time.perf_counter() - started) * 1000
        if not timed_out:
            child.returncode = os.waitstatus_to_exitcode(status)
    json.dump({"returncode": child.returncode, "timed_out": timed_out, "measurement": None if timed_out else {
        "wall_ms": elapsed, "user_ms": usage.ru_utime * 1000,
        "system_ms": usage.ru_stime * 1000,
        "max_rss_bytes": usage.ru_maxrss * (1 if sys.platform == "darwin" else 1024),
    }}, sys.stdout)


if __name__ == "__main__":
    main()
