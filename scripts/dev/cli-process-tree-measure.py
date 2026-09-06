#!/usr/bin/env python3
"""Separate memory-only trial; sampled simultaneous process-tree RSS, not latency."""

import json
import subprocess
import sys
import time

import psutil


def main():
    request = json.load(sys.stdin)
    peak = observations = max_processes = 0
    seen = set()
    started = time.monotonic()
    with open(request['stdout'], 'wb') as stdout, open(request['stderr'], 'wb') as stderr:
        child = subprocess.Popen(request['argv'], env=request['env'], cwd=request['cwd'],
                                 stdin=subprocess.DEVNULL, stdout=stdout, stderr=stderr)
        root = psutil.Process(child.pid)
        known = {root}
        timed_out = False
        try:
            while child.poll() is None:
                # Keep already-discovered descendants even if an intermediate
                # parent exits. Never add the measurement process itself.
                for process in list(known):
                    try:
                        known.update(process.children(recursive=True))
                    except psutil.NoSuchProcess:
                        pass
                total = count = 0
                for process in list(known):
                    try:
                        total += process.memory_info().rss
                        count += 1
                        seen.add((process.pid, process.create_time()))
                    except psutil.NoSuchProcess:
                        known.discard(process)
                peak = max(peak, total)
                max_processes = max(max_processes, count)
                observations += 1
                if time.monotonic() - started > request['timeout_seconds']:
                    timed_out = True
                    break
                time.sleep(.001)
        finally:
            if timed_out or child.poll() is None:
                for process in known:
                    try:
                        process.kill()
                    except psutil.NoSuchProcess:
                        pass
                child.kill()
            child.wait()
    json.dump({'returncode': child.returncode, 'timed_out': timed_out,
               'measurement': {'sampled_tree_peak_rss_bytes': peak,
                               'observations': observations,
                               'max_simultaneous_processes': max_processes,
                               'distinct_processes': len(seen)}}, sys.stdout)


if __name__ == '__main__':
    main()
