#!/usr/bin/env python3
"""Run every read shortcut against the real DWS backend.

This launches the built CLI once per read shortcut.  It does not use --mock and
does not use --dry-run.  Inputs are synthetic but realistic where a shortcut
expects a name, date, or query; resource identifiers remain test placeholders
when no resource has been created for that command.
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
OUT_PATH = ROOT / "docs" / "shortcut-real-read-results.json"
BIN = os.environ.get("DWS_REAL_TEST_BIN", "/private/tmp/dws-real-test")
DEVAPP_FIXTURE_ID = "678f27ec-4339-49d8-9c49-371b284bf552"
DEVAPP_FIXTURE_VERSION_ID = "4743accb-45e2-4bc9-8c96-74fc62ace2e8"
CHAT_FIXTURE_OPEN_CONVERSATION_ID = "cid3Jijzhe2aqs9ysOXjhi05g=="
CHAT_FIXTURE_GROUP_NAME = "浅曦-kida,Dennis,秋画"
CHAT_FIXTURE_DM_OPEN_CONVERSATION_ID = "cidie1367hAfBxqipzE59k5sknHLrHmvYkw98NADhfnjPI="
CHAT_FIXTURE_OPEN_MESSAGE_ID = "msgEuOor1PmFBNlx9M06N9z1Q=="
CHAT_FIXTURE_OPEN_TASK_ID = "y/wM6Lo+9GbIqtILYPv1BZDcMW+2FgnqskgcpdOiMdM="
CALENDAR_FIXTURE_EVENT_ID = "THN4YUtOTlplYU9sZzd2czE4YURLQT09_1784078100000"
DOC_FIXTURE_NODE_ID = "P0MALyR8knpgo9GycY7ZlMxlJ3bzYmDO"
DOC_FIXTURE_FOLDER_ID = "Amq4vjg89ZOAdqyaSMKpApXdW3kdP0wQ"
DOC_FIXTURE_EXPORT_JOB_ID = "29346731713"
SHEET_FIXTURE_NODE_ID = "mweZ92PV6O36dZbnsMZx70ylJxEKBD6p"
DING_FIXTURE_OPEN_DING_ID = "5D73E3AC29C780072D1CD56C6874ACC2"
CONTACT_FIXTURE_MOBILE = "13161187007"
AITABLE_FIXTURE_BASE_ID = "gpG2NdyVXQyZ0OmoSbd1vbA6JMwvDqPk"
AITABLE_FIXTURE_TABLE_ID = "hERWDMS"
AITABLE_FIXTURE_VIEW_ID = "qvGDAH2"
AITABLE_FIXTURE_FORM_VIEW_ID = "lmeV1cb"
AITABLE_FIXTURE_FIELD_ID = "01ZM8y7"
AITABLE_FIXTURE_RECORD_ID = "1015oH3OXy"
AITABLE_FIXTURE_DASHBOARD_ID = "KY9tlWg5NEHgfs8WT6IO2"
AITABLE_FIXTURE_CHART_ID = "widget-dlxFo0tNSImp4ITpDn5fQ"

HELD_CASES = {
    ("devapp", "+credentials-get"):
        "该命令会读取真实应用凭证/密钥；不能用真实 app 自动执行。当前仅用占位 ID 验证负向路径，真实成功需人工在安全环境单独确认。",
}


def ensure_matrix() -> dict:
    env = os.environ.copy()
    env.setdefault("GOCACHE", "/private/tmp/dws_gocache")
    raw = subprocess.check_output(
        ["go", "run", "./scripts/gen_shortcut_test_matrix.go"],
        cwd=ROOT,
        env=env,
        text=True,
    )
    MATRIX_PATH.write_text(raw, encoding="utf-8")
    return json.loads(raw)


def replace_flag_values(args: list[str], service: str, command: str) -> list[str]:
    today = dt.date.today()
    start = (dt.datetime.now() + dt.timedelta(days=1)).replace(hour=10, minute=0, second=0, microsecond=0).isoformat() + "+08:00"
    end = (dt.datetime.now() + dt.timedelta(days=1)).replace(hour=11, minute=0, second=0, microsecond=0).isoformat() + "+08:00"
    no_id = "DWSREALREADNOSUCHID0000000000000"
    no_conv = "cidDWSREALREADNOSUCHCONV"
    day = str(today)
    yesterday = str(today - dt.timedelta(days=1))
    datetime_start = dt.datetime.now().replace(hour=10, minute=0, second=0, microsecond=0).strftime("%Y-%m-%d %H:%M:%S")
    datetime_end = (dt.datetime.now() + dt.timedelta(hours=1)).replace(minute=0, second=0, microsecond=0).strftime("%Y-%m-%d %H:%M:%S")
    replacements = {
        "name": "DWS shortcut 真实测试",
        "query": "测试",
        "keyword": "测试",
        "q": "测试",
        "text": "测试",
        "title": "测试",
        "phone": "13000000000",
        "mobile": "13000000000",
        "user": "冬翔",
        "users": "冬翔",
        "to": "冬翔",
        "with": "冬翔",
        "who": "冬翔",
        "dept": "模型算法",
        "dept-id": "842379556",
        "department-id": "842379556",
        "start": start,
        "end": end,
        "from": str(today - dt.timedelta(days=7)),
        "to-date": str(today),
        "date": str(today),
        "time": datetime_start,
        "days": "7",
        "types": "leave",
        "columns": "1001",
        "limit": "10",
        "page": "1",
        "page-size": "10",
        "cursor": "0",
        "calendar-id": "primary",
        "event": no_id,
        "type": "ALL",
        "types": "leave",
        "columns": "1001",
        "role-types": "executor",
        "status": "false",
        "artifacts": "basic",
        "direction": "older",
        "file-types": "alidoc",
        "order-by": "name",
        "order": "asc",
        "space-id": "1",
        "space": "测试",
        "base": no_id,
        "base-id": no_id,
        "table": no_id,
        "table-id": no_id,
        "view-id": no_id,
        "record-id": no_id,
        "record-ids": no_id,
        "field-id": no_id,
        "dashboard-id": no_id,
        "chart-id": no_id,
        "workflow-id": no_id,
        "node": no_id,
        "doc": no_id,
        "folder": no_id,
        "workspace": no_id,
        "group": no_conv,
        "conversation-id": no_conv,
        "open-conversation-id": no_conv,
        "message-id": no_id,
        "msg-id": no_id,
        "id": no_id,
        "session-id": no_id,
        "process-instance-id": no_id,
        "task-id": no_id,
        "template-id": no_id,
        "mail-id": no_id,
        "filters": "{}",
        "sort": "[]",
    }
    if service == "calendar" and command == "+free-slots":
        replacements["from"] = "9"
        replacements["to"] = "18"
    if service == "calendar":
        replacements["event"] = CALENDAR_FIXTURE_EVENT_ID
        replacements["cursor"] = ""
        if command == "+freebusy":
            replacements["users"] = "103262"
    if service == "doc" and command == "+comment-list":
        replacements["type"] = "global"
    if service == "doc":
        replacements["node"] = DOC_FIXTURE_NODE_ID
        replacements["doc"] = DOC_FIXTURE_NODE_ID
        replacements["folder"] = DOC_FIXTURE_FOLDER_ID
        replacements["job-id"] = DOC_FIXTURE_EXPORT_JOB_ID
    if service == "drive":
        replacements["node"] = DOC_FIXTURE_NODE_ID
        replacements["folder"] = DOC_FIXTURE_FOLDER_ID
    if service == "todo":
        replacements["task-id"] = "55119034912"
        replacements["size"] = "10"
    if service == "aitable":
        replacements["name"] = "Real共创版设备去向登记"
        replacements["base"] = AITABLE_FIXTURE_BASE_ID
        replacements["base-id"] = AITABLE_FIXTURE_BASE_ID
        replacements["table"] = AITABLE_FIXTURE_TABLE_ID
        replacements["table-id"] = AITABLE_FIXTURE_TABLE_ID
        replacements["view-id"] = AITABLE_FIXTURE_VIEW_ID
        replacements["view-ids"] = AITABLE_FIXTURE_VIEW_ID
        replacements["field-id"] = AITABLE_FIXTURE_FIELD_ID
        replacements["field-ids"] = AITABLE_FIXTURE_FIELD_ID
        replacements["record-id"] = AITABLE_FIXTURE_RECORD_ID
        replacements["record-ids"] = AITABLE_FIXTURE_RECORD_ID
        replacements["dashboard-id"] = AITABLE_FIXTURE_DASHBOARD_ID
        replacements["chart-id"] = AITABLE_FIXTURE_CHART_ID
        if command.startswith("+form-"):
            replacements["view-id"] = AITABLE_FIXTURE_FORM_VIEW_ID
        if command == "+resolve-table":
            replacements["name"] = "Mac Mini"
    if service == "ding" and command == "+list":
        replacements["type"] = "ALL"
    if service == "ding" and command == "+receiver-status":
        replacements["ding-id"] = DING_FIXTURE_OPEN_DING_ID
    if service == "contact" and command == "+list-sub-depts":
        replacements["dept"] = "842379556"
    if service == "contact":
        replacements["name"] = "冬翔"
        if command == "+by-mobile":
            replacements["mobile"] = CONTACT_FIXTURE_MOBILE
        if command == "+resolve-dept":
            replacements["name"] = "模型算法"
    if service == "oa":
        now_ms = int(dt.datetime.now().timestamp() * 1000)
        replacements["start"] = str(now_ms - 7 * 24 * 60 * 60 * 1000)
        replacements["end"] = str(now_ms)
        replacements["page"] = "1"
        replacements["limit"] = "10"
    if service == "report":
        replacements["start"] = (dt.datetime.now() - dt.timedelta(days=7)).replace(microsecond=0).isoformat() + "+08:00"
        replacements["end"] = dt.datetime.now().replace(microsecond=0).isoformat() + "+08:00"
        replacements["modified-start"] = replacements["start"]
        replacements["modified-end"] = replacements["end"]
    if service == "attendance":
        replacements.update({
            "user": "202397",
            "users": "202397",
            "staff-ids": "202397",
            "operator-staff-id": "202397",
            "leave-code": "731ed089-62ff-4734-a6c7-3c8fcc8294fc",
            "leave-names": "年假",
            "start": yesterday,
            "end": day,
            "from": yesterday,
            "to-date": day,
            "date": day,
        })
        if command in {"+get-checkin-record", "+query-report-data", "+query-report-leave"}:
            replacements["start"] = datetime_start
            replacements["end"] = datetime_end
        if command == "+get-approve-template":
            replacements["type"] = "leave"
        if command == "+search-group":
            replacements["type"] = "FIXED"
    if service == "devapp":
        replacements["unified-app-id"] = DEVAPP_FIXTURE_ID
        replacements["version-id"] = DEVAPP_FIXTURE_VERSION_ID
        replacements["cursor"] = ""
    if service == "mail":
        replacements["email"] = "xinyang.dxy@alibaba-inc.com"
        replacements["folder"] = "2"
        replacements["query"] = "subject:测试"
        replacements["keyword"] = "董鑫阳"
        replacements["employee-no"] = "202397"
        replacements["size"] = "10"
        replacements["cursor"] = ""
        if command == "+find-mail-user":
            replacements["query"] = "董鑫阳"
    if service == "chat":
        replacements["group"] = CHAT_FIXTURE_OPEN_CONVERSATION_ID
        replacements["conversation-id"] = CHAT_FIXTURE_OPEN_CONVERSATION_ID
        replacements["open-conversation-id"] = CHAT_FIXTURE_OPEN_CONVERSATION_ID
        replacements["msg-ids"] = CHAT_FIXTURE_OPEN_MESSAGE_ID
        replacements["message-id"] = CHAT_FIXTURE_OPEN_MESSAGE_ID
        replacements["msg-id"] = CHAT_FIXTURE_OPEN_MESSAGE_ID
        replacements["open-task-id"] = CHAT_FIXTURE_OPEN_TASK_ID
        if command == "+group-members":
            replacements["group"] = CHAT_FIXTURE_GROUP_NAME
        if command == "+messages-read-status":
            replacements["conversation-id"] = CHAT_FIXTURE_DM_OPEN_CONVERSATION_ID
            replacements["users"] = "冬翔"
        if command == "+messages-resource-url":
            replacements["type"] = "mediaId"
        if command == "+bot-find":
            replacements["cursor"] = ""
    if service == "sheet" and command == "+list-sheets":
        replacements["node"] = SHEET_FIXTURE_NODE_ID
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
    if service == "chat" and command == "+chat-messages":
        return drop_flag(args, "--user")
    if service == "chat" and command == "+messages-list-direct":
        out = drop_flag(args, "--open-dingtalk-id")
        for i in range(len(out) - 1):
            if out[i] == "--user":
                out[i + 1] = "103262"
        return out
    if service == "calendar" and command == "+freebusy":
        return drop_flag(args, "--rooms")
    if service == "devapp" and command == "+permission-list":
        out = drop_flag(args, "--scope-value")
        out = drop_flag(out, "--scope-type")
        out = drop_flag(out, "--api-status")
        for i in range(len(out) - 1):
            if out[i] == "--auth-status":
                out[i + 1] = "ALL"
        return out
    if service == "doc" and command == "+search":
        out = drop_flag(args, "--extensions")
        out = drop_flag(out, "--created-from")
        out = drop_flag(out, "--created-to")
        out = drop_flag(out, "--visited-from")
        out = drop_flag(out, "--visited-to")
        out = drop_flag(out, "--creator-uids")
        out = drop_flag(out, "--editor-uids")
        out = drop_flag(out, "--mentioned-uids")
        out = drop_flag(out, "--workspace-ids")
        return out
    if service == "doc" and command == "+comment-list":
        out = drop_flag(args, "--cursor")
        out = drop_flag(out, "--resolve-status")
        return out
    if service == "doc" and command == "+list":
        out = drop_flag(args, "--workspace")
        out = drop_flag(out, "--cursor")
        return out
    if service == "report" and command == "+outbox-list":
        return drop_flag(args, "--template-name")
    return args


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--service", action="append", help="Only run shortcuts from this service; may repeat")
    parser.add_argument("--command", action="append", help="Only run this shortcut command; may repeat")
    parser.add_argument("--failed-only", action="store_true", help="Only rerun shortcuts currently marked non-success in the output report")
    ns = parser.parse_args()
    services = set(ns.service or [])
    commands = set(ns.command or [])

    matrix = ensure_matrix()
    rows = [r for r in matrix["results"] if r.get("risk") == "read"]
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
    results = []
    summary = {"total": len(rows), "ok": 0, "error": 0, "timeout": 0, "held": 0}
    for idx, r in enumerate(rows, 1):
        key = (r["service"], r["command"])
        if key in HELD_CASES:
            summary["held"] += 1
            item = {
                "service": r["service"],
                "command": r["command"],
                "risk": r["risk"],
                "method": "held; sensitive credential read",
                "status": "held",
                "input": "",
                "args": [],
                "stdout": "",
                "stderr": HELD_CASES[key],
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
        args = adjust_command_args(replace_flag_values(r["args"], r["service"], r["command"]), r["service"], r["command"])
        cmd = [BIN] + args
        started = time.time()
        status = "real-error"
        stdout = ""
        stderr = ""
        exit_code = None
        try:
            p = subprocess.run(cmd, text=True, capture_output=True, timeout=30)
            stdout = p.stdout.strip()
            stderr = p.stderr.strip()
            exit_code = p.returncode
            status = classify_real_status(exit_code, stdout)
            if status == "real-ok":
                status = "real-ok"
                summary["ok"] += 1
            else:
                summary["error"] += 1
        except subprocess.TimeoutExpired as e:
            status = "timeout"
            summary["timeout"] += 1
            if isinstance(e.stdout, str):
                stdout = e.stdout.strip()
            if isinstance(e.stderr, str):
                stderr = e.stderr.strip()
        duration_ms = int((time.time() - started) * 1000)
        item = {
            "service": r["service"],
            "command": r["command"],
            "risk": r["risk"],
            "method": "real-backend-read; no --mock; no --dry-run",
            "status": status,
            "input": shell_join(cmd),
            "args": args,
            "stdout": stdout,
            "stderr": stderr,
            "exit_code": exit_code,
            "duration_ms": duration_ms,
        }
        item = sanitize_result(item)
        category, fixability, note = classify_failure(item)
        if category != "passed":
            item["failure_category"] = category
            item["fixability"] = fixability
            item["diagnosis"] = note
        results.append(item)
        if idx % 25 == 0 or idx == len(rows):
            print(
                f"progress {idx}/{len(rows)} ok={summary['ok']} "
                f"error={summary['error']} timeout={summary['timeout']} held={summary['held']}",
                flush=True,
            )
            if not services and not commands and not ns.failed_only:
                OUT_PATH.write_text(json.dumps({
                    "generated_at": dt.datetime.now().isoformat(),
                    "summary": summary,
                    "failure_categories": summarize_failure_categories(results),
                    "results": results,
                }, ensure_ascii=False, indent=2), encoding="utf-8")
    if OUT_PATH.exists() and (services or commands or ns.failed_only):
        existing = json.loads(OUT_PATH.read_text(encoding="utf-8"))
        replace_keys = {(r["service"], r["command"]) for r in results}
        merged = [
            r for r in existing.get("results", [])
            if (r.get("service"), r.get("command")) not in replace_keys
        ]
        merged.extend(results)
    else:
        merged = results
    summary = summarize_results(merged, include_held=True)
    OUT_PATH.write_text(json.dumps({
        "generated_at": dt.datetime.now().isoformat(),
        "summary": summary,
        "failure_categories": summarize_failure_categories(merged),
        "results": merged,
    }, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"saved {OUT_PATH} batch={summarize_results(results, include_held=True)} merged={summary}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
