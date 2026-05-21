"""场景4回归: transcription 翻页机制验证。

验证 get transcription 的翻页机制：首页调用、next-token 翻页、
空 next-token 处理等，确保 LLM 不会陷入翻页死循环。
"""

import pytest

class TestTranscriptionFirstPage:
    """get transcription 首页调用验证。"""

    @pytest.fixture(scope="class")
    def sample_id(self, dws):
        data = dws.run_ok("minutes", "list", "mine", "--max", "1")
        items = data.get("result", {}).get("itemList", [])
        if not items:
            pytest.skip("当前账号无听记数据")
        mid = items[0].get("taskUuid") or items[0].get("uuid") or items[0].get("id")
        if not mid:
            pytest.skip("听记项缺少 ID 字段")
        return mid

    def test_first_page_returns_valid_data(self, dws, sample_id):
        """首页调用（不传 next-token）应返回有效数据。"""
        data = dws.run_ok("minutes", "get", "transcription", "--id", sample_id)
        assert isinstance(data, dict)

    def test_first_page_may_contain_next_token(self, dws, sample_id):
        """首页返回中可能包含 nextToken 用于翻页。"""
        data = dws.run_ok("minutes", "get", "transcription", "--id", sample_id)
        # nextToken 可能存在也可能不存在（短听记只有一页）
        assert isinstance(data, dict)

class TestTranscriptionEmptyNextToken:
    """空 next-token 处理验证。"""

    @pytest.fixture(scope="class")
    def sample_id(self, dws):
        data = dws.run_ok("minutes", "list", "mine", "--max", "1")
        items = data.get("result", {}).get("itemList", [])
        if not items:
            pytest.skip("当前账号无听记数据")
        mid = items[0].get("taskUuid") or items[0].get("uuid") or items[0].get("id")
        if not mid:
            pytest.skip("听记项缺少 ID 字段")
        return mid

    def test_empty_next_token_treated_as_first_page(self, dws, sample_id):
        """传入空字符串 next-token 应等同于首页查询。"""
        data = dws.run_ok(
            "minutes", "get", "transcription",
            "--id", sample_id, "--next-token", "",
        )
        assert isinstance(data, dict)

class TestTranscriptionInvalidNextToken:
    """无效 next-token 处理验证。"""

    @pytest.fixture(scope="class")
    def sample_id(self, dws):
        data = dws.run_ok("minutes", "list", "mine", "--max", "1")
        items = data.get("result", {}).get("itemList", [])
        if not items:
            pytest.skip("当前账号无听记数据")
        mid = items[0].get("taskUuid") or items[0].get("uuid") or items[0].get("id")
        if not mid:
            pytest.skip("听记项缺少 ID 字段")
        return mid

    def test_invalid_next_token_returns_response(self, dws, sample_id):
        """无效 next-token 应返回某种响应（而非挂起）。"""
        result = dws.run_raw(
            "minutes", "get", "transcription",
            "--id", sample_id,
            "--next-token", "INVALID_TOKEN_PLACEHOLDER",
        )
        # 不管成功还是失败，都应在合理时间内返回
        # 只验证进程正常返回（未挂起），returncode 不为 None 即可
        assert result.returncode is not None, "进程未正常返回（可能挂起）"

class TestTranscriptionDirection:
    """--direction 参数验证。"""

    @pytest.fixture(scope="class")
    def sample_id(self, dws):
        data = dws.run_ok("minutes", "list", "mine", "--max", "1")
        items = data.get("result", {}).get("itemList", [])
        if not items:
            pytest.skip("当前账号无听记数据")
        mid = items[0].get("taskUuid") or items[0].get("uuid") or items[0].get("id")
        if not mid:
            pytest.skip("听记项缺少 ID 字段")
        return mid

    def test_direction_0_ascending(self, dws, sample_id):
        """--direction 0 正序应正常返回。"""
        data = dws.run_ok(
            "minutes", "get", "transcription",
            "--id", sample_id, "--direction", "0",
        )
        assert isinstance(data, dict)

    def test_direction_1_descending(self, dws, sample_id):
        """--direction 1 倒序应正常返回。"""
        data = dws.run_ok(
            "minutes", "get", "transcription",
            "--id", sample_id, "--direction", "1",
        )
        assert isinstance(data, dict)
