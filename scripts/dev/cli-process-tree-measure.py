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
        try:
            root = psutil.Process(child.pid)
            known = {root}
        except psutil.NoSuchProcess:
            known = set()
        timed_out = False
        try:
            while True:
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
                if child.poll() is not None:
                    break
                if time.monotonic() - started > request['timeout_seconds']:
                    timed_out = True
                    break
                time.sleep(.001)
        finally:
            if timed_out or child.poll() is None:
                # Terminate the wrapper first so the production npm wrapper can
                # forward SIGTERM into its detached vendor process group.
                try:
                    child.terminate()
                except ProcessLookupError:
                    pass
                for process in list(known):
                    try:
                        known.update(process.children(recursive=True))
                    except psutil.NoSuchProcess:
                        pass
                for process in known:
                    try:
                        process.terminate()
                    except psutil.NoSuchProcess:
                        pass
                _, alive = psutil.wait_procs(list(known), timeout=2)
                for process in alive:
                    try:
                        process.kill()
                    except psutil.NoSuchProcess:
                        pass
                if child.poll() is None:
                    try:
                        child.kill()
                    except ProcessLookupError:
                        pass
            child.wait()
    json.dump({'returncode': child.returncode, 'timed_out': timed_out,
               'measurement': {'sampled_tree_peak_rss_bytes': peak,
                               'observations': observations,
                               'max_simultaneous_processes': max_processes,
                               'distinct_processes': len(seen)}}, sys.stdout)


if __name__ == '__main__':
    main()
