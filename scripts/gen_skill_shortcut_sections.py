#!/usr/bin/env python3
"""Generate shortcut discovery sections for DWS skills.

The skill should teach agents which high-level shortcut entries are available
without forcing large product catalogs into every common task. Leaf Schema
publishes the Agent contract, while leaf `--help` remains the source of truth
for accepted flags.
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
MONO_SKILL = ROOT / "skills" / "mono" / "SKILL.md"

SERVICE_TO_SKILL = {
    "aitable": ROOT / "skills" / "multi" / "dingtalk-aitable" / "SKILL.md",
    "attendance": ROOT / "skills" / "multi" / "dingtalk-misc" / "references" / "attendance.md",
    "calendar": ROOT / "skills" / "multi" / "dingtalk-calendar" / "SKILL.md",
    "chat": ROOT / "skills" / "multi" / "dingtalk-chat" / "SKILL.md",
    "contact": ROOT / "skills" / "multi" / "dingtalk-contact" / "SKILL.md",
    "devapp": ROOT / "skills" / "multi" / "dingtalk-dev" / "SKILL.md",
    "ding": ROOT / "skills" / "multi" / "dingtalk-misc" / "references" / "ding.md",
    "doc": ROOT / "skills" / "multi" / "dingtalk-doc" / "SKILL.md",
    "drive": ROOT / "skills" / "multi" / "dingtalk-drive" / "SKILL.md",
    "mail": ROOT / "skills" / "multi" / "dingtalk-mail" / "SKILL.md",
    "minutes": ROOT / "skills" / "multi" / "dingtalk-minutes" / "SKILL.md",
    "oa": ROOT / "skills" / "multi" / "dingtalk-misc" / "references" / "oa.md",
    "report": ROOT / "skills" / "multi" / "dingtalk-misc" / "references" / "report.md",
    "sheet": ROOT / "skills" / "multi" / "dingtalk-misc" / "references" / "sheet.md",
    "todo": ROOT / "skills" / "multi" / "dingtalk-todo" / "SKILL.md",
    "wiki": ROOT / "skills" / "multi" / "dingtalk-wiki" / "SKILL.md",
}

MONO_START = "<!-- VISIBLE_SHORTCUTS_OVERVIEW_START -->"
MONO_END = "<!-- VISIBLE_SHORTCUTS_OVERVIEW_END -->"
PRODUCT_START = "<!-- VISIBLE_SHORTCUTS_START -->"
PRODUCT_END = "<!-- VISIBLE_SHORTCUTS_END -->"

# Large, high-frequency product skills should route known intents directly and
# keep their full shortcut inventory in Runtime Catalog/Schema. Add services
# here only after verifying that the product skill has its own reviewed routing
# section and intent table; compacting a sparse skill without an alternative
# route would make its shortcuts harder to discover.
COMPACT_PRODUCT_SERVICES = {"chat"}


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


def mono_overview(items: list[dict[str, Any]]) -> str:
    counts = Counter(item["service"] for item in items)
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

下面只统计当前公开 catalog 中的 shortcut，不展开完整明细。已知意图应先按产品 Skill、意图表或任务 reference 选择唯一命令；命令已选中时直接执行，只在参数或安全语义不确定时读取 leaf Schema，在当前 Cobra flags 不确定时读取 leaf Help。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service <service> --format json` 做最后回退；不要为已知高频意图加载完整产品 Catalog。

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

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。命令已选中时直接执行；只在参数或安全语义不确定时读取 leaf Schema（例如 `dws schema --cli-path "{service} +<shortcut>" --format json`），在当前 Cobra flags 不确定时读取 `dws {service} <shortcut> --help`。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service {service} --format json` 批量发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
{os.linesep.join(table)}
{PRODUCT_END}"""


def compact_product_section(service: str, rows: list[dict[str, Any]]) -> str:
    return f"""{PRODUCT_START}
## Shortcut 发现（按需）

`{md_escape(service)}` 当前有 {len(rows)} 条公开 shortcut，完整清单保留在 Runtime Catalog 与 Schema，不在高频产品根 Skill 中重复展开。已知意图直接使用下方的优先路由、意图表或任务 reference；命令已选中时直接执行，只在参数/安全语义不确定时读取 leaf Schema，在当前 Cobra flags 不确定时读取 leaf Help。

仅当现有路由和 reference 都无法定位低频能力时，才执行 `dws shortcut list --service {md_escape(service)} --format json` 做最后回退；不要为已知高频意图加载完整 Shortcut Catalog 或产品级 Schema。
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


def update_product_skills(items: list[dict[str, Any]], check: bool) -> list[Path]:
    by_service: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for item in items:
        by_service[item["service"]].append(item)
    changed = []
    for service, path in SERVICE_TO_SKILL.items():
        if service not in by_service:
            continue
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
    changed = update_mono(items, args.check)
    changed.extend(update_product_skills(items, args.check))
    if args.check and changed:
        print("run: python3 scripts/gen_skill_shortcut_sections.py", file=sys.stderr)
        return 1
    print(f"visible_shortcuts={len(items)} services={len(set(item['service'] for item in items))}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
