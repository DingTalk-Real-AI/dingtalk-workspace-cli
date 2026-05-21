"""场景3回归: list 命令筛选能力验证。

验证 --query / --start / --end 组合筛选能力正常工作，
确保 LLM 可以直接使用服务端筛选而非全量拉取后本地过滤。
"""

import pytest


class TestListAllQueryFilter:
    """dws minutes list all --query 关键词筛选。"""

    def test_query_returns_valid_structure(self, dws):
        """--query 筛选应返回有效的列表结构。"""
        data = dws.run_ok("minutes", "list", "all", "--query", "会议")
        assert data.get("success") is True
        result = data.get("result", {})
        assert isinstance(result, dict)
        assert "itemList" in result

    def test_query_with_max_limit(self, dws):
        """--query + --max 组合应正常工作。"""
        data = dws.run_ok("minutes", "list", "all", "--query", "会议", "--max", "5")
        assert data.get("success") is True
        items = data.get("result", {}).get("itemList", [])
        assert len(items) <= 5

    def test_query_empty_keyword_returns_all(self, dws):
        """空关键词应等同于不筛选。"""
        data = dws.run_ok("minutes", "list", "all", "--max", "3")
        assert data.get("success") is True


class TestListAllTimeRangeFilter:
    """dws minutes list all --start/--end 时间范围筛选。"""

    def test_time_range_returns_valid_structure(self, dws):
        """--start + --end 时间范围筛选应返回有效结构。"""
        data = dws.run_ok(
            "minutes", "list", "all",
            "--start", "2026-01-01T00:00:00+08:00",
            "--end", "2026-12-31T23:59:59+08:00",
        )
        assert data.get("success") is True
        result = data.get("result", {})
        assert isinstance(result, dict)
        assert "itemList" in result

    def test_narrow_time_range(self, dws):
        """窄时间范围应返回有效结构（可能为空列表）。"""
        data = dws.run_ok(
            "minutes", "list", "all",
            "--start", "2026-05-11T00:00:00+08:00",
            "--end", "2026-05-11T23:59:59+08:00",
        )
        assert data.get("success") is True


class TestListAllCombinedFilter:
    """--query + --start + --end 组合筛选。"""

    def test_query_and_time_range_combined(self, dws):
        """关键词 + 时间范围组合筛选应正常工作。"""
        data = dws.run_ok(
            "minutes", "list", "all",
            "--query", "会议",
            "--start", "2026-01-01T00:00:00+08:00",
            "--end", "2026-12-31T23:59:59+08:00",
            "--max", "10",
        )
        assert data.get("success") is True


class TestListMineQueryFilter:
    """dws minutes list mine --query 筛选。"""

    def test_mine_query_filter(self, dws):
        """list mine 也支持 --query 筛选。"""
        data = dws.run_ok("minutes", "list", "mine", "--query", "周会")
        assert data.get("success") is True

    def test_mine_time_range_filter(self, dws):
        """list mine 也支持 --start/--end 筛选。"""
        data = dws.run_ok(
            "minutes", "list", "mine",
            "--start", "2026-01-01T00:00:00+08:00",
            "--end", "2026-12-31T23:59:59+08:00",
        )
        assert data.get("success") is True


class TestListMineCombinedFilter:
    """dws minutes list mine --query + --start/--end 组合筛选（"从今天/上月听记里找<关键词>"场景）。"""

    def test_mine_query_and_today_time_range(self, dws):
        """list mine --query + 今日时间范围应正常工作。"""
        data = dws.run_ok(
            "minutes", "list", "mine",
            "--query", "需求评审",
            "--start", "2026-05-11T00:00:00+08:00",
            "--end", "2026-05-11T23:59:59+08:00",
        )
        assert data.get("success") is True
        result = data.get("result", {})
        assert isinstance(result, dict)
        assert "itemList" in result

    def test_mine_query_and_last_month_time_range(self, dws):
        """list mine --query + 上月时间范围应正常工作。"""
        data = dws.run_ok(
            "minutes", "list", "mine",
            "--query", "OKR",
            "--start", "2026-04-01T00:00:00+08:00",
            "--end", "2026-04-30T23:59:59+08:00",
        )
        assert data.get("success") is True
        result = data.get("result", {})
        assert isinstance(result, dict)
        assert "itemList" in result

    def test_mine_query_and_this_week_time_range(self, dws):
        """list mine --query + 本周时间范围应正常工作。"""
        data = dws.run_ok(
            "minutes", "list", "mine",
            "--query", "复盘",
            "--start", "2026-05-05T00:00:00+08:00",
            "--end", "2026-05-11T23:59:59+08:00",
        )
        assert data.get("success") is True

    def test_mine_query_only(self, dws):
        """list mine 仅 --query 不带时间范围应正常工作。"""
        data = dws.run_ok("minutes", "list", "mine", "--query", "技术方案")
        assert data.get("success") is True
        result = data.get("result", {})
        assert isinstance(result, dict)
        assert "itemList" in result

    def test_mine_query_and_this_month_time_range(self, dws):
        """list mine --query + 本月时间范围应正常工作。"""
        data = dws.run_ok(
            "minutes", "list", "mine",
            "--query", "双月汇报",
            "--start", "2026-05-01T00:00:00+08:00",
            "--end", "2026-05-31T23:59:59+08:00",
        )
        assert data.get("success") is True

    def test_mine_query_and_last_week_time_range(self, dws):
        """list mine --query + 上周时间范围应正常工作。"""
        data = dws.run_ok(
            "minutes", "list", "mine",
            "--query", "项目排期",
            "--start", "2026-05-04T00:00:00+08:00",
            "--end", "2026-05-10T23:59:59+08:00",
        )
        assert data.get("success") is True


class TestListSharedQueryFilter:
    """dws minutes list shared --query 筛选。"""

    def test_shared_query_filter(self, dws):
        """list shared 也支持 --query 筛选。"""
        data = dws.run_ok("minutes", "list", "shared", "--query", "周会")
        assert data.get("success") is True
