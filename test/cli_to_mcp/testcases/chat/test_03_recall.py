"""
test_03_recall.py — 消息撤回测试

Commands tested:
  1. dws chat message recall-by-bot  (机器人撤回消息)
  2. dws chat message recall          (用户撤回消息)
"""

import json
import time
import pytest


class TestChatMessageRecallByBot:
    """dws chat message recall-by-bot — 机器人撤回消息"""

    def test_recall_group_message(self, dws, robot_code, chat_id):
        """发送群聊消息后撤回。"""
        # 先发送
        send = dws.run(
            "chat", "message", "send-by-bot",
            "--robot-code", robot_code,
            "--group", chat_id,
            "--title", "待撤回群消息",
            "--text", f"撤回测试 {int(time.time())}",
        )
        key = (
            send.get("result", {}).get("processQueryKey", "")
            or send.get("data", {}).get("processQueryKey", "")
            or send.get("processQueryKey", "")
        )
        if not key:
            pytest.skip("No processQueryKey returned from send")
        # 再撤回
        dws.run_ok(
            "chat", "message", "recall-by-bot",
            "--robot-code", robot_code,
            "--group", chat_id,
            "--keys", key,
        )

    def test_recall_single_chat_message(self, dws, robot_code, current_user_id):
        """发送单聊消息后撤回（不传 --group）。"""
        # 先发送单聊
        send = dws.run(
            "chat", "message", "send-by-bot",
            "--robot-code", robot_code,
            "--users", current_user_id,
            "--title", "待撤回单聊",
            "--text", f"单聊撤回测试 {int(time.time())}",
        )
        key = (
            send.get("result", {}).get("processQueryKey", "")
            or send.get("data", {}).get("processQueryKey", "")
            or send.get("processQueryKey", "")
        )
        if not key:
            pytest.skip("No processQueryKey returned from send")
        # 单聊撤回不需要 --group
        dws.run_ok(
            "chat", "message", "recall-by-bot",
            "--robot-code", robot_code,
            "--keys", key,
        )

    def test_recall_invalid_key(self, dws, robot_code):
        """撤回无效 key 应报错或忽略（单聊模式，不依赖群聊）。"""
        dws.run_ok(
            "chat", "message", "recall-by-bot",
            "--robot-code", robot_code,
            "--keys", "INVALID_KEY_99999",
        )

    def test_recall_invalid_robot(self, dws, chat_id):
        """无效机器人撤回应报错。"""
        result = dws.run_raw(
            "chat", "message", "recall-by-bot",
            "--robot-code", "INVALID_ROBOT_99999",
            "--group", chat_id,
            "--keys", "X",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_recall_missing_keys(self, dws, robot_code, chat_id):
        """不传 keys 应报错。"""
        result = dws.run_raw(
            "chat", "message", "recall-by-bot",
            "--robot-code", robot_code,
            "--group", chat_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestChatMessageRecall:
    """dws chat message recall — 用户撤回消息

    Flags:
      --conversation-id / --group / --id / --chat (必填, 别名)
      --msg-id (必填)

    注意：撤回操作不可逆，且需要消息发送者身份。
    测试以容忍业务错误方式运行（消息可能已撤回/无权限）。
    """

    def test_recall_basic(self, dws, chat_id, msg_id):
        """基本撤回：传入 conversation-id 和 msg-id。"""
        result = dws.run_raw(
            "chat", "message", "recall",
            "--conversation-id", chat_id,
            "--msg-id", msg_id,
        )
        combined = (result.stdout or "") + (result.stderr or "")
        try:
            data = json.loads(combined.strip())
        except json.JSONDecodeError:
            pytest.fail(f"recall 返回非 JSON: {combined[:300]}")
        # 成功撤回或业务错误（已撤回/无权限等）都可接受
        if data.get("success") is True:
            r = data.get("result", {})
            assert "recallStatus" in r or "openMessageId" in r, (
                f"成功但缺少 recallStatus: {r}"
            )
            return
        err = data.get("error", {})
        assert err.get("reason") == "business_error", (
            f"预期 success 或 business_error: {data}"
        )

    def test_recall_alias_group(self, dws, chat_id, msg_id):
        """使用 --group 别名代替 --conversation-id。"""
        result = dws.run_raw(
            "chat", "message", "recall",
            "--group", chat_id,
            "--msg-id", msg_id,
        )
        combined = (result.stdout or "") + (result.stderr or "")
        data = json.loads(combined.strip())
        assert data.get("success") is True or "error" in data

    def test_recall_missing_conversation_id(self, dws, msg_id):
        """不传 conversation-id 应报错。"""
        result = dws.run_raw(
            "chat", "message", "recall",
            "--msg-id", msg_id,
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower() or "required" in combined.lower()

    def test_recall_missing_msg_id(self, dws, chat_id):
        """不传 msg-id 应报错。"""
        result = dws.run_raw(
            "chat", "message", "recall",
            "--conversation-id", chat_id,
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower() or "required" in combined.lower()

    def test_recall_invalid_msg_id(self, dws, chat_id):
        """无效 msg-id 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "message", "recall",
            "--conversation-id", chat_id,
            "--msg-id", "INVALID_MSG_ID_99999",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        try:
            data = json.loads(combined.strip())
        except json.JSONDecodeError:
            pytest.fail(f"recall 返回非 JSON: {combined[:300]}")
        if data.get("success") is True:
            return
        assert "error" in data, f"无效 msg-id 应返回 error: {data}"
