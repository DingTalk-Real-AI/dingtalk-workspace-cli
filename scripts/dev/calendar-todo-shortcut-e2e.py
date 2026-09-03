#!/usr/bin/env python3
"""PII-safe live E2E for the public Calendar and Todo shortcut surfaces.

Raw command responses are kept in memory.  The script prints only stable
capability labels and reason codes; it never prints identities, resource IDs,
titles, room names, response bodies, traces, or local paths.
"""

from __future__ import annotations

import json
import os
import secrets
import subprocess
import sys
import tempfile
from datetime import datetime, timedelta
from pathlib import Path
from typing import Any, Callable


ROOT = Path(__file__).resolve().parents[2]
BIN = Path(os.environ.get("DWS_E2E_BIN", ROOT / "dws"))

CALENDAR_CASES = {
    "+agenda", "+attendee-list", "+book", "+book-list", "+book-search",
    "+cancel-event", "+conflicts", "+create", "+free", "+free-slots",
    "+freebusy", "+get", "+invite", "+my-free", "+next-event",
    "+reschedule", "+room-find", "+room-groups", "+room-search", "+rsvp",
    "+search-event", "+suggest-time", "+suggestion", "+today", "+tomorrow",
    "+update", "+week",
}
TODO_CASES = {
    "+assign", "+assign-multi", "+comment", "+complete", "+create",
    "+created-todos", "+due-today", "+get", "+get-my-tasks",
    "+get-related-tasks", "+list-attachment", "+list-comment", "+list-sub",
    "+overdue", "+remind", "+reminder", "+reopen", "+search",
    "+todo-done", "+update",
}


class E2EError(RuntimeError):
    def __init__(self, reason: str):
        super().__init__(reason)
        self.reason = reason


results: dict[str, tuple[str, str]] = {}


def record(product: str, command: str, status: str, reason: str = "") -> None:
    results[f"{product}/{command}"] = (status, reason)


def run_case(product: str, command: str, fn: Callable[[], None], *, blocked_ok: bool = False) -> None:
    try:
        fn()
    except E2EError as exc:
        record(product, command, "BLOCKED" if blocked_ok else "FAIL", exc.reason)
    except Exception:
        record(product, command, "FAIL", "unexpected_exception")
    else:
        record(product, command, "PASS")


def invoke(args: list[str], *, confirm: bool = False, expect_success: bool = True) -> Any:
    command = [str(BIN), *args]
    if "--format" not in command:
        command.extend(["--format", "json"])
    try:
        proc = subprocess.run(
            command,
            cwd=ROOT,
            input="yes\n" if confirm else "",
            text=True,
            capture_output=True,
            timeout=90,
            env={**os.environ, "NO_COLOR": "1"},
        )
    except subprocess.TimeoutExpired as exc:
        raise E2EError("command_timeout") from exc
    if expect_success and proc.returncode != 0:
        failure = None
        for raw in (proc.stdout, proc.stderr):
            try:
                candidate = json.loads(raw)
            except (json.JSONDecodeError, TypeError):
                continue
            if isinstance(candidate, dict) and isinstance(candidate.get("error"), dict):
                failure = candidate["error"]
                break
        if failure:
            code = failure.get("upstream_code") or failure.get("server_error_code")
            if isinstance(code, (str, int)) and str(code).isalnum():
                raise E2EError("upstream_" + str(code))
            subtype = failure.get("subtype") or failure.get("reason")
            if isinstance(subtype, str) and subtype.replace("_", "").isalnum():
                raise E2EError(subtype)
        raise E2EError("nonzero_exit")
    if not expect_success:
        if proc.returncode == 0:
            raise E2EError("unexpected_zero_exit")
        return None
    if not proc.stdout.strip():
        raise E2EError("empty_stdout")
    try:
        payload = json.loads(proc.stdout)
    except json.JSONDecodeError as exc:
        raise E2EError("malformed_json") from exc
    if isinstance(payload, dict) and "ok" in payload:
        if payload.get("ok") is not True or payload.get("outcome") != "success":
            raise E2EError("non_success_envelope")
        if "data" not in payload:
            raise E2EError("missing_data")
    return payload


def business(payload: Any) -> Any:
    if isinstance(payload, dict) and payload.get("ok") is True and "data" in payload:
        return payload["data"]
    return payload


def dictionaries(value: Any):
    if isinstance(value, dict):
        yield value
        for nested in value.values():
            yield from dictionaries(nested)
    elif isinstance(value, list):
        for nested in value:
            yield from dictionaries(nested)


def stable_string(value: Any, keys: tuple[str, ...]) -> str:
    for item in dictionaries(value):
        for key in keys:
            candidate = item.get(key)
            if isinstance(candidate, str) and candidate.strip():
                return candidate.strip()
    raise E2EError("missing_stable_id")


def explicit_list(value: Any, key: str) -> list[Any]:
    for item in dictionaries(value):
        if key not in item:
            continue
        candidate = item[key]
        if not isinstance(candidate, list):
            raise E2EError("malformed_collection")
        if any(not isinstance(entry, dict) or not entry for entry in candidate):
            raise E2EError("malformed_collection_item")
        return candidate
    raise E2EError("missing_collection")


def require_boolean(value: Any, key: str, expected: bool | None = None) -> bool:
    for item in dictionaries(value):
        if key in item:
            candidate = item[key]
            if not isinstance(candidate, bool):
                raise E2EError("malformed_boolean")
            if expected is not None and candidate is not expected:
                raise E2EError("boolean_mismatch")
            return candidate
    raise E2EError("missing_boolean")


def list_contains_id(value: Any, key: str, expected: str, id_keys: tuple[str, ...]) -> None:
    rows = explicit_list(business(value), key)
    for row in rows:
        for id_key in id_keys:
            if row.get(id_key) == expected:
                return
    raise E2EError("fixture_not_found")


def list_contains_text(value: Any, key: str, expected: str, text_keys: tuple[str, ...]) -> None:
    rows = explicit_list(business(value), key)
    for row in rows:
        if any(row.get(text_key) == expected for text_key in text_keys):
            return
    raise E2EError("fixture_not_found")


def iso(value: datetime) -> str:
    return value.isoformat(timespec="seconds")


def cleanup_calendar(event_ids: list[str]) -> int:
    failures = 0
    for event_id in reversed(event_ids):
        try:
            invoke(["calendar", "event", "delete", "--id", event_id, "--yes"])
        except E2EError:
            failures += 1
    return failures


def cleanup_todo(task_ids: list[str]) -> int:
    failures = 0
    for task_id in reversed(task_ids):
        try:
            invoke(["todo", "task", "delete", "--task-id", task_id, "--yes"])
        except E2EError:
            failures += 1
    return failures


def audit_cleanup(marker: str, start: datetime, end: datetime) -> None:
    cursor = ""
    for _ in range(20):
        args = [
            "calendar", "+agenda", "--start", iso(start), "--end", iso(end),
            "--limit", "100",
        ]
        if cursor:
            args.extend(["--cursor", cursor])
        payload = invoke(args)
        for event in explicit_list(business(payload), "events"):
            title = event.get("summary")
            status = str(event.get("status", "")).lower()
            if isinstance(title, str) and title.startswith(marker) and status not in {"cancelled", "canceled", "deleted"}:
                raise E2EError("calendar_cleanup_residual")
        pagination = (payload.get("meta") or {}).get("pagination") if isinstance(payload, dict) else None
        if not isinstance(pagination, dict):
            raise E2EError("cleanup_pagination_missing")
        if pagination.get("endpoint_exhausted") is True:
            break
        next_cursor = pagination.get("next_token")
        if not isinstance(next_cursor, str) or not next_cursor or next_cursor == cursor:
            raise E2EError("cleanup_pagination_stalled")
        cursor = next_cursor
    else:
        raise E2EError("cleanup_page_limit")

    todos = explicit_list(business(invoke(["todo", "+search", "--query", marker])), "todos")
    if any(isinstance(item.get("subject"), str) and item["subject"].startswith(marker) for item in todos):
        raise E2EError("todo_cleanup_residual")


def main() -> int:
    if not BIN.is_file():
        print("FAIL preflight binary_missing")
        return 1

    marker = "DWS-E2E-" + secrets.token_hex(6).upper()
    profile = business(invoke(["contact", "+me"]))
    self_id = stable_string(profile, ("userId",))
    self_name = stable_string(profile, ("name",))

    now = datetime.now().astimezone().replace(microsecond=0)
    event_start = now + timedelta(minutes=45)
    event_end = event_start + timedelta(minutes=40)
    overlap_start = event_start + timedelta(minutes=10)
    overlap_end = event_end + timedelta(minutes=10)
    tomorrow = (now + timedelta(days=1)).replace(hour=15, minute=0, second=0)
    tomorrow_end = tomorrow + timedelta(minutes=30)
    room_start = (now + timedelta(days=60)).replace(hour=15, minute=0, second=0)
    room_end = room_start + timedelta(minutes=30)
    today_start = now.replace(hour=0, minute=0, second=0)
    today_end = today_start + timedelta(days=1)
    week_end = now + timedelta(days=7)

    event_ids: list[str] = []
    task_ids: list[str] = []
    primary_event = ""
    overlap_event = ""
    book_event = ""
    primary_task = ""
    primary_task_title = marker + "-TODO-UPD"

    try:
        def create_primary_event() -> None:
            nonlocal primary_event
            payload = invoke([
                "calendar", "+create", "--title", marker + "-CAL-A",
                "--start", iso(event_start), "--end", iso(event_end),
                "--attendees", self_id,
            ], confirm=True)
            primary_event = stable_string(business(payload), ("eventId",))
            event_ids.append(primary_event)
            require_boolean(business(payload), "verified", True)
        run_case("calendar", "+create", create_primary_event)
        if not primary_event:
            raise E2EError("calendar_fixture_create_failed")

        def create_overlap() -> None:
            nonlocal overlap_event
            payload = invoke([
                "calendar", "+create", "--title", marker + "-CAL-B",
                "--start", iso(overlap_start), "--end", iso(overlap_end),
            ], confirm=True)
            overlap_event = stable_string(business(payload), ("eventId",))
            event_ids.append(overlap_event)
        create_overlap()

        run_case("calendar", "+get", lambda: stable_string(
            business(invoke(["calendar", "+get", "--event", primary_event])), ("eventId",)
        ))

        def update_event() -> None:
            payload = invoke([
                "calendar", "+update", "--event", primary_event,
                "--title", marker + "-CAL-A-UPD",
            ], confirm=True)
            require_boolean(business(payload), "verified", True)
        run_case("calendar", "+update", update_event)

        def agenda() -> None:
            payload = invoke([
                "calendar", "+agenda", "--start", iso(today_start),
                "--end", iso(today_end), "--limit", "100",
            ])
            list_contains_id(payload, "events", primary_event, ("eventId", "id"))
        run_case("calendar", "+agenda", agenda)

        def search_event() -> None:
            payload = invoke([
                "calendar", "+search-event", "--query", marker,
                "--start", iso(today_start), "--end", iso(today_end), "--limit", "100",
            ])
            list_contains_id(payload, "events", primary_event, ("eventId", "id"))
        run_case("calendar", "+search-event", search_event)

        def attendee_list() -> None:
            payload = invoke(["calendar", "+attendee-list", "--event", primary_event])
            rows = explicit_list(business(payload), "attendees")
            if not any(row.get("self") is True for row in rows):
                raise E2EError("self_attendee_not_found")
        run_case("calendar", "+attendee-list", attendee_list)

        def rsvp() -> None:
            payload = invoke([
                "calendar", "+rsvp", "--event", primary_event, "--status", "accept",
            ], confirm=True)
            require_boolean(business(payload), "success", True)
        run_case("calendar", "+rsvp", rsvp, blocked_ok=True)

        def invite() -> None:
            payload = invoke([
                "calendar", "+invite", "--event", overlap_event, "--with", self_name,
            ], confirm=True)
            require_boolean(business(payload), "verified", True)
        run_case("calendar", "+invite", invite)

        def conflicts() -> None:
            payload = invoke(["calendar", "+conflicts"])
            rows = explicit_list(business(payload), "conflicts")
            if not rows:
                raise E2EError("known_overlap_not_reported")
        run_case("calendar", "+conflicts", conflicts)

        run_case("calendar", "+today", lambda: list_contains_id(
            invoke(["calendar", "+today"]), "events", primary_event, ("eventId", "id")
        ))
        run_case("calendar", "+week", lambda: list_contains_id(
            invoke(["calendar", "+week"]), "events", primary_event, ("eventId", "id")
        ))

        def next_event() -> None:
            payload = business(invoke(["calendar", "+next-event"]))
            if not isinstance(payload, dict) or not isinstance(payload.get("event"), dict):
                raise E2EError("missing_next_event")
        run_case("calendar", "+next-event", next_event)

        def free_slots() -> None:
            payload = business(invoke(["calendar", "+free-slots", "--from", "0", "--to", "24"]))
            explicit_list(payload, "freeSlots")
            require_boolean(payload, "complete", True)
        run_case("calendar", "+free-slots", free_slots)

        def freebusy() -> None:
            payload = business(invoke([
                "calendar", "+freebusy", "--users", self_id,
                "--start", iso(event_start - timedelta(minutes=5)),
                "--end", iso(event_end + timedelta(minutes=5)),
            ]))
            explicit_list(payload, "busy")
        run_case("calendar", "+freebusy", freebusy)

        run_case("calendar", "+free", lambda: explicit_list(business(invoke([
            "calendar", "+free", "--who", self_name,
            "--start", iso(event_start - timedelta(minutes=5)),
            "--end", iso(event_end + timedelta(minutes=5)),
        ])), "busy"))

        def my_free() -> None:
            payload = business(invoke([
                "calendar", "+my-free", "--start", iso(event_start - timedelta(minutes=5)),
                "--end", iso(event_end + timedelta(minutes=5)),
            ]))
            require_boolean(payload, "free")
        run_case("calendar", "+my-free", my_free)

        run_case("calendar", "+suggestion", lambda: explicit_list(business(invoke([
            "calendar", "+suggestion", "--users", self_id, "--duration", "30",
            "--start", iso(room_start), "--end", iso(room_start + timedelta(hours=4)),
        ])), "suggestions"))
        run_case("calendar", "+suggest-time", lambda: explicit_list(business(invoke([
            "calendar", "+suggest-time", "--with", self_name, "--duration", "30",
            "--start", iso(room_start), "--end", iso(room_start + timedelta(hours=4)),
        ])), "suggestions"))

        def book() -> None:
            nonlocal book_event
            payload = invoke([
                "calendar", "+book", "--title", marker + "-CAL-TOMORROW",
                "--start", iso(tomorrow), "--end", iso(tomorrow_end), "--with", self_name,
            ], confirm=True)
            book_event = stable_string(business(payload), ("eventId", "id"))
            event_ids.append(book_event)
            require_boolean(business(payload), "verified", True)
        run_case("calendar", "+book", book)
        if book_event:
            run_case("calendar", "+tomorrow", lambda: list_contains_id(
                invoke(["calendar", "+tomorrow"]), "events", book_event, ("eventId", "id")
            ))

            def reschedule() -> None:
                payload = invoke([
                    "calendar", "+reschedule", "--event", book_event,
                    "--start", iso(tomorrow + timedelta(hours=1)),
                    "--end", iso(tomorrow_end + timedelta(hours=1)),
                ], confirm=True)
                require_boolean(business(payload), "verified", True)
            run_case("calendar", "+reschedule", reschedule)

            def cancel_event() -> None:
                payload = business(invoke(["calendar", "+cancel-event", "--event", book_event], confirm=True))
                require_boolean(payload, "verified", True)
                tombstone = business(invoke(["calendar", "+get", "--event", book_event]))
                status = stable_string(tombstone, ("status", "eventStatus", "event_status")).lower()
                if status not in {"cancelled", "canceled", "deleted"}:
                    raise E2EError("delete_tombstone_missing")
                event_ids.remove(book_event)
            run_case("calendar", "+cancel-event", cancel_event)
        else:
            for command in ("+tomorrow", "+reschedule", "+cancel-event"):
                record("calendar", command, "BLOCKED", "book_fixture_unavailable")

        run_case("calendar", "+room-groups", lambda: explicit_list(
            business(invoke(["calendar", "+room-groups", "--page", "0", "--limit", "20"])),
            "groups",
        ), blocked_ok=True)

        room_payload: Any = None
        def room_find() -> None:
            nonlocal room_payload
            room_payload = invoke([
                "calendar", "+room-find", "--start", iso(room_start), "--end", iso(room_end),
                "--page", "0", "--limit", "100",
            ])
            rows = explicit_list(business(room_payload), "rooms")
            if not rows:
                raise E2EError("safe_room_fixture_empty")
        run_case("calendar", "+room-find", room_find, blocked_ok=True)

        room_name = ""
        if room_payload is not None:
            try:
                room_name = stable_string(explicit_list(business(room_payload), "rooms"), ("roomName", "name"))
            except E2EError:
                room_name = ""
        if room_name:
            run_case("calendar", "+room-search", lambda: explicit_list(
                business(invoke(["calendar", "+room-search", "--room-name", room_name])), "rooms"
            ), blocked_ok=True)
        else:
            try:
                explicit_list(
                    business(invoke(["calendar", "+room-search", "--room-name", marker])),
                    "rooms",
                )
            except E2EError as exc:
                record("calendar", "+room-search", "FAIL", exc.reason)
            else:
                record("calendar", "+room-search", "BLOCKED", "nonempty_room_fixture_unavailable")

        def book_list() -> None:
            payload = business(invoke(["calendar", "+book-list"]))
            rows = explicit_list(payload, "calendars")
            if not rows:
                raise E2EError("calendar_book_fixture_empty")
        run_case("calendar", "+book-list", book_list)

        book_list_payload = invoke(["calendar", "+book-list"])
        try:
            book_name = stable_string(explicit_list(business(book_list_payload), "calendars"), ("name", "summary", "title"))
            run_case("calendar", "+book-search", lambda: explicit_list(
                business(invoke(["calendar", "+book-search", "--query", book_name])), "calendars"
            ))
        except E2EError as exc:
            record("calendar", "+book-search", "FAIL", exc.reason)

        def create_primary_todo() -> None:
            nonlocal primary_task
            payload = invoke([
                "todo", "+create", "--title", marker + "-TODO",
                "--executors", self_id, "--due", iso(now + timedelta(hours=3)),
                "--priority", "20",
            ], confirm=True)
            primary_task = stable_string(business(payload), ("taskId",))
            task_ids.append(primary_task)
            require_boolean(business(payload), "verified", True)
        run_case("todo", "+create", create_primary_todo)
        if not primary_task:
            raise E2EError("todo_fixture_create_failed")

        run_case("todo", "+get", lambda: stable_string(
            business(invoke(["todo", "+get", "--task-id", primary_task])), ("taskId",)
        ))

        def update_todo() -> None:
            payload = invoke([
                "todo", "+update", "--task-id", primary_task,
                "--title", primary_task_title, "--priority", "30",
            ], confirm=True)
            require_boolean(business(payload), "verified", True)
        run_case("todo", "+update", update_todo)

        run_case("todo", "+get-my-tasks", lambda: list_contains_id(
            invoke(["todo", "+get-my-tasks", "--all", "--max-pages", "40"]),
            "todos", primary_task, ("taskId", "id")
        ))
        run_case("todo", "+get-related-tasks", lambda: list_contains_id(
            invoke(["todo", "+get-related-tasks"]), "tasks", primary_task, ("taskId", "id")
        ))
        run_case("todo", "+search", lambda: list_contains_id(
            invoke(["todo", "+search", "--query", marker]), "todos", primary_task, ("taskId", "id")
        ))
        run_case("todo", "+created-todos", lambda: list_contains_id(
            invoke(["todo", "+created-todos"]), "created", primary_task, ("taskId", "id")
        ))
        run_case("todo", "+due-today", lambda: list_contains_id(
            invoke(["todo", "+due-today"]), "tasks", primary_task, ("taskId", "id")
        ))

        def add_comment() -> None:
            payload = invoke([
                "todo", "+comment", "--task-id", primary_task, "--content", marker + "-COMMENT",
            ], confirm=True)
            require_boolean(business(payload), "verified", True)
        run_case("todo", "+comment", add_comment)
        run_case("todo", "+list-comment", lambda: explicit_list(
            business(invoke(["todo", "+list-comment", "--task-id", primary_task])), "comments"
        ))

        def reminder() -> None:
            first = invoke([
                "todo", "+reminder", "--task-id", primary_task,
                "--base-time", "dueTime", "--due-date-offset", "-30",
            ], confirm=True)
            require_boolean(business(first), "terminalReceipt", True)
            require_boolean(business(first), "verified", False)
            second = invoke([
                "todo", "+reminder", "--task-id", primary_task, "--clear",
            ], confirm=True)
            require_boolean(business(second), "terminalReceipt", True)
        run_case("todo", "+reminder", reminder)

        def create_sub_fixture() -> None:
            payload = invoke([
                "todo", "task", "create-sub", "--parent-id", primary_task,
                "--title", marker + "-SUB", "--executors", self_id,
            ], confirm=True)
            task_ids.append(stable_string(business(payload), ("taskId", "id")))
        create_sub_fixture()
        run_case("todo", "+list-sub", lambda: explicit_list(
            business(invoke(["todo", "+list-sub", "--task-id", primary_task])), "subTasks"
        ))

        with tempfile.TemporaryDirectory(prefix="dws-ct-e2e-") as tmp:
            attachment = Path(tmp) / "fixture.txt"
            attachment.write_text(marker + "\n", encoding="utf-8")
            invoke([
                "todo", "task", "add-attachment", "--task-id", primary_task,
                "--file-path", str(attachment),
            ], confirm=True)
            run_case("todo", "+list-attachment", lambda: explicit_list(
                business(invoke(["todo", "+list-attachment", "--task-id", primary_task])), "attachments"
            ))

        def complete() -> None:
            payload = invoke(["todo", "+complete", "--task-id", primary_task], confirm=True)
            require_boolean(business(payload), "verified", True)
            require_boolean(business(payload), "isDone", True)
        run_case("todo", "+complete", complete)

        def reopen() -> None:
            payload = invoke(["todo", "+reopen", "--task-id", primary_task], confirm=True)
            require_boolean(business(payload), "verified", True)
            require_boolean(business(payload), "isDone", False)
        run_case("todo", "+reopen", reopen)

        def assigned(command: str, args: list[str]) -> None:
            payload = invoke(["todo", command, *args], confirm=True)
            task_id = stable_string(business(payload), ("taskId",))
            task_ids.append(task_id)
            require_boolean(business(payload), "verified", True)

        run_case("todo", "+assign", lambda: assigned(
            "+assign", ["--task", marker + "-ASSIGN", "--to", self_name, "--due", iso(now + timedelta(hours=4))]
        ))
        run_case("todo", "+assign-multi", lambda: assigned(
            "+assign-multi", ["--task", marker + "-ASSIGN-MULTI", "--to", self_name]
        ))
        run_case("todo", "+remind", lambda: assigned(
            "+remind", ["--task", marker + "-REMIND", "--at", iso(now + timedelta(hours=5))]
        ))

        overdue_task = stable_string(business(invoke([
            "todo", "+create", "--title", marker + "-OVERDUE", "--executors", self_id,
            "--due", iso(now - timedelta(hours=2)),
        ], confirm=True)), ("taskId",))
        task_ids.append(overdue_task)
        run_case("todo", "+overdue", lambda: list_contains_id(
            invoke(["todo", "+overdue"]), "overdue", overdue_task, ("taskId", "id")
        ))

        def todo_done() -> None:
            payload = invoke(["todo", "+todo-done", "--task", primary_task_title], confirm=True)
            require_boolean(business(payload), "verified", True)
        run_case("todo", "+todo-done", todo_done)

        invoke(["todo", "+upload-attachment", "--task-id", primary_task, "--file-path", "fixture"], expect_success=False)

    except E2EError as exc:
        record("preflight", "flow", "FAIL", exc.reason)
    finally:
        cleanup_failures = cleanup_calendar(event_ids) + cleanup_todo(task_ids)
        try:
            audit_cleanup(marker, now - timedelta(days=1), now + timedelta(days=70))
        except E2EError as exc:
            record("cleanup", "resource-audit", "FAIL", exc.reason)
        else:
            if cleanup_failures:
                record("cleanup", "resource-audit", "FAIL", "cleanup_command_failed")
            else:
                record("cleanup", "resource-audit", "PASS")

    for command in sorted(CALENDAR_CASES):
        results.setdefault(f"calendar/{command}", ("FAIL", "case_not_executed"))
    for command in sorted(TODO_CASES):
        results.setdefault(f"todo/{command}", ("FAIL", "case_not_executed"))

    failed = False
    for label in sorted(results):
        status, reason = results[label]
        if status == "FAIL":
            failed = True
        suffix = f" {reason}" if reason else ""
        print(f"{status} {label}{suffix}")
    counts = {status: sum(1 for value in results.values() if value[0] == status) for status in ("PASS", "BLOCKED", "FAIL")}
    print(f"SUMMARY pass={counts['PASS']} blocked={counts['BLOCKED']} fail={counts['FAIL']}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
