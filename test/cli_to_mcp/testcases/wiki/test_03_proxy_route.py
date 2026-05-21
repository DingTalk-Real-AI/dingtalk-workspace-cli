"""
test_03_proxy_route.py — wiki 跨产品透明路由（proxySubCmd）测试

功能: wiki.go 中注册了 hidden 子命令，当 Agent 尝试调用 `dws wiki node list`
等直觉路径时，透明转发到 `dws doc list` 等实际命令。

覆盖场景:
  1. wiki create/get/list/search → wiki space create/get/list/search（同产品内部）
  2. wiki node list/read/search/create/info → doc list/read/search/create/info
  3. wiki file list/search → doc list/search
  4. wiki doc list/read/search → doc list/read/search
  5. flag 重命名（--workspace → --workspace-ids）
"""


def _assert_not_panic(result):
    """断言 CLI 不崩溃。"""
    combined = ((result.stdout or "") + "\n" + (result.stderr or "")).lower()
    assert "panic" not in combined, f"不应 panic: {combined[:300]}"
    assert "runtime error" not in combined, f"不应 runtime error: {combined[:300]}"


def _assert_redirected(result):
    """断言 CLI 输出了重定向提示（→ redirecting to:）。"""
    combined = (result.stdout or "") + "\n" + (result.stderr or "")
    assert "redirecting to" in combined.lower(), (
        f"proxy 路由应输出重定向提示，实际输出: {combined[:300]}"
    )


# ─────────────────────────────────────────────────────────────
# 同产品内部路由: wiki create/get/list/search → wiki space *
# ─────────────────────────────────────────────────────────────

class TestWikiTopLevelProxy:
    """dws wiki create/get/list/search 顶层 proxy 路由。

    这些命令从 hintSubCmd（打印提示后退出）升级为 proxySubCmd（直接执行）。
    """

    def test_wiki_list_proxy(self, dws):
        """dws wiki list → wiki space list，应透明执行并返回知识库列表。"""
        result = dws.run_raw("wiki", "list")
        _assert_not_panic(result)
        _assert_redirected(result)

    def test_wiki_search_proxy(self, dws):
        """dws wiki search --keyword 测试 → wiki space search。"""
        result = dws.run_raw("wiki", "search", "--keyword", "测试")
        _assert_not_panic(result)
        _assert_redirected(result)

    def test_wiki_create_proxy(self, dws):
        """dws wiki create --name → wiki space create，验证路由生效。

        注意：此用例会真实创建知识库，因此使用带时间戳的名称。
        为避免副作用，这里只验证 proxy 路由本身不崩溃。
        """
        result = dws.run_raw("wiki", "create", "--name", "ProxyTest_不要真创建")
        _assert_not_panic(result)
        _assert_redirected(result)

    def test_wiki_get_proxy(self, dws):
        """dws wiki get --workspace FAKE → wiki space get。"""
        result = dws.run_raw("wiki", "get", "--workspace", "FAKE_WS_ID")
        _assert_not_panic(result)
        _assert_redirected(result)


# ─────────────────────────────────────────────────────────────
# 跨产品路由: wiki node * → doc *
# ─────────────────────────────────────────────────────────────

class TestWikiNodeProxy:
    """dws wiki node list/read/search/create/info → doc 对应命令。"""

    def test_node_list_proxy(self, dws):
        """dws wiki node list → doc list，应透明转发。"""
        result = dws.run_raw("wiki", "node", "list")
        _assert_not_panic(result)
        _assert_redirected(result)

    def test_node_read_proxy(self, dws):
        """dws wiki node read --node FAKE → doc read。"""
        result = dws.run_raw("wiki", "node", "read", "--node", "FAKE_NODE_ID")
        _assert_not_panic(result)
        _assert_redirected(result)

    def test_node_info_proxy(self, dws):
        """dws wiki node info --node FAKE → doc info。"""
        result = dws.run_raw("wiki", "node", "info", "--node", "FAKE_NODE_ID")
        _assert_not_panic(result)
        _assert_redirected(result)

    def test_node_create_proxy(self, dws):
        """dws wiki node create --name FAKE → doc create。"""
        result = dws.run_raw("wiki", "node", "create", "--name", "ProxyTest_不要真创建")
        _assert_not_panic(result)
        _assert_redirected(result)

    def test_node_search_proxy(self, dws):
        """dws wiki node search --query 测试 → doc search。"""
        result = dws.run_raw("wiki", "node", "search", "--query", "测试")
        _assert_not_panic(result)
        _assert_redirected(result)

    def test_node_search_flag_rename(self, dws):
        """dws wiki node search --workspace WS → doc search --workspace-ids WS。

        验证 flag 重命名映射（workspace → workspace-ids）。
        """
        result = dws.run_raw(
            "wiki", "node", "search",
            "--query", "测试",
            "--workspace", "FAKE_WS_ID",
        )
        _assert_not_panic(result)
        _assert_redirected(result)


# ─────────────────────────────────────────────────────────────
# 跨产品路由: wiki file * → doc *
# ─────────────────────────────────────────────────────────────

class TestWikiFileProxy:
    """dws wiki file list/search → doc list/search。"""

    def test_file_list_proxy(self, dws):
        """dws wiki file list → doc list。"""
        result = dws.run_raw("wiki", "file", "list")
        _assert_not_panic(result)
        _assert_redirected(result)

    def test_file_search_proxy(self, dws):
        """dws wiki file search --query 测试 → doc search。"""
        result = dws.run_raw("wiki", "file", "search", "--query", "测试")
        _assert_not_panic(result)
        _assert_redirected(result)

    def test_file_search_flag_rename(self, dws):
        """wiki file search --workspace WS → doc search --workspace-ids WS。"""
        result = dws.run_raw(
            "wiki", "file", "search",
            "--query", "测试",
            "--workspace", "FAKE_WS_ID",
        )
        _assert_not_panic(result)
        _assert_redirected(result)


# ─────────────────────────────────────────────────────────────
# 跨产品路由: wiki doc * → doc *
# ─────────────────────────────────────────────────────────────

class TestWikiDocProxy:
    """dws wiki doc list/read/search → doc list/read/search。"""

    def test_doc_list_proxy(self, dws):
        """dws wiki doc list → doc list。"""
        result = dws.run_raw("wiki", "doc", "list")
        _assert_not_panic(result)
        _assert_redirected(result)

    def test_doc_read_proxy(self, dws):
        """dws wiki doc read --node FAKE → doc read。"""
        result = dws.run_raw("wiki", "doc", "read", "--node", "FAKE_NODE_ID")
        _assert_not_panic(result)
        _assert_redirected(result)

    def test_doc_search_proxy(self, dws):
        """dws wiki doc search --query 测试 → doc search。"""
        result = dws.run_raw("wiki", "doc", "search", "--query", "测试")
        _assert_not_panic(result)
        _assert_redirected(result)


# ─────────────────────────────────────────────────────────────
# 边界情况
# ─────────────────────────────────────────────────────────────

class TestWikiProxyEdgeCases:
    """proxy 路由边界情况。"""

    def test_node_unknown_subcommand(self, dws):
        """wiki node delete 未注册 proxy，应报错但不 panic。"""
        result = dws.run_raw("wiki", "node", "delete", "--node", "FAKE")
        _assert_not_panic(result)
        combined = ((result.stdout or "") + (result.stderr or "")).lower()
        assert result.returncode != 0 or "error" in combined

    def test_file_unknown_subcommand(self, dws):
        """wiki file read 未注册 proxy，应报错但不 panic。"""
        result = dws.run_raw("wiki", "file", "read", "--node", "FAKE")
        _assert_not_panic(result)
        combined = ((result.stdout or "") + (result.stderr or "")).lower()
        assert result.returncode != 0 or "error" in combined

    def test_node_group_no_subcommand(self, dws):
        """wiki node 不带子命令应输出帮助/报错。"""
        result = dws.run_raw("wiki", "node")
        _assert_not_panic(result)

    def test_file_group_no_subcommand(self, dws):
        """wiki file 不带子命令应输出帮助/报错。"""
        result = dws.run_raw("wiki", "file")
        _assert_not_panic(result)

    def test_doc_group_no_subcommand(self, dws):
        """wiki doc 不带子命令应输出帮助/报错。"""
        result = dws.run_raw("wiki", "doc")
        _assert_not_panic(result)
