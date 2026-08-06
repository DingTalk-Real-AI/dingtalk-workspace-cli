#!/usr/bin/env python3
"""Regression tests for the Doc Skill and its optional wrapper script."""

import contextlib
import importlib.util
import io
import json
import subprocess
import unittest
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[2]
SKILL_ROOT = ROOT / "skills" / "multi" / "dingtalk-doc"
SCRIPT_PATH = SKILL_ROOT / "scripts" / "doc_create_and_write.py"
MONO_DOC_ROOT = ROOT / "skills" / "mono" / "references" / "products" / "doc"
MONO_SCRIPT_PATH = ROOT / "skills" / "mono" / "scripts" / "doc_create_and_write.py"


def load_script():
    spec = importlib.util.spec_from_file_location("doc_create_and_write", SCRIPT_PATH)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load {SCRIPT_PATH}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class DocSkillAlignmentTest(unittest.TestCase):
    def test_shortcuts_use_progressive_discovery(self):
        skill = (SKILL_ROOT / "SKILL.md").read_text(encoding="utf-8")
        catalog = json.loads(
            (ROOT / "docs" / "shortcut-public-catalog.json").read_text(
                encoding="utf-8"
            )
        )
        runtime_shortcuts = [
            row for row in catalog["results"] if row.get("service") == "doc"
        ]
        schema = json.loads(
            (
                ROOT
                / "internal"
                / "cli"
                / "schema_catalog"
                / "tools"
                / "doc.json"
            ).read_text(encoding="utf-8")
        )
        schema_shortcuts = [
            path for path in schema["tools"] if path.startswith("doc.shortcut_")
        ]
        self.assertEqual(17, len(runtime_shortcuts))
        self.assertEqual(17, len(schema_shortcuts))
        self.assertIn("17 条公开 Shortcut，已全部进入 Runtime Schema", skill)
        self.assertIn(
            "dws shortcut list --service doc --compact --format json", skill
        )
        self.assertNotIn("| `dws doc +", skill)

    def test_native_pipeline_is_the_only_default_chunker(self):
        texts = [
            path.read_text(encoding="utf-8")
            for path in [SKILL_ROOT / "SKILL.md", *SKILL_ROOT.rglob("*.md")]
        ]
        combined = "\n".join(texts)
        self.assertNotIn(">30000", combined)
        self.assertNotIn("超过 30000", combined)
        self.assertNotIn(">200KB", combined)
        self.assertNotIn("超过 200KB", combined)
        self.assertNotIn("doc get", combined)

    def test_workflow_and_identity_rules_are_explicit(self):
        skill = (SKILL_ROOT / "SKILL.md").read_text(encoding="utf-8")
        create_refs = "\n".join(
            path.read_text(encoding="utf-8")
            for path in [
                SKILL_ROOT / "references" / "doc" / "doc-create.md",
                MONO_DOC_ROOT / "doc-create.md",
            ]
        )
        block_refs = "\n".join(
            path.read_text(encoding="utf-8")
            for path in [
                SKILL_ROOT / "references" / "doc" / "doc-block.md",
                MONO_DOC_ROOT / "doc-block.md",
            ]
        )
        import_refs = "\n".join(
            path.read_text(encoding="utf-8")
            for path in [
                SKILL_ROOT / "references" / "doc" / "doc-import.md",
                MONO_DOC_ROOT / "doc-import.md",
            ]
        )
        self.assertIn("不能覆盖用户显式要求的正文 H1", skill)
        self.assertIn("禁止先搜索同名文档", create_refs)
        self.assertIn("显式块操作不可折叠", block_refs)
        self.assertIn("list.isOrdered=true", block_refs)
        self.assertIn("在线编辑硬路由", import_refs)

    def test_schema_selection_preserves_doc_drive_boundaries(self):
        doc = json.loads(
            (
                ROOT / "internal" / "cli" / "schema_hints" / "selection" / "doc.json"
            ).read_text(encoding="utf-8")
        )["tools"]
        drive = json.loads(
            (
                ROOT
                / "internal"
                / "cli"
                / "schema_hints"
                / "selection"
                / "drive.json"
            ).read_text(encoding="utf-8")
        )["tools"]

        self.assertIn(
            "正文 H1",
            " ".join(doc["doc.create_document"]["use_when"]),
        )
        self.assertIn(
            "list.isOrdered=true",
            " ".join(doc["doc.insert_document_block"]["use_when"]),
        )
        self.assertIn(
            "dws doc import",
            " ".join(drive["drive.upload"]["avoid_when"]),
        )

    def test_mono_and_multi_wrappers_stay_aligned(self):
        self.assertEqual(
            SCRIPT_PATH.read_text(encoding="utf-8"),
            MONO_SCRIPT_PATH.read_text(encoding="utf-8"),
        )


class DocCreateAndWriteTest(unittest.TestCase):
    def setUp(self):
        self.module = load_script()

    def test_decode_json_output_accepts_progress_lines(self):
        data = self.module.decode_json_output(
            '[INFO] 写入分片 (1/2)\n{\n  "success": true,\n'
            '  "nodeId": "doc-1"\n}\n'
        )
        self.assertEqual("doc-1", data["nodeId"])

    def test_folder_and_workspace_are_mutually_exclusive(self):
        with self.assertRaises(SystemExit):
            self.module.run(
                [
                    "--name",
                    "周报",
                    "--content",
                    "hello",
                    "--folder",
                    "folder-1",
                    "--workspace",
                    "workspace-1",
                    "--dry-run",
                ]
            )

    def test_run_dws_rejects_nonzero_and_business_failure(self):
        with mock.patch.object(
            self.module.subprocess,
            "run",
            return_value=subprocess.CompletedProcess(
                ["dws"], 2, stdout="", stderr="unknown flag"
            ),
        ):
            with self.assertRaisesRegex(self.module.ScriptError, "unknown flag"):
                self.module.run_dws(["doc", "create"])

        with mock.patch.object(
            self.module.subprocess,
            "run",
            return_value=subprocess.CompletedProcess(
                ["dws"],
                0,
                stdout='{"success":false,"message":"denied"}',
                stderr="",
            ),
        ):
            with self.assertRaisesRegex(self.module.ScriptError, "denied"):
                self.module.run_dws(["doc", "create"])

    def test_wrapper_uses_create_then_info_and_read_without_manual_update(self):
        calls = []

        def fake_run(args, dry_run=False):
            calls.append(list(args))
            if args[:2] == ["doc", "create"]:
                return {"success": True, "nodeId": "doc-1", "chunksWritten": 2}
            if args[:2] == ["doc", "info"]:
                return {"success": True, "docUrl": "https://example.test/doc-1"}
            return {"success": True, "markdown": "hello"}

        stdout = io.StringIO()
        with mock.patch.object(self.module, "run_dws", side_effect=fake_run):
            with contextlib.redirect_stdout(stdout):
                code = self.module.run(["--name", "周报", "--content", "hello"])

        self.assertEqual(0, code)
        self.assertEqual(["create", "info", "read"], [call[1] for call in calls])
        self.assertFalse(any(call[:2] == ["doc", "update"] for call in calls))
        summary = json.loads(stdout.getvalue().splitlines()[-1])
        self.assertEqual("doc-1", summary["nodeId"])
        self.assertTrue(summary["verified"])

    def test_dry_run_shows_create_and_verification_commands(self):
        stdout = io.StringIO()
        with contextlib.redirect_stdout(stdout):
            code = self.module.run(
                ["--name", "周报", "--content", "hello", "--dry-run"]
            )
        self.assertEqual(0, code)
        output = stdout.getvalue()
        self.assertIn("dws doc create", output)
        self.assertIn("dws doc info", output)
        self.assertIn("dws doc read", output)


if __name__ == "__main__":
    unittest.main()
