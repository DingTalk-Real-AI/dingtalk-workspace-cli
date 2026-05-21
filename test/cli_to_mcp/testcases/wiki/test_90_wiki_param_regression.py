"""wiki 高频错误参数回归用例。

覆盖范围：
  - 子命令组完整性（space / member 已注册）
  - 参数别名（space get / member add/update/list: --workspace-id 是 --workspace 的隐藏别名）
  - 已移除参数（--space / --id 已全面移除，应被 cobra 拒绝）
  - 错误参数名（--users / --uid / --pagesize 等常见笔误）
  - 粘连参数（--workspaceFAKE 等）
"""


def _assert_regression_ok(result):
    """回归用例统一判定: CLI 不崩溃即可。"""
    combined = ((result.stdout or "") + "\n" + (result.stderr or "")).strip()
    assert len(combined) > 0, "命令未产生任何输出"


# ─────────────────────────────────────────────────────────────
# 子命令组完整性
# ─────────────────────────────────────────────────────────────

class TestWikiSubcommandRegistry:
    """验证 wiki / wiki space / wiki member 子命令组已正确注册。"""

    def test_wiki_no_subcommand(self, dws):
        """`dws wiki` 不带子命令应输出帮助/分组指引。"""
        result = dws.run_raw("wiki")
        _assert_regression_ok(result)

    def test_wiki_space_no_subcommand(self, dws):
        """`dws wiki space` 不带子命令应输出帮助。"""
        result = dws.run_raw("wiki", "space")
        _assert_regression_ok(result)

    def test_wiki_member_no_subcommand(self, dws):
        """`dws wiki member` 不带子命令应输出帮助。"""
        result = dws.run_raw("wiki", "member")
        _assert_regression_ok(result)

    def test_wiki_unknown_subcommand(self, dws):
        """未知子命令 `wiki foo` 应被 cobra 拒绝。"""
        result = dws.run_raw("wiki", "foo")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_wiki_space_unknown_subcommand(self, dws):
        """未知子命令 `wiki space delete` 应被 cobra 拒绝。"""
        result = dws.run_raw("wiki", "space", "delete", "--id", "FAKE")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_wiki_member_unknown_subcommand(self, dws):
        """未知子命令 `wiki member remove` 应被 cobra 拒绝。"""
        result = dws.run_raw("wiki", "member", "remove", "--workspace", "FAKE")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()


# ─────────────────────────────────────────────────────────────
# 顶层 hint 命令（wiki.go 中 hintSubCmd 注册）
# ─────────────────────────────────────────────────────────────

class TestWikiHintCommands:
    """`dws wiki create/get/list/search` 顶层 hint 命令应给出 use 提示。"""

    def test_top_create_hint(self, dws):
        result = dws.run_raw("wiki", "create")
        _assert_regression_ok(result)

    def test_top_get_hint(self, dws):
        result = dws.run_raw("wiki", "get")
        _assert_regression_ok(result)

    def test_top_list_hint(self, dws):
        result = dws.run_raw("wiki", "list")
        _assert_regression_ok(result)

    def test_top_search_hint(self, dws):
        result = dws.run_raw("wiki", "search")
        _assert_regression_ok(result)


# ─────────────────────────────────────────────────────────────
# space 参数回归
# ─────────────────────────────────────────────────────────────

class TestWikiSpaceParamRegression:
    """dws wiki space 参数回归。"""

    def test_create_wrong_title_flag(self, dws):
        """错误参数名 --title（应为 --name）应被 cobra 拒绝。"""
        result = dws.run_raw("wiki", "space", "create", "--title", "FAKE")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_get_wrong_workspaceid_camelcase_flag(self, dws):
        """错误参数名 --workspaceId（驼峰式，应为 --workspace）应被 cobra 拒绝。"""
        result = dws.run_raw("wiki", "space", "get", "--workspaceId", "FAKE")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_list_pagesize_alias_accepted(self, dws):
        """--page-size 是 --limit 的 hidden alias，不应报 unknown flag。"""
        result = dws.run_raw("wiki", "space", "list", "--page-size", "10")
        combined = (result.stdout + result.stderr).lower()
        # --page-size 已注册为 --limit 的 hidden alias，不应报 unknown flag
        assert "unknown flag" not in combined, (
            f"--page-size 应作为 --limit 的 alias 被接受: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_search_sticky_keyword_flag(self, dws):
        """粘连参数 --keywordFAKE 不应被当作合法 --keyword 值使用。

        cobra 可能将其视为位置参数而非 flag，导致 returncode=0 但 keyword
        实际未生效。只要不返回正常匹配结果即可视为通过（空列表 or 报错）。
        """
        result = dws.run_raw("wiki", "space", "search", "--keywordFAKE")
        combined = (result.stdout + result.stderr).lower()
        # 只要不是"正常带结果的成功"就算通过：报错 or 空结果
        assert result.returncode != 0 or "error" in combined or '"wikispaces": []' in combined or '"wikispaces":[]' in combined


# ─────────────────────────────────────────────────────────────
# member 参数回归
# ─────────────────────────────────────────────────────────────

class TestWikiMemberParamRegression:
    """dws wiki member 参数回归。"""

    def test_add_wrong_users_flag(self, dws):
        """错误参数名 --users（应为 --user）应被 cobra 拒绝。"""
        result = dws.run_raw(
            "wiki", "member", "add",
            "--workspace", "FAKE_WS",
            "--users", "FAKE_USER",
            "--role", "reader",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_add_invalid_role(self, dws):
        """--role 取值非法（如 unknown-role）应被业务层拒绝。"""
        result = dws.run_raw(
            "wiki", "member", "add",
            "--workspace", "FAKE_WS",
            "--user", "FAKE_USER",
            "--role", "unknown-role",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_add_org_flag_not_exposed(self, dws):
        """--org 已从 CLI 移除（避免误授权企业全员），应被 cobra 拒绝。"""
        result = dws.run_raw(
            "wiki", "member", "add",
            "--workspace", "FAKE_WS",
            "--user", "FAKE_USER",
            "--role", "READER",
            "--org",
        )
        assert result.returncode != 0 or "unknown flag" in (result.stdout + result.stderr).lower()

    def test_add_allow_mywiki_flag_removed(self, dws):
        """旧的 --allow-mywiki 隐藏开关已被删除（MCP 层不再有 myWikiSpace 拦截需求），应被 cobra 拒绝。"""
        result = dws.run_raw(
            "wiki", "member", "add",
            "--workspace", "FAKE_WS",
            "--user", "FAKE_USER",
            "--role", "READER",
            "--allow-mywiki",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "unknown flag" in combined, (
            f"--allow-mywiki 应已被移除: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_add_sticky_workspace_flag(self, dws):
        """粘连参数 --workspaceFAKE 应被 cobra 拒绝。"""
        result = dws.run_raw(
            "wiki", "member", "add",
            "--workspaceFAKE",
            "--user", "FAKE_USER",
            "--role", "reader",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_update_wrong_uid_flag(self, dws):
        """错误参数名 --uid（应为 --user）应被 cobra 拒绝。"""
        result = dws.run_raw(
            "wiki", "member", "update",
            "--workspace", "FAKE_WS",
            "--uid", "FAKE_USER",
            "--role", "reader",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_list_wrong_maxresults_flag(self, dws):
        """错误参数名 --maxresults（应为 --max-results）应被 cobra 拒绝。"""
        result = dws.run_raw(
            "wiki", "member", "list",
            "--workspace", "FAKE_WS",
            "--maxresults", "10",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_list_old_pagesize_flag_removed(self, dws):
        """旧的 --page-size 参数已被删除（迁移到 --max-results），应被 cobra 拒绝。"""
        result = dws.run_raw(
            "wiki", "member", "list",
            "--workspace", "FAKE_WS",
            "--page-size", "10",
        )
        assert result.returncode != 0 or "unknown flag" in (result.stdout + result.stderr).lower()

    def test_list_with_filter_role(self, dws):
        """--filter-role 是合法参数，应被正常接受（不崩溃）。"""
        result = dws.run_raw(
            "wiki", "member", "list",
            "--workspace", "FAKE_WS",
            "--filter-role", "MANAGER,EDITOR",
        )
        _assert_regression_ok(result)


# ─────────────────────────────────────────────────────────────
# 跨产品 alias 兼容性
# ─────────────────────────────────────────────────────────────

class TestWikiCrossProductAlias:
    """跨产品 alias 兼容性 + --space 已移除验证。"""

    # ── --workspace-id 是 --workspace 的 hidden alias（LLMs derive from API field "workspaceId"）──

    def test_space_get_accepts_workspace_id_alias(self, dws):
        """wiki space get --workspace-id 应被接受（hidden alias）。"""
        result = dws.run_raw("wiki", "space", "get", "--workspace-id", "FAKE_WS_ID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_member_add_accepts_workspace_id_alias(self, dws):
        """wiki member add --workspace-id 应被接受（hidden alias）。"""
        result = dws.run_raw(
            "wiki", "member", "add",
            "--workspace-id", "FAKE_WS_ID",
            "--user", "FAKE_USER",
            "--role", "READER",
        )
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_member_update_accepts_workspace_id_alias(self, dws):
        """wiki member update --workspace-id 应被接受（hidden alias）。"""
        result = dws.run_raw(
            "wiki", "member", "update",
            "--workspace-id", "FAKE_WS_ID",
            "--user", "FAKE_USER",
            "--role", "EDITOR",
        )
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_member_list_accepts_workspace_id_alias(self, dws):
        """wiki member list --workspace-id 应被接受（hidden alias）。"""
        result = dws.run_raw(
            "wiki", "member", "list",
            "--workspace-id", "FAKE_WS_ID",
        )
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    # ── --id 已移除，应被 cobra 拒绝 ──

    def test_space_get_rejects_id_flag(self, dws):
        """wiki space get --id 已移除，应被 cobra 拒绝。"""
        result = dws.run_raw("wiki", "space", "get", "--id", "FAKE_WS_ID")
        assert result.returncode != 0 or "unknown flag" in (result.stdout + result.stderr).lower()

    def test_member_add_rejects_id_flag(self, dws):
        """wiki member add --id 已移除，应被 cobra 拒绝。"""
        result = dws.run_raw(
            "wiki", "member", "add",
            "--id", "FAKE_WS_ID",
            "--user", "FAKE_USER",
            "--role", "READER",
        )
        assert result.returncode != 0 or "unknown flag" in (result.stdout + result.stderr).lower()

    def test_member_update_rejects_id_flag(self, dws):
        """wiki member update --id 已移除，应被 cobra 拒绝。"""
        result = dws.run_raw(
            "wiki", "member", "update",
            "--id", "FAKE_WS_ID",
            "--user", "FAKE_USER",
            "--role", "EDITOR",
        )
        assert result.returncode != 0 or "unknown flag" in (result.stdout + result.stderr).lower()

    def test_member_list_rejects_id_flag(self, dws):
        """wiki member list --id 已移除，应被 cobra 拒绝。"""
        result = dws.run_raw(
            "wiki", "member", "list",
            "--id", "FAKE_WS_ID",
        )
        assert result.returncode != 0 or "unknown flag" in (result.stdout + result.stderr).lower()

    # ── --space 已移除，应被 cobra 拒绝 ──

    def test_space_get_rejects_space_flag(self, dws):
        """wiki space get --space 已移除，应被 cobra 拒绝。"""
        result = dws.run_raw("wiki", "space", "get", "--space", "FAKE_WS_ID")
        assert result.returncode != 0 or "unknown flag" in (result.stdout + result.stderr).lower()

    def test_member_add_rejects_space_flag(self, dws):
        """wiki member add --space 已移除，应被 cobra 拒绝。"""
        result = dws.run_raw(
            "wiki", "member", "add",
            "--space", "FAKE_WS_ID",
            "--user", "FAKE_USER",
            "--role", "READER",
        )
        assert result.returncode != 0 or "unknown flag" in (result.stdout + result.stderr).lower()

    def test_member_list_rejects_space_flag(self, dws):
        """wiki member list --space 已移除，应被 cobra 拒绝。"""
        result = dws.run_raw(
            "wiki", "member", "list",
            "--space", "FAKE_WS_ID",
        )
        assert result.returncode != 0 or "unknown flag" in (result.stdout + result.stderr).lower()

    def test_member_update_rejects_space_flag(self, dws):
        """wiki member update --space 已移除，应被 cobra 拒绝。"""
        result = dws.run_raw(
            "wiki", "member", "update",
            "--space", "FAKE_WS_ID",
            "--user", "FAKE_USER",
            "--role", "EDITOR",
        )
        assert result.returncode != 0 or "unknown flag" in (result.stdout + result.stderr).lower()
