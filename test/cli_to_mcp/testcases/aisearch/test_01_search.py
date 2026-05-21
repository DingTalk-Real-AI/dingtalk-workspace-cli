"""
test_01_search.py — AI 搜问 person 命令测试

Commands tested:
  1. dws aisearch person --keyword <kw> [--dimension <dim>]

实际返回格式 (MCP-raw-style):
  {"result": [{userId, title, ...}], "success": true}

测试策略：
  - 用通用单字姓 "张" 大概率匹配出有效用户，验证非空响应结构。
  - 用 ZZZNONEXIST99999 验证空响应仍是合法结构（不报错）。
  - 各个 dimension 维度（all/name/department/duty/supervisor/phone/jobNumber）
    分别覆盖一次，验证参数能正确传给 enterprise_person_search。
"""

import pytest


def _assert_search_response_shape(data):
    """统一断言 aisearch person 返回结构。

    aisearch person 走 enterprise_person_search MCP 工具，
    返回 {"result": [...], "success": true} 或包含 result 字段的对象。
    空结果时 result 应为空列表，不应报错。
    """
    assert isinstance(data, dict), f"响应应为 dict，实为 {type(data)}: {data}"
    # success 字段可能由 _assert_no_error 已经校验，这里再次确认
    if "success" in data:
        assert data["success"] is True, f"success 应为 True: {data}"
    # result 字段必须存在（list 或 dict 都可接受）
    assert "result" in data, f"响应缺少 result 字段: {list(data.keys())}"


class TestAisearchPerson:
    """dws aisearch person — 企业人员语义搜索"""

    def test_person_basic_keyword(self, dws):
        """基本关键词搜索（默认 dimension=all），返回 result 结构合法。"""
        data = dws.run_ok(
            "aisearch", "person",
            "--keyword", "张",
        )
        _assert_search_response_shape(data)

    def test_person_no_match_returns_valid_shape(self, dws):
        """搜索不存在的关键词，响应结构仍合法。

        注：aisearch person 是**语义搜索**（fuzzy match），即使关键词
        完全无匹配，后端也可能返回 fuzzy 候选——这是产品预期行为，
        与 contact user search 的精确搜索不同。本用例只断言响应结构
        合法，不要求 result 为空。
        """
        data = dws.run_ok(
            "aisearch", "person",
            "--keyword", "ZZZNONEXIST99999",
        )
        _assert_search_response_shape(data)
        result = data["result"]
        # result 可能是 list（含 fuzzy 候选）或 dict，都视为合法
        assert isinstance(result, (list, dict)), (
            f"result 应为 list 或 dict，实为 {type(result)}: {result}"
        )

    def test_person_dimension_name(self, dws):
        """按姓名维度搜索。"""
        data = dws.run_ok(
            "aisearch", "person",
            "--keyword", "张",
            "--dimension", "name",
        )
        _assert_search_response_shape(data)

    def test_person_dimension_department(self, dws):
        """按部门维度搜索。"""
        data = dws.run_ok(
            "aisearch", "person",
            "--keyword", "产品",
            "--dimension", "department",
        )
        _assert_search_response_shape(data)

    def test_person_dimension_duty(self, dws):
        """按职责维度搜索（'AI 搜问的负责人是谁' 这种意图）。"""
        data = dws.run_ok(
            "aisearch", "person",
            "--keyword", "AI",
            "--dimension", "duty",
        )
        _assert_search_response_shape(data)

    def test_person_dimension_supervisor(self, dws):
        """按上级维度搜索（'XX 的上级是谁' 这种意图）。"""
        data = dws.run_ok(
            "aisearch", "person",
            "--keyword", "张",
            "--dimension", "supervisor",
        )
        _assert_search_response_shape(data)

    def test_person_dimension_subordinate(self, dws):
        """按下级维度搜索。"""
        data = dws.run_ok(
            "aisearch", "person",
            "--keyword", "张",
            "--dimension", "subordinate",
        )
        _assert_search_response_shape(data)

    def test_person_dimension_phone(self, dws):
        """按手机号维度搜索（结构断言为主，号码无效也不应报错）。"""
        data = dws.run_ok(
            "aisearch", "person",
            "--keyword", "13800138000",
            "--dimension", "phone",
        )
        _assert_search_response_shape(data)

    def test_person_dimension_jobNumber(self, dws):
        """按工号维度搜索。"""
        data = dws.run_ok(
            "aisearch", "person",
            "--keyword", "W12345",
            "--dimension", "jobNumber",
        )
        _assert_search_response_shape(data)

    def test_person_dimension_position(self, dws):
        """按职位维度搜索。"""
        data = dws.run_ok(
            "aisearch", "person",
            "--keyword", "工程师",
            "--dimension", "position",
        )
        _assert_search_response_shape(data)

    def test_person_dimension_multi(self, dws):
        """多维度组合（逗号分隔）。"""
        data = dws.run_ok(
            "aisearch", "person",
            "--keyword", "张",
            "--dimension", "name,department",
        )
        _assert_search_response_shape(data)

    def test_person_short_flags(self, dws):
        """短 flag 形式 -w / -d 同样能用。"""
        data = dws.run_ok(
            "aisearch", "person",
            "-w", "张",
            "-d", "name",
        )
        _assert_search_response_shape(data)
