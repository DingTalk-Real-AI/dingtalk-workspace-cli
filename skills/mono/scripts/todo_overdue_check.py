#!/usr/bin/env python3
"""Return overdue open Todo tasks through the bounded Todo Shortcut."""

import argparse
import json
import subprocess
import sys
from datetime import datetime, timedelta, timezone
from typing import Any, Dict, List, Optional


TIMEZONE = timezone(timedelta(hours=8), name="Asia/Shanghai")


class ScriptError(RuntimeError):
    pass


def run_dws_json(args: List[str], dws: str = "dws") -> Dict[str, Any]:
    try:
        result = subprocess.run(
            [dws, *args], capture_output=True, text=True, timeout=120
        )
    except (subprocess.TimeoutExpired, FileNotFoundError) as exc:
        raise ScriptError(str(exc)) from exc
    if result.returncode != 0:
        raise ScriptError(result.stderr.strip() or f"dws exited {result.returncode}")
    try:
        payload = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        raise ScriptError("dws returned non-JSON output") from exc
    if not isinstance(payload, dict):
        raise ScriptError("dws returned a non-object JSON payload")
    if payload.get("ok") is False or payload.get("success") is False:
        error = payload.get("error", {})
        raise ScriptError(str(error.get("message") if isinstance(error, dict) else payload))
    return payload


def extract_overdue(payload: Dict[str, Any]) -> List[Dict[str, Any]]:
    data: Any = payload.get("data", payload)
    value = data.get("overdue") if isinstance(data, dict) else None
    if not isinstance(value, list) or not all(isinstance(item, dict) for item in value):
        raise ScriptError("Todo response is missing a valid overdue[] collection")
    return value


def run(argv: Optional[List[str]] = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dws", default="dws")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)
    command = ["todo", "+overdue", "--format", "json"]
    if args.dry_run:
        print(json.dumps({"command": [args.dws, *command]}, ensure_ascii=False))
        return 0

    try:
        now = datetime.now(TIMEZONE)
        items = []
        for item in extract_overdue(run_dws_json(command, args.dws)):
            task = item.get("taskId")
            due = item.get("dueTime")
            if not isinstance(task, str) or not task.strip() or due in (None, ""):
                raise ScriptError("overdue item is missing taskId or dueTime")
            try:
                due_ms = int(due)
                due_at = datetime.fromtimestamp(due_ms / 1000, TIMEZONE)
            except (OSError, TypeError, ValueError) as exc:
                raise ScriptError(f"invalid overdue dueTime: {due!r}") from exc
            items.append(
                {
                    "taskId": task.strip(),
                    "title": item.get("subject") or item.get("title") or "",
                    "dueTime": due_ms,
                    "dueTimeISO": due_at.isoformat(),
                    "daysOverdue": max(0, (now.date() - due_at.date()).days),
                }
            )
        items.sort(key=lambda item: item["dueTime"])
        print(
            json.dumps(
                {
                    "complete": True,
                    "timezone": str(TIMEZONE),
                    "checkedAt": now.isoformat(),
                    "count": len(items),
                    "todos": items,
                },
                ensure_ascii=False,
                indent=2,
            )
        )
        return 0
    except ScriptError as exc:
        print(json.dumps({"complete": False, "error": str(exc)}, ensure_ascii=False))
        return 2


if __name__ == "__main__":
    sys.exit(run())
