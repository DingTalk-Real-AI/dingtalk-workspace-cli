#!/usr/bin/env python3
"""Measure exactly one command from a fresh, small parent process.

Linux preserves pre-exec peak RSS. Forking the candidate directly from the
wire-parity verifier can include hundreds of MB of the verifier's JSON heap.
The sampler's own inherited high-water mark is never reported; wait4 measures
its child, forked after this fresh interpreter has discarded that address space.
"""

import json
import os
import subprocess
import sys
import time


def main():
    request = json.load(sys.stdin)
    with open(request["stdout"], "wb") as stdout, open(request["stderr"], "wb") as stderr:
        started = time.perf_counter()
        child = subprocess.Popen(request["argv"], env=request["env"], cwd=request["cwd"],
                                 stdin=subprocess.DEVNULL, stdout=stdout, stderr=stderr)
        _, status, usage = os.wait4(child.pid, 0)
        elapsed = (time.perf_counter() - started) * 1000
        child.returncode = os.waitstatus_to_exitcode(status)
    json.dump({"returncode": child.returncode, "measurement": {
        "wall_ms": elapsed, "user_ms": usage.ru_utime * 1000,
        "system_ms": usage.ru_stime * 1000,
        "max_rss_bytes": usage.ru_maxrss * (1 if sys.platform == "darwin" else 1024),
    }}, sys.stdout)


if __name__ == "__main__":
    main()
