"""
test_03_minutes_list_all.py — 听记 list all 测试 (1 command × 6 cases)

Commands tested:
  1. dws minutes list all  (list_by_keyword_and_time_range with noLimit)
"""

import pytest


@pytest.fixture(scope="session")
def minutes_id(dws):
    """获取听记 ID，优先用 list shared。"""
    data = dws.run_ok("minutes", "list", "shared")
    items = data.get("minutes", data.get("result", {}).get("minutes", []) if isinstance(data.get("result"), dict) else [])
    if not items:
        pytest.skip("No minutes available via list shared")
    mid = items[0].get("minutesId") or items[0].get("uuid") or items[0].get("id")
    if not mid:
        pytest.skip("Cannot extract minutesId")
    return mid


class TestMinutesListAll:
    """dws minutes list all"""

    def test_list_all_default(self, dws):
        """无参数调用 list all 应返回有效数据。"""
        data = dws.run_ok("minutes", "list", "all")
        assert data is not None

    def test_list_all_with_max(self, dws):
        """指定 --max 限制返回条数。"""
        data = dws.run_ok("minutes", "list", "all", "--max", "5")
        assert data is not None

    def test_list_all_with_query(self, dws):
        """使用 --query 关键字筛选。"""
        data = dws.run_ok("minutes", "list", "all", "--query", "测试")
        assert data is not None

    def test_list_all_with_time_range(self, dws):
        """使用 --start 和 --end 时间范围筛选。"""
        data = dws.run_ok(
            "minutes", "list", "all",
            "--start", "2025-01-01T00:00:00+08:00",
            "--end", "2026-12-31T23:59:59+08:00",
        )
        assert data is not None

    def test_list_all_idempotent(self, dws):
        """多次调用应返回稳定结果。"""
        data_first = dws.run_ok("minutes", "list", "all")
        data_second = dws.run_ok("minutes", "list", "all")
        assert data_first.get("status") == data_second.get("status")

    def test_list_all_invalid_time_range(self, dws):
        """start 晚于 end 应报错。"""
        result = dws.run_raw(
            "minutes", "list", "all",
            "--start", "2026-12-31T23:59:59+08:00",
            "--end", "2025-01-01T00:00:00+08:00",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
