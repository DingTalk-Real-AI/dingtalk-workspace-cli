#!/usr/bin/env python3
"""Run one shortcut command and append its real CLI result to a JSON report."""

from __future__ import annotations

import argparse
import datetime as dt
import json
import shlex
import subprocess
import time
from pathlib import Path

from shortcut_real_result import classify_failure, summarize_failure_categories, classify_real_status, summarize_results


ROOT = Path(__file__).resolve().parents[1]
OUT_PATH = ROOT / "docs" / "shortcut-real-write-results.json"


def load_report() -> dict:
    if not OUT_PATH.exists():
        return {
            "generated_at": dt.datetime.now().isoformat(),
            "summary": {"total": 0, "ok": 0, "error": 0, "timeout": 0, "held": 0},
            "results": [],
        }
    return json.loads(OUT_PATH.read_text(encoding="utf-8"))


def shell_join(cmd: list[str]) -> str:
    return " ".join(shlex.quote(x) for x in cmd)


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--service", required=True)
    p.add_argument("--command", required=True)
    p.add_argument("--risk", default="write")
    p.add_argument("--method", default="real-backend-manual-write; no --mock; no --dry-run")
    p.add_argument("--timeout", type=int, default=35)
    p.add_argument("argv", nargs=argparse.REMAINDER)
    ns = p.parse_args()
    argv = ns.argv
    if argv and argv[0] == "--":
        argv = argv[1:]
    if not argv:
        raise SystemExit("missing command argv")

    started = time.time()
    status = "real-error"
    stdout = ""
    stderr = ""
    exit_code = None
    try:
        proc = subprocess.run(argv, text=True, capture_output=True, timeout=ns.timeout)
        stdout = proc.stdout.strip()
        stderr = proc.stderr.strip()
        exit_code = proc.returncode
        status = classify_real_status(exit_code, stdout)
    except subprocess.TimeoutExpired as e:
        status = "timeout"
        if isinstance(e.stdout, str):
            stdout = e.stdout.strip()
        if isinstance(e.stderr, str):
            stderr = e.stderr.strip()
    duration_ms = int((time.time() - started) * 1000)

    report = load_report()
    # Replace previous result for the same shortcut only when this manual run is
    # more specific than a broad held/negative placeholder.
    result = {
        "service": ns.service,
        "command": ns.command,
        "risk": ns.risk,
        "method": ns.method,
        "status": status,
        "input": shell_join(argv),
        "args": argv[1:],
        "stdout": stdout,
        "stderr": stderr,
        "exit_code": exit_code,
        "duration_ms": duration_ms,
        "recorded_at": dt.datetime.now().isoformat(),
    }
    category, fixability, note = classify_failure(result)
    if category != "passed":
        result["failure_category"] = category
        result["fixability"] = fixability
        result["diagnosis"] = note
    old = report.get("results", [])
    old = [r for r in old if not (r.get("service") == ns.service and r.get("command") == ns.command)]
    old.append(result)
    report["results"] = old
    report["summary"] = summarize_results(old, include_held=True)
    report["failure_categories"] = summarize_failure_categories(old)
    report["generated_at"] = dt.datetime.now().isoformat()
    OUT_PATH.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if status == "real-ok" else 2


if __name__ == "__main__":
    raise SystemExit(main())
