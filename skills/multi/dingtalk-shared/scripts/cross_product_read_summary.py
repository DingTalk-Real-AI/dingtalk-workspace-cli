#!/usr/bin/env python3
"""Compact, read-only DWS summary for calendar, todos, and minutes."""

import argparse
import json
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor
from datetime import date, datetime, time, timedelta
from typing import Any, Dict, List, Optional, Tuple
from zoneinfo import ZoneInfo


PAGE_SIZE = 100
MAX_TODO_PAGES = 10
MAX_TODO_ITEMS = 20
MAX_ERROR_CHARS = 800
MAX_SUMMARY_CHARS = 8000
PRIORITY_LABELS = {10: "low", 20: "normal", 30: "high", 40: "urgent"}


def run_dws(args: List[str]) -> Tuple[Optional[Any], Optional[str]]:
    try:
        completed = subprocess.run(
            ["dws", *args], capture_output=True, text=True, timeout=90
        )
    except (FileNotFoundError, subprocess.TimeoutExpired) as exc:
        return None, str(exc)[:MAX_ERROR_CHARS]
    if completed.returncode != 0:
        message = completed.stderr.strip() or completed.stdout.strip()
        return None, message[:MAX_ERROR_CHARS]
    try:
        payload = json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        return None, f"invalid JSON: {exc}"[:MAX_ERROR_CHARS]
    if isinstance(payload, dict) and payload.get("success") is False:
        message = payload.get("errorMsg") or payload.get("errorCode") or "DWS request failed"
        return None, str(message)[:MAX_ERROR_CHARS]
    return payload, None


def result_of(payload: Any) -> Any:
    if isinstance(payload, dict) and "result" in payload:
        return payload["result"]
    return payload


def object_list(payload: Any, *keys: str) -> List[Dict[str, Any]]:
    value = result_of(payload)
    if isinstance(value, list):
        return [item for item in value if isinstance(item, dict)]
    if isinstance(value, dict):
        for key in keys:
            items = value.get(key)
            if isinstance(items, list):
                return [item for item in items if isinstance(item, dict)]
    return []


def calendar_summary(day: date, tz: ZoneInfo) -> Dict[str, Any]:
    start = datetime.combine(day, time.min, tzinfo=tz)
    end = start + timedelta(days=1)
    payload, error = run_dws([
        "calendar", "event", "list",
        "--start", start.isoformat(), "--end", end.isoformat(),
        "--format", "json",
    ])
    if error:
        return {"ok": False, "error": error, "count": 0, "items": []}
    events = object_list(payload, "events", "itemList")
    items = []
    for event in events:
        start_value = event.get("start")
        end_value = event.get("end")
        location = event.get("location")
        items.append({
            "title": event.get("summary") or event.get("title") or "",
            "start": (start_value.get("dateTime") if isinstance(start_value, dict) else start_value),
            "end": (end_value.get("dateTime") if isinstance(end_value, dict) else end_value),
            "location": (location.get("displayName") if isinstance(location, dict) else location) or "",
        })
    return {"ok": True, "count": len(items), "items": items}


def todo_page(page: int) -> Tuple[Optional[Any], Optional[str]]:
    return run_dws([
        "todo", "task", "list", "--status", "false",
        "--page", str(page), "--size", str(PAGE_SIZE), "--format", "json",
    ])


def timestamp_iso(value: Any, tz: ZoneInfo) -> Optional[str]:
    if value in (None, ""):
        return None
    try:
        return datetime.fromtimestamp(int(value) / 1000, tz=tz).isoformat()
    except (TypeError, ValueError, OSError):
        return str(value)


def todo_summary(tz: ZoneInfo) -> Dict[str, Any]:
    todos: List[Dict[str, Any]] = []
    complete = True
    for page in range(1, MAX_TODO_PAGES + 1):
        payload, error = todo_page(page)
        if error:
            return {"ok": False, "error": error, "count": len(todos), "items": []}
        items = object_list(payload, "todoCards", "itemList")
        todos.extend(items)
        inner = result_of(payload)
        has_more = bool(inner.get("hasMore")) if isinstance(inner, dict) else len(items) == PAGE_SIZE
        if not has_more or not items:
            break
    else:
        complete = False

    def sort_key(item: Dict[str, Any]) -> Tuple[int, int, int]:
        due = item.get("dueTime")
        due_value = int(due) if str(due or "").isdigit() else 2**63 - 1
        priority = item.get("priority")
        priority_value = int(priority) if str(priority or "").isdigit() else 0
        created = item.get("createdTime")
        created_value = int(created) if str(created or "").isdigit() else 0
        return (due_value, -priority_value, -created_value)

    selected = sorted(todos, key=sort_key)[:MAX_TODO_ITEMS]
    items = []
    for item in selected:
        priority = item.get("priority")
        try:
            priority_number = int(priority)
        except (TypeError, ValueError):
            priority_number = 20
        items.append({
            "title": item.get("subject") or item.get("title") or "",
            "priority": PRIORITY_LABELS.get(priority_number, str(priority_number)),
            "dueTime": timestamp_iso(item.get("dueTime"), tz),
        })
    return {
        "ok": True,
        "count": len(todos),
        "shown": len(items),
        "truncated": not complete or len(todos) > len(items),
        "items": items,
    }


def minutes_summary() -> Dict[str, Any]:
    payload, error = run_dws([
        "minutes", "list", "all", "--limit", "1", "--format", "json",
    ])
    if error:
        return {"ok": False, "error": error, "found": False}
    records = object_list(payload, "itemList", "items")
    if not records:
        return {"ok": True, "found": False}
    record = records[0]
    task_uuid = record.get("uuid") or record.get("taskUuid")
    if not task_uuid:
        return {"ok": False, "error": "latest minutes item has no uuid", "found": False}
    detail, error = run_dws([
        "minutes", "get", "summary", "--id", str(task_uuid), "--format", "json",
    ])
    if error:
        return {
            "ok": False, "error": error, "found": True,
            "title": record.get("title") or "", "startTime": record.get("startTimeISO"),
        }
    value = result_of(detail)
    if isinstance(value, dict):
        summary: Any = value.get("fullSummary") or value.get("summary") or value.get("content") or value
    else:
        summary = value
    if not isinstance(summary, str):
        summary = json.dumps(summary, ensure_ascii=False, separators=(",", ":"))
    was_truncated = len(summary) > MAX_SUMMARY_CHARS
    return {
        "ok": True,
        "found": True,
        "title": record.get("title") or "",
        "startTime": record.get("startTimeISO") or record.get("startTime"),
        "summary": summary[:MAX_SUMMARY_CHARS],
        "summaryTruncated": was_truncated,
    }


def dry_run(day: date, tz: ZoneInfo, included: set[str]) -> None:
    start = datetime.combine(day, time.min, tzinfo=tz)
    end = start + timedelta(days=1)
    commands = []
    if "calendar" in included:
        commands.append(["dws", "calendar", "event", "list", "--start", start.isoformat(), "--end", end.isoformat(), "--format", "json"])
    if "todos" in included:
        commands.append(["dws", "todo", "task", "list", "--status", "false", "--page", "1", "--size", str(PAGE_SIZE), "--format", "json"])
    if "minutes" in included:
        commands.extend([
            ["dws", "minutes", "list", "all", "--limit", "1", "--format", "json"],
            ["dws", "minutes", "get", "summary", "--id", "<TASK_UUID>", "--format", "json"],
        ])
    print(json.dumps({"readOnly": True, "commands": commands}, ensure_ascii=False))


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--date", default=datetime.now().date().isoformat())
    parser.add_argument("--timezone", default="Asia/Shanghai")
    parser.add_argument(
        "--include",
        default="calendar,todos,minutes",
        help="comma-separated subset of calendar,todos,minutes",
    )
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()
    try:
        day = date.fromisoformat(args.date)
        tz = ZoneInfo(args.timezone)
    except (ValueError, KeyError) as exc:
        parser.error(str(exc))
    included = {item.strip() for item in args.include.split(",") if item.strip()}
    unknown = included - {"calendar", "todos", "minutes"}
    if not included or unknown:
        parser.error(f"invalid --include value: {args.include}")
    if args.dry_run:
        dry_run(day, tz, included)
        return 0
    with ThreadPoolExecutor(max_workers=3) as pool:
        result = {
            "date": day.isoformat(),
            "timezone": args.timezone,
        }
        futures = {}
        if "calendar" in included:
            futures["calendar"] = pool.submit(calendar_summary, day, tz)
        if "todos" in included:
            futures["todos"] = pool.submit(todo_summary, tz)
        if "minutes" in included:
            futures["minutes"] = pool.submit(minutes_summary)
        for name in ("calendar", "todos", "minutes"):
            if name in futures:
                result[name] = futures[name].result()
    print(json.dumps(result, ensure_ascii=False, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    sys.exit(main())
