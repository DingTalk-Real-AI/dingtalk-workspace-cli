#!/usr/bin/env python3
"""Behavior regression cases for the calendar Skill's primary-calendar agenda."""

import contextlib
import importlib.util
import io
import json
import subprocess
import sys
import unittest
from datetime import datetime, timedelta, timezone
from pathlib import Path
from unittest import mock

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "skills/multi/dingtalk-calendar/scripts/calendar_today_agenda.py"
spec = importlib.util.spec_from_file_location("calendar_agenda", SCRIPT)
AGENDA = importlib.util.module_from_spec(spec)
spec.loader.exec_module(AGENDA)
WINDOW = ["--start", "2026-11-24T00:00:00+08:00", "--end", "2026-11-25T00:00:00+08:00"]


def book(cid, name=None, kind="primary", role="owner"):
    return {"calendarId": cid, "summary": name or cid, "type": kind, "privilege": role}


BOOKS = [book("primary", "我的日历（钉钉）"), book("opaque-mail-id", "我的日历（邮箱）")]


def event(eid="event-1", hour=9, status="confirmed", uid=None):
    value = {
        "id": eid, "summary": "会议 " + eid, "status": status,
        "start": {"dateTime": "2026-11-24T{:02d}:00:00+08:00".format(hour)},
        "end": {"dateTime": "2026-11-24T{:02d}:00:00+08:00".format(hour + 1)},
    }
    if uid:
        value["iCalUid"] = uid
    return value


def page(events, more=False, cursor=None):
    return {"events": events, "hasMore": more, "nextCursor": cursor}


def reply(result):
    return {"success": True, "result": result}


class CalendarAgendaTest(unittest.TestCase):
    def invoke(self, replies, args=None, output="json"):
        calls = []
        responses = iter(replies)

        def fake_run(cmd, **kwargs):
            calls.append(cmd)
            self.assertEqual(kwargs, {"capture_output": True, "text": True, "timeout": 60})
            response = next(responses)
            if isinstance(response, Exception):
                raise response
            if isinstance(response, subprocess.CompletedProcess):
                return response
            return subprocess.CompletedProcess(cmd, 0, json.dumps(response), "")

        stdout, stderr = io.StringIO(), io.StringIO()
        with mock.patch.object(AGENDA.subprocess, "run", side_effect=fake_run):
            with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
                code = AGENDA.run((WINDOW if args is None else args) + ["--format", output])
        result = json.loads(stdout.getvalue()) if output == "json" else stdout.getvalue()
        return code, result, calls

    def test_first_query_includes_mail_interview_and_excludes_other_calendars(self):
        other = [book("shared", kind="shared", role="reader"),
                 book("subscribed", kind="subscribed"),
                 book("team", kind="team")]
        ding = [event("d" + str(i), i + 8) for i in range(4)]
        ding += [event("cancel-1", 13, "cancelled"), event("cancel-2", 14, "canceled")]
        interview = event("interview", 16)
        interview["responseStatus"] = "accepted"
        code, result, calls = self.invoke([
            reply(BOOKS + other), reply(page(ding)), reply(page([interview])),
        ])
        self.assertEqual(code, 0)
        self.assertTrue(result["complete"])
        self.assertEqual(result["counts"], {"active": 5, "cancelled": 2, "unknown": 0, "total": 7})
        self.assertEqual(len(calls), 3)
        self.assertEqual(sum(cmd[1:4] == ["calendar", "book", "list"] for cmd in calls), 1)
        self.assertEqual([cmd[cmd.index("--calendar-id") + 1] for cmd in calls[1:]],
                         ["primary", "opaque-mail-id"])
        found = result["events"][-1]
        self.assertEqual(found["start"], "2026-11-24T16:00:00+08:00")
        self.assertEqual(found["sources"][0]["calendarId"], "opaque-mail-id")
        self.assertEqual(found["sources"][0]["eventId"], "interview")
        self.assertEqual(found["sources"][0]["responseStatus"], "accepted")

    def test_pagination_is_independent_and_each_book_is_discovered_once(self):
        code, result, calls = self.invoke([
            reply(BOOKS + [BOOKS[0]]),
            reply(page([event("d1")], True, "ding-next")),
            reply(page([event("d1"), event("d2", 10)])),
            reply(page([event("m1", 11)], True, "mail-next")),
            reply(page([event("m2", 12)])),
        ])
        self.assertEqual(code, 0)
        self.assertEqual(result["counts"]["total"], 4)
        self.assertEqual([b["pages"] for b in result["calendars"]], [2, 2])
        self.assertNotIn("--cursor", calls[1])
        self.assertIn("ding-next", calls[2])
        self.assertNotIn("--cursor", calls[3])
        self.assertIn("mail-next", calls[4])
        for cmd in calls[1:]:
            self.assertEqual(cmd[cmd.index("--limit") + 1], "100")

    def test_merge_requires_shared_uid_and_exact_instance_times(self):
        first = event("series", 9, uid="series-uid")
        second = event("series", 10, uid="series-uid")
        copy_in_mail = event("different-id", 9, uid="series-uid")
        copy_in_mail["start"]["dateTime"] = "2026-11-24T01:00:00Z"
        copy_in_mail["end"]["dateTime"] = "2026-11-24T02:00:00Z"
        unrelated = event("unrelated", 9)
        unrelated["summary"] = first["summary"]
        code, result, _ = self.invoke([
            reply(BOOKS), reply(page([first, second, first])),
            reply(page([copy_in_mail, unrelated])),
        ])
        self.assertEqual(code, 0)
        self.assertEqual(result["counts"]["total"], 3)
        merged = next(e for e in result["events"] if len(e["sources"]) == 2)
        self.assertEqual({s["eventId"] for s in merged["sources"]}, {"series", "different-id"})
        self.assertEqual(sum(e["iCalUid"] == "series-uid" for e in result["events"] if "iCalUid" in e), 2)

    def test_cross_calendar_status_conflict_preserves_both_sources(self):
        code, result, _ = self.invoke([
            reply(BOOKS),
            reply(page([event("d", uid="meeting")])),
            reply(page([event("m", status="cancelled", uid="meeting")])),
        ])
        self.assertEqual(code, 0)
        self.assertEqual(result["counts"]["unknown"], 1)
        merged = result["events"][0]
        self.assertEqual(merged["status"], "conflicting")
        self.assertEqual({s["status"] for s in merged["sources"]}, {"confirmed", "cancelled"})

    def test_failure_on_one_calendar_keeps_other_calendar_results(self):
        for broken_first in (True, False):
            with self.subTest(broken_first=broken_first):
                broken = subprocess.CompletedProcess([], 1, "", "permission denied")
                pages = [broken, reply(page([event("mail", 16)]))]
                if not broken_first:
                    pages.reverse()
                code, result, calls = self.invoke([reply(BOOKS)] + pages)
                self.assertEqual(code, 1)
                self.assertFalse(result["complete"])
                self.assertEqual(result["counts"]["total"], 1)
                self.assertEqual(len(calls), 3)
                self.assertEqual([b["complete"] for b in result["calendars"]],
                                 [not broken_first, broken_first])

    def test_only_ding_calendar_has_explicit_scope_note(self):
        code, result, _ = self.invoke([reply([BOOKS[0]]), reply(page([]))])
        self.assertEqual(code, 0)
        self.assertTrue(result["complete"])
        self.assertEqual(result["scope"], "discovered_own_primary_calendars")
        self.assertEqual(len(result["calendars"]), 1)
        self.assertTrue(any("邮箱" in note for note in result["notes"]))

    def test_no_eligible_calendar_is_not_a_successful_empty_agenda(self):
        for books in ([], [book("shared", kind="shared", role="reader")]):
            with self.subTest(books=books):
                code, result, calls = self.invoke([reply(books)])
                self.assertEqual(code, 1)
                self.assertFalse(result["complete"])
                self.assertEqual(len(calls), 1)
                self.assertTrue(result["failures"])

    def test_unknown_ownership_or_type_does_not_guess_by_name_or_id(self):
        for uncertain in (book("mail", "我的邮箱主日历", kind="unknown"),
                          book("mail", role=""), {"calendarId": "mail", "summary": "邮箱"},
                          {"summary": "我的日历", "type": "primary", "privilege": "owner"}):
            with self.subTest(uncertain=uncertain):
                code, result, calls = self.invoke([reply([BOOKS[0], uncertain]), reply(page([]))])
                self.assertEqual(code, 1)
                self.assertFalse(result["complete"])
                self.assertEqual(len(calls), 2)

    def test_conflicting_book_metadata_is_not_queried(self):
        code, result, calls = self.invoke([
            reply([BOOKS[0], book("primary", kind="shared", role="reader")]),
        ])
        self.assertEqual(code, 1)
        self.assertFalse(result["complete"])
        self.assertEqual(len(calls), 1)

    def test_explicit_calendar_scope_can_select_shared_calendar(self):
        shared = book("shared", kind="shared", role="reader")
        code, result, calls = self.invoke(
            [reply(BOOKS + [shared]), reply(page([event()]))],
            WINDOW + ["--calendar-id", "shared", "--calendar-id", "shared"],
        )
        self.assertEqual(code, 0)
        self.assertEqual(result["scope"], "specified_calendars")
        self.assertEqual(len(calls), 2)
        self.assertEqual(result["calendars"][0]["calendarId"], "shared")
        code, result, calls = self.invoke([reply(BOOKS)], WINDOW + ["--calendar-id", "missing"])
        self.assertEqual(code, 1)
        self.assertEqual(len(calls), 1)

    def test_discovery_failures_never_become_empty_success(self):
        errors = [
            {"success": False, "result": []}, {"result": []}, {},
            reply({}), reply(None),
            subprocess.CompletedProcess([], 0, "not json", ""),
            subprocess.CompletedProcess([], 1, "", "auth required"),
            FileNotFoundError("dws"), subprocess.TimeoutExpired("dws", 60),
        ]
        for response in errors:
            with self.subTest(response=repr(response)):
                code, result, calls = self.invoke([response])
                self.assertEqual(code, 1)
                self.assertFalse(result["complete"])
                self.assertTrue(result["failures"])
                self.assertEqual(len(calls), 1)

    def test_malformed_pages_and_rows_keep_valid_events_but_fail_completeness(self):
        cases = [
            {}, {"events": "bad", "hasMore": False},
            {"events": [event()], "hasMore": "false"},
            {"events": [event()], "hasMore": True, "nextCursor": ""},
            {"events": [event()], "hasMore": False, "nextCursor": "unexpected"},
            {"events": [event()], "hasMore": False, "nextCursor": 7},
            {"events": [event()]},
            page([{}, event()]), page([event(), {"id": "no-time"}]),
        ]
        for malformed in cases:
            with self.subTest(malformed=malformed):
                code, result, _ = self.invoke([reply([BOOKS[0]]), reply(malformed)])
                self.assertEqual(code, 1)
                self.assertFalse(result["complete"])
                self.assertTrue(result["failures"])
                if isinstance(malformed.get("events"), list) and event() in malformed["events"]:
                    self.assertEqual(result["counts"]["total"], 1)

    def test_cursor_cycle_stops_without_losing_successful_pages(self):
        code, result, calls = self.invoke([
            reply([BOOKS[0]]),
            reply(page([event("one")], True, "A")),
            reply(page([event("two", 10)], True, "B")),
            reply(page([event("three", 11)], True, "A")),
        ])
        self.assertEqual(code, 1)
        self.assertFalse(result["complete"])
        self.assertEqual(result["counts"]["total"], 3)
        self.assertEqual(len(calls), 4)

    def test_page_limit_is_partial_but_terminal_twentieth_page_is_complete(self):
        for terminal in (False, True):
            with self.subTest(terminal=terminal):
                replies = [reply([BOOKS[0]])]
                for index in range(20):
                    more = not (terminal and index == 19)
                    replies.append(reply(page([event(str(index))], more, str(index) if more else None)))
                code, result, calls = self.invoke(replies)
                self.assertEqual(code, 0 if terminal else 1)
                self.assertEqual(result["complete"], terminal)
                self.assertEqual(result["counts"]["total"], 20)
                self.assertEqual(len(calls), 21)

    def test_profile_is_identical_on_discovery_and_every_page(self):
        code, _, calls = self.invoke(
            [reply(BOOKS), reply(page([], True, "next")), reply(page([])), reply(page([]))],
            WINDOW + ["--profile", "corp:user"],
        )
        self.assertEqual(code, 0)
        for cmd in calls:
            self.assertEqual(cmd[:3], ["dws", "--profile", "corp:user"])

    def test_yesterday_and_week_use_beijing_midnights(self):
        now = datetime(2026, 11, 24, 17, 30, tzinfo=timezone.utc)
        start, end = AGENDA.get_range("yesterday", now)
        self.assertEqual(start.isoformat(), "2026-11-24T00:00:00+08:00")
        self.assertEqual(end.isoformat(), "2026-11-25T00:00:00+08:00")
        start, end = AGENDA.get_range("week", now)
        self.assertEqual(start.isoformat(), "2026-11-23T00:00:00+08:00")
        self.assertEqual(end - start, timedelta(days=7))

    def test_custom_window_preserves_instants_and_query_flags(self):
        args = ["--start", "2026-11-24T01:00:00Z", "--end", "2026-11-24T02:00:00Z"]
        code, result, calls = self.invoke([reply([BOOKS[0]]), reply(page([]))], args)
        self.assertEqual(code, 0)
        self.assertEqual(result["range"]["start"], "2026-11-24T09:00:00+08:00")
        self.assertIn("2026-11-24T10:00:00+08:00", calls[1])

    def test_invalid_arguments_fail_before_any_cli_request(self):
        cases = [
            ["--start", WINDOW[1]], ["--end", WINDOW[3]], ["yesterday"] + WINDOW,
            ["--start", "2026-11-24T09:00:00", "--end", WINDOW[3]],
            ["--start", WINDOW[3], "--end", WINDOW[1]],
            ["--start", "2026-11-24", "--end", WINDOW[3]],
            ["--profile", "corp:user,other:user"], ["--profile", ""],
            ["--calendar-id", ""],
            ["--start", "", "--end", ""],
        ]
        for args in cases:
            with self.subTest(args=args):
                with mock.patch.object(AGENDA.subprocess, "run") as child:
                    with contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit) as raised:
                        AGENDA.run(args)
                    self.assertEqual(raised.exception.code, 2)
                    child.assert_not_called()

    def test_dry_run_never_discovers_or_queries(self):
        code, result, calls = self.invoke([], ["yesterday", "--dry-run", "--profile", "corp:user"])
        self.assertEqual(code, 0)
        self.assertEqual(calls, [])
        self.assertTrue(result["dryRun"])
        self.assertIn("book", result["discoveryCommand"])
        self.assertIn("<calendarId-from-book-list>", result["eventCommandTemplate"])
        self.assertNotIn("complete", result)

    def test_all_day_and_unknown_status_are_preserved(self):
        all_day = event()
        all_day["start"] = {"date": "2026-11-24"}
        all_day["end"] = {"date": "2026-11-25"}
        all_day.pop("status")
        code, result, _ = self.invoke([reply([BOOKS[0]]), reply(page([all_day]))])
        self.assertEqual(code, 0)
        self.assertTrue(result["events"][0]["isAllDay"])
        self.assertEqual(result["counts"]["unknown"], 1)

    def test_range_is_half_open_and_keeps_overlapping_events(self):
        spanning = event("spanning")
        spanning["start"]["dateTime"] = "2026-11-23T23:30:00+08:00"
        spanning["end"]["dateTime"] = "2026-11-24T00:30:00+08:00"
        next_day = event("next-day")
        next_day["start"]["dateTime"] = "2026-11-25T00:00:00+08:00"
        next_day["end"]["dateTime"] = "2026-11-25T01:00:00+08:00"
        ended = event("ended")
        ended["start"]["dateTime"] = "2026-11-23T23:00:00+08:00"
        ended["end"]["dateTime"] = "2026-11-24T00:00:00+08:00"
        code, result, _ = self.invoke([
            reply([BOOKS[0]]), reply(page([spanning, next_day, ended])),
        ])
        self.assertEqual(code, 0)
        self.assertEqual([e["eventId"] for e in result["events"]], ["spanning"])

    def test_text_failure_does_not_announce_no_events(self):
        code, result, _ = self.invoke([{"success": False}], output="text")
        self.assertEqual(code, 1)
        self.assertNotIn("暂无日程", result)
        self.assertIn("不能判断是否有日程", result)
        self.assertNotIn("自由安排", result)

    def test_mono_and_multi_installations_receive_identical_script(self):
        mono = ROOT / "skills/mono/scripts/calendar_today_agenda.py"
        self.assertEqual(mono.read_bytes(), SCRIPT.read_bytes())


if __name__ == "__main__":
    unittest.main()
