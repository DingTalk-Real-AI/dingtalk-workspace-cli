#!/usr/bin/env python3
"""Regression tests for the Todo Skill Golden Routes and deterministic scripts."""

import contextlib
import importlib.util
import io
import json
import re
import sys
import tempfile
import unittest
from datetime import timedelta
from pathlib import Path
from unittest import mock


sys.dont_write_bytecode = True


ROOT = Path(__file__).resolve().parents[2]
TODO_ROOT = ROOT / "skills" / "multi" / "dingtalk-todo"


def load_script(filename):
    path = TODO_ROOT / "scripts" / filename
    spec = importlib.util.spec_from_file_location(f"todo_test_{path.stem}", path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load {path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


DAILY = load_script("todo_daily_summary.py")
OVERDUE = load_script("todo_overdue_check.py")
BATCH = load_script("todo_batch_create.py")


class TodoSkillAlignmentTest(unittest.TestCase):
    def test_golden_routes_prefer_verified_shortcuts(self):
        skill = (TODO_ROOT / "SKILL.md").read_text(encoding="utf-8")
        for route in (
            "todo +remind",
            "todo +assign",
            "todo +create",
            "todo +get-my-tasks",
            "todo +search",
            "todo +complete",
            "todo +update",
            "todo +reminder",
        ):
            with self.subTest(route=route):
                self.assertIn(route, skill)
        self.assertIn("## Golden Routes", skill)
        self.assertIn("只有当前 leaf 的 flag 或安全语义确实不明时才查精确 leaf", skill)
        self.assertLessEqual(len(skill.encode("utf-8")), 16000)

    def test_composite_lifecycle_starts_with_atomic_create(self):
        skill = (TODO_ROOT / "SKILL.md").read_text(encoding="utf-8")
        lifecycle = (TODO_ROOT / "references" / "02-task.md").read_text(
            encoding="utf-8"
        )
        self.assertIn("组合生命周期从原子创建开始", skill)
        self.assertIn("不要用创建 Shortcut 代替第一步", skill)
        self.assertIn("dws todo task create --title", lifecycle)
        self.assertIn("dws contact user get-self --format json", lifecycle)
        self.assertIn(
            'task create-sub --parent-id <PARENT_ID> --title "<标题>" --executors <USER_ID>',
            lifecycle,
        )
        self.assertNotIn("dws contact me --format json", lifecycle)

    def test_step_routing_keeps_shortcuts_and_dynamic_id_boundaries(self):
        lifecycle = (TODO_ROOT / "references" / "02-task.md").read_text(
            encoding="utf-8"
        )
        for route in (
            "+get-my-tasks",
            "+get-related-tasks",
            "+due-today",
            "+overdue",
            "+search",
            "+get",
            "+complete",
            "+reopen",
            "+update",
            "+comment",
            "+reminder",
            "+list-sub",
            "+list-attachment",
        ):
            with self.subTest(route=route):
                self.assertIn(route, lifecycle)
        for stable_id in (
            "taskId",
            "commentId",
            "attachmentId",
            "tagCode",
            "userId",
        ):
            with self.subTest(stable_id=stable_id):
                self.assertIn(stable_id, lifecycle)
        self.assertIn("禁止运行 `git tag`", lifecycle)

    def test_golden_route_table_keeps_exactly_three_columns(self):
        skill = (TODO_ROOT / "SKILL.md").read_text(encoding="utf-8")
        table = skill.split("## Golden Routes", 1)[1].split("## 低频原子能力", 1)[0]
        for row in (line for line in table.splitlines() if line.startswith("|")):
            with self.subTest(row=row):
                self.assertEqual(5, len(re.split(r"(?<!\\)\|", row)), row)

    def test_all_markdown_tables_keep_consistent_column_counts(self):
        for document in TODO_ROOT.rglob("*.md"):
            expected_columns = None
            for line_number, line in enumerate(
                document.read_text(encoding="utf-8").splitlines(), start=1
            ):
                if line.startswith("|") and line.endswith("|"):
                    columns = len(re.split(r"(?<!\\)\|", line))
                    if expected_columns is None:
                        expected_columns = columns
                    with self.subTest(document=document.name, line=line_number):
                        self.assertEqual(expected_columns, columns, line)
                else:
                    expected_columns = None

    def test_references_are_todo_only_and_reminder_contract_is_consistent(self):
        combined = "\n".join(
            path.read_text(encoding="utf-8")
            for path in (TODO_ROOT / "references").rglob("*.md")
        )
        self.assertNotIn("## #7 听记与会后", combined)
        self.assertNotIn("minutes list all", combined)
        self.assertNotIn("当前不支持单独的 `reminder`", combined)
        self.assertIn("提醒写入目前没有对应的查询接口", combined)

    def test_markdown_links_resolve(self):
        missing = []
        pattern = re.compile(r"\[[^\]]+\]\(([^)#]+)(?:#[^)]+)?\)")
        for document in TODO_ROOT.rglob("*.md"):
            for target in pattern.findall(document.read_text(encoding="utf-8")):
                if "://" not in target and not (document.parent / target).resolve().exists():
                    missing.append(f"{document.relative_to(ROOT)} -> {target}")
        self.assertEqual([], missing)


class TodoDailySummaryTest(unittest.TestCase):
    def test_uses_bounded_shortcut_and_excludes_missing_due(self):
        start, _ = DAILY.date_range("today")
        inside = int((start + timedelta(hours=9)).timestamp() * 1000)
        outside = int((start + timedelta(days=2)).timestamp() * 1000)
        calls = []

        def fake_run(args, dws="dws"):
            calls.append(args)
            return {
                "ok": True,
                "outcome": "success",
                "data": {
                    "complete": True,
                    "count": 3,
                    "todos": [
                        {"taskId": "in", "subject": "inside", "dueTime": inside},
                        {"taskId": "none", "subject": "no due"},
                        {"taskId": "out", "subject": "outside", "dueTime": outside},
                    ],
                },
            }

        stdout = io.StringIO()
        with mock.patch.object(DAILY, "run_dws_json", side_effect=fake_run):
            with contextlib.redirect_stdout(stdout):
                code = DAILY.run(["today"])
        self.assertEqual(0, code)
        self.assertEqual(1, len(calls))
        self.assertEqual(["todo", "+get-my-tasks"], calls[0][:2])
        self.assertIn("--all", calls[0])
        self.assertIn("--plan-finish-start", calls[0])
        self.assertEqual(
            ["in"], [item["taskId"] for item in json.loads(stdout.getvalue())["todos"]]
        )

    def test_incomplete_traversal_fails_closed(self):
        payload = {"ok": True, "data": {"complete": False, "todos": []}}
        stdout = io.StringIO()
        with mock.patch.object(DAILY, "run_dws_json", return_value=payload):
            with contextlib.redirect_stdout(stdout):
                code = DAILY.run(["today"])
        self.assertEqual(2, code)
        self.assertFalse(json.loads(stdout.getvalue())["complete"])


class TodoOverdueTest(unittest.TestCase):
    def test_uses_overdue_shortcut_and_empty_is_success(self):
        calls = []

        def fake_run(args, dws="dws"):
            calls.append(args)
            return {"ok": True, "outcome": "success", "data": {"overdue": []}}

        stdout = io.StringIO()
        with mock.patch.object(OVERDUE, "run_dws_json", side_effect=fake_run):
            with contextlib.redirect_stdout(stdout):
                code = OVERDUE.run([])
        self.assertEqual(0, code)
        self.assertEqual(["todo", "+overdue", "--format", "json"], calls[0])
        self.assertEqual(0, json.loads(stdout.getvalue())["count"])


class TodoBatchCreateTest(unittest.TestCase):
    def test_batch_uses_iso_due_captures_id_and_reads_back(self):
        with tempfile.TemporaryDirectory() as raw:
            source = Path(raw) / "todos.json"
            source.write_text(
                json.dumps(
                    [
                        {
                            "title": "reviewed task",
                            "executors": "user1",
                            "priority": 40,
                            "due": "2026-08-18",
                        }
                    ]
                ),
                encoding="utf-8",
            )
            calls = []

            def fake_run(args, dws="dws", **kwargs):
                calls.append(args)
                if args[:3] == ["todo", "task", "create"]:
                    return {"result": {"taskId": "task-1"}}
                return {
                    "ok": True,
                    "data": {
                        "todoDetailModel": {
                            "taskId": "task-1",
                            "subject": "reviewed task",
                        }
                    },
                }

            stdout = io.StringIO()
            with mock.patch.object(BATCH, "run_dws_json", side_effect=fake_run):
                with contextlib.redirect_stdout(stdout):
                    code = BATCH.run([str(source)])
        self.assertEqual(0, code)
        self.assertEqual(2, len(calls))
        due = calls[0][calls[0].index("--due") + 1]
        self.assertIn("T23:59:59+08:00", due)
        self.assertEqual(
            ["todo", "task", "get", "--task-id", "task-1"], calls[1][:5]
        )
        payload = json.loads(stdout.getvalue())
        self.assertTrue(payload["complete"])
        self.assertEqual("verified", payload["ledger"][0]["status"])

    def test_possible_commit_is_preserved_as_unknown(self):
        with tempfile.TemporaryDirectory() as raw:
            source = Path(raw) / "todos.json"
            source.write_text(
                '[{"title":"x","executors":"u"}]', encoding="utf-8"
            )
            failure = BATCH.ScriptError("timeout", commit_unknown=True)
            stdout = io.StringIO()
            with mock.patch.object(BATCH, "run_dws_json", side_effect=failure):
                with contextlib.redirect_stdout(stdout):
                    code = BATCH.run([str(source)])
        self.assertEqual(2, code)
        payload = json.loads(stdout.getvalue())
        self.assertEqual(1, payload["unknownCount"])
        self.assertEqual("unknown", payload["ledger"][0]["status"])


if __name__ == "__main__":
    unittest.main()
