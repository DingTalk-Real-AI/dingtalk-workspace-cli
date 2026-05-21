"""
test_02_member.py — 钉钉知识库成员管理测试

Commands tested:
  1. dws wiki member add     —— 添加成员（仅 USER 类型）
  2. dws wiki member update  —— 更新成员权限（仅 USER 类型）
  3. dws wiki member list    —— 查询成员列表

执行约束:
  - dws wiki member 走 wiki MCP server（add_member / update_member / list_member
    三个工具），需 dws >= 0.2.55；
  - 旧版本由 wiki/conftest.py 的产品级 skip 兜底；
  - 当前组织若未授权对应工具，dws.run() 会自动 pytest.skip。

参数规范（来自最新 mcp tool schema）:
  - add/update_member 入参为扁平的 `userIds: []string` + `roleId: string`(单值)；
    多个用户共用同一角色，单次最多 30 个 userId。
  - 角色枚举（roleId，必须大写）：MANAGER / EDITOR / DOWNLOADER / READER
    （OWNER 不可通过此接口添加 / 变更）。
  - 仅支持 USER 类型；ORG 类授权已在 MCP 网关层屏蔽，dws CLI 也不再暴露 --org。
  - list 不支持游标分页，仅 --max-results (上限 200) + --filter-role 过滤。
  - member 子命令的 primary flag 为 --workspace（知识库标识），
    --workspace-id 作为 hidden alias 保留兼容（LLMs 从 API 返回字段 workspaceId 推导）。

正向用例的断言策略:
  - dws.run() 内部已做字段级精确断言（success=True / 无 error 字段）；
  - 不再叠加无可信源的字段断言。

依赖 fixture:
  - test_workspace_id（wiki/conftest.py）—— 临时知识库 workspaceId（创建者=当前用户=OWNER）；
  - current_user_id（root conftest.py）—— 当前登录 userId；
  - other_user_id（root conftest.py）—— 非当前用户的 userId，作为被授权对象。
    OWNER 不可通过 add/update 再次授权，因此正向用例必须用 other_user_id；
    未设置 DINGTALK_TEST_OTHER_USER_ID 时该 fixture 会 pytest.skip。
"""

# ─────────────────────────────────────────────────────────────
# add
# ─────────────────────────────────────────────────────────────

class TestWikiMemberAdd:
    """dws wiki member add"""

    def test_add_single_user_reader(self, dws, test_workspace_id, other_user_id):
        """添加单个成员并授予 READER 角色（被授权人必须是非 OWNER 的用户）。"""
        dws.run(
            "wiki", "member", "add",
            "--workspace", test_workspace_id,
            "--user", other_user_id,
            "--role", "READER",
        )

    def test_add_multiple_users_same_role(self, dws, test_workspace_id, other_user_id):
        """逗号分隔的多个 user 应被解析为数组，统一授予 EDITOR。

        注：服务端对同一 userId 的重复条目会去重，因此用 other_user_id,other_user_id
        既能验证数组解析，又不会触发 OWNER 自授权错误。
        """
        dws.run(
            "wiki", "member", "add",
            "--workspace", test_workspace_id,
            "--user", f"{other_user_id},{other_user_id}",
            "--role", "EDITOR",
        )

    def test_add_role_lowercase_normalized(self, dws, test_workspace_id, other_user_id):
        """--role 接收小写时 CLI 应自动 normalize 为大写后透传。"""
        dws.run(
            "wiki", "member", "add",
            "--workspace", test_workspace_id,
            "--user", other_user_id,
            "--role", "downloader",
        )

    def test_add_with_workspace_id_alias(self, dws, test_workspace_id, other_user_id):
        """--workspace-id 是 --workspace 的隐藏别名（LLMs derive from API field workspaceId）。"""
        dws.run(
            "wiki", "member", "add",
            "--workspace-id", test_workspace_id,
            "--user", other_user_id,
            "--role", "MANAGER",
        )

class TestWikiMemberAddErrors:
    """dws wiki member add — 反向测试"""

    def test_missing_workspace(self, dws, current_user_id):
        """缺少 --workspace（且无任何别名）应非零退出。"""
        result = dws.run_raw(
            "wiki", "member", "add",
            "--user", current_user_id,
            "--role", "READER",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0
        # 错误信息应提示 --workspace 缺失
        assert "workspace" in combined, (
            f"错误输出应提示 workspace 缺失: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_missing_user(self, dws, test_workspace_id):
        """缺少 --user 时 cobra 应非零退出并提示 user 缺失（CLI 已不再支持 --org）。"""
        result = dws.run_raw(
            "wiki", "member", "add",
            "--workspace", test_workspace_id,
            "--role", "READER",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0
        assert "user" in combined, (
            f"错误输出应提示 --user 缺失: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_org_flag_not_exposed(self, dws, test_workspace_id, current_user_id):
        """--org 已从 CLI 移除（避免误授权企业全员），传入应被 cobra 拒绝。"""
        result = dws.run_raw(
            "wiki", "member", "add",
            "--workspace", test_workspace_id,
            "--user", current_user_id,
            "--role", "READER",
            "--org",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0, (
            f"--org 应被 cobra 拒绝: stdout={result.stdout}, stderr={result.stderr}"
        )
        assert "unknown flag" in combined or "org" in combined

    def test_missing_role(self, dws, test_workspace_id, current_user_id):
        """缺少 --role 应非零退出。"""
        result = dws.run_raw(
            "wiki", "member", "add",
            "--workspace", test_workspace_id,
            "--user", current_user_id,
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0
        assert "role" in combined, (
            f"错误输出应提示 --role 缺失: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_invalid_workspace(self, dws, current_user_id):
        """无效 workspaceId 应失败。"""
        result = dws.run_raw(
            "wiki", "member", "add",
            "--workspace", "INVALID_WS_99999",
            "--user", current_user_id,
            "--role", "READER",
        )
        assert result.returncode != 0

# ─────────────────────────────────────────────────────────────
# update
# ─────────────────────────────────────────────────────────────

class TestWikiMemberUpdate:
    """dws wiki member update"""

    def test_update_role_after_add(self, dws, test_workspace_id, other_user_id):
        """先 add 再 update：已存在成员的权限可被更新为 EDITOR。

        注：被操作对象必须是非 OWNER 的用户，否则 update 会被服务端拒绝。
        """
        dws.run(
            "wiki", "member", "add",
            "--workspace", test_workspace_id,
            "--user", other_user_id,
            "--role", "READER",
        )
        dws.run(
            "wiki", "member", "update",
            "--workspace", test_workspace_id,
            "--user", other_user_id,
            "--role", "EDITOR",
        )

class TestWikiMemberUpdateErrors:
    """dws wiki member update — 反向测试"""

    def test_missing_user(self, dws, test_workspace_id):
        result = dws.run_raw(
            "wiki", "member", "update",
            "--workspace", test_workspace_id,
            "--role", "READER",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0
        assert "user" in combined

    def test_missing_role(self, dws, test_workspace_id, current_user_id):
        result = dws.run_raw(
            "wiki", "member", "update",
            "--workspace", test_workspace_id,
            "--user", current_user_id,
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0
        assert "role" in combined

    def test_invalid_workspace(self, dws, current_user_id):
        result = dws.run_raw(
            "wiki", "member", "update",
            "--workspace", "INVALID_WS_99999",
            "--user", current_user_id,
            "--role", "READER",
        )
        assert result.returncode != 0

# ─────────────────────────────────────────────────────────────
# list
# ─────────────────────────────────────────────────────────────

class TestWikiMemberList:
    """dws wiki member list"""

    def test_list_basic(self, dws, test_workspace_id):
        """list 命令应返回 success=True 的合法 JSON 响应。"""
        dws.run("wiki", "member", "list", "--workspace", test_workspace_id)

    def test_list_with_max_results(self, dws, test_workspace_id):
        """指定 --max-results 应被 CLI 正确透传为 maxResults。"""
        dws.run(
            "wiki", "member", "list",
            "--workspace", test_workspace_id,
            "--max-results", "50",
        )

    def test_list_with_filter_role(self, dws, test_workspace_id):
        """--filter-role 接受逗号分隔的角色列表，应被透传为 filterRoleIds。"""
        dws.run(
            "wiki", "member", "list",
            "--workspace", test_workspace_id,
            "--filter-role", "MANAGER,EDITOR",
        )

    def test_list_alias_ls(self, dws, test_workspace_id):
        """list 有 alias 'ls'（见 wiki.go memberListCmd.Aliases）。"""
        dws.run("wiki", "member", "ls", "--workspace", test_workspace_id)

class TestWikiMemberListErrors:
    """dws wiki member list — 反向测试"""

    def test_missing_workspace(self, dws):
        """缺少 --workspace 应非零退出。"""
        result = dws.run_raw("wiki", "member", "list")
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0
        assert "workspace" in combined, (
            f"错误输出应提示 workspace 缺失: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_invalid_workspace(self, dws):
        result = dws.run_raw(
            "wiki", "member", "list",
            "--workspace", "INVALID_WS_99999",
        )
        assert result.returncode != 0
