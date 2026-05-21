"""
test_22_group_set_history.py — 设置新成员可查看历史消息选项测试

Commands tested:
  1. dws chat group set-history  (update_show_history_msg_option — 控制新成员入群后可见历史消息范围)

Setup 流程（按 5.14/5.18 评测要求）：
  - searched_chat_msg_id: 搜索一个名字含"测试"的群，并在群里现场发一条消息，
    返回 {"group_id": ..., "msg_id": ...}。set-history 仅依赖 group_id，
    msg 发送只是评测要求的"测试群有活跃消息"前置仪式。
"""

import pytest

from conftest import _parse_json, skip_if_backend_tool_missing


class TestChatGroupSetHistory:
    """dws chat group set-history"""

    @pytest.mark.parametrize("option", ["FORBIDDEN", "RECENT_100"])
    def test_set_history_valid_option(self, dws, searched_chat_msg_id, option):
        """FORBIDDEN / RECENT_100 — 普通账号可设置，应成功。

        ALL 选项需要额外权限，由 test_set_history_option_all_permission_denied
        单独覆盖（预期 AUTH_PERMISSION_DENIED）。
        """
        gid = searched_chat_msg_id["group_id"]
        proc = dws.run_raw(
            "chat", "group", "set-history",
            "--group", gid,
            "--option", option,
        )
        data = _parse_json(proc)
        skip_if_backend_tool_missing(data)
        assert data is not None and data.get("success") is True, f"option={option} 应成功: {data}"

    def test_set_history_option_all_permission_denied(self, dws, searched_chat_msg_id):
        """option=ALL 是当前账号无权限的合法选项，应明确返回 AUTH_PERMISSION_DENIED。

        断言契约：
          - success != true
          - error.message 包含 AUTH_PERMISSION_DENIED / Permission denied / 权限
          - error.server_key = "im"

        若未来后端为该账号开放了 ALL 权限或调整了错误码，此用例会显式失败提醒更新断言。
        """
        gid = searched_chat_msg_id["group_id"]
        proc = dws.run_raw(
            "chat", "group", "set-history",
            "--group", gid,
            "--option", "ALL",
        )
        data = _parse_json(proc)
        assert data is not None, (
            f"应返回 JSON: stdout={proc.stdout[:200]} stderr={proc.stderr[:200]}"
        )
        err = data.get("error") or {}
        msg = str(err.get("message") or "")
        assert data.get("success") is not True, (
            f"--option ALL 当前账号应被拒绝，但返回 success=true: {data}"
        )
        assert (
            "AUTH_PERMISSION_DENIED" in msg
            or "Permission denied" in msg
            or "权限" in msg
        ), (
            f"--option ALL 应返回权限不足类错误（AUTH_PERMISSION_DENIED / Permission denied / 权限），实际: {data}"
        )
        assert err.get("server_key") == "im", (
            f"server_key 应为 'im'，实际: {data}"
        )

    def test_set_history_missing_group(self, dws):
        """不传 --group 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "group", "set-history",
            "--option", "ALL",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_set_history_missing_option(self, dws, searched_chat_msg_id):
        """不传 --option 应报错（必填）。"""
        gid = searched_chat_msg_id["group_id"]
        result = dws.run_raw(
            "chat", "group", "set-history",
            "--group", gid,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_set_history_invalid_option(self, dws, searched_chat_msg_id):
        """非法 option 取值应被本地校验拦截。"""
        gid = searched_chat_msg_id["group_id"]
        result = dws.run_raw(
            "chat", "group", "set-history",
            "--group", gid,
            "--option", "BOGUS",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_set_history_invalid_group(self, dws):
        """无效 group 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "group", "set-history",
            "--group", "INVALID_CONV_99999",
            "--option", "ALL",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
