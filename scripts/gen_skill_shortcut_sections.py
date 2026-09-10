#!/usr/bin/env python3
"""Generate shortcut discovery sections for DWS skills.

The skill should teach agents which high-level shortcut entries are available
without forcing large product catalogs into every common task. Leaf Schema
publishes the Agent contract; exact leaf `--help` is a bounded recovery for an
unavailable Schema or an `unknown flag`. An `unknown command` must not query
Help for the nonexistent leaf.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from collections import Counter, defaultdict
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

import gen_shortcut_comparison as shortcut_source  # noqa: E402

CATALOG_PATH = ROOT / "docs" / "shortcut-public-catalog.json"
CHAT_SEMANTIC_CATALOG = ROOT / "internal" / "shortcut" / "semantic_catalog.json"
MONO_SKILL = ROOT / "skills" / "mono" / "SKILL.md"
SHARED_SKILL = ROOT / "skills" / "multi" / "dingtalk-shared" / "SKILL.md"
RUNTIME_CONTRACT_SOURCE = (
    ROOT / "skills" / "multi" / "dingtalk-shared" / "references" / "runtime-contract.md"
)
SERVICE_TO_SKILL = {
    "aitable": ROOT / "skills" / "multi" / "dingtalk-aitable" / "SKILL.md",
    "attendance": ROOT / "skills" / "multi" / "dingtalk-misc" / "references" / "attendance.md",
    "calendar": ROOT / "skills" / "multi" / "dingtalk-calendar" / "SKILL.md",
    "chat": ROOT / "skills" / "multi" / "dingtalk-chat" / "SKILL.md",
    "contact": ROOT / "skills" / "multi" / "dingtalk-contact" / "SKILL.md",
    "devapp": ROOT / "skills" / "multi" / "dingtalk-misc" / "references" / "devapp.md",
    "ding": ROOT / "skills" / "multi" / "dingtalk-misc" / "references" / "ding.md",
    "doc": ROOT / "skills" / "multi" / "dingtalk-doc" / "SKILL.md",
    "drive": ROOT / "skills" / "multi" / "dingtalk-drive" / "SKILL.md",
    "mail": ROOT / "skills" / "multi" / "dingtalk-mail" / "SKILL.md",
    "minutes": ROOT / "skills" / "multi" / "dingtalk-minutes" / "SKILL.md",
    "oa": ROOT / "skills" / "multi" / "dingtalk-misc" / "references" / "oa.md",
    "pat": ROOT / "skills" / "multi" / "dingtalk-misc" / "references" / "pat.md",
    "report": ROOT / "skills" / "multi" / "dingtalk-misc" / "references" / "report.md",
    "sheet": ROOT / "skills" / "multi" / "dingtalk-misc" / "references" / "sheet.md",
    "todo": ROOT / "skills" / "multi" / "dingtalk-todo" / "SKILL.md",
    "whiteboard": ROOT / "skills" / "multi" / "dingtalk-misc" / "references" / "whiteboard.md",
    "wiki": ROOT / "skills" / "multi" / "dingtalk-wiki" / "SKILL.md",
}
SERVICE_TO_SKILL_MIRRORS = {
    "sheet": [ROOT / "skills" / "mono" / "references" / "products" / "sheet.md"],
    "whiteboard": [ROOT / "skills" / "mono" / "references" / "products" / "whiteboard.md"],
}

MONO_START = "<!-- VISIBLE_SHORTCUTS_OVERVIEW_START -->"
MONO_END = "<!-- VISIBLE_SHORTCUTS_OVERVIEW_END -->"
PRODUCT_START = "<!-- VISIBLE_SHORTCUTS_START -->"
PRODUCT_END = "<!-- VISIBLE_SHORTCUTS_END -->"
RUNTIME_CONTRACT_START = "<!-- DWS_RUNTIME_CONTRACT_START -->"
RUNTIME_CONTRACT_END = "<!-- DWS_RUNTIME_CONTRACT_END -->"

# Large, high-frequency product skills should route known intents directly and
# keep their full shortcut inventory in Runtime Catalog/Schema. Add services
# here only after verifying that the product skill has its own reviewed routing
# section and intent table; compacting a sparse skill without an alternative
# route would make its shortcuts harder to discover.
COMPACT_PRODUCT_SERVICES = {"aitable", "chat", "doc", "drive", "minutes"}


def md_escape(value: Any) -> str:
    text = str(value or "")
    return text.replace("\\", "\\\\").replace("|", "\\|").replace("\n", " ")


def load_public_catalog() -> set[tuple[str, str]]:
    if not CATALOG_PATH.exists():
        return set()
    data = json.loads(CATALOG_PATH.read_text(encoding="utf-8"))
    return {
        (str(row["service"]), str(row["command"]))
        for row in data.get("results", [])
    }


def collect_visible() -> list[dict[str, Any]]:
    public_catalog = load_public_catalog()
    items = [
        item
        for item in shortcut_source.collect()
        if (item["service"], item["command"]) in public_catalog
    ]
    return sorted(items, key=lambda item: (item["service"], item["command"]))


def replace_block(text: str, start: str, end: str, block: str, fallback_anchor: str) -> str:
    if start in text and end in text:
        before = text.split(start, 1)[0]
        after = text.split(end, 1)[1]
        return before + block + after
    if fallback_anchor not in text:
        raise RuntimeError(f"fallback anchor not found: {fallback_anchor!r}")
    return text.replace(fallback_anchor, block + "\n\n" + fallback_anchor, 1)


def replace_required_block(text: str, start: str, end: str, block: str) -> str:
    if text.count(start) != 1 or text.count(end) != 1:
        raise RuntimeError(
            f"expected exactly one generated block {start!r} ... {end!r}"
        )
    before = text.split(start, 1)[0]
    after = text.split(end, 1)[1]
    return before + block + after


def runtime_contract_block() -> str:
    contract = RUNTIME_CONTRACT_SOURCE.read_text(encoding="utf-8").strip()
    if not contract.startswith("## 最小 DWS 执行契约"):
        raise RuntimeError(
            f"runtime contract must start with its canonical heading: {RUNTIME_CONTRACT_SOURCE}"
        )
    return (
        f"{RUNTIME_CONTRACT_START}\n"
        f"{contract}\n"
        f"{RUNTIME_CONTRACT_END}"
    )


def mono_overview(items: list[dict[str, Any]]) -> str:
    # The source collector cannot see a small number of commands whose final
    # canonical names are normalized at runtime. Overview counts come from the
    # reviewed public catalog so they cannot under-report those declarations.
    counts = Counter(service for service, _ in load_public_catalog())
    rows = []
    for service, count in sorted(counts.items()):
        path = SERVICE_TO_SKILL.get(service)
        skill = "—"
        if path:
            skill = next((part for part in reversed(path.parts) if part.startswith("dingtalk-")), path.parent.name)
        rows.append(f"| `{md_escape(service)}` | {count} | `{md_escape(skill)}` |")
    body = "\n".join(rows)
    return f"""{MONO_START}
## Shortcut 总览

下面只统计当前公开 catalog 中的 shortcut，不展开完整明细。已知意图先按产品 Skill、意图表或任务 reference 选唯一命令；参数/约束/安全不明时读一次 leaf 窄 Schema。Schema 不可用时才读一次已知 leaf Help；`unknown flag` 用同 leaf Help 修正一次；`unknown command` 不查 Help，先用错误的明确 suggestion，否则用已加载 Skill/reference 的明确兼容入口，仍无则报告漂移，不枚举全 Catalog。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service <service> --format json` 做最后回退；不要为已知意图加载完整产品 Catalog 或 root/parent Help。

| 服务 | shortcut 数 | multi skill |
|---|---:|---|
{body}
{MONO_END}"""


def product_section(service: str, rows: list[dict[str, Any]]) -> str:
    if service in COMPACT_PRODUCT_SERVICES:
        return compact_product_section(service, rows)

    table = []
    for item in rows:
        table.append(
            f"| `dws {md_escape(service)} {md_escape(item['command'])}` | "
            f"{md_escape(item['risk'])} | {md_escape(item['desc'])} |"
        )
    return f"""{PRODUCT_START}
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。按本 skill/recipe 路由，命中时 Shortcut 优先于原子命令。参数只查 `dws schema --cli-path "{service} +<shortcut>" --compact --jq '{{cli_path,parameters,constraints,confirmation}}' -f json`；仅需且已发布 `result` 时查 `--jq '{{cli_path,outcomes:.result.outcomes,pagination}}'`，字段级再查 `data_schema`；缺失不以 Help/样例推断。Schema 不可用才读一次已知 leaf Help；`unknown flag` 用同 leaf Help 修正一次。`unknown command` 禁 Help：错误 suggestion → 已加载 Skill/reference 明确入口；均无则报漂移。禁全 Catalog/root/parent/product Help；仅映射、接口或 provenance 审计省略 `--compact`。现有路由和 reference 均无法定位低频能力时，才用 `dws shortcut list --service {service} --format json` 发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
{os.linesep.join(table)}
{PRODUCT_END}"""


def chat_compatibility_counts(source: dict[str, Any]) -> Counter[str]:
    """Count callable aliases by catalog membership and actual CLI visibility."""
    counts: Counter[str] = Counter()
    default_availability = source.get("default_availability", "available")
    for record in source.get("shortcuts", {}).values():
        if record.get("disposition") != "alias_internal":
            continue
        if record.get("availability", default_availability) != "available":
            continue
        if record.get("public", False):
            counts["public"] += 1
        elif record.get("compatibility_visible", False):
            counts["compatibility_visible"] += 1
        else:
            counts["hidden"] += 1
    return counts


def compact_product_section(service: str, rows: list[dict[str, Any]]) -> str:
    # Compact skills intentionally do not depend on the source parser's ability
    # to recover every runtime-normalized declaration. The reviewed public
    # catalog is the count authority for this non-enumerating overview.
    public_count = sum(1 for item_service, _ in load_public_catalog() if item_service == service)
    if service == "chat":
        return f"""{PRODUCT_START}
## Shortcut 发现（Shortcut-first）

按 Golden Route/reference 选 Shortcut；仅缺底层字段用 atomic，低频走 reference/Catalog。

参数查 `dws schema --cli-path "chat <leaf>" --compact --jq '{{cli_path,parameters,constraints,confirmation}}' -f json`；仅需且已发布 `result` 时查 outcomes/pagination，字段级再查 `data_schema`；缺失不以 Help/样例推断。Schema 不可用才读一次已知 leaf Help；`unknown flag` 用同 leaf Help 修正一次。`unknown command` 禁 Help：错误 suggestion → 已加载 Skill/reference 明确入口；均无则报漂移。禁全 Catalog/root/parent/product Help；低频 reference 不默认 Help。
{PRODUCT_END}"""
    if service in {"doc", "drive"}:
        discovery = """已知意图按下方路由；参数/约束/安全不明时读一次 leaf 窄 Schema。仅需且已发布 `result` 时查 outcomes/pagination，字段级再查 `data_schema`；缺失不以 Help/样例推断。Schema 不可用才读一次已知 leaf Help；`unknown flag` 用同 leaf Help 修正一次。`unknown command` 禁 Help：错误 suggestion → 已加载 Skill/reference 明确入口；均无则报漂移。禁全 Catalog/root/parent/product Help；低频 reference 不默认 Help。"""
    elif service == "aitable":
        discovery = """已知 leaf 直接执行。参数只查 `dws schema --cli-path "aitable <leaf>" --compact --jq '{cli_path,parameters,constraints,confirmation}' -f json`；仅需且已发布 `result` 时查 outcomes/pagination，字段级再查 `data_schema`；缺失不以 Help/样例推断。Schema 不可用才读一次已知 leaf Help；`unknown flag` 用同 leaf Help 修正一次。`unknown command` 禁 Help：错误 suggestion → 已加载 Skill/reference 明确入口；均无则报漂移。禁全 Catalog/root/parent/product Help；低频 reference 不默认 Help；同一任务只读一个 Reference。"""
    else:
        discovery = """已知意图按下方路由/reference 直调；参数/约束/安全不明时读一次 leaf 窄 Schema。仅需且已发布 `result` 时查 outcomes/pagination，字段级再查 `data_schema`；缺失不以 Help/样例推断。Schema 不可用才读一次已知 leaf Help；`unknown flag` 用同 leaf Help 修正一次。`unknown command` 禁 Help：错误 suggestion → 已加载 Skill/reference 明确入口；均无则报漂移。禁全 Catalog/root/parent/product Help；低频 reference 不默认 Help。"""
    if service == "aitable":
        fallback = """仅当根路由、精确 task reference 和 `references/aitable.md` 的低频原子索引都无法定位能力时，才执行 `dws shortcut list --service aitable --format json` 做最终回退；不要为已知意图加载完整 Shortcut Catalog 或产品级 Schema。"""
    else:
        fallback = f"""仅当现有路由和 reference 都无法定位低频能力时，才执行 `dws shortcut list --service {md_escape(service)} --format json` 做最后回退；不要为已知高频意图加载完整 Shortcut Catalog 或产品级 Schema。"""
    return f"""{PRODUCT_START}
## Shortcut 发现（按需）

`{md_escape(service)}` 当前有 {public_count} 条公开 shortcut，完整清单保留在 Runtime Catalog 与 Schema，不在高频产品根 Skill 中重复展开。{discovery}

{fallback}
{PRODUCT_END}"""


def apply_update(path: Path, text: str, updated: str, check: bool) -> bool:
    if updated == text:
        return False
    if check:
        print(f"generated skill drift: {path.relative_to(ROOT)}", file=sys.stderr)
    else:
        path.write_text(updated, encoding="utf-8")
    return True


def update_mono(items: list[dict[str, Any]], check: bool) -> list[Path]:
    text = MONO_SKILL.read_text(encoding="utf-8")
    block = mono_overview(items)
    updated = replace_block(text, MONO_START, MONO_END, block, "## 产品总览")
    return [MONO_SKILL] if apply_update(MONO_SKILL, text, updated, check) else []


def update_runtime_contract(check: bool) -> list[Path]:
    block = runtime_contract_block()
    changed = []
    targets = [
        ROOT / "skills" / "mono" / "references" / "products" / "sheet.md",
        ROOT / "skills" / "multi" / "dingtalk-chat" / "SKILL.md",
        ROOT / "skills" / "multi" / "dingtalk-doc" / "SKILL.md",
        ROOT / "skills" / "multi" / "dingtalk-drive" / "SKILL.md",
        ROOT / "skills" / "multi" / "dingtalk-minutes" / "SKILL.md",
        SHARED_SKILL,
        ROOT / "skills" / "multi" / "dingtalk-misc" / "references" / "report.md",
        ROOT / "skills" / "multi" / "dingtalk-misc" / "references" / "sheet.md",
        ROOT / "skills" / "multi" / "dingtalk-wiki" / "SKILL.md",
    ]
    for path in targets:
        text = path.read_text(encoding="utf-8")
        updated = replace_required_block(
            text,
            RUNTIME_CONTRACT_START,
            RUNTIME_CONTRACT_END,
            block,
        )
        if apply_update(path, text, updated, check):
            changed.append(path)
    return changed


def update_product_skills(items: list[dict[str, Any]], check: bool) -> list[Path]:
    by_service: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for item in items:
        by_service[item["service"]].append(item)
    changed = []
    for service, primary_path in SERVICE_TO_SKILL.items():
        if service not in by_service:
            continue
        paths = [primary_path, *SERVICE_TO_SKILL_MIRRORS.get(service, [])]
        for path in paths:
            if not path.exists():
                raise RuntimeError(f"skill file not found for {service}: {path}")
            text = path.read_text(encoding="utf-8")
            block = product_section(service, by_service[service])
            anchor = "## 概念地图" if service == "devapp" else "## 意图表"
            updated = replace_block(text, PRODUCT_START, PRODUCT_END, block, anchor)
            if apply_update(path, text, updated, check):
                changed.append(path)
    return changed


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--check",
        action="store_true",
        help="verify generated skill sections are current without rewriting files",
    )
    args = parser.parse_args()

    items = collect_visible()
    changed = update_runtime_contract(args.check)
    changed.extend(update_mono(items, args.check))
    changed.extend(update_product_skills(items, args.check))
    if args.check and changed:
        print("run: python3 scripts/gen_skill_shortcut_sections.py", file=sys.stderr)
        return 1
    print(f"visible_shortcuts={len(items)} services={len(set(item['service'] for item in items))}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
