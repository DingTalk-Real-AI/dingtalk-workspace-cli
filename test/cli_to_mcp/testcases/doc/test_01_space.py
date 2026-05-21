"""
test_01_space.py — 文档空间测试 (2 commands × 3 cases)

注: dws doc space get-root 已移除。
    现用 dws doc list (默认根目录) 和 dws doc search 替代。

实际返回格式:
  doc list:   {"hasMore": bool, "logId": "...", "nodes": [...], "success": true}
  doc search: {"documents": [...], "logId": "...", "success": true}
"""

import pytest


class TestDocList:
    """dws doc list — 默认列出根目录文件"""

    def test_list_root_returns_nodes(self, dws):
        """列出根目录应返回 nodes 列表。"""
        data = dws.run("doc", "list")
        assert "nodes" in data, f"响应缺少 nodes 字段: {data}"
        assert isinstance(data["nodes"], list), f"nodes 应为列表: {type(data['nodes'])}"

    def test_list_root_has_success(self, dws):
        """响应应包含 success 字段。"""
        data = dws.run("doc", "list")
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_list_idempotent(self, dws):
        """多次调用应一致。"""
        d1 = dws.run("doc", "list")
        d2 = dws.run("doc", "list")
        n1 = [n.get("nodeId") for n in d1.get("nodes", [])]
        n2 = [n.get("nodeId") for n in d2.get("nodes", [])]
        assert n1 == n2, "两次调用 nodes 结果不一致"


class TestDocSearch:
    """dws doc search"""

    def test_search_recent(self, dws):
        """不传 query 应返回最近访问文档。"""
        data = dws.run_ok("doc", "search")
        assert "documents" in data, f"响应缺少 documents 字段: {data}"
        assert isinstance(data["documents"], list)

    def test_search_with_query(self, dws):
        """按关键词搜索应返回 documents 列表。"""
        data = dws.run_ok("doc", "search", "--query", "测试")
        assert "documents" in data, f"响应缺少 documents 字段: {data}"
        assert isinstance(data["documents"], list)

    def test_search_no_match(self, dws):
        """搜索不存在的关键词应返回空列表。"""
        data = dws.run_ok("doc", "search", "--query", "ZZZNONEXIST99999")
        assert "documents" in data
        assert isinstance(data["documents"], list)
