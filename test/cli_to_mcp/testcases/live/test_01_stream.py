"""
test_live.py — 直播测试 (1 command × 3 cases)

Commands tested:
  1. dws live stream list  (get_my_lives)

Response schema:
  {
    "success": true,
    "result": {
      "hasFinish": bool,
      "liveDetailModelList": [...],
      "total": int
    }
  }
"""


class TestLiveStreamList:
    """dws live stream list"""

    def test_list_returns_data(self, dws):
        """查询我的直播列表。"""
        data = dws.run_ok("live", "stream", "list")
        assert data.get("success") is True, f"Expected success=true, got: {data.get('success')}"
        result = data.get("result", {})
        assert isinstance(result, dict), f"result should be dict, got: {type(result)}"
        assert isinstance(result.get("liveDetailModelList"), list), \
            f"result.liveDetailModelList should be list, got: {type(result.get('liveDetailModelList'))}"
        assert isinstance(result.get("total"), int), \
            f"result.total should be int, got: {type(result.get('total'))}"

    def test_list_idempotent(self, dws):
        """多次调用应返回相同结构。"""
        d1 = dws.run_ok("live", "stream", "list")
        d2 = dws.run_ok("live", "stream", "list")
        assert d1.get("success") == d2.get("success"), \
            f"Idempotent check: success mismatch"
        assert d1.get("result", {}).get("total") == d2.get("result", {}).get("total"), \
            f"Idempotent check: total mismatch"

    def test_list_has_finish_field(self, dws):
        """返回结构应包含 hasFinish 字段。"""
        data = dws.run_ok("live", "stream", "list")
        result = data.get("result", {})
        assert "hasFinish" in result, \
            f"result should contain 'hasFinish', got keys: {list(result.keys())}"
        assert isinstance(result["hasFinish"], bool), \
            f"result.hasFinish should be bool, got: {type(result['hasFinish'])}"
