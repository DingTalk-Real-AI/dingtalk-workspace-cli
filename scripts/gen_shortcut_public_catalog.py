#!/usr/bin/env python3
"""Generate the public shortcut catalog from real test results."""

from __future__ import annotations

import datetime as dt
import json
import re
import subprocess
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
READ_PATH = ROOT / "docs" / "shortcut-real-read-results.json"
WRITE_PATH = ROOT / "docs" / "shortcut-real-write-results.json"
GO_PATH = ROOT / "internal" / "shortcut" / "public_catalog_generated.go"
CATALOG_PATH = ROOT / "docs" / "shortcut-public-catalog.json"
FOLLOWUP_MD_PATH = ROOT / "docs" / "shortcut-real-test-followups.md"
FOLLOWUP_JSON_PATH = ROOT / "docs" / "shortcut-real-test-followups.json"
SEMANTIC_PATHS = [
    ROOT / "internal" / "shortcut" / "semantic_catalog.json",
    ROOT / "internal" / "shortcut" / "semantic_catalog_doc.json",
    ROOT / "internal" / "shortcut" / "semantic_catalog_aitable.json",
    ROOT / "internal" / "shortcut" / "semantic_catalog_minutes.json",
    ROOT / "internal" / "shortcut" / "semantic_catalog_drive.json",
    ROOT / "internal" / "shortcut" / "semantic_catalog_wiki.json",
]


def load(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def squish(text: str, limit: int = 180) -> str:
    text = re.sub(r"\s+", " ", text or "").strip()
    if len(text) <= limit:
        return text
    return text[: limit - 1] + "…"


def error_text(row: dict[str, Any]) -> str:
    return f"{row.get('stderr') or ''}\n{row.get('stdout') or ''}"


def parse_error_payload(row: dict[str, Any]) -> dict[str, Any]:
    for key in ("stderr", "stdout"):
        raw = (row.get(key) or "").strip()
        if not raw:
            continue
        try:
            value = json.loads(raw)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict):
            err = value.get("error")
            if isinstance(err, dict):
                return err
            return value
    return {}


def evidence(row: dict[str, Any]) -> str:
    payload = parse_error_payload(row)
    msg = payload.get("message")
    if isinstance(msg, str) and msg.strip():
        return squish(msg, 220)
    return squish(error_text(row), 220)


def collect() -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    evidence_public: list[dict[str, Any]] = []
    followups: list[dict[str, Any]] = []
    evidence_by_key: dict[tuple[str, str], dict[str, Any]] = {}
    result_paths = [
        (suite, path)
        for suite, path in (("read", READ_PATH), ("write", WRITE_PATH))
        if path.exists()
    ]
    for suite, path in result_paths:
        data = load(path)
        for row in data.get("results", []):
            item = {
                "suite": suite,
                "service": row.get("service") or "",
                "command": row.get("command") or "",
                "risk": row.get("risk") or "",
                "status": row.get("status") or "",
            }
            evidence_by_key[(item["service"], item["command"])] = item
            if row.get("status") == "real-ok":
                evidence_public.append(item)
                continue
            followups.append({
                **item,
                "failure_category": row.get("failure_category") or "unclassified",
                "fixability": row.get("fixability") or "",
                "diagnosis": row.get("diagnosis") or "",
                "evidence": evidence(row),
            })

    # The raw real-run captures can be intentionally absent because they may
    # contain account-specific business data. In that case retain the committed
    # non-Chat evidence/follow-ups and only rebuild the reviewed Chat surface.
    if not result_paths:
        for row in load(CATALOG_PATH).get("results", []):
            item = dict(row)
            evidence_public.append(item)
            evidence_by_key[(item.get("service") or "", item.get("command") or "")] = item
        if FOLLOWUP_JSON_PATH.exists():
            followups = load(FOLLOWUP_JSON_PATH).get("results", [])

    # Chat visibility is a reviewed semantic decision, not a side effect of
    # whether the current account happened to have a fixture for a real run.
    # Keep the real-run rows as evidence/follow-ups, but publish Chat entries
    # exclusively from the reviewed semantic catalog.
    semantics = [load(path) for path in SEMANTIC_PATHS]
    semantic_services = {semantic.get("service") or "" for semantic in semantics}
    if "" in semantic_services or len(semantic_services) != len(semantics):
        raise ValueError(f"invalid or duplicate semantic catalog services: {semantic_services!r}")
    public = [row for row in evidence_public if row["service"] not in semantic_services]
    for semantic in semantics:
        service = semantic["service"]
        for command, record in semantic.get("shortcuts", {}).items():
            if not record.get("public"):
                continue
            if not record.get("reviewed"):
                raise ValueError(f"public semantic shortcut is not reviewed: {service} {command}")
            availability = (
                record.get("availability")
                or semantic.get("default_availability")
                or ""
            )
            if availability != "available":
                raise ValueError(
                    f"public semantic shortcut is not available: {service} {command}={availability}"
                )
            observed = evidence_by_key.get((service, command), {})
            risk = record.get("risk") or ""
            if not risk:
                raise ValueError(f"public semantic shortcut lacks reviewed risk: {service} {command}")
            if observed.get("risk") and observed["risk"] != risk:
                raise ValueError(
                    f"semantic shortcut risk drift: {service} {command}: "
                    f"reviewed={risk} observed={observed['risk']}"
                )
            public.append({
                "suite": "semantic",
                "service": service,
                "command": command,
                "risk": risk,
                "status": "reviewed_available",
                "disposition": record.get("disposition") or "",
                "semantic_delta": record.get("semantic_delta") or "",
                "availability": availability,
            })
    public.sort(key=lambda r: (r["service"], r["command"]))
    followups.sort(key=lambda r: (r["suite"], r["service"], r["command"]))
    return public, followups


def go_quote(s: str) -> str:
    return json.dumps(s, ensure_ascii=False)


def write_go(rows: list[dict[str, Any]]) -> None:
    lines = [
        "// Code generated by scripts/gen_shortcut_public_catalog.py; DO NOT EDIT.",
        "",
        "package shortcut",
        "",
        "// publicShortcutCatalog contains the generated shortcut surface used by",
        "// command discovery and skill generation.",
        "func generatedPublicShortcutCatalog() map[string]struct{} {",
        "\treturn map[string]struct{}{",
    ]
    for row in rows:
        key = f"{row['service']}\x00{row['command']}"
        lines.append(f"\t\t{go_quote(key)}: {{}},")
    lines.extend(["\t}", "}", ""])
    GO_PATH.write_text("\n".join(lines), encoding="utf-8")
    subprocess.run(["gofmt", "-w", str(GO_PATH)], check=True)


def md_escape(s: str) -> str:
    return str(s).replace("|", "\\|").replace("\n", "<br>")


def write_followup_md(rows: list[dict[str, Any]]) -> None:
    generated = dt.datetime.now().isoformat(timespec="seconds")
    lines = [
        "# Shortcut 真实测试跟进清单",
        "",
        f"生成时间：`{generated}`",
        "",
        "来源：`docs/shortcut-real-read-results.json` 与 `docs/shortcut-real-write-results.json`。",
        "",
        "口径：记录真实后端测试中需要继续定位的 case，用于 CR 和问题分派；Agent 使用入口以公开 shortcut catalog 和产品 skill 为准。",
        "",
        f"总计：{len(rows)} 条。",
        "",
        "| # | suite | shortcut | risk | status | category | fixability | 处理依据 |",
        "|---:|---|---|---|---|---|---|---|",
    ]
    for idx, row in enumerate(rows, 1):
        shortcut = f"{row['service']} {row['command']}"
        basis = row["diagnosis"] or row["evidence"]
        lines.append(
            f"| {idx} | {md_escape(row['suite'])} | `{md_escape(shortcut)}` | "
            f"{md_escape(row['risk'])} | {md_escape(row['status'])} | "
            f"{md_escape(row['failure_category'])} | {md_escape(row['fixability'])} | "
            f"{md_escape(squish(basis, 160))} |"
        )
    lines.append("")
    rendered = "\n".join(lines)
    if FOLLOWUP_MD_PATH.exists():
        existing = FOLLOWUP_MD_PATH.read_text(encoding="utf-8")
        without_time = re.compile(r"生成时间：`[^`]+`")
        if without_time.sub("生成时间：`<generated>`", existing) == without_time.sub(
            "生成时间：`<generated>`", rendered
        ):
            return
    FOLLOWUP_MD_PATH.write_text(rendered, encoding="utf-8")


def write_json(public: list[dict[str, Any]], followups: list[dict[str, Any]]) -> None:
    generated = dt.datetime.now().isoformat()
    write_json_if_changed(CATALOG_PATH, {
        "count": len(public),
        "results": public,
    }, generated)
    write_json_if_changed(FOLLOWUP_JSON_PATH, {
        "count": len(followups),
        "results": followups,
    }, generated)


def write_json_if_changed(path: Path, payload: dict[str, Any], generated: str) -> None:
    if path.exists():
        existing = load(path)
        existing.pop("generated_at", None)
        if existing == payload:
            return
    path.write_text(json.dumps({
        "generated_at": generated,
        **payload,
    }, ensure_ascii=False, indent=2), encoding="utf-8")


def main() -> int:
    public, followups = collect()
    write_go(public)
    write_followup_md(followups)
    write_json(public, followups)
    print(f"public={len(public)} followups={len(followups)}")
    print(f"written: {GO_PATH}")
    print(f"written: {CATALOG_PATH}")
    print(f"written: {FOLLOWUP_MD_PATH}")
    print(f"written: {FOLLOWUP_JSON_PATH}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
