"""
test_17_group_set_admin.py — 群管理员设置测试

Commands tested:
  1. dws chat group set-admin  (update_conv_member_roles — 设置/取消群管理员)

Fixtures:
  - multi_user_group_id          (conftest, session) — 包含同组织其他用户的双人群
  - chat_other_open_dingtalk_id  (conftest, session) — --users 参数必须是 openDingTalkId
"""

import pytest


class TestChatGroupSetAdmin:
    """dws chat group set-admin"""

    def test_set_admin_on(self, dws, multi_user_group_id, chat_other_open_dingtalk_id):
        """设置群管理员 — 正常路径。"""
        data = dws.run(
            "chat", "group", "set-admin",
            "--group", multi_user_group_id,
            "--users", chat_other_open_dingtalk_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_set_admin_off(self, dws, multi_user_group_id, chat_other_open_dingtalk_id):
        """取消群管理员 — 使用 --off 标志。"""
        data = dws.run(
            "chat", "group", "set-admin",
            "--group", multi_user_group_id,
            "--users", chat_other_open_dingtalk_id,
            "--off",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_set_admin_missing_group(self, dws, chat_other_open_dingtalk_id):
        """不传 --group 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "group", "set-admin",
            "--users", chat_other_open_dingtalk_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_set_admin_missing_users(self, dws, multi_user_group_id):
        """不传 --users 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "group", "set-admin",
            "--group", multi_user_group_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_set_admin_invalid_group(self, dws, chat_other_open_dingtalk_id):
        """无效 group 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "group", "set-admin",
            "--group", "INVALID_CONV_99999",
            "--users", chat_other_open_dingtalk_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_set_admin_toggle(self, dws, multi_user_group_id, chat_other_open_dingtalk_id):
        """设置再取消 — 验证可逆性。"""
        on_data = dws.run(
            "chat", "group", "set-admin",
            "--group", multi_user_group_id,
            "--users", chat_other_open_dingtalk_id,
        )
        assert on_data.get("success") is True, f"设置管理员应成功: {on_data}"
        off_data = dws.run(
            "chat", "group", "set-admin",
            "--group", multi_user_group_id,
            "--users", chat_other_open_dingtalk_id,
            "--off",
        )
        assert off_data.get("success") is True, f"取消管理员应成功: {off_data}"
