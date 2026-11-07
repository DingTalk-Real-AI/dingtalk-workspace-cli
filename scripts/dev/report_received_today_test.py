#!/usr/bin/env python3
"""Focused regression tests for the packaged Report inbox helper."""

from __future__ import annotations

import importlib.util
import subprocess
import sys
import unittest
from datetime import datetime
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[2]
sys.dont_write_bytecode = True
MULTI_SCRIPT = (
    ROOT
    / "skills"
    / "multi"
    / "dingtalk-misc"
    / "scripts"
    / "report_received_today.py"
)
MONO_SCRIPT = ROOT / "skills" / "mono" / "scripts" / "report_received_today.py"


def load_report_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


REPORT = load_report_module(MULTI_SCRIPT, "report_received_today_multi")


def inbox_page(reports: list[dict], *, complete: bool, next_token: int = 0) -> dict:
    return {
        "ok": True,
        "outcome": "success",
        "data": {
            "reports": reports,
            "count": len(reports),
            "complete": complete,
        },
        "meta": {
            "pagination": {
                "endpoint_exhausted": complete,
                "next_token": next_token,
            }
        },
    }


class ReportReceivedTodayTest(unittest.TestCase):
    def test_mono_multi_scripts_are_identical_and_executable(self) -> None:
        self.assertEqual(MONO_SCRIPT.read_bytes(), MULTI_SCRIPT.read_bytes())
        self.assertNotEqual(MONO_SCRIPT.stat().st_mode & 0o111, 0)
        self.assertNotEqual(MULTI_SCRIPT.stat().st_mode & 0o111, 0)
        load_report_module(MONO_SCRIPT, "report_received_today_mono")

    def test_query_window_clamps_first_second_after_midnight(self) -> None:
        now = datetime(2026, 8, 26, 0, 0, 0, 500000, tzinfo=REPORT.SHANGHAI)
        start, end = REPORT.query_window(1, now)
        self.assertEqual(start.isoformat(), "2026-08-26T00:00:00+08:00")
        self.assertEqual(end.isoformat(), "2026-08-26T00:00:01+08:00")
        self.assertGreater(end, start)

    def test_query_window_freezes_at_current_whole_second(self) -> None:
        now = datetime(2026, 8, 26, 9, 8, 7, 654321, tzinfo=REPORT.SHANGHAI)
        start, end = REPORT.query_window(3, now)
        self.assertEqual(start.isoformat(), "2026-08-24T00:00:00+08:00")
        self.assertEqual(end.isoformat(), "2026-08-26T09:08:07+08:00")

    def test_format_create_time_uses_shanghai_timezone(self) -> None:
        self.assertEqual(
            REPORT.format_create_time(0), "1970-01-01 08:00:00 +0800"
        )
        self.assertEqual(REPORT.format_create_time("unknown"), "unknown")

    def test_nonzero_error_keeps_structured_error_and_stderr_bounded(self) -> None:
        result = subprocess.CompletedProcess(
            ["dws"],
            2,
            stdout='{"error":{"code":"permission_denied","message":"denied"}}',
            stderr="diagnostic " + "x" * 10000,
        )
        detail = REPORT.process_error_detail(result)
        self.assertIn("permission_denied", detail)
        self.assertIn("stderr=", detail)
        self.assertLessEqual(len(detail), REPORT.MAX_ERROR_DETAIL_CHARS)

    def test_scan_rejects_invalid_display_limit_before_calling_dws(self) -> None:
        with mock.patch.object(REPORT, "run_dws") as run_dws:
            with self.assertRaisesRegex(REPORT.ReportCommandError, "展示上限"):
                REPORT.scan_inbox(
                    REPORT.datetime.now(REPORT.SHANGHAI),
                    REPORT.datetime.now(REPORT.SHANGHAI),
                    1,
                    display_limit=0,
                )
        run_dws.assert_not_called()

    def test_scan_deduplicates_identical_page_overlap(self) -> None:
        pages = [
            inbox_page(
                [
                    {"reportId": "older", "createTime": 1},
                    {"reportId": "same", "createTime": 2},
                ],
                complete=False,
                next_token=20,
            ),
            inbox_page(
                [{"reportId": "same", "createTime": 2}],
                complete=True,
            ),
        ]
        calls: list[dict] = []

        def fake_run_dws(_args: list[str], **kwargs: object) -> dict:
            calls.append(kwargs)
            return pages.pop(0)

        with mock.patch.object(REPORT, "run_dws", side_effect=fake_run_dws):
            scan = REPORT.scan_inbox(
                REPORT.datetime.now(REPORT.SHANGHAI),
                REPORT.datetime.now(REPORT.SHANGHAI),
                2,
                display_limit=1,
            )
        self.assertEqual(scan.total_count, 2)
        self.assertEqual(
            [item["reportId"] for item in scan.visible_items], ["older"]
        )
        self.assertEqual(len(calls), 2)
        self.assertTrue(all(call["timeout_seconds"] <= 60 for call in calls))

    def test_scan_rejects_conflicting_duplicate(self) -> None:
        pages = [
            inbox_page(
                [{"reportId": "same", "createTime": 1}],
                complete=False,
                next_token=20,
            ),
            inbox_page(
                [{"reportId": "same", "createTime": 2}],
                complete=True,
            ),
        ]
        with mock.patch.object(REPORT, "run_dws", side_effect=pages):
            with self.assertRaisesRegex(REPORT.ReportCommandError, "createTime 冲突"):
                REPORT.scan_inbox(
                    REPORT.datetime.now(REPORT.SHANGHAI),
                    REPORT.datetime.now(REPORT.SHANGHAI),
                    2,
                )


if __name__ == "__main__":
    unittest.main(verbosity=2)
