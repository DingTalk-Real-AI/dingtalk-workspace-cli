#!/usr/bin/env python3
"""Generate a backend/MCP issue handoff report from real shortcut results."""

from __future__ import annotations

import json
import re
from collections import defaultdict
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
READ_PATH = ROOT / "docs" / "shortcut-real-read-results.json"
WRITE_PATH = ROOT / "docs" / "shortcut-real-write-results.json"
OUT_PATH = ROOT / "docs" / "shortcut-backend-mcp-issues.md"


def load(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def squish(text: str, limit: int = 300) -> str:
    text = re.sub(r"\s+", " ", text or "").strip()
    if len(text) <= limit:
        return text
    return text[: limit - 1] + "…"


def parse_payload(row: dict[str, Any]) -> dict[str, Any]:
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


def message(row: dict[str, Any]) -> str:
    payload = parse_payload(row)
    msg = payload.get("message")
    if isinstance(msg, str) and msg:
        return msg
    return (row.get("stderr") or row.get("stdout") or "").strip()


def operation(row: dict[str, Any]) -> str:
    payload = parse_payload(row)
    op = payload.get("operation")
    if isinstance(op, str) and op:
        return op
    msg = message(row)
    match = re.search(r"\(operation:\s*([^)]+)\)", msg)
    return match.group(1) if match else "-"


def trace_id(row: dict[str, Any]) -> str:
    payload = parse_payload(row)
    value = payload.get("trace_id") or payload.get("traceId")
    if value:
        return str(value)
    msg = message(row)
    match = re.search(r'"trace_id":"([^"]+)"', msg)
    return match.group(1) if match else "-"


def issue_key(row: dict[str, Any]) -> str:
    service = row.get("service")
    command = row.get("command")
    msg = message(row).lower()
    op = operation(row)

    if service == "chat" and "receiveruid" in msg:
        return "chat-receiveruid-mapping"
    if service == "chat" and "applicantuid" in msg:
        return "chat-applicantuid-mapping"
    if service == "chat" and ("opencid" in msg or "openconversationid" in msg):
        return "chat-conversation-id-mapping"
    if service == "aitable" and "success\":true" in msg and "error" in msg:
        if "roleid is required" in msg:
            return "aitable-role-id-mapping"
        if "getdentrydto returns null" in msg:
            return "aitable-notfound-as-system-error"
        if "workflow" in msg or "workflow" in op:
            return "aitable-workflow-system-error"
        if command in {"+base-get-primary-doc-id", "+record-primary-doc-get"}:
            return "aitable-primary-doc-system-error"
        return "aitable-success-true-error-envelope"
    return f"{service}-other-backend-mcp"


ISSUE_META = {
    "chat-conversation-id-mapping": {
        "title": "Chat/IM 会话 ID 字段在 MCP/后端映射中疑似丢失",
        "owner": "IM MCP / IM 后端字段映射",
        "priority": "P1",
        "summary": "CLI 已传 group/conversation-id/open-conversation-id（部分 case 使用真实 cid），后端仍报 openCid/openConversationId/cid required。",
        "expect": "MCP schema/网关应接受并透传 openConversationId/openCid/cid 中的兼容字段；如果资源无效，应返回“无效会话”，而不是 required。",
    },
    "chat-receiveruid-mapping": {
        "title": "Chat card 发送 receiverUid 疑似未从 receiver 透传",
        "owner": "IM MCP / card 发送参数映射",
        "priority": "P1",
        "summary": "CLI 传入 receiver=103262，后端仍报 receiverUid 和 openConversationId 不能同时为空。",
        "expect": "receiver 应映射为 receiverUid，或 schema 明确要求 receiverUid；真实入参不应在 MCP 层丢失。",
    },
    "chat-applicantuid-mapping": {
        "title": "Chat 入群审批 applicantUid/inviterUid 疑似未透传",
        "owner": "IM MCP / 入群审批参数映射",
        "priority": "P1",
        "summary": "CLI 传入 applicant=103262、inviter=519019，后端仍报 applicantUid required。",
        "expect": "applicant/inviter 应映射为 applicantUid/inviterUid；如果 recordId/group 无效，应返回对应资源错误而不是 applicantUid 缺失。",
    },
    "aitable-success-true-error-envelope": {
        "title": "AI 表格 MCP 错误 envelope 语义不一致：success=true 但 error 非空/status=error",
        "owner": "AI 表格 MCP wrapper",
        "priority": "P1",
        "summary": "多条 AI 表格命令返回 MCP_TOOL_ERROR，内部 JSON 同时出现 success=true、status=error、error 非空。",
        "expect": "只要 error 非空或 status=error，success 应为 false，外层也应按业务错误返回稳定错误码/trace。",
    },
    "aitable-role-id-mapping": {
        "title": "AI 表格 roleId 参数疑似未被 MCP 正确读取",
        "owner": "AI 表格 MCP role 接口",
        "priority": "P1",
        "summary": "CLI 已传 --role-id，但 MCP 返回 roleId is required。",
        "expect": "role-id/roleId 字段应被正确映射；如果 role 不存在，返回 ROLE_NOT_FOUND，而不是 required。",
    },
    "aitable-notfound-as-system-error": {
        "title": "AI 表格无效 Base/Table/Field/Record 被包装成 SYSTEM_ERROR",
        "owner": "AI 表格 MCP wrapper / 后端错误码",
        "priority": "P2",
        "summary": "安全负向 ID 下，部分写接口返回 getDentryDTO returns null、type=SYSTEM_ERROR、retryable=true。",
        "expect": "资源不存在应返回 INPUT_ERROR/NOT_FOUND 且 retryable=false，避免误导调用方重试。",
    },
    "aitable-workflow-system-error": {
        "title": "AI 表格 Workflow 查询在真实 Base 下返回系统级错误",
        "owner": "AI 表格 Workflow MCP / 后端",
        "priority": "P1",
        "summary": "使用真实可访问 Base 查询 workflow list/get，返回 LIST_WORKFLOWS_ERROR/GET_WORKFLOW_ERROR。",
        "expect": "无 workflow 时应返回空列表或 WORKFLOW_NOT_FOUND；有后端异常时需提供稳定错误码和可排查 trace。",
    },
    "aitable-primary-doc-system-error": {
        "title": "AI 表格记录主文档查询在真实 record 下返回 no record/SYSTEM_ERROR",
        "owner": "AI 表格 primary doc MCP / 后端",
        "priority": "P2",
        "summary": "record-query 已能查到真实 recordId，但 primary-doc 查询返回 no record、type=SYSTEM_ERROR。",
        "expect": "若该记录无主文档，应返回空/未创建；若 recordId 语义不匹配，应返回明确参数错误，不应是系统错误。",
    },
}


def collect() -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for suite, path in (("read", READ_PATH), ("write", WRITE_PATH)):
        data = load(path)
        for row in data.get("results", []):
            if row.get("failure_category") == "backend-or-mcp-error":
                item = dict(row)
                item["suite"] = suite
                rows.append(item)
    return rows


def render_case(row: dict[str, Any]) -> str:
    return (
        f"| `{row['suite']}` | `{row.get('service')} {row.get('command')}` | "
        f"`{operation(row)}` | `{trace_id(row)}` | {squish(message(row), 180)} |\n"
        f"| | input | | | `{row.get('input') or ''}` |\n"
    )


def main() -> int:
    rows = collect()
    grouped: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for row in rows:
        grouped[issue_key(row)].append(row)

    lines: list[str] = []
    lines.append("# Shortcut 真实测试：后端 / MCP 问题整理\n")
    lines.append("这份报告只汇总 `failure_category = backend-or-mcp-error` 的 case，已尽量排除权限、缺真实资源、当前账号无数据等噪音。\n")
    lines.append("## 总览\n")
    lines.append(f"- Backend/MCP case 总数：{len(rows)}")
    lines.append(f"- 聚合问题数：{len(grouped)}")
    lines.append("- 复现口径：真实 dws CLI；无 mock；无 dry-run；命令输入和 trace_id 均来自真实测试结果。\n")

    lines.append("## 建议优先看\n")
    priority_order = [
        "chat-conversation-id-mapping",
        "chat-receiveruid-mapping",
        "chat-applicantuid-mapping",
        "aitable-success-true-error-envelope",
        "aitable-workflow-system-error",
        "aitable-role-id-mapping",
        "aitable-primary-doc-system-error",
        "aitable-notfound-as-system-error",
    ]
    for idx, key in enumerate([k for k in priority_order if k in grouped], 1):
        meta = ISSUE_META.get(key, {})
        lines.append(f"{idx}. [{meta.get('priority', 'P2')}] {meta.get('title', key)}（{len(grouped[key])} case）")
    lines.append("")

    for key in [k for k in priority_order if k in grouped] + [k for k in grouped if k not in priority_order]:
        cases = grouped[key]
        meta = ISSUE_META.get(key, {
            "title": key,
            "owner": "待确认",
            "priority": "P2",
            "summary": "未归入已知聚合类型，需要按 case 单独排查。",
            "expect": "请根据 operation/trace_id 判断是 schema、字段映射还是后端错误语义问题。",
        })
        lines.append(f"## {meta['title']}\n")
        lines.append(f"- 优先级：{meta['priority']}")
        lines.append(f"- 建议 owner：{meta['owner']}")
        lines.append(f"- 现象：{meta['summary']}")
        lines.append(f"- 期望：{meta['expect']}")
        lines.append(f"- 涉及 case：{len(cases)}\n")
        lines.append("| 套件 | shortcut | operation | trace_id | 证据 |")
        lines.append("|---|---|---|---|---|")
        for row in cases:
            lines.append(render_case(row).rstrip())
        lines.append("")

    OUT_PATH.write_text("\n".join(lines), encoding="utf-8")
    print(f"written: {OUT_PATH}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
