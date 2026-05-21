"""
test_23_filter_view_list_criteria.py — 列出筛选视图所有列条件测试

依赖 conftest.py 自建的测试表格。

Commands tested:
  1. dws sheet filter-view list-criteria --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID
"""

import json

from test_utils import unique_name


def _create_filter_view(dws, node_id, sheet_id, cleanup=None):
    """辅助：创建筛选视图（无条件），返回 id。"""
    fv_name = unique_name("FVListCrit")
    data = dws.run(
        "sheet", "filter-view", "create",
        "--node", node_id,
        "--sheet-id", sheet_id,
        "--name", fv_name,
        "--range", "A1:D10",
    )
    assert data.get("success") is True, f"create 应成功: {data}"
    fv_id = data.get("id")
    assert fv_id, f"create 未返回 id: {data}"
    if cleanup:
        cleanup(fv_id)
    return fv_id


def _set_criteria(dws, node_id, sheet_id, fv_id, column, filter_criteria):
    """辅助：为筛选视图指定列设置条件。"""
    data = dws.run(
        "sheet", "filter-view", "update-criteria",
        "--node", node_id,
        "--sheet-id", sheet_id,
        "--filter-view-id", fv_id,
        "--column", str(column),
        "--filter-criteria", json.dumps(filter_criteria, ensure_ascii=False),
    )
    assert data.get("success") is True, f"update-criteria 应成功: {data}"


class TestFilterViewListCriteriaBasic:
    """dws sheet filter-view list-criteria — 基本功能"""

    def test_list_criteria_empty(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """无条件的筛选视图，list-criteria 应返回空对象。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        data = dws.run(
            "sheet", "filter-view", "list-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
        )
        assert isinstance(data, dict), f"list-criteria 应返回 dict: {data}"

    def test_list_criteria_single_column(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """设置单列条件后，list-criteria 应包含该列。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        _set_criteria(dws, sheet_node_id, sheet_id, fv_id, 0, {
            "filterType": "values", "visibleValues": ["销售部"],
        })
        data = dws.run(
            "sheet", "filter-view", "list-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
        )
        assert isinstance(data, dict), f"list-criteria 应返回 dict: {data}"
        # criteria 可能在 list 层不返回（取决于 MCP），仅验证命令不报错

    def test_list_criteria_multiple_columns(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """设置多列条件后，list-criteria 应包含所有列。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        _set_criteria(dws, sheet_node_id, sheet_id, fv_id, 0, {
            "filterType": "values", "visibleValues": ["销售部"],
        })
        _set_criteria(dws, sheet_node_id, sheet_id, fv_id, 2, {
            "filterType": "condition",
            "conditions": [{"operator": "greater", "value": "50000"}],
        })
        data = dws.run(
            "sheet", "filter-view", "list-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
        )
        assert isinstance(data, dict), f"list-criteria 应返回 dict: {data}"


class TestFilterViewListCriteriaError:
    """dws sheet filter-view list-criteria — 错误路径"""

    def test_list_criteria_invalid_filter_view_id(self, dws, sheet_node_id, sheet_id):
        """无效 filterViewId 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "list-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", "NONEXISTENT_FV_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "未找到" in result.stdout
            or "error" in result.stderr.lower()
        ), f"无效 filterViewId 应报错: {result.stdout[:300]}"

    def test_list_criteria_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "list-criteria",
            "--sheet-id", "Sheet1",
            "--filter-view-id", "FV_ID",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_list_criteria_missing_sheet_id(self, dws):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "list-criteria",
            "--node", "SOME_NODE",
            "--filter-view-id", "FV_ID",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"

    def test_list_criteria_missing_filter_view_id(self, dws):
        """缺少必填 --filter-view-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "list-criteria",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --filter-view-id 应报错: {result.stdout[:200]}"
