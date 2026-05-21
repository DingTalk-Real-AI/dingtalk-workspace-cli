"""
test_20_combine_forward.py — 合并转发消息测试

Commands tested:
  1. dws chat message combine-forward  (combine_forward_messages — 合并多条消息转发到目标会话)

Setup 流程（按 5.14/5.18 评测要求）：
  - searched_chat_msg_id 提供搜索到的"测试群" + 群内现场发的一条消息
  - 源/目标会话默认都是 searched group（自我合并转发，不污染其他群）
"""

import pytest

from conftest import _parse_json, skip_if_backend_tool_missing


class TestChatMessageCombineForward:
    """dws chat message combine-forward"""

    def test_combine_forward_basic(self, dws, searched_chat_msg_id):
        """合并转发单条消息（最小输入）— 正常路径。"""
        gid = searched_chat_msg_id["group_id"]
        mid = searched_chat_msg_id["msg_id"]
        proc = dws.run_raw(
            "chat", "message", "combine-forward",
            "--src-conversation-id", gid,
            "--msg-ids", mid,
            "--dest-conversation-id", gid,
        )
        data = _parse_json(proc)
        skip_if_backend_tool_missing(data)
        assert data is not None and data.get("success") is True, f"success 应为 True: {data}"

    def test_combine_forward_with_uuid(self, dws, searched_chat_msg_id):
        """合并转发并指定幂等键 — 正常路径。"""
        import time
        gid = searched_chat_msg_id["group_id"]
        mid = searched_chat_msg_id["msg_id"]
        proc = dws.run_raw(
            "chat", "message", "combine-forward",
            "--src-conversation-id", gid,
            "--msg-ids", mid,
            "--dest-conversation-id", gid,
            "--uuid", f"ci_combine_{int(time.time())}",
        )
        data = _parse_json(proc)
        skip_if_backend_tool_missing(data)
        assert data is not None and data.get("success") is True, f"success 应为 True: {data}"

    def test_combine_forward_missing_src_conversation_id(self, dws, searched_chat_msg_id):
        """不传 --src-conversation-id 应报错（必填）。"""
        gid = searched_chat_msg_id["group_id"]
        mid = searched_chat_msg_id["msg_id"]
        result = dws.run_raw(
            "chat", "message", "combine-forward",
            "--msg-ids", mid,
            "--dest-conversation-id", gid,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_combine_forward_missing_msg_ids(self, dws, searched_chat_msg_id):
        """不传 --msg-ids 应报错（必填）。"""
        gid = searched_chat_msg_id["group_id"]
        result = dws.run_raw(
            "chat", "message", "combine-forward",
            "--src-conversation-id", gid,
            "--dest-conversation-id", gid,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_combine_forward_missing_dest_conversation_id(self, dws, searched_chat_msg_id):
        """不传 --dest-conversation-id 应报错（必填）。"""
        gid = searched_chat_msg_id["group_id"]
        mid = searched_chat_msg_id["msg_id"]
        result = dws.run_raw(
            "chat", "message", "combine-forward",
            "--src-conversation-id", gid,
            "--msg-ids", mid,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_combine_forward_invalid_msg_id(self, dws, searched_chat_msg_id):
        """无效 msg-id 应返回业务错误（或 success=false）。"""
        gid = searched_chat_msg_id["group_id"]
        result = dws.run_raw(
            "chat", "message", "combine-forward",
            "--src-conversation-id", gid,
            "--msg-ids", "INVALID_MSG_99999",
            "--dest-conversation-id", gid,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
