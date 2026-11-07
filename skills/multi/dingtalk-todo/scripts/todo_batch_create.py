#!/usr/bin/env python3
"""Create up to 30 Todo tasks, preserve every task ID, and verify each task."""

import argparse
import hashlib
import json
import subprocess
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional


TIMEZONE = timezone(timedelta(hours=8), name="Asia/Shanghai")
MAX_ITEMS = 30
MAX_FILE_SIZE = 10 * 1024 * 1024
ALLOWED_PRIORITIES = {10, 20, 30, 40}
PLAN_DIGEST_DOMAIN = b"dws-todo-batch-plan-v1\x00"
MIN_EPOCH_MILLISECONDS = 1_000_000_000_000
MAX_EPOCH_MILLISECONDS = 9_999_999_999_999


class ScriptError(RuntimeError):
    def __init__(self, message: str, *, commit_unknown: bool = False):
        super().__init__(message)
        self.commit_unknown = commit_unknown


def run_dws_json(
    args: List[str], dws: str = "dws", *, write_started: bool = False
) -> Dict[str, Any]:
    try:
        result = subprocess.run(
            [dws, *args],
            capture_output=True,
            text=True,
            stdin=subprocess.DEVNULL,
            timeout=120,
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


def response_objects(value: Dict[str, Any], depth: int = 0) -> Iterable[Dict[str, Any]]:
    if depth > 4:
        return
    yield value
    for key in ("result", "data"):
        child = value.get(key)
        if isinstance(child, dict):
            yield from response_objects(child, depth + 1)


def first_string(payload: Dict[str, Any], keys: Iterable[str]) -> str:
    for obj in response_objects(payload):
        for key in keys:
            value = obj.get(key)
            if isinstance(value, str) and value.strip():
                return value.strip()
    return ""


def require_todo_detail(payload: Dict[str, Any]) -> Dict[str, Any]:
    for obj in response_objects(payload):
        for key in ("todoDetailModel", "todo", "task"):
            if key not in obj:
                continue
            detail = obj[key]
            if not isinstance(detail, dict):
                raise ScriptError(f"readback {key} is not an object")
            return detail
    raise ScriptError("readback response did not contain a Todo detail object")


def normalize_due(value: Any) -> Optional[str]:
    if value in (None, ""):
        return None
    raw = str(value).strip()
    if raw.isdigit():
        milliseconds = int(raw)
        if not MIN_EPOCH_MILLISECONDS <= milliseconds <= MAX_EPOCH_MILLISECONDS:
            raise ScriptError(
                "epoch-millisecond due time must be a 13-digit value "
                f"between {MIN_EPOCH_MILLISECONDS} and {MAX_EPOCH_MILLISECONDS}: "
                f"{raw}"
            )
        try:
            return datetime.fromtimestamp(milliseconds / 1000, TIMEZONE).isoformat()
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


def batch_plan_digest(items: List[Dict[str, Any]]) -> str:
    canonical = json.dumps(
        items,
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
        allow_nan=False,
    ).encode("utf-8")
    digest = hashlib.sha256(PLAN_DIGEST_DOMAIN + canonical).hexdigest()
    return f"sha256:{digest}"


def run(argv: Optional[List[str]] = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", type=Path)
    parser.add_argument("--dws", default="dws")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument(
        "--yes",
        action="store_true",
        help="confirm execution of the exact validated batch after user approval",
    )
    parser.add_argument(
        "--confirm-digest",
        help="planDigest from the user-approved dry-run output",
    )
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

    plan_digest = batch_plan_digest(items)
    if not args.dry_run and (not args.yes or not args.confirm_digest):
        print(
            json.dumps(
                {
                    "complete": False,
                    "dryRun": False,
                    "reason": "confirmation_required",
                    "executionStarted": False,
                    "error": (
                        "preview the exact batch with --dry-run; after the user "
                        "confirms that planDigest, rerun with --yes and "
                        "--confirm-digest <planDigest>"
                    ),
                },
                ensure_ascii=False,
            )
        )
        return 2
    if not args.dry_run and args.confirm_digest != plan_digest:
        print(
            json.dumps(
                {
                    "complete": False,
                    "dryRun": False,
                    "reason": "plan_mismatch",
                    "executionStarted": False,
                    "confirmedPlanDigest": args.confirm_digest,
                    "actualPlanDigest": plan_digest,
                    "error": (
                        "the validated batch differs from the confirmed dry-run; "
                        "preview again and obtain confirmation for the new planDigest"
                    ),
                },
                ensure_ascii=False,
            )
        )
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

        # The script-level confirmation covers this exact validated batch. Pass the
        # Runtime bypass only to the writes; readback remains an ordinary read.
        create.append("--yes")

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
                detail_object = require_todo_detail(detail)
                actual_identifier = first_string(
                    detail_object, ("taskId", "todoTaskId")
                )
                if actual_identifier != identifier:
                    raise ScriptError(
                        f"readback taskId mismatch: expected {identifier!r}, "
                        f"got {actual_identifier!r}"
                    )
                actual_title = first_string(detail_object, ("subject", "title"))
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
        "planDigest": plan_digest,
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
