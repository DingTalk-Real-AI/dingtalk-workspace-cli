"""
test_24_filter_view_get_criteria.py — 获取单列筛选条件详情测试

依赖 conftest.py 自建的测试表格。

Commands tested:
  1. dws sheet filter-view get-criteria --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID --column N
"""

import json

from test_utils import unique_name


def _create_filter_view(dws, node_id, sheet_id, cleanup=None):
    """辅助：创建筛选视图（无条件），返回 id。"""
    fv_name = unique_name("FVGetCrit")
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


class TestFilterViewGetCriteriaBasic:
    """dws sheet filter-view get-criteria — 基本功能"""

    def test_get_criteria_values_filter(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """获取按值筛选条件，验证返回包含 filterType。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        _set_criteria(dws, sheet_node_id, sheet_id, fv_id, 0, {
            "filterType": "values", "visibleValues": ["销售部"],
        })
        data = dws.run(
            "sheet", "filter-view", "get-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "0",
        )
        assert isinstance(data, dict), f"get-criteria 应返回 dict: {data}"
        assert "filterType" in data, f"响应缺少 filterType: {data}"

    def test_get_criteria_condition_filter(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """获取按条件筛选的详情。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        _set_criteria(dws, sheet_node_id, sheet_id, fv_id, 2, {
            "filterType": "condition",
            "conditions": [{"operator": "greater", "value": "50000"}],
        })
        data = dws.run(
            "sheet", "filter-view", "get-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "2",
        )
        assert isinstance(data, dict), f"get-criteria 应返回 dict: {data}"
        assert "filterType" in data, f"响应缺少 filterType: {data}"

    def test_get_criteria_nonexistent_column(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """查询未设置条件的列，应报错或返回提示。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        result = dws.run_raw(
            "sheet", "filter-view", "get-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "3",
        )
        assert (
            result.returncode != 0
            or "未设置" in result.stdout
            or "no criteria" in result.stdout.lower()
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"未设置条件的列应有提示: {result.stdout[:300]}"


class TestFilterViewGetCriteriaChain:
    """dws sheet filter-view get-criteria — 完整链路"""

    def test_set_then_get(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """设置条件 → 获取条件，验证一致性。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        expected_values = ["销售部", "市场部"]
        _set_criteria(dws, sheet_node_id, sheet_id, fv_id, 1, {
            "filterType": "values", "visibleValues": expected_values,
        })
        data = dws.run(
            "sheet", "filter-view", "get-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "1",
        )
        assert isinstance(data, dict), f"get-criteria 应返回 dict: {data}"
        if "visibleValues" in data:
            actual_values = data["visibleValues"]
            for expected in expected_values:
                assert expected in actual_values, (
                    f"visibleValues 应包含 {expected}: {actual_values}"
                )

    def test_set_clear_then_get(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """设置条件 → 清除条件 → 获取条件，应报错/返回空。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        _set_criteria(dws, sheet_node_id, sheet_id, fv_id, 0, {
            "filterType": "values", "visibleValues": ["销售部"],
        })
        dws.run(
            "sheet", "filter-view", "delete-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "0",
        )
        result = dws.run_raw(
            "sheet", "filter-view", "get-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "0",
        )
        assert (
            result.returncode != 0
            or "未设置" in result.stdout
            or "no criteria" in result.stdout.lower()
            or "error" in result.stdout.lower()
            or "{}" in result.stdout
        ), f"清除后获取应有提示: {result.stdout[:300]}"


class TestFilterViewGetCriteriaError:
    """dws sheet filter-view get-criteria — 错误路径"""

    def test_get_criteria_invalid_filter_view_id(self, dws, sheet_node_id, sheet_id):
        """无效 filterViewId 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "get-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", "NONEXISTENT_FV_99999",
            "--column", "0",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "未找到" in result.stdout
            or "error" in result.stderr.lower()
        ), f"无效 filterViewId 应报错: {result.stdout[:300]}"

    def test_get_criteria_missing_column(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """缺少必填 --column 参数应报错。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        result = dws.run_raw(
            "sheet", "filter-view", "get-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
            or "column" in (result.stdout + result.stderr).lower()
        ), f"缺少 --column 应报错: {result.stdout[:200]}"

    def test_get_criteria_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "get-criteria",
            "--sheet-id", "Sheet1",
            "--filter-view-id", "FV_ID",
            "--column", "0",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_get_criteria_missing_sheet_id(self, dws):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "get-criteria",
            "--node", "SOME_NODE",
            "--filter-view-id", "FV_ID",
            "--column", "0",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"

    def test_get_criteria_missing_filter_view_id(self, dws):
        """缺少必填 --filter-view-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "get-criteria",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--column", "0",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --filter-view-id 应报错: {result.stdout[:200]}"

    def test_get_criteria_negative_column(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """--column 为负数应报错。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        result = dws.run_raw(
            "sheet", "filter-view", "get-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "-1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"负数 column 应报错: {result.stdout[:200]}"
