#!/usr/bin/env python3
"""Check RFC file-hit budgets using at least seven Go benchmark samples.

The median is across benchmark averages, not an invocation latency percentile.
"""

import argparse
import json
from pathlib import Path
import re
import statistics


BUDGETS = {
    "meta-open-authenticate-decode-lookup": 5_000_000,
    "selected-open-locator-authenticate-decode-index": 15_000_000,
}
LINE = re.compile(r"^BenchmarkRealSchemaFileHit/(\S+)\s+\d+\s+(\d+(?:\.\d+)?)\s+ns/op\s+(\d+)\s+B/op\s+(\d+)\s+allocs/op\s*$")


def check(text):
    samples = {name: [] for name in BUDGETS}
    for line in text.splitlines():
        match = LINE.fullmatch(line)
        if not match:
            continue
        label, ns, allocated, allocations = match.groups()
        label = re.sub(r"-\d+$", "", label)
        if label in samples:
            samples[label].append((float(ns), int(allocated), int(allocations)))
    result = {}
    for name, budget in BUDGETS.items():
        values = samples[name]
        if len(values) < 7:
            raise ValueError(f"{name}: found {len(values)} samples, require at least seven")
        latency = statistics.median(row[0] for row in values)
        allocated = statistics.median(row[1] for row in values)
        result[name] = {"samples": len(values), "median_ns_per_op": latency,
                        "median_bytes_per_op": allocated,
                        "median_allocations_per_op": statistics.median(row[2] for row in values),
                        "latency_budget_ns": budget, "allocation_budget_bytes": 8_000_000,
                        "passed": latency <= budget and allocated <= 8_000_000}
    return {"scope": "median of Go benchmark averages; warm OS page cache",
            "stages": result, "passed": all(stage["passed"] for stage in result.values())}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", type=Path)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    try:
        report = check(args.input.read_text())
    except ValueError as error:
        parser.error(str(error))
    rendered = json.dumps(report, indent=2) + "\n"
    if args.output:
        args.output.write_text(rendered)
    print(rendered, end="")
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
