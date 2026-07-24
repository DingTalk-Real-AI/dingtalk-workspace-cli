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


# ---------------------------------------------------------------------------
# 上层 / 下层数据一致性（投影保真）
#
# 警示案例：contact +list-roles、oa +list-forms 在 exit 0、无 error 信封的情况下
# 依然静默返空——底层 MCP 明明有 57 个角色 / 93 张表单，投影层因为容器 key 对不上
# （result[].labels[] 未下钻 / 缺 processCodeList）把数据全吃掉了。只看上层（便捷层
# 投影输出）永远发现不了这类问题；**shortcut 务必对比上层投影与下层原始后端数据**。
#
# 判定规则因此新增一条铁律：一个只读/列表类 shortcut，若上层投影为空，必须能拿下层
# 原始响应来佐证「本就没有数据」。拿不到下层、或下层明明有数据，都不能算 real-ok。
# ---------------------------------------------------------------------------

# 便捷层投影输出里承载业务列表的常见容器字段名。
_PROJECTION_LIST_KEYS = (
    "roles", "forms", "users", "user", "items", "list", "records", "files",
    "docs", "nodes", "messages", "conversations", "groups", "events",
    "attendees", "rooms", "threads", "folders", "tags", "templates", "contacts",
    "spaces", "tasks", "todos", "created", "members", "depts", "departments",
    "sheets", "bases", "tables", "views", "apps", "minutes", "reports",
    "comments", "attachments", "instances", "robots", "bots", "results",
    "workflows", "permissions", "versions", "calendars", "cards",
)


def count_projection_items(stdout: str) -> int | None:
    """从便捷层投影输出里数出条目数（上层）。

    返回 None 表示这段输出不像一个「列表型」投影（例如详情命令、纯文本、写操作
    回执），此时不参与空投影判定。识别到列表型投影时返回其条目数（可能为 0）。
    """
    best: int | None = None
    for value in _json_values(stdout):
        if isinstance(value, list):
            best = max(best or 0, len(value))
            continue
        if not isinstance(value, dict):
            continue
        count_field = value.get("count")
        if isinstance(count_field, bool):
            count_field = None
        if isinstance(count_field, int):
            best = max(best or 0, count_field)
        for key in _PROJECTION_LIST_KEYS:
            member = value.get(key)
            if isinstance(member, list):
                best = max(best or 0, len(member))
    return best


def backend_record_count(raw: Any, depth: int = 0) -> int:
    """递归估算下层原始后端响应里的业务条目数（下层真值）。

    取任意深度下「最大的一个对象数组」的长度作为「后端是否有数据」的代理指标——
    足以判定投影是否把非空的底层数据吃成了空。
    """
    if depth > 6 or raw is None:
        return 0
    best = 0
    if isinstance(raw, list):
        obj_items = sum(1 for item in raw if isinstance(item, dict))
        best = max(best, obj_items)
        for item in raw:
            best = max(best, backend_record_count(item, depth + 1))
    elif isinstance(raw, dict):
        for value in raw.values():
            best = max(best, backend_record_count(value, depth + 1))
    return best


def _backend_payload(result: dict[str, Any]) -> Any:
    """取出随 result 一起记录的下层原始后端响应（若采集了的话）。

    约定字段：backend_raw / raw_backend / backend / lower_layer / backend_stdout。
    backend_stdout 是字符串时按 JSON 解析。"""
    for key in ("backend_raw", "raw_backend", "backend", "lower_layer"):
        if key in result and result[key] not in (None, "", {}, []):
            return result[key]
    text = result.get("backend_stdout")
    if isinstance(text, str) and text.strip():
        values = _json_values(text)
        if values:
            return values[0] if len(values) == 1 else values
    return None


def compare_layers(shortcut_stdout: str, backend_raw: Any) -> tuple[bool, int | None, int]:
    """对比上层投影与下层原始后端数据。

    返回 (是否投影吃了数据, 上层条目数, 下层条目数)。当上层是列表型投影且为空、
    而下层明显有对象数组时，判定为投影数据丢失（True）。"""
    upper = count_projection_items(shortcut_stdout)
    lower = backend_record_count(backend_raw)
    lost = upper == 0 and lower > 0
    return lost, upper, lower


def classify_real_status(
    exit_code: int | None,
    stdout: str,
    current_status: str | None = None,
    *,
    backend_raw: Any = None,
) -> str:
    if current_status in {"timeout", "held"}:
        return current_status
    if exit_code != 0:
        return "real-error"
    if payload_indicates_error(stdout):
        return "real-error"
    # 上下层比对：下层有数据但上层投影为空 = 投影吃数据，绝不能算通过。
    if backend_raw is not None:
        lost, _upper, _lower = compare_layers(stdout, backend_raw)
        if lost:
            return "real-error"
    return "real-ok"


def _haystack(result: dict[str, Any]) -> str:
    return f"{result.get('stdout') or ''}\n{result.get('stderr') or ''}".lower()


def classify_failure(result: dict[str, Any]) -> tuple[str, str, str]:
    """Return (category, fixability, note) for a real-test result."""
    status = result.get("status")

    # 上下层比对优先：即便进程 exit 0、无 error 信封、status 被记成 real-ok，只要
    # 采集到的下层原始后端有数据而上层投影为空，就是投影把数据吃了——判定为
    # projection-data-loss，属于 shortcut 自身可修的 bug（改投影层的容器 key/下钻）。
    backend_raw = _backend_payload(result)
    if backend_raw is not None and result.get("exit_code") in (0, None):
        lost, upper, lower = compare_layers(result.get("stdout") or "", backend_raw)
        if lost:
            return (
                "projection-data-loss",
                "cli-projection-fix-needed",
                f"上层便捷层投影为空（{upper} 条），但下层原始后端有数据（约 {lower} 条）；"
                "投影层的容器 key/嵌套下钻与真实响应结构不匹配，把非空数据吃成了空。"
                "需修投影层解析，并补「喂真实响应结构、断言非空」的单测。",
            )

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


def projection_audit(result: dict[str, Any]) -> dict[str, Any] | None:
    """投影保真审计：对只读/列表类 shortcut 强制「上下层比对」这条铁律。

    返回 None 表示无需关注；否则返回一个 warning dict 供报告展示：
      - projection-data-loss：下层有数据、上层空 —— 确定的投影吃数据 bug；
      - empty-projection-unverified：上层投影为空但缺下层采集 —— 无法证明「本就为空」，
        必须补采下层原始响应（原始 MCP 响应 / 对应 leaf 命令）再比对，禁止直接判 real-ok。
    """
    stdout = result.get("stdout") or ""
    upper = count_projection_items(stdout)
    if upper is None or upper > 0:
        return None  # 非列表型投影，或上层本就有数据 —— 最常见的正常情形。
    backend_raw = _backend_payload(result)
    if backend_raw is None:
        return {
            "kind": "empty-projection-unverified",
            "upper_count": upper,
            "backend_count": None,
            "note": "便捷层投影为空，但未采集下层原始后端响应，无法证明底层确实无数据；"
            "按铁律必须补采下层后再比对，不能直接判 real-ok。",
        }
    lost, _upper, lower = compare_layers(stdout, backend_raw)
    if lost:
        return {
            "kind": "projection-data-loss",
            "upper_count": upper,
            "backend_count": lower,
            "note": "下层原始后端有数据但上层投影为空：投影层解析与真实响应结构不匹配。",
        }
    return None


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
