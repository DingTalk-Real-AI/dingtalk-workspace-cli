#!/usr/bin/env python3
"""Fast unit tests for schema_command_smoke.py (no dws binary required)."""

from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path
from unittest import mock


SCRIPT = Path(__file__).with_name("schema_command_smoke.py")
SPEC = importlib.util.spec_from_file_location("schema_command_smoke", SCRIPT)
if SPEC is None or SPEC.loader is None:  # pragma: no cover - import guard
    raise RuntimeError(f"cannot import {SCRIPT}")
smoke = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = smoke
SPEC.loader.exec_module(smoke)


class SchemaCommandSmokeTest(unittest.TestCase):
    def test_value_prefers_example_and_enum_before_numeric_heuristic(self) -> None:
        self.assertEqual(
            smoke.value_for(
                "mute-time",
                {"type": "integer", "example": 600000, "enum": [300000]},
                "chat.set_group_member_mute_list",
            ),
            "600000",
        )
        self.assertEqual(
            smoke.value_for(
                "mute-time",
                {"type": "integer", "enum": [300000, 600000]},
                "chat.set_group_member_mute_list",
            ),
            "300000",
        )

    def test_build_command_never_injects_yes_or_output_format(self) -> None:
        leaf = {
            "canonical_path": "sample.update",
            "cli_path": "sample update",
            "parameters": {
                "mode": {
                    "type": "string",
                    "required": True,
                    "enum": ["safe"],
                },
                "yes": {"type": "boolean", "required": True},
            },
            "positionals": [],
            "constraints": {},
            "dry_run": {"preview_kind": "plan"},
        }
        command = smoke.build_command("dws", leaf, include_optional=False)
        self.assertEqual(
            command,
            ["dws", "sample", "update", "--mode", "safe", "--dry-run"],
        )
        self.assertNotIn("--yes", command)
        self.assertNotIn("--format", command)

    def test_build_command_rejects_inherited_only_dry_run(self) -> None:
        leaf = {
            "canonical_path": "sample.read",
            "cli_path": "sample read",
            "parameters": {},
        }
        with self.assertRaisesRegex(ValueError, "explicit leaf dry_run"):
            smoke.build_command("dws", leaf, include_optional=False)

    def test_global_flag_presence_does_not_declare_capability(self) -> None:
        leaf = {
            "canonical_path": "sample.read",
            "cli_path": "sample read",
            "parameters": {},
        }
        self.assertFalse(smoke.declared_dry_run(leaf))
        with (
            mock.patch.object(smoke, "help_schema_flags", return_value=set()),
            mock.patch.object(smoke, "run_smoke_case") as run_case,
        ):
            results = smoke.run_one("dws", Path("."), leaf, 1, False)
        run_case.assert_not_called()
        self.assertEqual(len(results), 1)
        self.assertEqual(results[0].case_name, "contract")
        self.assertEqual(results[0].status, "contract_pass")

    def test_explicit_capability_adds_runtime_case_after_contract(self) -> None:
        leaf = {
            "canonical_path": "sample.plan",
            "cli_path": "sample plan",
            "parameters": {},
            "positionals": [],
            "constraints": {},
            "dry_run": {"preview_kind": "plan"},
        }
        runtime_result = smoke.SmokeResult(
            canonical_path="sample.plan",
            case_name="dry_run/default",
            cli_path="sample plan",
            command=["dws", "sample", "plan", "--dry-run"],
            status="runtime_exit_pass",
            exit_code=0,
            stdout="",
            stderr="",
        )
        with (
            mock.patch.object(smoke, "help_schema_flags", return_value=set()),
            mock.patch.object(
                smoke, "run_smoke_case", return_value=runtime_result
            ) as run_case,
        ):
            results = smoke.run_one("dws", Path("."), leaf, 1, False)
        run_case.assert_called_once()
        self.assertEqual(
            [(result.case_name, result.status) for result in results],
            [
                ("contract", "contract_pass"),
                ("dry_run/default", "runtime_exit_pass"),
            ],
        )

    def test_schema_help_mismatch_blocks_runtime(self) -> None:
        leaf = {
            "canonical_path": "sample.plan",
            "cli_path": "sample plan",
            "parameters": {"target": {"type": "string"}},
            "dry_run": {"preview_kind": "plan"},
        }
        with (
            mock.patch.object(smoke, "help_schema_flags", return_value=set()),
            mock.patch.object(smoke, "run_smoke_case") as run_case,
        ):
            results = smoke.run_one("dws", Path("."), leaf, 1, False)
        run_case.assert_not_called()
        self.assertEqual(results[0].status, "schema_flag_mismatch")
        self.assertIn("target", results[0].error)

    def test_help_schema_flag_missing_from_schema_blocks_runtime(self) -> None:
        leaf = {
            "canonical_path": "sample.plan",
            "cli_path": "sample plan",
            "parameters": {},
            "dry_run": {"preview_kind": "plan"},
        }
        with (
            mock.patch.object(
                smoke, "help_schema_flags", return_value={"new-local-flag"}
            ),
            mock.patch.object(smoke, "run_smoke_case") as run_case,
        ):
            results = smoke.run_one("dws", Path("."), leaf, 1, False)
        run_case.assert_not_called()
        self.assertEqual(results[0].status, "schema_flag_mismatch")
        self.assertIn("new-local-flag", results[0].error)

    def test_help_parser_keeps_effective_flags_and_ignores_root_controls(self) -> None:
        help_text = """Usage:
  dws sample run [flags]

Flags:
  -f, --format string   Business export format
      --json string     Complete business JSON payload
  -h, --help            help for run

Global Flags:
      --client-id string   OAuth client ID
      --debug              debug logging
      --dimension string   Product-level query dimension
      --dry-run            preview
      --fields string      output fields
      --jq string          output filter
      --keyword string     Product-level query keyword
      --mock               mock transport
      --profile string     profile
      --timeout int        timeout
  -v, --verbose            verbose
  -y, --yes                confirmation bypass
"""
        self.assertEqual(
            smoke.effective_schema_flags_from_help(help_text),
            {"format", "json", "dimension", "keyword"},
        )

    def test_help_parser_excludes_only_reviewed_generic_payload_usage(self) -> None:
        help_text = """Flags:
      --json string     Base JSON object payload for this tool invocation
      --params string   Additional JSON object payload merged after --json
      --payload string  Business payload
  -h, --help            help for run
"""
        self.assertEqual(
            smoke.effective_schema_flags_from_help(help_text), {"payload"}
        )

    def test_runtime_exit_zero_is_health_not_preview_proof(self) -> None:
        leaf = {
            "canonical_path": "sample.plan",
            "cli_path": "sample plan",
            "parameters": {},
            "positionals": [],
            "constraints": {},
            "dry_run": {"preview_kind": "plan"},
        }
        proc = mock.Mock(returncode=0, stdout="ordinary success", stderr="")
        with mock.patch.object(smoke.subprocess, "run", return_value=proc):
            result = smoke.run_smoke_case(
                "dws", Path("."), "sample.plan", leaf, 1, False, ()
            )
        self.assertEqual(result.status, "runtime_exit_pass")
        self.assertEqual(smoke.result_to_dict(result)["phase"], "runtime_exit_health")

    def test_inventory_is_loaded_once_and_paths_filter_full_leaves(self) -> None:
        listing = {
            "products": [
                {
                    "tools": [
                        {
                            "canonical_path": "sample.first",
                            "cli_path": "sample first",
                            "parameters": {},
                        },
                        {
                            "canonical_path": "sample.second",
                            "cli_path": "sample second",
                            "parameters": {"id": {"type": "string"}},
                        },
                    ]
                }
            ]
        }
        with mock.patch.object(smoke, "run_json", return_value=listing) as run_json:
            leaves = smoke.load_schema_leaves(
                "dws", Path("."), 1, requested_paths=["sample.second"]
            )
        run_json.assert_called_once_with(
            ["dws", "schema", "--all", "--format", "json"],
            Path("."),
            1,
            attempts=1,
        )
        self.assertEqual(
            [leaf["canonical_path"] for leaf in leaves], ["sample.second"]
        )
        self.assertIn("parameters", leaves[0])

    def test_inventory_rejects_summary_tools(self) -> None:
        listing = {
            "products": [
                {
                    "tools": [
                        {
                            "canonical_path": "sample.summary",
                            "cli_path": "sample summary",
                        }
                    ]
                }
            ]
        }
        with mock.patch.object(smoke, "run_json", return_value=listing):
            with self.assertRaisesRegex(RuntimeError, "summary, not a full leaf"):
                smoke.load_schema_leaves("dws", Path("."), 1)

    def test_inventory_rejects_non_object_parameters(self) -> None:
        for parameters in (None, []):
            with self.subTest(parameters=parameters):
                listing = {
                    "products": [
                        {
                            "tools": [
                                {
                                    "canonical_path": "sample.invalid",
                                    "cli_path": "sample invalid",
                                    "parameters": parameters,
                                }
                            ]
                        }
                    ]
                }
                with mock.patch.object(smoke, "run_json", return_value=listing):
                    with self.assertRaisesRegex(
                        RuntimeError, "parameters must be an object"
                    ):
                        smoke.load_schema_leaves("dws", Path("."), 1)

    def test_inventory_rejects_non_object_parameter_entry(self) -> None:
        listing = {
            "products": [
                {
                    "tools": [
                        {
                            "canonical_path": "sample.invalid",
                            "cli_path": "sample invalid",
                            "parameters": {"target": None},
                        }
                    ]
                }
            ]
        }
        with mock.patch.object(smoke, "run_json", return_value=listing):
            with self.assertRaisesRegex(
                RuntimeError, "parameter 'target' must be an object"
            ):
                smoke.load_schema_leaves("dws", Path("."), 1)

    def test_inventory_rejects_invalid_dry_run_shape_and_kind(self) -> None:
        invalid = [
            None,
            [],
            {},
            {"preview_kind": ""},
            {"preview_kind": " PLAN "},
            {"preview_kind": "unknown"},
            {"preview_kind": 1},
            {"preview_kind": "plan", "invented": True},
        ]
        for capability in invalid:
            with self.subTest(capability=capability):
                listing = {
                    "products": [
                        {
                            "tools": [
                                {
                                    "canonical_path": "sample.invalid",
                                    "cli_path": "sample invalid",
                                    "parameters": {},
                                    "dry_run": capability,
                                }
                            ]
                        }
                    ]
                }
                with mock.patch.object(smoke, "run_json", return_value=listing):
                    with self.assertRaisesRegex(RuntimeError, "invalid dry_run"):
                        smoke.load_schema_leaves("dws", Path("."), 1)

    def test_inventory_accepts_every_reviewed_dry_run_preview_kind(self) -> None:
        for preview_kind in sorted(smoke.DRY_RUN_PREVIEW_KINDS):
            with self.subTest(preview_kind=preview_kind):
                listing = {
                    "products": [
                        {
                            "tools": [
                                {
                                    "canonical_path": "sample.valid",
                                    "cli_path": "sample valid",
                                    "parameters": {},
                                    "dry_run": {"preview_kind": preview_kind},
                                }
                            ]
                        }
                    ]
                }
                with mock.patch.object(smoke, "run_json", return_value=listing):
                    leaves = smoke.load_schema_leaves("dws", Path("."), 1)
                self.assertTrue(smoke.declared_dry_run(leaves[0]))

        remote_reads_listing = {
            "products": [
                {
                    "tools": [
                        {
                            "canonical_path": "sample.remote-plan",
                            "cli_path": "sample remote-plan",
                            "parameters": {},
                            "dry_run": {
                                "preview_kind": "plan",
                                "remote_reads": True,
                            },
                        }
                    ]
                }
            ]
        }
        with mock.patch.object(smoke, "run_json", return_value=remote_reads_listing):
            leaves = smoke.load_schema_leaves("dws", Path("."), 1)
        self.assertTrue(smoke.declared_dry_run(leaves[0]))

    def test_inventory_rejects_non_boolean_dry_run_remote_reads(self) -> None:
        listing = {
            "products": [
                {
                    "tools": [
                        {
                            "canonical_path": "sample.invalid",
                            "cli_path": "sample invalid",
                            "parameters": {},
                            "dry_run": {
                                "preview_kind": "plan",
                                "remote_reads": "yes",
                            },
                        }
                    ]
                }
            ]
        }
        with mock.patch.object(smoke, "run_json", return_value=listing):
            with self.assertRaisesRegex(RuntimeError, "remote_reads must be a boolean"):
                smoke.load_schema_leaves("dws", Path("."), 1)


if __name__ == "__main__":
    unittest.main()
