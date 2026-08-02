#!/usr/bin/env python3
"""Run reversible Chat Shortcut writes against disposable DWS fixtures.

The script creates single-user groups whose names start with
``DWS-SHORTCUT-AUDIT``. It invokes the real Shortcut entry point, captures the
lower MCP response from the same call, rolls back toggles where supported, and
dismisses the groups in ``finally``. The persisted report contains only command
names, lower tool names, counts, and sanitized error classes.

This is intentionally separate from the exhaustive 57-command dry-run audit:
commands requiring another human actor, a configured robot/Webhook, or a
pending join request are not fabricated and remain listed as external-fixture
requirements.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from run_chat_shortcut_live_audit import (
    Capture,
    error_facts,
    first_list,
    first_string,
    response_is_error,
    run_capture,
)


EXTERNAL_FIXTURE_COMMANDS = {
    "+chat-add-bot": "requires_configured_robot",
    "+chat-audit-join": "requires_pending_join_request_and_second_actor",
    "+chat-mute-member": "requires_second_test_member",
    "+chat-remove-bot": "requires_configured_robot",
    "+chat-set-admin": "requires_second_test_member",
    "+chat-transfer-owner": "requires_second_test_member",
    "+messages-batch-recall-by-bot": "requires_robot_code_and_process_key",
    "+messages-batch-send-by-bot": "requires_robot_code",
    "+messages-recall-by-bot": "requires_robot_code_and_process_key",
    "+messages-send-by-bot": "requires_robot_code",
    "+messages-send-by-webhook": "requires_webhook_token",
}


@dataclass
class NativeResult:
    exit_code: int | None
    payload: Any


def native(
    binary: Path,
    args: list[str],
    timeout: int,
    *,
    accept_error: bool = False,
) -> NativeResult:
    try:
        proc = subprocess.run(
            [str(binary), *args, "--format", "json"],
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
    except subprocess.TimeoutExpired:
        return NativeResult(None, {})
    try:
        payload = json.loads(proc.stdout or proc.stderr)
    except json.JSONDecodeError:
        payload = {}
    if not accept_error and (
        proc.returncode != 0 or not isinstance(payload, dict) or response_is_error(payload)
    ):
        raise RuntimeError(f"native DWS fixture command failed: {' '.join(args[:3])}")
    return NativeResult(proc.returncode, payload)


def objects(value: Any) -> list[dict[str, Any]]:
    found: list[dict[str, Any]] = []
    if isinstance(value, dict):
        found.append(value)
        for item in value.values():
            found.extend(objects(item))
    elif isinstance(value, list):
        for item in value:
            found.extend(objects(item))
    return found


def latest_message(binary: Path, group_id: str, marker: str, timeout: int) -> dict[str, Any]:
    for _ in range(5):
        result = native(
            binary,
            [
                "chat",
                "message",
                "list",
                "--group",
                group_id,
                "--time",
                time.strftime("%Y-%m-%d 00:00:00"),
                "--direction",
                "newer",
                "--limit",
                "50",
            ],
            timeout,
            accept_error=True,
        )
        payload = result.payload
        if result.exit_code != 0 or response_is_error(payload):
            time.sleep(1)
            continue
        rows = [
            item
            for item in objects(payload)
            if item.get("openMessageId") and marker in str(item.get("content") or "")
        ]
        if rows:
            return rows[-1]
        time.sleep(1)
    raise RuntimeError("sent fixture message did not become readable")


def find_media_fixture(binary: Path, timeout: int) -> tuple[str, str, str]:
    payload = native(
        binary,
        [
            "chat",
            "message",
            "list-all",
            "--start",
            time.strftime("%Y-%m-%d 00:00:00", time.localtime(time.time() - 31 * 86400)),
            "--end",
            time.strftime("%Y-%m-%d 23:59:59"),
            "--limit",
            "100",
            "--cursor",
            "0",
        ],
        timeout,
    ).payload
    for item in objects(payload):
        content = str(item.get("content") or "")
        match = re.search(r"mediaId=([^\s)]+)", content)
        if match and item.get("openMessageId") and item.get("openConversationId"):
            return (
                str(item["openConversationId"]),
                str(item["openMessageId"]),
                match.group(1),
            )
    return "", "", ""


def create_group(
    binary: Path, name: str, user_id: str, timeout: int, *, thread: bool = False
) -> str:
    args = [
        "chat",
        "group",
        "create",
        "--name",
        name,
        "--users",
        user_id,
        "--type",
        "NORMAL",
        "--yes",
    ]
    if thread:
        args.append("--thread")
    payload = native(binary, args, timeout).payload
    group_id = first_string(payload, "openConversationId")
    if not group_id:
        raise RuntimeError("created group response lacks openConversationId")
    return group_id


def dismiss_group(binary: Path, group_id: str, timeout: int) -> None:
    if not group_id:
        return
    native(
        binary,
        ["chat", "group", "dismiss", "--group", group_id, "--yes"],
        timeout,
        accept_error=True,
    )


def summarize_action(
    command: str, expected_tool: str, captures: list[Capture]
) -> dict[str, Any]:
    tools = [
        f"{server}/{tool}"
        for capture in captures
        for server, tool, _ in capture.raw_calls
    ]
    lower_errors = [
        raw
        for capture in captures
        for _, _, raw in capture.raw_calls
        if response_is_error(raw)
    ]
    failed = next(
        (
            capture
            for capture in captures
            if capture.timed_out
            or capture.exit_code != 0
            or capture.upper() is None
            or response_is_error(capture.upper())
        ),
        None,
    )
    expected_seen = any(tool.endswith("/" + expected_tool) for tool in tools)
    if failed is not None:
        status = "timeout" if failed.timed_out else "error"
    elif lower_errors:
        status = "lower_error"
    elif not expected_seen:
        status = "wrong_tool"
    else:
        status = "pass"
    return {
        "command": command,
        "status": status,
        "expected_tool": expected_tool,
        "invocation_count": len(captures),
        "lower_call_count": len(tools),
        "lower_tools": tools,
        "error": error_facts(failed.stderr) if failed is not None else {},
    }


def run_live(binary: Path, timeout: int) -> list[dict[str, Any]]:
    auth = native(binary, ["auth", "status"], timeout).payload
    user_id = str(auth.get("user_id") or "")
    user_name = str(auth.get("user_name") or "")
    if not user_id or not user_name:
        raise RuntimeError("auth status lacks user identity")

    suffix = time.strftime("%Y%m%d-%H%M%S")
    group_name = f"DWS-SHORTCUT-AUDIT-LIVE-{suffix}"
    thread_name = f"DWS-SHORTCUT-AUDIT-THREAD-LIVE-{suffix}"
    quit_name = f"DWS-SHORTCUT-AUDIT-QUIT-LIVE-{suffix}"
    group_id = ""
    thread_group_id = ""
    quit_group_id = ""
    category_id = ""
    role_id = ""
    results: list[dict[str, Any]] = []

    def shortcut_case(
        command: str,
        expected_tool: str,
        actions: list[list[str]],
    ) -> list[Capture]:
        captures = [
            run_capture(binary, command, [*args, "--yes"], timeout) for args in actions
        ]
        results.append(summarize_action(command, expected_tool, captures))
        return captures

    try:
        group_id = create_group(binary, group_name, user_id, timeout)
        thread_group_id = create_group(
            binary, thread_name, user_id, timeout, thread=True
        )
        quit_group_id = create_group(binary, quit_name, user_id, timeout)

        members = run_capture(
            binary,
            "+chat-members-list",
            ["--conversation-id", group_id, "--member-types", "user"],
            timeout,
        )
        member_open_id = first_string(
            members.upper(), "openDingtalkId", "openDingTalkId"
        )
        if not member_open_id:
            raise RuntimeError("single-user fixture group lacks member identity")

        marker = f"DWS-SHORTCUT-AUDIT-WRITE-{suffix}"
        native(
            binary,
            [
                "chat",
                "message",
                "send",
                "--group",
                group_id,
                "--text",
                marker,
                "--uuid",
                f"dws-shortcut-audit-{suffix}-seed",
                "--yes",
            ],
            timeout,
        )
        message = latest_message(binary, group_id, marker, timeout)
        message_id = str(message["openMessageId"])

        combine_marker = f"{marker}-COMBINE-SECOND"
        native(
            binary,
            [
                "chat",
                "message",
                "send",
                "--group",
                group_id,
                "--text",
                combine_marker,
                "--uuid",
                f"dws-shortcut-audit-{suffix}-combine-second",
                "--yes",
            ],
            timeout,
        )
        combine_message = latest_message(
            binary, group_id, combine_marker, timeout
        )
        combine_message_id = str(combine_message["openMessageId"])

        thread_marker = f"DWS-SHORTCUT-AUDIT-THREAD-{suffix}"
        native(
            binary,
            [
                "chat",
                "message",
                "send",
                "--group",
                thread_group_id,
                "--text",
                thread_marker,
                "--uuid",
                f"dws-shortcut-audit-{suffix}-thread",
                "--yes",
            ],
            timeout,
        )
        topic = latest_message(binary, thread_group_id, thread_marker, timeout)
        topic_message_id = str(topic["openMessageId"])
        topic_id = str(topic.get("openConvThreadId") or "")

        category_create = shortcut_case(
            "+category-create",
            "create_conv_category",
            [["--title", f"DWS审计{suffix[-6:]}"]],
        )
        category_id = first_string(category_create[-1].upper(), "categoryId")
        if category_id:
            shortcut_case(
                "+category-rename",
                "rename_conv_category",
                [["--category-id", category_id, "--title", f"DWS改名{suffix[-6:]}"]],
            )
            shortcut_case(
                "+category-add-conversation",
                "add_conv_to_categories",
                [["--group", group_id, "--category-ids", category_id]],
            )
            shortcut_case(
                "+category-remove-conversation",
                "remove_conv_from_categories",
                [["--group", group_id, "--category-ids", category_id]],
            )
            shortcut_case(
                "+category-delete",
                "delete_conv_category",
                [["--category-id", category_id]],
            )
            category_id = ""

        shortcut_case(
            "+chat-mute",
            "set_group_mute",
            [["--group", group_id], ["--group", group_id, "--off"]],
        )
        shortcut_case(
            "+chat-set-history",
            "update_show_history_msg_option",
            [
                ["--group", group_id, "--option", "RECENT_100"],
                ["--group", group_id, "--option", "FORBIDDEN"],
            ],
        )
        shortcut_case(
            "+chat-update-alias",
            "update_user_group_alias",
            [["--group", group_id, "--alias-title", f"DWS审计备注-{suffix}"]],
        )
        _, _, media_id = find_media_fixture(binary, timeout)
        if media_id:
            shortcut_case(
                "+chat-update-icon",
                "update_group_icon",
                [["--group", group_id, "--icon-media-id", media_id]],
            )
        shortcut_case(
            "+chat-update-nick",
            "update_group_nick",
            [["--group", group_id, "--nick", f"DWS审计昵称-{suffix[-6:]}"]],
        )
        shortcut_case(
            "+chat-update-settings",
            "update_group_settings",
            [
                ["--group", group_id, "--setting-key", "searchable", "--status", "1"],
                ["--group", group_id, "--setting-key", "searchable", "--status", "0"],
            ],
        )
        role_create = shortcut_case(
            "+chat-role-add",
            "add_custom_group_role",
            [["--group", group_id, "--name", f"DWS审计身份-{suffix[-6:]}"]],
        )
        role_id = first_string(role_create[-1].upper(), "openRoleId", "roleId")
        if role_id:
            shortcut_case(
                "+chat-role-update",
                "update_custom_group_role",
                [
                    [
                        "--group",
                        group_id,
                        "--role-id",
                        role_id,
                        "--name",
                        f"DWS审计身份-已更新-{suffix[-6:]}",
                    ]
                ],
            )
            shortcut_case(
                "+chat-role-set-user",
                "set_custom_user_roles",
                [
                    [
                        "--group",
                        group_id,
                        "--user",
                        member_open_id,
                        "--role-ids",
                        role_id,
                    ]
                ],
            )
            shortcut_case(
                "+chat-role-remove-user",
                "remove_custom_user_roles",
                [
                    [
                        "--group",
                        group_id,
                        "--user",
                        member_open_id,
                        "--role-ids",
                        role_id,
                    ]
                ],
            )
            shortcut_case(
                "+chat-role-remove",
                "remove_custom_group_role",
                [["--group", group_id, "--role-id", role_id]],
            )
            role_id = ""

        shortcut_case(
            "+conversation-mute",
            "update_notification_off",
            [
                ["--conversation-id", group_id],
                ["--conversation-id", group_id, "--off"],
            ],
        )
        shortcut_case(
            "+conversation-mute-at-all",
            "update_at_all_notification_off",
            [
                ["--conversation-id", group_id],
                ["--conversation-id", group_id, "--off"],
            ],
        )
        shortcut_case(
            "+conversation-mute-red-envelope",
            "update_red_env_notification_off",
            [
                ["--conversation-id", group_id],
                ["--conversation-id", group_id, "--off"],
            ],
        )
        shortcut_case(
            "+conversation-set-top",
            "set_top_conversation",
            [
                ["--conversation-id", group_id],
                ["--conversation-id", group_id, "--off"],
            ],
        )
        shortcut_case(
            "+conversation-mark-unread",
            "mark_conversation_unread",
            [["--conversation-id", group_id]],
        )
        shortcut_case(
            "+conversation-clear-red-point",
            "clear_conversation_red_point",
            [["--conversation-id", group_id]],
        )
        shortcut_case(
            "+conversation-mark-read",
            "mark_message_read",
            [["--conversation-id", group_id, "--message-id", message_id]],
        )
        shortcut_case(
            "+conversation-clear-all-red-point",
            "clear_all_red_point",
            [[]],
        )

        shortcut_case(
            "+send-to-group",
            "send_personal_message",
            [["--group", group_name, "--text", f"{marker}-send-to-group"]],
        )
        shortcut_case(
            "+dm",
            "send_personal_message",
            [["--to", user_name, "--text", f"{marker}-dm-self"]],
        )
        shortcut_case(
            "+broadcast",
            "send_personal_message",
            [["--to", user_name, "--text", f"{marker}-broadcast-self"]],
        )

        shortcut_case(
            "+messages-add-emoji",
            "add_emoji_reaction",
            [
                [
                    "--conversation-id",
                    group_id,
                    "--msg-id",
                    message_id,
                    "--emoji",
                    "赞",
                ]
            ],
        )
        shortcut_case(
            "+messages-remove-emoji",
            "remove_emoji_reaction",
            [
                [
                    "--conversation-id",
                    group_id,
                    "--msg-id",
                    message_id,
                    "--emoji",
                    "赞",
                ]
            ],
        )
        shortcut_case(
            "+messages-set-pin",
            "set_pin_message",
            [["--open-conversation-id", group_id, "--msg-id", message_id]],
        )
        shortcut_case(
            "+messages-unset-pin",
            "unset_pin_message",
            [["--open-conversation-id", group_id, "--msg-id", message_id]],
        )
        shortcut_case(
            "+messages-set-top",
            "set_top_message",
            [["--open-conversation-id", group_id, "--msg-id", message_id]],
        )
        shortcut_case(
            "+messages-unset-top",
            "unset_top_message",
            [["--open-conversation-id", group_id, "--msg-id", message_id]],
        )
        shortcut_case(
            "+messages-forward",
            "forward_message",
            [
                [
                    "--src-conversation-id",
                    group_id,
                    "--msg-id",
                    message_id,
                    "--dest-conversation-id",
                    group_id,
                    "--uuid",
                    f"dws-forward-{suffix}",
                ]
            ],
        )
        shortcut_case(
            "+messages-combine-forward",
            "combine_forward_messages",
            [
                [
                    "--src-conversation-id",
                    group_id,
                    "--msg-ids",
                    f"{message_id},{combine_message_id}",
                    "--dest-conversation-id",
                    group_id,
                    "--uuid",
                    f"dws-combine-{suffix}",
                ]
            ],
        )
        if topic_id:
            shortcut_case(
                "+messages-forward-topic",
                "forward_topic",
                [
                    [
                        "--src-msg-id",
                        topic_message_id,
                        "--src-conversation-id",
                        thread_group_id,
                        "--src-thread-id",
                        topic_id,
                        "--dest-conversation-id",
                        group_id,
                    ]
                ],
            )

        emotion_create = shortcut_case(
            "+messages-create-text-emotion",
            "create_text_emotion",
            [
                [
                    "--emotion-name",
                    f"DWS审计-{suffix[-6:]}",
                    "--text",
                    "audit",
                    "--background-id",
                    "im_bg_5",
                ]
            ],
        )
        emotion_id = first_string(emotion_create[-1].upper(), "emotionId")
        if emotion_id:
            emotion_args = [
                "--conversation-id",
                group_id,
                "--msg-id",
                message_id,
                "--emotion-id",
                emotion_id,
                "--emotion-name",
                f"DWS审计-{suffix[-6:]}",
                "--text",
                "audit",
                "--background-id",
                "im_bg_5",
            ]
            shortcut_case(
                "+messages-add-text-emotion",
                "add_text_emotion",
                [emotion_args],
            )
            shortcut_case(
                "+messages-remove-text-emotion",
                "remove_text_emotion",
                [emotion_args],
            )

        card_create = shortcut_case(
            "+messages-send-card",
            "create_and_send_card",
            [["--group", group_id]],
        )
        biz_id = first_string(card_create[-1].upper(), "bizId")
        if biz_id:
            shortcut_case(
                "+messages-update-card",
                "update_streaming_card",
                [
                    [
                        "--biz-id",
                        biz_id,
                        "--content",
                        f"{marker}-card-finished",
                        "--flow-status",
                        "3",
                    ]
                ],
            )

        recall_marker = f"{marker}-recall"
        native(
            binary,
            [
                "chat",
                "message",
                "send",
                "--group",
                group_id,
                "--text",
                recall_marker,
                "--uuid",
                f"dws-recall-{suffix}",
                "--yes",
            ],
            timeout,
        )
        recall_message = latest_message(binary, group_id, recall_marker, timeout)
        shortcut_case(
            "+messages-recall",
            "recall_message",
            [
                [
                    "--conversation-id",
                    group_id,
                    "--msg-id",
                    str(recall_message["openMessageId"]),
                ]
            ],
        )

        shortcut_case(
            "+conversation-hide",
            "hide_conversation",
            [["--conversation-id", group_id]],
        )
        shortcut_case(
            "+conversation-clear-messages",
            "clear_conversation_messages",
            [["--conversation-id", group_id]],
        )
        quit_result = shortcut_case(
            "+chat-quit",
            "quit_group",
            [["--group", quit_group_id]],
        )
        if summarize_action("+chat-quit", "quit_group", quit_result)["status"] == "pass":
            quit_group_id = ""

        dismiss_capture = shortcut_case(
            "+chat-dismiss",
            "dismiss_group",
            [["--group", group_id]],
        )
        if summarize_action("+chat-dismiss", "dismiss_group", dismiss_capture)["status"] == "pass":
            group_id = ""

        for command, requirement in sorted(EXTERNAL_FIXTURE_COMMANDS.items()):
            results.append(
                {
                    "command": command,
                    "status": "external_fixture_required",
                    "requirement": requirement,
                }
            )
        return sorted(results, key=lambda item: item["command"])
    finally:
        if category_id:
            run_capture(
                binary,
                "+category-delete",
                ["--category-id", category_id, "--yes"],
                timeout,
            )
        if role_id:
            run_capture(
                binary,
                "+chat-role-remove",
                ["--group", group_id, "--role-id", role_id, "--yes"],
                timeout,
            )
        dismiss_group(binary, quit_group_id, timeout)
        dismiss_group(binary, thread_group_id, timeout)
        dismiss_group(binary, group_id, timeout)


def markdown(results: list[dict[str, Any]]) -> str:
    counts: dict[str, int] = {}
    for item in results:
        counts[item["status"]] = counts.get(item["status"], 0) + 1
    lines = [
        "# Chat Shortcut live write audit",
        "",
        "Disposable DWS-created fixtures were used. Reversible toggles were rolled",
        "back and all temporary groups were dismissed. No business values are stored.",
        "",
        f"- Total covered: {len(results)}",
    ]
    for status in sorted(counts):
        lines.append(f"- {status}: {counts[status]}")
    lines.extend(
        [
            "",
            "| Shortcut | Status | Invocations | Lower tools / requirement |",
            "|---|---|---:|---|",
        ]
    )
    for item in results:
        evidence = ", ".join(item.get("lower_tools", [])) or item.get(
            "requirement", ""
        )
        lines.append(
            f"| `{item['command']}` | {item['status']} | "
            f"{item.get('invocation_count', '')} | {evidence} |"
        )
    lines.append("")
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--dws", required=True, type=Path)
    parser.add_argument("--out-dir", required=True, type=Path)
    parser.add_argument("--timeout", type=int, default=60)
    parser.add_argument(
        "--yes-live",
        action="store_true",
        help="required acknowledgement that real temporary writes will occur",
    )
    args = parser.parse_args()
    if not args.yes_live:
        parser.error("--yes-live is required")
    binary = args.dws.resolve()
    if not binary.is_file():
        parser.error(f"--dws is not a file: {binary}")

    results = run_live(binary, args.timeout)
    args.out_dir.mkdir(parents=True, exist_ok=True)
    payload = {
        "scope": "chat-write-shortcuts-live-disposable",
        "count": len(results),
        "results": results,
    }
    json_path = args.out_dir / "chat-write-live-audit.json"
    md_path = args.out_dir / "chat-write-live-audit.md"
    json_path.write_text(
        json.dumps(payload, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    md_path.write_text(markdown(results), encoding="utf-8")
    print(md_path)
    return 1 if any(
        item["status"] not in {"pass", "external_fixture_required"}
        for item in results
    ) else 0


if __name__ == "__main__":
    raise SystemExit(main())
