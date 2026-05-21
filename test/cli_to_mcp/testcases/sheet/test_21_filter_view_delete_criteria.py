"""
test_21_filter_view_delete_criteria.py — 删除筛选视图列条件测试

依赖 conftest.py 自建的测试表格。

Commands tested:
  1. dws sheet filter-view delete-criteria --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID --column N
"""

import json

from test_utils import unique_name


def _create_filter_view_with_criteria(dws, node_id, sheet_id, column=0, cleanup=None):
    """辅助：创建筛选视图并设置筛选条件，返回 filterViewId。

    Args:
        cleanup: filter_view_cleanup fixture 回调，传入后自动注册 teardown 删除。
    """
    fv_name = unique_name("FVClear")
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

    # 设置筛选条件
    criteria = json.dumps(
        {"filterType": "values", "visibleValues": ["销售部"]},
        ensure_ascii=False,
    )
    set_data = dws.run(
        "sheet", "filter-view", "update-criteria",
        "--node", node_id,
        "--sheet-id", sheet_id,
        "--filter-view-id", fv_id,
        "--column", str(column),
        "--filter-criteria", criteria,
    )
    assert set_data.get("success") is True, f"update-criteria 应成功: {set_data}"
    return fv_id


def _create_filter_view(dws, node_id, sheet_id, cleanup=None, name=None):
    """辅助：创建筛选视图（无条件），返回 id。

    Args:
        cleanup: filter_view_cleanup fixture 回调，传入后自动注册 teardown 删除。
    """
    fv_name = name or unique_name("FVNoCrit")
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


class TestFilterViewClearCriteriaBasic:
    """dws sheet filter-view delete-criteria — 基本清除"""

    def test_clear_after_set(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """设置条件后清除，验证 success。"""
        fv_id = _create_filter_view_with_criteria(dws, sheet_node_id, sheet_id, column=0, cleanup=filter_view_cleanup)
        data = dws.run(
            "sheet", "filter-view", "delete-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "0",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_clear_no_existing_criteria(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """对没有筛选条件的列执行清除，不应报错。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        data = dws.run(
            "sheet", "filter-view", "delete-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "0",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_clear_different_column(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """清除第 2 列的筛选条件。"""
        fv_id = _create_filter_view_with_criteria(dws, sheet_node_id, sheet_id, column=2, cleanup=filter_view_cleanup)
        data = dws.run(
            "sheet", "filter-view", "delete-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "2",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_clear_column_zero(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """清除第 0 列（边界值）的筛选条件。"""
        fv_id = _create_filter_view_with_criteria(dws, sheet_node_id, sheet_id, column=0, cleanup=filter_view_cleanup)
        data = dws.run(
            "sheet", "filter-view", "delete-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "0",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"


class TestFilterViewClearCriteriaChain:
    """dws sheet filter-view delete-criteria — 完整链路"""

    def test_set_then_clear_then_set(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """设置 → 清除 → 重新设置，验证全程成功。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)

        # 设置条件
        criteria = json.dumps(
            {"filterType": "values", "visibleValues": ["市场部"]},
            ensure_ascii=False,
        )
        dws.run(
            "sheet", "filter-view", "update-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "1",
            "--filter-criteria", criteria,
        )

        # 清除条件
        clear_data = dws.run(
            "sheet", "filter-view", "delete-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "1",
        )
        assert clear_data.get("success") is True, f"clear 应成功: {clear_data}"

        # 重新设置不同条件
        new_criteria = json.dumps(
            {"filterType": "condition", "conditions": [{"operator": "greater", "value": "50000"}]},
        )
        set_data = dws.run(
            "sheet", "filter-view", "update-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "1",
            "--filter-criteria", new_criteria,
        )
        assert set_data.get("success") is True, f"重新设置应成功: {set_data}"

    def test_clear_multiple_columns(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """对多列分别设置条件后逐列清除。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)

        # 设置两列条件
        for col in [0, 2]:
            criteria = json.dumps(
                {"filterType": "values", "visibleValues": ["测试值"]},
                ensure_ascii=False,
            )
            dws.run(
                "sheet", "filter-view", "update-criteria",
                "--node", sheet_node_id,
                "--sheet-id", sheet_id,
                "--filter-view-id", fv_id,
                "--column", str(col),
                "--filter-criteria", criteria,
            )

        # 逐列清除
        for col in [0, 2]:
            data = dws.run(
                "sheet", "filter-view", "delete-criteria",
                "--node", sheet_node_id,
                "--sheet-id", sheet_id,
                "--filter-view-id", fv_id,
                "--column", str(col),
            )
            assert data.get("success") is True, f"清除第 {col} 列应成功: {data}"


class TestFilterViewClearCriteriaError:
    """dws sheet filter-view delete-criteria — 错误路径"""

    def test_clear_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "delete-criteria",
            "--sheet-id", "Sheet1",
            "--filter-view-id", "FV_ID",
            "--column", "0",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_clear_missing_sheet_id(self, dws):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "delete-criteria",
            "--node", "SOME_NODE",
            "--filter-view-id", "FV_ID",
            "--column", "0",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"

    def test_clear_missing_filter_view_id(self, dws):
        """缺少必填 --filter-view-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "delete-criteria",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--column", "0",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --filter-view-id 应报错: {result.stdout[:200]}"

    def test_clear_negative_column(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """--column 为负数应报错。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        result = dws.run_raw(
            "sheet", "filter-view", "delete-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--column", "-1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"column=-1 应报错: {result.stdout[:200]}"

    def test_clear_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "delete-criteria",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--filter-view-id", "FV_ID",
            "--column", "0",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    def test_clear_invalid_filter_view_id(self, dws, sheet_node_id, sheet_id):
        """无效 filterViewId 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "delete-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", "NONEXISTENT_FV_99999",
            "--column", "0",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 filterViewId 应报错: {result.stdout[:200]}"