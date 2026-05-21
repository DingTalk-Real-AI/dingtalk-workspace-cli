"""
test_21_group_dismiss.py — 解散群聊测试

Commands tested:
  1. dws chat group dismiss  (dismiss_group — 解散群聊，需要群主权限，操作不可逆)

Fixtures:
  - transient_group_id (conftest, function) — 一次性独立群，避免污染共享群
"""

import pytest

from conftest import _parse_json, skip_if_backend_tool_missing


class TestChatGroupDismiss:
    """dws chat group dismiss"""

    def test_dismiss_basic(self, dws, transient_group_id):
        """解散一次性群 — 正常路径（破坏性，使用 transient_group_id）。"""
        proc = dws.run_raw(
            "chat", "group", "dismiss",
            "--group", transient_group_id,
        )
        data = _parse_json(proc)
        skip_if_backend_tool_missing(data)
        assert data is not None and data.get("success") is True, f"success 应为 True: {data}"

    def test_dismiss_missing_group(self, dws):
        """不传 --group 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "group", "dismiss",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_dismiss_invalid_group(self, dws):
        """无效 group 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "group", "dismiss",
            "--group", "INVALID_CONV_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
