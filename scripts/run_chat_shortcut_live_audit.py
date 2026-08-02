#!/usr/bin/env python3
"""Run a PII-safe live audit of every read-only Chat Shortcut.

The audit exercises the real Shortcut entry point and, when DWS_DUMP_RAW is
enabled by the locally built binary, captures the lower MCP text response from
the same invocation. Raw business responses are kept in memory only. The
written JSON/Markdown reports contain shapes, counts, tool names, and redacted
error facts, but no message text, user names, IDs, or invite/download URLs.

This is deliberately a live-account audit, not a mock/Help contract check.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import signal
import subprocess
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

SCRIPT_DIR = Path(__file__).resolve().parent
sys.path.insert(0, str(SCRIPT_DIR))

from shortcut_real_result import count_projection_items, payload_indicates_error  # noqa: E402


READ_SHORTCUTS = (
    "+at-me",
    "+bot-find",
    "+bot-search",
    "+category-list",
    "+category-list-conversations",
    "+chat-bots",
    "+chat-get-by-id",
    "+chat-invite-url",
    "+chat-list-all",
    "+chat-list-join-requests",
    "+chat-list-mine",
    "+chat-members-list",
    "+chat-members-get",
    "+chat-messages",
    "+chat-role-list",
    "+chat-role-query-user",
    "+chat-search",
    "+conversation-info",
    "+conversation-list",
    "+conversation-list-top",
    "+group-members",
    "+messages-list",
    "+messages-list-direct",
    "+messages-list-pin",
    "+messages-list-unread-conversations",
    "+messages-mget",
    "+messages-query-send-status",
    "+messages-read-status",
    "+messages-resource-download",
    "+messages-resource-url",
    "+my-groups",
    "+search-msg",
    "+thread-replies",
    "+unread-chats",
)

LIST_PATHS_BY_TOOL = {
    "search_at_me_message": (
        ("result", "conversationMessagesList", "*", "messages"),
    ),
    "search_bots": (("result", "bots"), ("result", "list")),
    "search_my_robots": (("result", "robots"), ("result", "list")),
    "list_user_define_conv_categories": (
        ("result", "categories"),
        ("result", "categoryList"),
    ),
    "list_conversations_by_category": (
        ("result", "conversations"),
        ("result", "conversationList"),
    ),
    "list_group_bots": (("result", "bots"), ("result", "robots")),
    "list_my_groups_pagination": (("result", "groups"),),
    "list_owned_or_admin_groups": (("result", "groups"),),
    "list_group_member_by_ids": (
        ("result", "members"),
        ("result", "list"),
    ),
    "list_custom_group_roles": (("result", "roles"), ("result", "list")),
    "list_all_conversations": (
        ("result", "conversations"),
        ("result", "conversationList"),
    ),
    "list_top_conversations": (
        ("result", "conversations"),
        ("result", "conversationList"),
    ),
    "get_group_members": (("result", "list"), ("result", "members")),
    "list_conversation_message_v2": (
        ("result", "messages"),
        ("result", "messageList"),
    ),
    "list_individual_chat_message": (
        ("result", "messages"),
        ("result", "messageList"),
    ),
    "list_pin_messages": (
        ("result", "messages"),
        ("result", "pinMessages"),
        ("result", "list"),
    ),
    "unread_message_conversation_list": (
        ("result", "conversations"),
        ("result", "conversationList"),
    ),
    "list_messages_by_ids": (("result", "messages"), ("result", "list")),
    "search_messages_by_keyword": (
        ("result", "messages"),
        ("result", "messageList"),
        ("result", "list"),
    ),
    "list_topic_replies": (
        ("result", "messages"),
        ("result", "replies"),
        ("result", "list"),
    ),
}

@dataclass
class Capture:
    command: str
    argv: list[str]
    exit_code: int | None
    stdout: str
    stderr: str
    raw_calls: list[tuple[str, str, Any]]
    duration_ms: int
    timed_out: bool = False

    def upper(self) -> Any:
        try:
            return json.loads(self.stdout)
        except (json.JSONDecodeError, TypeError):
            return None


def parse_raw_calls(stderr: str) -> tuple[list[tuple[str, str, Any]], str]:
    raw_calls: list[tuple[str, str, Any]] = []
    clean_lines: list[str] = []
    for line in (stderr or "").splitlines():
        if not line.startswith("DWSRAW\t"):
            clean_lines.append(line)
            continue
        fields = line.split("\t", 3)
        if len(fields) != 4:
            continue
        try:
            raw_calls.append((fields[1], fields[2], json.loads(fields[3])))
        except json.JSONDecodeError:
            raw_calls.append((fields[1], fields[2], fields[3]))
    return raw_calls, "\n".join(clean_lines)


def run_capture(binary: Path, command: str, args: list[str], timeout: int) -> Capture:
    argv = [str(binary), "chat", command, *args, "--format", "json"]
    env = os.environ.copy()
    env["DWS_DUMP_RAW"] = "1"
    started = time.monotonic()
    proc = subprocess.Popen(
        argv,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        env=env,
        start_new_session=True,
    )
    try:
        stdout, stderr = proc.communicate(timeout=timeout)
        raw_calls, clean_stderr = parse_raw_calls(stderr)
        return Capture(
            command=command,
            argv=argv,
            exit_code=proc.returncode,
            stdout=stdout,
            stderr=clean_stderr,
            raw_calls=raw_calls,
            duration_ms=round((time.monotonic() - started) * 1000),
        )
    except subprocess.TimeoutExpired as exc:
        # dws may have transport children which inherit its output pipes.
        # Killing only the immediate process leaves those descendants alive and
        # communicate() can block forever despite the advertised timeout.
        os.killpg(proc.pid, signal.SIGKILL)
        stdout, stderr = proc.communicate()
        raw_calls, clean_stderr = parse_raw_calls(stderr)
        return Capture(
            command=command,
            argv=argv,
            exit_code=None,
            stdout=stdout,
            stderr=clean_stderr,
            raw_calls=raw_calls,
            duration_ms=round((time.monotonic() - started) * 1000),
            timed_out=True,
        )


def at_path(value: Any, path: tuple[str, ...]) -> list[Any] | None:
    current = value
    for index, part in enumerate(path):
        if part == "*":
            if not isinstance(current, list):
                return None
            remainder = path[index + 1 :]
            combined: list[Any] = []
            for item in current:
                found = at_path(item, remainder)
                if isinstance(found, list):
                    combined.extend(found)
            return combined
        if not isinstance(current, dict) or part not in current:
            return None
        current = current[part]
    return current if isinstance(current, list) else None


def lower_count(tool: str, raw: Any) -> int | None:
    for path in LIST_PATHS_BY_TOOL.get(tool, ()):
        found = at_path(raw, path)
        if found is not None:
            return len(found)
    return None


def response_is_error(value: Any) -> bool:
    if not isinstance(value, dict):
        return False
    if value.get("success") is False:
        return True
    if str(value.get("status") or "").lower() == "error":
        return True
    for key in ("error", "errorCode", "error_code", "code"):
        item = value.get(key)
        if item not in (None, False, "", 0, "0", "ok", "success", "succeed"):
            return True
    return False


def error_facts(stderr: str) -> dict[str, Any]:
    facts: dict[str, Any] = {}
    try:
        value = json.loads(stderr)
        error = value.get("error", value) if isinstance(value, dict) else {}
        if isinstance(error, dict):
            for key in ("category", "code", "reason", "server_error_code", "server_key"):
                if key in error:
                    facts[key] = error[key]
            message = str(error.get("message") or "")
        else:
            message = stderr
    except json.JSONDecodeError:
        message = stderr
    lowered = message.lower()
    for marker, label in (
        ("permission", "permission"),
        ("not found", "not_found"),
        ("不存在", "not_found"),
        ("invalid", "invalid_argument"),
        ("必填", "missing_argument"),
        ("超时", "timeout"),
    ):
        if marker in lowered:
            facts["message_class"] = label
            break
    return facts


def shape(value: Any) -> dict[str, Any]:
    if isinstance(value, dict):
        return {"type": "object", "keys": sorted(value)}
    if isinstance(value, list):
        return {"type": "array", "length": len(value)}
    if value is None:
        return {"type": "none"}
    return {"type": type(value).__name__}


def reaction_message_count(value: Any, *, upper: bool) -> int:
    """Count reaction-bearing message objects without retaining business values."""
    count = 0
    if isinstance(value, dict):
        keys = ("reactions", "emotionReplyList", "reactionList") if upper else (
            "emotionReplyList",
            "reactionList",
            "reactions",
        )
        if any(value.get(key) not in (None, [], {}) for key in keys):
            count += 1
        for item in value.values():
            count += reaction_message_count(item, upper=upper)
    elif isinstance(value, list):
        for item in value:
            count += reaction_message_count(item, upper=upper)
    return count


def thread_message_count(value: Any, *, upper: bool) -> int:
    keys = {"threadId"} if upper else {
        "openConvThreadId",
        "openConversationThreadId",
        "threadId",
        "topicId",
    }
    count = 0
    if isinstance(value, dict):
        if any(value.get(key) not in (None, "") for key in keys):
            count += 1
        for item in value.values():
            count += thread_message_count(item, upper=upper)
    elif isinstance(value, list):
        for item in value:
            count += thread_message_count(item, upper=upper)
    return count


def pagination_meta(value: Any) -> dict[str, Any]:
    """Extract only completeness facts; never retain cursor values."""
    if isinstance(value, dict):
        has_more = None
        for key in ("hasMore", "has_more"):
            if key in value and isinstance(value[key], bool):
                has_more = value[key]
                break
        cursor_present = any(
            value.get(key) not in (None, "", 0, "0")
            for key in ("nextCursor", "next_cursor", "nextToken", "next_token", "pageToken", "page_token")
        )
        next_page = value.get("nextPage")
        resume_present = cursor_present or (
            isinstance(next_page, dict)
            and next_page.get("time") not in (None, "")
        )
        if has_more is not None or resume_present:
            return {
                "has_more": has_more,
                "next_cursor_present": cursor_present,
                "resume_present": resume_present,
            }
        for item in value.values():
            found = pagination_meta(item)
            if found:
                return found
    elif isinstance(value, list):
        for item in value:
            found = pagination_meta(item)
            if found:
                return found
    return {}


def summarize_capture(capture: Capture) -> dict[str, Any]:
    upper = capture.upper()
    raw = capture.raw_calls[-1][2] if capture.raw_calls else None
    tool = capture.raw_calls[-1][1] if capture.raw_calls else None
    upper_count = count_projection_items(capture.stdout)
    expected_lower_count = lower_count(tool, raw) if tool else None
    pagination_raw = raw
    if capture.command == "+chat-members-list":
        counts = upper.get("counts", {}) if isinstance(upper, dict) else {}
        upper_count = counts.get("total") if isinstance(counts, dict) else None
        expected_lower_count = 0
        for _, call_tool, call_raw in capture.raw_calls:
            if call_tool in {"get_group_members", "list_group_bots"}:
                item_count = lower_count(call_tool, call_raw)
                if item_count is not None:
                    expected_lower_count += item_count
            if call_tool == "get_group_members":
                pagination_raw = call_raw
    elif capture.command == "+messages-resource-download":
        upper_count = (
            1
            if isinstance(upper, dict)
            and isinstance(upper.get("sizeBytes"), (int, float))
            and upper.get("sizeBytes") >= 0
            and bool(upper.get("localPath"))
            else 0
        )
        expected_lower_count = (
            1
            if first_string(raw, "resourceUrl", "downloadUrl")
            else 0
        )
    exact_passthrough = bool(capture.raw_calls and upper == raw)
    upper_reactions = reaction_message_count(upper, upper=True)
    lower_reactions = reaction_message_count(raw, upper=False)
    upper_threads = thread_message_count(upper, upper=True)
    lower_threads = thread_message_count(raw, upper=False)
    upper_pagination = pagination_meta(upper)
    lower_pagination = pagination_meta(pagination_raw)

    if capture.timed_out:
        status = "timeout"
    elif capture.exit_code != 0:
        status = "backend_error" if capture.stderr else "cli_error"
    elif payload_indicates_error(capture.stdout) or response_is_error(raw):
        status = "error_envelope"
    elif not capture.raw_calls:
        status = "no_lower_capture"
    elif exact_passthrough:
        status = "pass"
    elif expected_lower_count is not None and upper_count != expected_lower_count:
        status = "projection_mismatch"
    elif lower_reactions > 0 and upper_reactions != lower_reactions:
        status = "projection_mismatch"
    elif lower_threads > 0 and upper_threads != lower_threads:
        status = "projection_mismatch"
    elif lower_pagination.get("has_more") is True and (
        upper_pagination.get("has_more") is not True
        or not upper_pagination.get("resume_present")
    ):
        status = "pagination_mismatch"
    elif expected_lower_count == 0 and upper_count == 0:
        status = "pass_empty"
    else:
        status = "pass"

    return {
        "command": capture.command,
        "status": status,
        "exit_code": capture.exit_code,
        "duration_ms": capture.duration_ms,
        "upper": {
            **shape(upper),
            "count": upper_count,
            "reaction_message_count": upper_reactions,
            "thread_message_count": upper_threads,
            "pagination": upper_pagination,
        },
        "lower": {
            "call_count": len(capture.raw_calls),
            "tools": [f"{server}/{name}" for server, name, _ in capture.raw_calls],
            "shape": shape(raw),
            "count": expected_lower_count,
            "reaction_message_count": lower_reactions,
            "thread_message_count": lower_threads,
            "pagination": lower_pagination,
        },
        "exact_passthrough": exact_passthrough,
        "error": error_facts(capture.stderr) if capture.stderr else {},
    }


def blocked(command: str, requirement: str) -> dict[str, Any]:
    return {
        "command": command,
        "status": "fixture_blocked",
        "requirement": requirement,
    }


def dig_values(value: Any, keys: set[str]) -> list[Any]:
    found: list[Any] = []
    if isinstance(value, dict):
        for key, item in value.items():
            if key in keys and item not in (None, "", [], {}):
                found.append(item)
            found.extend(dig_values(item, keys))
    elif isinstance(value, list):
        for item in value:
            found.extend(dig_values(item, keys))
    elif isinstance(value, str):
        stripped = value.strip()
        if stripped.startswith(("{", "[")):
            try:
                embedded = json.loads(stripped)
            except json.JSONDecodeError:
                embedded = None
            if isinstance(embedded, (dict, list)):
                found.extend(dig_values(embedded, keys))
    return found


def first_string(value: Any, *keys: str) -> str:
    for item in dig_values(value, set(keys)):
        if isinstance(item, str) and item.strip():
            return item.strip()
        if isinstance(item, (int, float)) and not isinstance(item, bool):
            return str(int(item))
    return ""


def first_list(value: Any, *keys: str) -> list[Any]:
    for item in dig_values(value, set(keys)):
        if isinstance(item, list):
            return item
    return []


def safe_keyword(messages: list[Any]) -> str:
    for message in messages:
        if not isinstance(message, dict):
            continue
        text = message.get("text")
        if not isinstance(text, str):
            continue
        for token in re.findall(r"[\u4e00-\u9fff]{2,6}|[A-Za-z0-9]{3,12}", text):
            if token:
                return token
    return "工作"


def group_candidates(capture: Capture) -> list[dict[str, Any]]:
    raw = capture.raw_calls[-1][2] if capture.raw_calls else {}
    groups = first_list(raw, "groups")
    valid = [item for item in groups if isinstance(item, dict)]
    priority = {"OWNER": 0, "ADMIN": 1}
    return sorted(valid, key=lambda item: priority.get(str(item.get("myRole")), 2))


def merge_group_candidates(
    groups_all: Capture, groups_mine: Capture
) -> list[dict[str, Any]]:
    all_groups = group_candidates(groups_all)
    mine_groups = group_candidates(groups_mine)
    ordered: list[dict[str, Any]] = []
    seen: set[str] = set()
    for item in mine_groups:
        group_id = str(item.get("openConversationId") or "")
        if not group_id or group_id in seen:
            continue
        # The owned/admin endpoint carries myRole and ownerOpenDingtalkId,
        # which are needed to locate a message sent by the current user. Prefer
        # that richer record; the all-groups page is only a fallback.
        ordered.append(item)
        seen.add(group_id)
    for item in all_groups:
        group_id = str(item.get("openConversationId") or "")
        if group_id and group_id not in seen:
            ordered.append(item)
            seen.add(group_id)
    return ordered


def live_audit(
    binary: Path, timeout: int, max_group_probes: int, max_member_probes: int
) -> list[dict[str, Any]]:
    results: dict[str, dict[str, Any]] = {}
    captures: dict[str, Capture] = {}
    group_override = os.environ.get("DWS_AUDIT_GROUP_ID", "").strip()
    numeric_group_override = os.environ.get("DWS_AUDIT_NUMERIC_GROUP_ID", "").strip()
    open_task_override = os.environ.get("DWS_AUDIT_OPEN_TASK_ID", "").strip()
    topic_group_override = os.environ.get("DWS_AUDIT_TOPIC_GROUP_ID", "").strip()
    topic_id_override = os.environ.get("DWS_AUDIT_TOPIC_ID", "").strip()
    media_group_override = os.environ.get("DWS_AUDIT_MEDIA_GROUP_ID", "").strip()
    media_message_override = os.environ.get("DWS_AUDIT_MEDIA_MESSAGE_ID", "").strip()
    media_resource_override = os.environ.get("DWS_AUDIT_MEDIA_RESOURCE_ID", "").strip()

    def run(command: str, *args: str, retries: int = 0) -> Capture:
        capture = run_capture(binary, command, list(args), timeout)
        for _ in range(retries):
            if summarize_capture(capture)["status"] not in {
                "backend_error",
                "cli_error",
                "error_envelope",
                "timeout",
                "no_lower_capture",
            }:
                break
            capture = run_capture(binary, command, list(args), timeout)
        captures[command] = capture
        results[command] = summarize_capture(capture)
        return capture

    category = run("+category-list")
    groups_all = run("+chat-list-all", "--limit", "200")
    groups_mine = run("+chat-list-mine", "--limit", "200")
    run("+conversation-list", "--limit", "100")
    run("+conversation-list-top", "--limit", "100")
    run("+messages-list-unread-conversations", "--count", "100")
    run("+unread-chats", "--count", "100")
    run("+chat-list-join-requests", "--limit", "50")
    run("+at-me", "--days", "30")
    run("+bot-search", "--page", "1", "--size", "100")
    run("+bot-find", "--query", "机器人", "--limit", "20")
    run("+my-groups")

    category_id = first_string(category.upper(), "categoryId", "category_id")
    if category_id:
        run("+category-list-conversations", "--category-id", category_id)
    else:
        results["+category-list-conversations"] = blocked(
            "+category-list-conversations",
            "账号下需要至少一个真实会话分组 categoryId",
        )

    candidates = merge_group_candidates(groups_all, groups_mine)
    group = candidates[0] if candidates else {}
    messages: Capture | None = None
    best_group: dict[str, Any] = {}
    best_score = -1
    # A valid but quiet first group is not sufficient evidence for the
    # message-dependent Shortcuts. Walk real joined groups until one produces a
    # non-empty message page. Prefer a page that also supplies downstream
    # fixtures (a message sent by the current user, media, or a thread ID).
    # This prevents the audit from stopping at the first technically valid but
    # fixture-poor group.
    probe_candidates = candidates[:max_group_probes]
    if group_override:
        forced = next(
            (
                candidate
                for candidate in candidates
                if str(candidate.get("openConversationId") or "") == group_override
            ),
            {"openConversationId": group_override},
        )
        probe_candidates = [forced]
    for candidate in probe_candidates:
        candidate_id = str(candidate.get("openConversationId") or "")
        if not candidate_id:
            continue
        probe = run_capture(
            binary,
            "+messages-list",
            [
                "--group",
                candidate_id,
                "--time",
                time.strftime("%Y-%m-%d %H:%M:%S"),
                "--forward=false",
                "--limit",
                "50",
            ],
            timeout,
        )
        probe_summary = summarize_capture(probe)
        if probe_summary["status"] not in {"pass", "pass_empty"}:
            continue
        rows = first_list(probe.upper(), "messages")
        own_open_id = (
            str(candidate.get("ownerOpenDingtalkId") or "")
            if str(candidate.get("myRole") or "") == "OWNER"
            else ""
        )
        own_message = any(
            isinstance(item, dict) and item.get("senderId") == own_open_id
            for item in rows
        )
        probe_raw = probe.raw_calls[-1][2] if probe.raw_calls else {}
        has_resource = bool(
            first_string(probe_raw, "mediaId", "resourceId", "downloadCode")
        )
        has_thread = bool(
            first_string(
                probe_raw,
                "openConvThreadId",
                "openConversationThreadId",
                "threadId",
                "topicId",
            )
        )
        score = (
            int(probe_summary.get("upper", {}).get("count") or 0)
            + (1000 if own_message else 0)
            + (500 if has_resource else 0)
            + (500 if has_thread else 0)
        )
        if score > best_score:
            best_score = score
            best_group = candidate
            messages = probe
    if messages is not None:
        group = best_group

    group_id = str(group.get("openConversationId") or "")
    group_name = str(group.get("title") or group.get("name") or "")

    if group_id:
        conversation = run("+conversation-info", "--group", group_id)
        run("+chat-invite-url", "--group", group_id, "--expires-seconds", "60")
        run("+chat-bots", "--group", group_id)
        run("+chat-role-list", "--group", group_id)
        run("+messages-list-pin", "--open-conversation-id", group_id, "--size", "100")
        if messages is None:
            messages = run(
                "+messages-list",
                "--group",
                group_id,
                "--time",
                time.strftime("%Y-%m-%d %H:%M:%S"),
                "--forward=false",
                "--limit",
                "50",
            )
        else:
            captures["+messages-list"] = messages
            results["+messages-list"] = summarize_capture(messages)
        run(
            "+chat-messages",
            "--group",
            group_id,
            "--time",
            time.strftime("%Y-%m-%d %H:%M:%S"),
            "--direction",
            "older",
            "--limit",
            "50",
            retries=2,
        )
        if group_name:
            run("+chat-search", "--query", group_name, "--limit", "20")
            members = run("+group-members", "--group", group_name)
            run("+chat-members-list", "--group", group_name)
        else:
            results["+chat-search"] = blocked("+chat-search", "真实群需要有可搜索标题")
            results["+group-members"] = blocked("+group-members", "真实群需要有可搜索标题")
            results["+chat-members-list"] = blocked(
                "+chat-members-list", "真实群需要有可搜索标题"
            )
            members = None

        numeric_group_id = numeric_group_override or first_string(
            conversation.raw_calls[-1][2] if conversation.raw_calls else {},
            "groupId",
            "group_id",
        )
        if numeric_group_id and numeric_group_id.isdigit():
            run("+chat-get-by-id", "--group-id", numeric_group_id)
        else:
            results["+chat-get-by-id"] = blocked(
                "+chat-get-by-id",
                "需要一个真实数字群号 groupId；openConversationId 不能替代",
            )

        member_rows = (
            first_list(members.upper(), "members") if members is not None else []
        )
        member_open_id = first_string(
            member_rows,
            "openDingtalkId",
            "openDingTalkId",
            "memberDingtalkId",
        )
        if member_open_id:
            run(
                "+chat-members-get",
                "--id",
                group_id,
                "--users",
                member_open_id,
            )
            role_query_capture: Capture | None = None
            for member in member_rows[:max_member_probes]:
                candidate_open_id = first_string(
                    member,
                    "openDingtalkId",
                    "openDingTalkId",
                    "memberDingtalkId",
                )
                if not candidate_open_id:
                    continue
                probe = run_capture(
                    binary,
                    "+chat-role-query-user",
                    ["--group", group_id, "--user", candidate_open_id],
                    timeout,
                )
                role_query_capture = probe
                if summarize_capture(probe)["status"] in {"pass", "pass_empty"}:
                    break
            if role_query_capture is not None:
                captures["+chat-role-query-user"] = role_query_capture
                results["+chat-role-query-user"] = summarize_capture(
                    role_query_capture
                )
            direct_capture: Capture | None = None
            cross_org_only = True
            for member in member_rows[:max_member_probes]:
                candidate_open_id = first_string(
                    member,
                    "openDingtalkId",
                    "openDingTalkId",
                    "memberDingtalkId",
                )
                if not candidate_open_id:
                    continue
                probe = run_capture(
                    binary,
                    "+messages-list-direct",
                    [
                        "--open-dingtalk-id",
                        candidate_open_id,
                        "--time",
                        time.strftime("%Y-%m-%d %H:%M:%S"),
                        "--forward=false",
                        "--limit",
                        "20",
                    ],
                    timeout,
                )
                summary = summarize_capture(probe)
                if summary["status"] in {"pass", "pass_empty"}:
                    direct_capture = probe
                    break
                if summary.get("error", {}).get("server_error_code") != (
                    "CrossOrgPermissionDenied"
                ):
                    cross_org_only = False
                    direct_capture = probe
                    break
            if direct_capture is not None:
                captures["+messages-list-direct"] = direct_capture
                results["+messages-list-direct"] = summarize_capture(direct_capture)
            elif cross_org_only:
                results["+messages-list-direct"] = blocked(
                    "+messages-list-direct",
                    "需要一个当前账号有权读取单聊记录的同组织成员",
                )
        else:
            requirement = "需要一个可读取 openDingTalkId 的真实群成员"
            for command in (
                "+chat-members-get",
                "+chat-role-query-user",
                "+messages-list-direct",
            ):
                results[command] = blocked(command, requirement)

        message_rows = first_list(messages.upper(), "messages")
        own_open_id = (
            str(group.get("ownerOpenDingtalkId") or "")
            if str(group.get("myRole") or "") == "OWNER"
            else ""
        )
        own_message_rows = [
            item
            for item in message_rows
            if isinstance(item, dict) and item.get("senderId") == own_open_id
        ]
        message_id = first_string(
            own_message_rows or message_rows,
            "messageId",
            "openMessageId",
            "openMsgId",
        )
        if message_id:
            run("+messages-mget", "--msg-ids", message_id)
            if own_message_rows:
                run(
                    "+messages-read-status",
                    "--conversation-id",
                    group_id,
                    "--message-id",
                    message_id,
                )
            else:
                results["+messages-read-status"] = blocked(
                    "+messages-read-status",
                    "需要当前账号自己发送的一条真实消息 openMessageId",
                )
        else:
            requirement = "需要该群内至少一条可读取的真实消息 openMessageId"
            results["+messages-mget"] = blocked("+messages-mget", requirement)
            results["+messages-read-status"] = blocked(
                "+messages-read-status", requirement
            )

        keyword = safe_keyword(message_rows)
        run(
            "+search-msg",
            "--group",
            group_id,
            "--query",
            keyword,
            "--days",
            "30",
        )

        raw_messages = messages.raw_calls[-1][2] if messages.raw_calls else {}
        topic_id = topic_id_override or first_string(
            raw_messages,
            "openConvThreadId",
            "openConversationThreadId",
            "threadId",
            "topicId",
        )
        if topic_id:
            run(
                "+thread-replies",
                "--group",
                topic_group_override or group_id,
                "--topic-id",
                topic_id,
                "--time",
                time.strftime("%Y-%m-%d %H:%M:%S"),
                "--limit",
                "20",
            )
        else:
            results["+thread-replies"] = blocked(
                "+thread-replies",
                "需要一个真实话题群及其 topic/thread ID",
            )

        resource_id = media_resource_override
        resource_message_id = media_message_override
        resource_group_id = media_group_override or group_id
        for raw_message in first_list(raw_messages, "messages", "messageList"):
            if resource_id and resource_message_id:
                break
            if not isinstance(raw_message, dict):
                continue
            candidate_resource_id = first_string(
                raw_message,
                "mediaId",
                "resourceId",
                "downloadCode",
            )
            candidate_message_id = first_string(
                raw_message,
                "openMessageId",
                "openMsgId",
            )
            if candidate_resource_id and candidate_message_id:
                resource_id = candidate_resource_id
                resource_message_id = candidate_message_id
                break
        if resource_id and resource_message_id:
            run(
                "+messages-resource-url",
                "--type",
                "mediaId",
                "--resource-id",
                resource_id,
                "--message-id",
                resource_message_id,
                "--open-conversation-id",
                resource_group_id,
            )
            download_output = (
                Path("tmp")
                / "im-shortcut-live-audit"
                / f".read-resource-{os.getpid()}-{time.time_ns()}.bin"
            )
            download_output.parent.mkdir(parents=True, exist_ok=True)
            try:
                download_capture = run(
                    "+messages-resource-download",
                    "--type",
                    "mediaId",
                    "--resource-id",
                    resource_id,
                    "--message-id",
                    resource_message_id,
                    "--open-conversation-id",
                    resource_group_id,
                    "--output",
                    str(download_output),
                )
                download_summary = results["+messages-resource-download"]
                upper_download = download_capture.upper()
                expected_size = (
                    upper_download.get("sizeBytes")
                    if isinstance(upper_download, dict)
                    else None
                )
                if (
                    download_summary["status"] == "pass"
                    and (
                        not download_output.is_file()
                        or not isinstance(expected_size, (int, float))
                        or download_output.stat().st_size != expected_size
                    )
                ):
                    download_summary["status"] = "local_artifact_mismatch"
            finally:
                download_output.unlink(missing_ok=True)
        else:
            results["+messages-resource-url"] = blocked(
                "+messages-resource-url",
                "需要一条含 mediaId 的真实图片/视频/语音消息",
            )
            results["+messages-resource-download"] = blocked(
                "+messages-resource-download",
                "需要一条含 mediaId 的真实图片/视频/语音消息",
            )
    else:
        requirement = "当前账号需要至少加入一个可读真实群"
        for command in (
            "+conversation-info",
            "+chat-invite-url",
            "+chat-bots",
            "+chat-role-list",
            "+messages-list-pin",
            "+messages-list",
            "+chat-messages",
            "+chat-search",
            "+group-members",
            "+chat-members-list",
            "+chat-get-by-id",
            "+chat-members-get",
            "+chat-role-query-user",
            "+messages-list-direct",
            "+messages-mget",
            "+messages-read-status",
            "+search-msg",
            "+thread-replies",
            "+messages-resource-download",
            "+messages-resource-url",
        ):
            results[command] = blocked(command, requirement)

    open_task_id = open_task_override
    for capture in captures.values():
        if open_task_id:
            break
        for _, _, raw in capture.raw_calls:
            open_task_id = first_string(raw, "openTaskId", "taskId")
            if open_task_id:
                break
        if open_task_id:
            break
    if open_task_id:
        run("+messages-query-send-status", "--open-task-id", open_task_id)
    else:
        results["+messages-query-send-status"] = blocked(
            "+messages-query-send-status",
            "需要一次真实消息发送返回的 openTaskId",
        )

    return [results.get(command, blocked(command, "审计器未覆盖")) for command in READ_SHORTCUTS]


def report_markdown(results: list[dict[str, Any]]) -> str:
    counts: dict[str, int] = {}
    for result in results:
        counts[result["status"]] = counts.get(result["status"], 0) + 1
    lines = [
        "# Chat Shortcut live read audit",
        "",
        "This report was produced through the real Shortcut entry point and the",
        "lower MCP response captured from the same invocation. It intentionally",
        "contains no business payload values.",
        "",
        f"- Total: {len(results)}",
    ]
    for status in sorted(counts):
        lines.append(f"- {status}: {counts[status]}")
    lines.extend(
        [
            "",
            "| Shortcut | Status | Upper | Lower | Reactions U/L | Threads U/L | MCP calls | Evidence / requirement |",
            "|---|---|---:|---:|---:|---:|---:|---|",
        ]
    )
    for result in results:
        upper = result.get("upper", {}).get("count")
        lower = result.get("lower", {}).get("count")
        calls = result.get("lower", {}).get("call_count")
        upper_reactions = result.get("upper", {}).get("reaction_message_count")
        lower_reactions = result.get("lower", {}).get("reaction_message_count")
        upper_threads = result.get("upper", {}).get("thread_message_count")
        lower_threads = result.get("lower", {}).get("thread_message_count")
        tools = ", ".join(result.get("lower", {}).get("tools", []))
        evidence = result.get("requirement") or tools
        lines.append(
            f"| `{result['command']}` | {result['status']} | "
            f"{'' if upper is None else upper} | "
            f"{'' if lower is None else lower} | "
            f"{'' if upper_reactions is None else upper_reactions}/"
            f"{'' if lower_reactions is None else lower_reactions} | "
            f"{'' if upper_threads is None else upper_threads}/"
            f"{'' if lower_threads is None else lower_threads} | "
            f"{'' if calls is None else calls} | {evidence} |"
        )
    lines.append("")
    return "\n".join(lines)


def validate_report(results: list[dict[str, Any]]) -> None:
    commands = [result["command"] for result in results]
    if tuple(commands) != READ_SHORTCUTS:
        raise RuntimeError("read Shortcut audit coverage drifted from the expected set")
    encoded = json.dumps(results, ensure_ascii=False)
    for forbidden in ("stdout", "stderr", "backend_raw", "raw_backend", "inviteUrl"):
        if f'"{forbidden}"' in encoded:
            raise RuntimeError(f"sanitized report contains forbidden field {forbidden!r}")
    if "https://" in encoded or "http://" in encoded:
        raise RuntimeError("sanitized report appears to contain a URL")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--dws", required=True, type=Path, help="locally built dws binary")
    parser.add_argument("--out-dir", required=True, type=Path)
    parser.add_argument("--timeout", type=int, default=60)
    parser.add_argument("--max-group-probes", type=int, default=8)
    parser.add_argument("--max-member-probes", type=int, default=8)
    args = parser.parse_args()

    binary = args.dws.resolve()
    if not binary.is_file():
        parser.error(f"--dws is not a file: {binary}")
    numeric_group_id = os.environ.get("DWS_AUDIT_NUMERIC_GROUP_ID", "").strip()
    if numeric_group_id and not numeric_group_id.isdigit():
        parser.error("DWS_AUDIT_NUMERIC_GROUP_ID must contain only digits")
    topic_override = [
        os.environ.get("DWS_AUDIT_TOPIC_GROUP_ID", "").strip(),
        os.environ.get("DWS_AUDIT_TOPIC_ID", "").strip(),
    ]
    if any(topic_override) and not all(topic_override):
        parser.error("topic fixture override requires both group and topic IDs")
    media_override = [
        os.environ.get("DWS_AUDIT_MEDIA_GROUP_ID", "").strip(),
        os.environ.get("DWS_AUDIT_MEDIA_MESSAGE_ID", "").strip(),
        os.environ.get("DWS_AUDIT_MEDIA_RESOURCE_ID", "").strip(),
    ]
    if any(media_override) and not all(media_override):
        parser.error("media fixture override requires group, message, and resource IDs")
    results = live_audit(
        binary,
        args.timeout,
        args.max_group_probes,
        args.max_member_probes,
    )
    validate_report(results)

    args.out_dir.mkdir(parents=True, exist_ok=True)
    json_path = args.out_dir / "chat-read-live-audit.json"
    markdown_path = args.out_dir / "chat-read-live-audit.md"
    json_path.write_text(
        json.dumps(
            {
                "scope": "chat-read-shortcuts",
                "count": len(results),
                "results": results,
            },
            ensure_ascii=False,
            indent=2,
        )
        + "\n",
        encoding="utf-8",
    )
    markdown_path.write_text(report_markdown(results), encoding="utf-8")
    print(markdown_path)

    failures = {
        "projection_mismatch",
        "pagination_mismatch",
        "backend_error",
        "cli_error",
        "error_envelope",
        "timeout",
        "no_lower_capture",
        "local_artifact_mismatch",
    }
    return 1 if any(result["status"] in failures for result in results) else 0


if __name__ == "__main__":
    raise SystemExit(main())
