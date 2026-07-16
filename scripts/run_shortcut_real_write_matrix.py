#!/usr/bin/env python3
"""Run write/high-risk shortcuts against the real CLI with safe negative inputs.

This is intentionally not a mock and not a dry-run.  It launches the built CLI
for every write/high-risk shortcut and records the real stdout/stderr/exit code.

To avoid broad irreversible side effects, target identifiers and people are
rewritten to obviously-nonexistent test values.  Commands that have no
resource/user target and would necessarily mutate the current account state are
held and recorded rather than executed.
"""

from __future__ import annotations

import datetime as dt
import argparse
import json
import os
import shlex
import subprocess
import time
from pathlib import Path

from shortcut_real_result import (
    classify_failure,
    sanitize_result,
    summarize_failure_categories,
    classify_real_status,
    summarize_results,
)


ROOT = Path(__file__).resolve().parents[1]
MATRIX_PATH = Path("/private/tmp/dws-shortcut-matrix.json")
OUT_PATH = ROOT / "docs" / "shortcut-real-write-results.json"
BIN = os.environ.get("DWS_REAL_TEST_BIN", "/private/tmp/dws-real-test")

NO_SAFE_NEGATIVE_TARGET = {
    ("chat", "+conversation-clear-all-red-point"):
        "该命令没有目标 ID，会直接修改当前账号所有会话红点状态；需要逐项授权后单独跑。",
    ("chat", "+category-create"):
        "该命令只需要标题，真实执行会在当前账号创建会话分类；需要逐项授权后单独跑。",
}


def ensure_matrix() -> dict:
    if not MATRIX_PATH.exists():
        env = os.environ.copy()
        env.setdefault("GOCACHE", "/private/tmp/dws_gocache")
        raw = subprocess.check_output(
            ["go", "run", "./scripts/gen_shortcut_test_matrix.go"],
            cwd=ROOT,
            env=env,
            text=True,
        )
        MATRIX_PATH.write_text(raw, encoding="utf-8")
    return json.loads(MATRIX_PATH.read_text(encoding="utf-8"))


def replace_flag_values(args: list[str], now: str, service: str = "", command: str = "") -> list[str]:
    future_start = (
        dt.datetime.now() + dt.timedelta(days=7)
    ).replace(hour=10, minute=0, second=0, microsecond=0).isoformat() + "+08:00"
    future_end = (
        dt.datetime.now() + dt.timedelta(days=7)
    ).replace(hour=10, minute=15, second=0, microsecond=0).isoformat() + "+08:00"

    no_user = "__DWS_SHORTCUT_REAL_TEST_NO_SUCH_USER__"
    no_id = "DWSREALTESTNOSUCHID0000000000000"
    no_conv = "cidDWSREALTESTNOSUCHCONV"
    huge_num = "999999999999"
    text = f"DWS shortcut 真实测试 {now}，安全负向输入，请忽略。"

    replacements = {
        # People / users.
        "to": no_user,
        "with": no_user,
        "owner": no_user,
        "target": no_user,
        "user": no_user,
        "users": no_user,
        "user-ids": no_user,
        "new-owner": no_user,
        "applicant": no_user,
        "inviter": no_user,
        "approver-user-id": no_user,
        "add-users": no_user,
        "remove-users": "",
        "add-extra-users": "",
        "remove-extra-users": "",
        "at-user-ids": no_user,
        "at-users": no_user,
        "at-open-dingtalk-ids": "",
        "open-dingtalk-ids": "",
        # Conversation / message / Ding targets.
        "group": no_conv,
        "id": no_id,
        "conversation-id": no_conv,
        "open-conversation-id": no_conv,
        "src-conversation-id": no_conv,
        "dest-conversation-id": no_conv,
        "message-id": no_id,
        "msg-id": no_id,
        "msg-ids": no_id,
        "src-msg-id": no_id,
        "src-thread-id": no_id,
        "keys": no_id,
        "biz-id": no_id,
        "category-ids": huge_num,
        "receiver": no_user,
        "robot-code": "__DWS_SHORTCUT_REAL_TEST_NO_SUCH_ROBOT__",
        "bot-id": no_id,
        "token": "__DWS_SHORTCUT_REAL_TEST_NO_SUCH_WEBHOOK_TOKEN__",
        # DingTalk document / app / aitable / wiki ids.
        "base-id": no_id,
        "base": no_id,
        "table-id": no_id,
        "table": no_id,
        "field-id": no_id,
        "record-id": huge_num,
        "record-ids": no_id,
        "view-id": no_id,
        "dashboard-id": no_id,
        "chart-id": no_id,
        "workflow-id": no_id,
        "role-id": no_id,
        "role-ids": no_id,
        "section-id": no_id,
        "parent-section-id": no_id,
        "new-parent-section-id": no_id,
        "node-id": no_id,
        "node": no_id,
        "doc": no_id,
        "folder": no_id,
        "workspace": no_id,
        "template-id": no_id,
        "comment-key": no_id,
        "block-id": no_id,
        "unified-app-id": no_id,
        "icon-media-id": no_id,
        "media-id": no_id,
        "session-id": no_id,
        "import-id": no_id,
        "space": "__DWS_SHORTCUT_REAL_TEST_NO_SUCH_SPACE__",
        # Numeric identifiers.
        "category-id": huge_num,
        "record-id-num": huge_num,
        "record-id-int": huge_num,
        "class-id": huge_num,
        "group-id": huge_num,
        "plan-id": huge_num,
        "result-id": huge_num,
        # Text-like values.
        "title": f"DWS shortcut 真实测试 {now}",
        "name": f"DWS shortcut 真实测试 {now}",
        "new-name": f"DWS shortcut 真实测试 {now}",
        "alias-title": f"DWS shortcut 真实测试 {now}",
        "nick": f"DWS测试{now}",
        "desc": "DWS shortcut 真实测试描述，可删除",
        "description": "DWS shortcut 真实测试描述，可删除",
        "field-description": "DWS shortcut 真实测试字段说明",
        "text": text,
        "content": text,
        "comment": text,
        "note": text,
        "remark": text,
        "selected-text": "DWS测试",
        "task": f"DWS shortcut 真实测试待办 {now}",
        "keyword": f"__DWS_SHORTCUT_REAL_TEST_NO_SUCH_APPROVAL_{now}__",
        "reason": "DWS shortcut 真实测试安全负向清理",
        "uuid": f"dws-shortcut-real-negative-{now}",
        # Times.
        "due": future_end,
        "at": future_start,
        "start": future_start,
        "end": future_end,
        "time": future_start,
        # Structured values.
        "config": "{}",
        "layout": "{}",
        "json": "{}",
        "records": "[]",
        "ai-config": "{}",
        "field-mapping": "{}",
        "class-vo": '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"09:00","across":0},{"checkType":"OffDuty","checkTime":"18:00","across":0}]}]}',
        "group-vo": '{"defaultClassId":999999999999,"workDayClassList":[999999999999,999999999999,999999999999,999999999999,999999999999,0,0]}',
        "visibility-rules": '[{"type":"dept","visible":["-1"]}]',
        "sub-roles": "[]",
        "skills": "[]",
        "scope-values": "[]",
        "event-codes": "[]",
        "schedules": '[{"userId":"202397","workDate":"2026-07-16 09:00:00","classId":999999999999,"isRest":"N"}]',
        "class-ids": "[999999999999]",
        "add-depts": "",
        "remove-depts": "",
        # URLs / app config.
        "homepage-url": "https://example.invalid/dws-shortcut-real-test",
        "pc-homepage-url": "https://example.invalid/dws-shortcut-real-test",
        "omp-url": "https://example.invalid/dws-shortcut-real-test",
        "outgoing-url": "https://example.invalid/dws-shortcut-real-test",
        "event-callback-url": "https://example.invalid/dws-shortcut-real-test",
        "redirect-urls": "https://example.invalid/dws-shortcut-real-test",
        "sso-urls": "https://example.invalid/dws-shortcut-real-test",
        "ip-whitelist": "127.0.0.1",
        "url": "https://example.invalid/dws-shortcut-real-test",
        # Misc.
        "type": "FIXED",
        "record-name-key": "task",
        "file-name": f"dws-shortcut-real-{now}.txt",
        "mime-type": "text/plain",
        "version": "0.0.1-dws-real-test",
        "version-id": no_id,
        "member-type": "USER",
        "emotion-id": no_id,
        "emotion-name": "DWS测试表情",
        "background-id": no_id,
        "emoji": "👍",
        "pair": "测试=>测试",
        "leave-code": no_id,
        "num": "1",
    }
    if service == "chat" and command == "+messages-send-card":
        replacements["receiver"] = "103262"
    if service == "chat" and command == "+chat-audit-join":
        replacements["applicant"] = "103262"
        replacements["inviter"] = "519019"
    if service == "doc" and command == "+comment-create-inline":
        replacements["start"] = "0"
        replacements["end"] = "1"
    if service == "attendance" and command == "+save-leave-balance":
        replacements["start"] = (dt.date.today() + dt.timedelta(days=1)).isoformat()
        replacements["end"] = (dt.date.today() + dt.timedelta(days=30)).isoformat()
    if service == "aitable" and command == "+record-update":
        replacements["records"] = '[{"recordId":"recDWSREALTEST","cells":{}}]'
    if service == "aitable" and command == "+view-update":
        replacements["desc"] = "{}"

    out = list(args)
    if "--format" not in out:
        out.extend(["--format", "json"])
    i = 0
    while i < len(out) - 1:
        if out[i].startswith("--"):
            key = out[i][2:]
            if key in replacements and not out[i + 1].startswith("--"):
                out[i + 1] = replacements[key]
                i += 2
                continue
        i += 1
    return out


def shell_join(cmd: list[str]) -> str:
    return " ".join(shlex.quote(x) for x in cmd)


def drop_flag(args: list[str], flag: str) -> list[str]:
    out: list[str] = []
    i = 0
    while i < len(args):
        if args[i] == flag:
            i += 2 if i + 1 < len(args) and not args[i + 1].startswith("--") else 1
            continue
        out.append(args[i])
        i += 1
    return out


def adjust_command_args(args: list[str], service: str, command: str) -> list[str]:
    if service == "chat" and command == "+messages-send-card":
        # --group and --receiver are mutually exclusive; exercise the direct-message path.
        return drop_flag(args, "--group")
    return args


def setup_prerequisite(service: str, command: str, now: str) -> dict | None:
    if service == "todo" and command == "+todo-done":
        title = f"DWS shortcut 真实测试待办 {now}"
        due = (
            dt.datetime.now() + dt.timedelta(days=7)
        ).replace(hour=10, minute=15, second=0, microsecond=0).isoformat() + "+08:00"
        cmd = [
            BIN,
            "todo",
            "+remind",
            "--task",
            title,
            "--at",
            due,
            "--yes",
            "--format",
            "json",
        ]
        started = time.time()
        p = subprocess.run(cmd, text=True, capture_output=True, timeout=25)
        return {
            "purpose": "为 todo +todo-done 创建当前账号自己的临时待办 fixture",
            "input": shell_join(cmd),
            "stdout": p.stdout.strip(),
            "stderr": p.stderr.strip(),
            "exit_code": p.returncode,
            "duration_ms": int((time.time() - started) * 1000),
        }
    return None


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--service", action="append", help="Only run shortcuts from this service; may repeat")
    parser.add_argument("--command", action="append", help="Only run this shortcut command; may repeat")
    parser.add_argument("--missing-only", action="store_true", help="Skip shortcuts already present in the output report")
    parser.add_argument("--failed-only", action="store_true", help="Only rerun shortcuts currently marked non-success in the output report")
    ns = parser.parse_args()
    services = set(ns.service or [])
    commands = set(ns.command or [])

    matrix = ensure_matrix()
    rows = [r for r in matrix["results"] if r.get("risk") != "read"]
    if services:
        rows = [r for r in rows if r["service"] in services]
    if commands:
        rows = [r for r in rows if r["command"] in commands]
    if ns.failed_only and OUT_PATH.exists():
        existing = json.loads(OUT_PATH.read_text(encoding="utf-8"))
        failed = {
            (r.get("service"), r.get("command"))
            for r in existing.get("results", [])
            if r.get("status") != "real-ok"
        }
        rows = [r for r in rows if (r["service"], r["command"]) in failed]
    if ns.missing_only and OUT_PATH.exists():
        existing = json.loads(OUT_PATH.read_text(encoding="utf-8"))
        done = {(r.get("service"), r.get("command")) for r in existing.get("results", [])}
        rows = [r for r in rows if (r["service"], r["command"]) not in done]
    now = dt.datetime.now().strftime("%Y%m%d-%H%M%S")
    results = []
    batch_summary = {"total": len(rows), "ok": 0, "error": 0, "timeout": 0, "held": 0}

    for idx, r in enumerate(rows, 1):
        key = (r["service"], r["command"])
        if key in NO_SAFE_NEGATIVE_TARGET:
            batch_summary["held"] += 1
            item = {
                "service": r["service"],
                "command": r["command"],
                "risk": r["risk"],
                "method": "held; no safe negative target",
                "status": "held",
                "input": "",
                "args": [],
                "stdout": "",
                "stderr": NO_SAFE_NEGATIVE_TARGET[key],
                "exit_code": None,
                "duration_ms": 0,
            }
            item = sanitize_result(item)
            category, fixability, note = classify_failure(item)
            item["failure_category"] = category
            item["fixability"] = fixability
            item["diagnosis"] = note
            results.append(item)
            continue

        setup = None
        try:
            setup = setup_prerequisite(r["service"], r["command"], now)
        except subprocess.TimeoutExpired as e:
            setup = {
                "purpose": "创建前置真实资源超时",
                "input": "",
                "stdout": e.stdout.strip() if isinstance(e.stdout, str) else "",
                "stderr": e.stderr.strip() if isinstance(e.stderr, str) else "",
                "exit_code": None,
                "duration_ms": 25000,
            }

        args = adjust_command_args(replace_flag_values(r["args"], now, r["service"], r["command"]), r["service"], r["command"])
        cmd = [BIN] + args
        started = time.time()
        status = "real-error"
        stdout = ""
        stderr = ""
        exit_code = None
        try:
            p = subprocess.run(cmd, text=True, capture_output=True, timeout=25)
            stdout = p.stdout.strip()
            stderr = p.stderr.strip()
            exit_code = p.returncode
            status = classify_real_status(exit_code, stdout)
            if status == "real-ok":
                status = "real-ok"
                batch_summary["ok"] += 1
            else:
                batch_summary["error"] += 1
        except subprocess.TimeoutExpired as e:
            status = "timeout"
            batch_summary["timeout"] += 1
            if isinstance(e.stdout, str):
                stdout = e.stdout.strip()
            if isinstance(e.stderr, str):
                stderr = e.stderr.strip()
        duration_ms = int((time.time() - started) * 1000)
        item = {
            "service": r["service"],
            "command": r["command"],
            "risk": r["risk"],
            "method": "real-backend-safe-negative-write; no --mock; no --dry-run",
            "status": status,
            "input": shell_join(cmd),
            "args": args,
            "stdout": stdout,
            "stderr": stderr,
            "exit_code": exit_code,
            "duration_ms": duration_ms,
        }
        if setup is not None:
            item["setup"] = setup
        item = sanitize_result(item)
        category, fixability, note = classify_failure(item)
        if category != "passed":
            item["failure_category"] = category
            item["fixability"] = fixability
            item["diagnosis"] = note
        results.append(item)
        if idx % 10 == 0 or idx == len(rows):
            print(
                f"progress {idx}/{len(rows)} ok={batch_summary['ok']} "
                f"error={batch_summary['error']} timeout={batch_summary['timeout']} held={batch_summary['held']}",
                flush=True,
            )
    if OUT_PATH.exists():
        existing = json.loads(OUT_PATH.read_text(encoding="utf-8"))
    else:
        existing = {"results": []}
    replace_keys = {(r["service"], r["command"]) for r in results}
    merged = [
        r for r in existing.get("results", [])
        if (r.get("service"), r.get("command")) not in replace_keys
    ]
    merged.extend(results)
    summary = summarize_results(merged, include_held=True)
    failure_categories = summarize_failure_categories(merged)

    OUT_PATH.write_text(json.dumps({
        "generated_at": dt.datetime.now().isoformat(),
        "summary": summary,
        "failure_categories": failure_categories,
        "results": merged,
    }, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"saved {OUT_PATH} batch={batch_summary} merged={summary}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
