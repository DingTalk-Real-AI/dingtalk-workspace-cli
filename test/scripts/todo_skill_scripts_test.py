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
MONO_SCRIPTS = ROOT / "skills" / "mono" / "scripts"
TODO_SCRIPT_NAMES = (
    "todo_batch_create.py",
    "todo_daily_summary.py",
    "todo_overdue_check.py",
)


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
    def test_mono_and_multi_todo_scripts_stay_identical_and_stdlib_only(self):
        for filename in TODO_SCRIPT_NAMES:
            with self.subTest(filename=filename):
                multi = (TODO_ROOT / "scripts" / filename).read_text(encoding="utf-8")
                mono = (MONO_SCRIPTS / filename).read_text(encoding="utf-8")
                self.assertEqual(multi, mono)
                self.assertNotIn("zoneinfo", multi)
                self.assertNotIn("ZoneInfo", multi)

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
        for payload in (
            {"ok": True, "data": {"complete": False, "todos": []}},
            {"success": True, "result": {"todoCards": [], "hasMore": False}},
        ):
            with self.subTest(payload=payload):
                stdout = io.StringIO()
                with mock.patch.object(DAILY, "run_dws_json", return_value=payload):
                    with contextlib.redirect_stdout(stdout):
                        code = DAILY.run(["today"])
                self.assertEqual(2, code)
                self.assertFalse(json.loads(stdout.getvalue())["complete"])

    def test_missing_task_id_fails_closed(self):
        start, _ = DAILY.date_range("today")
        inside = int((start + timedelta(hours=9)).timestamp() * 1000)
        for task_id in (None, "", "   ", {"nested": "task-1"}):
            with self.subTest(task_id=task_id):
                item = {
                    "subject": "missing id",
                    "dueTime": inside,
                }
                if task_id is not None:
                    item["taskId"] = task_id
                payload = {
                    "ok": True,
                    "data": {"complete": True, "todos": [item]},
                }
                stdout = io.StringIO()
                with mock.patch.object(DAILY, "run_dws_json", return_value=payload):
                    with contextlib.redirect_stdout(stdout):
                        code = DAILY.run(["today"])
                result = json.loads(stdout.getvalue())
                self.assertEqual(2, code)
                self.assertFalse(result["complete"])
                self.assertIn("stable taskId", result["error"])


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

    def test_missing_or_malformed_task_id_fails_closed(self):
        for task_id in (None, "", "   ", {"nested": "task-1"}):
            with self.subTest(task_id=task_id):
                payload = {
                    "ok": True,
                    "data": {"overdue": [{"taskId": task_id, "dueTime": 1}]},
                }
                stdout = io.StringIO()
                with mock.patch.object(
                    OVERDUE, "run_dws_json", return_value=payload
                ):
                    with contextlib.redirect_stdout(stdout):
                        code = OVERDUE.run([])
                result = json.loads(stdout.getvalue())
                self.assertEqual(2, code)
                self.assertFalse(result["complete"])
                self.assertIn("taskId", result["error"])


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

            def fake_subprocess_run(argv, **kwargs):
                calls.append((argv, kwargs))
                if argv[1:4] == ["todo", "task", "create"]:
                    payload = {"result": {"taskId": "task-1"}}
                else:
                    payload = {
                        "ok": True,
                        "data": {
                            "todoDetailModel": {
                                "taskId": "task-1",
                                "subject": "reviewed task",
                            }
                        },
                    }
                return mock.Mock(
                    returncode=0,
                    stdout=json.dumps(payload),
                    stderr="",
                )

            stdout = io.StringIO()
            preview_stdout = io.StringIO()
            with mock.patch.object(BATCH.subprocess, "run") as preview_run_dws:
                with contextlib.redirect_stdout(preview_stdout):
                    preview_code = BATCH.run(
                        [str(source), "--dry-run", "--dws", "fake-dws"]
                    )
            self.assertEqual(0, preview_code)
            preview_run_dws.assert_not_called()
            plan_digest = json.loads(preview_stdout.getvalue())["planDigest"]

            with mock.patch.object(
                BATCH.subprocess, "run", side_effect=fake_subprocess_run
            ):
                with contextlib.redirect_stdout(stdout):
                    code = BATCH.run(
                        [
                            str(source),
                            "--yes",
                            "--confirm-digest",
                            plan_digest,
                            "--dws",
                            "fake-dws",
                        ]
                    )
        self.assertEqual(0, code)
        self.assertEqual(2, len(calls))
        self.assertEqual(
            [
                "fake-dws",
                "todo",
                "task",
                "create",
                "--title",
                "reviewed task",
                "--executors",
                "user1",
                "--priority",
                "40",
                "--due",
                "2026-08-18T23:59:59+08:00",
                "--format",
                "json",
                "--yes",
            ],
            calls[0][0],
        )
        self.assertEqual(
            [
                "fake-dws",
                "todo",
                "task",
                "get",
                "--task-id",
                "task-1",
                "--format",
                "json",
            ],
            calls[1][0],
        )
        for _, kwargs in calls:
            self.assertTrue(kwargs["capture_output"])
            self.assertTrue(kwargs["text"])
            self.assertIs(BATCH.subprocess.DEVNULL, kwargs["stdin"])
            self.assertEqual(120, kwargs["timeout"])
        payload = json.loads(stdout.getvalue())
        self.assertTrue(payload["complete"])
        self.assertEqual(plan_digest, payload["planDigest"])
        self.assertEqual("verified", payload["ledger"][0]["status"])

    def test_same_title_with_different_readback_id_is_unverified(self):
        with tempfile.TemporaryDirectory() as raw:
            source = Path(raw) / "todos.json"
            source.write_text(
                '[{"title":"same title","executors":"user1"}]', encoding="utf-8"
            )
            items = BATCH.validate(json.loads(source.read_text(encoding="utf-8")))
            plan_digest = BATCH.batch_plan_digest(items)
            responses = [
                {"result": {"taskId": "task-1"}},
                {
                    "taskId": "task-1",
                    "data": {
                        "todoDetailModel": {
                            "taskId": "task-2",
                            "subject": "same title",
                        }
                    }
                },
            ]
            stdout = io.StringIO()
            with mock.patch.object(BATCH, "run_dws_json", side_effect=responses):
                with contextlib.redirect_stdout(stdout):
                    code = BATCH.run(
                        [
                            str(source),
                            "--yes",
                            "--confirm-digest",
                            plan_digest,
                        ]
                    )
        payload = json.loads(stdout.getvalue())
        self.assertEqual(2, code)
        self.assertFalse(payload["complete"])
        self.assertEqual(1, payload["unverifiedCount"])
        self.assertEqual("task-1", payload["ledger"][0]["taskId"])
        self.assertEqual("unverified", payload["ledger"][0]["status"])
        self.assertIn("readback taskId mismatch", payload["ledger"][0]["error"])

    def test_batch_fails_closed_for_unprovable_create_and_readback_states(self):
        cases = (
            (
                "malformed-create-id",
                [{"result": {"taskId": {"nested": "task-1"}}}],
                "unknown",
            ),
            (
                "missing-detail",
                [{"result": {"taskId": "task-1"}}, {"data": {}}],
                "unverified",
            ),
            (
                "title-mismatch",
                [
                    {"result": {"taskId": "task-1"}},
                    {
                        "data": {
                            "todoDetailModel": {
                                "taskId": "task-1",
                                "subject": "wrong title",
                            }
                        }
                    },
                ],
                "unverified",
            ),
        )
        for name, responses, expected_status in cases:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as raw:
                source = Path(raw) / "todos.json"
                source.write_text(
                    '[{"title":"reviewed task","executors":"user1"}]',
                    encoding="utf-8",
                )
                items = BATCH.validate(
                    json.loads(source.read_text(encoding="utf-8"))
                )
                plan_digest = BATCH.batch_plan_digest(items)
                stdout = io.StringIO()
                with mock.patch.object(
                    BATCH, "run_dws_json", side_effect=responses
                ):
                    with contextlib.redirect_stdout(stdout):
                        code = BATCH.run(
                            [
                                str(source),
                                "--yes",
                                "--confirm-digest",
                                plan_digest,
                            ]
                        )
                payload = json.loads(stdout.getvalue())
                self.assertEqual(2, code)
                self.assertFalse(payload["complete"])
                self.assertEqual(expected_status, payload["ledger"][0]["status"])

    def test_unconfirmed_batch_stops_before_first_dws_call(self):
        with tempfile.TemporaryDirectory() as raw:
            source = Path(raw) / "todos.json"
            source.write_text(
                '[{"title":"reviewed task","executors":"user1"}]',
                encoding="utf-8",
            )
            stdout = io.StringIO()
            with mock.patch.object(BATCH.subprocess, "run") as run_dws:
                with contextlib.redirect_stdout(stdout):
                    code = BATCH.run([str(source)])
        self.assertEqual(2, code)
        run_dws.assert_not_called()
        payload = json.loads(stdout.getvalue())
        self.assertFalse(payload["complete"])
        self.assertEqual("confirmation_required", payload["reason"])
        self.assertFalse(payload["executionStarted"])

    def test_yes_without_confirmed_digest_stops_before_first_dws_call(self):
        with tempfile.TemporaryDirectory() as raw:
            source = Path(raw) / "todos.json"
            source.write_text(
                '[{"title":"reviewed task","executors":"user1"}]',
                encoding="utf-8",
            )
            stdout = io.StringIO()
            with mock.patch.object(BATCH.subprocess, "run") as run_dws:
                with contextlib.redirect_stdout(stdout):
                    code = BATCH.run([str(source), "--yes"])
        self.assertEqual(2, code)
        run_dws.assert_not_called()
        payload = json.loads(stdout.getvalue())
        self.assertEqual("confirmation_required", payload["reason"])
        self.assertFalse(payload["executionStarted"])

    def test_dry_run_previews_exact_batch_without_confirmation_bypass(self):
        with tempfile.TemporaryDirectory() as raw:
            source = Path(raw) / "todos.json"
            source.write_text(
                '[{"title":"reviewed task","executors":"user1"}]',
                encoding="utf-8",
            )
            stdout = io.StringIO()
            with mock.patch.object(BATCH.subprocess, "run") as run_dws:
                with contextlib.redirect_stdout(stdout):
                    code = BATCH.run([str(source), "--dry-run"])
        self.assertEqual(0, code)
        run_dws.assert_not_called()
        payload = json.loads(stdout.getvalue())
        self.assertTrue(payload["complete"])
        self.assertTrue(payload["dryRun"])
        self.assertRegex(payload["planDigest"], r"^sha256:[0-9a-f]{64}$")
        self.assertEqual(
            [
                "dws",
                "todo",
                "task",
                "create",
                "--title",
                "reviewed task",
                "--executors",
                "user1",
                "--format",
                "json",
            ],
            payload["ledger"][0]["command"],
        )
        self.assertNotIn("--yes", payload["ledger"][0]["command"])

    def test_plan_digest_is_stable_for_equivalent_validated_content(self):
        first = BATCH.validate(
            [
                {
                    "title": " reviewed task ",
                    "executors": "user1",
                    "priority": "40",
                    "due": "2026-08-18",
                }
            ]
        )
        second = BATCH.validate(
            [
                {
                    "due": "2026-08-18T23:59:59+08:00",
                    "priority": 40,
                    "executors": "user1",
                    "title": "reviewed task",
                }
            ]
        )
        self.assertEqual(
            BATCH.batch_plan_digest(first), BATCH.batch_plan_digest(second)
        )

    def test_changed_file_rejects_confirmed_digest_before_first_dws_call(self):
        with tempfile.TemporaryDirectory() as raw:
            source = Path(raw) / "todos.json"
            source.write_text(
                '[{"title":"reviewed task","executors":"user1"}]',
                encoding="utf-8",
            )
            preview_stdout = io.StringIO()
            with mock.patch.object(BATCH.subprocess, "run") as preview_run_dws:
                with contextlib.redirect_stdout(preview_stdout):
                    preview_code = BATCH.run([str(source), "--dry-run"])
            self.assertEqual(0, preview_code)
            preview_run_dws.assert_not_called()
            confirmed_digest = json.loads(preview_stdout.getvalue())["planDigest"]

            source.write_text(
                '[{"title":"changed task","executors":"user1"}]',
                encoding="utf-8",
            )
            stdout = io.StringIO()
            with mock.patch.object(BATCH.subprocess, "run") as run_dws:
                with contextlib.redirect_stdout(stdout):
                    code = BATCH.run(
                        [
                            str(source),
                            "--yes",
                            "--confirm-digest",
                            confirmed_digest,
                        ]
                    )
        self.assertEqual(2, code)
        run_dws.assert_not_called()
        payload = json.loads(stdout.getvalue())
        self.assertEqual("plan_mismatch", payload["reason"])
        self.assertFalse(payload["executionStarted"])
        self.assertEqual(confirmed_digest, payload["confirmedPlanDigest"])
        self.assertNotEqual(confirmed_digest, payload["actualPlanDigest"])

    def test_short_numeric_due_stops_before_first_dws_call(self):
        for due in ("0", "123", "2026", "0000000000123"):
            with self.subTest(due=due), tempfile.TemporaryDirectory() as raw:
                source = Path(raw) / "todos.json"
                source.write_text(
                    json.dumps(
                        [
                            {
                                "title": "must not be created",
                                "executors": "user1",
                                "due": due,
                            }
                        ]
                    ),
                    encoding="utf-8",
                )
                stdout = io.StringIO()
                with mock.patch.object(BATCH.subprocess, "run") as run_dws:
                    with contextlib.redirect_stdout(stdout):
                        code = BATCH.run(
                            [
                                str(source),
                                "--yes",
                                "--confirm-digest",
                                "sha256:" + "0" * 64,
                            ]
                        )
                self.assertEqual(2, code)
                run_dws.assert_not_called()
                payload = json.loads(stdout.getvalue())
                self.assertFalse(payload["complete"])
                self.assertIn("13-digit", payload["error"])

    def test_modern_epoch_millisecond_due_is_accepted(self):
        self.assertEqual(
            "2025-01-01T08:00:00+08:00",
            BATCH.normalize_due("1735689600000"),
        )

    def test_possible_commit_is_preserved_as_unknown(self):
        with tempfile.TemporaryDirectory() as raw:
            source = Path(raw) / "todos.json"
            source.write_text(
                '[{"title":"x","executors":"u"}]', encoding="utf-8"
            )
            failure = BATCH.ScriptError("timeout", commit_unknown=True)
            items = BATCH.validate(json.loads(source.read_text(encoding="utf-8")))
            plan_digest = BATCH.batch_plan_digest(items)
            stdout = io.StringIO()
            with mock.patch.object(BATCH, "run_dws_json", side_effect=failure):
                with contextlib.redirect_stdout(stdout):
                    code = BATCH.run(
                        [
                            str(source),
                            "--yes",
                            "--confirm-digest",
                            plan_digest,
                        ]
                    )
        self.assertEqual(2, code)
        payload = json.loads(stdout.getvalue())
        self.assertEqual(1, payload["unknownCount"])
        self.assertEqual("unknown", payload["ledger"][0]["status"])


if __name__ == "__main__":
    unittest.main()
