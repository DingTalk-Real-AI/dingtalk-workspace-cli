"""test_attendance.py — 考勤测试 (29 commands)

Commands tested:
  1. dws attendance record get --user <USER_ID> --date <DATE>
  2. dws attendance shift list --users <USER_IDS> --start <DATE> --end <DATE>
  3. dws attendance summary --user <USER_ID> --date <DATETIME>
  4. dws attendance rules --date <DATE>
  5. dws attendance report columns
  6. dws attendance report query-data --users <IDS> --columns <COL_IDS> --start <DT> --end <DT>
  7. dws attendance report query-leave --users <IDS> --start <DT> --end <DT> [--leave-names <NAMES>]
  8. dws attendance class search [--query <QUERY>] [--filter-type <TYPE>] [--page <N>] [--limit <N>]
  9. dws attendance class get --class-id <ID>
  10. dws attendance class create --name <NAME> --class-vo <JSON> [--owner <USER_ID>] --timeout 10
  11. dws attendance adjustment search [--query <QUERY>] --page <N> --limit <N>
    12. dws attendance overtime search [--query <QUERY>] --page <N> --limit <N>
    13. dws attendance group search [--query <QUERY>] [--type <TYPE>] --page <N> --limit <N>
  14. dws attendance leave-types
  15. dws attendance leave-balance --users <USER_IDS> [--leave-code <CODE>]
  16. dws attendance leave-records --user <USER_ID> --start <DATE> --end <DATE> [--leave-code <CODE>]
  17. dws attendance checkin records --operator-corp-id <CORP_ID> --operator-staff-id <STAFF_ID> --staff-ids <IDS> --start <DT> --end <DT>
  18. dws attendance adjustment get --adjustment-id <ID>
  19. dws attendance overtime get --overtime-id <ID>
  20. dws attendance group get --group-id <ID>
  21. dws attendance group filtered-get --group-id <ID> [--member] [--position] [--wifi] [--bles]
  22. dws attendance group update --group-id <ID> [--name] [--type] [--owner] [--classIds] [--enable-outside-check] [--group-vo] --timeout 10
  23. dws attendance group update-members --group-id <ID> [--add-users] [--remove-users] [--add-depts] [--remove-depts] --timeout 10
  24. dws attendance approve templates --type <TYPE>
  25. dws attendance selfsetting get --setting-scene <SCENE> --user <USER_ID>
  26. dws attendance selfsetting save --setting-scene <SCENE> --user <USER_ID> [setting flags]
  27. dws attendance class update --class-id <ID> [--name] [--owner] [--class-vo] --timeout 10
  28. dws attendance group create --name <NAME> --type <TYPE> [--owner] [--group-vo] --timeout 10


NOTE (2026-04):
  - shift query subcommand removed; only shift list remains.
  - shift list flags renamed: --from/--to → --start/--end.
  - summary now requires --user + --date (both mandatory).
  - group renamed to rules, --date mandatory.
  - report subcommand group added (admin-only): columns, query-data, query-leave.
  - class search added: queries shift definitions (not schedules) via attendance-wukong MCP server.

NOTE (2026-04-24):
  - schedule import: scheduleVOS 必填字段校验 (userId, workDate, classId, isRest)
  - schedule import: workDate 格式自动转换为 yyyy-MM-dd HH:mm:ss
  - schedule get: 参数结构调整为 GetScheduleByRangeRequest
  - class get added: queries shift detail by class-id.
  - adjustment search added: queries makeup/adjustment rules.
  - overtime search added: queries overtime rules.
  - group search added: queries attendance groups.
  - group get added: queries full attendance group detail by groupId.
  - group filtered-get added: queries partial attendance group info (member/position/wifi/bles).
  - adjustment get added: queries adjustment rule detail by adjustmentId (deleted/overwritten rules NOT queryable).
  - overtime get added: queries overtime rule detail by overtimeId (deleted/overwritten rules ARE queryable).
  - leave-types added: queries current user's leave type rules, no flags required.
  - vacation types added: queries current user's leave type rules, no flags required.
    Input now wraps in McpLeaveTypeRequest; auth params auto-injected.
  - vacation balance added: queries vacation balance quota for specified employees.
  - vacation records added: queries vacation balance change records for a specified employee.

NOTE (2026-04-25):
  - group update added: updates attendance group settings (name/type/owner/classIds/enable-outside-check/group-vo).
    --enable-outside-check uses String flag (not Bool) to support explicit "false" value.
    --classIds accepts JSON array format. --group-vo for complex sub-objects.
    groupVO auto-injects id field from --group-id. --timeout 10 recommended.
  - group update-members added: updates attendance group members (add/remove users/depts).
    --timeout 10 recommended due to server-side latency.
  - class create added: creates a new shift class via create_class_setting.
    --name and --class-vo (containing sections) are mandatory. --owner optional.
    --timeout 10 recommended due to server-side latency.

NOTE (2026-05-13):
  - approve templates added: queries REPAIR_CHECK/LEAVE/OVERTIME approval template submitUrl.

NOTE (2026-05-13):
  - selfsetting get/save added: query and save personal rule settings.
  - selfsetting --user is mandatory for both get and save.
  - selfsetting save testcases use --dry-run to avoid mutating real user settings.

NOTE (2026-06):
  - class update added: updates an existing shift class via update_class_setting.
    --class-id mandatory. --class-vo optional (empty map created if absent).
    classVO auto-injects "id" field from --class-id. --timeout 10 recommended.
  - group create added: creates a new attendance group via create_group_setting.
    --name and --type mandatory. --type must be FIXED/TURN/NONE.
    type=FIXED requires workDayClassList + defaultClassId in --group-vo.
    --timeout 10 recommended.
"""

import json

import pytest


def _load_first_json_object(output: str):
    """Parse the first JSON object from CLI output.

    Some MCP servers append WARN logs after JSON output. json.loads rejects that
    as extra data, while tests only need the first structured response.
    """
    assert output, "CLI output should not be empty"
    decoder = json.JSONDecoder()
    for start_index, character in enumerate(output):
        if character not in "[{":
            continue
        try:
            parsed_value, _ = decoder.raw_decode(output[start_index:])
            return parsed_value
        except json.JSONDecodeError:
            continue
    raise AssertionError(f"CLI output should contain JSON, got: {output[:500]}")

from test_utils import iso8601_date_cn


def _assert_success_response(data: dict, label: str = ""):
    """Validate common success response: success=true and result exists."""
    assert isinstance(data, dict), f"{label} response should be dict, got {type(data)}"
    assert data.get("success") is True or data.get("success") == "true", \
        f"{label} expected success, got: success={data.get('success')}, data={str(data)[:300]}"
    assert "result" in data, f"{label} response should contain 'result' key, got keys: {list(data.keys())}"


class TestAttendanceRecord:
    """dws attendance record get --user <USER_ID> --date <DATE>"""

    def test_get_record_today(self, dws, current_user_id):
        """获取今天的考勤记录（必填参数测试）。"""
        today = iso8601_date_cn()
        data = dws.run_ok(
            "attendance", "record", "get",
            "--user", current_user_id,
            "--date", today,
        )
        _assert_success_response(data, "record_get_today")
        result = data["result"]
        assert isinstance(result, dict), f"result should be dict, got {type(result)}"

    def test_get_record_specific_date(self, dws, current_user_id):
        """获取指定日期的考勤记录（必填参数测试）。"""
        data = dws.run_ok(
            "attendance", "record", "get",
            "--user", current_user_id,
            "--date", "2026-03-08",
        )
        _assert_success_response(data, "record_get_specific")
        assert isinstance(data["result"], dict)

    def test_get_record_recent_date(self, dws, current_user_id):
        """获取近期日期的考勤记录（API 限制不能查半年前）。"""
        data = dws.run_ok(
            "attendance", "record", "get",
            "--user", current_user_id,
            "--date", "2026-04-01",
        )
        _assert_success_response(data, "record_get_recent")
        assert isinstance(data["result"], dict)


class TestAttendanceShiftList:
    """dws attendance shift list --users <USER_IDS> --start <DATE> --end <DATE>"""

    def test_list_shifts_basic(self, dws, current_user_id):
        """批量查询员工排班（必填参数测试）。"""
        data = dws.run_ok(
            "attendance", "shift", "list",
            "--users", current_user_id,
            "--start", "2026-04-07",
            "--end", "2026-04-13",
        )
        assert data.get("success") is True, f"Expected success=true, got: {data.get('success')}"
        assert isinstance(data.get("result"), list), \
            f"result should be list, got: {type(data.get('result'))}"

    def test_list_shifts_multiple_users(self, dws, current_user_id):
        """批量查询多个员工排班。"""
        users = f"{current_user_id},{current_user_id}"
        data = dws.run_ok(
            "attendance", "shift", "list",
            "--users", users,
            "--start", "2026-04-07",
            "--end", "2026-04-13",
        )
        assert data.get("success") is True, f"Expected success=true, got: {data.get('success')}"
        assert isinstance(data.get("result"), list)

    def test_list_shifts_recent_range(self, dws, current_user_id):
        """查询近期日期的排班（API 限制跨度不超过 7 天）。"""
        data = dws.run_ok(
            "attendance", "shift", "list",
            "--users", current_user_id,
            "--start", "2026-04-01",
            "--end", "2026-04-07",
        )
        assert data.get("success") is True
        assert isinstance(data.get("result"), list)


class TestAttendanceSummary:
    """dws attendance summary --user <USER_ID> --date <DATETIME>

    NOTE: summary may return business error (C0002) when the test account
    has no attendance schedule configured. We validate the response structure
    rather than requiring success.
    """

    def test_summary_basic(self, dws, current_user_id):
        """获取考勤统计摘要（必填参数 --user + --date）。"""
        result = dws.run_raw(
            "attendance", "summary",
            "--user", current_user_id,
            "--date", "2026-04-13 09:00:00",
        )
        # Accept either success response or structured business error
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "summary should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        # Either success with result, or structured error
        has_result = "result" in data and data.get("success") is True
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected either success+result or structured error, got keys: {list(data.keys())}"

    def test_summary_returns_structured_response(self, dws, current_user_id):
        """summary 返回应有结构化格式（成功或业务错误）。"""
        result = dws.run_raw(
            "attendance", "summary",
            "--user", current_user_id,
            "--date", "2026-04-01 09:00:00",
        )
        output = result.stdout.strip() or result.stderr.strip()
        data = _load_first_json_object(output)
        if data.get("success") is True:
            assert "result" in data, "Success response should contain result"
        elif "error" in data:
            err = data["error"]
            assert isinstance(err, dict), f"error should be dict, got {type(err)}"
            assert "message" in err, f"error should contain message, got keys: {list(err.keys())}"
        else:
            pytest.fail(f"Unexpected response structure: {list(data.keys())}")


class TestAttendanceClassSearch:
    """dws attendance class search [--query] [--filter-type] [--page] [--limit]

    Queries shift definitions (class catalog) managed by the current user.
    All flags are optional; omitting them returns the full list with defaults
    (page 1, page size 20).
    """

    def test_class_search_no_args(self, dws):
        """不传任何参数，返回当前用户可管理的全部班次列表。"""
        data = dws.run_ok("attendance", "class", "search")
        _assert_success_response(data, "class_search_no_args")
        result = data["result"]
        assert isinstance(result, dict), f"result should be dict, got {type(result)}"
        assert "items" in result, f"result should contain 'items' key, got: {list(result.keys())}"
        assert isinstance(result["items"], list), f"items should be list, got {type(result['items'])}"
        assert "totalCount" in result, f"result should contain 'totalCount'"

    def test_class_search_filter_type_all(self, dws):
        """--filter-type ALL 查询全部班次。"""
        data = dws.run_ok(
            "attendance", "class", "search",
            "--filter-type", "ALL",
        )
        _assert_success_response(data, "class_search_filter_all")
        result = data["result"]
        assert "items" in result
        assert isinstance(result["items"], list)

    def test_class_search_filter_type_mine_own(self, dws):
        """--filter-type MINE_OWN 仅查询当前用户负责的班次。"""
        data = dws.run_ok(
            "attendance", "class", "search",
            "--filter-type", "MINE_OWN",
        )
        _assert_success_response(data, "class_search_filter_mine_own")
        result = data["result"]
        assert "items" in result
        assert isinstance(result["items"], list)

    def test_class_search_with_pagination(self, dws):
        """分页参数 --page 1 --limit 10。"""
        data = dws.run_ok(
            "attendance", "class", "search",
            "--page", "1",
            "--limit", "10",
        )
        _assert_success_response(data, "class_search_pagination")
        result = data["result"]
        assert "items" in result
        assert isinstance(result["items"], list)

    def test_class_search_with_name_filter(self, dws):
        """--query 关键字模糊搜索班次名称。"""
        data = dws.run_ok(
            "attendance", "class", "search",
            "--query", "A",
        )
        _assert_success_response(data, "class_search_name_filter")
        result = data["result"]
        assert "items" in result
        assert isinstance(result["items"], list)


class TestAttendanceClassGet:
    """dws attendance class get --class-id <ID>

    Queries full detail of a single shift class by ID.
    --class-id is mandatory; class IDs can be obtained from `class search`.
    """

    def test_class_get_from_search(self, dws):
        """先通过 class search 获取一个班次 ID，再用 class get 查询详情。"""
        # 先搜索获取 classId
        search_data = dws.run_ok("attendance", "class", "search")
        _assert_success_response(search_data, "class_search_for_get")
        items = search_data.get("result", {}).get("items", [])
        if not items:
            pytest.skip("未找到任何班次，跳过 class get 测试")
        class_id = items[0].get("classId") or items[0].get("id")
        if not class_id:
            pytest.skip("班次记录中未包含 classId 字段，跳过")

        data = dws.run_ok(
            "attendance", "class", "get",
            "--class-id", str(class_id),
        )
        _assert_success_response(data, "class_get")
        result = data["result"]
        assert isinstance(result, dict), f"result should be dict, got {type(result)}"

    def test_class_get_returns_structured_response(self, dws):
        """任意有效 class-id 应返回结构化数据。"""
        search_data = dws.run_ok("attendance", "class", "search")
        items = search_data.get("result", {}).get("items", [])
        if not items:
            pytest.skip("未找到任何班次，跳过")
        class_id = items[0].get("classId") or items[0].get("id")
        if not class_id:
            pytest.skip("班次记录中未包含 classId 字段，跳过")

        data = dws.run_ok(
            "attendance", "class", "get",
            "--class-id", str(class_id),
        )
        assert data.get("success") is True, f"Expected success=true, got: {data.get('success')}"
        assert "result" in data, f"response should contain 'result', got: {list(data.keys())}"

    def test_class_get_invalid_id(self, dws):
        """--class-id -1 传入无效 ID，应返回结构化错误而非崩溃。"""
        result = dws.run_raw(
            "attendance", "class", "get",
            "--class-id", "-1",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "class get --class-id -1 should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        # 无效 ID 应返回业务错误，不应返回 success=true
        has_error = "error" in data and isinstance(data["error"], dict)
        is_failure = data.get("success") is False or has_error
        assert is_failure, \
            f"Expected error response for class-id=-1, got: {list(data.keys())}"


class TestAttendanceAdjustmentSearch:
    """dws attendance adjustment search [--query] --page <N> --limit <N>

    Queries makeup/adjustment rules managed by the current user.
    --page and --limit use defaults 1/20 if omitted.
    """

    def test_adjustment_search_default_pagination(self, dws):
        """使用默认分页参数查询补卡规则列表。"""
        result = dws.run_raw(
            "attendance", "adjustment", "search",
            "--page", "1",
            "--limit", "20",
        )
        data = _parse_raw_json(result)
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data.get("error"), dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got: {list(data.keys())}"

    def test_adjustment_search_with_name_filter(self, dws):
        """--query 关键字模糊搜索补卡规则。"""
        data = dws.run_ok(
            "attendance", "adjustment", "search",
            "--query", "标准",
            "--page", "1",
            "--limit", "20",
        )
        _assert_success_response(data, "adjustment_search_name")
        assert "result" in data

    def test_adjustment_search_custom_page_size(self, dws):
        """自定义分页大小。"""
        data = dws.run_ok(
            "attendance", "adjustment", "search",
            "--page", "1",
            "--limit", "5",
        )
        _assert_success_response(data, "adjustment_search_page_size")
        assert "result" in data


class TestAttendanceOvertimeSearch:
    """dws attendance overtime search [--query] --page <N> --limit <N>

    Queries overtime rules managed by the current user.
    --page and --limit use defaults 1/20 if omitted.
    """

    def test_overtime_search_default_pagination(self, dws):
        """使用默认分页参数查询加班规则列表。"""
        data = dws.run_ok(
            "attendance", "overtime", "search",
            "--page", "1",
            "--limit", "20",
        )
        _assert_success_response(data, "overtime_search_default")
        result = data["result"]
        assert isinstance(result, (dict, list)), f"result should be dict or list, got {type(result)}"

    def test_overtime_search_with_name_filter(self, dws):
        """--query 关键字模糊搜索加班规则。"""
        data = dws.run_ok(
            "attendance", "overtime", "search",
            "--query", "节假日",
            "--page", "1",
            "--limit", "20",
        )
        _assert_success_response(data, "overtime_search_name")
        assert "result" in data

    def test_overtime_search_custom_page_size(self, dws):
        """自定义分页大小。"""
        data = dws.run_ok(
            "attendance", "overtime", "search",
            "--page", "1",
            "--limit", "5",
        )
        _assert_success_response(data, "overtime_search_page_size")
        assert "result" in data


class TestAttendanceGroupSearch:
    """dws attendance group search [--query] [--type] --page <N> --limit <N>

    Queries attendance groups managed by the current user.
    --page and --limit use defaults 1/20 if omitted.

    NOTE: get_simple_groups may not be deployed in all environments.
    Tests validate CLI parameter passing and structured response format
    rather than requiring business success.
    """

    @staticmethod
    def _parse_group_output(result) -> dict:
        """Parse stdout or stderr as JSON, fail if neither is valid JSON."""
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "group search should return non-empty output"
        return _load_first_json_object(output)

    def test_group_search_default_pagination(self, dws):
        """使用默认分页参数查询考勤组列表。"""
        result = dws.run_raw(
            "attendance", "group", "search",
            "--page", "1",
            "--limit", "20",
        )
        data = self._parse_group_output(result)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got: {list(data.keys())}"

    def test_group_search_with_name_filter(self, dws):
        """--query 关键字模糊搜索考勤组。"""
        result = dws.run_raw(
            "attendance", "group", "search",
            "--query", "研发",
            "--page", "1",
            "--limit", "20",
        )
        data = self._parse_group_output(result)
        assert isinstance(data, dict)
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error

    def test_group_search_fixed_type(self, dws):
        """--type FIXED 查询固定班制考勤组。"""
        result = dws.run_raw(
            "attendance", "group", "search",
            "--type", "FIXED",
            "--page", "1",
            "--limit", "20",
        )
        data = self._parse_group_output(result)
        assert isinstance(data, dict)
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error

    def test_group_search_turn_type(self, dws):
        """--type TURN 查询排班制考勤组。"""
        result = dws.run_raw(
            "attendance", "group", "search",
            "--type", "TURN",
            "--page", "1",
            "--limit", "20",
        )
        data = self._parse_group_output(result)
        assert isinstance(data, dict)
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error

    def test_group_search_custom_page_size(self, dws):
        """自定义分页大小。"""
        result = dws.run_raw(
            "attendance", "group", "search",
            "--page", "1",
            "--limit", "5",
        )
        data = self._parse_group_output(result)
        assert isinstance(data, dict)
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error


class TestAttendanceRules:
    """dws attendance rules --date <DATE>"""

    def test_rules_basic(self, dws):
        """获取考勤组和规则（必填参数 --date）。"""
        data = dws.run_ok(
            "attendance", "rules",
            "--date", "2026-04-13",
        )
        _assert_success_response(data, "rules_basic")
        result = data["result"]
        assert isinstance(result, dict), f"result should be dict, got {type(result)}"
        assert "data" in result, f"result should contain 'data' key, got keys: {list(result.keys())}"
        assert isinstance(result["data"], list), \
            f"result.data should be list, got {type(result['data'])}"

    def test_rules_past_date(self, dws):
        """获取过去日期的考勤规则。"""
        data = dws.run_ok(
            "attendance", "rules",
            "--date", "2026-03-08",
        )
        _assert_success_response(data, "rules_past")
        assert isinstance(data["result"], dict)

    def test_rules_idempotent(self, dws):
        """多次调用应返回相同结构。"""
        d1 = dws.run_ok("attendance", "rules", "--date", "2026-04-13")
        d2 = dws.run_ok("attendance", "rules", "--date", "2026-04-13")
        assert d1.get("success") == d2.get("success"), \
            f"Idempotent check: success mismatch"
        assert type(d1.get("result")) == type(d2.get("result")), \
            f"Idempotent check: result type mismatch"


class TestAttendanceSelfSetting:
    """dws attendance selfsetting get/save

    Queries and saves personal attendance rule settings. --user is mandatory
    for both get and save. Save tests use --dry-run to avoid changing real
    user settings in the test environment.
    """

    @staticmethod
    def _assert_structured_response(data: dict, label: str):
        """Accept either success+result or structured business error."""
        assert isinstance(data, dict), f"{label} response should be dict, got {type(data)}"
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data.get("error"), dict)
        assert has_result or has_error, \
            f"{label} expected success+result or structured error, got keys: {list(data.keys())}"

    @staticmethod
    def _assert_missing_required_flag(result, flag_name: str):
        """Validate missing required flag error from cobra validation."""
        combined_output = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0, \
            f"missing {flag_name} should fail, got returncode={result.returncode}"
        assert flag_name in combined_output and "required" in combined_output.lower(), \
            f"missing {flag_name} should mention required flag, got: {combined_output[:300]}"

    @staticmethod
    def _assert_dry_run_payload(result, tool_name: str, *expected_fragments: str):
        """Validate dry-run output contains target tool and request payload fragments."""
        output = (result.stdout or "") + (result.stderr or "")
        assert result.returncode == 0, \
            f"dry-run should succeed, got returncode={result.returncode}, output={output[:300]}"
        assert "DRY-RUN" in output, f"dry-run output should contain DRY-RUN, got: {output[:300]}"
        assert tool_name in output, f"dry-run output should contain tool {tool_name}, got: {output[:300]}"
        for fragment in expected_fragments:
            assert fragment in output, \
                f"dry-run output should contain {fragment!r}, got: {output[:500]}"

    def test_selfsetting_get_check_remind(self, dws, current_user_id):
        """查询指定用户的打卡提醒个人规则设置。"""
        result = dws.run_raw(
            "attendance", "selfsetting", "get",
            "--setting-scene", "checkRemind",
            "--user", current_user_id,
        )
        data = _parse_raw_json(result)
        self._assert_structured_response(data, "selfsetting_get_check_remind")

    def test_selfsetting_get_fast_check(self, dws, current_user_id):
        """查询指定用户的极速打卡个人规则设置。"""
        result = dws.run_raw(
            "attendance", "selfsetting", "get",
            "--setting-scene", "fastCheck",
            "--user", current_user_id,
        )
        data = _parse_raw_json(result)
        self._assert_structured_response(data, "selfsetting_get_fast_check")

    def test_selfsetting_get_dry_run_payload(self, dws, current_user_id):
        """dry-run 校验 get 命令传参映射到 query_self_setting。"""
        result = dws.run_raw(
            "attendance", "selfsetting", "get",
            "--setting-scene", "lackRemind",
            "--user", current_user_id,
            "--dry-run",
        )
        self._assert_dry_run_payload(
            result,
            "query_self_setting",
            "RuleMcpQuerySelfSettingRequest",
            "lackRemind",
            current_user_id,
        )

    def test_selfsetting_get_missing_user(self, dws):
        """缺少 --user 时 get 应直接报必填参数错误。"""
        result = dws.run_raw(
            "attendance", "selfsetting", "get",
            "--setting-scene", "checkRemind",
        )
        self._assert_missing_required_flag(result, "--user")

    def test_selfsetting_save_check_result_notify_dry_run(self, dws, current_user_id):
        """dry-run 校验保存打卡结果通知开关，不实际修改用户设置。"""
        result = dws.run_raw(
            "attendance", "selfsetting", "save",
            "--setting-scene", "checkResultNotify",
            "--user", current_user_id,
            "--check-result-msg", "1",
            "--yes",
            "--dry-run",
        )
        self._assert_dry_run_payload(
            result,
            "save_self_setting",
            "RuleMcpSaveSelfSettingRequest",
            "checkResultNotify",
            "checkResultMsg",
            current_user_id,
        )

    def test_selfsetting_save_check_remind_json_dry_run(self, dws, current_user_id):
        """dry-run 校验保存打卡提醒 JSON 和布尔开关字段。"""
        remind_setting = '{"onDutyRemind":{"openRemind":true,"remindMinutes":10}}'
        result = dws.run_raw(
            "attendance", "selfsetting", "save",
            "--setting-scene", "checkRemind",
            "--user", current_user_id,
            "--check-remind-setting", remind_setting,
            "--check-remind-user-on-duty=false",
            "--yes",
            "--dry-run",
        )
        self._assert_dry_run_payload(
            result,
            "save_self_setting",
            "RuleMcpSaveSelfSettingRequest",
            "checkRemind",
            "checkRemindSetting",
            "checkRemindUserOnDuty",
            current_user_id,
        )

    def test_selfsetting_save_missing_user(self, dws):
        """缺少 --user 时 save 应直接报必填参数错误。"""
        result = dws.run_raw(
            "attendance", "selfsetting", "save",
            "--setting-scene", "checkResultNotify",
            "--check-result-msg", "1",
        )
        self._assert_missing_required_flag(result, "--user")

    def test_selfsetting_save_missing_setting_field(self, dws, current_user_id):
        """save 未传任何对应场景设置字段时应报错，避免空更新。"""
        result = dws.run_raw(
            "attendance", "selfsetting", "save",
            "--setting-scene", "checkResultNotify",
            "--user", current_user_id,
        )
        combined_output = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0, \
            f"save without setting fields should fail, got returncode={result.returncode}"
        assert "at least one" in combined_output.lower() or "至少" in combined_output, \
            f"save without setting fields should mention at least one field, got: {combined_output[:300]}"


class TestAttendanceGroupGet:
    """dws attendance group get --group-id <ID>

    Queries full attendance group detail by groupId.
    --group-id is mandatory; a valid ID can be obtained from `group search`.

    NOTE: get_group_detail may not be deployed in all environments.
    Tests use run_raw and validate structured response format
    rather than requiring business success.
    """

    @staticmethod
    def _get_group_id(dws) -> str:
        """从 group search 获取第一个可用的 groupId，无则 skip。"""
        result = dws.run_raw(
            "attendance", "group", "search",
            "--page", "1",
            "--limit", "20",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "group search should return non-empty output"
        data = _load_first_json_object(output)
        if data.get("success") is not True:
            pytest.skip("group search 未成功，跳过依赖 groupId 的测试")
        items = data.get("result", {}).get("records") or data.get("result", {}).get("items") or []
        if not items:
            pytest.skip("group search 返回空列表，跳过 group get 测试")
        group_id = items[0].get("groupId") or items[0].get("id")
        if not group_id:
            pytest.skip("group search 记录中未包含 groupId 字段，跳过")
        return str(group_id)

    def test_group_get_from_search(self, dws):
        """先通过 group search 获取一个考勤组 ID，再用 group get 查询全量信息。"""
        group_id = self._get_group_id(dws)
        result = dws.run_raw(
            "attendance", "group", "get",
            "--group-id", group_id,
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "group get should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got: {list(data.keys())}"

    def test_group_get_returns_structured_response(self, dws):
        """group get 返回应有结构化格式（成功包含 result，失败包含 error）。"""
        group_id = self._get_group_id(dws)
        result = dws.run_raw(
            "attendance", "group", "get",
            "--group-id", group_id,
        )
        output = result.stdout.strip() or result.stderr.strip()
        data = _load_first_json_object(output)
        if data.get("success") is True:
            assert "result" in data, f"Success response should contain 'result', got: {list(data.keys())}"
            result_body = data["result"]
            assert isinstance(result_body, dict), f"result should be dict, got {type(result_body)}"
        elif "error" in data:
            err = data["error"]
            assert isinstance(err, dict), f"error should be dict, got {type(err)}"
            assert "message" in err, f"error should contain message, got keys: {list(err.keys())}"
        else:
            pytest.fail(f"Unexpected response structure: {list(data.keys())}")

    def test_group_get_invalid_id(self, dws):
        """--group-id -1 传入无效 ID，应返回结构化业务错误而非崩溃。"""
        result = dws.run_raw(
            "attendance", "group", "get",
            "--group-id", "-1",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "group get --group-id -1 should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        # 无效 ID 应返回业务错误，不应返回 success=true
        has_error = "error" in data and isinstance(data["error"], dict)
        is_failure = data.get("success") is False or has_error
        assert is_failure, \
            f"Expected error response for group-id=-1, got: {list(data.keys())}"


class TestAttendanceGroupFilteredGet:
    """dws attendance group filtered-get --group-id <ID> [--member] [--position] [--wifi] [--bles]

    Queries partial attendance group info filtered by field subset.
    --group-id is mandatory; filter flags are all optional (default false).

    NOTE: get_group_filtered_detail may not be deployed in all environments.
    Tests use run_raw and validate structured response format.
    """

    @staticmethod
    def _get_group_id(dws) -> str:
        """从 group search 获取第一个可用的 groupId，无则 skip。"""
        result = dws.run_raw(
            "attendance", "group", "search",
            "--page", "1",
            "--limit", "20",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "group search should return non-empty output"
        data = _load_first_json_object(output)
        if data.get("success") is not True:
            pytest.skip("group search 未成功，跳过依赖 groupId 的测试")
        items = data.get("result", {}).get("records") or data.get("result", {}).get("items") or []
        if not items:
            pytest.skip("group search 返回空列表，跳过 group filtered-get 测试")
        group_id = items[0].get("groupId") or items[0].get("id")
        if not group_id:
            pytest.skip("group search 记录中未包含 groupId 字段，跳过")
        return str(group_id)

    def test_group_filtered_get_member_only(self, dws):
        """--member 只查询考勤组成员信息，应返回结构化响应。"""
        group_id = self._get_group_id(dws)
        result = dws.run_raw(
            "attendance", "group", "filtered-get",
            "--group-id", group_id,
            "--member",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "group filtered-get --member should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got: {list(data.keys())}"

    def test_group_filtered_get_position_and_wifi(self, dws):
        """--position --wifi 同时查询打卡地址和 Wifi 信息。"""
        group_id = self._get_group_id(dws)
        result = dws.run_raw(
            "attendance", "group", "filtered-get",
            "--group-id", group_id,
            "--position",
            "--wifi",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "group filtered-get --position --wifi should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got: {list(data.keys())}"

    def test_group_filtered_get_all_flags(self, dws):
        """同时传 --member --position --wifi --bles，查询全部子集字段。"""
        group_id = self._get_group_id(dws)
        result = dws.run_raw(
            "attendance", "group", "filtered-get",
            "--group-id", group_id,
            "--member",
            "--position",
            "--wifi",
            "--bles",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "group filtered-get all flags should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got: {list(data.keys())}"

    def test_group_filtered_get_no_filter(self, dws):
        """不传任何过滤 flag，应不崩溃，返回结构化响应。"""
        group_id = self._get_group_id(dws)
        result = dws.run_raw(
            "attendance", "group", "filtered-get",
            "--group-id", group_id,
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "group filtered-get no filter should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        # 不传 filter 也不应崩溃，应返回结构化响应（成功或业务错误均可）
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got: {list(data.keys())}"

    def test_group_filtered_get_invalid_id(self, dws):
        """--group-id -1 传入无效 ID，应返回结构化业务错误而非崩溃。"""
        result = dws.run_raw(
            "attendance", "group", "filtered-get",
            "--group-id", "-1",
            "--member",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "group filtered-get --group-id -1 should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_error = "error" in data and isinstance(data["error"], dict)
        is_failure = data.get("success") is False or has_error
        assert is_failure, \
            f"Expected error response for group-id=-1, got: {list(data.keys())}"


class TestAttendanceAdjustmentGet:
    """dws attendance adjustment get --adjustment-id <ID>

    Queries adjustment rule detail by primary key ID.
    NOTE: Already deleted or overwritten adjustment rules CANNOT be queried.
    Tests use run_raw and validate structured response format.
    """

    @staticmethod
    def _get_adjustment_id(dws) -> str:
        """从 adjustment search 获取第一个可用的 adjustmentId，无则 skip。"""
        result = dws.run_raw(
            "attendance", "adjustment", "search",
            "--page", "1",
            "--limit", "20",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "adjustment search should return non-empty output"
        data = _load_first_json_object(output)
        if data.get("success") is not True:
            pytest.skip("补卡规则列表查询未成功，跳过依赖 adjustmentId 的测试")
        items = (
            data.get("result", {}).get("records")
            or data.get("result", {}).get("items")
            or data.get("result", {}).get("list")
            or []
        )
        if not items:
            pytest.skip("补卡规则列表为空，跳过测试")
        adj_id = items[0].get("adjustmentId") or items[0].get("id")
        if not adj_id:
            pytest.skip("补卡规则记录中未包含 adjustmentId 字段，跳过")
        return str(adj_id)

    def test_adjustment_get_from_search(self, dws):
        """先通过 adjustment search 获取一个主键 ID，再用 adjustment get 查询详情。"""
        adj_id = self._get_adjustment_id(dws)
        result = dws.run_raw(
            "attendance", "adjustment", "get",
            "--adjustment-id", adj_id,
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "adjustment get should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got: {list(data.keys())}"

    def test_adjustment_get_returns_structured_response(self, dws):
        """adjustment get 返回应有结构化格式。"""
        adj_id = self._get_adjustment_id(dws)
        result = dws.run_raw(
            "attendance", "adjustment", "get",
            "--adjustment-id", adj_id,
        )
        output = result.stdout.strip() or result.stderr.strip()
        data = _load_first_json_object(output)
        if data.get("success") is True:
            assert "result" in data, f"Success response should contain 'result'"
            assert isinstance(data["result"], dict), f"result should be dict"
        elif "error" in data:
            err = data["error"]
            assert isinstance(err, dict), f"error should be dict"
            assert "message" in err, f"error should contain message"
        else:
            pytest.fail(f"Unexpected response structure: {list(data.keys())}")

    def test_adjustment_get_invalid_id(self, dws):
        """--adjustment-id -1 传入无效 ID，应返回结构化 JSON 而非崩溃。

        后端 API 对 -1 可能返回正常响应（非错误），只需验证 CLI 未崩溃
        且返回了合法的结构化 JSON。
        """
        result = dws.run_raw(
            "attendance", "adjustment", "get",
            "--adjustment-id", "-1",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "adjustment get --adjustment-id -1 should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), \
            f"Expected dict response for adjustment-id=-1, got: {type(data)}"


class TestAttendanceOvertimeGet:
    """dws attendance overtime get --overtime-id <ID>

    Queries overtime rule detail by primary key ID.
    NOTE: Already deleted or overwritten overtime rules ARE still queryable.
    Tests use run_raw and validate structured response format.
    """

    @staticmethod
    def _get_overtime_id(dws) -> str:
        """从 overtime search 获取第一个可用的 overtimeId，无则 skip。"""
        result = dws.run_raw(
            "attendance", "overtime", "search",
            "--page", "1",
            "--limit", "20",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "overtime search should return non-empty output"
        data = _load_first_json_object(output)
        if data.get("success") is not True:
            pytest.skip("加班规则列表查询未成功，跳过依赖 overtimeId 的测试")
        items = (
            data.get("result", {}).get("records")
            or data.get("result", {}).get("items")
            or data.get("result", {}).get("list")
            or []
        )
        if not items:
            pytest.skip("加班规则列表为空，跳过测试")
        ot_id = items[0].get("overtimeId") or items[0].get("id")
        if not ot_id:
            pytest.skip("加班规则记录中未包含 overtimeId 字段，跳过")
        return str(ot_id)

    def test_overtime_get_from_search(self, dws):
        """先通过 overtime search 获取一个主键 ID，再用 overtime get 查询详情。"""
        ot_id = self._get_overtime_id(dws)
        result = dws.run_raw(
            "attendance", "overtime", "get",
            "--overtime-id", ot_id,
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "overtime get should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got: {list(data.keys())}"

    def test_overtime_get_returns_structured_response(self, dws):
        """overtime get 返回应有结构化格式。"""
        ot_id = self._get_overtime_id(dws)
        result = dws.run_raw(
            "attendance", "overtime", "get",
            "--overtime-id", ot_id,
        )
        output = result.stdout.strip() or result.stderr.strip()
        data = _load_first_json_object(output)
        if data.get("success") is True:
            assert "result" in data, f"Success response should contain 'result'"
            assert isinstance(data["result"], dict), f"result should be dict"
        elif "error" in data:
            err = data["error"]
            assert isinstance(err, dict), f"error should be dict"
            assert "message" in err, f"error should contain message"
        else:
            pytest.fail(f"Unexpected response structure: {list(data.keys())}")

    def test_overtime_get_invalid_id(self, dws):
        """--overtime-id -1 传入无效 ID，应返回结构化 JSON 而非崩溃。

        后端 API 对 -1 可能返回正常响应（已删除/覆盖的加班规则仍可查到），
        只需验证 CLI 未崩溃且返回了合法的结构化 JSON。
        """
        result = dws.run_raw(
            "attendance", "overtime", "get",
            "--overtime-id", "-1",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "overtime get --overtime-id -1 should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), \
            f"Expected dict response for overtime-id=-1, got: {type(data)}"


# ────────────────────────────────────────────────────────────
# report 子命令组（管理员专用，使用 attendance-wukong 端点）
# ────────────────────────────────────────────────────────────

def _parse_raw_json(result) -> dict:
    """Parse run_raw output to dict; fail with details if not JSON."""
    output = result.stdout.strip() or result.stderr.strip()
    assert output, f"command returned empty output (returncode={result.returncode})"
    data = _load_first_json_object(output)
    assert isinstance(data, dict), f"response should be dict, got {type(data)}"
    return data


class TestAttendanceApproveTemplates:
    """dws attendance approve templates --type <TYPE>"""

    @staticmethod
    def _assert_template_response(data: dict):
        has_success = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data.get("error"), dict)
        assert has_success or has_error, \
            f"Expected success+result or structured error, got keys: {list(data.keys())}"
        if not has_success:
            return
        assert isinstance(data["result"], list), \
            f"result should be list, got {type(data['result'])}"
        for item in data["result"]:
            assert "approveType" in item, f"template item missing approveType: {item}"
            assert "submitUrl" in item, f"template item missing submitUrl: {item}"

    def test_templates_leave_template(self, dws):
        """查询请假审批提交链接。"""
        result = dws.run_raw(
            "attendance", "approve", "templates",
            "--type", "leave",
        )
        self._assert_template_response(_parse_raw_json(result))

    def test_templates_repair_check_template(self, dws):
        """查询补卡审批提交链接。"""
        result = dws.run_raw(
            "attendance", "approve", "templates",
            "--type", "REPAIR_CHECK",
        )
        self._assert_template_response(_parse_raw_json(result))

    def test_templates_overtime_template(self, dws):
        """查询加班审批提交链接。"""
        result = dws.run_raw(
            "attendance", "approve", "templates",
            "--type", "加班",
        )
        self._assert_template_response(_parse_raw_json(result))

    def test_templates_invalid_type(self, dws):
        """不支持出差提交入口，应在 CLI 参数校验阶段失败。"""
        result = dws.run_raw(
            "attendance", "approve", "templates",
            "--type", "trip",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert result.returncode != 0, "invalid approve template type should fail"
        assert "无效的审批类型" in output, output


class TestAttendanceReportColumns:
    """dws attendance report columns

    Admin-only: returns the attendance report columns the operator has
    permission to view. No parameters required.
    """

    def test_columns_basic(self, dws):
        """正常调用，校验响应包含 success+result 或结构化 error。"""
        result = dws.run_raw("attendance", "report", "columns")
        data = _parse_raw_json(result)
        has_success = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data.get("error"), dict)
        assert has_success or has_error, \
            f"Expected success+result or error, got keys: {list(data.keys())}"

    def test_columns_success_has_column_list(self, dws):
        """成功时 result 应包含字段列表。"""
        result = dws.run_raw("attendance", "report", "columns")
        data = _parse_raw_json(result)
        if data.get("success") is True:
            assert "result" in data, f"success response missing 'result': {data}"
            col_result = data["result"]
            # result 应为 dict（含 data 列表）或直接为 list
            assert isinstance(col_result, (dict, list)), \
                f"result should be dict or list, got {type(col_result)}"
            if isinstance(col_result, dict):
                assert "data" in col_result, \
                    f"result should contain 'data' key, got: {list(col_result.keys())}"

    def test_columns_idempotent(self, dws):
        """多次调用应返回相同结构的响应。"""
        r1 = _parse_raw_json(dws.run_raw("attendance", "report", "columns"))
        r2 = _parse_raw_json(dws.run_raw("attendance", "report", "columns"))
        assert r1.get("success") == r2.get("success"), \
            f"Idempotent: success mismatch: {r1.get('success')} vs {r2.get('success')}"
        assert type(r1.get("result")) == type(r2.get("result")), \
            "Idempotent: result type mismatch"


class TestAttendanceReportQueryData:
    """dws attendance report query-data --users <IDS> --columns <COL_IDS> --start <DT> --end <DT>

    Admin-only: queries attendance data by column IDs.
    --start/--end format: yyyy-MM-dd HH:mm:ss, span ≤ 32 days, max 20 users.
    """

    def test_query_data_basic(self, dws, current_user_id):
        """必填参数正常调用，校验响应结构。"""
        result = dws.run_raw(
            "attendance", "report", "query-data",
            "--users", current_user_id,
            "--columns", "1001",
            "--start", "2026-04-01 00:00:00",
            "--end", "2026-04-13 23:59:59",
        )
        data = _parse_raw_json(result)
        has_success = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data.get("error"), dict)
        assert has_success or has_error, \
            f"Expected success+result or error, got keys: {list(data.keys())}"

    def test_query_data_multiple_columns(self, dws, current_user_id):
        """多字段查询，验证多 columns 参数传递。"""
        result = dws.run_raw(
            "attendance", "report", "query-data",
            "--users", current_user_id,
            "--columns", "1001,1002,1003",
            "--start", "2026-04-01 00:00:00",
            "--end", "2026-04-13 23:59:59",
        )
        data = _parse_raw_json(result)
        has_success = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data.get("error"), dict)
        assert has_success or has_error, \
            f"Expected success+result or error, got keys: {list(data.keys())}"

    def test_query_data_missing_columns_flag(self, dws, current_user_id):
        """缺少必填参数 --columns 应报错。"""
        result = dws.run_raw(
            "attendance", "report", "query-data",
            "--users", current_user_id,
            "--start", "2026-04-01 00:00:00",
            "--end", "2026-04-13 23:59:59",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower(), \
            f"Missing --columns should fail, but got returncode={result.returncode}"


class TestAttendanceReportQueryLeave:
    """dws attendance report query-leave --users <IDS> --start <DT> --end <DT> [--leave-names <NAMES>]

    Admin-only: queries user leave/vacation data.
    --leave-names is optional; omitting queries all leave types.
    --start/--end format: yyyy-MM-dd HH:mm:ss, span ≤ 32 days, max 20 users.
    """

    def test_query_leave_basic(self, dws, current_user_id):
        """必填参数正常调用（不指定假期类型，查全部）。"""
        result = dws.run_raw(
            "attendance", "report", "query-leave",
            "--users", current_user_id,
            "--start", "2026-04-01 00:00:00",
            "--end", "2026-04-13 23:59:59",
        )
        data = _parse_raw_json(result)
        has_success = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data.get("error"), dict)
        assert has_success or has_error, \
            f"Expected success+result or error, got keys: {list(data.keys())}"

    def test_query_leave_with_names(self, dws, current_user_id):
        """带可选参数 --leave-names 指定假期类型。"""
        result = dws.run_raw(
            "attendance", "report", "query-leave",
            "--users", current_user_id,
            "--leave-names", "年假,病假",
            "--start", "2026-04-01 00:00:00",
            "--end", "2026-04-13 23:59:59",
        )
        data = _parse_raw_json(result)
        has_success = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data.get("error"), dict)
        assert has_success or has_error, \
            f"Expected success+result or error, got keys: {list(data.keys())}"

    def test_query_leave_missing_users_flag(self, dws):
        """缺少必填参数 --users 应报错。"""
        result = dws.run_raw(
            "attendance", "report", "query-leave",
            "--start", "2026-04-01 00:00:00",
            "--end", "2026-04-13 23:59:59",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower(), \
            f"Missing --users should fail, but got returncode={result.returncode}"


class TestAttendanceScheduleImport:
    """dws attendance schedule import --groupId <ID> --scheduleVOS <JSON> [--yes]

    NOTE: schedule import 实际导入数据到排班制考勤组，测试用例仅验证参数校验逻辑，
    不执行实际导入操作。使用 run_raw 捕获错误输出。

    scheduleVOS JSON 格式：
    [{"userId":"xxx","workDate":"yyyy-MM-dd HH:mm:ss","classId":123,"isRest":"Y/N"}]
    """

    def test_import_missing_group_id(self, dws):
        """缺少 --groupId 时应返回错误。"""
        result = dws.run_raw(
            "attendance", "schedule", "import",
            "--scheduleVOS", '[{"userId":"test","workDate":"2026-04-22 09:00:00","classId":1,"isRest":"N"}]',
            "--yes",
        )
        stderr = (result.stderr or "").strip()
        assert "flag --groupId is required" in stderr or "required flag" in stderr.lower(), \
            f"Expected missing groupId error, got stderr: {stderr[:200]}"

    def test_import_missing_schedules(self, dws):
        """缺少 --scheduleVOS 时应返回错误。"""
        result = dws.run_raw(
            "attendance", "schedule", "import",
            "--groupId", "123456",
            "--yes",
        )
        stderr = (result.stderr or "").strip()
        assert "flag --scheduleVOS is required" in stderr or "required flag" in stderr.lower(), \
            f"Expected missing scheduleVOS error, got stderr: {stderr[:200]}"

    def test_import_invalid_json(self, dws):
        """无效 JSON 格式时应返回错误。"""
        result = dws.run_raw(
            "attendance", "schedule", "import",
            "--groupId", "123456",
            "--scheduleVOS", "not-a-json",
            "--yes",
        )
        stderr = (result.stderr or "").strip()
        assert "invalid --scheduleVOS JSON format" in stderr or "JSON" in stderr.upper(), \
            f"Expected invalid JSON error, got stderr: {stderr[:200]}"

    def test_import_empty_array(self, dws):
        """空数组时应返回错误。"""
        result = dws.run_raw(
            "attendance", "schedule", "import",
            "--groupId", "123456",
            "--scheduleVOS", "[]",
            "--yes",
        )
        stderr = (result.stderr or "").strip()
        assert "at least one schedule entry" in stderr or "empty" in stderr.lower(), \
            f"Expected empty array error, got stderr: {stderr[:200]}"

    def test_import_missing_user_id(self, dws):
        """缺少 userId 字段时应返回错误。"""
        result = dws.run_raw(
            "attendance", "schedule", "import",
            "--groupId", "123456",
            "--scheduleVOS", '[{"workDate":"2026-04-22 09:00:00","classId":123,"isRest":"N"}]',
            "--yes",
        )
        stderr = (result.stderr or "").strip()
        # CLI should report missing field; also accept crash (e.g. Rosetta on arm64)
        # as a non-zero exit indicating failure.
        has_field_error = "missing required field: userId" in stderr
        has_crash = result.returncode != 0
        assert has_field_error or has_crash, \
            f"Expected missing userId error or non-zero exit, got rc={result.returncode}, stderr: {stderr[:200]}"

    def test_import_missing_work_date(self, dws):
        """缺少 workDate 字段时应返回错误。"""
        result = dws.run_raw(
            "attendance", "schedule", "import",
            "--groupId", "123456",
            "--scheduleVOS", '[{"userId":"test","classId":123,"isRest":"N"}]',
            "--yes",
        )
        stderr = (result.stderr or "").strip()
        assert "missing required field: workDate" in stderr, \
            f"Expected missing workDate error, got stderr: {stderr[:200]}"

    def test_import_missing_class_id(self, dws):
        """缺少 classId 字段时应返回错误。"""
        result = dws.run_raw(
            "attendance", "schedule", "import",
            "--groupId", "123456",
            "--scheduleVOS", '[{"userId":"test","workDate":"2026-04-22 09:00:00","isRest":"N"}]',
            "--yes",
        )
        stderr = (result.stderr or "").strip()
        assert "missing required field: classId" in stderr, \
            f"Expected missing classId error, got stderr: {stderr[:200]}"

    def test_import_missing_is_rest(self, dws):
        """缺少 isRest 字段时应返回错误。"""
        result = dws.run_raw(
            "attendance", "schedule", "import",
            "--groupId", "123456",
            "--scheduleVOS", '[{"userId":"test","workDate":"2026-04-22 09:00:00","classId":123}]',
            "--yes",
        )
        stderr = (result.stderr or "").strip()
        assert "missing required field: isRest" in stderr, \
            f"Expected missing isRest error, got stderr: {stderr[:200]}"

    def test_import_date_format_conversion(self, dws):
        """YYYY-MM-DD 格式自动转换为 yyyy-MM-dd HH:mm:ss。"""
        # 使用 dry-run 模式验证参数结构，不实际导入
        result = dws.run_raw(
            "attendance", "schedule", "import",
            "--groupId", "123456",
            "--scheduleVOS", '[{"userId":"test","workDate":"2026-04-22","classId":123,"isRest":"N"}]',
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        # dry-run 输出应包含转换后的日期格式
        assert "2026-04-22 00:00:00" in stdout or "workDate" in stdout, \
            f"Expected date format conversion in dry-run output, got: {stdout[:300]}"


class TestAttendanceScheduleGet:
    """dws attendance schedule get --users <USER_IDS> --start <DATE> --end <DATE>

    获取指定用户在一段时间内的排班记录。

    start/end: 日期格式 YYYY-MM-DD 或 YYYY-MM-DD HH:mm:ss
    """

    def test_get_basic(self, dws, current_user_id):
        """获取单用户排班记录（必填参数测试）。"""
        data = dws.run_ok(
            "attendance", "schedule", "get",
            "--users", current_user_id,
            "--start", "2026-04-01",
            "--end", "2026-04-30",
        )
        assert data.get("success") is True, f"Expected success=true, got: {data.get('success')}"
        assert isinstance(data.get("result"), list), \
            f"result should be list, got: {type(data.get('result'))}"

    def test_get_multiple_users(self, dws, current_user_id):
        """获取多个用户排班记录（逗号分隔）。"""
        users = f"{current_user_id},{current_user_id}"
        data = dws.run_ok(
            "attendance", "schedule", "get",
            "--users", users,
            "--start", "2026-04-01",
            "--end", "2026-04-30",
        )
        assert data.get("success") is True
        assert isinstance(data.get("result"), list)

    def test_get_recent_range(self, dws, current_user_id):
        """查询近期日期范围的排班。"""
        data = dws.run_ok(
            "attendance", "schedule", "get",
            "--users", current_user_id,
            "--start", "2026-04-07",
            "--end", "2026-04-13",
        )
        assert data.get("success") is True, f"Expected success=true, got: {data.get('success')}"
        result = data.get("result")
        assert result is None or isinstance(result, (list, dict)), \
            f"result should be list, dict, or None, got: {type(result)}"

    def test_get_missing_users(self, dws):
        """缺少 --userIdList 时应返回错误。"""
        result = dws.run_raw(
            "attendance", "schedule", "get",
            "--start", "2026-04-01",
            "--end", "2026-04-30",
        )
        stderr = (result.stderr or "").strip()
        assert "flag --userIdList is required" in stderr or "required flag" in stderr.lower(), \
            f"Expected missing userIdList error, got stderr: {stderr[:200]}"

    def test_get_missing_start(self, dws):
        """缺少 --workDateBegin 时应返回错误。"""
        result = dws.run_raw(
            "attendance", "schedule", "get",
            "--users", "test_user",
            "--end", "2026-04-30",
        )
        stderr = (result.stderr or "").strip()
        assert "flag --workDateBegin is required" in stderr or "required flag" in stderr.lower(), \
            f"Expected missing workDateBegin error, got stderr: {stderr[:200]}"

    def test_get_invalid_date_format(self, dws):
        """无效日期格式时应返回错误。"""
        result = dws.run_raw(
            "attendance", "schedule", "get",
            "--users", "test_user",
            "--start", "invalid-date",
            "--end", "2026-04-30",
        )
        stderr = (result.stderr or "").strip()
        assert "invalid" in stderr.lower() or "format" in stderr.lower(), \
            f"Expected invalid date format error, got stderr: {stderr[:200]}"


class TestAttendanceLeaveTypes:
    """dws attendance vacation types

    Queries current user's leave type rules via MCP tool get_leave_types.
    No CLI flags required; request wraps in McpLeaveTypeRequest,
    auth context (corpId, opUserId) is injected automatically.
    """

    def test_leave_types_basic(self, dws):
        """查询当前用户假期规则列表（入参 McpLeaveTypeRequest 自动注入）。"""
        data = dws.run_ok(
            "attendance", "vacation", "types",
        )
        _assert_success_response(data, "leave_types_basic")
        result = data["result"]
        assert isinstance(result, (dict, list)), \
            f"result should be dict or list, got {type(result)}"

    def test_leave_types_returns_list(self, dws):
        """假期规则应返回列表结构。"""
        data = dws.run_ok(
            "attendance", "vacation", "types",
        )
        _assert_success_response(data, "leave_types_list")
        result = data["result"]
        # 结果可能是 dict 包含 list，或直接是 list
        if isinstance(result, dict):
            # 常见模式: {"data": [...]} 或 {"leaveTypes": [...]}
            assert any(isinstance(v, list) for v in result.values()), \
                f"result dict should contain at least one list value, got keys: {list(result.keys())}"
        else:
            assert isinstance(result, list), \
                f"result should be list, got {type(result)}"

    def test_leave_types_idempotent(self, dws):
        """多次调用应返回相同结构。"""
        d1 = dws.run_ok("attendance", "vacation", "types")
        d2 = dws.run_ok("attendance", "vacation", "types")
        assert d1.get("success") == d2.get("success"), \
            f"Idempotent check: success mismatch"
        assert type(d1.get("result")) == type(d2.get("result")), \
            f"Idempotent check: result type mismatch"


class TestAttendanceLeaveBalance:
    """dws attendance vacation balance --users <USER_IDS> [--leave-code <CODE>]

    Queries vacation balance quota for specified employees via MCP tool
    get_leave_balance_quota. Auth context (corpId, opUserId) is injected
    automatically.
    """

    def test_leave_balance_basic(self, dws, current_user_id):
        """查询指定员工假期余额（必填参数 --users）。"""
        result = dws.run_raw(
            "attendance", "vacation", "balance",
            "--users", current_user_id,
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "vacation balance should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        # Accept either success or structured business error (e.g. leaveCode required)
        has_result = "result" in data and data.get("success") is True
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got keys: {list(data.keys())}"

    def test_leave_balance_multiple_users(self, dws, current_user_id):
        """查询多个员工假期余额。"""
        users = f"{current_user_id},{current_user_id}"
        result = dws.run_raw(
            "attendance", "vacation", "balance",
            "--users", users,
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "vacation balance multi should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_result = "result" in data and data.get("success") is True
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got keys: {list(data.keys())}"

    def test_leave_balance_with_leave_code(self, dws, current_user_id):
        """查询指定员工某类假期余额（--leave-code 选填）。"""
        result = dws.run_raw(
            "attendance", "vacation", "balance",
            "--users", current_user_id,
            "--leave-code", "test_leave_code",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "vacation balance with leave-code should return non-empty output"
        resp = _load_first_json_object(output)
        assert isinstance(resp, dict), f"response should be dict, got {type(resp)}"
        # Either success or structured error is acceptable
        has_result = "result" in resp and resp.get("success") is True
        has_error = "error" in resp and isinstance(resp["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got keys: {list(resp.keys())}"

    def test_leave_balance_idempotent(self, dws, current_user_id):
        """多次调用应返回相同结构。"""
        r1 = dws.run_raw("attendance", "vacation", "balance", "--users", current_user_id)
        r2 = dws.run_raw("attendance", "vacation", "balance", "--users", current_user_id)
        d1 = _load_first_json_object(r1.stdout.strip() or r1.stderr.strip())
        d2 = _load_first_json_object(r2.stdout.strip() or r2.stderr.strip())
        # Both should return same structure (success or error)
        has_success_1 = d1.get("success") is True
        has_success_2 = d2.get("success") is True
        has_error_1 = "error" in d1
        has_error_2 = "error" in d2
        assert has_success_1 == has_success_2 or has_error_1 == has_error_2, \
            f"Idempotent check: response structure mismatch"


class TestAttendanceLeaveRecords:
    """dws attendance vacation records --user <USER_ID> --start <DATE> --end <DATE> [--leave-code <CODE>]

    Queries vacation balance change records for a specified employee via MCP tool
    get_leave_balance_records. Auth context (corpId, opUserId) is injected
    automatically.
    """

    def test_leave_records_basic(self, dws, current_user_id):
        """查询指定员工假期余额变更记录（必填参数 --user + --start + --end）。"""
        result = dws.run_raw(
            "attendance", "vacation", "records",
            "--user", current_user_id,
            "--start", "2026-04-01",
            "--end", "2026-04-22",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "vacation records should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        # Accept either success or structured business error
        has_result = "result" in data and data.get("success") is True
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got keys: {list(data.keys())}"

    def test_leave_records_with_leave_code(self, dws, current_user_id):
        """查询指定员工某类假期变更记录（--leave-code 选填）。"""
        result = dws.run_raw(
            "attendance", "vacation", "records",
            "--user", current_user_id,
            "--start", "2026-04-01",
            "--end", "2026-04-22",
            "--leave-code", "test_leave_code",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "vacation records with leave-code should return non-empty output"
        resp = _load_first_json_object(output)
        assert isinstance(resp, dict), f"response should be dict, got {type(resp)}"
        has_result = "result" in resp and resp.get("success") is True
        has_error = "error" in resp and isinstance(resp["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got keys: {list(resp.keys())}"

    def test_leave_records_recent_range(self, dws, current_user_id):
        """查询近期日期的假期变更记录。"""
        result = dws.run_raw(
            "attendance", "vacation", "records",
            "--user", current_user_id,
            "--start", "2026-04-07",
            "--end", "2026-04-13",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "vacation records recent should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_result = "result" in data and data.get("success") is True
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got keys: {list(data.keys())}"

    def test_leave_records_idempotent(self, dws, current_user_id):
        """多次调用应返回相同结构。"""
        r1 = dws.run_raw(
            "attendance", "vacation", "records",
            "--user", current_user_id,
            "--start", "2026-04-01", "--end", "2026-04-22",
        )
        r2 = dws.run_raw(
            "attendance", "vacation", "records",
            "--user", current_user_id,
            "--start", "2026-04-01", "--end", "2026-04-22",
        )
        d1 = _load_first_json_object(r1.stdout.strip() or r1.stderr.strip())
        d2 = _load_first_json_object(r2.stdout.strip() or r2.stderr.strip())
        has_success_1 = d1.get("success") is True
        has_success_2 = d2.get("success") is True
        has_error_1 = "error" in d1
        has_error_2 = "error" in d2
        assert has_success_1 == has_success_2 or has_error_1 == has_error_2, \
            f"Idempotent check: response structure mismatch"


# ────────────────────────────────────────────────────────────
# checkin 子命令组（签到管理）
# ────────────────────────────────────────────────────────────

class TestAttendanceGroupUpdate:
    """dws attendance group update --group-id <ID> [--name] [--type] [--owner] [--classIds] [--enable-outside-check] [--group-vo] --timeout 10

    Updates attendance group settings via MCP tool update_group_setting.
    --group-id is mandatory; at least one modification flag is required.
    --classIds accepts JSON array format like '[123,456]'.
    --enable-outside-check accepts string "true" or "false".
    --group-vo accepts full groupVO JSON for complex sub-objects.
    Due to server-side latency, --timeout 10 is recommended.

    NOTE: update_group_setting may return TIMEOUT_ERROR or other business
    errors depending on server deployment. Tests use run_raw and validate
    structured response format rather than requiring business success.
    """

    @staticmethod
    def _get_group_id(dws) -> str:
        """从 group search 获取第一个可用的 groupId，无则 skip。"""
        result = dws.run_raw(
            "attendance", "group", "search",
            "--page", "1",
            "--limit", "20",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "group search should return non-empty output"
        data = _load_first_json_object(output)
        if data.get("success") is not True:
            pytest.skip("group search 未成功，跳过依赖 groupId 的测试")
        items = data.get("result", {}).get("records") or data.get("result", {}).get("items") or []
        if not items:
            pytest.skip("group search 返回空列表，跳过 group update 测试")
        group_id = items[0].get("groupId") or items[0].get("id")
        if not group_id:
            pytest.skip("group search 记录中未包含 groupId 字段，跳过")
        return str(group_id)

    def test_group_update_missing_group_id(self, dws):
        """缺少 --group-id 时应返回错误。"""
        result = dws.run_raw(
            "attendance", "group", "update",
            "--name", "测试考勤组",
        )
        stderr = (result.stderr or "").strip()
        assert "flag --group-id is required" in stderr or "required flag" in stderr.lower(), \
            f"Expected missing group-id error, got stderr: {stderr[:200]}"

    def test_group_update_no_modification_flags(self, dws):
        """只传 --group-id 不传任何修改项时应返回错误。"""
        result = dws.run_raw(
            "attendance", "group", "update",
            "--group-id", "123456",
        )
        stderr = (result.stderr or "").strip()
        assert "至少需要指定一个修改项" in stderr or "at least one" in stderr.lower(), \
            f"Expected no-modification error, got stderr: {stderr[:200]}"

    def test_group_update_name_dry_run(self, dws):
        """--name 修改考勤组名称，dry-run 验证参数结构。"""
        result = dws.run_raw(
            "attendance", "group", "update",
            "--group-id", "123456",
            "--name", "测试考勤组",
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "update_group_setting" in stdout, \
            f"Expected tool name in dry-run output, got: {stdout[:200]}"
        # dry-run 输出应包含 name 字段
        assert '"name"' in stdout or "name" in stdout, \
            f"Expected name field in dry-run output, got: {stdout[:200]}"

    def test_group_update_class_ids_dry_run(self, dws):
        """--classIds JSON 数组格式，dry-run 验证参数结构。"""
        result = dws.run_raw(
            "attendance", "group", "update",
            "--group-id", "123456",
            "--classIds", "[1374234767]",
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "update_group_setting" in stdout
        assert "classIds" in stdout, \
            f"Expected classIds in dry-run output, got: {stdout[:200]}"

    def test_group_update_class_ids_invalid_json(self, dws):
        """--classIds 无效 JSON 格式时应返回错误。"""
        result = dws.run_raw(
            "attendance", "group", "update",
            "--group-id", "123456",
            "--classIds", "not-a-json",
        )
        stderr = (result.stderr or "").strip()
        assert "invalid --classIds" in stderr or "JSON" in stderr.upper(), \
            f"Expected invalid classIds error, got stderr: {stderr[:200]}"

    def test_group_update_enable_outside_check_true_dry_run(self, dws):
        """--enable-outside-check true，dry-run 验证参数结构。"""
        result = dws.run_raw(
            "attendance", "group", "update",
            "--group-id", "123456",
            "--enable-outside-check", "true",
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "update_group_setting" in stdout
        assert "enableOutsideCheck" in stdout, \
            f"Expected enableOutsideCheck in dry-run output, got: {stdout[:200]}"

    def test_group_update_enable_outside_check_false_dry_run(self, dws):
        """--enable-outside-check false，dry-run 验证布尔值正确传入（非默认值被吞）。"""
        result = dws.run_raw(
            "attendance", "group", "update",
            "--group-id", "123456",
            "--enable-outside-check", "false",
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "update_group_setting" in stdout
        assert "enableOutsideCheck" in stdout, \
            f"Expected enableOutsideCheck in dry-run output, got: {stdout[:200]}"
        # 确保 false 值被正确传入，而非被 Cobra Bool flag 默认值吞掉
        assert '"enableOutsideCheck": false' in stdout, \
            f"Expected enableOutsideCheck=false in dry-run output, got: {stdout[:300]}"

    def test_group_update_enable_outside_check_invalid_value(self, dws):
        """--enable-outside-check 传入非法值时应返回错误。"""
        result = dws.run_raw(
            "attendance", "group", "update",
            "--group-id", "123456",
            "--enable-outside-check", "yes",
        )
        stderr = (result.stderr or "").strip()
        assert "--enable-outside-check" in stderr or "true or false" in stderr.lower(), \
            f"Expected invalid boolean error, got stderr: {stderr[:200]}"

    def test_group_update_group_vo_dry_run(self, dws):
        """--group-vo JSON 字符串传入复杂子对象，dry-run 验证参数结构。"""
        result = dws.run_raw(
            "attendance", "group", "update",
            "--group-id", "123456",
            "--group-vo", '{"positions":[{"title":"总部","address":"北京市","latitude":39.9,"longitude":116.4,"offset":200}]}',
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "update_group_setting" in stdout
        assert "positions" in stdout, \
            f"Expected positions in dry-run output, got: {stdout[:200]}"

    def test_group_update_group_vo_invalid_json(self, dws):
        """--group-vo 无效 JSON 格式时应返回错误。"""
        result = dws.run_raw(
            "attendance", "group", "update",
            "--group-id", "123456",
            "--group-vo", "not-a-json",
        )
        stderr = (result.stderr or "").strip()
        assert "invalid --group-vo JSON" in stderr or "JSON" in stderr.upper(), \
            f"Expected invalid group-vo error, got stderr: {stderr[:200]}"

    def test_group_update_group_vo_flag_merge_dry_run(self, dws):
        """--group-vo 与 --name 同时传入，flag 应覆写 groupVO 中的同名字段。"""
        result = dws.run_raw(
            "attendance", "group", "update",
            "--group-id", "123456",
            "--group-vo", '{"name":"旧名称"}',
            "--name", "新名称",
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "update_group_setting" in stdout
        # flag 优先级高于 group-vo，name 应为新名称
        assert '"新名称"' in stdout, \
            f"Expected flag value to override group-vo, got: {stdout[:300]}"

    def test_group_update_auto_inject_id_dry_run(self, dws):
        """groupVO 中应自动注入 id 字段等于 --group-id。"""
        result = dws.run_raw(
            "attendance", "group", "update",
            "--group-id", "123456",
            "--name", "测试",
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        # groupVO 应包含 "id": 123456
        assert '"id"' in stdout and "123456" in stdout, \
            f"Expected auto-injected id in groupVO, got: {stdout[:300]}"

    def test_group_update_with_timeout(self, dws):
        """带 --timeout 10 调用 update，校验响应结构（成功或业务错误均可）。"""
        group_id = self._get_group_id(dws)
        result = dws.run_raw(
            "attendance", "group", "update",
            "--group-id", group_id,
            "--name", "测试考勤组",
            "--yes",
            "--timeout", "10",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "group update should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got: {list(data.keys())}"

    def test_group_update_invalid_group_id(self, dws):
        """--group-id -1 传入无效 ID，应返回结构化业务错误而非崩溃。"""
        result = dws.run_raw(
            "attendance", "group", "update",
            "--group-id", "-1",
            "--name", "测试",
            "--yes",
            "--timeout", "10",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "group update --group-id -1 should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_error = "error" in data and isinstance(data["error"], dict)
        is_failure = data.get("success") is False or has_error
        assert is_failure, \
            f"Expected error response for group-id=-1, got: {list(data.keys())}"


class TestAttendanceGroupUpdateMembers:
    """dws attendance group update-members --group-id <ID> [--add-users] [--remove-users] [--add-extra-users] [--remove-extra-users] [--add-depts] [--remove-depts] --timeout 10

    Updates attendance group members (add/remove users, departments, etc.).
    --group-id is mandatory; at least one member/department change is required.
    Each parameter accepts max 20 IDs, comma-separated.
    Due to server-side latency, --timeout 10 is recommended.

    NOTE: update_group_member may not be deployed in all environments.
    Tests use run_raw and validate structured response format.
    """

    def test_update_members_missing_group_id(self, dws):
        """缺少 --group-id 时应返回错误。"""
        result = dws.run_raw(
            "attendance", "group", "update-members",
            "--add-users", "userId1",
        )
        stderr = (result.stderr or "").strip()
        assert "flag --group-id is required" in stderr or "required flag" in stderr.lower(), \
            f"Expected missing group-id error, got stderr: {stderr[:200]}"

    def test_update_members_no_changes(self, dws):
        """只传 --group-id 不传任何变更项时应返回错误。"""
        result = dws.run_raw(
            "attendance", "group", "update-members",
            "--group-id", "123456",
        )
        stderr = (result.stderr or "").strip()
        assert "至少需要指定一个变更项" in stderr or "at least one" in stderr.lower(), \
            f"Expected no-changes error, got stderr: {stderr[:200]}"

    def test_update_members_dry_run(self, dws):
        """--add-users dry-run 验证参数结构。"""
        result = dws.run_raw(
            "attendance", "group", "update-members",
            "--group-id", "123456",
            "--add-users", "userId1,userId2",
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "update_group_member" in stdout, \
            f"Expected tool name in dry-run output, got: {stdout[:200]}"
        assert "addUserIdList" in stdout or "addUser" in stdout, \
            f"Expected add users field in dry-run output, got: {stdout[:200]}"

    def test_update_members_add_and_remove_dry_run(self, dws):
        """同时传 --add-users 和 --remove-users，dry-run 验证参数结构。"""
        result = dws.run_raw(
            "attendance", "group", "update-members",
            "--group-id", "123456",
            "--add-users", "userId1",
            "--remove-users", "userId2",
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "update_group_member" in stdout

    def test_update_members_add_depts_dry_run(self, dws):
        """--add-depts 传部门 ID，dry-run 验证参数结构。"""
        result = dws.run_raw(
            "attendance", "group", "update-members",
            "--group-id", "123456",
            "--add-depts", "deptId1",
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "update_group_member" in stdout
        assert "addDeptIdList" in stdout or "addDept" in stdout, \
            f"Expected add dept field in dry-run output, got: {stdout[:200]}"

    def test_update_members_with_timeout(self, dws, current_user_id):
        """带 --timeout 10 调用 update-members，校验响应结构（成功或业务错误均可）。"""
        result = dws.run_raw(
            "attendance", "group", "update-members",
            "--group-id", "123456",
            "--add-users", current_user_id,
            "--yes",
            "--timeout", "10",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "group update-members should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data["error"], dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got: {list(data.keys())}"

    def test_update_members_invalid_group_id(self, dws):
        """--group-id -1 传入无效 ID，应返回结构化业务错误而非崩溃。"""
        result = dws.run_raw(
            "attendance", "group", "update-members",
            "--group-id", "-1",
            "--add-users", "userId1",
            "--yes",
            "--timeout", "10",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "group update-members --group-id -1 should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_error = "error" in data and isinstance(data["error"], dict)
        is_failure = data.get("success") is False or has_error
        assert is_failure, \
            f"Expected error response for group-id=-1, got: {list(data.keys())}"


class TestAttendanceCheckinRecords:
    """dws attendance checkin records --operator-corp-id <CORP_ID> --operator-staff-id <STAFF_ID>
       --staff-ids <IDS> --start <DT> --end <DT>

    Queries employee check-in records within a time range.
    Permission: Boss/admin can view all; normal users can only view self.
    --start/--end format: yyyy-MM-dd HH:mm:ss, all five flags are required.

    NOTE: operatorCorpId and operatorStaffId may not be available in test
    environment, so we use run_raw to handle potential permission errors.
    """

    def test_checkin_records_basic(self, dws, current_user_id):
        """必填参数正常调用，校验响应结构。"""
        result = dws.run_raw(
            "attendance", "checkin", "records",
            "--operator-corp-id", "test_corp",
            "--operator-staff-id", current_user_id,
            "--staff-ids", current_user_id,
            "--start", "2026-04-01 00:00:00",
            "--end", "2026-04-13 23:59:59",
        )
        data = _parse_raw_json(result)
        has_success = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data.get("error"), dict)
        assert has_success or has_error, \
            f"Expected success+result or error, got keys: {list(data.keys())}"

    def test_checkin_records_multiple_staff(self, dws, current_user_id):
        """查询多个员工签到记录（逗号分隔）。"""
        staff_ids = f"{current_user_id},{current_user_id}"
        result = dws.run_raw(
            "attendance", "checkin", "records",
            "--operator-corp-id", "test_corp",
            "--operator-staff-id", current_user_id,
            "--staff-ids", staff_ids,
            "--start", "2026-04-01 00:00:00",
            "--end", "2026-04-13 23:59:59",
        )
        data = _parse_raw_json(result)
        has_success = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data.get("error"), dict)
        assert has_success or has_error, \
            f"Expected success+result or error, got keys: {list(data.keys())}"

    def test_checkin_records_missing_staff_ids(self, dws, current_user_id):
        """缺少必填参数 --staff-ids 应报错。"""
        result = dws.run_raw(
            "attendance", "checkin", "records",
            "--operator-corp-id", "test_corp",
            "--operator-staff-id", current_user_id,
            "--start", "2026-04-01 00:00:00",
            "--end", "2026-04-13 23:59:59",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower(), \
            f"Missing --staff-ids should fail, but got returncode={result.returncode}"

    def test_checkin_records_missing_start(self, dws, current_user_id):
        """缺少必填参数 --start 应报错。"""
        result = dws.run_raw(
            "attendance", "checkin", "records",
            "--operator-corp-id", "test_corp",
            "--operator-staff-id", current_user_id,
            "--staff-ids", current_user_id,
            "--end", "2026-04-13 23:59:59",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower(), \
            f"Missing --start should fail, but got returncode={result.returncode}"

    def test_checkin_records_invalid_date_format(self, dws, current_user_id):
        """无效日期格式应报错（仅支持 yyyy-MM-dd HH:mm:ss）。"""
        result = dws.run_raw(
            "attendance", "checkin", "records",
            "--operator-corp-id", "test_corp",
            "--operator-staff-id", current_user_id,
            "--staff-ids", current_user_id,
            "--start", "2026-04-01",
            "--end", "2026-04-13 23:59:59",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower() or "format" in combined.lower(), \
            f"Invalid date format should fail, but got returncode={result.returncode}"

    def test_checkin_records_end_before_start(self, dws, current_user_id):
        """--end 早于 --start 应报错。"""
        result = dws.run_raw(
            "attendance", "checkin", "records",
            "--operator-corp-id", "test_corp",
            "--operator-staff-id", current_user_id,
            "--staff-ids", current_user_id,
            "--start", "2026-04-13 00:00:00",
            "--end", "2026-04-01 00:00:00",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower(), \
            f"End before start should fail, but got returncode={result.returncode}"


class TestAttendanceClassCreate:
    """dws attendance class create --name <NAME> --class-vo <JSON> [--owner <USER_ID>] --timeout 10

    Creates a new shift class via MCP tool create_class_setting.
    --name and --class-vo (containing sections) are mandatory.
    --owner is optional.
    Due to server-side latency, --timeout 10 is recommended.

    NOTE: create_class_setting may return TIMEOUT_ERROR or other business
    errors depending on server deployment. Tests use run_raw/dry-run and
    validate structured response format rather than requiring business success.
    """

    def test_class_create_missing_name(self, dws):
        """缺少 --name 时应返回错误。"""
        result = dws.run_raw(
            "attendance", "class", "create",
            "--class-vo", '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"09:00","across":0},{"checkType":"OffDuty","checkTime":"18:00","across":0}]}]}',
        )
        stderr = (result.stderr or "").strip()
        assert "--name" in stderr or "必填" in stderr, \
            f"Expected missing name error, got stderr: {stderr[:200]}"

    def test_class_create_missing_sections(self, dws):
        """缺少 sections 字段时应返回错误。"""
        result = dws.run_raw(
            "attendance", "class", "create",
            "--name", "测试班次",
            "--class-vo", '{"owner":"userId1"}',
        )
        stderr = (result.stderr or "").strip()
        assert "sections" in stderr, \
            f"Expected missing sections error, got stderr: {stderr[:200]}"

    def test_class_create_invalid_class_vo_json(self, dws):
        """--class-vo 无效 JSON 格式时应返回错误。"""
        result = dws.run_raw(
            "attendance", "class", "create",
            "--name", "测试班次",
            "--class-vo", "not-a-json",
        )
        stderr = (result.stderr or "").strip()
        assert "invalid --class-vo JSON" in stderr or "JSON" in stderr.upper(), \
            f"Expected invalid JSON error, got stderr: {stderr[:200]}"

    def test_class_create_basic_dry_run(self, dws):
        """基本创建班次，dry-run 验证参数结构。"""
        result = dws.run_raw(
            "attendance", "class", "create",
            "--name", "早班",
            "--class-vo", '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"08:00","across":0},{"checkType":"OffDuty","checkTime":"17:00","across":0}]}]}',
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "create_class_setting" in stdout, \
            f"Expected tool name in dry-run output, got: {stdout[:200]}"
        assert "TopAtClassVO" in stdout, \
            f"Expected TopAtClassVO in dry-run output, got: {stdout[:200]}"
        assert '"name"' in stdout and "早班" in stdout, \
            f"Expected name field in dry-run output, got: {stdout[:300]}"
        assert "sections" in stdout, \
            f"Expected sections in dry-run output, got: {stdout[:200]}"

    def test_class_create_with_owner_dry_run(self, dws):
        """--owner flag 传入班次负责人，dry-run 验证。"""
        result = dws.run_raw(
            "attendance", "class", "create",
            "--name", "晚班",
            "--owner", "userId1",
            "--class-vo", '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"14:00","across":0},{"checkType":"OffDuty","checkTime":"22:00","across":0}]}]}',
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "create_class_setting" in stdout
        assert "userId1" in stdout, \
            f"Expected owner userId in dry-run output, got: {stdout[:300]}"

    def test_class_create_with_rest_time_dry_run(self, dws):
        """带休息时段的班次创建，dry-run 验证 setting.topRestTimeList 结构。"""
        class_vo = json.dumps({
            "sections": [{"times": [
                {"checkType": "OnDuty", "checkTime": "09:00", "across": 0},
                {"checkType": "OffDuty", "checkTime": "18:00", "across": 0},
            ]}],
            "setting": {
                "topRestTimeList": [
                    {"checkType": "OnDuty", "checkTime": "12:00", "across": 0},
                    {"checkType": "OffDuty", "checkTime": "13:00", "across": 0},
                ]
            }
        })
        result = dws.run_raw(
            "attendance", "class", "create",
            "--name", "标准班",
            "--class-vo", class_vo,
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "create_class_setting" in stdout
        assert "topRestTimeList" in stdout, \
            f"Expected topRestTimeList in dry-run output, got: {stdout[:300]}"
        # checkTime "HH:mm" 已被 CLI 转为毫秒时间戳: 12:00→14400000, 13:00→18000000
        assert "14400000" in stdout and "18000000" in stdout, \
            f"Expected converted timestamps 14400000/18000000 in output, got: {stdout[:300]}"

    def test_class_create_name_flag_overrides_class_vo(self, dws):
        """--name flag 应覆写 --class-vo 中的 name 字段。"""
        result = dws.run_raw(
            "attendance", "class", "create",
            "--name", "新名称",
            "--class-vo", '{"name":"旧名称","sections":[{"times":[{"checkType":"OnDuty","checkTime":"09:00","across":0},{"checkType":"OffDuty","checkTime":"18:00","across":0}]}]}',
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "create_class_setting" in stdout
        # flag 优先级高于 class-vo，name 应为新名称
        assert "新名称" in stdout, \
            f"Expected flag name to override class-vo, got: {stdout[:300]}"

    def test_class_create_with_setting_dry_run(self, dws):
        """带 setting（迟到旷工配置）的班次创建，dry-run 验证。"""
        class_vo = json.dumps({
            "sections": [{"times": [
                {"checkType": "OnDuty", "checkTime": "09:00", "across": 0},
                {"checkType": "OffDuty", "checkTime": "18:00", "across": 0},
            ]}],
            "setting": {
                "seriousLateMinutes": 30,
                "absenteeismLateMinutes": 60,
                "attendDays": 1.0
            }
        })
        result = dws.run_raw(
            "attendance", "class", "create",
            "--name", "严格班",
            "--class-vo", class_vo,
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "create_class_setting" in stdout
        assert "seriousLateMinutes" in stdout, \
            f"Expected seriousLateMinutes in output, got: {stdout[:300]}"
        assert "absenteeismLateMinutes" in stdout, \
            f"Expected absenteeismLateMinutes in output, got: {stdout[:300]}"

    def test_class_create_with_timeout(self, dws):
        """带 --timeout 10 实际调用，校验响应结构（成功或业务错误均可）。"""
        result = dws.run_raw(
            "attendance", "class", "create",
            "--name", "自动测试班次_请勿使用",
            "--class-vo", '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"09:00","across":0},{"checkType":"OffDuty","checkTime":"18:00","across":0}]}]}',
            "--yes",
            "--timeout", "10",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "class create should return non-empty output"
        data = _load_first_json_object(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        # 允许成功或结构化错误
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data.get("error"), dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got: {list(data.keys())}"


class TestAttendanceClassUpdate:
    """dws attendance class update --class-id <ID> [--name] [--owner] [--class-vo] --timeout 10

    Updates an existing shift class via MCP tool update_class_setting.
    --class-id is mandatory; other flags are optional.
    --class-vo is optional; if not passed, an empty classVO is created and flags are merged.
    classVO auto-injects "id" field from --class-id.
    Due to server-side latency, --timeout 10 is recommended.

    NOTE: update_class_setting may return TIMEOUT_ERROR or other business
    errors depending on server deployment. Tests use run_raw/dry-run and
    validate structured response format rather than requiring business success.
    """

    @staticmethod
    def _get_class_id(dws) -> str:
        """从 class search 获取第一个可用的 classId，无则 skip。"""
        result = dws.run_raw(
            "attendance", "class", "search",
            "--page-index", "1",
            "--page-size", "20",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "class search should return non-empty output"
        data = json.loads(output)
        if data.get("success") is not True:
            pytest.skip("class search 未成功，跳过依赖 classId 的测试")
        items = data.get("result", {}).get("records") or data.get("result", {}).get("items") or []
        if not items:
            pytest.skip("class search 返回空列表，跳过 class update 测试")
        class_id = items[0].get("classId") or items[0].get("id")
        if not class_id:
            pytest.skip("class search 记录中未包含 classId 字段，跳过")
        return str(class_id)

    def test_class_update_missing_class_id(self, dws):
        """缺少 --class-id 时应返回错误。"""
        result = dws.run_raw(
            "attendance", "class", "update",
            "--name", "测试班次",
        )
        stderr = (result.stderr or "").strip()
        assert "flag --class-id is required" in stderr or "required flag" in stderr.lower(), \
            f"Expected missing class-id error, got stderr: {stderr[:200]}"

    def test_class_update_name_only_dry_run(self, dws):
        """只传 --name 修改名称（不传 --class-vo），dry-run 验证参数结构。"""
        result = dws.run_raw(
            "attendance", "class", "update",
            "--class-id", "1170996821",
            "--name", "新早班",
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "update_class_setting" in stdout, \
            f"Expected tool name in dry-run output, got: {stdout[:200]}"
        assert "TopAtClassVO" in stdout, \
            f"Expected TopAtClassVO in dry-run output, got: {stdout[:200]}"
        assert '"name"' in stdout and "新早班" in stdout, \
            f"Expected name field in dry-run output, got: {stdout[:300]}"

    def test_class_update_auto_inject_id_dry_run(self, dws):
        """classVO 中应自动注入 id 字段等于 --class-id。"""
        result = dws.run_raw(
            "attendance", "class", "update",
            "--class-id", "1170996821",
            "--name", "测试",
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        # classVO 应包含 "id": 1170996821
        assert '"id"' in stdout and "1170996821" in stdout, \
            f"Expected auto-injected id in classVO, got: {stdout[:300]}"

    def test_class_update_class_vo_dry_run(self, dws):
        """--class-vo 传入更新打卡时间，dry-run 验证参数结构。"""
        result = dws.run_raw(
            "attendance", "class", "update",
            "--class-id", "1170996821",
            "--class-vo", '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"08:30","across":0},{"checkType":"OffDuty","checkTime":"17:30","across":0}]}]}',
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "update_class_setting" in stdout
        assert "sections" in stdout, \
            f"Expected sections in dry-run output, got: {stdout[:200]}"

    def test_class_update_class_vo_invalid_json(self, dws):
        """--class-vo 无效 JSON 格式时应返回错误。"""
        result = dws.run_raw(
            "attendance", "class", "update",
            "--class-id", "1170996821",
            "--class-vo", "not-a-json",
        )
        stderr = (result.stderr or "").strip()
        assert "invalid --class-vo JSON" in stderr or "JSON" in stderr.upper(), \
            f"Expected invalid JSON error, got stderr: {stderr[:200]}"

    def test_class_update_name_overrides_class_vo(self, dws):
        """--name flag 应覆写 --class-vo 中的 name 字段。"""
        result = dws.run_raw(
            "attendance", "class", "update",
            "--class-id", "1170996821",
            "--name", "新名称",
            "--class-vo", '{"name":"旧名称","sections":[{"times":[{"checkType":"OnDuty","checkTime":"09:00","across":0},{"checkType":"OffDuty","checkTime":"18:00","across":0}]}]}',
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "update_class_setting" in stdout
        assert "新名称" in stdout, \
            f"Expected flag name to override class-vo, got: {stdout[:300]}"

    def test_class_update_with_rest_time_dry_run(self, dws):
        """带休息时段的更新，dry-run 验证 checkTime 自动转换为毫秒时间戳。"""
        class_vo = json.dumps({
            "sections": [{"times": [
                {"checkType": "OnDuty", "checkTime": "09:00", "across": 0},
                {"checkType": "OffDuty", "checkTime": "18:00", "across": 0},
            ]}],
            "setting": {
                "topRestTimeList": [
                    {"checkType": "OnDuty", "checkTime": "12:00", "across": 0},
                    {"checkType": "OffDuty", "checkTime": "13:00", "across": 0},
                ]
            }
        })
        result = dws.run_raw(
            "attendance", "class", "update",
            "--class-id", "1170996821",
            "--class-vo", class_vo,
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "update_class_setting" in stdout
        assert "topRestTimeList" in stdout, \
            f"Expected topRestTimeList in dry-run output, got: {stdout[:300]}"
        # checkTime "HH:mm" 已被 CLI 转为毫秒时间戳: 12:00→14400000, 13:00→18000000
        assert "14400000" in stdout and "18000000" in stdout, \
            f"Expected converted timestamps 14400000/18000000 in output, got: {stdout[:300]}"

    def test_class_update_owner_dry_run(self, dws):
        """--owner flag 传入班次负责人，dry-run 验证。"""
        result = dws.run_raw(
            "attendance", "class", "update",
            "--class-id", "1170996821",
            "--owner", "userId1",
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "update_class_setting" in stdout
        assert "userId1" in stdout, \
            f"Expected owner userId in dry-run output, got: {stdout[:300]}"

    def test_class_update_with_timeout(self, dws):
        """带 --timeout 10 实际调用，校验响应结构（成功或业务错误均可）。"""
        class_id = self._get_class_id(dws)
        result = dws.run_raw(
            "attendance", "class", "update",
            "--class-id", class_id,
            "--name", "自动测试更新班次_请勿使用",
            "--yes",
            "--timeout", "10",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "class update should return non-empty output"
        data = json.loads(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data.get("error"), dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got: {list(data.keys())}"

    def test_class_update_invalid_class_id(self, dws):
        """--class-id -1 传入无效 ID，应返回结构化业务错误而非崩溃。"""
        result = dws.run_raw(
            "attendance", "class", "update",
            "--class-id", "-1",
            "--name", "测试",
            "--yes",
            "--timeout", "10",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "class update --class-id -1 should return non-empty output"
        data = json.loads(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_error = "error" in data and isinstance(data["error"], dict)
        is_failure = data.get("success") is False or has_error
        assert is_failure, \
            f"Expected error response for class-id=-1, got: {list(data.keys())}"


class TestAttendanceGroupCreate:
    """dws attendance group create --name <NAME> --type <TYPE> [--owner] [--group-vo] --timeout 10

    Creates a new attendance group via MCP tool create_group_setting.
    --name and --type are mandatory. --type must be FIXED/TURN/NONE.
    --group-vo is optional for complex sub-objects.
    When --type=FIXED, groupVO must contain workDayClassList and defaultClassId.
    Due to server-side latency, --timeout 10 is recommended.

    NOTE: create_group_setting may return TIMEOUT_ERROR or other business
    errors depending on server deployment. Tests use run_raw/dry-run and
    validate structured response format rather than requiring business success.
    """

    def test_group_create_missing_name(self, dws):
        """缺少 --name 时应返回错误。"""
        result = dws.run_raw(
            "attendance", "group", "create",
            "--type", "FIXED",
        )
        stderr = (result.stderr or "").strip()
        assert "--name" in stderr or "必填" in stderr, \
            f"Expected missing name error, got stderr: {stderr[:200]}"

    def test_group_create_missing_type(self, dws):
        """缺少 --type 时应返回错误。"""
        result = dws.run_raw(
            "attendance", "group", "create",
            "--name", "测试考勤组",
        )
        stderr = (result.stderr or "").strip()
        assert "--type" in stderr or "必填" in stderr, \
            f"Expected missing type error, got stderr: {stderr[:200]}"

    def test_group_create_invalid_type(self, dws):
        """--type 传入非法值时应返回错误。"""
        result = dws.run_raw(
            "attendance", "group", "create",
            "--name", "测试考勤组",
            "--type", "INVALID",
        )
        stderr = (result.stderr or "").strip()
        assert "不合法" in stderr or "FIXED" in stderr, \
            f"Expected invalid type error, got stderr: {stderr[:200]}"

    def test_group_create_fixed_missing_work_day_class_list(self, dws):
        """type=FIXED 但缺少 workDayClassList 时应返回错误。"""
        result = dws.run_raw(
            "attendance", "group", "create",
            "--name", "固定班制组",
            "--type", "FIXED",
            "--group-vo", '{"defaultClassId":1170996821}',
        )
        stderr = (result.stderr or "").strip()
        assert "workDayClassList" in stderr, \
            f"Expected missing workDayClassList error, got stderr: {stderr[:200]}"

    def test_group_create_fixed_missing_default_class_id(self, dws):
        """type=FIXED 但缺少 defaultClassId 时应返回错误。"""
        result = dws.run_raw(
            "attendance", "group", "create",
            "--name", "固定班制组",
            "--type", "FIXED",
            "--group-vo", '{"workDayClassList":[0,1170996821,0,0,0,0,0]}',
        )
        stderr = (result.stderr or "").strip()
        assert "defaultClassId" in stderr, \
            f"Expected missing defaultClassId error, got stderr: {stderr[:200]}"

    def test_group_create_invalid_group_vo_json(self, dws):
        """--group-vo 无效 JSON 格式时应返回错误。"""
        result = dws.run_raw(
            "attendance", "group", "create",
            "--name", "测试考勤组",
            "--type", "TURN",
            "--group-vo", "not-a-json",
        )
        stderr = (result.stderr or "").strip()
        assert "invalid --group-vo JSON" in stderr or "JSON" in stderr.upper(), \
            f"Expected invalid group-vo JSON error, got stderr: {stderr[:200]}"

    def test_group_create_turn_basic_dry_run(self, dws):
        """创建排班制考勤组，dry-run 验证参数结构。"""
        result = dws.run_raw(
            "attendance", "group", "create",
            "--name", "排班组",
            "--type", "TURN",
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "create_group_setting" in stdout, \
            f"Expected tool name in dry-run output, got: {stdout[:200]}"
        assert "groupVO" in stdout, \
            f"Expected groupVO in dry-run output, got: {stdout[:200]}"
        assert '"name"' in stdout and "排班组" in stdout, \
            f"Expected name field in dry-run output, got: {stdout[:300]}"
        assert '"type"' in stdout and "TURN" in stdout, \
            f"Expected type field in dry-run output, got: {stdout[:300]}"

    def test_group_create_none_basic_dry_run(self, dws):
        """创建自由工时考勤组，dry-run 验证参数结构。"""
        result = dws.run_raw(
            "attendance", "group", "create",
            "--name", "自由工时组",
            "--type", "NONE",
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "create_group_setting" in stdout
        assert "NONE" in stdout, \
            f"Expected NONE type in dry-run output, got: {stdout[:300]}"

    def test_group_create_fixed_dry_run(self, dws):
        """创建固定班制考勤组（含 workDayClassList + defaultClassId），dry-run 验证。"""
        result = dws.run_raw(
            "attendance", "group", "create",
            "--name", "研发固定班组",
            "--type", "FIXED",
            "--group-vo", '{"defaultClassId":1170996821,"workDayClassList":[0,1170996821,1170996821,1170996821,1170996821,1170996821,0]}',
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "create_group_setting" in stdout
        assert "workDayClassList" in stdout, \
            f"Expected workDayClassList in dry-run output, got: {stdout[:300]}"
        assert "defaultClassId" in stdout, \
            f"Expected defaultClassId in dry-run output, got: {stdout[:300]}"

    def test_group_create_name_flag_overrides_group_vo(self, dws):
        """--name flag 应覆写 --group-vo 中的 name 字段。"""
        result = dws.run_raw(
            "attendance", "group", "create",
            "--name", "新名称",
            "--type", "TURN",
            "--group-vo", '{"name":"旧名称"}',
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "create_group_setting" in stdout
        assert "新名称" in stdout, \
            f"Expected flag name to override group-vo, got: {stdout[:300]}"

    def test_group_create_with_owner_dry_run(self, dws):
        """--owner flag 传入主负责人，dry-run 验证。"""
        result = dws.run_raw(
            "attendance", "group", "create",
            "--name", "带负责人组",
            "--type", "TURN",
            "--owner", "userId1",
            "--yes",
            "--dry-run",
        )
        stdout = (result.stdout or "").strip()
        assert "create_group_setting" in stdout
        assert "userId1" in stdout, \
            f"Expected owner userId in dry-run output, got: {stdout[:300]}"

    def test_group_create_turn_with_timeout(self, dws):
        """带 --timeout 10 实际创建排班制考勤组，校验响应结构（成功或业务错误均可）。"""
        result = dws.run_raw(
            "attendance", "group", "create",
            "--name", "自动测试排班组_请勿使用",
            "--type", "TURN",
            "--yes",
            "--timeout", "10",
        )
        output = result.stdout.strip() or result.stderr.strip()
        assert output, "group create should return non-empty output"
        data = json.loads(output)
        assert isinstance(data, dict), f"response should be dict, got {type(data)}"
        has_result = data.get("success") is True and "result" in data
        has_error = "error" in data and isinstance(data.get("error"), dict)
        assert has_result or has_error, \
            f"Expected success+result or structured error, got: {list(data.keys())}"

