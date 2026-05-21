"""
test_05_template.py — 模板搜索测试 (1 command)

Commands tested:
  19. dws aitable template search   (search_templates)
"""

import pytest


class TestTemplateSearch:
    """dws aitable template search"""

    def test_search_common_keyword(self, dws):
        """搜索 '项目管理' 应返回至少 1 个模板。"""
        data = dws.run(
            "aitable", "template", "search", "--query", "项目管理"
        )
        templates = data["data"]["templates"]
        assert isinstance(templates, list)
        assert len(templates) >= 1
        # 验证结构
        t = templates[0]
        assert "templateId" in t
        assert "name" in t

    def test_search_with_limit(self, dws):
        """--limit 2 限制返回数量。"""
        data = dws.run(
            "aitable", "template", "search",
            "--query", "项目", "--limit", "2",
        )
        templates = data["data"]["templates"]
        assert len(templates) <= 2

    def test_search_pagination(self, dws):
        """翻页: 先取 1 条，再用 cursor 取下一页。"""
        page1 = dws.run(
            "aitable", "template", "search",
            "--query", "项目", "--limit", "1",
        )
        cursor = page1["data"].get("nextCursor")
        if not cursor:
            pytest.skip("No next page for template search")

        page2 = dws.run(
            "aitable", "template", "search",
            "--query", "项目", "--limit", "1", "--cursor", cursor,
        )
        t2 = page2["data"]["templates"]
        assert len(t2) >= 1
        # 不应与第一页重复
        id1 = page1["data"]["templates"][0]["templateId"]
        id2 = t2[0]["templateId"]
        assert id1 != id2

    def test_search_no_result(self, dws):
        """搜索不存在的关键词应返回空列表。"""
        data = dws.run(
            "aitable", "template", "search",
            "--query", "ZZZZZ_不存在的模板_99999",
        )
        templates = data["data"].get("templates", [])
        assert templates is None or len(templates) == 0
