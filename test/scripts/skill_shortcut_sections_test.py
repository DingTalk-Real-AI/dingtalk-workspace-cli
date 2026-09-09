#!/usr/bin/env python3
"""Regression coverage for compatibility visibility in generated Skill discovery."""

import json
import sys
import unittest
from pathlib import Path
from unittest import mock

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts"))
import gen_skill_shortcut_sections as generator


class ChatCompatibilityCountsTest(unittest.TestCase):
    def test_visibility_and_availability_are_independent(self):
        source = {"shortcuts": {
            "public": {"disposition": "alias_internal", "public": True},
            "hidden": {"disposition": "alias_internal", "public": False},
            "cli_only": {"disposition": "alias_internal", "compatibility_visible": True},
            "unavailable": {"disposition": "alias_internal", "availability": "unavailable"},
            "deprecated": {"disposition": "alias_internal", "availability": "deprecated"},
            "canonical": {"disposition": "semantic_adapter", "public": True},
        }}
        self.assertEqual(generator.chat_compatibility_counts(source), {
            "public": 1, "hidden": 1, "compatibility_visible": 1,
        })

    def test_inherited_availability_and_explicit_override(self):
        source = {"default_availability": "unavailable", "shortcuts": {
            "inherited": {"disposition": "alias_internal", "public": False},
            "explicit": {"disposition": "alias_internal", "availability": "available"},
        }}
        self.assertEqual(generator.chat_compatibility_counts(source), {"hidden": 1})
        self.assertEqual(generator.chat_compatibility_counts({}), {})

    def test_hidden_migration_changes_only_hidden_count(self):
        source = json.loads(generator.CHAT_SEMANTIC_CATALOG.read_text(encoding="utf-8"))
        after = generator.chat_compatibility_counts(source)
        legacy = source["shortcuts"].pop("+active-conversations")
        self.assertFalse(legacy["public"])
        self.assertEqual(legacy["primary"], "+recent-conversations")
        before = generator.chat_compatibility_counts(source)
        self.assertEqual(after["public"], before["public"])
        self.assertEqual(after["compatibility_visible"], before["compatibility_visible"])
        self.assertEqual(after["hidden"], before["hidden"] + 1)

    def test_render_distinguishes_all_three_visibility_classes(self):
        source = {"shortcuts": {
            "+public": {"disposition": "alias_internal", "public": True},
            "+hidden": {"disposition": "alias_internal"},
            "+cli-only": {"disposition": "alias_internal", "compatibility_visible": True},
        }}
        fake_path = mock.Mock()
        fake_path.read_text.return_value = json.dumps(source)
        with mock.patch.object(generator, "CHAT_SEMANTIC_CATALOG", fake_path):
            text = generator.product_section("chat", [])
        self.assertIn("1 条 public 兼容入口", text)
        self.assertIn("1 条兼容入口仅 CLI 可见、不在 public Catalog", text)
        self.assertIn("1 条隐藏兼容入口仍可执行", text)
        self.assertNotIn("3 条 public", text)

    def test_committed_chat_discovery_matches_generation(self):
        path = generator.SERVICE_TO_SKILL["chat"]
        text = path.read_text(encoding="utf-8")
        updated = generator.replace_block(
            text, generator.PRODUCT_START, generator.PRODUCT_END,
            generator.product_section("chat", []), "## 意图表",
        )
        self.assertEqual(text, updated, "regenerate Skill sections after catalog changes")


if __name__ == "__main__":
    unittest.main()
