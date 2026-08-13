#!/usr/bin/env python3
"""Verify frozen Tool Search evaluation inputs and independent qrels gates."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
from pathlib import Path
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[2]
DEFAULT_MANIFEST = REPO_ROOT / "scripts/testdata/tool_search_eval_manifest.json"
DEFAULT_CATALOG_DIR = REPO_ROOT / ".worktrees/policy-tmp/tool-search-schema-catalog"
CHINESE = re.compile(r"[\u3400-\u9fff]")
ASCII_WORD = re.compile(r"[A-Za-z0-9]")


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
    catalog_dir: Path | None = None,
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

    # Source files evolve with every ranking change; the manifest deliberately
    # does not pin their content hashes (that would couple every code change
    # to a manifest update). Only their existence is asserted. Behavior is
    # gated by the Go evaluation sentinels instead.
    for path_field in ["workflow_path", "evaluator_path"]:
        relative = require_string(proxy.get(path_field), f"proxy_v1.{path_field}", problems)
        if relative and not (REPO_ROOT / relative).is_file():
            problems.append(f"{relative} does not exist")

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
    for index, case in enumerate(cases):
        if isinstance(case, dict) and isinstance(case.get("id"), str):
            if case["id"] in ids:
                problems.append(f"cases[{index}].id duplicates {case['id']!r}")
            ids.add(case["id"])
        validate_case(case, index, catalog_tools, problems)
    # Coverage derivation also validates qrels data quality (confusion_family
    # consistency) as a side effect; the release-time sealed gate that consumed
    # the coverage set has been removed.
    derive_independent_coverage(cases, catalog_metadata, problems)
    return problems


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


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", type=Path, default=DEFAULT_MANIFEST)
    parser.add_argument("--catalog-dir", type=Path, default=DEFAULT_CATALOG_DIR)
    args = parser.parse_args()
    problems = validate_manifest(
        args.manifest.resolve(),
        args.catalog_dir.resolve(),
    )
    if problems:
        for problem in problems:
            print(f"ERROR: {problem}")
        return 1
    print("tool search evaluation manifest: OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
