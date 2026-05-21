"""
test_devdoc.py — 开放平台文档搜索测试 (1 command × 3 cases)

Commands tested:
  1. dws devdoc article search  (search_open_platform_docs)

Response schema:
  {
    "success": true,
    "result": {
      "currentPage": int,
      "hasMore": bool,
      "pageSize": int,
      "totalCount": int,
      "items": [ { "title": str, "desc": str, "url": str } ]
    }
  }

NOTE: CLI flag is --query (not --keyword).
"""


class TestDevdocArticleSearch:
    """dws devdoc article search"""

    def test_search_mcp(self, dws):
        """搜索 MCP 相关文档。"""
        data = dws.run_ok(
            "devdoc", "article", "search",
            "--query", "MCP",
            "--page", "1", "--size", "10",
        )
        assert data.get("success") is True, f"Expected success=true, got: {data.get('success')}"
        result = data.get("result", {})
        assert isinstance(result, dict), f"result should be dict, got: {type(result)}"
        assert isinstance(result.get("items"), list), \
            f"result.items should be list, got: {type(result.get('items'))}"
        assert isinstance(result.get("totalCount"), int), \
            f"result.totalCount should be int, got: {type(result.get('totalCount'))}"
        # MCP is a common topic, should have results
        assert result["totalCount"] > 0, "Search for 'MCP' should return results"

    def test_search_chinese(self, dws):
        """搜索中文关键词。"""
        data = dws.run_ok(
            "devdoc", "article", "search",
            "--query", "机器人",
            "--page", "1", "--size", "5",
        )
        assert data.get("success") is True
        result = data.get("result", {})
        assert isinstance(result.get("items"), list)
        assert "currentPage" in result, f"result should contain 'currentPage', got keys: {list(result.keys())}"

    def test_search_page2(self, dws):
        """翻页搜索。"""
        data = dws.run_ok(
            "devdoc", "article", "search",
            "--query", "openConversationId",
            "--page", "2", "--size", "5",
        )
        assert data.get("success") is True
        result = data.get("result", {})
        assert isinstance(result.get("items"), list)
        assert "hasMore" in result, f"result should contain 'hasMore', got keys: {list(result.keys())}"
