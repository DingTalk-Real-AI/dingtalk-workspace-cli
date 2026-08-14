#!/usr/bin/env python3

import importlib.util
import unittest
from pathlib import Path
from unittest.mock import patch
from zoneinfo import ZoneInfo


SCRIPT_PATH = Path(__file__).with_name("cross_product_read_summary.py")
SPEC = importlib.util.spec_from_file_location("cross_product_read_summary", SCRIPT_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC and SPEC.loader
SPEC.loader.exec_module(MODULE)


class CrossProductReadSummaryTest(unittest.TestCase):
    def test_todo_summary_is_compact_and_formats_semantics(self):
        payload = {
            "success": True,
            "result": {
                "todoCards": [
                    {
                        "subject": "later",
                        "priority": 20,
                        "dueTime": 1786096800000,
                        "createdTime": 2,
                    },
                    {
                        "subject": "urgent",
                        "priority": 40,
                        "dueTime": 1760371200000,
                        "createdTime": 1,
                    },
                ]
            },
        }
        with patch.object(MODULE, "run_dws", return_value=(payload, None)):
            result = MODULE.todo_summary(ZoneInfo("Asia/Shanghai"))

        self.assertTrue(result["ok"])
        self.assertEqual(result["count"], 2)
        self.assertEqual(result["items"][0]["title"], "urgent")
        self.assertEqual(result["items"][0]["priority"], "urgent")
        self.assertRegex(result["items"][0]["dueTime"], r"^\d{4}-\d{2}-\d{2}T")

    def test_minutes_uses_latest_item_then_fetches_summary(self):
        list_payload = {
            "success": True,
            "result": {
                "itemList": [
                    {"uuid": "real-id", "title": "latest", "startTimeISO": "2026-08-14T10:00:00+08:00"}
                ]
            },
        }
        summary_payload = {
            "success": True,
            "result": {"fullSummary": "real summary"},
        }
        with patch.object(
            MODULE, "run_dws", side_effect=[(list_payload, None), (summary_payload, None)]
        ) as mocked:
            result = MODULE.minutes_summary()

        self.assertTrue(result["ok"])
        self.assertTrue(result["found"])
        self.assertEqual(result["summary"], "real summary")
        self.assertIn("real-id", mocked.call_args_list[1].args[0])

    def test_dws_business_error_is_not_treated_as_success(self):
        completed = unittest.mock.Mock(
            returncode=0,
            stdout='{"success":false,"errorCode":"500","errorMsg":"failed"}',
            stderr="",
        )
        with patch.object(MODULE.subprocess, "run", return_value=completed):
            payload, error = MODULE.run_dws(["todo", "task", "list"])

        self.assertIsNone(payload)
        self.assertEqual(error, "failed")


if __name__ == "__main__":
    unittest.main()
