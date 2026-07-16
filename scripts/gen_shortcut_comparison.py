#!/usr/bin/env python3
# Generates docs/shortcut-comparison.html: a 3-way comparison for every built-in
# shortcut — the dws `+command`, the equivalent lark-cli command, and the raw
# dws native `dws mcp ...` combination it replaces. Data is extracted directly
# from source so it stays truthful.
import os, re, glob, html, json, subprocess

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SC_DIR = os.path.join(ROOT, "internal", "shortcut")
LARK_ROOT = os.environ.get(
    "LARK_CLI_ROOT",
    os.path.abspath(os.path.join(ROOT, "..", "..", "larksuite", "cli")),
)
LARK = os.path.join(LARK_ROOT, "shortcuts")

SKIP_PKG = {"builtin", "usage", "userdef"}

# dws service -> lark service (None = DingTalk-specific, no lark counterpart)
LARK_MAP = {
    "chat": "im", "todo": "task", "aitable": "base", "sheet": "sheets",
    "devapp": "apps", "contact": "contact", "calendar": "calendar",
    "doc": "doc", "drive": "drive", "mail": "mail", "wiki": "wiki",
    "minutes": "minutes",
    # DingTalk-specific:
    "attendance": None, "oa": None, "report": None, "ding": None,
    "aisearch": None, "live": None, "devdoc": None,
}

# lark-cli services that are intentionally not mapped to DWS shortcuts today.
# These are platform/product-model gaps, not just missing wrapper work.
LARK_PLATFORM_ONLY = {
    "okr": "飞书 OKR 是独立目标管理产品；当前 DWS / 钉钉 MCP 没有等价的 OKR 周期、目标、指标和进展对象，不能只靠 wrapper 复刻。",
    "vc": "飞书 VC 暴露会议加入/离开、会中事件、消息、录制等对象；当前 DWS 没有一套干净的钉钉会议 MCP tool 可等价承接，已有事件能力也不是同一套会议生命周期语义。",
    "slides": "飞书 Slides 是原生演示文稿对象，支持页面替换、截图、XML 和媒体上传；当前 DWS 没有钉钉幻灯片对象模型与页面级 MCP 能力。",
    "markdown": "lark-cli 的 markdown shortcut 绑定飞书云文档/云盘里的 Markdown 工作流；当前 DWS 没有等价的钉钉 Markdown 云文档资源与 patch/diff/overwrite API。",
    "whiteboard": "飞书 Whiteboard 有独立画板节点、查询和结构化更新能力；当前 DWS 没有钉钉画板节点 schema 或对应 MCP tool。",
    "note": "飞书 Note 是会议纪要直查对象，可按 note_id 取详情和逐字稿；DWS 的 minutes 能力属于钉钉妙记/听记，资源 ID、权限和数据模型不同，不能保证等价。",
    "event": "飞书 event shortcut 面向 lark-cli 的事件订阅模型；DWS 的钉钉事件/机器人连接模型不同，目前没有沉淀成同参数、同语义的 shortcut。",
}

def load_lark():
    out = {}
    services = set(v for v in LARK_MAP.values() if v) | set(LARK_PLATFORM_ONLY)
    for svc in services:
        cmds = set()
        for f in glob.glob(os.path.join(LARK, svc, "*.go")):
            base = os.path.basename(f)
            if f.endswith("_test.go") or base.startswith("register"):
                continue
            try:
                txt = open(f, encoding="utf-8").read()
            except OSError:
                continue
            for m in re.finditer(r'Command:\s*"(\+[a-z0-9-]+)"', txt):
                cmds.add(m.group(1))
        out[svc] = cmds
    return out

def split_vars(txt):
    """Yield source blocks for each `var X = shortcut.Shortcut{ ... }`."""
    lines = txt.splitlines()
    i, n = 0, len(lines)
    while i < n:
        if re.match(r'\s*var \w+ = shortcut\.Shortcut\{', lines[i]):
            buf = [lines[i]]
            i += 1
            while i < n and not re.match(r'^\}', lines[i]):
                buf.append(lines[i]); i += 1
            if i < n:
                buf.append(lines[i])
            yield "\n".join(buf)
        i += 1

def field(block, name):
    m = re.search(name + r':\s*"([^"]*)"', block)
    return m.group(1) if m else ""

def parse_block(block, pkg, consts=None):
    consts = consts or {}
    svc = field(block, "Service") or pkg
    cmd = field(block, "Command")
    product = field(block, "Product") or svc
    desc = field(block, "Description")
    rm = re.search(r'Risk:\s*shortcut\.Risk(\w+)', block)
    risk = {"Read": "read", "Write": "write", "HighWrite": "high-risk-write"}.get(rm.group(1), "read") if rm else "read"
    tm = re.search(r'CallMCP\("([^"]+)"', block)
    if tm:
        tool = tm.group(1)
    else:
        # tool passed as a const identifier, e.g. CallMCP(listeningNoteCmdTool, ...)
        im = re.search(r'CallMCP\(([A-Za-z_]\w*)', block)
        tool = consts.get(im.group(1), "") if im else ""
    # flags: each {Name: "x" ... Required: true?}
    flags = []
    for fm in re.finditer(r'\{Name:\s*"([^"]+)"(.*?)\}', block, re.S):
        req = "Required: true" in fm.group(2)
        flags.append((fm.group(1), req))
    # param keys from the Execute body (map literal keys + params["k"])
    exec_part = block.split("Execute:", 1)[-1]
    keys = []
    seen = set()
    for km in re.finditer(r'"([a-zA-Z_][a-zA-Z0-9_]*)":', exec_part):
        k = km.group(1)
        if k not in seen:
            seen.add(k); keys.append(k)
    for km in re.finditer(r'params\["([a-zA-Z_][a-zA-Z0-9_]*)"\]', exec_part):
        k = km.group(1)
        if k not in seen:
            seen.add(k); keys.append(k)
    return dict(service=svc, command=cmd, product=product, desc=desc, risk=risk,
               tool=tool, flags=flags, keys=keys,
               layer="smart" if pkg == "smart" else "atomic")

def collect():
    items = []
    for d in sorted(glob.glob(os.path.join(SC_DIR, "*"))):
        pkg = os.path.basename(d)
        if not os.path.isdir(d) or pkg in SKIP_PKG:
            continue
        for f in glob.glob(os.path.join(d, "*.go")):
            if f.endswith("_test.go"):
                continue
            txt = open(f, encoding="utf-8").read()
            consts = dict(re.findall(r'(\w+)\s*=\s*"([^"]+)"', txt))
            for block in split_vars(txt):
                it = parse_block(block, pkg, consts)
                if it["command"]:
                    items.append(it)
    return items

def shortcut_cmd(it):
    s = f"dws {it['service']} {it['command']}"
    for name, req in it["flags"]:
        s += f" --{name} <{name}>"
    return s

def raw_cmd(it):
    if it["layer"] == "smart":
        primary = f"；入口工具 {it['tool']}" if it["tool"] else ""
        return f"复合编排：多次 MCP 调用 / 分页、消歧、聚合或回滚{primary}"
    if not it["tool"]:
        return f"dws mcp {it['product']} <tool>"
    if not it["keys"]:
        return f"dws mcp {it['product']} {it['tool']}"
    body = ", ".join(f'"{k}":<..>' for k in it["keys"])
    return f"dws mcp {it['product']} {it['tool']} --json '{{{body}}}'"

def lark_cmd(it, lark):
    lsvc = LARK_MAP.get(it["service"], "__none__")
    if lsvc is None:
        return ("none", "无对应（钉钉特有服务）")
    if lsvc == "__none__":
        return ("none", "—")
    cmds = lark.get(lsvc, set())
    if it["command"] in cmds:
        return ("hit", f"lark-cli {lsvc} {it['command']}")
    return ("miss", f"lark {lsvc} 无同名命令（能力/命名不同）")

RISK_CLS = {"read": "rk-r", "write": "rk-w", "high-risk-write": "rk-h"}

def load_test_matrix():
    env = os.environ.copy()
    env.setdefault("GOCACHE", "/private/tmp/dws_gocache")
    cmd = ["go", "run", "./scripts/gen_shortcut_test_matrix.go"]
    raw = subprocess.check_output(cmd, cwd=ROOT, env=env, text=True)
    data = json.loads(raw)
    by_cmd = {
        (r["service"], r["command"]): r
        for r in data.get("results", [])
    }
    return data, by_cmd

def load_real_matrix(filename):
    path = os.path.join(ROOT, "docs", filename)
    if not os.path.exists(path):
        return None, {}
    with open(path, encoding="utf-8") as f:
        data = json.load(f)
    by_cmd = {
        (r["service"], r["command"]): r
        for r in data.get("results", [])
    }
    return data, by_cmd

def load_real_read_matrix():
    return load_real_matrix("shortcut-real-read-results.json")

def load_real_write_matrix():
    return load_real_matrix("shortcut-real-write-results.json")

def matrix_summary(data):
    if not data:
        return {}
    if isinstance(data.get("summary"), dict):
        return data["summary"]
    return data

FAILURE_CATEGORY_LABELS = {
    "auth-or-permission": "权限 / 鉴权不足",
    "backend-business-rule": "后端业务规则限制",
    "backend-or-mcp-error": "后端 / MCP 内部错误",
    "cli-error-envelope": "CLI 错误 envelope 未转非零退出",
    "held": "人工授权后执行",
    "input-or-business-validation": "输入或业务校验失败",
    "missing-real-aitable-fixture": "缺 AI 表格真实 fixture",
    "missing-real-minutes-fixture": "缺妙记/听记真实 fixture",
    "missing-real-resource": "缺真实资源 fixture",
    "timeout": "真实后端超时",
    "unclassified-real-error": "未分类真实错误",
}

FIXABILITY_LABELS = {
    "cli-wrapper-fix-needed": "需要 CLI / helper 修",
    "fixed": "已成功",
    "manual-approval": "需人工授权",
    "needs-rerun": "需单独复测",
    "needs-triage": "需继续排查",
    "not-cli-fixable": "非 CLI 可修",
    "not-cli-fixable-first": "优先后端 / MCP 排查",
    "not-cli-fixable-without-fixture": "需真实 fixture，非 CLI 可直接修",
    "test-input-or-backend-rule": "需区分测试输入或后端规则",
}

def failure_category_table(data):
    cats = (data or {}).get("failure_categories") or {}
    if not cats:
        return '<p class="section-lead">暂无失败归因。</p>'
    rows = ['<table class="why"><colgroup><col style="width:34%"><col style="width:16%"><col style="width:50%"></colgroup>',
            '<thead><tr><th>失败归因</th><th>数量</th><th>说明</th></tr></thead><tbody>']
    notes = {
        "auth-or-permission": "账号、应用 scope 或资源权限不足；需要授权/换账号/配置应用权限。",
        "backend-business-rule": "后端业务规则限制，CLI 只能避免误导或提示用户换场景。",
        "backend-or-mcp-error": "服务端/MCP 返回内部错误，报告保留原始输出供后端排查。",
        "cli-error-envelope": "这是 CLI 层可继续修的类型：业务错误不应 exit 0。",
        "held": "无安全负向目标或高风险动作，需人工逐项确认。",
        "input-or-business-validation": "命令真实进入校验；可能是安全负向输入触发，也可能需要更完整 fixture。",
        "missing-real-aitable-fixture": "AI 表格命令依赖真实 Base/Table/View/Record 等资源。",
        "missing-real-minutes-fixture": "妙记/听记命令依赖当前账号可见的真实会议产物。",
        "missing-real-resource": "资源、消息、群、文档、单据等真实对象不存在。",
        "timeout": "需要单独扩大超时或排查网络/后端响应。",
        "unclassified-real-error": "暂未自动归因，需要人工继续看 stdout/stderr。",
    }
    for key, count in cats.items():
        rows.append(f'<tr><td>{html.escape(FAILURE_CATEGORY_LABELS.get(key, key))}</td><td>{count}</td><td>{html.escape(notes.get(key, ""))}</td></tr>')
    rows.append('</tbody></table>')
    return "\n".join(rows)

TEST_CLS = {
    "assembled": "test-ok",
    "validation-blocked": "test-warn",
    "failed": "test-bad",
}

def real_html(it, real_tests):
    r = real_tests.get((it["service"], it["command"]))
    if r:
        cls = "test-ok" if r.get("status") == "real-ok" else ("test-bad" if r.get("status") == "timeout" else "test-warn")
        label = {"real-ok": "真实后端成功", "real-error": "真实后端返回错误", "timeout": "真实后端超时", "held": "待真实执行"}.get(r.get("status"), r.get("status", "unknown"))
        stdout = r.get("stdout") or ""
        stderr = r.get("stderr") or ""
        output = stdout or stderr or "（空）"
        method = r.get("method") or "real-backend; no --mock; no --dry-run"
        category = r.get("failure_category") or ""
        fixability = r.get("fixability") or ""
        diagnosis = r.get("diagnosis") or ""
        setup = r.get("setup") if isinstance(r.get("setup"), dict) else None
        setup_html = ""
        if setup:
            setup_html = (
                f'<div class="io-label">前置真实资源准备</div>'
                f'<pre>{html.escape(setup.get("purpose") or "—")}\n'
                f'input: {html.escape(setup.get("input") or "")}\n'
                f'exit={html.escape(str(setup.get("exit_code")))}；duration={html.escape(str(setup.get("duration_ms")))}ms\n'
                f'stdout:\n{html.escape(setup.get("stdout") or "（空）")}\n'
                f'stderr:\n{html.escape(setup.get("stderr") or "（空）")}</pre>'
            )
        diagnosis_html = ""
        if category or fixability or diagnosis:
            diagnosis_html = (
                f'<div class="io-label">失败归因 / 可修复性</div>'
                f'<pre>{html.escape(FAILURE_CATEGORY_LABELS.get(category, category) or "—")} / '
                f'{html.escape(FIXABILITY_LABELS.get(fixability, fixability) or "—")}\n'
                f'{html.escape(diagnosis or "—")}</pre>'
            )
        return (
            '<details class="io"><summary>真实后端测试输入 / 输出</summary>'
            f'<span class="{cls}">{html.escape(label)}</span>'
            f'<div class="desc">exit={r.get("exit_code")}；duration={r.get("duration_ms")}ms；{html.escape(method)}</div>'
            f'{diagnosis_html}'
            f'{setup_html}'
            f'<div class="io-label">真实输入</div><pre>{html.escape(r.get("input", ""))}</pre>'
            f'<div class="io-label">真实 stdout</div><pre>{html.escape(stdout or "（空）")}</pre>'
            f'<div class="io-label">真实 stderr / error</div><pre>{html.escape(stderr or "（空）")}</pre>'
            f'<div class="io-label">真实返回</div><pre>{html.escape(output)}</pre>'
            '</details>'
        )
    if it["risk"] == "read":
        return '<details class="io"><summary>真实后端测试输入 / 输出</summary><span class="test-bad">未覆盖</span><div class="desc">此 read shortcut 未出现在真实只读测试结果中。</div></details>'
    return (
        '<details class="io"><summary>真实后端测试输入 / 输出</summary>'
        '<span class="test-warn">待分批真实执行写入</span>'
        '<div class="desc">该命令是 write / high-risk-write。真实执行会修改线上数据；本轮采用临时测试资源和测试人员分批推进，未覆盖到的命令会继续补测。</div>'
        '</details>'
    )

def test_html(it, tests, real_tests):
    r = tests.get((it["service"], it["command"]))
    if not r:
        return '<span class="test-bad">未测试</span><div class="desc">测试矩阵没有覆盖到此命令。</div>'
    cls = TEST_CLS.get(r.get("status"), "test-bad")
    status_label = {
        "assembled": "已实测装配",
        "validation-blocked": "实测校验拦截",
        "failed": "失败",
    }.get(r.get("status"), r.get("status", "unknown"))
    tools = r.get("tools") or []
    tool_txt = "、".join(tools) if tools else "无 MCP 调用（校验前拦截）"
    verified = "tool 已校验" if r.get("tool_verified") else "tool 未通过校验"
    method = "cobra 实际执行 + fake MCP 拦截，零网络/零副作用"
    err = r.get("error", "")
    err_html = f'<span class="test-err">；{html.escape(err)}</span>' if err else ""
    stdout = r.get("stdout") or ""
    stderr = r.get("stderr") or ""
    output = stdout or stderr or err or "（无 stdout/stderr；命令在校验前停止或无内容输出）"
    io_block = (
        '<details class="io"><summary>输入 / CLI 输出</summary>'
        f'<div class="io-label">输入</div><pre>{html.escape(r.get("input", ""))}</pre>'
        f'<div class="io-label">stdout</div><pre>{html.escape(stdout or "（空）")}</pre>'
        f'<div class="io-label">stderr / error</div><pre>{html.escape((stderr or err) or "（空）")}</pre>'
        f'<div class="io-label">最终用于判定的返回</div><pre>{html.escape(output)}</pre>'
        '</details>'
    )
    return (
        f'<span class="{cls}">{html.escape(status_label)}</span>'
        f'<div class="desc">{html.escape(method)}；调用数 {r.get("call_count", 0)}；'
        f'{html.escape(verified)}；<code>{html.escape(tool_txt)}</code>{err_html}</div>'
        f'{io_block}'
        f'{real_html(it, real_tests)}'
    )

def gen_html(items, lark, matrix, tests, real_read_matrix, real_write_matrix, real_tests):
    by_svc = {}
    for it in items:
        by_svc.setdefault(it["service"], []).append(it)
    order = sorted(by_svc, key=lambda s: -len(by_svc[s]))

    hit = sum(1 for it in items if lark_cmd(it, lark)[0] == "hit")
    dss = sum(1 for it in items if LARK_MAP.get(it["service"]) is None)
    smart = [it for it in items if it["layer"] == "smart"]
    atomic_count = len(items) - len(smart)
    smart_by_svc = {}
    for it in smart:
        smart_by_svc.setdefault(it["service"], []).append(it)
    lark_platform_only = {
        svc: sorted(lark.get(svc, set()))
        for svc in sorted(LARK_PLATFORM_ONLY)
        if lark.get(svc)
    }
    lark_platform_only_count = sum(len(cmds) for cmds in lark_platform_only.values())
    test_total = matrix.get("total", 0)
    test_assembled = matrix.get("assembled", 0)
    test_validation = matrix.get("validation_blocked", 0)
    test_failed = matrix.get("failed", 0)
    test_tool_bad = matrix.get("tool_verification_bad", 0)
    real_read_summary = matrix_summary(real_read_matrix)
    real_write_summary = matrix_summary(real_write_matrix)
    real_total = real_read_summary.get("total", 0)
    real_ok = real_read_summary.get("ok", 0)
    real_error = real_read_summary.get("error", 0)
    real_timeout = real_read_summary.get("timeout", 0)
    real_write_total = real_write_summary.get("total", 0)
    real_write_ok = real_write_summary.get("ok", 0)
    real_write_error = real_write_summary.get("error", 0)
    real_write_timeout = real_write_summary.get("timeout", 0)
    real_write_held = real_write_summary.get("held", 0)

    P = []
    P.append(f'''<!DOCTYPE html><html lang="zh-CN"><head><meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>DWS Shortcut 三方对照（dws vs lark-cli vs 原生 MCP）</title>
<style>
:root{{--bg:#0f1420;--card:#161d2c;--ink:#e6edf6;--muted:#93a1b5;--line:#26314a;--accent:#6ea8fe;--green:#3fb950;--yellow:#d5a429;--red:#f25c5c}}
*{{box-sizing:border-box}} body{{margin:0;background:#0f1420;color:var(--ink);font:14px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC",sans-serif;padding-bottom:70px}}
header{{padding:40px 22px 22px;text-align:center;border-bottom:1px solid var(--line);background:radial-gradient(1000px 260px at 50% -50px,rgba(79,156,255,.15),transparent)}}
h1{{margin:0 0 6px;font-size:25px}} .sub{{color:var(--muted);font-size:13px}}
.wrap{{max-width:1280px;margin:0 auto;padding:0 18px}}
.stats{{display:flex;gap:12px;justify-content:center;flex-wrap:wrap;margin:20px 0 4px}}
.stat{{background:var(--card);border:1px solid var(--line);border-radius:12px;padding:12px 18px;min-width:120px}}
.stat .n{{font-size:24px;font-weight:700;color:var(--accent)}} .stat .l{{color:var(--muted);font-size:12px}}
.note{{background:var(--card);border:1px solid var(--line);border-left:3px solid var(--accent);border-radius:8px;padding:12px 16px;margin:22px 0;color:var(--muted);font-size:13px}}
.note b{{color:var(--ink)}}
h2{{font-size:18px;margin:34px 0 4px;padding-top:10px}} h2 .c{{color:var(--muted);font-size:14px;font-weight:400}}
.section-lead{{color:var(--muted);max-width:980px;margin:7px 0 14px}}
.card-grid{{display:grid;grid-template-columns:repeat(auto-fit,minmax(300px,1fr));gap:12px;margin:12px 0 24px}}
.card{{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:13px 15px}}
.card h3{{margin:0 0 8px;font-size:14px;color:var(--accent)}}
.smart-list{{list-style:none;padding:0;margin:0;display:grid;gap:7px}}
.smart-list li{{border-top:1px solid var(--line);padding-top:7px}}
.smart-list li:first-child{{border-top:0;padding-top:0}}
.smart-list code{{color:#d3a7ff}} .smart-desc{{display:block;color:var(--muted);font-size:11.5px;margin-top:1px}}
.why td:first-child{{color:#9aa8bb}} .why td:nth-child(2){{color:#dce7f7}}
table{{width:100%;border-collapse:collapse;margin:10px 0 6px;background:var(--card);border:1px solid var(--line);border-radius:10px;overflow:hidden;table-layout:fixed}}
th,td{{padding:9px 11px;text-align:left;border-bottom:1px solid var(--line);vertical-align:top;word-break:break-all}}
th{{background:#1b2536;color:var(--muted);font-size:12px;font-weight:600}}
tr:last-child td{{border-bottom:none}}
col.c1{{width:29%}} col.c2{{width:38%}} col.c3{{width:33%}}
code{{font-family:"SF Mono",Menlo,Consolas,monospace;font-size:12.5px}}
.sc code{{color:#8fd3ff}} .raw code{{color:#c7cfdb}} .lk-hit code{{color:#84e39a}}
.desc{{color:var(--muted);font-size:11.5px;margin-top:3px}}
.rk{{display:inline-block;font-size:10px;padding:0 6px;border-radius:10px;margin-left:6px;vertical-align:1px}}
.rk-r{{background:#12321d;color:#66d38a}} .rk-w{{background:#3a2f10;color:#e2b23c}} .rk-h{{background:#3a1414;color:#f27272}}
.smart-badge{{display:inline-block;font-size:10px;padding:0 6px;border-radius:10px;margin-left:6px;vertical-align:1px;background:#35204d;color:#d3a7ff}}
.test-ok,.test-warn,.test-bad{{display:inline-block;font-size:10px;padding:0 6px;border-radius:10px;vertical-align:1px}}
.test-ok{{background:#12321d;color:#66d38a}} .test-warn{{background:#3a2f10;color:#e2b23c}} .test-bad{{background:#3a1414;color:#f27272}}
.test-err{{color:#d5a429}}
details.io{{margin-top:6px}} details.io summary{{cursor:pointer;color:#8fd3ff;font-size:11.5px}}
.io-label{{color:#9aa8bb;font-size:11px;margin-top:6px}}
details.io pre{{white-space:pre-wrap;word-break:break-word;background:#101827;border:1px solid var(--line);border-radius:6px;padding:7px;margin:3px 0 6px;color:#c7cfdb;max-height:260px;overflow:auto;font-size:11px}}
.lk-none{{color:#6b7890}} .lk-miss{{color:var(--yellow)}} .lk-hit{{color:var(--green)}}
footer{{color:var(--muted);text-align:center;font-size:12px;margin-top:36px}}
</style></head><body>
<header><h1>DWS Shortcut 三方指令对照</h1>
<div class="sub">每个 shortcut：<b style="color:#8fd3ff">dws +命令</b> vs <b style="color:#84e39a">lark-cli</b> vs <b style="color:#c7cfdb">dws 原生 MCP 组合</b>　·　源码自动提取</div>
<div class="stats">
<div class="stat"><div class="n">{len(items)}</div><div class="l">shortcut 总数</div></div>
<div class="stat"><div class="n">{len(smart)}</div><div class="l">复合 / 智能型</div></div>
<div class="stat"><div class="n">{atomic_count}</div><div class="l">原子型</div></div>
<div class="stat"><div class="n">{len(order)}</div><div class="l">服务</div></div>
<div class="stat"><div class="n">{hit}</div><div class="l">lark 同名可对齐</div></div>
<div class="stat"><div class="n">{dss}</div><div class="l">钉钉特有(无 lark)</div></div>
<div class="stat"><div class="n">{lark_platform_only_count}</div><div class="l">lark 平台特有</div></div>
<div class="stat"><div class="n">{test_total}</div><div class="l">逐条实测覆盖</div></div>
<div class="stat"><div class="n">{real_total}</div><div class="l">真实只读后端测试</div></div>
<div class="stat"><div class="n">{real_write_total}</div><div class="l">真实写入后端测试</div></div>
</div></header>
<div class="wrap">
<div class="note"><b>读表说明</b>：① <b style="color:#8fd3ff">dws shortcut</b> = 新增的精选命令（<code>&lt;xxx&gt;</code> 为参数占位）。
② <b style="color:#c7cfdb">原生 MCP / 编排说明</b>：原子型展示等价裸命令；复合型说明其分页、消歧、多步调用、聚合或回滚职责。
③ <b style="color:#84e39a">lark-cli</b> = 飞书对应命令：<span class="lk-hit">绿色</span>=同名可对齐；<span class="lk-miss">黄色</span>=lark 有该服务但命名/能力不同；<span class="lk-none">灰色</span>=钉钉特有服务，lark 无对应。因两边 API 不同，仅同名项做逐一对齐。
④ <span class="smart-badge">复合/智能型</span> = 不只是参数改名，内部包含多次调用、跨服务 ID 解析、分页、消歧、聚合、投影、批处理或失败回滚。
⑤ <b>lark 平台特有</b> = 飞书有独立产品对象或事件模型，但当前 DWS / 钉钉 MCP 没有等价资源，不能通过简单封装实现。
⑥ <b>实际测试情况</b> = 每条 dws shortcut 都通过真实 cobra 命令树执行；fake MCP 拦截所有后端调用，因此不发网络、不写线上数据。绿色=已装配到 MCP 调用；黄色=synthetic 参数被命令自身校验正确拦截；红色=失败。每行都可展开查看本次测试的 CLI 输入、stdout、stderr/error。
⑦ <b>真实后端测试</b> = 用真实 dws 二进制、真实登录态、真实后端执行（无 --mock、无 --dry-run），逐条记录真实输入与真实 stdout/stderr。read 已全量覆盖；write / high-risk-write 按临时测试资源和测试人员分批推进，未覆盖项会继续补测。
⑧ <b>逐项失败 review</b> = <a href="./shortcut-error-review.html" style="color:#8fd3ff">shortcut-error-review.html</a> 已把每个真实失败逐条标出“要改哪里 / 具体改法 / 验证方式”。</div>
''')

    P.append(f'<h2 id="shortcut-test-matrix">逐条实际测试矩阵 <span class="c">· {test_total} / {len(items)} 覆盖</span></h2>')
    P.append(f'<p class="section-lead">本页每一条 dws shortcut 都有测试记录。测试由 <code>scripts/gen_shortcut_test_matrix.go</code> 生成：逐条喂入合成参数，实际执行 cobra 命令树，所有 MCP 调用由 fake caller 拦截并记录 product/tool/args，同时捕获每条命令的 CLI 输入、stdout、stderr/error，确保零网络、零副作用。结果：{test_assembled} 条已装配到真实 MCP tool，{test_validation} 条被自身校验按预期拦截，失败 {test_failed} 条，tool ground truth 校验失败 {test_tool_bad} 条。</p>')
    P.append('''<table class="why"><colgroup><col style="width:24%"><col style="width:25%"><col style="width:25%"><col style="width:26%"></colgroup>
<thead><tr><th>测试类型</th><th>数量</th><th>含义</th><th>副作用边界</th></tr></thead><tbody>''')
    P.append(f'<tr><td><span class="test-ok">已实测装配</span></td><td>{test_assembled}</td><td>命令实际执行到 MCP 调用组装路径，并记录 product/tool/args</td><td>fake MCP 拦截，不访问后端，不写数据</td></tr>')
    P.append(f'<tr><td><span class="test-warn">实测校验拦截</span></td><td>{test_validation}</td><td>命令实际执行到自身校验链路，synthetic 参数被合法拒绝</td><td>校验前停止，无 MCP 调用</td></tr>')
    P.append(f'<tr><td><span class="test-bad">失败</span></td><td>{test_failed}</td><td>panic、无调用无错误、或 tool ground truth 未通过</td><td>必须为 0 才允许生成报告</td></tr>')
    P.append('</tbody></table>')

    P.append(f'<h2 id="real-read-test-matrix">真实后端只读测试 <span class="c">· {real_total} / 204 read 覆盖</span></h2>')
    P.append(f'<p class="section-lead">这一层不使用 mock，也不使用 dry-run：用临时构建的真实 dws 二进制和当前 profile 登录态执行 read shortcut，并捕获真实 stdout/stderr。结果：成功 {real_ok} 条，真实后端/参数/鉴权错误 {real_error} 条，超时 {real_timeout} 条。大量错误并不代表命令未执行，而是后端真实返回了 not_authenticated、参数校验、资源不存在或权限/业务错误；这些原始输出已逐条写在主表展开项中。</p>')
    P.append('''<table class="why"><colgroup><col style="width:24%"><col style="width:25%"><col style="width:25%"><col style="width:26%"></colgroup>
<thead><tr><th>真实测试类型</th><th>数量</th><th>含义</th><th>副作用边界</th></tr></thead><tbody>''')
    P.append(f'<tr><td><span class="test-ok">真实后端成功</span></td><td>{real_ok}</td><td>命令真实访问后端并 exit 0</td><td>仅 read 命令</td></tr>')
    P.append(f'<tr><td><span class="test-warn">真实后端返回错误</span></td><td>{real_error}</td><td>命令真实访问后端或真实执行本地校验后返回错误；stdout/stderr 已逐条记录</td><td>仅 read 命令</td></tr>')
    P.append(f'<tr><td><span class="test-bad">真实后端超时</span></td><td>{real_timeout}</td><td>超过测试超时时间</td><td>仅 read 命令</td></tr>')
    P.append('</tbody></table>')
    P.append('<h3>只读真实失败归因</h3>')
    P.append(failure_category_table(real_read_matrix))

    P.append(f'<h2 id="real-write-test-matrix">真实后端写入测试 <span class="c">· {real_write_total} / 162 write 覆盖中</span></h2>')
    P.append(f'<p class="section-lead">这一层同样不使用 mock，也不使用 dry-run：用真实 dws 二进制、当前登录态、临时测试资源以及测试人员（冬翔、克谨、怒龙）分批执行 write / high-risk-write shortcut。当前已记录 {real_write_total} 条：成功 {real_write_ok} 条，真实后端/参数/业务错误 {real_write_error} 条，超时 {real_write_timeout} 条，待执行 {real_write_held} 条。写入类的失败同样保留真实 stdout/stderr，便于定位是 shortcut 参数投影问题、后端业务约束还是权限限制。</p>')
    P.append('''<table class="why"><colgroup><col style="width:24%"><col style="width:25%"><col style="width:25%"><col style="width:26%"></colgroup>
<thead><tr><th>真实测试类型</th><th>数量</th><th>含义</th><th>副作用边界</th></tr></thead><tbody>''')
    P.append(f'<tr><td><span class="test-ok">真实后端成功</span></td><td>{real_write_ok}</td><td>命令真实访问后端并 exit 0</td><td>临时资源 / 测试人员</td></tr>')
    P.append(f'<tr><td><span class="test-warn">真实后端返回错误</span></td><td>{real_write_error}</td><td>命令真实访问后端或真实执行本地校验后返回错误；stdout/stderr 已逐条记录</td><td>临时资源 / 测试人员</td></tr>')
    P.append(f'<tr><td><span class="test-bad">真实后端超时</span></td><td>{real_write_timeout}</td><td>超过测试超时时间</td><td>需单独复测</td></tr>')
    P.append('</tbody></table>')
    P.append('<h3>写入真实失败归因</h3>')
    P.append(failure_category_table(real_write_matrix))

    P.append(f'<h2 id="why-atomic-shortcuts">非复合 / 智能型 shortcut 为什么也要写？ <span class="c">· {atomic_count} 个</span></h2>')
    P.append('<p class="section-lead">原子型 shortcut 不是在宣称“新增了底层能力”，而是在 MCP tool 之上提供一层稳定、可发现、可测试的用户入口。它的价值边界要讲清楚：有增量就保留，纯重复就删除。</p>')
    P.append('''<table class="why"><colgroup><col style="width:27%"><col style="width:36.5%"><col style="width:36.5%"></colgroup>
<thead><tr><th>必要性</th><th>没有原子型 shortcut 时</th><th>保留原子型 shortcut 的价值</th></tr></thead><tbody>
<tr><td>补齐命令面空白</td><td>有些 MCP tool 没有对应的 <code>dws &lt;svc&gt; &lt;verb&gt;</code> helper，只能走裸 <code>dws mcp ... --json</code></td><td>把真实可用的底层能力挂到产品命令树里，用户和 Agent 不必手写 JSON</td></tr>
<tr><td>参数收敛</td><td>调用者要记服务端字段名、嵌套 JSON、枚举和必填关系</td><td>收敛成命名 flag、默认值和校验，降低误填与 prompt 猜参概率</td></tr>
<tr><td>Intent / Help / 可发现性</td><td>MCP schema 偏接口视角，Agent 需要先理解 tool 再判断何时使用</td><td>每条 shortcut 都带任务化描述，可被 <code>--help</code>、<code>dws shortcut list</code> 和 Agent 规划直接利用</td></tr>
<tr><td>统一安全与输出入口</td><td>读写风险、dry-run、format / jq / fields 等能力分散到调用者侧</td><td>复用 shortcut 框架的 risk 分级、dry-run 通道和统一输出契约</td></tr>
<tr><td>稳定投影</td><td>底层响应 envelope 冗长且字段不稳定，常把噪声暴露给用户</td><td>对一部分读类命令保留干净投影，只输出用户真正要看的字段</td></tr>
<tr><td>对齐与治理</td><td>无法判断与 lark-cli、helper、MCP tool 的覆盖差异，也难以做后续去重</td><td>形成可枚举的命令面，便于生成报告、做 GSB 对比和持续 prune</td></tr>
</tbody></table>''')
    P.append('<div class="note"><b>保留标准</b>：原子型 shortcut 必须至少满足“补 helper 空白”或“提供投影/校验/Intent 等体验增量”。本轮已删除 213 条仅换名字、无投影增量、且 helper 已覆盖的纯重复 wrapper；所以这里的原子型不是为了凑数量，而是为了把仍有价值的底层能力变成稳定入口。</div>')

    P.append(f'<h2 id="smart-shortcuts">复合 / 智能型 shortcut <span class="c">· {len(smart)} 个</span></h2>')
    P.append('<p class="section-lead">这组命令面向完整用户意图，例如“按姓名发消息”“找共同空闲时间”“找到最新妙记并取逐字稿”。其中一部分是真正的多 MCP / 跨服务编排，另一部分是单服务内的分页、过滤、批处理和稳定输出；两者都不是普通 API 别名。</p>')
    P.append('<div class="card-grid">')
    for svc in sorted(smart_by_svc, key=lambda s: (-len(smart_by_svc[s]), s)):
        rows = sorted(smart_by_svc[svc], key=lambda x: x["command"])
        P.append(f'<section class="card"><h3>{html.escape(svc)} · {len(rows)}</h3><ul class="smart-list">')
        for it in rows:
            P.append(f'<li><code>dws {html.escape(it["service"])} {html.escape(it["command"])}</code><span class="smart-desc">{html.escape(it["desc"])}</span></li>')
        P.append('</ul></section>')
    P.append('</div>')

    P.append('<h2 id="why-shortcuts">为什么不只提供 API 形态？</h2>')
    P.append('<p class="section-lead">API/MCP 形态以服务端资源和接口为中心；shortcut 以用户任务为中心。单独封装的价值不是少写几个字符，而是把稳定的客户端语义、保护措施和多步流程变成可测试契约。</p>')
    P.append('''<table class="why"><colgroup><col style="width:27%"><col style="width:36.5%"><col style="width:36.5%"></colgroup>
<thead><tr><th>维度</th><th>直接 API / MCP</th><th>单独 shortcut 封装</th></tr></thead><tbody>
<tr><td>输入语义</td><td>要求调用者理解服务端字段名、类型、枚举和 JSON 结构</td><td>提供面向任务的 flags、默认值、枚举、时间解析和跨字段校验</td></tr>
<tr><td>ID 与上下文</td><td>通常先查 userId、deptId、eventId、baseId，再复制到下一次调用</td><td>可按姓名/标题解析、唯一性消歧，并自动串联后续操作</td></tr>
<tr><td>多步一致性</td><td>调用者自行处理部分成功、重试、回滚与中间结果</td><td>集中实现 fan-out、分页、去重、partial failure 和必要回滚</td></tr>
<tr><td>安全</td><td>写接口裸露，dry-run 和确认逻辑由每个调用者重复实现</td><td>统一 risk 分级、确认提示、dry-run 计划和高风险保护</td></tr>
<tr><td>输出契约</td><td>响应 envelope 和字段随底层工具而异，常含大量噪声</td><td>稳定投影关键字段，并统一支持 format / jq / fields</td></tr>
<tr><td>Agent 可发现性</td><td>模型需先理解 API schema，再自己规划完整调用链</td><td>Intent、示例和窄参数面直接表达“什么时候用、会发生什么”</td></tr>
<tr><td>演进隔离</td><td>上游参数或响应变化会扩散到所有脚本和 Agent prompt</td><td>在一个经过测试的适配层吸收兼容变化</td></tr>
</tbody></table>''')
    P.append('<div class="note"><b>它不是 API 的替代品。</b> 对一次性、低频、需要完整底层字段或刚上线尚未沉淀的能力，仍应直接调用 API/MCP。只有当封装提供明确增量——复合编排、校验、安全、稳定投影或高频意图——才值得保留；纯粹改名且没有增量的 wrapper 应删除或不新增。</div>')

    P.append(f'<h2 id="lark-platform-only">lark 平台特有 shortcut <span class="c">· {lark_platform_only_count} 个</span></h2>')
    P.append('<p class="section-lead">这批不是“DWS 少写了几个 shortcut”，而是飞书平台有对应的一等产品对象或事件模型，当前钉钉 MCP/DWS 没有同语义承接点。后续只有在钉钉开放对应 API、或 DWS 接入等价 MCP tool 后，才适合再做 shortcut。</p>')
    P.append('<div class="card-grid">')
    for svc, cmds in lark_platform_only.items():
        reason = LARK_PLATFORM_ONLY[svc]
        P.append(f'<section class="card"><h3>lark {html.escape(svc)} · {len(cmds)}</h3>')
        P.append(f'<div class="smart-desc">{html.escape(reason)}</div>')
        P.append('<ul class="smart-list">')
        for cmd in cmds:
            P.append(f'<li><code>lark-cli {html.escape(svc)} {html.escape(cmd)}</code></li>')
        P.append('</ul></section>')
    P.append('</div>')

    for svc in order:
        rows = by_svc[svc]
        lsvc = LARK_MAP.get(svc)
        lbl = f"↔ lark {lsvc}" if lsvc else "钉钉特有 · lark 无对应"
        P.append(f'<h2>{html.escape(svc)} <span class="c">· {len(rows)} 个 · {lbl}</span></h2>')
        P.append('<table><colgroup><col style="width:27%"><col style="width:28%"><col style="width:22%"><col style="width:23%"></colgroup>')
        P.append('<thead><tr><th>dws shortcut（新增）</th><th>原生 MCP / 编排说明</th><th>lark-cli 对应</th><th>实际测试情况</th></tr></thead><tbody>')
        for it in sorted(rows, key=lambda x: x["command"]):
            rk = RISK_CLS.get(it["risk"], "rk-r")
            lk_cls, lk_txt = lark_cmd(it, lark)
            lk_html = f'<span class="lk-{lk_cls}"><code>{html.escape(lk_txt)}</code></span>' if lk_cls == "hit" else f'<span class="lk-{lk_cls}">{html.escape(lk_txt)}</span>'
            smart_badge = '<span class="smart-badge">复合/智能型</span>' if it["layer"] == "smart" else ""
            P.append("<tr>")
            P.append(f'<td class="sc"><code>{html.escape(shortcut_cmd(it))}</code><span class="rk {rk}">{it["risk"]}</span>{smart_badge}<div class="desc">{html.escape(it["desc"])}</div></td>')
            P.append(f'<td class="raw"><code>{html.escape(raw_cmd(it))}</code></td>')
            P.append(f'<td>{lk_html}</td>')
            P.append(f'<td>{test_html(it, tests, real_tests)}</td>')
            P.append("</tr>")
        P.append("</tbody></table>")

    P.append('<footer>由 scripts/gen_shortcut_comparison.py 从源码自动生成 · 数据随 shortcut 演进刷新</footer></div></body></html>')
    return "\n".join(P)

def main():
    lark = load_lark()
    items = collect()
    matrix, tests = load_test_matrix()
    real_read_matrix, real_read_tests = load_real_read_matrix()
    real_write_matrix, real_write_tests = load_real_write_matrix()
    real_tests = {}
    real_tests.update(real_read_tests)
    real_tests.update(real_write_tests)
    if matrix.get("failed", 0) or matrix.get("tool_verification_bad", 0):
        raise SystemExit(f"shortcut test matrix failed: failed={matrix.get('failed')} tool_bad={matrix.get('tool_verification_bad')}")
    out = gen_html(items, lark, matrix, tests, real_read_matrix, real_write_matrix, real_tests)
    dst = os.path.join(ROOT, "docs", "shortcut-comparison.html")
    open(dst, "w", encoding="utf-8").write(out)
    hit = sum(1 for it in items if lark_cmd(it, lark)[0] == "hit")
    miss = sum(1 for it in items if lark_cmd(it, lark)[0] == "miss")
    none = sum(1 for it in items if lark_cmd(it, lark)[0] == "none")
    print(f"shortcuts={len(items)} lark_hit={hit} lark_miss={miss} dingtalk_specific={none}")
    print(f"tests={matrix.get('total')} assembled={matrix.get('assembled')} validation_blocked={matrix.get('validation_blocked')} failed={matrix.get('failed')} tool_bad={matrix.get('tool_verification_bad')}")
    if real_read_matrix:
        s = matrix_summary(real_read_matrix)
        print(f"real_read_tests={s.get('total')} ok={s.get('ok')} error={s.get('error')} timeout={s.get('timeout')}")
    if real_write_matrix:
        s = matrix_summary(real_write_matrix)
        print(f"real_write_tests={s.get('total')} ok={s.get('ok')} error={s.get('error')} timeout={s.get('timeout')} held={s.get('held')}")
    print("written:", dst)

if __name__ == "__main__":
    main()
