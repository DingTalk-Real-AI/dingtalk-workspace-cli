#!/usr/bin/env python3
"""Behavior checks for the legacy AITable import compatibility wrapper."""

from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[2]
MONO_SCRIPT = ROOT / "skills" / "mono" / "scripts" / "aitable_import_via_task.py"
MULTI_SCRIPT = (
    ROOT
    / "skills"
    / "multi"
    / "dingtalk-aitable"
    / "scripts"
    / "aitable_import_via_task.py"
)
SPEC = importlib.util.spec_from_file_location("aitable_import_via_task", MONO_SCRIPT)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class AITableImportViaTaskTest(unittest.TestCase):
    def test_initial_import_delegates_to_canonical_shortcut(self) -> None:
        args = MODULE.build_parser().parse_args(
            [
                "base12345",
                "records.xlsx",
                "--table-id",
                "table12345",
                "--header-row",
                "2",
                "--field-mapping",
                '{"目标":"源列"}',
                "--timeout",
                "15",
                "--confirmed",
            ]
        )
        command = MODULE.build_dws_command(args)
        self.assertEqual(["dws", "aitable", "+import-file"], command[:3])
        self.assertIn("--base-id", command)
        self.assertIn("--file", command)
        self.assertIn("--yes", command)
        self.assertNotIn("+import-upload", command)
        self.assertNotIn("+import-data", command)

    def test_resume_delegates_with_same_import_id_only(self) -> None:
        args = MODULE.build_parser().parse_args(
            ["legacyBase", "--resume-import-id", "import123", "--confirmed"]
        )
        command = MODULE.build_dws_command(args)
        self.assertEqual(1, command.count("+import-file"))
        self.assertEqual("import123", command[command.index("--resume-import-id") + 1])
        self.assertNotIn("--base-id", command)
        self.assertNotIn("--file", command)

    def test_confirmation_and_initial_file_are_required(self) -> None:
        missing_confirmation = MODULE.build_parser().parse_args(
            ["base12345", "records.csv"]
        )
        with self.assertRaisesRegex(ValueError, "--confirmed"):
            MODULE.build_dws_command(missing_confirmation)

        missing_file = MODULE.build_parser().parse_args(
            ["base12345", "--confirmed"]
        )
        with self.assertRaisesRegex(ValueError, "文件路径"):
            MODULE.build_dws_command(missing_file)

    def test_main_replaces_process_without_parsing_dws_output(self) -> None:
        argv = [str(MONO_SCRIPT), "base12345", "records.csv", "--confirmed"]
        with mock.patch.object(sys, "argv", argv), mock.patch.object(
            MODULE.os, "execvp"
        ) as execvp:
            MODULE.main()
        command = execvp.call_args.args[1]
        self.assertEqual(command[0], execvp.call_args.args[0])
        self.assertEqual(["dws", "aitable", "+import-file"], command[:3])

    def test_mono_and_multi_wrappers_stay_identical(self) -> None:
        self.assertEqual(MONO_SCRIPT.read_bytes(), MULTI_SCRIPT.read_bytes())


if __name__ == "__main__":
    unittest.main()
