"""
test_oa.py — OA 审批测试 (1 command × 3 cases)

Commands tested:
  1. dws oa approval list-pending  (list_pending_approvals_for_me)

返回格式: {"result": {"hasMore":bool, "processInstanceList":[], "totalCount":int}, "success":true}
时间参数使用 ISO-8601（与 CLI `parseISOTimeToMillis` 一致）。
"""

from datetime import datetime, timedelta

from test_utils import TZ_CN, iso8601_cn

PROCESS_CODE = "PROC-C6A9BDC0-93BD-459D-91E9-DF26B6981ACA"


class TestOaListPending:
    """dws oa approval list-pending"""

    def test_list_pending_this_week(self, dws):
        """查询本周待审批。"""
        now = datetime.now(TZ_CN)
        time_from = iso8601_cn(now - timedelta(days=7))
        time_to = iso8601_cn(now)
        data = dws.run_ok(
            "oa", "approval", "list-pending",
            "--start", time_from, "--end", time_to,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        result = data.get("result", {})
        assert isinstance(result, dict), f"result 应为 dict: {type(result)}"
        assert "processInstanceList" in result, f"result 缺少 processInstanceList: {result.keys()}"

    def test_list_pending_past_month(self, dws):
        """查询过去 30 天待审批。"""
        now = datetime.now(TZ_CN)
        time_from = iso8601_cn(now - timedelta(days=30))
        time_to = iso8601_cn(now)
        data = dws.run_ok(
            "oa", "approval", "list-pending",
            "--start", time_from, "--end", time_to,
        )
        assert data.get("success") is True
        result = data.get("result", {})
        assert "totalCount" in result, f"result 缺少 totalCount: {result.keys()}"

    def test_list_pending_narrow_range(self, dws):
        """查询极短时间范围内的审批。"""
        now = datetime.now(TZ_CN)
        time_from = iso8601_cn(now - timedelta(hours=1))
        time_to = iso8601_cn(now)
        data = dws.run_ok(
            "oa", "approval", "list-pending",
            "--start", time_from, "--end", time_to,
        )
        assert data.get("success") is True
        result = data.get("result", {})
        assert isinstance(result.get("processInstanceList"), list)
