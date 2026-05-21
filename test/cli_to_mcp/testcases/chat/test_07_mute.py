"""
test_07_mute.py — 会话消息免打扰测试

Commands tested:
  1. dws chat mute  (开启/关闭会话消息免打扰)
"""

import pytest


class TestChatMute:
    """dws chat mute"""

    def test_mute_enable(self, dws, chat_id):
        """开启群聊免打扰。"""
        data = dws.run(
            "chat", "mute",
            "--conversation-id", chat_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_mute_disable(self, dws, chat_id):
        """关闭群聊免打扰（--off）。"""
        data = dws.run(
            "chat", "mute",
            "--conversation-id", chat_id,
            "--off",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_mute_with_id_alias(self, dws, chat_id):
        """使用 --id 别名替代 --conversation-id。"""
        data = dws.run(
            "chat", "mute",
            "--id", chat_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_mute_missing_conversation_id(self, dws):
        """不传 conversation-id 应报错（必填）。"""
        result = dws.run_raw("chat", "mute")
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_mute_invalid_conversation_id(self, dws):
        """无效 conversation-id 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "mute",
            "--conversation-id", "INVALID_CONV_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
