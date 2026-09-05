#!/usr/bin/env python3
"""Retain every go test -json byte while keeping hosted CI progress bounded.

Use with shell pipefail: go test -json ... 2>&1 | this-script --output raw.jsonl.
This recorder does not turn the test process's failure into a success.
"""

import argparse
from collections import defaultdict, deque
import json
from pathlib import Path
import sys
import time


def memory_snapshot():
    """Read only aggregate memory and process names/RSS, never argv or environ."""
    if not Path('/proc/meminfo').exists():
        return None
    try:
        values = dict(line.split(':', 1) for line in Path('/proc/meminfo').read_text().splitlines())
        result = {'available_mib': int(values['MemAvailable'].split()[0]) // 1024,
                  'total_mib': int(values['MemTotal'].split()[0]) // 1024}
        # A container may hit its own memory limit while host MemAvailable is
        # still large. Include cgroup v2 counters without reading process env.
        try:
            root = Path('/sys/fs/cgroup')
            group = next(line[3:] for line in Path('/proc/self/cgroup').read_text().splitlines()
                         if line.startswith('0::'))
            candidates = [(root / group.lstrip('/')).resolve(), root]
            for candidate in candidates:
                if not candidate.is_relative_to(root) or not (candidate / 'memory.current').exists():
                    continue
                counters = {'path': str(candidate)}
                for field in ('memory.current', 'memory.max', 'memory.peak', 'memory.events'):
                    path = candidate / field
                    if path.exists():
                        counters[field] = path.read_text().strip()
                result['cgroup_v2'] = counters
                break
        except (OSError, StopIteration, ValueError):
            pass
        processes = []
        for path in Path('/proc').iterdir():
            if not path.name.isdigit():
                continue
            try:
                status = dict(line.split(':', 1) for line in (path / 'status').read_text().splitlines())
                name = status['Name'].strip()
                if name.endswith('.test') or name in ('go', 'compile', 'link', 'Runner.Worker', 'Runner.Listener'):
                    processes.append({'name': name, 'rss_mib': int(status.get('VmRSS', '0').split()[0]) // 1024})
            except (OSError, KeyError, ValueError):
                continue
        result['largest_test_or_runner_processes'] = sorted(processes, key=lambda p: p['rss_mib'], reverse=True)[:6]
        return result
    except (OSError, KeyError, ValueError):
        return None


def record(source, raw_output, console):
    recent = defaultdict(lambda: deque(maxlen=12))
    active = {}
    last_observation = time.monotonic()

    def emit(message):
        print(message, file=console, flush=True)
        raw_output.flush()

    for raw in source:
        raw_output.write(raw)
        try:
            event = json.loads(raw)
            if not isinstance(event, dict):
                raise ValueError('not an event object')
        except (ValueError, UnicodeDecodeError):
            emit('[go diagnostic] ' + raw.decode('utf-8', 'replace').rstrip()[:2048])
            continue
        action, package, test = event.get('Action'), event.get('Package', ''), event.get('Test', '')
        if action == 'build-output':
            emit('[build] ' + str(event.get('Output', '')).rstrip()[:2048])
            continue
        if action == 'build-fail':
            emit('[BUILD FAIL] ' + str(event.get('ImportPath', '')))
            continue
        if action == 'start':
            active[package] = {}
            emit(f'[package start] {package}')
        if action in ('run', 'cont'):
            active.setdefault(package, {})[test] = None
            if '/' not in test:
                emit(f'[{action}] {package} {test}')
        elif action == 'output':
            recent[package].append(str(event.get('Output', '')).rstrip()[:2048])
        elif action == 'fail':
            emit(f'[FAIL] {package} {test} ({event.get("Elapsed", "?")}s)')
            for output in recent[package]:
                emit('  ' + output)
        if action in ('pass', 'fail', 'skip', 'pause') and test:
            active.get(package, {}).pop(test, None)
        if action in ('pass', 'fail', 'skip') and not test:
            emit(f'[package {action}] {package} ({event.get("Elapsed", "?")}s)')
            active.pop(package, None)
            recent.pop(package, None)
        if time.monotonic() - last_observation >= 10:
            emit('[progress] ' + json.dumps({'active': {p: list(t)[-5:] for p, t in active.items()},
                                            'memory': memory_snapshot()}, sort_keys=True))
            last_observation = time.monotonic()
    raw_output.flush()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    with args.output.open('xb') as raw:
        record(sys.stdin.buffer, raw, sys.stdout)


if __name__ == '__main__':
    main()
