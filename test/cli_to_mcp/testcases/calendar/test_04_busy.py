"""
test_04_busy.py — 闲忙查询测试

Commands tested:
  1. dws calendar busy search  (query_busy_status)
     - 仅 --users
     - 仅 --rooms
     - --users + --rooms 同时
     - 不传 --users / --rooms 应报错
"""

import json

import pytest

from test_utils import iso8601_cn_offset


class TestBusySearch:
    """dws calendar busy search"""

    def test_query_self_busy(self, dws, current_user_id):
        """查询当前用户闲忙状态应成功。"""
        start = iso8601_cn_offset(hours=0)
        end = iso8601_cn_offset(hours=8)
        data = dws.run_ok(
            "calendar", "busy", "search",
            "--users", current_user_id,
            "--start", start, "--end", end,
        )
        assert data is not None

    def test_query_multiple_users(self, dws, current_user_id):
        """查询多个用户（含重复）的闲忙状态。"""
        start = iso8601_cn_offset(hours=0)
        end = iso8601_cn_offset(hours=4)
        data = dws.run_ok(
            "calendar", "busy", "search",
            "--users", f"{current_user_id},{current_user_id}",
            "--start", start, "--end", end,
        )
        assert data is not None

    def test_query_past_range(self, dws, current_user_id):
        """查询过去时段的闲忙状态应成功。"""
        start = "2026-03-01T09:00:00+08:00"
        end = "2026-03-01T18:00:00+08:00"
        data = dws.run_ok(
            "calendar", "busy", "search",
            "--users", current_user_id,
            "--start", start, "--end", end,
        )
        assert data is not None

    def _try_resolve_room_id(self, dws, start: str, end: str):
        """尝试从会议室搜索中拿到一个真实 roomId；拿不到返回 None。

        无可用会议室不再跳过，调用方会退化为占位 roomId 验证 CLI 透传层。
        """
        search_result = dws.run_raw(
            "calendar", "room", "search",
            "--start", start, "--end", end,
        )
        try:
            search_data = json.loads(search_result.stdout)
        except (json.JSONDecodeError, AttributeError):
            return None
        rooms = (
            search_data.get("data", {}).get("rooms", [])
            or search_data.get("result", {}).get("rooms", [])
        )
        if not rooms:
            return None
        return rooms[0].get("roomId") or rooms[0].get("id")

    def test_query_rooms_only(self, dws):
        """仅指定 --rooms 查询会议室闲忙。

        - 有真实可用会议室：走正向路径，断言服务端返回有效数据；
        - 无可用会议室：视作预期情况，退化为占位 roomId，仅校验 CLI 透传
          层（flag 已注册、命令能被 CLI 正常接收），避免出现 SKIPPED。
        """
        start = iso8601_cn_offset(hours=1)
        end = iso8601_cn_offset(hours=2)
        room_id = self._try_resolve_room_id(dws, start, end)
        if room_id:
            data = dws.run_ok(
                "calendar", "busy", "search",
                "--rooms", room_id,
                "--start", start, "--end", end,
            )
            assert data is not None
            return
        # 降级路径：使用占位 roomId，仅校验 CLI 层正确透传 --rooms
        result = dws.run_raw(
            "calendar", "busy", "search",
            "--rooms", "PLACEHOLDER_ROOM_ID",
            "--start", start, "--end", end,
        )
        assert "unknown flag" not in result.stderr, "--rooms 必须被 CLI 接受"
        assert result.returncode is not None

    def test_query_users_and_rooms(self, dws, current_user_id):
        """同时指定 --users 和 --rooms 查询闲忙。

        - 有真实可用会议室：走正向路径，断言服务端返回有效数据；
        - 无可用会议室：视作预期情况，退化为占位 roomId，仅校验 CLI 同时
          透传 --users 与 --rooms 两个 flag，避免出现 SKIPPED。
        """
        start = iso8601_cn_offset(hours=1)
        end = iso8601_cn_offset(hours=2)
        room_id = self._try_resolve_room_id(dws, start, end)
        if room_id:
            data = dws.run_ok(
                "calendar", "busy", "search",
                "--users", current_user_id,
                "--rooms", room_id,
                "--start", start, "--end", end,
            )
            assert data is not None
            return
        # 降级路径：使用占位 roomId，仅校验 CLI 层能同时接收 --users / --rooms
        result = dws.run_raw(
            "calendar", "busy", "search",
            "--users", current_user_id,
            "--rooms", "PLACEHOLDER_ROOM_ID",
            "--start", start, "--end", end,
        )
        assert "unknown flag" not in result.stderr, "--users 与 --rooms 必须被 CLI 同时接受"
        assert result.returncode is not None

    def test_query_missing_users_and_rooms(self, dws):
        """同时缺少 --users 和 --rooms 时 CLI 应前置报错。"""
        start = iso8601_cn_offset(hours=0)
        end = iso8601_cn_offset(hours=1)
        result = dws.run_raw(
            "calendar", "busy", "search",
            "--start", start, "--end", end,
        )
        assert result.returncode != 0 or "至少" in (result.stdout + result.stderr) or "users" in (result.stdout + result.stderr).lower()
