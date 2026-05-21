"""calendar 高频错误参数回归用例。"""

from test_utils import iso8601_cn_offset


class TestCalendarParamRegression:
    def test_event_create_time_with_space_separator(self, dws):
        result = dws.run_raw(
            "calendar", "event", "create",
            "--title", "时间格式测试",
            "--start", "2026-03-23 14:00:00",
            "--end", "2026-03-23 15:00:00",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_event_create_time_without_timezone(self, dws):
        result = dws.run_raw(
            "calendar", "event", "create",
            "--title", "无时区测试",
            "--start", "2026-03-23T14:00:00",
            "--end", "2026-03-23T15:00:00",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_room_add_invalid_room_should_fail(self, dws):
        result = dws.run_raw(
            "calendar", "room", "add",
            "--event-id", "INVALID_EVENT",
            "--room-id", "INVALID_ROOM",
            "--start", iso8601_cn_offset(hours=1),
            "--end", iso8601_cn_offset(hours=2),
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

