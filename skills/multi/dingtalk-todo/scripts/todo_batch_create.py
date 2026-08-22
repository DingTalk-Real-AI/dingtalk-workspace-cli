#!/usr/bin/env python3
"""Create up to 30 Todo tasks, preserve every task ID, and verify each task."""

import argparse
import json
import subprocess
import sys
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional
from zoneinfo import ZoneInfo


TIMEZONE = ZoneInfo("Asia/Shanghai")
MAX_ITEMS = 30
MAX_FILE_SIZE = 10 * 1024 * 1024
ALLOWED_PRIORITIES = {10, 20, 30, 40}


class ScriptError(RuntimeError):
    def __init__(self, message: str, *, commit_unknown: bool = False):
        super().__init__(message)
        self.commit_unknown = commit_unknown


def run_dws_json(
    args: List[str], dws: str = "dws", *, write_started: bool = False
) -> Dict[str, Any]:
    try:
        result = subprocess.run(
            [dws, *args], capture_output=True, text=True, timeout=120
        )
    except subprocess.TimeoutExpired as exc:
        raise ScriptError("dws timed out", commit_unknown=write_started) from exc
    except FileNotFoundError as exc:
        raise ScriptError(str(exc), commit_unknown=False) from exc

    payload: Optional[Dict[str, Any]] = None
    if result.stdout.strip():
        try:
            decoded = json.loads(result.stdout)
            if isinstance(decoded, dict):
                payload = decoded
        except json.JSONDecodeError:
            payload = None

    if result.returncode != 0:
        error = payload.get("error", {}) if payload else {}
        started = error.get("execution_started") if isinstance(error, dict) else None
        message = ""
        if isinstance(error, dict):
            message = str(error.get("message") or error.get("hint") or "")
        message = message or result.stderr.strip() or f"dws exited {result.returncode}"
        raise ScriptError(
            message,
            commit_unknown=write_started and started is not False,
        )
    if payload is None:
        raise ScriptError("dws returned non-object or invalid JSON", commit_unknown=write_started)
    if payload.get("ok") is False or payload.get("success") is False:
        error = payload.get("error", {})
        started = error.get("execution_started") if isinstance(error, dict) else None
        message = str(error.get("message") if isinstance(error, dict) else payload)
        raise ScriptError(message, commit_unknown=write_started and started is not False)
    if payload.get("outcome") == "pending":
        raise ScriptError("create outcome is pending", commit_unknown=write_started)
    return payload


def walk_objects(value: Any) -> Iterable[Dict[str, Any]]:
    if isinstance(value, dict):
        yield value
        for child in value.values():
            yield from walk_objects(child)
    elif isinstance(value, list):
        for child in value:
            yield from walk_objects(child)


def first_string(payload: Dict[str, Any], keys: Iterable[str]) -> str:
    for obj in walk_objects(payload):
        for key in keys:
            value = obj.get(key)
            if value not in (None, ""):
                return str(value)
    return ""


def normalize_due(value: Any) -> Optional[str]:
    if value in (None, ""):
        return None
    raw = str(value).strip()
    if raw.isdigit():
        try:
            return datetime.fromtimestamp(int(raw) / 1000, TIMEZONE).isoformat()
        except (OSError, OverflowError, ValueError) as exc:
            raise ScriptError(f"invalid epoch-millisecond due time: {raw}") from exc
    if len(raw) == 10:
        try:
            day = datetime.strptime(raw, "%Y-%m-%d").replace(
                hour=23, minute=59, second=59, tzinfo=TIMEZONE
            )
            return day.isoformat()
        except ValueError as exc:
            raise ScriptError(f"invalid due date: {raw}") from exc
    try:
        parsed = datetime.fromisoformat(raw.replace("Z", "+00:00"))
    except ValueError as exc:
        raise ScriptError(
            f"due must be YYYY-MM-DD, epoch milliseconds, or ISO-8601: {raw}"
        ) from exc
    if parsed.tzinfo is None:
        raise ScriptError(f"ISO due time must include a timezone: {raw}")
    return parsed.isoformat()


def validate(items: Any) -> List[Dict[str, Any]]:
    if not isinstance(items, list) or not items:
        raise ScriptError("input must be a non-empty JSON array")
    if len(items) > MAX_ITEMS:
        raise ScriptError(f"a batch may contain at most {MAX_ITEMS} tasks")
    validated: List[Dict[str, Any]] = []
    for index, item in enumerate(items, 1):
        if not isinstance(item, dict):
            raise ScriptError(f"item {index} must be an object")
        title = str(item.get("title") or "").strip()
        executors = str(item.get("executors") or "").strip()
        if not title or not executors:
            raise ScriptError(f"item {index} requires non-empty title and executors")
        priority = item.get("priority")
        if priority is not None:
            try:
                priority = int(priority)
            except (TypeError, ValueError) as exc:
                raise ScriptError(f"item {index} has invalid priority") from exc
            if priority not in ALLOWED_PRIORITIES:
                raise ScriptError(
                    f"item {index} priority must be one of {sorted(ALLOWED_PRIORITIES)}"
                )
        due = normalize_due(item.get("due"))
        recurrence = item.get("recurrence")
        if recurrence and not due:
            raise ScriptError(f"item {index} recurrence requires due")
        validated.append(
            {
                "title": title,
                "executors": executors,
                "priority": priority,
                "due": due,
                "recurrence": (
                    str(recurrence).replace("\\n", "\n") if recurrence else None
                ),
            }
        )
    return validated


def run(argv: Optional[List[str]] = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", type=Path)
    parser.add_argument("--dws", default="dws")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)

    try:
        if not args.input.is_file():
            raise ScriptError(f"input file not found: {args.input}")
        if args.input.stat().st_size > MAX_FILE_SIZE:
            raise ScriptError(f"input exceeds {MAX_FILE_SIZE} bytes")
        items = validate(json.loads(args.input.read_text(encoding="utf-8")))
    except (OSError, json.JSONDecodeError, ScriptError) as exc:
        print(json.dumps({"complete": False, "error": str(exc)}, ensure_ascii=False))
        return 2

    ledger: List[Dict[str, Any]] = []
    for item in items:
        create = [
            "todo",
            "task",
            "create",
            "--title",
            item["title"],
            "--executors",
            item["executors"],
        ]
        if item["priority"] is not None:
            create.extend(["--priority", str(item["priority"])])
        if item["due"]:
            create.extend(["--due", item["due"]])
        if item["recurrence"]:
            create.extend(["--recurrence", item["recurrence"]])
        create.extend(["--format", "json"])
        if args.dry_run:
            ledger.append({"title": item["title"], "command": [args.dws, *create]})
            continue

        entry: Dict[str, Any] = {"title": item["title"], "status": "unknown"}
        try:
            created = run_dws_json(create, args.dws, write_started=True)
            identifier = first_string(created, ("taskId", "todoTaskId"))
            if not identifier:
                raise ScriptError(
                    "create response did not contain a stable taskId",
                    commit_unknown=True,
                )
            entry["taskId"] = identifier
            try:
                detail = run_dws_json(
                    [
                        "todo",
                        "task",
                        "get",
                        "--task-id",
                        identifier,
                        "--format",
                        "json",
                    ],
                    args.dws,
                )
                actual_title = first_string(detail, ("subject", "title"))
                if actual_title != item["title"]:
                    raise ScriptError(
                        f"readback title mismatch: expected {item['title']!r}, "
                        f"got {actual_title!r}"
                    )
                entry["status"] = "verified"
            except ScriptError as exc:
                entry.update({"status": "unverified", "error": str(exc)})
        except ScriptError as exc:
            entry.update(
                {
                    "status": "unknown" if exc.commit_unknown else "failed",
                    "error": str(exc),
                }
            )
        ledger.append(entry)

    complete = args.dry_run or all(item.get("status") == "verified" for item in ledger)
    output = {
        "complete": complete,
        "dryRun": args.dry_run,
        "requestedCount": len(items),
        "verifiedCount": sum(item.get("status") == "verified" for item in ledger),
        "failedCount": sum(item.get("status") == "failed" for item in ledger),
        "unverifiedCount": sum(item.get("status") == "unverified" for item in ledger),
        "unknownCount": sum(item.get("status") == "unknown" for item in ledger),
        "ledger": ledger,
    }
    print(json.dumps(output, ensure_ascii=False, indent=2))
    return 0 if complete else 2


if __name__ == "__main__":
    sys.exit(run())
