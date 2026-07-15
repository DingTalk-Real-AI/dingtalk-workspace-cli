#!/usr/bin/env python3
"""Helpers for classifying real shortcut CLI executions.

Some DWS helper paths can exit 0 while returning a JSON error envelope on
stdout.  Real-test reports should reflect the backend payload, not just the
process exit code.
"""

from __future__ import annotations

import json
import re
from typing import Any


def _json_values(text: str) -> list[Any]:
    """Parse one or more adjacent JSON values from stdout."""
    values: list[Any] = []
    text = (text or "").strip()
    if not text:
        return values

    decoder = json.JSONDecoder()
    idx = 0
    length = len(text)
    while idx < length:
        while idx < length and text[idx].isspace():
            idx += 1
        if idx >= length:
            break
        try:
            value, next_idx = decoder.raw_decode(text, idx)
        except json.JSONDecodeError:
            return values
        values.append(value)
        idx = next_idx
    return values


def _truthy_error(value: Any) -> bool:
    if value is None or value is False:
        return False
    if isinstance(value, str):
        return value.strip() != ""
    if isinstance(value, (list, dict)):
        return len(value) > 0
    return True


def _truthy_error_code(value: Any) -> bool:
    if value is None or value is False:
        return False
    if isinstance(value, str):
        code = value.strip()
        if not code:
            return False
        return code.lower() not in {"0", "ok", "success", "succeed"}
    if isinstance(value, (int, float)):
        return value != 0
    return True


def payload_indicates_error(stdout: str) -> bool:
    for value in _json_values(stdout):
        if not isinstance(value, dict):
            continue
        status = value.get("status")
        if isinstance(status, str) and status.lower() == "error":
            return True
        if value.get("success") is False:
            return True
        if _truthy_error(value.get("error")):
            return True
        if _truthy_error_code(value.get("errorCode")):
            return True
        if _truthy_error_code(value.get("error_code")):
            return True
        if _truthy_error_code(value.get("code")):
            return True
    return False


def classify_real_status(exit_code: int | None, stdout: str, current_status: str | None = None) -> str:
    if current_status in {"timeout", "held"}:
        return current_status
    if exit_code != 0:
        return "real-error"
    if payload_indicates_error(stdout):
        return "real-error"
    return "real-ok"


def _haystack(result: dict[str, Any]) -> str:
    return f"{result.get('stdout') or ''}\n{result.get('stderr') or ''}".lower()


def classify_failure(result: dict[str, Any]) -> tuple[str, str, str]:
    """Return (category, fixability, note) for a real-test result."""
    status = result.get("status")
    if status == "real-ok":
        return ("passed", "fixed", "真实后端执行成功。")
    if status == "timeout":
        return ("timeout", "needs-rerun", "命令超过测试超时时间；需单独复测或扩大超时。")
    if status == "held":
        return ("held", "manual-approval", "高风险或无安全目标，需人工逐项授权后执行。")

    text = _haystack(result)
    service = result.get("service")
    command = result.get("command")
    method = result.get("method") or ""

    if "payload-classified-error" in method or (result.get("exit_code") == 0 and payload_indicates_error(result.get("stdout") or "")):
        return (
            "cli-error-envelope",
            "cli-wrapper-fix-needed",
            "进程退出码为 0，但 stdout JSON 含 error/status:error/errorCode；需要底层 helper 或 MCP 调用层把业务错误转成非零退出。",
        )
    if (
        "not_authenticated" in text
        or "auth_permission" in text
        or "permission" in text
        or "forbidden" in text
        or "auth_error" in text
        or "权限" in text
        or "没有开发者身份" in text
        or "无权限" in text
        or "当前登录用户无权限" in text
    ):
        return (
            "auth-or-permission",
            "not-cli-fixable",
            "真实账号、应用 scope 或资源权限不足；CLI 只能如实暴露，不能在本仓库内修复权限。",
        )
    if "invalid_base_id" in text or "base_not_found" in text or "invalid source baseid" in text or "failed to resolve docid from baseid" in text:
        return (
            "missing-real-aitable-fixture",
            "not-cli-fixable-without-fixture",
            "AI 表格命令需要真实 Base/Table/View/Record 等资源；安全负向 ID 只能验证调用链，不能让后端成功。",
        )
    if (
        "resource_not_found" in text
        or "not found" in text
        or "does not exist" in text
        or "does not exit" in text
        or "not exist" in text
        or "不存在" in text
        or "没找到" in text
        or "no such" in text
        or "not_found" in text
        or "无效的会话" in text
        or "opencid无效" in text
        or "openconversationid无效" in text
        or "openmessageid解密失败" in text
        or "failed to decrypt" in text
        or "invalid openmsgid" in text
        or "opendingid 无效" in text
        or "event does not exist" in text
        or "nodeid 格式不合法" in text
    ):
        return (
            "missing-real-resource",
            "not-cli-fixable-without-fixture",
            "真实测试使用的资源/单据/消息/群/文档不存在；需要准备对应 fixture 后才能期望成功，不属于 shortcut 参数投影错误。",
        )
    if service == "calendar" and command == "+respond-event" and "organizer" in text:
        return (
            "backend-business-rule",
            "not-cli-fixable",
            "后端业务规则：日程组织者不能修改自己的参会响应；需要用非组织者账号复测。",
        )
    if service == "minutes" and ("ai_minutes" in text or "暂无妙记" in text):
        return (
            "missing-real-minutes-fixture",
            "not-cli-fixable-without-fixture",
            "当前账号没有满足条件的妙记/听记或录制会话；需准备真实会议产物后复测。",
        )
    if service == "chat" and command == "+messages-send-card" and "receiveruid" in text:
        return (
            "backend-or-mcp-error",
            "not-cli-fixable-first",
            "dry-run 已证明 CLI 装配了 receiverUid；真实后端仍报 receiverUid/openConversationId 为空，优先按 MCP schema/服务端字段映射问题处理。",
        )
    if service == "chat" and command == "+chat-audit-join" and "applicantuid" in text:
        return (
            "backend-or-mcp-error",
            "not-cli-fixable-first",
            "dry-run 已证明 CLI 装配了 applicantUid/inviterUid；真实后端仍报 applicantUid 缺失，优先按 MCP schema/服务端字段映射问题处理。",
        )
    if (
        service == "chat"
        and (
            "opencid or cid is required" in text
            or "openconversationid or cid is required" in text
            or "openconversationid is required" in text
        )
    ):
        return (
            "backend-or-mcp-error",
            "not-cli-fixable-first",
            "fake MCP 已证明 CLI 已装配会话 ID 字段；真实后端仍报 openConversationId/openCid/cid 缺失，优先按 MCP schema/服务端字段映射问题处理。",
        )
    if (
        "system_error" in text
        or "system error" in text
        or "mcp_server_error" in text
        or "nullpointer" in text
        or "mcp_tool_error" in text
    ):
        return (
            "backend-or-mcp-error",
            "not-cli-fixable-first",
            "后端/MCP 服务返回内部错误；CLI 无法直接修复，但报告保留 trace/stdout 供服务端排查。",
        )
    if (
        "参数" in text
        or "param" in text
        or "required" in text
        or "不能为空" in text
        or "invalid argument" in text
        or "invalid " in text
        or "validation" in text
        or "json 解析失败" in text
        or "格式错误" in text
    ):
        return (
            "input-or-business-validation",
            "test-input-or-backend-rule",
            "命令已真实进入本地/后端校验；若该项仍使用安全负向输入，则失败符合预期；若使用真实 fixture 仍失败，再作为 CLI bug 处理。",
        )
    return (
        "unclassified-real-error",
        "needs-triage",
        "真实后端返回错误，当前无法自动判断归因；需结合 stdout/stderr 单独排查。",
    )


def summarize_results(results: list[dict[str, Any]], include_held: bool = True) -> dict[str, int]:
    summary = {"total": len(results), "ok": 0, "error": 0, "timeout": 0}
    if include_held:
        summary["held"] = 0
    for r in results:
        status = r.get("status")
        if status == "real-ok":
            summary["ok"] += 1
        elif status == "timeout":
            summary["timeout"] += 1
        elif status == "held" and include_held:
            summary["held"] += 1
        else:
            summary["error"] += 1
    return summary


def summarize_failure_categories(results: list[dict[str, Any]]) -> dict[str, int]:
    out: dict[str, int] = {}
    for r in results:
        category = r.get("failure_category")
        if not category:
            category, _, _ = classify_failure(r)
        if category == "passed":
            continue
        out[category] = out.get(category, 0) + 1
    return dict(sorted(out.items(), key=lambda kv: (-kv[1], kv[0])))


_OSS_URL_RE = re.compile(r"https?://[^\s\"'<>]*oss[^\s\"'<>]*", re.IGNORECASE)
_ALIBABA_ACCESS_KEY_ID_RE = re.compile(r"\bLTAI[A-Za-z0-9]{12,}\b")
_OSS_QUERY_KEY_RE = re.compile(
    r"(?i)\b(OSSAccessKeyId|AccessKeyId|accessKeyId|access_key_id)=([^&\s\"'<>]+)"
)
_OSS_SIGNATURE_RE = re.compile(r"(?i)\b(Signature|security-token|x-oss-security-token)=([^&\s\"'<>]+)")
_SECRET_ASSIGN_RE = re.compile(
    r'(?i)("?(?:accessKeySecret|access_key_secret|clientSecret|client_secret|appSecret|app_secret|secret)"?\s*[:=]\s*")([^"]+)(")'
)


def sanitize_text(text: str) -> str:
    """Redact secrets from real CLI outputs before writing reports.

    Real backend read commands may return signed OSS URLs or app credential
    fields.  Reports need the shape of stdout/stderr for debugging, not live
    credentials or presigned download URLs.
    """
    if not text:
        return text
    text = _OSS_URL_RE.sub("[REDACTED_OSS_URL]", text)
    text = _ALIBABA_ACCESS_KEY_ID_RE.sub("[REDACTED_ALIBABA_ACCESS_KEY_ID]", text)
    text = _OSS_QUERY_KEY_RE.sub(lambda m: f"{m.group(1)}=[REDACTED_ALIBABA_ACCESS_KEY_ID]", text)
    text = _OSS_SIGNATURE_RE.sub(lambda m: f"{m.group(1)}=[REDACTED_SIGNATURE]", text)
    text = _SECRET_ASSIGN_RE.sub(lambda m: f"{m.group(1)}[REDACTED_SECRET]{m.group(3)}", text)
    return text


def sanitize_value(value: Any) -> Any:
    if isinstance(value, str):
        return sanitize_text(value)
    if isinstance(value, list):
        return [sanitize_value(v) for v in value]
    if isinstance(value, dict):
        out: dict[str, Any] = {}
        for key, item in value.items():
            key_l = str(key).lower()
            if key_l in {
                "accesskeysecret",
                "access_key_secret",
                "clientsecret",
                "client_secret",
                "appsecret",
                "app_secret",
                "secret",
                "signature",
            }:
                out[key] = "[REDACTED_SECRET]"
            elif key_l in {"accesskeyid", "access_key_id", "ossaccesskeyid"}:
                out[key] = "[REDACTED_ALIBABA_ACCESS_KEY_ID]"
            elif key_l in {"resourceurl", "url", "downloadurl"} and isinstance(item, str) and "oss" in item.lower():
                out[key] = "[REDACTED_OSS_URL]"
            else:
                out[key] = sanitize_value(item)
        return out
    return value


def sanitize_result(result: dict[str, Any]) -> dict[str, Any]:
    return sanitize_value(result)
