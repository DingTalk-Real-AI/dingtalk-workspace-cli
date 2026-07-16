#!/usr/bin/env python3
"""Generate shortcut sections for DWS skills.

The skill should teach agents which high-level shortcut entries are available
in the public catalog. `dws shortcut list` publishes their machine-readable
contract, while leaf `--help` remains the source of truth for accepted flags.
"""

from __future__ import annotations

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
    "attendance": ROOT / "skills" / "multi" / "dingtalk-attendance" / "SKILL.md",
    "calendar": ROOT / "skills" / "multi" / "dingtalk-calendar" / "SKILL.md",
    "chat": ROOT / "skills" / "multi" / "dingtalk-chat" / "SKILL.md",
    "contact": ROOT / "skills" / "multi" / "dingtalk-contact" / "SKILL.md",
    "devapp": ROOT / "skills" / "multi" / "dingtalk-dev" / "SKILL.md",
    "ding": ROOT / "skills" / "multi" / "dingtalk-ding" / "SKILL.md",
    "doc": ROOT / "skills" / "multi" / "dingtalk-doc" / "SKILL.md",
    "drive": ROOT / "skills" / "multi" / "dingtalk-drive" / "SKILL.md",
    "mail": ROOT / "skills" / "multi" / "dingtalk-mail" / "SKILL.md",
    "minutes": ROOT / "skills" / "multi" / "dingtalk-minutes" / "SKILL.md",
    "oa": ROOT / "skills" / "multi" / "dingtalk-oa" / "SKILL.md",
    "report": ROOT / "skills" / "multi" / "dingtalk-report" / "SKILL.md",
    "sheet": ROOT / "skills" / "multi" / "dingtalk-sheet" / "SKILL.md",
    "todo": ROOT / "skills" / "multi" / "dingtalk-todo" / "SKILL.md",
    "wiki": ROOT / "skills" / "multi" / "dingtalk-wiki" / "SKILL.md",
}

MONO_START = "<!-- VISIBLE_SHORTCUTS_OVERVIEW_START -->"
MONO_END = "<!-- VISIBLE_SHORTCUTS_OVERVIEW_END -->"
PRODUCT_START = "<!-- VISIBLE_SHORTCUTS_START -->"
PRODUCT_END = "<!-- VISIBLE_SHORTCUTS_END -->"


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
        skill = path.parent.name if path else "—"
        rows.append(f"| `{md_escape(service)}` | {count} | `{md_escape(skill)}` | `dws shortcut list --service {md_escape(service)} --format json` |")
    body = "\n".join(rows)
    return f"""{MONO_START}
## Shortcut 总览

下面统计当前公开 catalog 中的 shortcut。mono 模式不展开 200+ 行明细，避免 skill 过重；需要执行时先按产品路由，再用 `dws shortcut list --service <service> --format json` 读取参数、约束、风险和示例，最后用 `dws <service> +<shortcut> --help` 核对当前 Cobra flags。multi 模式的各产品 skill 会展开该产品的 shortcut 表。

| 服务 | shortcut 数 | multi skill | 发现命令 |
|---|---:|---|---|
{body}
{MONO_END}"""


def product_section(service: str, rows: list[dict[str, Any]]) -> str:
    table = []
    for item in rows:
        table.append(
            f"| `dws {md_escape(service)} {md_escape(item['command'])}` | "
            f"{md_escape(item['risk'])} | {md_escape(item['desc'])} |"
        )
    return f"""{PRODUCT_START}
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 来自独立于 Runtime Schema 的公开 catalog。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 `dws shortcut list --service {service} --format json` 读取参数、约束、风险和示例，并以 `dws {service} <shortcut> --help` 核对当前 Cobra flags；不要对 `+` 路径调用 `dws schema`。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
{os.linesep.join(table)}
{PRODUCT_END}"""


def update_mono(items: list[dict[str, Any]]) -> None:
    text = MONO_SKILL.read_text(encoding="utf-8")
    block = mono_overview(items)
    updated = replace_block(text, MONO_START, MONO_END, block, "## 产品总览")
    MONO_SKILL.write_text(updated, encoding="utf-8")


def update_product_skills(items: list[dict[str, Any]]) -> None:
    by_service: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for item in items:
        by_service[item["service"]].append(item)
    for service, path in SERVICE_TO_SKILL.items():
        if service not in by_service:
            continue
        if not path.exists():
            raise RuntimeError(f"skill file not found for {service}: {path}")
        text = path.read_text(encoding="utf-8")
        block = product_section(service, by_service[service])
        anchor = "## 概念地图" if service == "devapp" else "## 意图表"
        updated = replace_block(text, PRODUCT_START, PRODUCT_END, block, anchor)
        path.write_text(updated, encoding="utf-8")


def main() -> int:
    items = collect_visible()
    update_mono(items)
    update_product_skills(items)
    print(f"visible_shortcuts={len(items)} services={len(set(item['service'] for item in items))}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
