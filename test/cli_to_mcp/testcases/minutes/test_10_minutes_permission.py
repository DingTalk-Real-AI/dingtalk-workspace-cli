"""
test_10_minutes_permission.py — 听记成员权限管理测试 (2 commands × 4 cases each)

Commands tested:
  1. dws minutes permission add     (add_member_permission)
  2. dws minutes permission remove  (remove_member_permission)
"""

from typing import Optional

import pytest


def _extract_items_from_list(data: dict) -> list:
    """从 list 返回的多种可能结构中提取 items 列表。"""
    result = data.get("result", {})
    if isinstance(result, dict):
        items = result.get("itemList", []) or result.get("minutes", []) or result.get("items", [])
        if items:
            return items
    if isinstance(result, list):
        return result
    return data.get("minutes", []) or data.get("itemList", []) or data.get("items", [])


def _extract_id_from_item(item: dict) -> Optional[str]:
    """从单个听记 item 中提取 ID。"""
    return (
        item.get("taskUuid")
        or item.get("minutesId")
        or item.get("uuid")
        or item.get("id")
    )


@pytest.fixture(scope="session")
def minutes_id(dws):
    """获取听记 ID，优先用 list all，其次 list shared。"""
    for subcmd in ("all", "shared"):
        data = dws.run_ok("minutes", "list", subcmd)
        items = _extract_items_from_list(data)
        if items:
            mid = _extract_id_from_item(items[0])
            if mid:
                return mid
    pytest.skip("No minutes available")


class TestPermissionAdd:
    """dws minutes permission add"""

    def test_add_permission_basic(self, dws, minutes_id):
        """添加成员权限（基本用法，仅查看权限）。"""
        result = dws.run_raw(
            "minutes", "permission", "add",
            "--ids", minutes_id,
            "--member-uids", "999999999",
            "--policy", "4",
        )
        # 命令语法应正确，不应报 unknown flag
        assert "unknown flag" not in result.stderr

    def test_add_permission_with_cover(self, dws, minutes_id):
        """添加成员权限并覆盖已有权限。"""
        result = dws.run_raw(
            "minutes", "permission", "add",
            "--ids", minutes_id,
            "--member-uids", "999999999",
            "--policy", "3",
            "--cover",
        )
        assert "unknown flag" not in result.stderr

    def test_add_permission_with_sub_resources(self, dws, minutes_id):
        """添加成员权限并指定权限子模块。"""
        result = dws.run_raw(
            "minutes", "permission", "add",
            "--ids", minutes_id,
            "--member-uids", "999999999",
            "--policy", "3",
            "--sub-resources", "OrigContent,Summary",
        )
        assert "unknown flag" not in result.stderr

    def test_add_permission_uuids_alias(self, dws, minutes_id):
        """使用 --uuids 别名添加权限。"""
        result = dws.run_raw(
            "minutes", "permission", "add",
            "--uuids", minutes_id,
            "--member-uids", "999999999",
            "--policy", "4",
        )
        assert "unknown flag" not in result.stderr

    def test_add_permission_task_uuids_alias(self, dws, minutes_id):
        """使用 --task-uuids 别名添加权限。"""
        result = dws.run_raw(
            "minutes", "permission", "add",
            "--task-uuids", minutes_id,
            "--member-uids", "999999999",
            "--policy", "4",
        )
        assert "unknown flag" not in result.stderr

    def test_add_permission_missing_ids(self, dws):
        """缺少 --ids 参数应报错。"""
        result = dws.run_raw(
            "minutes", "permission", "add",
            "--member-uids", "999999999",
            "--policy", "3",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_add_permission_missing_member_uids(self, dws, minutes_id):
        """缺少 --member-uids 参数应报错。"""
        result = dws.run_raw(
            "minutes", "permission", "add",
            "--ids", minutes_id,
            "--policy", "3",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_add_permission_invalid_policy(self, dws, minutes_id):
        """无效的 --policy 值应报错。"""
        result = dws.run_raw(
            "minutes", "permission", "add",
            "--ids", minutes_id,
            "--member-uids", "999999999",
            "--policy", "99",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_add_permission_invalid_member_uid(self, dws, minutes_id):
        """非数字的 --member-uids 应报错。"""
        result = dws.run_raw(
            "minutes", "permission", "add",
            "--ids", minutes_id,
            "--member-uids", "not_a_number",
            "--policy", "3",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()


class TestPermissionRemove:
    """dws minutes permission remove"""

    def test_remove_permission_basic(self, dws, minutes_id):
        """移除成员权限（基本用法）。"""
        result = dws.run_raw(
            "minutes", "permission", "remove",
            "--ids", minutes_id,
            "--member-uids", "999999999",
        )
        # 命令语法应正确，不应报 unknown flag
        assert "unknown flag" not in result.stderr

    def test_remove_permission_uuids_alias(self, dws, minutes_id):
        """使用 --uuids 别名移除权限。"""
        result = dws.run_raw(
            "minutes", "permission", "remove",
            "--uuids", minutes_id,
            "--member-uids", "999999999",
        )
        assert "unknown flag" not in result.stderr

    def test_remove_permission_task_uuids_alias(self, dws, minutes_id):
        """使用 --task-uuids 别名移除权限。"""
        result = dws.run_raw(
            "minutes", "permission", "remove",
            "--task-uuids", minutes_id,
            "--member-uids", "999999999",
        )
        assert "unknown flag" not in result.stderr

    def test_remove_permission_missing_ids(self, dws):
        """缺少 --ids 参数应报错。"""
        result = dws.run_raw(
            "minutes", "permission", "remove",
            "--member-uids", "999999999",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_remove_permission_missing_member_uids(self, dws, minutes_id):
        """缺少 --member-uids 参数应报错。"""
        result = dws.run_raw(
            "minutes", "permission", "remove",
            "--ids", minutes_id,
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_remove_permission_invalid_member_uid(self, dws, minutes_id):
        """非数字的 --member-uids 应报错。"""
        result = dws.run_raw(
            "minutes", "permission", "remove",
            "--ids", minutes_id,
            "--member-uids", "abc_invalid",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()
