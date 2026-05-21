"""
test_16_group_mute_member.py — 群成员禁言测试

Commands tested:
  1. dws chat group-mute-member  (set_group_member_mute_list — 指定成员禁言/取消禁言)

Fixtures:
  - multi_user_group_id          (conftest, session) — 包含同组织其他用户的双人群
  - chat_other_open_dingtalk_id  (conftest, session) — --users 参数必须是 openDingTalkId

注意:
  --users 参数实际是 openDingTalkId 列表（不是 userId）。
  --mute-time 必须为毫秒枚举值: 300000(5min) / 3600000(1h) / 86400000(1d)
                                / 604800000(7d) / 2592000000(30d)
"""

import pytest


class TestChatGroupMuteMember:
    """dws chat group-mute-member"""

    def test_mute_member_basic(self, dws, multi_user_group_id, chat_other_open_dingtalk_id):
        """禁言指定成员（1小时）— 正常路径。

        注：mute 操作 --mute-time 是必填，无默认值，
        合法值: 300000/3600000/86400000/604800000/2592000000。
        """
        data = dws.run(
            "chat", "group-mute-member",
            "--group", multi_user_group_id,
            "--users", chat_other_open_dingtalk_id,
            "--mute-time", "3600000",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_mute_member_with_time(self, dws, multi_user_group_id, chat_other_open_dingtalk_id):
        """禁言指定成员 5 分钟（300000 毫秒）。"""
        data = dws.run(
            "chat", "group-mute-member",
            "--group", multi_user_group_id,
            "--users", chat_other_open_dingtalk_id,
            "--mute-time", "300000",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_mute_member_off(self, dws, multi_user_group_id, chat_other_open_dingtalk_id):
        """取消指定成员禁言 — 使用 --off 标志。"""
        data = dws.run(
            "chat", "group-mute-member",
            "--group", multi_user_group_id,
            "--users", chat_other_open_dingtalk_id,
            "--off",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_mute_member_missing_group(self, dws, chat_other_open_dingtalk_id):
        """不传 --group 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "group-mute-member",
            "--users", chat_other_open_dingtalk_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_mute_member_missing_users(self, dws, multi_user_group_id):
        """不传 --users 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "group-mute-member",
            "--group", multi_user_group_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_mute_member_invalid_mute_time(self, dws, multi_user_group_id, chat_other_open_dingtalk_id):
        """无效 mute-time（非枚举值）应返回业务错误。"""
        result = dws.run_raw(
            "chat", "group-mute-member",
            "--group", multi_user_group_id,
            "--users", chat_other_open_dingtalk_id,
            "--mute-time", "60",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
            or "MuteTimeEnum" in result.stdout
            or "MuteTimeEnum" in (result.stderr or "")
        )

    def test_mute_member_toggle(self, dws, multi_user_group_id, chat_other_open_dingtalk_id):
        """禁言再取消 — 验证可逆性。"""
        on_data = dws.run(
            "chat", "group-mute-member",
            "--group", multi_user_group_id,
            "--users", chat_other_open_dingtalk_id,
            "--mute-time", "300000",
        )
        assert on_data.get("success") is True, f"禁言应成功: {on_data}"
        off_data = dws.run(
            "chat", "group-mute-member",
            "--group", multi_user_group_id,
            "--users", chat_other_open_dingtalk_id,
            "--off",
        )
        assert off_data.get("success") is True, f"取消禁言应成功: {off_data}"
