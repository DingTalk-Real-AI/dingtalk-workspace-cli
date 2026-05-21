"""
test_15_group_mute.py — 群全员禁言测试

Commands tested:
  1. dws chat group-mute  (set_group_mute — 群全员禁言/取消禁言)

Fixtures:
  - session_group_id (conftest, session)
"""

import pytest


class TestChatGroupMute:
    """dws chat group-mute"""

    def test_group_mute_on(self, dws, session_group_id):
        """开启群全员禁言 — 正常路径。"""
        data = dws.run(
            "chat", "group-mute",
            "--group", session_group_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_group_mute_off(self, dws, session_group_id):
        """取消群全员禁言 — 使用 --off 标志。"""
        data = dws.run(
            "chat", "group-mute",
            "--group", session_group_id,
            "--off",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_group_mute_missing_group(self, dws):
        """不传 --group 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "group-mute",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_group_mute_invalid_group(self, dws):
        """无效 group 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "group-mute",
            "--group", "INVALID_CONV_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_group_mute_toggle(self, dws, session_group_id):
        """开启再关闭 — 验证可逆性。"""
        on_data = dws.run(
            "chat", "group-mute",
            "--group", session_group_id,
        )
        assert on_data.get("success") is True, f"开启禁言应成功: {on_data}"
        off_data = dws.run(
            "chat", "group-mute",
            "--group", session_group_id,
            "--off",
        )
        assert off_data.get("success") is True, f"取消禁言应成功: {off_data}"
