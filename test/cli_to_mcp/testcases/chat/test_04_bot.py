"""
test_04_bot.py — 机器人搜索测试 (1 command × N cases)

Commands tested:
  1. dws chat bot search  (搜索我创建的机器人)
"""


class TestChatBotSearch:
    """dws chat bot search — 搜索我创建的机器人"""

    def test_search_default(self, dws):
        """默认搜索返回结果。"""
        data = dws.run("chat", "bot", "search")
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert isinstance(data.get("robotList"), list), f"robotList 应为列表: {data}"

    def test_search_with_page(self, dws):
        """指定页码搜索。"""
        data = dws.run("chat", "bot", "search", "--page", "1")
        assert data.get("success") is True
        assert isinstance(data.get("robotList"), list)

    def test_search_with_size(self, dws):
        """指定每页条数搜索。"""
        data = dws.run("chat", "bot", "search", "--size", "10")
        assert data.get("success") is True
        assert isinstance(data.get("robotList"), list)

    def test_search_with_name(self, dws):
        """按名称搜索机器人。"""
        data = dws.run("chat", "bot", "search", "--name", "测试")
        assert data.get("success") is True
        assert isinstance(data.get("robotList"), list)

    def test_search_with_size(self, dws):
        """指定每页条数（--size）。"""
        data = dws.run("chat", "bot", "search", "--size", "5")
        assert data.get("success") is True
        assert isinstance(data.get("robotList"), list)
