#!/usr/bin/env python3
"""Verify the committed shortcut runtime map matches the skill catalog."""

from __future__ import annotations

import json
import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
CATALOG_PATH = ROOT / "docs" / "shortcut-public-catalog.json"
GO_PATH = ROOT / "internal" / "shortcut" / "public_catalog_generated.go"
GO_ENTRY = re.compile(r'^\s*("(?:\\.|[^"\\])*"):\s*\{\},\s*$')


def fail(message: str) -> None:
    raise SystemExit(f"shortcut public catalog drift: {message}")


def main() -> None:
    payload = json.loads(CATALOG_PATH.read_text(encoding="utf-8"))
    rows = payload.get("results")
    if not isinstance(rows, list):
        fail("docs catalog results must be a list")

    documented: set[str] = set()
    for index, row in enumerate(rows):
        if not isinstance(row, dict):
            fail(f"results[{index}] must be an object")
        service = row.get("service")
        command = row.get("command")
        if not isinstance(service, str) or not service or not isinstance(command, str) or not command:
            fail(f"results[{index}] requires non-empty service and command")
        key = f"{service}\0{command}"
        if key in documented:
            fail(f"duplicate docs entry {service} {command}")
        documented.add(key)

    if payload.get("count") != len(documented):
        fail(f"docs count={payload.get('count')!r}, actual={len(documented)}")

    runtime: set[str] = set()
    for line in GO_PATH.read_text(encoding="utf-8").splitlines():
        match = GO_ENTRY.match(line)
        if not match:
            continue
        key = json.loads(match.group(1))
        if key in runtime:
            service, command = key.split("\0", 1)
            fail(f"duplicate runtime entry {service} {command}")
        runtime.add(key)

    if documented != runtime:
        missing_runtime = sorted(documented - runtime)
        missing_docs = sorted(runtime - documented)
        details = []
        if missing_runtime:
            details.append("missing from runtime: " + ", ".join(key.replace("\0", " ") for key in missing_runtime[:10]))
        if missing_docs:
            details.append("missing from docs: " + ", ".join(key.replace("\0", " ") for key in missing_docs[:10]))
        fail("; ".join(details))

    print(f"shortcut public catalog drift check: ok ({len(runtime)} entries)")


if __name__ == "__main__":
    main()
