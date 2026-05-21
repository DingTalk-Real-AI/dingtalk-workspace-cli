"""chat 高频错误参数回归用例。"""


class TestChatSearchParamRegression:
    def test_search_missing_query(self, dws):
        result = dws.run_raw("chat", "search")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()
