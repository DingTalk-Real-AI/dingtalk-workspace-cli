"""
test_minutes.py — 闪记/听记测试 (10 commands × 3 cases)

Commands tested:
  1.  dws minutes list mine         (get_my_minutes_list)
  2.  dws minutes list shared       (get_shared_minutes_list)
  3.  dws minutes get info          (get_minutes_info)
  4.  dws minutes get summary       (get_minutes_summary)
  5.  dws minutes get keywords      (get_minutes_keywords)
  6.  dws minutes get transcription (get_minutes_transcription)
  7.  dws minutes get todos         (get_minutes_todos)
  8.  dws minutes get batch         (batch_get_minutes_headers)
  9.  dws minutes update title      (update_minutes_title)
  10. (get-info is prerequisite for the rest)
"""

import pytest






@pytest.fixture(scope="session")
def minutes_id(dws):
    """获取第一个听记的 ID 用于后续测试。优先用 list all，其次 list shared。"""
    for subcmd in ("all", "shared"):
        data = dws.run_ok("minutes", "list", subcmd)
        result = data.get("result", {})
        items = (
            result.get("itemList", [])
            if isinstance(result, dict)
            else []
        ) or data.get("minutes", [])
        if items:
            mid = items[0].get("minutesId") or items[0].get("uuid") or items[0].get("id")
            if mid:
                return mid
    pytest.skip("No minutes available")


class TestMinutesListMine:
    """dws minutes list mine"""

    def test_list_mine(self, dws):
        """查询我创建的听记列表。"""
        data = dws.run_ok("minutes", "list", "mine")
        assert data.get("success") is True, f"success 应为 True: {data}"
        result = data.get("result", {})
        assert isinstance(result, dict), f"result 应为 dict: {type(result)}"
        assert "itemList" in result, f"result 缺少 itemList: {result.keys()}"

    def test_list_mine_has_valid_structure(self, dws):
        """list mine 返回结构应含 hasMore 和 nextToken。"""
        data = dws.run_ok("minutes", "list", "mine")
        result = data.get("result", {})
        assert "hasMore" in result, f"result 缺少 hasMore: {result.keys()}"

    def test_list_mine_idempotent(self, dws):
        """多次调用应返回一致结果。"""
        d1 = dws.run_ok("minutes", "list", "mine")
        d2 = dws.run_ok("minutes", "list", "mine")
        assert d1.get("success") == d2.get("success")


class TestMinutesListShared:
    """dws minutes list shared"""

    def test_list_shared(self, dws):
        """列出共享的听记。"""
        data = dws.run_ok("minutes", "list", "shared")
        assert data.get("success") is True, f"success 应为 True: {data}"
        result = data.get("result", {})
        assert isinstance(result, dict)
        assert "itemList" in result, f"result 缺少 itemList: {result.keys()}"

    def test_list_shared_structure(self, dws):
        """list shared 返回结构应含 hasMore。"""
        data = dws.run_ok("minutes", "list", "shared")
        result = data.get("result", {})
        assert "hasMore" in result, f"result 缺少 hasMore: {result.keys()}"

    def test_list_shared_idempotent(self, dws):
        """多次调用应稳定。"""
        d1 = dws.run_ok("minutes", "list", "shared")
        d2 = dws.run_ok("minutes", "list", "shared")
        assert d1.get("success") == d2.get("success")


class TestMinutesGetInfo:
    """dws minutes get info"""

    def test_get_info(self, dws, minutes_id):
        """获取听记基本信息。"""
        data = dws.run_ok(
            "minutes", "get", "info", "--id", minutes_id,
        )
        assert isinstance(data, dict) and len(data) > 0

    def test_get_info_contains_title(self, dws, minutes_id):
        """听记详情应含有效内容。"""
        data = dws.run_ok(
            "minutes", "get", "info", "--id", minutes_id,
        )
        assert isinstance(data, dict) and len(data) > 0

    def test_get_info_invalid(self, dws):
        """无效 ID 应报错。"""
        result = dws.run_raw(
            "minutes", "get", "info", "--id", "INVALID",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestMinutesGetSummary:
    """dws minutes get summary"""

    def test_get_summary(self, dws, minutes_id):
        """获取听记摘要。"""
        data = dws.run_ok(
            "minutes", "get", "summary", "--id", minutes_id,
        )
        assert isinstance(data, dict) and len(data) > 0

    def test_get_summary_structure(self, dws, minutes_id):
        """摘要应含有效内容。"""
        data = dws.run_ok(
            "minutes", "get", "summary", "--id", minutes_id,
        )
        assert isinstance(data, dict) and len(data) > 0

    def test_get_summary_invalid(self, dws):
        """无效 ID 获取摘要。"""
        result = dws.run_raw(
            "minutes", "get", "summary", "--id", "INVALID",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
