"""
test_01_user.py — 用户查询测试 (4 commands × 3+ cases)

实际返回格式:
  get-self:      {"result": [{orgEmployeeModel: {userId,...}}], "success": true}
  search:        {"result": [{userId, name, nick,...}], "success": true}
  search-mobile: {"arguments":[], "result":{userId,orgUserName}, "success":true}
  get:           {"result": [{orgEmployeeModel:{...}}], "success": true}

消歧要点:
  user search 不返回 depts（部门），多命中时须追加 user get --ids 获取部门信息消歧。
"""

import pytest


class TestUserGetSelf:
    """dws contact user get-self"""

    def test_get_self_returns_profile(self, dws):
        """返回当前用户信息，result 应为非空列表。"""
        data = dws.run("contact", "user", "get-self")
        assert data.get("success") is True
        result = data.get("result")
        assert isinstance(result, list), f"result 应为 list, 实为: {type(result)}"
        assert len(result) >= 1, "result 不应为空"

    def test_get_self_contains_userId(self, dws):
        """返回数据中必须包含 userId。"""
        data = dws.run("contact", "user", "get-self")
        result = data["result"]
        user = result[0]
        emp = user.get("orgEmployeeModel", {})
        assert "userId" in emp, f"orgEmployeeModel 中缺少 userId: {emp.keys()}"
        assert isinstance(emp["userId"], str)
        assert len(emp["userId"]) > 0

    def test_get_self_contains_orgUserName(self, dws):
        """返回数据中必须包含 orgUserName。"""
        data = dws.run("contact", "user", "get-self")
        emp = data["result"][0]["orgEmployeeModel"]
        assert "orgUserName" in emp
        assert isinstance(emp["orgUserName"], str)
        assert len(emp["orgUserName"]) > 0


class TestUserSearch:
    """dws contact user search — 返回 {"result": [...], "success": true}"""

    def test_search_returns_result_list(self, dws):
        """搜索结果应返回 result 列表。"""
        data = dws.run_ok(
            "contact", "user", "search", "--keyword", "测试",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert "result" in data, f"响应缺少 result 字段: {data.keys()}"
        assert isinstance(data["result"], list)

    def test_search_no_match_returns_empty(self, dws):
        """搜索不存在的用户应返回空 result 列表。"""
        data = dws.run_ok(
            "contact", "user", "search",
            "--keyword", "ZZZNONEXIST99999",
        )
        assert data.get("success") is True
        assert "result" in data
        assert isinstance(data["result"], list)
        assert len(data["result"]) == 0

    def test_search_chinese_name(self, dws):
        """搜索中文名字应返回有效结构。"""
        data = dws.run_ok(
            "contact", "user", "search", "--keyword", "张",
        )
        assert data.get("success") is True
        assert "result" in data
        assert isinstance(data["result"], list)


class TestUserSearchMobile:
    """dws contact user search-mobile — MCP格式"""

    def test_search_valid_mobile(self, dws):
        """按真实手机号搜索应返回有效结构，result 可能为空（取决于测试数据）。"""
        data = dws.run(
            "contact", "user", "search-mobile",
            "--mobile", "17681800166",
        )
        assert data.get("success") is True
        result = data.get("result", {})
        assert isinstance(result, dict)
        if result:
            assert "userId" in result, f"result 非空但缺少 userId: {result}"
            assert "orgUserName" in result

    def test_search_invalid_format_returns_error(self, dws):
        """无效手机号格式：API 可能返回错误或仍返回结果，但不应崩溃。"""
        result = dws.run_raw(
            "contact", "user", "search-mobile",
            "--mobile", "INVALID_NOT_A_PHONE",
        )
        # API 可能不校验格式而仍返回 JSON，也可能返回错误；
        # 核心断言：命令应可执行且产出可识别输出。
        combined = (result.stdout or "") + (result.stderr or "")
        assert len(combined.strip()) > 0, "命令未产生任何输出"

    def test_search_self_mobile(self, dws, current_user_id):
        """搜索手机号应返回有效结构，result 非空时应包含有效 userId。"""
        data = dws.run(
            "contact", "user", "search-mobile",
            "--mobile", "17681800166",
        )
        result = data.get("result", {})
        if result:
            assert "userId" in result, f"result 非空但缺少 userId: {result}"
            assert isinstance(result["userId"], str)
            assert len(result["userId"]) > 0


class TestUserGet:
    """dws contact user get — 返回 {result:[...], success:true}"""

    def test_get_by_id(self, dws, current_user_id):
        """按 userId 获取用户详情，result 应为非空列表。"""
        data = dws.run(
            "contact", "user", "get",
            "--ids", current_user_id,
        )
        assert data.get("success") is True
        result = data.get("result")
        assert isinstance(result, list)
        assert len(result) >= 1

    def test_get_returns_orgEmployeeModel(self, dws, current_user_id):
        """返回的用户应包含 orgEmployeeModel（注: get 返回的子字段
        与 get-self 不同，get 不一定含 userId 字段）。"""
        data = dws.run(
            "contact", "user", "get",
            "--ids", current_user_id,
        )
        user = data["result"][0]
        assert "orgEmployeeModel" in user
        emp = user["orgEmployeeModel"]
        assert isinstance(emp, dict)
        # get 接口返回 orgName 但不一定返回 userId
        assert "orgName" in emp, f"orgEmployeeModel 缺少 orgName: {list(emp.keys())}"

    def test_get_invalid_id_returns_empty_model(self, dws):
        """获取无效 userId: API 返回含空字段的 orgEmployeeModel。"""
        data = dws.run_ok(
            "contact", "user", "get",
            "--ids", "INVALID_USER_99999",
        )
        result = data.get("result", [])
        assert isinstance(result, list)
        # API 会返回一个对象但字段全为 null
        if len(result) > 0:
            emp = result[0].get("orgEmployeeModel", {})
            assert emp.get("depts") is None, "无效用户的 depts 应为 null"


# ── 重名消歧 ─────────────────────────────────────────────────────────────


class TestDisambiguation:
    """重名消歧流程：user search 不返回部门 → user get --ids 返回部门 → 可消歧。

    核心链路：
      1. contact user search 返回多人时，结果中不含 depts 字段
      2. 追加 contact user get --ids userId1,userId2,... 可获取 depts
      3. 组合两者即可展示「姓名+部门+职位」供用户选择
    """

    def test_search_result_lacks_dept_fields(self, dws):
        """user search 返回的每条结果中不包含 depts 字段，这是重名无法消歧的根因。"""
        data = dws.run_ok(
            "contact", "user", "search", "--keyword", "张",
        )
        results = data.get("result", [])
        if not results:
            pytest.skip("当前环境搜索'张'无结果，跳过字段检查")
        for item in results:
            # search 结果直接在顶层，不在 orgEmployeeModel 里
            assert "depts" not in item, (
                f"user search 结果不应含 depts 字段，但发现: {list(item.keys())}"
            )

    def test_search_result_lacks_deptName(self, dws):
        """user search 返回的每条结果中不包含 deptName 字段。"""
        data = dws.run_ok(
            "contact", "user", "search", "--keyword", "张",
        )
        results = data.get("result", [])
        if not results:
            pytest.skip("当前环境搜索'张'无结果，跳过字段检查")
        for item in results:
            assert "deptName" not in item, (
                f"user search 结果不应含 deptName 字段，但发现: {list(item.keys())}"
            )

    def test_search_result_has_no_nested_employee_model(self, dws):
        """user search 结果应保持扁平结构，不应出现 orgEmployeeModel 嵌套。

        背景与目的：
          - 消歧规则依赖于 user search 不返回部门信息，需上游再跟 user get 拼装；
          - 若未来有人把 search 结果封装成与 user get 一致的嵌套 orgEmployeeModel
            结构，depts 会藏在子对象里，导致现有 `depts not in item`
            断言仍通过，但消歧规则被悄悄破坏。
          - 本用例在结构层强制阻止该漂移。
        """
        data = dws.run_ok(
            "contact", "user", "search", "--keyword", "张",
        )
        results = data.get("result", [])
        if not results:
            pytest.skip("当前环境搜索'张'无结果，跳过结构检查")
        for item in results:
            assert "orgEmployeeModel" not in item, (
                f"user search 结果应保持扁平，不应出现 orgEmployeeModel 嵌套: "
                f"{list(item.keys())}"
            )

    def test_get_by_ids_returns_depts_for_single_user(self, dws, current_user_id):
        """user get --ids 返回 orgEmployeeModel.depts，可提供部门信息。"""
        data = dws.run(
            "contact", "user", "get",
            "--ids", current_user_id,
        )
        emp = data["result"][0]["orgEmployeeModel"]
        # depts 字段必须存在（可能为 null 或 list，但 key 必须有）
        assert "depts" in emp, (
            f"user get 的 orgEmployeeModel 应含 depts 字段，实际: {list(emp.keys())}"
        )

    def test_get_by_ids_returns_orgName_for_single_user(self, dws, current_user_id):
        """user get --ids 返回 orgEmployeeModel.orgName（职位/姓名），可辅助消歧。"""
        data = dws.run(
            "contact", "user", "get",
            "--ids", current_user_id,
        )
        emp = data["result"][0]["orgEmployeeModel"]
        assert "orgName" in emp, (
            f"user get 的 orgEmployeeModel 应含 orgName 字段，实际: {list(emp.keys())}"
        )

    def test_search_then_get_disambiguation_flow(self, dws):
        """完整消歧流程：search 多命中 → get --ids 批量获取部门 → 可区分。

        验证当 search 返回多人时，通过 get --ids 可以拿到每人部门信息。
        """
        # Step 1: search
        data = dws.run_ok(
            "contact", "user", "search", "--keyword", "张",
        )
        results = data.get("result", [])
        if len(results) < 2:
            pytest.skip(f"搜索'张'仅返回 {len(results)} 条，无法测试多命中消歧")

        # Step 2: 提取 userId 列表
        user_ids = []
        for item in results:
            uid = item.get("userId")
            if uid:
                user_ids.append(uid)
        if len(user_ids) < 2:
            pytest.skip(f"搜索结果中有效 userId 不足 2 个 ({len(user_ids)})，跳过")

        # Step 3: get --ids 批量获取详情（取前 5 个，避免过长）
        ids_str = ",".join(user_ids[:5])
        detail_data = dws.run(
            "contact", "user", "get",
            "--ids", ids_str,
        )
        detail_results = detail_data.get("result", [])
        assert len(detail_results) >= 2, (
            f"get --ids 应返回至少 2 条详情，实际: {len(detail_results)}"
        )

        # Step 4: 验证每条详情包含 depts 字段（消歧的关键）
        for user in detail_results:
            emp = user.get("orgEmployeeModel", {})
            assert "depts" in emp, (
                f"user get 详情应含 depts 字段用于消歧，实际: {list(emp.keys())}"
            )
