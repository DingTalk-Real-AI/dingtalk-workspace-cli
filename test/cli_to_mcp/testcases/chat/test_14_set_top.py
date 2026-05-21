"""
test_14_set_top.py — 会话置顶测试

Commands tested:
  1. dws chat set-top  (set_top_conversation — 会话置顶/取消置顶)

Fixtures:
  - session_group_id (conftest, session)
"""

import pytest


class TestChatSetTop:
    """dws chat set-top"""

    def test_set_top_on(self, dws, session_group_id):
        """置顶会话 — 正常路径。"""
        data = dws.run(
            "chat", "set-top",
            "--conversation-id", session_group_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_set_top_off(self, dws, session_group_id):
        """取消置顶 — 使用 --off 标志。"""
        data = dws.run(
            "chat", "set-top",
            "--conversation-id", session_group_id,
            "--off",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_set_top_missing_conversation_id(self, dws):
        """不传 --conversation-id 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "set-top",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_set_top_invalid_conversation_id(self, dws):
        """无效 conversation-id 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "set-top",
            "--conversation-id", "INVALID_CONV_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_set_top_idempotent(self, dws, session_group_id):
        """重复置顶应幂等成功。"""
        data = dws.run(
            "chat", "set-top",
            "--conversation-id", session_group_id,
        )
        assert data.get("success") is True, f"第一次置顶应成功: {data}"
        data2 = dws.run(
            "chat", "set-top",
            "--conversation-id", session_group_id,
        )
        assert data2.get("success") is True, f"重复置顶应幂等成功: {data2}"
        # 清理：取消置顶
        dws.run_raw(
            "chat", "set-top",
            "--conversation-id", session_group_id,
            "--off",
        )
