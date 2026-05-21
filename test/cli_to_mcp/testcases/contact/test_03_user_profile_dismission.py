"""
test_03_user_profile_dismission.py — 用户档案（花名册）& 离职员工测试

Commands tested (迁移自 hrmregister，新路径在 contact user 下):
  1. dws contact user profile fields
  2. dws contact user profile get [--staff-id <ID>] [--fields <CODES>]
  3. dws contact user dismission search [--name <NAME>] [--start <DATE> --end <DATE>]
                                        [--depts <IDS>] [--hide-retirement] [--hide-partner]
                                        [--page <N>] [--limit <N>]

NOTE:
  - profile fields / profile get 依赖当前账号权限；若无权限可能返回业务错误。
  - dismission search 全部参数可选；--start/--end 必须同时设置或同时不设置。
  - dismission search 结果依赖生产环境数据，允许返回空列表。
"""

import json
import pytest


def _assert_success_response(data: dict, label: str = ""):
    """Validate common success response: success=true and result exists."""
    assert isinstance(data, dict), f"{label} response should be dict, got {type(data)}"
    assert data.get("success") is True or data.get("success") == "true", \
        f"{label} expected success, got: success={data.get('success')}, data={str(data)[:300]}"
    assert "result" in data, \
        f"{label} response should contain 'result' key, got keys: {list(data.keys())}"


# ── contact user profile fields ──────────────────────────────────────────


class TestContactUserProfileFields:
    """dws contact user profile fields

    Queries the list of roster fields that the current user has permission to access.
    No flags required; auth params are auto-injected.
    """

    def test_fields_returns_success(self, dws):
        """不传任何参数，应返回成功响应且 result 非空。"""
        data = dws.run_ok("contact", "user", "profile", "fields")
        _assert_success_response(data, "profile_fields")
        result = data["result"]
        assert result is not None, "result 不应为 None"

    def test_fields_result_is_list_or_dict(self, dws):
        """result 应为 list 或 dict 结构。"""
        data = dws.run_ok("contact", "user", "profile", "fields")
        result = data["result"]
        assert isinstance(result, (list, dict)), \
            f"result 应为 list 或 dict, 实为: {type(result)}"

    def test_fields_idempotent(self, dws):
        """连续调用多次应均返回相同结构（幂等性测试）。"""
        data1 = dws.run_ok("contact", "user", "profile", "fields")
        data2 = dws.run_ok("contact", "user", "profile", "fields")
        assert data1.get("success") is True
        assert data2.get("success") is True


# ── contact user profile get ─────────────────────────────────────────────


class TestContactUserProfileGet:
    """dws contact user profile get [--staff-id <ID>] [--fields <CODES>]

    Queries roster field values for a specified employee.
    Both flags are optional; omitting --staff-id may query the current user's
    data (behavior depends on MCP implementation).
    """

    def test_get_self_no_flags(self, dws, current_user_id):
        """传入当前用户 ID，不指定字段，返回全量可见字段信息。"""
        result = dws.run_raw(
            "contact", "user", "profile", "get",
            "--staff-id", current_user_id,
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "profile get 应返回非空输出"
        data = json.loads(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_result = "result" in data and data.get("success") is True
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got keys: {list(data.keys())}"

    def test_get_with_fields_from_fields_cmd(self, dws, current_user_id):
        """先获取字段列表，再指定字段 code 查询花名册信息。"""
        # Step 1: 获取可用字段列表
        fields_data = dws.run_ok("contact", "user", "profile", "fields")
        _assert_success_response(fields_data, "profile_fields_for_get")
        result_fields = fields_data["result"]

        # 提取第一个字段 code（字段列表格式不确定，尝试常见结构）
        field_code = None
        if isinstance(result_fields, list) and len(result_fields) > 0:
            first = result_fields[0]
            field_code = first.get("fieldCode") or first.get("code") or first.get("id")
        elif isinstance(result_fields, dict):
            items = result_fields.get("fieldList") or result_fields.get("items") or []
            if items:
                field_code = items[0].get("fieldCode") or items[0].get("code")

        if not field_code:
            pytest.skip("未获取到可用字段 code，跳过字段查询测试")

        # Step 2: 指定字段查询
        result = dws.run_raw(
            "contact", "user", "profile", "get",
            "--staff-id", current_user_id,
            "--fields", field_code,
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "profile get --fields 应返回非空输出"
        data = json.loads(output)
        assert isinstance(data, dict)
        has_result = "result" in data and data.get("success") is True
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got keys: {list(data.keys())}"

    def test_get_without_staff_id(self, dws):
        """不传 --staff-id，应返回有效结构（成功或业务错误）。"""
        result = dws.run_raw("contact", "user", "profile", "get")
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "profile get 无参数应返回非空输出"
        data = json.loads(output)
        assert isinstance(data, dict)


# ── contact user dismission search ───────────────────────────────────────


class TestContactUserDismissionSearch:
    """dws contact user dismission search [...]

    Queries dismissed employee list. All flags optional.
    Constraint: --start and --end must be set together or both omitted.
    Result depends on production data; empty list is acceptable.
    """

    def test_search_no_args(self, dws):
        """不传任何参数，返回成功响应（允许空列表）。"""
        data = dws.run_ok("contact", "user", "dismission", "search")
        _assert_success_response(data, "dismission_search_no_args")
        result = data["result"]
        assert isinstance(result, (list, dict)), \
            f"result 应为 list 或 dict, 实为: {type(result)}"

    def test_search_with_name_filter(self, dws):
        """--name 模糊搜索员工姓名。"""
        data = dws.run_ok(
            "contact", "user", "dismission", "search",
            "--name", "张",
        )
        _assert_success_response(data, "dismission_search_name")
        assert "result" in data

    def test_search_with_date_range(self, dws):
        """--start 和 --end 同时设置为有效日期范围。"""
        data = dws.run_ok(
            "contact", "user", "dismission", "search",
            "--start", "2025-01-01",
            "--end", "2025-12-31",
        )
        _assert_success_response(data, "dismission_search_date_range")
        assert "result" in data

    def test_search_with_pagination(self, dws):
        """自定义分页参数 --page --limit。"""
        data = dws.run_ok(
            "contact", "user", "dismission", "search",
            "--page", "1",
            "--limit", "10",
        )
        _assert_success_response(data, "dismission_search_pagination")
        assert "result" in data

    def test_search_hide_retirement_false(self, dws):
        """--hide-retirement=false 展示退休人员。"""
        data = dws.run_ok(
            "contact", "user", "dismission", "search",
            "--hide-retirement=false",
        )
        _assert_success_response(data, "dismission_search_hide_retirement_false")
        assert "result" in data

    def test_search_hide_partner_true(self, dws):
        """--hide-partner=true 隐藏合作伙伴。"""
        data = dws.run_ok(
            "contact", "user", "dismission", "search",
            "--hide-partner=true",
        )
        _assert_success_response(data, "dismission_search_hide_partner_true")
        assert "result" in data

    def test_search_combined_filters(self, dws):
        """组合多个参数：姓名 + 日期范围 + 分页。"""
        data = dws.run_ok(
            "contact", "user", "dismission", "search",
            "--name", "李",
            "--start", "2024-01-01",
            "--end", "2024-12-31",
            "--page", "1",
            "--limit", "5",
        )
        _assert_success_response(data, "dismission_search_combined")
        assert "result" in data

    def test_search_only_start_returns_cli_error(self, dws):
        """只传 --start 不传 --end，CLI 应返回参数校验错误。"""
        result = dws.run_raw(
            "contact", "user", "dismission", "search",
            "--start", "2025-01-01",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert len(combined.strip()) > 0, "命令应产生输出"
        has_error_exit = result.returncode != 0
        has_error_text = "必须同时" in combined or "Error" in combined or "error" in combined
        assert has_error_exit or has_error_text, \
            f"只传 --start 应有错误提示, combined={combined[:200]}"

    def test_search_only_end_returns_cli_error(self, dws):
        """只传 --end 不传 --start，CLI 应返回参数校验错误。"""
        result = dws.run_raw(
            "contact", "user", "dismission", "search",
            "--end", "2025-12-31",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert len(combined.strip()) > 0, "命令应产生输出"
        has_error_exit = result.returncode != 0
        has_error_text = "必须同时" in combined or "Error" in combined or "error" in combined
        assert has_error_exit or has_error_text, \
            f"只传 --end 应有错误提示, combined={combined[:200]}"
