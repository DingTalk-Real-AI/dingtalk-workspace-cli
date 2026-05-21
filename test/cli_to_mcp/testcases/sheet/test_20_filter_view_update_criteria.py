"""
test_20_filter_view_update_criteria.py — 设置筛选视图列条件测试

依赖 conftest.py 自建的测试表格。

Commands tested:
  1. dws sheet filter-view update-criteria ... --column N --filter-criteria JSON (按值筛选)
  2. dws sheet filter-view update-criteria ... (按条件筛选)
  3. dws sheet filter-view update-criteria ... (按颜色筛选)
  4. dws sheet filter-view update-criteria ... (多条件 + conditionOperator)
"""

import json

from test_utils import unique_name


def _create_filter_view(dws, node_id, sheet_id, cleanup=None, name=None, range_addr="A1:D10"):
    """辅助：创建筛选视图并返回 id。

    Args:
        cleanup: filter_view_cleanup fixture 回调，传入后自动注册 teardown 删除。
    """
    fv_name = name or unique_name("FVSetCrit")
    data = dws.run(
        "sheet", "filter-view", "create",
        "--node", node_id,
        "--sheet-id", sheet_id,
        "--name", fv_name,
        "--range", range_addr,
    )
    assert data.get("success") is True, f"create 应成功: {data}"
    fv_id = data.get("id")
    assert fv_id, f"create 未返回 id: {data}"
    if cleanup:
        cleanup(fv_id)
    return fv_id


class TestFilterViewSetCriteriaValues:
    """dws sheet filter-view update-criteria — 按值筛选"""

    def test_set_values_criteria(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """设置按值筛选条件，只显示"销售部"。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        criteria = json.dumps(
            {"filterType": "values", "visibleValues": ["销售部"]},
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter-view", "update-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "0",
            "--filter-criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_set_values_criteria_multi_values(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """设置按值筛选条件，允许多个值。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        criteria = json.dumps(
            {"filterType": "values", "visibleValues": ["销售部", "市场部", "研发部"]},
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter-view", "update-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "1",
            "--filter-criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"


class TestFilterViewSetCriteriaCondition:
    """dws sheet filter-view update-criteria — 按条件筛选"""

    def test_set_condition_greater(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """设置按条件筛选：大于 50000。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        criteria = json.dumps(
            {
                "filterType": "condition",
                "conditions": [{"operator": "greater", "value": "50000"}],
            },
        )
        data = dws.run(
            "sheet", "filter-view", "update-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "2",
            "--filter-criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_set_condition_contains(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """设置按条件筛选：包含"完"。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        criteria = json.dumps(
            {
                "filterType": "condition",
                "conditions": [{"operator": "contains", "value": "完"}],
            },
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter-view", "update-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "3",
            "--filter-criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_set_condition_two_conditions_and(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """设置双条件（and）：大于等于 30000 且 小于 70000。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        criteria = json.dumps(
            {
                "filterType": "condition",
                "conditions": [
                    {"operator": "greater-equal", "value": "30000"},
                    {"operator": "less", "value": "70000"},
                ],
                "conditionOperator": "and",
            },
        )
        data = dws.run(
            "sheet", "filter-view", "update-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "2",
            "--filter-criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_set_condition_two_conditions_or(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """设置双条件（or）：等于"完成" 或 等于"pending"。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        criteria = json.dumps(
            {
                "filterType": "condition",
                "conditions": [
                    {"operator": "equal", "value": "完成"},
                    {"operator": "equal", "value": "pending"},
                ],
                "conditionOperator": "or",
            },
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter-view", "update-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "3",
            "--filter-criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"


class TestFilterViewSetCriteriaColor:
    """dws sheet filter-view update-criteria — 按颜色筛选"""

    def test_set_background_color(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """设置按背景色筛选。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        criteria = json.dumps(
            {"filterType": "color", "backgroundColor": "#FF0000"},
        )
        data = dws.run(
            "sheet", "filter-view", "update-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "0",
            "--filter-criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_set_font_color(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """设置按字体色筛选。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        criteria = json.dumps(
            {"filterType": "color", "fontColor": "#0000FF"},
        )
        data = dws.run(
            "sheet", "filter-view", "update-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "0",
            "--filter-criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"


class TestFilterViewSetCriteriaDifferentColumns:
    """dws sheet filter-view update-criteria — 不同列"""

    def test_set_criteria_column_zero(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """设置第 0 列的筛选条件。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        criteria = json.dumps(
            {"filterType": "values", "visibleValues": ["张三"]},
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter-view", "update-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "0",
            "--filter-criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_set_criteria_column_three(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """设置第 3 列的筛选条件。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        criteria = json.dumps(
            {"filterType": "values", "visibleValues": ["完成"]},
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter-view", "update-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "3",
            "--filter-criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"


class TestFilterViewSetCriteriaError:
    """dws sheet filter-view update-criteria — 错误路径"""

    def test_set_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "update-criteria",
            "--sheet-id", "Sheet1",
            "--filter-view-id", "FV_ID",
            "--column", "0",
            "--filter-criteria", '{"filterType":"values","visibleValues":["a"]}',
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_set_missing_sheet_id(self, dws):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "update-criteria",
            "--node", "SOME_NODE",
            "--filter-view-id", "FV_ID",
            "--column", "0",
            "--filter-criteria", '{"filterType":"values","visibleValues":["a"]}',
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"

    def test_set_missing_filter_view_id(self, dws):
        """缺少必填 --filter-view-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "update-criteria",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--column", "0",
            "--filter-criteria", '{"filterType":"values","visibleValues":["a"]}',
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --filter-view-id 应报错: {result.stdout[:200]}"

    def test_set_missing_filter_criteria(self, dws):
        """缺少必填 --filter-criteria 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "update-criteria",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--filter-view-id", "FV_ID",
            "--column", "0",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --filter-criteria 应报错: {result.stdout[:200]}"

    def test_set_invalid_criteria_json(self, dws, sheet_node_id, sheet_id):
        """--filter-criteria 传非法 JSON 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "update-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", "SOME_FV",
            "--column", "0",
            "--filter-criteria", "NOT_VALID_JSON",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"非法 JSON 应报错: {result.stdout[:200]}"

    def test_set_negative_column(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """--column 为负数应报错。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        result = dws.run_raw(
            "sheet", "filter-view", "update-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "-1",
            "--filter-criteria", '{"filterType":"values","visibleValues":["a"]}',
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"column=-1 应报错: {result.stdout[:200]}"

    def test_set_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "update-criteria",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--filter-view-id", "FV_ID",
            "--column", "0",
            "--filter-criteria", '{"filterType":"values","visibleValues":["a"]}',
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    def test_set_invalid_filter_view_id(self, dws, sheet_node_id, sheet_id):
        """无效 filterViewId 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "update-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", "NONEXISTENT_FV_99999",
            "--column", "0",
            "--filter-criteria", '{"filterType":"values","visibleValues":["a"]}',
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 filterViewId 应报错: {result.stdout[:200]}"
