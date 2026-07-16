#!/usr/bin/env python3
"""Generate a per-error review report from real shortcut test results."""

from __future__ import annotations

import json
import re
from collections import Counter
import html
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
READ_PATH = ROOT / "docs" / "shortcut-real-read-results.json"
WRITE_PATH = ROOT / "docs" / "shortcut-real-write-results.json"
OUT_PATH = ROOT / "docs" / "shortcut-error-review.html"


CATEGORY_LABELS = {
    "auth-or-permission": "鉴权/权限",
    "backend-business-rule": "后端业务规则",
    "backend-or-mcp-error": "后端/MCP",
    "input-or-business-validation": "输入/业务校验",
    "missing-real-aitable-fixture": "缺 AI 表格 fixture",
    "missing-real-minutes-fixture": "缺妙记 fixture",
    "missing-real-resource": "缺真实资源",
    "held": "敏感/高风险暂缓",
}


RESOURCE_AUDIT = [
    ("chat +group-members", "已补齐", "查到真实群名 `浅曦-kida,Dennis,秋画`，runner 已改为用群名而不是 openConversationId；真实回归成功。", "无需后续动作。"),
    ("chat +messages-mget", "已补齐", "复用真实单聊消息 `msgEuOor1PmFBNlx9M06N9z1Q==`；真实回归成功。", "无需后续动作。"),
    ("chat +messages-read-status", "已补齐", "复用真实单聊会话 `cidie1367hAfBxqipzE59k5sknHLrHmvYkw98NADhfnjPI=` 与同一 openMessageId；真实回归成功。", "无需后续动作。"),
    ("chat +messages-query-send-status", "已补齐", "复用真实发送返回的 openTaskId；真实回归成功。", "无需后续动作。"),
    ("ding +receiver-status", "已补齐", "先只读 `ding +list` 找到已有 openDingId，再查询 receiver status；没有新发 DING，真实回归成功。", "无需后续动作。"),
    ("sheet +list-sheets", "已自造", "创建临时在线表格 `DWS shortcut 真实测试表格 20260715`，nodeId=`mweZ92PV6O36dZbnsMZx70ylJxEKBD6p`；真实回归成功。", "后续可保留为稳定 fixture，或测试结束后人工清理。"),
    ("todo +todo-done", "已自造并修复 CLI", "runner 会先创建当前账号自己的临时待办，再执行 `todo +todo-done`；同时修复了 todo 列表 pageSize=50 返回空、响应多层 result unwrap 不稳的问题；真实回归成功。", "无需后续动作；代码已有单测覆盖 nested result。"),
    ("contact +by-mobile", "已按用户授权补齐", "使用用户指定手机号 `13161187007` 作为真实 fixture；runner 只在该命令上替换 mobile，不扩散到其它服务。", "真实回归成功后该项将从失败列表移除；若后续要脱敏公开报告，可再加展示层脱敏。"),
    ("attendance +get-class / +get-group / +get-group-filtered", "不建议自造", "`attendance +search-class` 与 `+search-group --type FIXED` 均返回空；创建班次/考勤组会改组织考勤配置，属于高影响业务数据。", "需要考勤后端/业务同学提供可读测试班次与考勤组 ID。"),
    ("chat +chat-get-by-id", "暂未找到", "该 shortcut 只接受数字 groupId；真实群列表只返回 openConversationId，没有数字群号字段。", "需要 IM 后端提供可用数字 groupId，或评估是否新增 openConversationId 形态的 shortcut。"),
    ("chat +messages-resource-url", "暂未自造", "需要真实含 mediaId 的图片/文件/视频消息；当前文本消息无法产生 resource-id。", "可在测试群发一条图片/文件消息并提取 mediaId 后补 fixture；注意会产生群消息。"),
    ("chat +thread-replies", "暂未自造", "需要真实话题消息 topicId；普通群消息不能替代。", "需要话题群 fixture，或由 IM 同学提供当前账号可访问 topicId。"),
    ("aitable +dashboard-share-get", "真实资源仍失败", "已有真实 base/dashboard，但 share-get 返回 404 `Failed to get dashboard share config`；更像分享配置未开启或后端接口行为问题。", "需要 AI 表格/后端确认如何创建/开启 dashboard share fixture，或修正 404 语义。"),
    ("write 类 delete/recall/approve/wiki move/copy 等", "不自动自造", "这些命令即便能造资源，也会涉及删除、撤回、审批通过、知识库移动/复制等高影响动作。", "需要逐项授权和专门测试空间/机器人/审批单据，不建议混在批量回归里自动跑。"),
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


def operation(row: dict[str, Any]) -> str:
    payload = parse_error_payload(row)
    op = payload.get("operation")
    if isinstance(op, str) and op:
        return op
    msg = payload.get("message")
    if isinstance(msg, str):
        m = re.search(r"\(operation:\s*([^)]+)\)", msg)
        if m:
            return m.group(1)
    return "-"


def trace_id(row: dict[str, Any]) -> str:
    payload = parse_error_payload(row)
    tid = payload.get("trace_id") or payload.get("traceId")
    return str(tid) if tid else "-"


def evidence(row: dict[str, Any]) -> str:
    payload = parse_error_payload(row)
    msg = payload.get("message")
    if isinstance(msg, str) and msg.strip():
        return squish(msg, 220)
    return squish(error_text(row), 220)


def action_for(row: dict[str, Any], suite: str) -> tuple[str, str, str]:
    """Return (owner, action, verification) for one failing row."""
    category = row.get("failure_category") or "unclassified"
    service = row.get("service") or ""
    command = row.get("command") or ""
    text = error_text(row).lower()
    msg = evidence(row)

    if category == "auth-or-permission":
        return (
            "权限/应用配置",
            "给当前登录账号、DWS 应用或对应资源补齐权限/scope；本仓库 shortcut 不应绕过权限。拿 trace_id 给服务端/开放平台排查具体 scope。",
            f"补权限后重跑 `{service} {command}`；若仍是 permission，再看 trace_id。",
        )

    if category == "missing-real-aitable-fixture":
        return (
            "测试数据",
            "准备真实 Base/Table/View/Field/Record/Role/Chart/Dashboard 等 fixture，并把真实 ID 写入真实测试 runner；当前安全负向 ID 只能证明调用链，不可能成功。",
            "fixture 准备好后重跑对应 aitable 命令；预期从 not_found/invalid_base_id 变为成功或更具体业务错误。",
        )

    if category == "missing-real-minutes-fixture":
        return (
            "测试数据",
            "准备当前账号可见的真实妙记/听记/录制会话，或把 runner 的搜索关键词改成必然能命中的会议产物。",
            "用真实 taskUuid/note/minutes 资源重跑 minutes 命令。",
        )

    if category == "missing-real-resource":
        if "opencid" in text or "openconversationid" in text:
            return (
                "测试数据",
                "替换为当前账号真实可访问的群/会话 openConversationId；如果用真实群仍报“无效”，再查 IM 后端解析。",
                "先用 `chat +my-groups` 或群搜索拿真实会话 ID，再重跑。",
            )
        if "openmessageid" in text or "openmsgid" in text or "invalid openmsgid" in text:
            return (
                "测试数据",
                "替换为真实 openMessageId，并保证消息属于当前账号可访问会话；当前 no-such ID 只能验证负向路径。",
                "先用消息列表拿 messageId，再重跑消息详情/状态/撤回类命令。",
            )
        if service == "attendance":
            return (
                "测试数据",
                "替换为真实考勤班次/考勤组/员工/假期等资源 ID；当前 no-such ID 只能验证负向路径。",
                "先用考勤列表/管理后台拿真实 ID，再重跑该 attendance 命令。",
            )
        if service == "contact":
            return (
                "测试数据",
                "替换为当前组织通讯录中真实存在的人、手机号、部门或花名册对象；当前占位查询不会命中。",
                "用真实姓名/手机号/部门重跑 contact 命令。",
            )
        if "table_not_found" in text or "node" in text or "doc" in text:
            return (
                "测试数据",
                "替换为真实文档/节点 ID；文档评论、分享、版本等命令需要资源存在且账号可访问。",
                "用真实 doc/node 重跑；若仍失败再看 doc/doc-comment 工具字段。",
            )
        return (
            "测试数据",
            "把 runner 里的安全负向 ID 换成真实资源 ID；当前错误说明调用已进后端，但资源不存在。",
            f"准备 fixture 后重跑 `{service} {command}`。",
        )

    if category == "backend-or-mcp-error":
        if service == "chat" and (
            "opencid" in text
            or "openconversationid" in text
            or "receiveruid" in text
            or "applicantuid" in text
        ):
            return (
                "后端/MCP schema",
                "CLI fake MCP 已证明字段已装配；需要修 MCP tool schema 或网关字段映射，确认 openConversationId/openCid/cid、receiverUid、applicantUid 等字段没有在 schema 校验/转发时被丢弃。",
                "修 MCP 后不改 shortcut，直接重跑真实命令；预期错误从 required 变为资源无效或成功。",
            )
        if service == "aitable":
            return (
                "后端/MCP schema",
                "修 aitable MCP wrapper 的参数校验和错误语义：不要返回 success=true+error；对 required 字段给出 CLI 可识别的参数名，系统错误要带 retryable/trace。",
                "MCP 修完后重跑该 aitable 命令，并确认 stdout JSON 不再出现 success=true 但 error 非空。",
            )
        return (
            "后端/MCP 服务",
            "优先修服务端/MCP 内部错误或 schema 映射；CLI 当前只能如实暴露错误，不应吞掉或伪造成功。",
            "服务端修复后按 trace_id/operation 重跑同一命令。",
        )

    if category == "backend-business-rule":
        return (
            "测试账号/业务规则",
            "按后端业务规则换账号或换场景；例如组织者不能响应自己的日程邀请，这不是 CLI bug。",
            "用满足业务角色的账号重跑。",
        )

    if category == "held":
        return (
            "人工安全确认",
            "该项涉及敏感读取或无安全负向目标，不适合自动用真实资源跑；需要在安全环境逐项人工确认。",
            f"人工确认后单独重跑 `{service} {command}`，并避免在报告中泄露密钥/凭证。",
        )

    if category == "input-or-business-validation":
        if "没有找到名称包含" in msg or "没搜到" in msg or "暂无" in msg or "没有已处理" in msg or "当前没有" in msg or "今天没有" in msg or "没有与你相关" in msg:
            return (
                "测试数据",
                "准备能命中的真实数据，或把 runner 查询词改成当前账号一定存在的对象；shortcut 本身不需要改。",
                "造数后重跑，预期从 validation empty result 变为成功。",
            )
        if "roomid invalid" in text or "会议室数量，超过上限100" in msg:
            return (
                "测试输入",
                "会议室类命令需要真实 roomId 或更小会议室分组；修改 runner 先定位会议室/分组，再喂给查询命令。",
                "用真实 roomId/分组重跑 calendar room/freebusy 命令。",
            )
        if "token decode create_at fail" in text:
            return (
                "测试输入",
                "calendarId 不能用占位/primary；runner 应先通过日历列表拿真实 calendarId，再用于 agenda。",
                "用真实 calendarId 重跑 `calendar +agenda`。",
            )
        if "unsupported setting key" in text:
            return (
                "测试输入/shortcut 枚举",
                "把测试输入的 setting-key 从 x 改为后端支持的 key；同时可在 shortcut flag 上补 enum，避免用户传非法 key。",
                "改 runner 后重跑；如果补 enum，跑 shortcut 单测确认校验文案。",
            )
        if "暂不支持保存该文字表情" in msg:
            return (
                "测试输入/业务规则",
                "换成后端支持的文字表情组合，或把该命令保留为业务负向；CLI 不应绕过后端限制。",
                "用一个真实可保存的表情模板重跑。",
            )
        if "opentaskid解密失败" in text:
            return (
                "测试数据",
                "消息发送状态查询需要真实 openTaskId；先真实发送/获取任务 ID，再作为 fixture 输入。",
                "用真实 openTaskId 重跑。",
            )
        if "taskid is null" in text:
            return (
                "后端/MCP schema",
                "CLI 已传 task-id 但后端看到 null，优先查 todo MCP 参数映射；若 schema 只认嵌套 request，需要同步修改 shortcut 参数投影。",
                "用 dry-run/fake MCP 对比真实 trace，确认 taskId 是否在 MCP 层丢失。",
            )
        if "system error" in text or "mcp_server_error" in text or "系统繁忙" in msg:
            return (
                "后端服务",
                "服务端返回系统繁忙/内部错误；CLI 侧可增加重试提示，但根因需要服务端修。",
                "稍后重跑并带 trace_id 给服务端。",
            )
        if "business error: success=false" in text or "business error: code" in text:
            return (
                "后端业务/测试 fixture",
                "后端只返回 success=false，信息不足；先准备真实合法 fixture，若仍无细节，需要后端补充错误码/错误信息。",
                "用真实资源重跑；若仍 success=false，把 operation+trace_id 给后端。",
            )
        return (
            "测试输入/业务校验",
            "当前命令已进入后端业务校验；先把测试输入换成真实合法 fixture，再判断是否需要改 shortcut。",
            f"重跑 `{service} {command}` 并比较 stdout/stderr。",
        )

    return (
        "待人工 triage",
        "当前分类不足，需要结合 trace_id、operation 和完整 stdout/stderr 单独查。",
        "先复现单条命令，再决定改 CLI、runner 还是后端。",
    )


def esc(value: Any) -> str:
    return html.escape(str(value), quote=True)


def table_rows(rows: list[dict[str, Any]], suite: str) -> list[str]:
    out: list[str] = []
    for idx, row in enumerate(rows, 1):
        owner, action, verify = action_for(row, suite)
        category = CATEGORY_LABELS.get(row.get("failure_category"), row.get("failure_category", "-"))
        cmd = f"{row.get('service')} {row.get('command')}"
        out.append(
            "<tr>"
            f"<td class=\"num\">{idx}</td>"
            f"<td><code>{esc(cmd)}</code></td>"
            f"<td><span class=\"risk\">{esc(row.get('risk') or '-')}</span></td>"
            f"<td><span class=\"cat\">{esc(category)}</span></td>"
            f"<td><span class=\"owner\">{esc(owner)}</span></td>"
            f"<td>{esc(action)}</td>"
            f"<td>{esc(verify)}</td>"
            f"<td><code>{esc(operation(row))}</code></td>"
            f"<td><code>{esc(trace_id(row))}</code></td>"
            f"<td class=\"evidence\">{esc(evidence(row))}</td>"
            "</tr>"
        )
    return out


def stat_cards(read: dict[str, Any], write: dict[str, Any], read_errors: list[dict[str, Any]], write_errors: list[dict[str, Any]]) -> str:
    all_errors = read_errors + write_errors
    owner_counts = Counter(action_for(row, "all")[0] for row in all_errors)
    category_counts = Counter(CATEGORY_LABELS.get(row.get("failure_category"), row.get("failure_category", "-")) for row in all_errors)
    cards = [
        ("Read 失败", read["summary"]["error"]),
        ("Write 失败", write["summary"]["error"]),
        ("逐项 review", len(all_errors)),
        ("测试数据/fixture", owner_counts.get("测试数据", 0)),
        ("权限/应用配置", owner_counts.get("权限/应用配置", 0)),
        ("后端/MCP schema", owner_counts.get("后端/MCP schema", 0)),
    ]
    card_html = "\n".join(f'<div class="stat"><div class="n">{esc(v)}</div><div class="l">{esc(k)}</div></div>' for k, v in cards)
    category_rows = "\n".join(
        f"<tr><td>{esc(k)}</td><td>{esc(v)}</td></tr>"
        for k, v in category_counts.most_common()
    )
    owner_rows = "\n".join(
        f"<tr><td>{esc(k)}</td><td>{esc(v)}</td></tr>"
        for k, v in owner_counts.most_common()
    )
    return f"""
<div class="stats">{card_html}</div>
<div class="summary-grid">
<section><h2>按错误类型</h2><table><thead><tr><th>类型</th><th>数量</th></tr></thead><tbody>{category_rows}</tbody></table></section>
<section><h2>按要改哪里</h2><table><thead><tr><th>要改哪里</th><th>数量</th></tr></thead><tbody>{owner_rows}</tbody></table></section>
</div>
"""


def render_table(title: str, rows: list[dict[str, Any]], suite: str) -> str:
    body = "\n".join(table_rows(rows, suite))
    return f"""
<h2>{esc(title)} <span class="count">· {len(rows)} 条</span></h2>
<table class="review">
<thead><tr>
<th>#</th><th>命令</th><th>风险</th><th>类型</th><th>要改哪里</th><th>具体改法</th><th>验证方式</th><th>operation</th><th>trace_id</th><th>证据</th>
</tr></thead>
<tbody>
{body}
</tbody>
</table>
"""


def render_resource_audit() -> str:
    rows = "\n".join(
        "<tr>"
        f"<td><code>{esc(cmd)}</code></td>"
        f"<td><span class=\"cat\">{esc(status)}</span></td>"
        f"<td>{esc(finding)}</td>"
        f"<td>{esc(next_step)}</td>"
        "</tr>"
        for cmd, status, finding, next_step in RESOURCE_AUDIT
    )
    return f"""
<h2>缺真实资源复盘 <span class="count">· 可自造/可查资源处理结果</span></h2>
<table>
<thead><tr><th>命令/范围</th><th>处理状态</th><th>本次实际排查/造数结果</th><th>后续建议</th></tr></thead>
<tbody>{rows}</tbody>
</table>
"""


def render() -> str:
    read = load(READ_PATH)
    write = load(WRITE_PATH)
    read_errors = [r for r in read["results"] if r.get("status") != "real-ok"]
    write_errors = [r for r in write["results"] if r.get("status") != "real-ok"]

    return f"""<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Shortcut 真实测试失败逐项 review</title>
<style>
:root{{--bg:#0f1420;--card:#151d2b;--line:#263246;--text:#dce7f7;--muted:#91a0b5;--blue:#8fd3ff;--green:#66d38a;--yellow:#e2b23c;--red:#f27272;--purple:#d3a7ff}}
*{{box-sizing:border-box}}
body{{margin:0;background:var(--bg);color:var(--text);font:13px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,"PingFang SC","Microsoft YaHei",sans-serif}}
header{{padding:28px 32px 14px;border-bottom:1px solid var(--line);background:linear-gradient(180deg,#172033,#0f1420)}}
h1{{margin:0 0 8px;font-size:26px}}
h2{{margin:28px 0 10px;font-size:18px}}
.sub,.note,.count{{color:var(--muted)}}
.wrap{{padding:18px 32px 40px;max-width:1800px;margin:0 auto}}
.note{{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:12px 14px;margin:10px 0 18px}}
.stats{{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:12px;margin:18px 0}}
.stat{{background:var(--card);border:1px solid var(--line);border-radius:12px;padding:14px}}
.stat .n{{font-size:24px;color:var(--blue);font-weight:700}}
.stat .l{{color:var(--muted);font-size:12px}}
.summary-grid{{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:18px;margin:16px 0 22px}}
table{{width:100%;border-collapse:collapse;background:var(--card);border:1px solid var(--line);border-radius:10px;overflow:hidden}}
th,td{{padding:8px 10px;border-bottom:1px solid var(--line);vertical-align:top;text-align:left}}
th{{background:#1b2536;color:var(--muted);font-size:12px;font-weight:600;position:sticky;top:0;z-index:1}}
tr:last-child td{{border-bottom:none}}
code{{font-family:"SF Mono",Menlo,Consolas,monospace;color:#c7cfdb;font-size:12px}}
.review{{table-layout:fixed}}
.review th:nth-child(1),.review td:nth-child(1){{width:44px}}
.review th:nth-child(2),.review td:nth-child(2){{width:165px}}
.review th:nth-child(3),.review td:nth-child(3){{width:70px}}
.review th:nth-child(4),.review td:nth-child(4){{width:120px}}
.review th:nth-child(5),.review td:nth-child(5){{width:130px}}
.review th:nth-child(8),.review td:nth-child(8){{width:130px}}
.review th:nth-child(9),.review td:nth-child(9){{width:180px}}
.review td{{word-break:break-word}}
.num{{color:var(--muted);text-align:right}}
.risk{{color:var(--green)}}
.cat{{color:var(--yellow)}}
.owner{{color:var(--purple)}}
.evidence{{color:#c7cfdb;font-size:12px}}
a{{color:var(--blue)}}
</style>
</head>
<body>
<header>
<h1>Shortcut 真实测试失败逐项 review</h1>
<div class="sub">由 <code>scripts/gen_shortcut_error_review.py</code> 从真实测试结果生成；目标是把每个失败项落到“应该改哪里”。</div>
</header>
<main class="wrap">
<div class="note">
Read：{esc(read['summary']['total'])} 条，成功 {esc(read['summary']['ok'])}，失败 {esc(read['summary']['error'])}，超时 {esc(read['summary']['timeout'])}。
Write：{esc(write['summary']['total'])} 条，成功 {esc(write['summary']['ok'])}，失败 {esc(write['summary']['error'])}，超时 {esc(write['summary']['timeout'])}。
判定口径：如果 fake MCP 已看到字段但真实后端仍报 required，按后端/MCP schema 映射处理；如果真实后端报资源无效/不存在，按 fixture 处理；权限类不在 CLI 中绕过。
</div>
{stat_cards(read, write, read_errors, write_errors)}
{render_resource_audit()}
{render_table("Read 失败逐项", read_errors, "read")}
{render_table("Write 失败逐项", write_errors, "write")}
</main>
</body>
</html>
"""


def main() -> int:
    OUT_PATH.write_text(render(), encoding="utf-8")
    print(f"written: {OUT_PATH}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
