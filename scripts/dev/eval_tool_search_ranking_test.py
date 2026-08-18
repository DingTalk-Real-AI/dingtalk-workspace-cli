#!/usr/bin/env python3
"""Fast unit tests for eval_tool_search_ranking.py."""

from __future__ import annotations

import importlib.util
import json
import math
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("eval_tool_search_ranking.py")
SPEC = importlib.util.spec_from_file_location("eval_tool_search_ranking", SCRIPT)
if SPEC is None or SPEC.loader is None:  # pragma: no cover
    raise RuntimeError(f"cannot import {SCRIPT}")
ranking = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = ranking
SPEC.loader.exec_module(ranking)


class ToolSearchRankingEvalTest(unittest.TestCase):
    def tool(self, canonical: str, summary: str) -> ranking.ToolDocument:
        return ranking.ToolDocument(
            canonical=canonical,
            product=canonical.partition(".")[0],
            cli_path=canonical.replace(".", " "),
            identities=(canonical,),
            fields={
                "identity": canonical,
                "summary": summary,
                "description": "",
                "parameters": "",
            },
            use_when=(),
            avoid_when=(),
            examples=(),
        )

    def test_tokenize_preserves_identifier_and_splits_mixed_text(self) -> None:
        tokens = ranking.tokenize("chat.send_personal_message 给群聊发送FilePath")
        self.assertIn("chat.send_personal_message", tokens)
        self.assertIn("群聊", tokens)
        self.assertIn("file", tokens)
        self.assertIn("path", tokens)

    def test_parameter_projection_separates_names_descriptions_and_types(self) -> None:
        parameters = {
            "file": {
                "property": "filePath",
                "description": "本地文件路径",
                "interface_description": "上传的媒体文件",
                "type": "string",
                "interface_type": "path",
                "enum": ["document"],
            }
        }
        names = ranking.parameter_text(
            parameters,
            include_names=True,
            include_descriptions=False,
            include_types=False,
        )
        self.assertIn("filePath", names)
        self.assertIn("document", names)
        self.assertNotIn("本地文件路径", names)

        descriptions = ranking.parameter_text(
            parameters,
            include_names=False,
            include_descriptions=True,
            include_types=False,
        )
        self.assertIn("本地文件路径", descriptions)
        self.assertIn("上传的媒体文件", descriptions)
        self.assertNotIn("filePath", descriptions)

        types = ranking.parameter_text(
            parameters,
            include_names=False,
            include_descriptions=False,
            include_types=True,
        )
        self.assertEqual(types, "string path")

    def test_load_catalog_merges_build_time_product_shards(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            catalog_dir = Path(temporary)
            tools_dir = catalog_dir / "tools"
            tools_dir.mkdir()
            (catalog_dir / "catalog.json").write_text(
                json.dumps(
                    {
                        "source_hash": "sha256:source",
                        "surface_hash": "sha256:surface",
                        "catalog": {
                            "agent_metadata": {"surface_tools": 2}
                        },
                    }
                ),
                encoding="utf-8",
            )
            pairs = [("chat", "chat.send"), ("drive", "drive.get")]
            for product, canonical in pairs:
                (tools_dir / f"{product}.json").write_text(
                    json.dumps(
                        {
                            "product": product,
                            "tools": {
                                canonical: {
                                    "canonical_path": canonical,
                                    "cli_path": canonical.replace(".", " "),
                                    "agent_summary": canonical,
                                    "product_id": product,
                                }
                            },
                        }
                    ),
                    encoding="utf-8",
                )

            source_hash, tools = ranking.load_catalog(catalog_dir)

        self.assertEqual(source_hash, "sha256:source")
        self.assertEqual(
            [tool.canonical for tool in tools], ["chat.send", "drive.get"]
        )

    def test_load_catalog_rejects_duplicate_shard_tools(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            catalog_dir = Path(temporary)
            tools_dir = catalog_dir / "tools"
            tools_dir.mkdir()
            (catalog_dir / "catalog.json").write_text(
                json.dumps({"catalog": {"agent_metadata": {"surface_tools": 1}}}),
                encoding="utf-8",
            )
            shard = {"tools": {"chat.send": {"canonical_path": "chat.send"}}}
            for name in ["a.json", "b.json"]:
                (tools_dir / name).write_text(json.dumps(shard), encoding="utf-8")

            with self.assertRaisesRegex(ValueError, "duplicate tools"):
                ranking.load_catalog(catalog_dir)

    def test_bm25_ranks_relevant_tool_first(self) -> None:
        tools = [
            self.tool("chat.send", "发送群聊消息"),
            self.tool("drive.download", "下载钉盘文件"),
        ]
        for ranker_type in [
            ranking.BM25Ranker,
            ranking.BM25LRanker,
            ranking.BM25PlusRanker,
        ]:
            with self.subTest(ranker=ranker_type.__name__):
                ranker = ranker_type(tools)
                self.assertEqual(
                    ranker.rank_many(["群聊发消息"])["群聊发消息"][0],
                    "chat.send",
                )

    def test_vector_and_set_rankers_rank_relevant_chinese_first(self) -> None:
        tools = [
            self.tool("chat.send", "发送群聊消息"),
            self.tool("drive.download", "下载钉盘文件"),
        ]
        for ranker_type in [ranking.TfidfCosineRanker, ranking.WeightedJaccardRanker]:
            with self.subTest(ranker=ranker_type.__name__):
                ranker = ranker_type(tools)
                self.assertEqual(ranker.rank("群聊发消息")[0], "chat.send")

    def test_stable_rank_breaks_ties_by_canonical(self) -> None:
        self.assertEqual(
            ranking.stable_rank({"z.tool": 1.0, "a.tool": 1.0}),
            ["a.tool", "z.tool"],
        )

    def test_rrf_rewards_agreement(self) -> None:
        fused = ranking.rrf_rank(
            [["solo", "shared"], ["other", "shared"]]
        )
        self.assertEqual(fused[0], "shared")

    def test_rrf_k_changes_tail_influence_but_not_tie_break(self) -> None:
        rankings = [["a", "shared"], ["b", "shared"]]
        self.assertEqual(ranking.rrf_rank(rankings, k=10.0)[0], "shared")
        self.assertEqual(ranking.rrf_rank(rankings, k=60.0)[0], "shared")

    def test_forbidden_metric_is_lower_when_forbidden_tool_is_absent(self) -> None:
        cases = [ranking.SingleCase("query", "danger.delete", "danger")]
        metrics = ranking.evaluate_forbidden(cases, {"query": ["safe.read"]})
        self.assertEqual(metrics["forbidden_at_5"], 0.0)

    def test_workflow_decomposition_round_robins_steps(self) -> None:
        case = ranking.WorkflowCase(
            case_id="sample",
            query="do both",
            required=("first", "second"),
            subqueries=("step one", "step two"),
        )
        result = ranking.decompose_workflow_rankings(
            [case],
            {
                "step one": ["first", "shared"],
                "step two": ["second", "shared"],
            },
        )
        self.assertEqual(result["do both"][:2], ["first", "second"])

    def test_workflow_ndcg_rewards_required_tools_near_the_top(self) -> None:
        case = ranking.WorkflowCase(
            case_id="sample",
            query="do both",
            required=("first", "second"),
            subqueries=(),
        )
        best = ranking.evaluate_workflows(
            [case], {"do both": ["first", "second", "other"]}
        )
        delayed = ranking.evaluate_workflows(
            [case], {"do both": ["other", "first", "second"]}
        )
        self.assertEqual(best["mean_ndcg_at_5"], 1.0)
        self.assertGreater(
            best["mean_ndcg_at_5"], delayed["mean_ndcg_at_5"]
        )

    def test_query_language_slices(self) -> None:
        self.assertEqual(ranking.query_language_slice("给群里发文件"), "chinese_only")
        self.assertEqual(
            ranking.query_language_slice("读取 baseId 对应表格"),
            "mixed_chinese_ascii",
        )
        self.assertEqual(ranking.query_language_slice("chat send"), "non_chinese")

    def test_bm25_variant_formulas_match_hand_calculation(self) -> None:
        documents = {"doc": ["term", "term"], "other": ["other"]}
        length_ratio = 2.0 / 1.5
        idf = math.log(2.0)
        query_weight = math.log(2.0)

        plus = ranking.BM25VariantIndex(documents, variant="bm25plus")
        plus_contribution = 2.0 * 1.9 / (
            2.0 + 0.9 * (0.6 + 0.4 * length_ratio)
        ) + 1.0
        self.assertAlmostEqual(
            plus.scores("term")["doc"],
            idf * plus_contribution * query_weight,
        )

        bm25l = ranking.BM25VariantIndex(documents, variant="bm25l")
        normalized = 2.0 / (0.6 + 0.4 * length_ratio)
        bm25l_contribution = 1.9 * (normalized + 1.0) / (
            0.9 + normalized + 1.0
        )
        self.assertAlmostEqual(
            bm25l.scores("term")["doc"],
            idf * bm25l_contribution * query_weight,
        )

    def test_product_cluster_bootstrap_uses_equal_product_weight(self) -> None:
        cases = [
            ranking.SingleCase("a1", "a", "large"),
            ranking.SingleCase("a2", "a", "large"),
            ranking.SingleCase("a3", "a", "large"),
            ranking.SingleCase("b1", "b", "small"),
        ]
        result = ranking.paired_product_cluster_bootstrap_interval(
            cases,
            baseline=[False, False, False, True],
            candidate=[True, True, True, False],
            samples=200,
        )
        self.assertEqual(result["product_clusters"], 2)
        self.assertAlmostEqual(result["macro_product_recall_at_5_delta"], 0.0)

    def test_percentile_uses_nearest_rank(self) -> None:
        values = [5.0, 1.0, 4.0, 2.0, 3.0]
        self.assertEqual(ranking.percentile(values, 0.5), 3.0)
        self.assertEqual(ranking.percentile(values, 0.95), 5.0)


if __name__ == "__main__":
    unittest.main()
