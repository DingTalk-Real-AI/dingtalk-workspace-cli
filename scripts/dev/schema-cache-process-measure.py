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
            child.kill()
            # A deadline may race with successful wait4 completion. Popen's
            # wait tolerates an already-reaped child; failed samples have no
            # resource-usage claim and are rejected by the coordinator.
            child.wait()
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
