"""
test_10_list_by_ids.py — 批量查询消息测试

Commands tested:
  1. dws chat message list-by-ids

Flags:
  --msg-ids (必填, CSV, max 50)
"""

import json
import pytest


class TestChatMessageListByIds:
    """dws chat message list-by-ids — 批量按 ID 查询消息"""

    def test_single_msg_id(self, dws, msg_id):
        """单条消息 ID 查询，应返回 messages 数组。"""
        data = dws.run(
            "chat", "message", "list-by-ids",
            "--msg-ids", msg_id,
        )
        result = data.get("result", {})
        assert "messages" in result, f"缺少 messages 字段: {result}"
        msgs = result["messages"]
        assert isinstance(msgs, list), f"messages 应为 list: {type(msgs)}"

    def test_single_msg_id_fields(self, dws, msg_id):
        """返回的消息应包含关键字段。"""
        data = dws.run(
            "chat", "message", "list-by-ids",
            "--msg-ids", msg_id,
        )
        msgs = data.get("result", {}).get("messages", [])
        if not msgs:
            pytest.skip("未返回消息，可能消息已被删除")
        msg = msgs[0]
        for key in ("openMessageId", "openConversationId"):
            assert key in msg, f"消息缺少字段 {key}: {msg}"

    def test_multiple_msg_ids(self, dws, msg_id):
        """多条消息 ID 查询（用同一个 ID 模拟逗号分隔）。"""
        data = dws.run(
            "chat", "message", "list-by-ids",
            "--msg-ids", f"{msg_id},{msg_id}",
        )
        result = data.get("result", {})
        assert "messages" in result

    def test_invalid_msg_id(self, dws):
        """无效消息 ID 应返回空列表或忽略无效项。"""
        result = dws.run_raw(
            "chat", "message", "list-by-ids",
            "--msg-ids", "INVALID_MSG_ID_99999",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        try:
            data = json.loads(combined.strip())
        except json.JSONDecodeError:
            pytest.fail(f"list-by-ids 返回非 JSON: {combined[:300]}")
        if data.get("success") is True:
            msgs = data.get("result", {}).get("messages", [])
            assert isinstance(msgs, list), f"messages 应为 list: {msgs}"
        else:
            assert "error" in data

    def test_missing_msg_ids(self, dws):
        """不传 --msg-ids 应报错。"""
        result = dws.run_raw(
            "chat", "message", "list-by-ids",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower() or "required" in combined.lower(), (
            f"缺少必填参数应报错: {combined[:300]}"
        )
