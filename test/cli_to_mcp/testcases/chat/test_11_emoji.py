"""
test_11_emoji.py — chat message add-emoji / remove-emoji 测试用例

覆盖命令:
  - chat message add-emoji    (为消息添加 emoji 表情回应)
  - chat message remove-emoji (移除消息 emoji 表情回应)

必填参数: --conversation-id (别名 --group/--id/--chat), --msg-id, --emoji
两个命令均幂等，成功返回 {"success": true, "result": {}}
"""

import json
import pytest


class TestChatMessageAddEmoji:
    """chat message add-emoji 测试"""

    def test_add_emoji_basic(self, dws, chat_id, msg_id):
        """基本添加 emoji 回应"""
        data = dws.run(
            "chat", "message", "add-emoji",
            "--conversation-id", chat_id,
            "--msg-id", msg_id,
            "--emoji", "1",  # 竖起大拇指
        )
        assert data.get("success") is True

    def test_add_emoji_alias_group(self, dws, chat_id, msg_id):
        """使用 --group 别名"""
        data = dws.run(
            "chat", "message", "add-emoji",
            "--group", chat_id,
            "--msg-id", msg_id,
            "--emoji", "1",
        )
        assert data.get("success") is True

    def test_add_emoji_alias_id(self, dws, chat_id, msg_id):
        """使用 --id 别名"""
        data = dws.run(
            "chat", "message", "add-emoji",
            "--id", chat_id,
            "--msg-id", msg_id,
            "--emoji", "1",
        )
        assert data.get("success") is True

    def test_add_emoji_missing_conversation_id(self, dws, msg_id):
        """缺少 --conversation-id 应失败"""
        result = dws.run_raw(
            "chat", "message", "add-emoji",
            "--msg-id", msg_id,
            "--emoji", "1",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "required" in combined.lower() or "error" in combined.lower()

    def test_add_emoji_missing_msg_id(self, dws, chat_id):
        """缺少 --msg-id 应失败"""
        result = dws.run_raw(
            "chat", "message", "add-emoji",
            "--conversation-id", chat_id,
            "--emoji", "1",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "required" in combined.lower() or "error" in combined.lower()

    def test_add_emoji_missing_emoji(self, dws, chat_id, msg_id):
        """缺少 --emoji 应失败"""
        result = dws.run_raw(
            "chat", "message", "add-emoji",
            "--conversation-id", chat_id,
            "--msg-id", msg_id,
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "required" in combined.lower() or "error" in combined.lower()


class TestChatMessageRemoveEmoji:
    """chat message remove-emoji 测试"""

    def test_remove_emoji_basic(self, dws, chat_id, msg_id):
        """基本移除 emoji 回应（先添加再移除）"""
        # 先确保存在
        dws.run(
            "chat", "message", "add-emoji",
            "--conversation-id", chat_id,
            "--msg-id", msg_id,
            "--emoji", "100",  # 满分
        )
        # 再移除
        data = dws.run(
            "chat", "message", "remove-emoji",
            "--conversation-id", chat_id,
            "--msg-id", msg_id,
            "--emoji", "100",
        )
        assert data.get("success") is True

    def test_remove_emoji_alias_chat(self, dws, chat_id, msg_id):
        """使用 --chat 别名"""
        data = dws.run(
            "chat", "message", "remove-emoji",
            "--chat", chat_id,
            "--msg-id", msg_id,
            "--emoji", "1",
        )
        assert data.get("success") is True

    def test_remove_emoji_idempotent(self, dws, chat_id, msg_id):
        """重复移除应该幂等成功"""
        for _ in range(2):
            data = dws.run(
                "chat", "message", "remove-emoji",
                "--conversation-id", chat_id,
                "--msg-id", msg_id,
                "--emoji", "999",
            )
            assert data.get("success") is True

    def test_remove_emoji_missing_conversation_id(self, dws, msg_id):
        """缺少 --conversation-id 应失败"""
        result = dws.run_raw(
            "chat", "message", "remove-emoji",
            "--msg-id", msg_id,
            "--emoji", "1",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "required" in combined.lower() or "error" in combined.lower()

    def test_remove_emoji_missing_msg_id(self, dws, chat_id):
        """缺少 --msg-id 应失败"""
        result = dws.run_raw(
            "chat", "message", "remove-emoji",
            "--conversation-id", chat_id,
            "--emoji", "1",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "required" in combined.lower() or "error" in combined.lower()

    def test_remove_emoji_missing_emoji(self, dws, chat_id, msg_id):
        """缺少 --emoji 应失败"""
        result = dws.run_raw(
            "chat", "message", "remove-emoji",
            "--conversation-id", chat_id,
            "--msg-id", msg_id,
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "required" in combined.lower() or "error" in combined.lower()
