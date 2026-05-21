"""
test_12_reply.py — 引用回复消息测试（单聊/群聊均可）

Commands tested:
  1. dws chat message reply  (send_personal_message — 引用回复消息)

说明：开放平台对单聊与群聊使用统一的 openConversationId，因此 --conversation-id
      参数同时适用于两种场景。

Fixtures:
  - session_group_id  (conftest, session)
  - session_msg_id    (conftest, session)
  - current_open_dingtalk_id  (conftest, session) — --ref-sender 必须是 openDingTalkId
"""

import pytest


class TestChatMessageReply:
    """dws chat message reply"""

    def test_reply_basic(self, dws, session_group_id, session_msg_id, current_open_dingtalk_id):
        """引用回复 — 正常路径。"""
        data = dws.run(
            "chat", "message", "reply",
            "--conversation-id", session_group_id,
            "--ref-msg-id", session_msg_id,
            "--ref-sender", current_open_dingtalk_id,
            "--text", "自动化测试回复消息",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_reply_missing_conversation_id(self, dws, session_msg_id, current_open_dingtalk_id):
        """不传 --conversation-id 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "message", "reply",
            "--ref-msg-id", session_msg_id,
            "--ref-sender", current_open_dingtalk_id,
            "--text", "X",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_reply_missing_text(self, dws, session_group_id, session_msg_id, current_open_dingtalk_id):
        """不传 --text 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "message", "reply",
            "--conversation-id", session_group_id,
            "--ref-msg-id", session_msg_id,
            "--ref-sender", current_open_dingtalk_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_reply_invalid_msg_id(self, dws, session_group_id, current_open_dingtalk_id):
        """无效 ref-msg-id 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "message", "reply",
            "--conversation-id", session_group_id,
            "--ref-msg-id", "INVALID_MSG_99999",
            "--ref-sender", current_open_dingtalk_id,
            "--text", "X",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_reply_invalid_conversation_id(self, dws, current_open_dingtalk_id):
        """无效 conversation-id 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "message", "reply",
            "--conversation-id", "INVALID_CONV_99999",
            "--ref-msg-id", "FAKE_MSG_ID",
            "--ref-sender", current_open_dingtalk_id,
            "--text", "X",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
