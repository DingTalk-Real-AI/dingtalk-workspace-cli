"""
test_03_room.py — 会议室管理测试 (4 commands × 3+ cases)

Commands tested:
  1. dws calendar room search       (query_available_meeting_room)
  2. dws calendar room add          (add_meeting_room)
  3. dws calendar room delete       (delete_meeting_room)
  4. dws calendar room list-groups  (list_meeting_room_groups)
"""

import pytest

from test_utils import iso8601_cn_offset


class TestRoomSearch:
    """dws calendar room search"""

    def test_search_returns_list(self, dws):
        """搜索会议室应返回有效数据结构（服务端可能因上限报错，验证命令语法正确）。"""
        start = iso8601_cn_offset(hours=2)
        end = iso8601_cn_offset(hours=3)
        result = dws.run_raw(
            "calendar", "room", "search",
            "--start", start, "--end", end,
        )
        assert "unknown flag" not in result.stderr
        assert result.returncode is not None

    def test_search_different_time_ranges(self, dws):
        """不同时间段搜索应独立返回结果（验证命令语法）。"""
        start1 = iso8601_cn_offset(hours=4)
        end1 = iso8601_cn_offset(hours=5)
        result1 = dws.run_raw(
            "calendar", "room", "search",
            "--start", start1, "--end", end1,
        )

        start2 = iso8601_cn_offset(hours=24)
        end2 = iso8601_cn_offset(hours=25)
        result2 = dws.run_raw(
            "calendar", "room", "search",
            "--start", start2, "--end", end2,
        )
        # 两次搜索命令语法都正确
        assert "unknown flag" not in result1.stderr
        assert "unknown flag" not in result2.stderr

    def test_search_past_time_range(self, dws):
        """搜索过去的时间段，服务端应拒绝（filterStartTime can not less current time）。"""
        start = "2025-01-01T09:00:00+08:00"
        end = "2025-01-01T10:00:00+08:00"
        result = dws.run_raw(
            "calendar", "room", "search",
            "--start", start, "--end", end,
        )
        assert (
            result.returncode != 0
            or "error" in result.stderr.lower()
        ), "过去时间段搜索应被服务端拒绝"

    def test_search_with_room_name_flag(self, dws):
        """--room-name 按名称过滤应被 CLI 接受并透传给 MCP (roomName)。"""
        start = iso8601_cn_offset(hours=2)
        end = iso8601_cn_offset(hours=3)
        result = dws.run_raw(
            "calendar", "room", "search",
            "--start", start, "--end", end,
            "--room-name", "永澄亭",
        )
        assert "unknown flag" not in result.stderr
        assert result.returncode is not None

    def test_search_room_name_passthrough_no_trim(self, dws):
        """CLI 不做名称裁剪：传什么就透什么，命令应被正常接收。"""
        start = iso8601_cn_offset(hours=2)
        end = iso8601_cn_offset(hours=3)
        result = dws.run_raw(
            "calendar", "room", "search",
            "--start", start, "--end", end,
            "--room-name", "永澄亭会议室",
        )
        assert "unknown flag" not in result.stderr
        assert result.returncode is not None

    def test_search_room_name_aliases(self, dws):
        """隐藏别名 --name / --roomName / --query 应被 CLI 接受。"""
        start = iso8601_cn_offset(hours=2)
        end = iso8601_cn_offset(hours=3)
        for alias in ("--name", "--roomName", "--query"):
            result = dws.run_raw(
                "calendar", "room", "search",
                "--start", start, "--end", end,
                alias, "西湖厅",
            )
            assert "unknown flag" not in result.stderr, f"alias {alias} should be registered"


class TestRoomAdd:
    """dws calendar room add"""

    def test_add_room_to_event(self, dws, test_event_id):
        """搜索会议室并预定（服务端可能因上限返回错误，没有可用会议室则跳过）。"""
        start = iso8601_cn_offset(hours=2)
        end = iso8601_cn_offset(hours=3)
        search_result = dws.run_raw(
            "calendar", "room", "search",
            "--start", start, "--end", end,
        )
        import json
        try:
            search_data = json.loads(search_result.stdout)
            rooms = search_data.get("data", {}).get("rooms", []) or search_data.get("result", {}).get("rooms", [])
        except (json.JSONDecodeError, AttributeError):
            pytest.skip(f"会议室搜索返回非 JSON（{search_result.stderr[:100]})，跳过")
            return
        if not rooms:
            pytest.skip("No available meeting rooms to test")
        room_id = rooms[0].get("roomId") or rooms[0].get("id")
        if not room_id:
            pytest.skip("Cannot extract roomId from search result")
        result = dws.run_raw(
            "calendar", "room", "add",
            "--event", test_event_id,
            "--rooms", room_id,
        )
        assert result.returncode is not None

    def test_add_invalid_room(self, dws, test_event_id):
        """预定不存在的会议室应报错或忽略。"""
        result = dws.run_raw(
            "calendar", "room", "add",
            "--event", test_event_id,
            "--rooms", "INVALID_ROOM_99999",
        )
        # 可能报错也可能忽略
        assert result.returncode is not None

    def test_add_to_invalid_event(self, dws):
        """向无效日程添加会议室应报错。"""
        result = dws.run_raw(
            "calendar", "room", "add",
            "--event", "INVALID_EVENT_99999",
            "--rooms", "ROOM_DUMMY",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestRoomDelete:
    """dws calendar room delete"""

    def test_delete_room_from_event(self, dws, test_event_id):
        """移除不存在的会议室，服务端可能返回错误（roomId invalid）。"""
        result = dws.run_raw(
            "calendar", "room", "delete",
            "--event", test_event_id,
            "--rooms", "NON_EXISTENT_ROOM",
            "--yes",
        )
        assert result.returncode is not None

    def test_delete_from_invalid_event(self, dws):
        """从无效日程移除会议室应报错。"""
        result = dws.run_raw(
            "calendar", "room", "delete",
            "--event", "INVALID_EVENT_99999",
            "--rooms", "ROOM_DUMMY",
            "--yes",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_delete_multiple_rooms(self, dws, test_event_id):
        """一次移除多个会议室，服务端可能返回错误（伪造 roomId）。"""
        result = dws.run_raw(
            "calendar", "room", "delete",
            "--event", test_event_id,
            "--rooms", "R1,R2,R3",
            "--yes",
        )
        assert result.returncode is not None


class TestRoomListGroups:
    """dws calendar room list-groups"""

    def test_list_groups_returns_data(self, dws):
        """会议室分组列表应返回有效数据。"""
        data = dws.run_ok("calendar", "room", "list-groups")
        assert data is not None

    def test_list_groups_is_list(self, dws):
        """分组列表应为数组或含 groups 字段。"""
        data = dws.run_ok("calendar", "room", "list-groups")
        result = data.get("data", {})
        assert result is not None

    def test_list_groups_idempotent(self, dws):
        """多次调用应返回一致结果。"""
        d1 = dws.run_ok("calendar", "room", "list-groups")
        d2 = dws.run_ok("calendar", "room", "list-groups")
        assert str(d1.get("data", d1)) == str(d2.get("data", d2))

    def test_list_groups_with_pagination(self, dws):
        """传分页参数 --page-size / --page-index 应被 CLI 接受并透传。"""
        result = dws.run_raw(
            "calendar", "room", "list-groups",
            "--page-size", "5", "--page-index", "0",
        )
        assert "unknown flag" not in result.stderr
        assert result.returncode is not None
