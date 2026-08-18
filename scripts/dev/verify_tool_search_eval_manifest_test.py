#!/usr/bin/env python3
"""Unit tests for verify_tool_search_eval_manifest.py."""

from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


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

    def test_collecting_manifest_is_valid(self) -> None:
        self.assertEqual(verify.validate_manifest(verify.DEFAULT_MANIFEST), [])

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
