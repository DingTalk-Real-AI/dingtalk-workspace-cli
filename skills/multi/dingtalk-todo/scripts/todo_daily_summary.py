#!/usr/bin/env python3
"""Return open Todo tasks due today, tomorrow, or this week."""

import argparse
import json
import subprocess
import sys
from datetime import datetime, timedelta
from typing import Any, Dict, List, Optional, Tuple
from zoneinfo import ZoneInfo


TIMEZONE = ZoneInfo("Asia/Shanghai")
MAX_PAGES = 40


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


def date_range(scope: str, now: Optional[datetime] = None) -> Tuple[datetime, datetime]:
    current = now.astimezone(TIMEZONE) if now else datetime.now(TIMEZONE)
    today = current.replace(hour=0, minute=0, second=0, microsecond=0)
    if scope == "today":
        return today, today + timedelta(days=1)
    if scope == "tomorrow":
        start = today + timedelta(days=1)
        return start, start + timedelta(days=1)
    start = today - timedelta(days=today.weekday())
    return start, start + timedelta(days=7)


def extract_cards(payload: Dict[str, Any]) -> List[Dict[str, Any]]:
    data: Any = payload.get("data", payload)
    if isinstance(data, dict) and "todos" in data:
        if data.get("complete") is not True:
            raise ScriptError("Todo traversal did not prove endpoint exhaustion")
        value = data["todos"]
    else:
        legacy = payload.get("result", payload)
        value = legacy.get("todoCards") if isinstance(legacy, dict) else None
    if not isinstance(value, list) or not all(isinstance(item, dict) for item in value):
        raise ScriptError("Todo response is missing a valid todos[] collection")
    return value


def due_millis(item: Dict[str, Any]) -> Optional[int]:
    value = item.get("dueTime") or item.get("planFinishDate") or item.get("due")
    if value in (None, ""):
        return None
    try:
        return int(value)
    except (TypeError, ValueError) as exc:
        raise ScriptError(f"invalid due time for task {task_id(item)!r}: {value!r}") from exc


def task_id(item: Dict[str, Any]) -> str:
    value = item.get("taskId") or item.get("id") or item.get("todoTaskId")
    return "" if value is None else str(value)


def run(argv: Optional[List[str]] = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "scope", nargs="?", choices=("today", "tomorrow", "week"), default="today"
    )
    parser.add_argument("--dws", default="dws")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)

    start, end = date_range(args.scope)
    start_ms, end_ms = int(start.timestamp() * 1000), int(end.timestamp() * 1000)
    command = [
        "todo",
        "+get-my-tasks",
        "--status",
        "false",
        "--plan-finish-start",
        str(start_ms),
        "--plan-finish-end",
        str(end_ms),
        "--all",
        "--max-pages",
        str(MAX_PAGES),
        "--format",
        "json",
    ]
    if args.dry_run:
        print(json.dumps({"command": [args.dws, *command]}, ensure_ascii=False))
        return 0

    try:
        cards = extract_cards(run_dws_json(command, args.dws))
        selected = []
        for item in cards:
            due = due_millis(item)
            if due is None or due < start_ms or due >= end_ms:
                continue
            selected.append(
                {
                    "taskId": task_id(item),
                    "title": item.get("subject") or item.get("title") or "",
                    "priority": item.get("priority"),
                    "dueTime": due,
                    "dueTimeISO": datetime.fromtimestamp(
                        due / 1000, TIMEZONE
                    ).isoformat(),
                }
            )
        print(
            json.dumps(
                {
                    "complete": True,
                    "scope": args.scope,
                    "timezone": str(TIMEZONE),
                    "range": {
                        "start": start.isoformat(),
                        "endExclusive": end.isoformat(),
                    },
                    "count": len(selected),
                    "todos": selected,
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
