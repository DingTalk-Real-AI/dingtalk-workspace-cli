"""
test_05_permission.py — 文档协作权限测试

Commands tested:
  1. dws doc permission add     —— 添加协作者（仅 USER 类型）
  2. dws doc permission update  —— 更新协作者权限（仅 USER 类型）
  3. dws doc permission list    —— 列出协作者

执行约束:
  - dws doc permission 走 doc MCP server（add_permission / update_permission /
    list_permission 三个工具），需 dws >= 0.2.55；
  - 旧版 CLI 不包含该子命令组，cobra 会以 "unknown command" 退出码 != 0 失败；
  - 当前组织若未授权对应工具，CLI 返回 AUTH_PERMISSION_DENIED，
    根 conftest.py 的 dws.run()/run_raw 已对此自动 pytest.skip 或非零退出。

参数规范（来自 mcp tool schema）:
  - 角色枚举（roleId，必须大写）：MANAGER / EDITOR / DOWNLOADER / READER
    （OWNER 不可通过此接口添加）
  - CLI 仅暴露 USER 成员类型；--user 必填（逗号分隔的 userId 列表）。
    backend schema 上还支持 ORG 类型，但当前 CLI 不暴露 --org 以避免误授权全员。
  - list 不支持游标分页，仅 --max-results (上限 200) + --filter-role 过滤。

正向用例的断言策略:
  - dws.run() 内部已做字段级精确断言（success=True / 无 error 字段，见根 conftest），
    本身即满足"业务字段精确断言"规范，因此正向用例不再叠加无可信源的字段断言。

依赖 fixture:
  - test_doc_node_id（doc/conftest.py）—— 临时文档 nodeId（创建者=当前用户=OWNER）；
  - current_user_id（root conftest.py）—— 当前登录 userId，仅用于反向参数校验
    用例（不会真正打到服务端，只用于凑齐 cobra 必填参数）；
  - other_user_id（root conftest.py）—— 非当前用户的 userId，作为正向 add/update
    的被授权对象。
    OWNER（即 current_user_id 自身）不能通过 add/update 接口再次授权，服务端会
    返回 internalError，因此所有真正打到服务端的正向用例必须使用 other_user_id；
    未设置环境变量 DINGTALK_TEST_OTHER_USER_ID 时该 fixture 会 pytest.skip。
"""

# ─────────────────────────────────────────────────────────────
# add
# ─────────────────────────────────────────────────────────────

class TestDocPermissionAdd:
    """dws doc permission add"""

    def test_add_single_user_reader(self, dws, test_doc_node_id, other_user_id):
        """添加单个协作者并授予 READER 角色（被授权对象必须是非 OWNER 的用户）。"""
        dws.run(
            "doc", "permission", "add",
            "--node", test_doc_node_id,
            "--user", other_user_id,
            "--role", "READER",
        )

    def test_add_multiple_users_same_role(self, dws, test_doc_node_id, other_user_id):
        """逗号分隔的多个 user 应被解析为数组，统一授予 EDITOR。

        注：服务端对同一 userId 的重复条目会去重，因此用 other_user_id,other_user_id
        既能验证数组解析，又不会触发 OWNER 自授权错误。
        """
        dws.run(
            "doc", "permission", "add",
            "--node", test_doc_node_id,
            "--user", f"{other_user_id},{other_user_id}",
            "--role", "EDITOR",
        )

    def test_add_role_lowercase_normalized(self, dws, test_doc_node_id, other_user_id):
        """--role 接收小写时 CLI 应自动 normalize 为大写后透传。"""
        dws.run(
            "doc", "permission", "add",
            "--node", test_doc_node_id,
            "--user", other_user_id,
            "--role", "downloader",
        )

    def test_add_with_workspace(self, dws, test_doc_node_id, other_user_id):
        """--workspace 选填，仅用于辅助构造返回的 docUrl，CLI 应正常透传。"""
        dws.run(
            "doc", "permission", "add",
            "--node", test_doc_node_id,
            "--user", other_user_id,
            "--role", "READER",
            "--workspace", "FAKE_WS_ID",
        )

class TestDocPermissionAddErrors:
    """dws doc permission add — 反向测试"""

    def test_missing_node(self, dws, current_user_id):
        """缺少 --node 时 cobra 应报 'required flag --node not set' 并非零退出。"""
        result = dws.run_raw(
            "doc", "permission", "add",
            "--user", current_user_id,
            "--role", "READER",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0, (
            f"缺少 --node 应非零退出: stdout={result.stdout}, stderr={result.stderr}"
        )
        assert "node" in combined, (
            f"错误输出应提示 --node 缺失: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_missing_role(self, dws, test_doc_node_id, current_user_id):
        """缺少 --role 时 CLI 应非零退出并提示 role 缺失。"""
        result = dws.run_raw(
            "doc", "permission", "add",
            "--node", test_doc_node_id,
            "--user", current_user_id,
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0
        assert "role" in combined, (
            f"错误输出应提示 --role 缺失: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_missing_user(self, dws, test_doc_node_id):
        """缺少 --user 时 cobra 应非零退出并提示 user 缺失（CLI 已不再支持 --org）。"""
        result = dws.run_raw(
            "doc", "permission", "add",
            "--node", test_doc_node_id,
            "--role", "READER",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0
        assert "user" in combined, (
            f"错误输出应提示 --user 缺失: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_org_flag_not_exposed(self, dws, test_doc_node_id, current_user_id):
        """--org 已从 CLI 移除（避免误授权企业全员），传入应被 cobra 拒绝。"""
        result = dws.run_raw(
            "doc", "permission", "add",
            "--node", test_doc_node_id,
            "--user", current_user_id,
            "--role", "READER",
            "--org",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0, (
            f"--org 应被 cobra 拒绝: stdout={result.stdout}, stderr={result.stderr}"
        )
        assert "unknown flag" in combined or "org" in combined

    def test_invalid_node(self, dws, current_user_id):
        """无效 nodeId 时 MCP server 返回业务错误，CLI 非零退出。"""
        result = dws.run_raw(
            "doc", "permission", "add",
            "--node", "INVALID_NODE_99999",
            "--user", current_user_id,
            "--role", "READER",
        )
        assert result.returncode != 0, (
            f"无效 nodeId 应非零退出: stdout={result.stdout}, stderr={result.stderr}"
        )

# ─────────────────────────────────────────────────────────────
# update
# ─────────────────────────────────────────────────────────────

class TestDocPermissionUpdate:
    """dws doc permission update"""

    def test_update_role_after_add(self, dws, test_doc_node_id, other_user_id):
        """先 add 再 update：已存在协作者权限可被更新为 EDITOR。

        注：被操作对象必须是非 OWNER 的用户，否则 update 会被服务端拒绝。
        """
        dws.run(
            "doc", "permission", "add",
            "--node", test_doc_node_id,
            "--user", other_user_id,
            "--role", "READER",
        )
        dws.run(
            "doc", "permission", "update",
            "--node", test_doc_node_id,
            "--user", other_user_id,
            "--role", "EDITOR",
        )

class TestDocPermissionUpdateErrors:
    """dws doc permission update — 反向测试"""

    def test_missing_user(self, dws, test_doc_node_id):
        result = dws.run_raw(
            "doc", "permission", "update",
            "--node", test_doc_node_id,
            "--role", "READER",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0
        assert "user" in combined

    def test_missing_role(self, dws, test_doc_node_id, current_user_id):
        result = dws.run_raw(
            "doc", "permission", "update",
            "--node", test_doc_node_id,
            "--user", current_user_id,
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0
        assert "role" in combined

    def test_invalid_node(self, dws, current_user_id):
        result = dws.run_raw(
            "doc", "permission", "update",
            "--node", "INVALID_NODE_99999",
            "--user", current_user_id,
            "--role", "READER",
        )
        assert result.returncode != 0

# ─────────────────────────────────────────────────────────────
# list
# ─────────────────────────────────────────────────────────────

class TestDocPermissionList:
    """dws doc permission list"""

    def test_list_basic(self, dws, test_doc_node_id):
        """list 命令应返回 success=True 的合法 JSON 响应。"""
        dws.run("doc", "permission", "list", "--node", test_doc_node_id)

    def test_list_with_max_results(self, dws, test_doc_node_id):
        """指定 --max-results 应被 CLI 正确透传为 maxResults。"""
        dws.run(
            "doc", "permission", "list",
            "--node", test_doc_node_id,
            "--max-results", "50",
        )

    def test_list_with_filter_role(self, dws, test_doc_node_id):
        """--filter-role 接受逗号分隔的角色列表，应被透传为 filterRoleIds。"""
        dws.run(
            "doc", "permission", "list",
            "--node", test_doc_node_id,
            "--filter-role", "OWNER,MANAGER,EDITOR",
        )

class TestDocPermissionListErrors:
    """dws doc permission list — 反向测试"""

    def test_missing_node(self, dws):
        """缺少 --node 应非零退出。"""
        result = dws.run_raw("doc", "permission", "list")
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0
        assert "node" in combined, (
            f"错误输出应提示 --node 缺失: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_invalid_node(self, dws):
        """无效 nodeId 应非零退出。"""
        result = dws.run_raw(
            "doc", "permission", "list",
            "--node", "INVALID_NODE_99999",
        )
        assert result.returncode != 0
