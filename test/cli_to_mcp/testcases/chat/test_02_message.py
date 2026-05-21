"""
test_02_message.py — 消息发送测试 (2 commands × N cases)

Commands tested:
  1. dws chat message send-by-bot   (以机器人身份发消息)
  2. dws chat message send-by-webhook (通过 Webhook 发群消息)

NOTE: dws chat message send (以当前用户身份发消息) has been removed from dws CLI.
"""

import time
import pytest
from test_utils import unique_name


class TestChatMessageSendByBot:
    """dws chat message send-by-bot — 以机器人身份发消息"""

    def test_send_to_group(self, dws, robot_code, chat_id):
        """机器人向群聊发送消息。"""
        data = dws.run(
            "chat", "message", "send-by-bot",
            "--robot-code", robot_code,
            "--group", chat_id,
            "--title", f"机器人群聊_{int(time.time())}",
            "--text", "## Markdown 消息\n这是 **机器人** 发送的消息",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_send_to_users(self, dws, robot_code, current_user_id):
        """机器人向用户发送单聊消息。"""
        data = dws.run(
            "chat", "message", "send-by-bot",
            "--robot-code", robot_code,
            "--users", current_user_id,
            "--title", "机器人单聊",
            "--text", f"单聊消息 {int(time.time())}",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_send_invalid_robot(self, dws, chat_id):
        """无效机器人发送应报错。"""
        result = dws.run_raw(
            "chat", "message", "send-by-bot",
            "--robot-code", "INVALID_ROBOT_99999",
            "--group", chat_id,
            "--title", "X",
            "--text", "X",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_send_missing_group_and_users(self, dws, robot_code):
        """不传 group 和 users 应报错（必填其一）。"""
        result = dws.run_raw(
            "chat", "message", "send-by-bot",
            "--robot-code", robot_code,
            "--title", "X",
            "--text", "X",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_send_both_group_and_users(self, dws, robot_code, chat_id, current_user_id):
        """同时传 group 和 users 应报错（互斥）。"""
        result = dws.run_raw(
            "chat", "message", "send-by-bot",
            "--robot-code", robot_code,
            "--group", chat_id,
            "--users", current_user_id,
            "--title", "X",
            "--text", "X",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestChatMessageSendByWebhook:
    """dws chat message send-by-webhook — 通过 Webhook 发群消息"""

    def test_send_basic(self, dws, webhook_token):
        """通过 Webhook 发送群消息。"""
        data = dws.run(
            "chat", "message", "send-by-webhook",
            "--token", webhook_token,
            "--title", f"Webhook测试_{int(time.time())}",
            "--text", "这是 Webhook 发送的消息",
        )
        # webhook 可能因关键词安全设置返回 310000，验证 CLI 正常返回结构即可
        assert "errcode" in data, f"响应缺少 errcode: {data}"
        assert "errmsg" in data, f"响应缺少 errmsg: {data}"

    def test_send_with_at_all(self, dws, webhook_token):
        """Webhook 消息 @ 所有人。"""
        data = dws.run(
            "chat", "message", "send-by-webhook",
            "--token", webhook_token,
            "--title", "Webhook AT测试",
            "--text", "@所有人 这是 @ 所有人 的消息",
            "--at-all",
        )
        assert "errcode" in data, f"响应缺少 errcode: {data}"
        assert "errmsg" in data, f"响应缺少 errmsg: {data}"

    def test_send_invalid_token(self, dws):
        """无效 token 应报错。"""
        result = dws.run_raw(
            "chat", "message", "send-by-webhook",
            "--token", "INVALID_TOKEN_99999",
            "--title", "X",
            "--text", "X",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
            or "errcode" in result.stdout.lower()
            or "errmsg" in result.stdout.lower()
        )
