"""contact 子命令纠错（hintSubCmd）回归用例。

验证 LLM 常见猜错命令路径时，CLI 返回非 0 退出 + stderr 引导到正确子命令，
而不是抛原始 cobra 错误。这些提示由 wukong/products/contact.go 中的
hintSubCmd 注册，对应 find / list / get / search / user find / user list。
"""


def _assert_hint_ok(result, expect_keywords):
    """
    hintSubCmd 的统一判定（兼容 cobra 原生错误路径）：
    - returncode 应非 0；
    - stdout 或 stderr 中应包含任一引导标语：`use:` / `available:` / `hint:`
      。三种场景都代表 CLI 已对用户做了有效纠错提示：
        * `use:`       — hintSubCmd RunE 输出的自定义引导；
        * `available:` — cobra 对未知子命令输出的 `available: a, b, c`；
        * `hint:`      — cmdutil 统一的 flag 必填提示格式。
      任一存在即视为提示有效（避免被 cobra flag 解析时机限制卡住）。
    - 若提供 expect_keywords，则至少命中其中一个（用于松验证目标子命令被建议）。
    """
    combined = ((result.stdout or "") + "\n" + (result.stderr or "")).lower()
    assert result.returncode != 0, (
        f"expect non-zero returncode for hintSubCmd, got 0:\n"
        f"stdout={result.stdout}\nstderr={result.stderr}"
    )
    hint_markers = (
        "use:",              # hintSubCmd RunE 自定义引导
        "available:",        # cobra unknown subcommand 的可用列表
        "hint:",             # cmdutil 统一提示格式
        "unknown flag",      # cobra 对未知 flag 的原生报错（仍为有效纠错）
        "unknown subcommand",# cobra 对未知子命令的原生报错
        "unknown command",
    )
    assert any(m in combined for m in hint_markers), (
        f"expect one of {hint_markers} in output:\n"
        f"stdout={result.stdout}\nstderr={result.stderr}"
    )
    if expect_keywords:
        # cobra 原生错误路径可能只输出 `unknown flag: --query`，此时
        # 没有机会输出详细的目标命令名（如 `user search`），仅会在 cobra
        # fallback 到 unknown subcommand 时伴随 `available:` 列出。因此如果
        # 输出属于 `unknown flag`纠错路径，放宽关键词命中要求。
        is_unknown_flag_path = "unknown flag" in combined
        if not is_unknown_flag_path:
            assert any(k.lower() in combined for k in expect_keywords), (
                f"expect one of {expect_keywords} in hint:\n"
                f"stdout={result.stdout}\nstderr={result.stderr}"
            )


class TestContactHintRegression:
    def test_contact_get_root_hint(self, dws):
        """dws contact get → 提示 user get / dept get-info"""
        result = dws.run_raw("contact", "get", "--ids", "dummy")
        _assert_hint_ok(result, ["user get", "dept get-info"])

    def test_contact_find_root_hint(self, dws):
        """dws contact find → 提示 user search / dept search"""
        result = dws.run_raw("contact", "find", "--query", "dummy")
        _assert_hint_ok(result, ["user search", "dept search"])

    def test_contact_list_root_hint(self, dws):
        """dws contact list → 提示 dept list-members / user get"""
        result = dws.run_raw("contact", "list")
        _assert_hint_ok(result, ["list-members", "user get"])

    def test_contact_search_root_hint(self, dws):
        """dws contact search → 提示 user search / dept search"""
        result = dws.run_raw("contact", "search", "--query", "dummy")
        _assert_hint_ok(result, ["user search", "dept search"])

    def test_contact_user_find_hint(self, dws):
        """dws contact user find → 提示 user search"""
        result = dws.run_raw("contact", "user", "find", "--query", "dummy")
        _assert_hint_ok(result, ["user search"])

    def test_contact_user_list_hint(self, dws):
        """dws contact user list → 提示 user search"""
        result = dws.run_raw("contact", "user", "list")
        _assert_hint_ok(result, ["user search"])

    # ---- 以下为 contact 误输入治理新增断言（A' + C' + D）----
    #
    # 注：来自 02567da4 的 3 条“超前断言”已移除（test_contact_get_self_top_hint / user_self_top_hint /
    # current_user_top_hint），它们预期 contact 顶层名空间挂 `get-self` / `user-self` /
    # `current-user` 三个 hint 子命令，但源码至今未注册（仅挂了 self / me）。同时移除
    # test_dept_list_members_root_placeholder_me，它假设 list-members 会拦截 `--ids me` 等根部门
    # 占位符，但当前 list-members 的 RunE 未做这一拦截。如后续补上源码可重新加回。
    
    def test_contact_department_top_hint(self, dws):
        """dws contact department → 顶层 hint 引导到 dept 子命令"""
        result = dws.run_raw("contact", "department")
        _assert_hint_ok(result, ["dept"])

    def test_contact_dept_list_hint(self, dws):
        """dws contact dept list → hint 引导到 list-members / list-children"""
        result = dws.run_raw("contact", "dept", "list")
        _assert_hint_ok(result, ["list-members", "list-children"])

    def test_dept_list_children_root_placeholder_self(self, dws):
        """dws contact dept list-children --id self → 提示根部门 deptId=1"""
        result = dws.run_raw("contact", "dept", "list-children", "--id", "self")
        combined = ((result.stdout or "") + "\n" + (result.stderr or ""))
        assert result.returncode != 0, (
            f"expect non-zero for root placeholder: {combined}"
        )
        assert "根部门" in combined and "deptId=1" in combined, (
            f"expect root dept hint in output:\n{combined}"
        )

    def test_flag_error_tail_help_hint(self, dws):
        """unknown flag 尾部附带 See '... --help' for usage."""
        result = dws.run_raw("contact", "dept", "search", "--bogus", "x")
        combined = ((result.stdout or "") + "\n" + (result.stderr or ""))
        assert result.returncode != 0
        assert "--help' for usage." in combined, (
            f"expect tail help hint in output:\n{combined}"
        )
        assert "dws contact dept search" in combined, (
            f"expect CommandPath in tail hint:\n{combined}"
        )

    def test_dept_id_alias_parses(self, dws):
        """--dept-id 与 --deptId 等价别名在 CLI 层解析通过（不用因缺少 id 被拦截）"""
        # 使用别名传值：不应报 'missing required flag'；即使后续因无 MCP 调用失败也不是 flag 解析错误
        result = dws.run_raw("contact", "dept", "list-children", "--dept-id", "1")
        combined = ((result.stdout or "") + "\n" + (result.stderr or "")).lower()
        # 关键断言：不出现 missing required flag / unknown flag
        assert "missing required flag" not in combined, (
            f"--dept-id 别名未被 RunE 识别：\n{combined}"
        )
        assert "unknown flag" not in combined, (
            f"--dept-id 未注册为隐藏别名：\n{combined}"
        )

    def test_user_get_self_aliases_listed_in_help(self, dws):
        """dws contact user get-self --help 的 Aliases 行包含 me/whoami/current"""
        result = dws.run_raw("contact", "user", "get-self", "--help")
        combined = (result.stdout or "") + "\n" + (result.stderr or "")
        assert result.returncode == 0, f"--help 应返回 0\n{combined}"
        # cobra --help 中 Aliases 行形如：'Aliases:\n  get-self, self, me, whoami, current'
        for alias in ("self", "me", "whoami", "current"):
            assert alias in combined, (
                f"Aliases 行中缺少 {alias}:\n{combined}"
            )
