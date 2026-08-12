#!/usr/bin/env python3
"""Audit all Chat write Shortcuts with real fixtures and --dry-run.

The script discovers reusable IDs through real read calls, then invokes every
write/high-risk Chat Shortcut with --dry-run. It verifies that no write MCP
response was observed and that each ordinary Shortcut rendered the expected
tool plus required lower-layer argument keys. Reports contain fixture kinds
and argument keys only, never fixture values or business payloads.
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

SCRIPT_DIR = Path(__file__).resolve().parent
sys.path.insert(0, str(SCRIPT_DIR))

from run_chat_shortcut_live_audit import (  # noqa: E402
    Capture,
    first_list,
    first_string,
    run_capture,
)


@dataclass(frozen=True)
class DryRunCase:
    command: str
    args: tuple[str, ...]
    tool: str | None
    required_argument_keys: frozenset[str] = frozenset()
    semantic_preview: bool = False


WRITE_COMMANDS = (
    "+broadcast",
    "+category-add-conversation",
    "+category-create",
    "+category-delete",
    "+category-remove-conversation",
    "+category-rename",
    "+chat-add-bot",
    "+chat-audit-join",
    "+chat-create",
    "+chat-dismiss",
    "+chat-mute",
    "+chat-mute-member",
    "+chat-quit",
    "+chat-remove-bot",
    "+chat-role-add",
    "+chat-role-remove",
    "+chat-role-remove-user",
    "+chat-role-set-user",
    "+chat-role-update",
    "+chat-set-admin",
    "+chat-set-history",
    "+chat-transfer-owner",
    "+chat-update",
    "+chat-update-alias",
    "+chat-update-icon",
    "+chat-update-nick",
    "+chat-update-settings",
    "+conversation-clear-all-red-point",
    "+conversation-clear-messages",
    "+conversation-clear-red-point",
    "+conversation-hide",
    "+conversation-mark-read",
    "+conversation-mark-unread",
    "+conversation-mute",
    "+conversation-mute-at-all",
    "+conversation-mute-red-envelope",
    "+conversation-set-top",
    "+dm",
    "+flag-cancel",
    "+flag-create",
    "+messages-add-emoji",
    "+messages-add-text-emotion",
    "+messages-batch-recall-by-bot",
    "+messages-batch-send-by-bot",
    "+messages-combine-forward",
    "+messages-create-text-emotion",
    "+messages-forward",
    "+messages-forward-topic",
    "+messages-recall",
    "+messages-recall-by-bot",
    "+messages-remove-emoji",
    "+messages-remove-text-emotion",
    "+messages-reply",
    "+messages-send",
    "+messages-send-by-bot",
    "+messages-send-by-webhook",
    "+messages-send-card",
    "+messages-set-pin",
    "+messages-set-top",
    "+messages-unset-pin",
    "+messages-unset-top",
    "+messages-update-card",
    "+send-to-group",
)

READ_TOOLS_ALLOWED_DURING_DRY_RUN = {
    ("contact", "get_current_user_profile"),
    ("contact", "search_contact_by_key_word"),
    ("im", "search_groups"),
}


class FixtureSet:
    def __init__(self) -> None:
        self.values: dict[str, str] = {}
        self.sources: dict[str, str] = {}

    def put(self, name: str, value: str, fallback: str) -> str:
        clean = str(value or "").strip()
        if clean:
            self.values[name] = clean
            self.sources[name] = "real"
        else:
            self.values[name] = fallback
            self.sources[name] = "synthetic"
        return self.values[name]

    def __getitem__(self, name: str) -> str:
        return self.values[name]


def read_json_command(binary: Path, args: list[str], timeout: int) -> Any:
    try:
        proc = subprocess.run(
            [str(binary), *args, "--format", "json"],
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
        return json.loads(proc.stdout) if proc.returncode == 0 else {}
    except (subprocess.TimeoutExpired, json.JSONDecodeError):
        return {}


def discover_fixtures(binary: Path, timeout: int) -> FixtureSet:
    fx = FixtureSet()
    auth = read_json_command(binary, ["auth", "status"], timeout)
    user_name = fx.put("user_name", str(auth.get("user_name") or ""), "DWS审计用户")
    user_id = fx.put("user_id", str(auth.get("user_id") or ""), "dws_audit_user")

    category = run_capture(binary, "+category-list", [], timeout)
    category_id = fx.put(
        "category_id",
        first_string(category.upper(), "categoryId", "category_id"),
        "1",
    )

    mine = run_capture(binary, "+chat-list-mine", ["--limit", "200"], timeout)
    group_rows = first_list(mine.upper(), "groups")
    groups = [item for item in group_rows if isinstance(item, dict)]
    group = groups[0] if groups else {}
    messages: Capture | None = None
    for candidate in groups[:8]:
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
        if first_string(
            first_list(probe.upper(), "messages"),
            "messageId",
            "openMessageId",
            "openMsgId",
        ):
            group = candidate
            messages = probe
            break
    group_id = fx.put(
        "group_id",
        str(group.get("openConversationId") or ""),
        "cid_dws_shortcut_audit",
    )
    group_name = fx.put(
        "group_name",
        str(group.get("name") or group.get("title") or ""),
        "DWS Shortcut 审计群",
    )

    members = run_capture(binary, "+group-members", ["--group", group_name], timeout)
    member_rows = first_list(members.upper(), "members")
    member_open_id = fx.put(
        "member_open_id",
        first_string(
            member_rows,
            "openDingtalkId",
            "openDingTalkId",
            "memberDingtalkId",
        ),
        "did_dws_shortcut_audit",
    )

    if messages is None:
        messages = run_capture(
            binary,
            "+messages-list",
            [
                "--group",
                group_id,
                "--time",
                time.strftime("%Y-%m-%d %H:%M:%S"),
                "--forward=false",
                "--limit",
                "50",
            ],
            timeout,
        )
    message_rows = first_list(messages.upper(), "messages")
    message_id = fx.put(
        "message_id",
        first_string(message_rows, "messageId", "openMessageId", "openMsgId"),
        "msg_dws_shortcut_audit",
    )

    roles = run_capture(binary, "+chat-role-list", ["--group", group_id], timeout)
    role_id = fx.put(
        "role_id",
        first_string(roles.upper(), "openRoleId", "roleId"),
        "role_dws_shortcut_audit",
    )

    group_bots = run_capture(binary, "+chat-bots", ["--group", group_id], timeout)
    bot_id = fx.put(
        "bot_id",
        first_string(group_bots.upper(), "openBotId", "botId"),
        "bot_dws_shortcut_audit",
    )

    bots = run_capture(
        binary,
        "+bot-find",
        ["--query", "机器人", "--limit", "20"],
        timeout,
    )
    robot_code = fx.put(
        "robot_code",
        first_string(bots.upper(), "robotCode", "code"),
        "robot_dws_shortcut_audit",
    )

    joins = run_capture(
        binary,
        "+chat-list-join-requests",
        ["--limit", "50"],
        timeout,
    )
    join_raw = joins.raw_calls[-1][2] if joins.raw_calls else joins.upper()
    fx.put(
        "join_record_id",
        first_string(join_raw, "applyRecordId", "recordId"),
        "1",
    )
    fx.put(
        "join_applicant",
        first_string(join_raw, "applicantUid", "applicantUserId"),
        user_id,
    )
    fx.put(
        "join_inviter",
        first_string(join_raw, "inviterUid", "inviterUserId"),
        user_id,
    )

    # The current read surface does not guarantee these resource types. They
    # remain syntactically valid dry-run values and are explicitly reported as
    # synthetic until a disposable live fixture supplies real values.
    fx.put("media_id", "", "@dws_shortcut_audit_media")
    fx.put("topic_id", "", "thread_dws_shortcut_audit")
    fx.put("process_key", "", "process_dws_shortcut_audit")
    fx.put("emotion_id", "", "emotion_dws_shortcut_audit")
    fx.put("biz_id", "", "biz_dws_shortcut_audit")
    fx.put("webhook_token", "", "token_dws_shortcut_audit")
    fx.put("uuid", "", "dws-shortcut-audit-uuid")
    fx.put("message_text", "", "[DWS Shortcut dry-run audit: not sent]")
    # The IM category backend accepts at most 15 Unicode code points.
    fx.put("category_title", "", "DWS dry-run")
    fx.put("role_name", "", "DWS审计身份")
    fx.put("alias_title", "", "DWS审计群备注")
    fx.put("nick", "", "DWS审计昵称")
    fx.put("emotion_name", "", "审计")
    fx.put("emotion_text", "", "dry-run")
    fx.put("emotion_background", "", "im_bg_5")
    return fx


def case(
    command: str,
    args: list[str],
    tool: str | None,
    *required_keys: str,
    semantic_preview: bool = False,
) -> DryRunCase:
    return DryRunCase(
        command=command,
        args=tuple(args),
        tool=tool,
        required_argument_keys=frozenset(required_keys),
        semantic_preview=semantic_preview,
    )


def build_cases(fx: FixtureSet) -> list[DryRunCase]:
    g = fx["group_id"]
    u = fx["member_open_id"]
    m = fx["message_id"]
    r = fx["role_id"]
    rc = fx["robot_code"]
    text = fx["message_text"]
    cid = fx["category_id"]
    return [
        case(
            "+broadcast",
            ["--to", fx["user_name"], "--text", text, "--ai-tag"],
            "send_personal_message",
            "receiverOpenDingTalkId",
            "msgType",
            "content",
            semantic_preview=True,
        ),
        case(
            "+category-add-conversation",
            ["--group", g, "--category-ids", cid],
            "add_conv_to_categories",
            "openConversationId",
            "categoryIds",
        ),
        case("+category-create", ["--title", fx["category_title"]], "create_conv_category", "title"),
        case("+category-delete", ["--category-id", cid], "delete_conv_category", "categoryId"),
        case(
            "+category-remove-conversation",
            ["--group", g, "--category-ids", cid],
            "remove_conv_from_categories",
            "openConversationId",
            "categoryIds",
        ),
        case(
            "+category-rename",
            ["--category-id", cid, "--title", fx["category_title"]],
            "rename_conv_category",
            "categoryId",
            "title",
        ),
        case(
            "+chat-add-bot",
            ["--robot-code", rc, "--id", g],
            "add_robot_to_group",
            "robotCode",
            "openConversationId",
        ),
        case(
            "+chat-audit-join",
            [
                "--group",
                g,
                "--record-id",
                fx["join_record_id"],
                "--applicant",
                fx["join_applicant"],
                "--inviter",
                fx["join_inviter"],
                "--status",
                "AuditApprove",
                "--description",
                "DWS dry-run",
            ],
            "audit_join_group",
            "openConversationId",
            "applyRecordId",
            "applicantUid",
            "inviterUid",
            "status",
            "auditDescription",
        ),
        case(
            "+chat-create",
            [
                "--name",
                "DWS dry-run group",
                "--users",
                fx["user_id"],
                "--type",
                "INTERNAL",
            ],
            "create_group_conversation",
            "groupName",
            "groupMembers",
            "groupType",
        ),
        case("+chat-dismiss", ["--group", g], "dismiss_group", "openConversationId"),
        case("+chat-mute", ["--group", g], "set_group_mute", "openConversationId", "mute"),
        case(
            "+chat-mute-member",
            ["--group", g, "--users", u, "--mute-time", "300000"],
            "set_group_member_mute_list",
            "openConversationId",
            "cid",
            "openDingTalkIds",
            "mute",
            "muteTime",
        ),
        case("+chat-quit", ["--group", g], "quit_group", "openConversationId"),
        case(
            "+chat-remove-bot",
            ["--id", g, "--bot-id", fx["bot_id"]],
            "remove_robot_in_group",
            "openConversationId",
            "openBotId",
        ),
        case(
            "+chat-role-add",
            ["--group", g, "--name", fx["role_name"]],
            "add_custom_group_role",
            "openConversationId",
            "name",
        ),
        case(
            "+chat-role-remove",
            ["--group", g, "--role-id", r],
            "remove_custom_group_role",
            "openConversationId",
            "openRoleId",
        ),
        case(
            "+chat-role-remove-user",
            ["--group", g, "--user", u, "--role-ids", r],
            "remove_custom_user_roles",
            "openConversationId",
            "openDingTalkId",
            "openRoleIds",
        ),
        case(
            "+chat-role-set-user",
            ["--group", g, "--user", u, "--role-ids", r],
            "set_custom_user_roles",
            "openConversationId",
            "openDingTalkId",
            "openRoleIds",
        ),
        case(
            "+chat-role-update",
            ["--group", g, "--role-id", r, "--name", fx["role_name"]],
            "update_custom_group_role",
            "openConversationId",
            "openRoleId",
            "name",
        ),
        case(
            "+chat-set-admin",
            ["--group", g, "--users", u],
            "update_conv_member_roles",
            "openConversationId",
            "openDingTalkIds",
            "admin",
        ),
        case(
            "+chat-set-history",
            ["--group", g, "--option", "RECENT_100"],
            "update_show_history_msg_option",
            "openConversationId",
            "option",
        ),
        case(
            "+chat-transfer-owner",
            ["--group", g, "--new-owner", u],
            "transfer_group_owner",
            "openConversationId",
            "cid",
            "newOwnerOpenDingTalkId",
        ),
        case(
            "+chat-update",
            ["--group", g, "--name", "DWS dry-run group"],
            "update_group_name",
            "openconversation_id",
            "group_name",
        ),
        case(
            "+chat-update-alias",
            ["--group", g, "--alias-title", fx["alias_title"]],
            "update_user_group_alias",
            "openConversationId",
            "aliasTitle",
        ),
        case(
            "+chat-update-icon",
            ["--group", g, "--icon-media-id", fx["media_id"]],
            "update_group_icon",
            "openConversationId",
            "iconMediaId",
        ),
        case(
            "+chat-update-nick",
            ["--group", g, "--nick", fx["nick"]],
            "update_group_nick",
            "openConversationId",
            "nick",
        ),
        case(
            "+chat-update-settings",
            ["--group", g, "--setting-key", "searchable", "--status", "1"],
            "update_group_settings",
            "openConversationId",
            "settingKey",
            "status",
        ),
        case("+conversation-clear-all-red-point", [], "clear_all_red_point"),
        case(
            "+conversation-clear-messages",
            ["--conversation-id", g],
            "clear_conversation_messages",
            "openConversationId",
            "cid",
        ),
        case(
            "+conversation-clear-red-point",
            ["--conversation-id", g],
            "clear_conversation_red_point",
            "openConversationId",
            "cid",
        ),
        case(
            "+conversation-hide",
            ["--conversation-id", g],
            "hide_conversation",
            "openConversationId",
            "cid",
        ),
        case(
            "+conversation-mark-read",
            ["--conversation-id", g, "--message-id", m],
            "mark_message_read",
            "openConversationId",
            "openMessageId",
        ),
        case(
            "+conversation-mark-unread",
            ["--conversation-id", g],
            "mark_conversation_unread",
            "openConversationId",
            "cid",
        ),
        case(
            "+conversation-mute",
            ["--conversation-id", g],
            "update_notification_off",
            "openConversationId",
            "cid",
            "mute",
        ),
        case(
            "+conversation-mute-at-all",
            ["--conversation-id", g],
            "update_at_all_notification_off",
            "openConversationId",
            "mute",
        ),
        case(
            "+conversation-mute-red-envelope",
            ["--conversation-id", g],
            "update_red_env_notification_off",
            "openConversationId",
            "mute",
        ),
        case(
            "+conversation-set-top",
            ["--conversation-id", g],
            "set_top_conversation",
            "openConversationId",
            "cid",
            "top",
            semantic_preview=True,
        ),
        case(
            "+dm",
            ["--to", fx["user_name"], "--text", text, "--ai-tag"],
            "send_personal_message",
            "receiverOpenDingTalkId",
            "msgType",
            "content",
        ),
        case(
            "+flag-cancel",
            ["--message-id", m, "--conversation-id", g],
            "remove_message_favorite",
            "openMessageId",
            "openConversationId",
            semantic_preview=True,
        ),
        case(
            "+flag-create",
            ["--message-id", m, "--conversation-id", g],
            "add_message_favorite",
            "openMessageId",
            "openConversationId",
            semantic_preview=True,
        ),
        case(
            "+messages-add-emoji",
            ["--conversation-id", g, "--msg-id", m, "--emoji", "赞"],
            "add_emoji_reaction",
            "openConversationId",
            "openMsgId",
            "emojiName",
        ),
        case(
            "+messages-add-text-emotion",
            [
                "--conversation-id",
                g,
                "--msg-id",
                m,
                "--emotion-id",
                fx["emotion_id"],
                "--emotion-name",
                fx["emotion_name"],
                "--text",
                fx["emotion_text"],
                "--background-id",
                fx["emotion_background"],
            ],
            "add_text_emotion",
            "openConversationId",
            "openMsgId",
            "emotionId",
            "emotionName",
            "text",
            "backgroundId",
        ),
        case(
            "+messages-batch-recall-by-bot",
            ["--robot-code", rc, "--keys", fx["process_key"]],
            "batch_recall_robot_users_msg",
            "robotCode",
            "processQueryKeys",
        ),
        case(
            "+messages-batch-send-by-bot",
            [
                "--robot-code",
                rc,
                "--title",
                "DWS dry-run",
                "--text",
                text,
                "--open-dingtalk-ids",
                u,
                "--at-all",
            ],
            "batch_send_robot_msg_to_users",
            "robotCode",
            "title",
            "markdown",
            "openDingtalkIds",
            "isAtAll",
        ),
        case(
            "+messages-combine-forward",
            [
                "--src-conversation-id",
                g,
                "--msg-ids",
                m,
                "--dest-conversation-id",
                g,
                "--uuid",
                fx["uuid"],
            ],
            "combine_forward_messages",
            "srcOpenCid",
            "srcOpenMessageIds",
            "destOpenCid",
            "uuid",
        ),
        case(
            "+messages-create-text-emotion",
            [
                "--emotion-name",
                fx["emotion_name"],
                "--text",
                fx["emotion_text"],
                "--background-id",
                fx["emotion_background"],
            ],
            "create_text_emotion",
            "emotionName",
            "text",
            "backgroundId",
        ),
        case(
            "+messages-forward",
            [
                "--src-conversation-id",
                g,
                "--msg-id",
                m,
                "--dest-conversation-id",
                g,
                "--uuid",
                fx["uuid"],
            ],
            "forward_message",
            "srcOpenCid",
            "srcOpenMessageId",
            "destOpenCid",
            "uuid",
        ),
        case(
            "+messages-forward-topic",
            [
                "--src-msg-id",
                m,
                "--src-conversation-id",
                g,
                "--src-thread-id",
                fx["topic_id"],
                "--dest-conversation-id",
                g,
            ],
            "forward_topic",
            "srcOpenMessageId",
            "srcOpenConversationId",
            "srcOpenConvThreadId",
            "destOpenConversationId",
        ),
        case(
            "+messages-recall",
            ["--conversation-id", g, "--msg-id", m],
            "recall_message",
            "openConversationId",
            "openMessageId",
        ),
        case(
            "+messages-recall-by-bot",
            ["--robot-code", rc, "--group", g, "--keys", fx["process_key"]],
            "recall_robot_group_message",
            "robotCode",
            "openConversationId",
            "processQueryKeys",
        ),
        case(
            "+messages-remove-emoji",
            ["--conversation-id", g, "--msg-id", m, "--emoji", "赞"],
            "remove_emoji_reaction",
            "openConversationId",
            "openMsgId",
            "emojiName",
        ),
        case(
            "+messages-remove-text-emotion",
            [
                "--conversation-id",
                g,
                "--msg-id",
                m,
                "--emotion-id",
                fx["emotion_id"],
                "--emotion-name",
                fx["emotion_name"],
                "--text",
                fx["emotion_text"],
                "--background-id",
                fx["emotion_background"],
            ],
            "remove_text_emotion",
            "openConversationId",
            "openMsgId",
            "emotionId",
            "emotionName",
            "text",
            "backgroundId",
        ),
        case(
            "+messages-reply",
            [
                "--conversation-id",
                g,
                "--ref-msg-id",
                m,
                "--ref-sender",
                u,
                "--text",
                text,
                "--uuid",
                fx["uuid"],
            ],
            "send_personal_message",
            "openConversationId",
            "msgType",
            "content",
            "uuid",
        ),
        case(
            "+messages-send",
            [
                "--identity",
                "user",
                "--group",
                g,
                "--text",
                text,
                "--uuid",
                fx["uuid"],
            ],
            "send_personal_message",
            "openConversationId",
            "msgType",
            "content",
            "uuid",
            semantic_preview=True,
        ),
        case(
            "+messages-send-by-bot",
            [
                "--robot-code",
                rc,
                "--group",
                g,
                "--title",
                "DWS dry-run",
                "--text",
                text,
                "--at-open-dingtalk-ids",
                u,
                "--at-all",
            ],
            "send_robot_group_message",
            "robotCode",
            "openConversationId",
            "title",
            "markdown",
            "atOpendingtalkIds",
            "isAtAll",
        ),
        case(
            "+messages-send-by-webhook",
            [
                "--token",
                fx["webhook_token"],
                "--title",
                "DWS dry-run",
                "--text",
                text,
                "--at-all",
                "--at-users",
                fx["user_id"],
            ],
            "send_message_by_custom_robot",
            "robotToken",
            "title",
            "text",
            "isAtAll",
            "atUserIds",
        ),
        case(
            "+messages-send-card",
            ["--group", g],
            "create_and_send_card",
            "openConversationId",
        ),
        case(
            "+messages-set-pin",
            ["--open-conversation-id", g, "--msg-id", m],
            "set_pin_message",
            "openConversationId",
            "cid",
            "openMessageId",
        ),
        case(
            "+messages-set-top",
            ["--open-conversation-id", g, "--msg-id", m],
            "set_top_message",
            "openConversationId",
            "openMessageId",
        ),
        case(
            "+messages-unset-pin",
            ["--open-conversation-id", g, "--msg-id", m],
            "unset_pin_message",
            "openConversationId",
            "cid",
            "openMessageId",
        ),
        case(
            "+messages-unset-top",
            ["--open-conversation-id", g, "--msg-id", m],
            "unset_top_message",
            "openConversationId",
            "openMessageId",
        ),
        case(
            "+messages-update-card",
            [
                "--biz-id",
                fx["biz_id"],
                "--content",
                text,
                "--flow-status",
                "3",
            ],
            "update_streaming_card",
            "bizId",
            "msgContent",
            "flowStatus",
        ),
        case(
            "+send-to-group",
            ["--group", fx["group_name"], "--text", text, "--ai-tag"],
            "send_personal_message",
            "openConversationId",
            "msgType",
            "content",
        ),
    ]


def summarize(case_item: DryRunCase, capture: Capture) -> dict[str, Any]:
    upper = capture.upper()
    raw_tools = [(server, tool) for server, tool, _ in capture.raw_calls]
    unexpected_calls = [
        f"{server}/{tool}"
        for server, tool in raw_tools
        if (server, tool) not in READ_TOOLS_ALLOWED_DURING_DRY_RUN
    ]
    argument_keys: list[str] = []
    missing: list[str] = []
    actual_tool = None

    if capture.timed_out:
        status = "timeout"
    elif capture.exit_code != 0:
        status = "error"
    elif unexpected_calls:
        status = "write_call_observed"
    elif not isinstance(upper, dict):
        status = "invalid_preview"
    elif case_item.semantic_preview:
        actual_tool = upper.get("tool")
        actions = upper.get("actions")
        if isinstance(actions, list):
            key_union: set[str] = set()
            for action in actions:
                if not isinstance(action, dict):
                    continue
                arguments = action.get("arguments")
                if isinstance(arguments, dict):
                    key_union.update(arguments)
            argument_keys = sorted(key_union)
        missing = sorted(case_item.required_argument_keys - set(argument_keys))
        status = (
            "pass"
            if upper.get("dry_run") is True
            and upper.get("executed") is False
            and upper.get("preview_kind") == "plan"
            and actual_tool == case_item.tool
            and int(upper.get("actionCount") or 0) > 0
            and int(upper.get("failedCount") or 0) == 0
            and not missing
            else "invalid_preview"
        )
    else:
        actual_tool = upper.get("tool")
        arguments = upper.get("arguments")
        if isinstance(arguments, dict):
            argument_keys = sorted(arguments)
        missing = sorted(case_item.required_argument_keys - set(argument_keys))
        if (
            upper.get("dry_run") is True
            and upper.get("executed") is False
            and actual_tool == case_item.tool
            and not missing
        ):
            status = "pass"
        else:
            status = "invalid_preview"

    return {
        "command": case_item.command,
        "status": status,
        "exit_code": capture.exit_code,
        "duration_ms": capture.duration_ms,
        "expected_tool": case_item.tool,
        "actual_tool": actual_tool,
        "argument_keys": argument_keys,
        "missing_argument_keys": missing,
        "read_calls": [f"{server}/{tool}" for server, tool in raw_tools],
        "unexpected_calls": unexpected_calls,
        "semantic_preview": case_item.semantic_preview,
    }


def run_audit(binary: Path, timeout: int) -> tuple[FixtureSet, list[dict[str, Any]]]:
    fixtures = discover_fixtures(binary, timeout)
    cases = build_cases(fixtures)
    if tuple(item.command for item in cases) != WRITE_COMMANDS:
        raise RuntimeError("write Shortcut dry-run coverage drifted from expected 63")
    results: list[dict[str, Any]] = []
    for item in cases:
        capture = run_capture(
            binary,
            item.command,
            [*item.args, "--dry-run", "--yes"],
            timeout,
        )
        results.append(summarize(item, capture))
    return fixtures, results


def markdown(fixtures: FixtureSet, results: list[dict[str, Any]]) -> str:
    counts: dict[str, int] = {}
    for result in results:
        counts[result["status"]] = counts.get(result["status"], 0) + 1
    real = sorted(name for name, source in fixtures.sources.items() if source == "real")
    synthetic = sorted(
        name for name, source in fixtures.sources.items() if source == "synthetic"
    )
    lines = [
        "# Chat Shortcut write dry-run audit",
        "",
        "All commands used `--dry-run --yes`. Real read calls were allowed only",
        "for semantic name resolution; no write MCP response was observed.",
        "",
        f"- Total: {len(results)}",
    ]
    for status in sorted(counts):
        lines.append(f"- {status}: {counts[status]}")
    lines.extend(
        [
            f"- Real fixture kinds: {', '.join(real)}",
            f"- Synthetic fixture kinds: {', '.join(synthetic)}",
            "",
            "| Shortcut | Status | Tool | Argument keys | Read-only resolution |",
            "|---|---|---|---|---|",
        ]
    )
    for result in results:
        lines.append(
            f"| `{result['command']}` | {result['status']} | "
            f"{result.get('actual_tool') or result.get('expected_tool') or 'semantic preview'} | "
            f"{', '.join(result.get('argument_keys', []))} | "
            f"{', '.join(result.get('read_calls', []))} |"
        )
    lines.append("")
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--dws", required=True, type=Path)
    parser.add_argument("--out-dir", required=True, type=Path)
    parser.add_argument("--timeout", type=int, default=30)
    args = parser.parse_args()

    binary = args.dws.resolve()
    if not binary.is_file():
        parser.error(f"--dws is not a file: {binary}")
    fixtures, results = run_audit(binary, args.timeout)

    args.out_dir.mkdir(parents=True, exist_ok=True)
    payload = {
        "scope": "chat-write-shortcuts-dry-run",
        "count": len(results),
        "fixture_sources": fixtures.sources,
        "results": results,
    }
    json_path = args.out_dir / "chat-write-dry-run-audit.json"
    markdown_path = args.out_dir / "chat-write-dry-run-audit.md"
    json_path.write_text(
        json.dumps(payload, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    markdown_path.write_text(markdown(fixtures, results), encoding="utf-8")
    print(markdown_path)
    return 1 if any(result["status"] != "pass" for result in results) else 0


if __name__ == "__main__":
    raise SystemExit(main())
