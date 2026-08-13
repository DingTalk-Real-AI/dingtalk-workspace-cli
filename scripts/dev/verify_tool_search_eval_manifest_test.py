#!/usr/bin/env python3
"""Unit tests for verify_tool_search_eval_manifest.py."""

from __future__ import annotations

import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path
from collections import Counter


SCRIPT = Path(__file__).with_name("verify_tool_search_eval_manifest.py")
SPEC = importlib.util.spec_from_file_location("verify_tool_search_eval_manifest", SCRIPT)
if SPEC is None or SPEC.loader is None:  # pragma: no cover
    raise RuntimeError(f"cannot import {SCRIPT}")
verify = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = verify
SPEC.loader.exec_module(verify)


class ToolSearchEvalManifestTest(unittest.TestCase):
    def test_language_slice(self) -> None:
        self.assertEqual(verify.language_slice("给群里发文件"), "chinese_only")
        self.assertEqual(
            verify.language_slice("读取 baseId 对应的表格"),
            "mixed_chinese_ascii",
        )
        self.assertEqual(verify.language_slice("send a file to the group"), "english")

    def test_go_algorithm_source_freeze_matches(self) -> None:
        manifest = verify.json.loads(verify.DEFAULT_MANIFEST.read_text(encoding="utf-8"))
        freeze = manifest["algorithm_freeze"]
        self.assertEqual(
            verify.sha256_source_set(freeze["go_source_paths"]),
            freeze["go_source_sha256"],
        )
        self.assertEqual(
            verify.sha256_source_set(freeze["gate_source_paths"]),
            freeze["gate_source_sha256"],
        )

    def test_collecting_manifest_is_valid_but_not_release_ready(self) -> None:
        self.assertEqual(verify.validate_manifest(verify.DEFAULT_MANIFEST, False), [])
        sealed_problems = verify.validate_manifest(verify.DEFAULT_MANIFEST, True)
        self.assertTrue(
            any("must be sealed" in problem for problem in sealed_problems),
            sealed_problems,
        )
        self.assertTrue(
            any("english has 0 cases" in problem for problem in sealed_problems),
            sealed_problems,
        )

    def test_graded_equivalent_qrels_are_accepted(self) -> None:
        catalog = {
            "dev.search_open_platform_docs_rag",
            "devdoc.search_open_platform_docs_rag",
        }
        case = {
            "id": "devdoc-search",
            "query": "查 OAuth2 接入文档",
            "language": "mixed_chinese_ascii",
            "qrels": [
                {
                    "canonical": "devdoc.search_open_platform_docs_rag",
                    "relevance": 3,
                },
                {
                    "canonical": "dev.search_open_platform_docs_rag",
                    "relevance": 2,
                },
            ],
        }
        problems: list[str] = []
        language, workflow = verify.validate_case(case, 0, catalog, problems)
        self.assertEqual(language, "mixed_chinese_ascii")
        self.assertFalse(workflow)
        self.assertEqual(problems, [])

    def test_forbidden_requires_alternative_gold(self) -> None:
        case = {
            "id": "unsafe",
            "query": "删除记录",
            "language": "chinese_only",
            "qrels": [{"canonical": "safe.read", "relevance": 3}],
            "forbidden": ["danger.delete"],
        }
        problems: list[str] = []
        verify.validate_case(case, 0, {"safe.read", "danger.delete"}, problems)
        self.assertTrue(any("alternative_gold" in problem for problem in problems))

    def test_forbidden_and_alternative_gold_are_catalog_bound_and_disjoint(self) -> None:
        case = {
            "id": "unsafe",
            "query": "删除记录",
            "language": "chinese_only",
            "qrels": [{"canonical": "safe.read", "relevance": 3}],
            "forbidden": ["danger.delete", "unknown.tool"],
            "alternative_gold": ["danger.delete"],
        }
        problems: list[str] = []
        verify.validate_case(case, 0, {"safe.read", "danger.delete"}, problems)
        self.assertTrue(any("unknown Catalog tool" in problem for problem in problems), problems)
        self.assertTrue(any("must be disjoint" in problem for problem in problems), problems)

    def test_independent_result_accepts_generator_envelope_payload(self) -> None:
        manifest = verify.json.loads(verify.DEFAULT_MANIFEST.read_text(encoding="utf-8"))
        report = {
            "version": "tool-search-independent-evaluation.v1",
            "catalog": {
                "source_hash": manifest["proxy_v1"]["catalog_source_hash"],
                "surface_hash": manifest["proxy_v1"]["catalog_surface_hash"],
            },
            "overall": {"cases": 3, "recall_at_5": 1.0},
            "control_overall": {"cases": 3, "recall_at_5": 0.9},
            "recall_at_5_delta": 0.1,
            "product_cluster_recall_at_5_delta_ci_95": {"lower": 0.05, "upper": 0.15},
            "language_slices": {
                name: {"cases": 1, "recall_at_5": 1.0}
                for name in ["chinese_only", "mixed_chinese_ascii", "english"]
            },
            "safety": {"forbidden_exposure_at_5": 0.0, "alternative_recall_at_5": 1.0, "sibling_exposure_at_5": 0.0},
            "workflow": {"cases": 1, "complete_at_5": 1.0, "required_recall_at_5": 1.0},
        }
        self.assertIs(verify.unwrap_independent_result({"independent": report}), report)
        problems: list[str] = []
        verify.validate_independent_result(
            report,
            manifest["independent_test_v1"],
            manifest["proxy_v1"],
            Counter(chinese_only=1, mixed_chinese_ascii=1, english=1, workflow=1),
            problems,
        )
        self.assertEqual(problems, [])

    def test_validate_manifest_unwraps_real_generator_file_shape(self) -> None:
        manifest = json.loads(verify.DEFAULT_MANIFEST.read_text(encoding="utf-8"))
        with tempfile.TemporaryDirectory() as directory:
            temporary = Path(directory)
            qrels = temporary / "qrels.json"
            qrels.write_text(json.dumps({"version": "tool-search-qrels.v1", "state": "collecting", "cases": []}), encoding="utf-8")
            manifest["independent_test_v1"]["path"] = str(qrels)
            manifest_path = temporary / "manifest.json"
            manifest_path.write_text(json.dumps(manifest), encoding="utf-8")
            result_path = temporary / "result.json"
            result_path.write_text(json.dumps({"diagnostic": {}, "independent": {"version": "wrong"}}), encoding="utf-8")
            problems = verify.validate_manifest(manifest_path, True, independent_result_path=result_path)
            self.assertTrue(any("result version" in problem for problem in problems), problems)

    def test_release_coverage_is_derived_not_self_asserted(self) -> None:
        metadata = {
            "chat.send": {"product_id": "chat", "effect": "write", "interface_ref": {"rpc_name": "send"}},
            "chat.delete": {"product_id": "chat", "effect": "destructive", "interface_ref": {"rpc_name": "delete"}},
            "doc.read": {"product_id": "doc", "effect": "read", "interface_ref": {"rpc_name": "get"}},
            "doc.fetch": {"product_id": "devdoc", "effect": "read", "interface_ref": {"rpc_name": "get"}},
        }
        cases = [
            {
                "qrels": [{"canonical": "chat.send", "relevance": 3}],
                "forbidden": ["chat.delete"],
                "alternative_gold": ["chat.send"],
                "confusion_family": ["chat.delete", "chat.send.other"],
                "workflow": {"required": ["chat.send", "doc.read"]},
            },
            {
                "qrels": [
                    {"canonical": "doc.read", "relevance": 3},
                    {"canonical": "doc.fetch", "relevance": 2},
                ]
            },
            {"qrels": [{"canonical": "chat.delete", "relevance": 3}]},
        ]
        problems: list[str] = []
        coverage = verify.derive_independent_coverage(cases, metadata, problems)
        self.assertIn("all_reviewed_products", coverage)
        self.assertIn("read_write_destructive", coverage)
        self.assertIn("graded_equivalent_entrypoints", coverage)
        self.assertNotIn("sibling_confusion", coverage)
        self.assertTrue(any("confusion_family" in problem for problem in problems))


if __name__ == "__main__":
    unittest.main()
