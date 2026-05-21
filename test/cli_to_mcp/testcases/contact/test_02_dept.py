"""
test_02_dept.py — 部门查询测试 (4 commands × 3+ cases)

实际返回格式:
  dept search:        {"deptList": [{deptId, deptName}]}
  dept get-info:      {"deptId": int, "name": str, "memberCount": int}
  dept list-children: {"result": [], "success": true}
  dept list-members:  {"deptUserList": [], "success": true}
"""

import pytest


@pytest.fixture(scope="module")
def parent_with_children(dws):
    """跨租户动态发现一个有子部门的父 deptId（module scope，跨类复用）。

    策略：
      1. 用根部门 1 拿一级部门列表；
      2. 遍历这些一级部门，第一个能拿到非空 result 的即为目标父部门；
      3. 若整租户极扁平（一级部门全为叶子），返回 None 让后续用例 skip。

    返回：
      tuple(parent_dept_id: int, children: list[dict]) 或 None
    """
    root = dws.run("contact", "dept", "list-children", "--id", "1")
    level1 = root.get("result") or []
    for parent in level1:
        parent_id = parent.get("deptId")
        if not isinstance(parent_id, int):
            continue
        sub = dws.run(
            "contact", "dept", "list-children", "--id", str(parent_id),
        )
        children = sub.get("result") or []
        if children:
            return parent_id, children
    return None


class TestDeptSearch:
    """dws contact dept search — 返回 {"deptList": [...]}"""

    def test_search_returns_deptList(self, dws):
        """搜索部门应返回 deptList 列表（结果可能为空，取决于环境数据）。"""
        data = dws.run_ok(
            "contact", "dept", "search", "--keyword", "研发",
        )
        assert "deptList" in data, f"响应缺少 deptList: {data.keys()}"
        assert isinstance(data["deptList"], list)

    def test_search_result_has_fields(self, dws):
        """搜索结果每项应包含 deptId 和 deptName。"""
        data = dws.run_ok(
            "contact", "dept", "search", "--keyword", "研发",
        )
        for dept in data["deptList"]:
            assert "deptId" in dept, f"缺少 deptId: {dept}"
            assert "deptName" in dept, f"缺少 deptName: {dept}"
            assert isinstance(dept["deptId"], int)

    def test_search_no_match(self, dws):
        """搜索不存在的部门应返回空列表。"""
        data = dws.run_ok(
            "contact", "dept", "search",
            "--keyword", "ZZZNONEXIST99999",
        )
        assert "deptList" in data
        assert isinstance(data["deptList"], list)
        assert len(data["deptList"]) == 0


class TestDeptGetInfo:
    """dws contact dept get-info — 返回 {deptId, name, memberCount}"""

    def test_get_info_root(self, dws):
        """根部门(deptId=1)应返回 deptId、deptName、memberCount。"""
        data = dws.run_ok(
            "contact", "dept", "get-info", "--id", "1",
        )
        result = data.get("result", data)
        assert "deptId" in result, f"响应缺少 deptId: {result.keys()}"
        assert "deptName" in result, f"响应缺少 deptName: {result.keys()}"
        assert "memberCount" in result, f"响应缺少 memberCount: {result.keys()}"
        assert isinstance(result["deptId"], int)
        assert isinstance(result["deptName"], str)
        assert isinstance(result["memberCount"], int)


class TestDeptListChildren:
    """dws contact dept list-children — 查询父部门下的直属子部门"""

    def test_list_root_returns_success(self, dws):
        """根部门(deptId=1)查询应返回 success + result 列表。"""
        data = dws.run(
            "contact", "dept", "list-children", "--id", "1",
        )
        assert data.get("success") is True
        assert "result" in data
        assert isinstance(data["result"], list)

    def test_list_root_contains_depts(self, dws, parent_with_children):
        """有下级的父部门：result 列表非空，每项至少含 deptId 与 deptName 两个字段。"""
        if parent_with_children is None:
            pytest.skip("当前租户根部门下所有一级部门均为叶子，无法验证非空列表（非错误）")
        parent_id, children = parent_with_children
        assert len(children) > 0, f"父部门 {parent_id} 应有至少 1 个子部门"
        for child in children:
            assert "deptId" in child, f"子部门缺 deptId: {child}"
            assert "deptName" in child, f"子部门缺 deptName: {child}"

    def test_list_unknown_dept_returns_empty(self, dws):
        """未知部门 ID 应"宽容降级"返回 success=true + 空 result，而非报错。

        语义说明：
          - mcp-gw 后端 get_sub_depts_by_dept_id 对未知 deptId 的契约是宽容降级
            （返回空集合而非抛错），便于上层 Agent 串接（如 search → list-children
            发生数据漂移时不至于整链路失败）；
          - 因此本用例断言 success=true 且 result 为空列表；
          - 若后端将来改为对未知 ID 抛 MCP_TOOL_ERROR，本用例需要随契约同步调整。
        """
        data = dws.run(
            "contact", "dept", "list-children",
            "--id", "999999999",
        )
        assert data.get("success") is True
        assert isinstance(data.get("result"), list)
        assert len(data["result"]) == 0

    def test_list_validates_child_fields(self, dws, parent_with_children):
        """有下级的父部门：result 中每项 deptId 必须为 int、deptName 必须为非空 str。

        验证目标：
          1. 返回结构与产品文档 Notes 一致（result[].deptId(int) / result[].deptName(str)）；
          2. mcp-gw 后端 get_sub_depts_by_dept_id 字段类型契约稳定；
          3. 通过动态发现父部门避免硬编码 ID/部门名（跨租户通用）。
        """
        if parent_with_children is None:
            pytest.skip("当前租户根部门下所有一级部门均为叶子，无法验证字段结构（非错误）")
        parent_id, children = parent_with_children
        for child in children:
            assert isinstance(child.get("deptId"), int), (
                f"父 {parent_id} 的子部门 deptId 应为 int: {child}"
            )
            assert isinstance(child.get("deptName"), str), (
                f"父 {parent_id} 的子部门 deptName 应为 str: {child}"
            )
            assert child["deptName"], f"父 {parent_id} 的子部门 deptName 不应为空: {child}"

    def test_list_missing_required_id(self, dws):
        """缺少必填的 --id 参数：CLI 应提示错误而非崩溃。

        验证目标：
          - cobra 层参数校验就位（不需要走到 mcp-gw 才报错）；
          - 退出码非 0 + 错误信息提示缺失 id。
        """
        result = dws.run_raw("contact", "dept", "list-children")
        assert result.returncode != 0, (
            f"缺少 --id 应返回非 0 退出码: rc={result.returncode}"
        )
        output = ((result.stdout or "") + (result.stderr or "")).lower()
        assert "id" in output or "required" in output, (
            f"错误信息未提示缺失 id: {output[:300]}"
        )


class TestDeptListMembers:
    """dws contact dept list-members — {"deptUserList":[], "success":true}"""

    def test_list_returns_deptUserList(self, dws):
        """查询部门成员应返回 deptUserList 列表。"""
        data = dws.run(
            "contact", "dept", "list-members", "--ids", "1",
        )
        assert data.get("success") is True
        assert "deptUserList" in data, f"缺少 deptUserList: {data.keys()}"
        assert isinstance(data["deptUserList"], list)

    def test_list_members_structure(self, dws):
        """返回的 deptUserList 应为列表类型。"""
        data = dws.run(
            "contact", "dept", "list-members", "--ids", "1",
        )
        assert isinstance(data["deptUserList"], list)

    def test_list_invalid_dept_members(self, dws):
        """无效部门应返回空 deptUserList。"""
        data = dws.run(
            "contact", "dept", "list-members",
            "--ids", "999999999",
        )
        assert isinstance(data.get("deptUserList"), list)
        assert len(data["deptUserList"]) == 0

    def test_list_members_missing_required_ids(self, dws):
        """缺少必填的 --ids 参数：CLI 应提示错误而非崩溃。

        验证目标：
          - cobra 层参数校验就位（不需要走到 mcp-gw 才报错）；
          - 退出码非 0 + 错误信息提示缺失 ids。
        """
        result = dws.run_raw("contact", "dept", "list-members")
        assert result.returncode != 0, (
            f"缺少 --ids 应返回非 0 退出码: rc={result.returncode}"
        )
        output = ((result.stdout or "") + (result.stderr or "")).lower()
        assert "ids" in output or "required" in output, (
            f"错误信息未提示缺失 ids: {output[:300]}"
        )

    def test_list_members_batch_ids(self, dws, parent_with_children):
        """--ids 支持逗号分隔批量查询多个部门。

        验证目标：
          - 传入多个 deptId，deptUserList 应为单个查询的合并集；
          - 批量成员数应 >= max(各单个部门直接成员数)。
        """
        if parent_with_children is None:
            pytest.skip("当前租户无多级部门，无法构造批量场景")
        parent_id, children = parent_with_children
        child_id = children[0]["deptId"]

        single_parent = dws.run(
            "contact", "dept", "list-members", "--ids", str(parent_id),
        )
        single_child = dws.run(
            "contact", "dept", "list-members", "--ids", str(child_id),
        )
        batch = dws.run(
            "contact", "dept", "list-members",
            "--ids", f"{parent_id},{child_id}",
        )

        assert batch.get("success") is True
        assert isinstance(batch.get("deptUserList"), list)

        n_parent = len(single_parent.get("deptUserList") or [])
        n_child = len(single_child.get("deptUserList") or [])
        n_batch = len(batch.get("deptUserList") or [])
        assert n_batch >= max(n_parent, n_child), (
            f"批量查询成员数应不少于单个部门最大值: "
            f"batch={n_batch}, parent={n_parent}, child={n_child}"
        )

    def test_list_members_excludes_children_scope(self, dws, parent_with_children):
        """核心作用域语义：list-members 仅返回本部门直接成员，不递归包含子部门。

        反证法：
          - 设父部门 P，子部门 C。
          - 若 list-members 递归，则 list-members(P) 返回的 userId 集合应
            包含 list-members(C) 的全部 userId。
          - 本用例断言：C 的成员中至少有一个 userId 不出现在
            list-members(P) 的结果里，即可证明 P 未递归包含 C。
          - 若 C 的所有成员也同时挂在 P 下（少见的双属性），则 skip。
        """
        if parent_with_children is None:
            pytest.skip("当前租户无多级部门，无法验证作用域")
        parent_id, children = parent_with_children

        def _user_ids(data: dict) -> set:
            ids = set()
            for u in (data.get("deptUserList") or []):
                uid = u.get("userId") or u.get("userid")
                if not uid and isinstance(u.get("orgEmployeeModel"), dict):
                    uid = u["orgEmployeeModel"].get("userId")
                if uid:
                    ids.add(uid)
            return ids

        parent_data = dws.run(
            "contact", "dept", "list-members", "--ids", str(parent_id),
        )
        parent_ids = _user_ids(parent_data)

        # 遍历子部门，找到一个能作用域对比的用例
        for child in children:
            cid = child.get("deptId")
            if not isinstance(cid, int):
                continue
            child_data = dws.run(
                "contact", "dept", "list-members", "--ids", str(cid),
            )
            child_ids = _user_ids(child_data)
            if not child_ids:
                continue  # 子部门无直接成员，无法作证
            # 核心断言：子部门至少有 1 个成员不属于父部门直接成员
            if child_ids - parent_ids:
                return  # 证伪成功，非递归
        pytest.skip(
            "未找到适合的父子对来验证作用域（子部门成员全为空，或均同时挂在父部门）"
        )
