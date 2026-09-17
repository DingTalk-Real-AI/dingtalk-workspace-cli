#!/usr/bin/env python3
"""Measure CLI parent latency with a slow loopback telemetry proxy (no credentials)."""

import argparse
import hashlib
import json
import os
import pathlib
import platform
import resource
import socketserver
import subprocess
import threading
import time


def percentiles(values):
    ordered = sorted(values)
    return {
        f"p{p}_ms": round(ordered[int((len(ordered) - 1) * p / 100)], 3)
        for p in (50, 95, 99)
    }


class SlowProxy(socketserver.BaseRequestHandler):
    def handle(self):
        try:
            self.request.settimeout(7)
            if self.request.recv(8192):
                with self.server.counter_lock:
                    self.server.connections += 1
                time.sleep(6)
        except OSError:
            pass


class ProxyServer(socketserver.ThreadingTCPServer):
    daemon_threads = True
    allow_reuse_address = True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=pathlib.Path)
    parser.add_argument("--output", required=True, type=pathlib.Path)
    parser.add_argument("--work-dir", required=True, type=pathlib.Path)
    parser.add_argument("--samples", type=int, default=400)
    args = parser.parse_args()
    if args.samples < 1:
        parser.error("--samples must be positive")
    binary = args.binary.resolve(strict=True)
    work = args.work_dir.resolve()
    work.mkdir(parents=True, exist_ok=True)
    # Fresh roots keep profile identity and credentials out of the measurement.
    sandbox = work / ("run-" + str(time.time_ns()))
    for name in ("home", "config", "cache", "tmp"):
        (sandbox / name).mkdir(parents=True)
    output = args.output.resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin"), "LANG": "C"}
    env.update(
        HOME=str(sandbox / "home"),
        DWS_CONFIG_DIR=str(sandbox / "config"),
        XDG_CACHE_HOME=str(sandbox / "cache"),
        TMPDIR=str(sandbox / "tmp"),
        ALL_PROXY="",
        NO_PROXY="",
    )
    result = {
        "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
        "host": platform.system() + "/" + platform.machine(),
        "python": platform.python_version(),
        "samples_per_mode": args.samples,
        "warmups_per_mode": 5,
        "method": "alternating on/off; lower empirical quantiles; delta = on quantile - off quantile",
        "limits_ms": {"p95": 10, "p99": 20},
        "scenarios": {},
    }
    with ProxyServer(("127.0.0.1", 0), SlowProxy) as server:
        server.counter_lock = threading.Lock()
        server.connections = 0
        threading.Thread(target=server.serve_forever, daemon=True).start()
        proxy = f"http://127.0.0.1:{server.server_address[1]}"
        env.update(HTTP_PROXY=proxy, HTTPS_PROXY=proxy)
        try:
            for name, command in {
                "version": ["version"],
                "help": ["--help"],
                "invalid": ["version", "--invalid-telemetry-benchmark-flag"],
            }.items():
                wall = {"off": [], "on": []}
                cpu = {"off": [], "on": []}
                baseline = None
                print("benchmark " + name, flush=True)
                for i in range(args.samples + 5):
                    for mode in (("off", "on") if i % 2 == 0 else ("on", "off")):
                        invocation_env = dict(env, DO_NOT_TRACK="1" if mode == "off" else "")
                        before = resource.getrusage(resource.RUSAGE_CHILDREN)
                        started = time.perf_counter()
                        process = subprocess.run(
                            [str(binary), *command], env=invocation_env,
                            capture_output=True, timeout=30 if i < 5 else 3,
                        )
                        elapsed = (time.perf_counter() - started) * 1000
                        after = resource.getrusage(resource.RUSAGE_CHILDREN)
                        signature = (process.returncode, process.stdout, process.stderr)
                        if baseline is None:
                            baseline = signature
                        if signature != baseline:
                            raise RuntimeError(name + ": output or exit code changed")
                        if (name != "invalid" and process.returncode != 0) or (name == "invalid" and process.returncode == 0):
                            raise RuntimeError(name + ": unexpected exit status")
                        if i >= 5:
                            wall[mode].append(elapsed)
                            cpu[mode].append(1000 * (
                                after.ru_utime - before.ru_utime + after.ru_stime - before.ru_stime
                            ))
                summary = {mode: percentiles(values) for mode, values in wall.items()}
                summary["delta_ms"] = {
                    key: round(summary["on"][key] - summary["off"][key], 3)
                    for key in summary["on"]
                }
                summary["parent_cpu"] = {mode: percentiles(values) for mode, values in cpu.items()}
                summary["exit_code"] = baseline[0]
                summary["passed"] = summary["delta_ms"]["p95_ms"] <= 10 and summary["delta_ms"]["p99_ms"] <= 20
                result["scenarios"][name] = {"summary": summary, "samples_ms": wall}
                output.write_text(json.dumps(result, indent=2) + "\n")
                print(json.dumps({name: summary}), flush=True)
            # Keep the loopback proxy alive until bounded senders have expired.
            time.sleep(6.5)
            result["proxy_connections"] = server.connections
            result["passed"] = server.connections > 0 and all(
                item["summary"]["passed"] for item in result["scenarios"].values()
            )
            output.write_text(json.dumps(result, indent=2) + "\n")
        finally:
            server.shutdown()
    return 0 if result["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
