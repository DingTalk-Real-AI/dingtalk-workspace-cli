#!/usr/bin/env python3
"""Verify frozen Tool Search evaluation inputs and independent qrels gates."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
from collections import Counter
from pathlib import Path
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[2]
DEFAULT_MANIFEST = REPO_ROOT / "scripts/testdata/tool_search_eval_manifest.json"
DEFAULT_CATALOG_DIR = REPO_ROOT / ".worktrees/policy-tmp/tool-search-schema-catalog"
CHINESE = re.compile(r"[\u3400-\u9fff]")
ASCII_WORD = re.compile(r"[A-Za-z0-9]")


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def sha256_source_set(paths: list[str]) -> str:
    """Hash path names and bytes so renames and edits both invalidate a freeze."""
    digest = hashlib.sha256()
    for relative in sorted(paths):
        target = REPO_ROOT / relative
        digest.update(relative.encode("utf-8"))
        digest.update(b"\0")
        digest.update(target.read_bytes())
        digest.update(b"\0")
    return digest.hexdigest()


def language_slice(query: str) -> str:
    has_chinese = bool(CHINESE.search(query))
    has_ascii = bool(ASCII_WORD.search(query))
    if has_chinese and has_ascii:
        return "mixed_chinese_ascii"
    if has_chinese:
        return "chinese_only"
    return "english"


def require_string(value: Any, field: str, problems: list[str]) -> str:
    if not isinstance(value, str) or not value.strip():
        problems.append(f"{field} must be a non-empty string")
        return ""
    return value.strip()


def validate_case(case: Any, index: int, catalog_tools: set[str], problems: list[str]) -> tuple[str, bool]:
    prefix = f"cases[{index}]"
    if not isinstance(case, dict):
        problems.append(f"{prefix} must be an object")
        return "", False
    require_string(case.get("id"), f"{prefix}.id", problems)
    query = require_string(case.get("query"), f"{prefix}.query", problems)
    declared_language = require_string(case.get("language"), f"{prefix}.language", problems)
    actual_language = language_slice(query) if query else ""
    if declared_language and declared_language != actual_language:
        problems.append(
            f"{prefix}.language={declared_language!r} does not match {actual_language!r}"
        )
    qrels = case.get("qrels")
    if not isinstance(qrels, list) or not qrels:
        problems.append(f"{prefix}.qrels must be a non-empty list")
    else:
        seen: set[str] = set()
        for qrel_index, qrel in enumerate(qrels):
            qrel_prefix = f"{prefix}.qrels[{qrel_index}]"
            if not isinstance(qrel, dict):
                problems.append(f"{qrel_prefix} must be an object")
                continue
            canonical = require_string(qrel.get("canonical"), f"{qrel_prefix}.canonical", problems)
            relevance = qrel.get("relevance")
            if canonical in seen:
                problems.append(f"{qrel_prefix}.canonical duplicates {canonical!r}")
            seen.add(canonical)
            if canonical and canonical not in catalog_tools:
                problems.append(f"{qrel_prefix}.canonical {canonical!r} is not in the Catalog")
            if not isinstance(relevance, int) or relevance not in {1, 2, 3}:
                problems.append(f"{qrel_prefix}.relevance must be 1, 2 or 3")
    forbidden = case.get("forbidden", [])
    alternatives = case.get("alternative_gold", [])
    if forbidden:
        valid_forbidden = isinstance(forbidden, list) and all(isinstance(item, str) and item.strip() for item in forbidden)
        if not valid_forbidden:
            problems.append(f"{prefix}.forbidden must be a non-empty string list")
        elif len(set(forbidden)) != len(forbidden):
            problems.append(f"{prefix}.forbidden must not contain duplicates")
        elif any(item not in catalog_tools for item in forbidden):
            problems.append(f"{prefix}.forbidden contains an unknown Catalog tool")
        valid_alternatives = isinstance(alternatives, list) and bool(alternatives) and all(
            isinstance(item, str) and item.strip() for item in alternatives
        )
        if not valid_alternatives:
            problems.append(f"{prefix}.alternative_gold is required when forbidden is non-empty")
        elif len(set(alternatives)) != len(alternatives):
            problems.append(f"{prefix}.alternative_gold must not contain duplicates")
        elif any(item not in catalog_tools for item in alternatives):
            problems.append(f"{prefix}.alternative_gold contains an unknown Catalog tool")
        if valid_forbidden and valid_alternatives and set(forbidden).intersection(alternatives):
            problems.append(f"{prefix}.forbidden and alternative_gold must be disjoint")
    elif alternatives:
        problems.append(f"{prefix}.alternative_gold requires a non-empty forbidden list")
    workflow = case.get("workflow")
    is_workflow = workflow is not None
    if is_workflow:
        if not isinstance(workflow, dict):
            problems.append(f"{prefix}.workflow must be an object")
        else:
            required = workflow.get("required", [])
            if not isinstance(required, list) or not 2 <= len(required) <= 4:
                problems.append(f"{prefix}.workflow.required must contain 2 to 4 tools")
            elif any(item not in catalog_tools for item in required):
                problems.append(f"{prefix}.workflow.required contains an unknown Catalog tool")
            subqueries = workflow.get("subqueries", [])
            if subqueries and (
                not isinstance(subqueries, list)
                or not all(isinstance(item, str) and item.strip() for item in subqueries)
            ):
                problems.append(f"{prefix}.workflow.subqueries must be a non-empty string list when present")
    return actual_language, is_workflow


def load_generated_catalog(catalog_dir: Path, problems: list[str]) -> tuple[dict[str, Any], set[str], dict[str, dict[str, Any]]]:
    envelope_path = catalog_dir / "catalog.json"
    tools_dir = catalog_dir / "tools"
    if not envelope_path.is_file():
        problems.append(f"generated Catalog envelope does not exist: {envelope_path}")
        return {}, set(), {}
    if not tools_dir.is_dir():
        problems.append(f"generated Catalog tools directory does not exist: {tools_dir}")
        return {}, set(), {}
    envelope = json.loads(envelope_path.read_text(encoding="utf-8"))
    tools: set[str] = set()
    metadata: dict[str, dict[str, Any]] = {}
    for shard_path in sorted(tools_dir.glob("*.json")):
        shard = json.loads(shard_path.read_text(encoding="utf-8"))
        shard_tools = shard.get("tools")
        if not isinstance(shard_tools, dict):
            problems.append(f"generated Catalog shard has no tools object: {shard_path}")
            continue
        overlap = tools.intersection(shard_tools)
        if overlap:
            problems.append(f"generated Catalog shards duplicate tools: {sorted(overlap)}")
        tools.update(shard_tools)
        metadata.update(shard_tools)
    return envelope, tools, metadata


def validate_manifest(
    manifest_path: Path,
    require_sealed: bool,
    catalog_dir: Path | None = None,
    independent_result_path: Path | None = None,
) -> list[str]:
    problems: list[str] = []
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    if manifest.get("version") != "tool-search-eval-manifest.v1":
        problems.append("manifest version must be tool-search-eval-manifest.v1")
    base_commit = require_string(manifest.get("base_repository_commit"), "base_repository_commit", problems)
    if base_commit:
        exists = subprocess.run(
            ["git", "cat-file", "-e", f"{base_commit}^{{commit}}"],
            cwd=REPO_ROOT,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=False,
        )
        if exists.returncode != 0:
            problems.append("base_repository_commit does not exist")
        else:
            ancestor = subprocess.run(
                ["git", "merge-base", "--is-ancestor", base_commit, "HEAD"],
                cwd=REPO_ROOT,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                check=False,
            )
            if ancestor.returncode != 0:
                problems.append("base_repository_commit is not an ancestor of HEAD")

    owners = manifest.get("owners", {})
    retrieval_owner = require_string(owners.get("retrieval"), "owners.retrieval", problems)
    evaluation_owner = require_string(
        owners.get("independent_evaluation"), "owners.independent_evaluation", problems
    )
    if retrieval_owner and retrieval_owner == evaluation_owner:
        problems.append("retrieval and independent evaluation owners must differ")
    require_string(owners.get("separation_rule"), "owners.separation_rule", problems)

    proxy = manifest.get("proxy_v1", {})

    freeze = manifest.get("algorithm_freeze", {})
    source_paths = freeze.get("go_source_paths")
    expected_source_hash = require_string(
        freeze.get("go_source_sha256"), "algorithm_freeze.go_source_sha256", problems
    )
    if not isinstance(source_paths, list) or not source_paths or not all(
        isinstance(item, str) and item.strip() for item in source_paths
    ):
        problems.append("algorithm_freeze.go_source_paths must be a non-empty string list")
    else:
        missing = [relative for relative in source_paths if not (REPO_ROOT / relative).is_file()]
        for relative in missing:
            problems.append(f"algorithm source {relative} does not exist")
        if not missing and expected_source_hash and sha256_source_set(source_paths) != expected_source_hash:
            problems.append("Go Tool Search algorithm source differs from frozen manifest")
    gate_paths = freeze.get("gate_source_paths")
    expected_gate_hash = require_string(
        freeze.get("gate_source_sha256"), "algorithm_freeze.gate_source_sha256", problems
    )
    if not isinstance(gate_paths, list) or not gate_paths or not all(
        isinstance(item, str) and item.strip() for item in gate_paths
    ):
        problems.append("algorithm_freeze.gate_source_paths must be a non-empty string list")
    else:
        missing = [relative for relative in gate_paths if not (REPO_ROOT / relative).is_file()]
        for relative in missing:
            problems.append(f"evaluation gate source {relative} does not exist")
        if not missing and expected_gate_hash and sha256_source_set(gate_paths) != expected_gate_hash:
            problems.append("Tool Search evaluation gate source differs from frozen manifest")
    for path_field, hash_field in [
        ("workflow_path", "workflow_file_sha256"),
        ("evaluator_path", "evaluator_file_sha256"),
    ]:
        relative = require_string(proxy.get(path_field), f"proxy_v1.{path_field}", problems)
        expected = require_string(proxy.get(hash_field), f"proxy_v1.{hash_field}", problems)
        if relative and expected:
            target = REPO_ROOT / relative
            if not target.is_file():
                problems.append(f"{relative} does not exist")
            elif sha256_file(target) != expected:
                problems.append(f"{relative} hash differs from frozen manifest")

    catalog_payload: dict[str, Any] = {}
    catalog_tools: set[str] = set()
    catalog_metadata: dict[str, dict[str, Any]] = {}
    if catalog_dir is not None:
        catalog_payload, catalog_tools, catalog_metadata = load_generated_catalog(catalog_dir, problems)
        if len(catalog_tools) != proxy.get("tool_count"):
            problems.append("proxy_v1.tool_count differs from generated Catalog")
        if catalog_payload.get("source_hash") != proxy.get("catalog_source_hash"):
            problems.append("proxy_v1.catalog_source_hash differs from generated Catalog")
        if catalog_payload.get("surface_hash") != proxy.get("catalog_surface_hash"):
            problems.append("proxy_v1.catalog_surface_hash differs from generated Catalog")

    independent = manifest.get("independent_test_v1", {})
    qrels_relative = require_string(independent.get("path"), "independent_test_v1.path", problems)
    qrels_path = REPO_ROOT / qrels_relative
    if not qrels_path.is_file():
        problems.append(f"{qrels_relative} does not exist")
        return problems
    qrels = json.loads(qrels_path.read_text(encoding="utf-8"))
    if qrels.get("version") != "tool-search-qrels.v1":
        problems.append("independent qrels version must be tool-search-qrels.v1")
    cases = qrels.get("cases")
    if not isinstance(cases, list):
        problems.append("independent qrels cases must be a list")
        cases = []
    ids: set[str] = set()
    counts: Counter[str] = Counter()
    for index, case in enumerate(cases):
        if isinstance(case, dict) and isinstance(case.get("id"), str):
            if case["id"] in ids:
                problems.append(f"cases[{index}].id duplicates {case['id']!r}")
            ids.add(case["id"])
        language, is_workflow = validate_case(case, index, catalog_tools, problems)
        if language:
            counts[language] += 1
        if is_workflow:
            counts["workflow"] += 1
    derived_coverage = derive_independent_coverage(cases, catalog_metadata, problems)

    if require_sealed:
        if independent.get("state") != "sealed" or qrels.get("state") != "sealed":
            problems.append("independent test must be sealed before release-gate evaluation")
        expected_hash = independent.get("file_sha256")
        if not isinstance(expected_hash, str) or sha256_file(qrels_path) != expected_hash:
            problems.append("independent qrels hash is absent or differs from sealed manifest")
        for field in ["sealed_at", "retrieval_signature", "independent_evaluation_signature"]:
            require_string(independent.get(field), f"independent_test_v1.{field}", problems)
        for slice_name, minimum in independent.get("minimum_counts", {}).items():
            if counts[slice_name] < minimum:
                problems.append(
                    f"independent slice {slice_name} has {counts[slice_name]} cases; requires {minimum}"
                )
        for required in independent.get("required_coverage", []):
            if required not in derived_coverage:
                problems.append(f"independent coverage {required} is absent")
        if independent_result_path is None or not independent_result_path.is_file():
            problems.append("sealed independent evaluation result is required")
        else:
            result = json.loads(independent_result_path.read_text(encoding="utf-8"))
            validate_independent_result(unwrap_independent_result(result), independent, proxy, counts, problems)
    return problems


def unwrap_independent_result(result: Any) -> Any:
    if isinstance(result, dict) and "independent" in result:
        return result["independent"]
    return result


def derive_independent_coverage(
    cases: list[Any], catalog_metadata: dict[str, dict[str, Any]], problems: list[str]
) -> set[str]:
    """Derive release coverage from qrels and Catalog facts, never self-asserted labels."""
    coverage: set[str] = set()
    if not catalog_metadata:
        return coverage
    catalog_products = {str(tool.get("product_id", "")) for tool in catalog_metadata.values()}
    judged: set[str] = set()
    effects: set[str] = set()
    workflow_ok = False
    graded_equivalent = False
    forbidden_alternative = False
    sibling_confusion = False
    for index, case in enumerate(cases):
        if not isinstance(case, dict):
            continue
        canonicals = [
            qrel.get("canonical") for qrel in case.get("qrels", []) if isinstance(qrel, dict)
        ]
        for canonical in canonicals:
            if canonical in catalog_metadata:
                judged.add(canonical)
                effects.add(str(catalog_metadata[canonical].get("effect", "")))
        if len(canonicals) >= 2:
            backing = {
                json.dumps(catalog_metadata.get(item, {}).get("interface_ref", {}), sort_keys=True)
                for item in canonicals
            }
            relevances = {
                qrel.get("relevance") for qrel in case.get("qrels", []) if isinstance(qrel, dict)
            }
            if len(backing) == 1 and "{}" not in backing and len(relevances) >= 2:
                graded_equivalent = True
        forbidden = case.get("forbidden", [])
        alternatives = case.get("alternative_gold", [])
        if forbidden and alternatives:
            forbidden_alternative = True
        workflow = case.get("workflow")
        if isinstance(workflow, dict) and 2 <= len(workflow.get("required", [])) <= 4:
            workflow_ok = True
        confusion = case.get("confusion_family", [])
        if confusion:
            if not isinstance(confusion, list) or len(confusion) < 2 or any(item not in catalog_metadata for item in confusion):
                problems.append(f"cases[{index}].confusion_family must contain at least two Catalog tools")
            elif set(confusion).intersection(canonicals):
                problems.append(f"cases[{index}].confusion_family must be disjoint from qrels")
            elif len({catalog_metadata[item].get("product_id") for item in confusion}) != 1:
                problems.append(f"cases[{index}].confusion_family must belong to one product")
            else:
                sibling_confusion = True
    judged_products = {str(catalog_metadata[item].get("product_id", "")) for item in judged}
    if judged_products == catalog_products:
        coverage.add("all_reviewed_products")
    if {"read", "write", "destructive"}.issubset(effects):
        coverage.add("read_write_destructive")
    if sibling_confusion:
        coverage.add("sibling_confusion")
    if graded_equivalent:
        coverage.add("graded_equivalent_entrypoints")
    if forbidden_alternative:
        coverage.add("forbidden_with_alternative_gold")
    if workflow_ok:
        coverage.add("two_to_four_step_workflows")
    return coverage


def validate_independent_result(
    result: Any,
    independent: dict[str, Any],
    proxy: dict[str, Any],
    counts: Counter[str],
    problems: list[str],
) -> None:
    if not isinstance(result, dict) or result.get("version") != "tool-search-independent-evaluation.v1":
        problems.append("independent result version must be tool-search-independent-evaluation.v1")
        return
    catalog = result.get("catalog", {})
    if catalog.get("source_hash") != proxy.get("catalog_source_hash") or catalog.get("surface_hash") != proxy.get("catalog_surface_hash"):
        problems.append("independent result Catalog differs from frozen proxy generation")
    thresholds = independent.get("thresholds", {})
    overall = result.get("overall", {})
    if overall.get("cases") != sum(counts[name] for name in ["chinese_only", "mixed_chinese_ascii", "english"]):
        problems.append("independent result case count differs from sealed qrels")
    minimum_overall = thresholds.get("minimum_overall_recall_at_5")
    if not isinstance(minimum_overall, (int, float)) or overall.get("recall_at_5", -1) < minimum_overall:
        problems.append("independent overall Recall@5 is below the frozen threshold")
    control = result.get("control_overall", {})
    if control.get("cases") != overall.get("cases"):
        problems.append("independent control case count differs from candidate")
    delta = result.get("recall_at_5_delta")
    ci = result.get("product_cluster_recall_at_5_delta_ci_95", {})
    noninferiority_margin = thresholds.get("recall_at_5_noninferiority_margin")
    if not isinstance(delta, (int, float)) or not isinstance(ci.get("lower"), (int, float)) or not isinstance(ci.get("upper"), (int, float)):
        problems.append("independent paired BM25 delta and product-cluster CI are required")
    elif not isinstance(noninferiority_margin, (int, float)) or ci["lower"] < -noninferiority_margin:
        problems.append("independent candidate fails BM25 Recall@5 noninferiority")
    if thresholds.get("default_switch_requested"):
        minimum_effect = thresholds.get("minimum_default_switch_recall_at_5_gain")
        if not isinstance(minimum_effect, (int, float)) or ci.get("lower", float("-inf")) <= minimum_effect:
            problems.append("independent candidate lacks the frozen Recall@5 gain for a default switch")
    slices = result.get("language_slices", {})
    minimum_slice = thresholds.get("minimum_language_recall_at_5")
    for name in ["chinese_only", "mixed_chinese_ascii", "english"]:
        payload = slices.get(name, {})
        if payload.get("cases") != counts[name]:
            problems.append(f"independent result {name} count differs from sealed qrels")
        if not isinstance(minimum_slice, (int, float)) or payload.get("recall_at_5", -1) < minimum_slice:
            problems.append(f"independent result {name} Recall@5 is below the frozen threshold")
    safety = result.get("safety", {})
    maximum_forbidden = thresholds.get("maximum_forbidden_exposure_at_5")
    minimum_alternative = thresholds.get("minimum_alternative_recall_at_5")
    maximum_sibling = thresholds.get("maximum_sibling_exposure_at_5")
    if not isinstance(maximum_forbidden, (int, float)) or safety.get("forbidden_exposure_at_5", 2) > maximum_forbidden:
        problems.append("independent forbidden exposure is above the frozen threshold")
    if not isinstance(minimum_alternative, (int, float)) or safety.get("alternative_recall_at_5", -1) < minimum_alternative:
        problems.append("independent alternative Recall@5 is below the frozen threshold")
    if not isinstance(maximum_sibling, (int, float)) or safety.get("sibling_exposure_at_5", 2) > maximum_sibling:
        problems.append("independent sibling exposure is above the frozen threshold")
    workflow = result.get("workflow", {})
    if workflow.get("cases") != counts["workflow"]:
        problems.append("independent workflow count differs from sealed qrels")
    if workflow.get("complete_at_5", -1) < thresholds.get("minimum_workflow_complete_at_5", 2):
        problems.append("independent workflow Complete@5 is below the frozen threshold")
    if workflow.get("required_recall_at_5", -1) < thresholds.get("minimum_workflow_required_recall_at_5", 2):
        problems.append("independent workflow required Recall@5 is below the frozen threshold")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", type=Path, default=DEFAULT_MANIFEST)
    parser.add_argument("--catalog-dir", type=Path, default=DEFAULT_CATALOG_DIR)
    parser.add_argument("--require-sealed", action="store_true")
    parser.add_argument("--independent-result", type=Path)
    args = parser.parse_args()
    problems = validate_manifest(
        args.manifest.resolve(),
        args.require_sealed,
        args.catalog_dir.resolve(),
        args.independent_result.resolve() if args.independent_result else None,
    )
    if problems:
        for problem in problems:
            print(f"ERROR: {problem}")
        return 1
    print("tool search evaluation manifest: OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
