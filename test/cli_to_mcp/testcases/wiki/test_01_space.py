"""
test_01_space.py — 钉钉知识库 Space 命令测试

Commands tested:
  1. dws wiki space create  —— 创建知识库
  2. dws wiki space get     —— 查看知识库详情
  3. dws wiki space list    —— 列出知识库
  4. dws wiki space search  —— 搜索知识库

执行约束:
  - 需要 dws >= 0.2.55；旧版本由 wiki/conftest.py 的产品级 skip 兜底。

正向用例的断言策略:
  - dws.run() 内部已校验 success=True / 无 error 字段；
  - 仅对从 wiki.go 源码可推导的字段（workspaceId）做精确断言，避免猜测。
"""

import time
import uuid

import pytest


# ─────────────────────────────────────────────────────────────
# create
# ─────────────────────────────────────────────────────────────

class TestWikiSpaceCreate:
    """dws wiki space create"""

    def test_create_basic(self, dws):
        """仅传 --name 应成功创建知识库并返回 workspaceId。"""
        name = f"CLI_WikiSpace_{int(time.time())}_{uuid.uuid4().hex[:6]}"
        data = dws.run("wiki", "space", "create", "--name", name)
        inner = data.get("result", data)
        workspace_id = (
            inner.get("workspaceId")
            or inner.get("id")
            or inner.get("wikiSpaceId")
        )
        assert workspace_id and isinstance(workspace_id, str), (
            f"create 响应必须含 workspaceId（或同义字段），实际: {data}"
        )

    def test_create_with_description(self, dws):
        """同时传 --name 与 --description 应成功创建。"""
        name = f"CLI_WkDesc_{uuid.uuid4().hex[:6]}"
        dws.run(
            "wiki", "space", "create",
            "--name", name,
            "--description", "集成测试创建的知识库",
        )


class TestWikiSpaceCreateErrors:
    """dws wiki space create — 反向测试"""

    def test_missing_name(self, dws):
        """缺少 --name 应非零退出。"""
        result = dws.run_raw("wiki", "space", "create")
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0
        assert "name" in combined, (
            f"错误输出应提示 --name 缺失: stdout={result.stdout}, stderr={result.stderr}"
        )


# ─────────────────────────────────────────────────────────────
# get
# ─────────────────────────────────────────────────────────────

class TestWikiSpaceGet:
    """dws wiki space get"""

    def test_get_after_create(self, dws, test_workspace_id):
        """通过 workspaceId 查询应成功。"""
        dws.run("wiki", "space", "get", "--workspace", test_workspace_id)


class TestWikiSpaceGetErrors:
    """dws wiki space get — 反向测试"""

    def test_missing_workspace(self, dws):
        """缺少 --workspace 应非零退出。"""
        result = dws.run_raw("wiki", "space", "get")
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0
        assert "workspace" in combined, (
            f"错误输出应提示 --workspace 缺失: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_invalid_workspace(self, dws):
        """无效 workspaceId 应失败。"""
        result = dws.run_raw("wiki", "space", "get", "--workspace", "INVALID_WS_99999")
        assert result.returncode != 0


# ─────────────────────────────────────────────────────────────
# list
# ─────────────────────────────────────────────────────────────

class TestWikiSpaceList:
    """dws wiki space list"""

    def test_list_default(self, dws):
        """不带参数默认 type=orgWikiSpace。"""
        dws.run("wiki", "space", "list")

    def test_list_my_wiki_space(self, dws):
        """--type myWikiSpace 应返回个人空间。"""
        dws.run("wiki", "space", "list", "--type", "myWikiSpace")

    def test_list_org_wiki_space_with_limit(self, dws):
        """--type orgWikiSpace --limit 5 应正常返回。"""
        dws.run(
            "wiki", "space", "list",
            "--type", "orgWikiSpace",
            "--limit", "5",
        )


# ─────────────────────────────────────────────────────────────
# search
# ─────────────────────────────────────────────────────────────

class TestWikiSpaceSearch:
    """dws wiki space search"""

    def test_search_with_keyword(self, dws):
        """传入 --keyword 应正常返回搜索结果。"""
        dws.run("wiki", "space", "search", "--keyword", "测试")

    def test_search_my_wiki_space(self, dws):
        """--type myWikiSpace 时省略 keyword 也应成功。"""
        dws.run("wiki", "space", "search", "--type", "myWikiSpace")

    def test_search_with_limit(self, dws):
        """--keyword + --limit 组合应正常返回。"""
        dws.run(
            "wiki", "space", "search",
            "--keyword", "文档",
            "--limit", "5",
        )


class TestWikiSpaceSearchErrors:
    """dws wiki space search — 反向测试"""

    def test_search_missing_keyword(self, dws):
        """非 myWikiSpace 模式下缺少 --query 应非零退出。"""
        result = dws.run_raw("wiki", "space", "search")
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0
        assert "query" in combined, (
            f"错误输出应提示 --query 缺失: stdout={result.stdout}, stderr={result.stderr}"
        )
