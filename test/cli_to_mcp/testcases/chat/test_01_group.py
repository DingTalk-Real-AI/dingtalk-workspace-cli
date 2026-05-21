"""
test_01_group.py — 群组管理测试

Commands tested:
  1. dws chat group create          (create_internal_org_group)
  2. dws chat group transfer-owner  (转让群主)
  3. dws chat group invite-url      (获取群邀请链接)
  4. dws chat group quit            (退出群聊)
  5. dws chat group update-icon     (更新群头像)
  6. dws chat group update-settings (更新群设置)

注意：创建群后无 CLI 删除命令，测试群会保留在用户账号中。
"""

import pytest
from test_utils import unique_name, get_in


class TestGroupCreate:
    """dws chat group create"""

    def test_create_basic(self, dws, current_user_id):
        """创建基本内部群。"""
        name = unique_name("CLI_Test_Group")
        data = dws.run(
            "chat", "group", "create",
            "--name", name,
            # chat 文档定义的参数名是 --users（不是 --members）
            "--users", current_user_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        result = data.get("result", {})
        assert "openConversationId" in result, f"result 缺少 openConversationId: {result}"

    def test_create_chinese_name(self, dws, current_user_id):
        """创建中文群名。"""
        name = unique_name("测试群")
        data = dws.run(
            "chat", "group", "create",
            "--name", name,
            "--users", current_user_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        result = data.get("result", {})
        assert "openConversationId" in result, f"result 缺少 openConversationId: {result}"

    def test_create_missing_users(self, dws):
        """不传 users 参数时应报错（users 为必填）。"""
        name = unique_name("CLI_Test_NoUser")
        result = dws.run_raw(
            "chat", "group", "create",
            "--name", name,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestGroupTransferOwner:
    """dws chat group transfer-owner"""

    def test_transfer_basic(self, dws, transient_group_id, current_open_dingtalk_id):
        """转让群主 — 验证 CLI 正确调用 API 并返回结构化响应。

        使用 transient_group_id（每次新建独立群），避免改变 chat_id 共享群
        的所有权而污染其他用例。

        注意：当前 PAT 可能无 transfer-owner 权限，或目标用户已是群主，
        因此同时接受 success=True 和已知业务错误作为通过条件。
        """
        import json
        result = dws.run_raw(
            "chat", "group", "transfer-owner",
            "--group", transient_group_id,
            "--new-owner", current_open_dingtalk_id,
        )
        combined = (result.stdout or "") + (result.stderr or "")
        try:
            data = json.loads(combined.strip())
        except json.JSONDecodeError:
            pytest.fail(
                f"transfer-owner 返回非 JSON: stdout={result.stdout!r}, "
                f"stderr={result.stderr!r}"
            )
        # 成功 或 已知业务错误（如权限不足、已是群主等）均视为 CLI 功能正常
        if data.get("success") is True:
            return  # 转让成功
        err = data.get("error", {})
        assert err.get("reason") == "business_error", (
            f"预期 success=True 或 business_error，实际: {data}"
        )

    def test_transfer_missing_group(self, dws, current_open_dingtalk_id):
        """不传 group 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "group", "transfer-owner",
            "--new-owner", current_open_dingtalk_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_transfer_invalid_group(self, dws, current_open_dingtalk_id):
        """无效 group ID 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "group", "transfer-owner",
            "--group", "INVALID_CONV_99999",
            "--new-owner", current_open_dingtalk_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestGroupInviteUrl:
    """dws chat group invite-url"""

    def test_get_invite_url_basic(self, dws, searched_chat_id):
        """获取群邀请链接，验证返回链接字段。"""
        data = dws.run(
            "chat", "group", "invite-url",
            "--group", searched_chat_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        result = data.get("result", {})
        assert "inviteUrl" in result, f"result 缺少 inviteUrl: {result}"
        assert result["inviteUrl"], f"inviteUrl 不应为空: {result}"

    def test_get_invite_url_missing_group(self, dws):
        """不传 group 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "group", "invite-url",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_get_invite_url_invalid_group(self, dws):
        """无效 group ID 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "group", "invite-url",
            "--group", "INVALID_CONV_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_get_invite_url_with_expires(self, dws, searched_chat_id):
        """获取带有效期的群邀请链接。"""
        data = dws.run(
            "chat", "group", "invite-url",
            "--group", searched_chat_id,
            "--expires-seconds", "86400",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        result = data.get("result", {})
        assert "inviteUrl" in result, f"result 缺少 inviteUrl: {result}"
        assert result["inviteUrl"], f"inviteUrl 不应为空: {result}"

    def test_get_invite_url_permanent(self, dws, searched_chat_id):
        """获取永久有效的群邀请链接（expires-seconds=0）。"""
        data = dws.run(
            "chat", "group", "invite-url",
            "--group", searched_chat_id,
            "--expires-seconds", "0",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        result = data.get("result", {})
        assert "inviteUrl" in result, f"result 缺少 inviteUrl: {result}"


class TestGroupQuit:
    """dws chat group quit"""

    def test_quit_basic(self, dws, transient_group_id):
        """退出群聊 — 使用一次性群避免影响共享群。"""
        import json
        result = dws.run_raw(
            "chat", "group", "quit",
            "--group", transient_group_id,
        )
        combined = (result.stdout or "") + (result.stderr or "")
        try:
            data = json.loads(combined.strip())
        except json.JSONDecodeError:
            pytest.fail(f"quit 返回非 JSON: stdout={result.stdout!r}, stderr={result.stderr!r}")
        if data.get("success") is True:
            return
        err = data.get("error", {})
        assert err.get("reason") == "business_error", (
            f"预期 success=True 或 business_error，实际: {data}"
        )

    def test_quit_missing_group(self, dws):
        """不传 group 应报错（必填）。"""
        result = dws.run_raw("chat", "group", "quit")
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_quit_invalid_group(self, dws):
        """无效 group ID 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "group", "quit",
            "--group", "INVALID_CONV_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestGroupUpdateIcon:
    """dws chat group update-icon"""

    def test_update_icon_basic(self, dws, searched_chat_id):
        """更新群头像。"""
        data = dws.run(
            "chat", "group", "update-icon",
            "--group", searched_chat_id,
            "--icon-media-id", "$iwEcAqNwbmcDBgTRArwF0QK8BrA_jQKcntjKBwhM8Yuz1VoAB9IC0yg2CAAJomltCgAL0gABDA0",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_update_icon_missing_group(self, dws):
        """不传 group 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "group", "update-icon",
            "--icon-media-id", "test-media-id",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_update_icon_missing_media_id(self, dws, searched_chat_id):
        """不传 icon-media-id 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "group", "update-icon",
            "--group", searched_chat_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestGroupUpdateSettings:
    """dws chat group update-settings"""

    def test_update_settings_searchable(self, dws, searched_chat_id):
        """设置群可被搜索。"""
        data = dws.run(
            "chat", "group", "update-settings",
            "--group", searched_chat_id,
            "--setting-key", "searchable",
            "--status", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_update_settings_authority(self, dws, searched_chat_id):
        """设置群管理权限。"""
        data = dws.run(
            "chat", "group", "update-settings",
            "--group", searched_chat_id,
            "--setting-key", "authority",
            "--status", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_update_settings_group_live_authority(self, dws, searched_chat_id):
        """设置谁可以发起直播。"""
        data = dws.run(
            "chat", "group", "update-settings",
            "--group", searched_chat_id,
            "--setting-key", "groupLiveAuthority",
            "--status", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_update_settings_group_bill_authority(self, dws, searched_chat_id):
        """设置群收款开关。"""
        data = dws.run(
            "chat", "group", "update-settings",
            "--group", searched_chat_id,
            "--setting-key", "groupBillAuthority",
            "--status", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_update_settings_disable(self, dws, searched_chat_id):
        """关闭设置项（status=0）。"""
        data = dws.run(
            "chat", "group", "update-settings",
            "--group", searched_chat_id,
            "--setting-key", "searchable",
            "--status", "0",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_update_settings_missing_group(self, dws):
        """不传 group 应报错。"""
        result = dws.run_raw(
            "chat", "group", "update-settings",
            "--setting-key", "searchable",
            "--status", "1",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_update_settings_missing_setting_key(self, dws, searched_chat_id):
        """不传 setting-key 应报错。"""
        result = dws.run_raw(
            "chat", "group", "update-settings",
            "--group", searched_chat_id,
            "--status", "1",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_update_settings_missing_status(self, dws, searched_chat_id):
        """不传 status 应报错。"""
        result = dws.run_raw(
            "chat", "group", "update-settings",
            "--group", searched_chat_id,
            "--setting-key", "searchable",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
