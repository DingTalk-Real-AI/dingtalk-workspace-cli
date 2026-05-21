"""
test_13_forward.py — 转发单条消息测试（源/目标会话均支持单聊/群聊）

Commands tested:
  1. dws chat message forward  (forward_message — 转发消息到任意会话)

说明：开放平台对单聊与群聊使用统一的 openConversationId，因此源/目标会话
      参数同时适用于两种场景，组合包括 群→群、群→单、单→群、单→单。

Fixtures:
  - session_group_id    (conftest, session) — 源会话（群）
  - multi_user_group_id (conftest, session) — 目标会话（群）
  - session_msg_id      (conftest, session) — 待转发消息
"""

import pytest


class TestChatMessageForward:
    """dws chat message forward"""

    def test_forward_basic(self, dws, session_group_id, session_msg_id, multi_user_group_id):
        """转发消息 — 正常路径。"""
        data = dws.run(
            "chat", "message", "forward",
            "--src-conversation-id", session_group_id,
            "--msg-id", session_msg_id,
            "--dest-conversation-id", multi_user_group_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_forward_missing_src_conversation_id(self, dws, session_msg_id, multi_user_group_id):
        """不传 --src-conversation-id 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "message", "forward",
            "--msg-id", session_msg_id,
            "--dest-conversation-id", multi_user_group_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_forward_missing_msg_id(self, dws, session_group_id, multi_user_group_id):
        """不传 --msg-id 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "message", "forward",
            "--src-conversation-id", session_group_id,
            "--dest-conversation-id", multi_user_group_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_forward_missing_dest_conversation_id(self, dws, session_group_id, session_msg_id):
        """不传 --dest-conversation-id 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "message", "forward",
            "--src-conversation-id", session_group_id,
            "--msg-id", session_msg_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_forward_invalid_msg_id(self, dws, session_group_id, multi_user_group_id):
        """无效 msg-id 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "message", "forward",
            "--src-conversation-id", session_group_id,
            "--msg-id", "INVALID_MSG_99999",
            "--dest-conversation-id", multi_user_group_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
