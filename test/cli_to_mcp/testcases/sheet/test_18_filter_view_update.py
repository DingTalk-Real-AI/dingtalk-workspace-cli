"""
test_18_filter_view_update.py — 更新筛选视图属性测试

依赖 conftest.py 自建的测试表格。

Commands tested:
  1. dws sheet filter-view update ... --name NEW_NAME
  2. dws sheet filter-view update ... --range NEW_RANGE
  3. dws sheet filter-view update ... --criteria CRITERIA_JSON
  4. dws sheet filter-view update ... --name --range --criteria 组合
"""

import json

from test_utils import unique_name


def _create_filter_view(dws, node_id, sheet_id, cleanup=None, name=None, range_addr="A1:D10"):
    """辅助：创建筛选视图并返回 id。

    Args:
        cleanup: filter_view_cleanup fixture 回调，传入后自动注册 teardown 删除。
    """
    fv_name = name or unique_name("FVUpd")
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
    return fv_id, fv_name


class TestFilterViewUpdateName:
    """dws sheet filter-view update — 更新名称"""

    def test_update_name(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """更新筛选视图名称。"""
        fv_id, _ = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        new_name = unique_name("Renamed")
        data = dws.run(
            "sheet", "filter-view", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--name", new_name,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("name") == new_name, f"name 应为 {new_name}: {data}"


class TestFilterViewUpdateRange:
    """dws sheet filter-view update — 更新范围"""

    def test_update_range(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """更新筛选视图范围。"""
        fv_id, _ = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        new_range = "A1:F20"
        data = dws.run(
            "sheet", "filter-view", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--range", new_range,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("range") == new_range, f"range 应为 {new_range}: {data}"


class TestFilterViewUpdateCriteria:
    """dws sheet filter-view update — 更新筛选条件"""

    def test_update_criteria(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """通过 update 设置筛选条件。"""
        fv_id, _ = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        criteria = json.dumps(
            [{"column": 0, "filterType": "values", "visibleValues": ["市场部"]}],
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter-view", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_update_criteria_multi_column(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """通过 update 同时设置多列的筛选条件。"""
        fv_id, _ = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        criteria = json.dumps(
            [
                {"column": 0, "filterType": "values", "visibleValues": ["销售部"]},
                {"column": 2, "filterType": "condition", "conditions": [{"operator": "less", "value": "60000"}]},
            ],
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter-view", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"


class TestFilterViewUpdateCombined:
    """dws sheet filter-view update — 组合更新"""

    def test_update_name_and_range(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """同时更新名称和范围。"""
        fv_id, _ = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        new_name = unique_name("Combined")
        new_range = "B2:E15"
        data = dws.run(
            "sheet", "filter-view", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--name", new_name,
            "--range", new_range,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("name") == new_name, f"name 应为 {new_name}: {data}"
        assert data.get("range") == new_range, f"range 应为 {new_range}: {data}"

    def test_update_all_fields(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """同时更新名称、范围和筛选条件。"""
        fv_id, _ = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        new_name = unique_name("AllFields")
        new_range = "A1:G30"
        criteria = json.dumps(
            [{"column": 1, "filterType": "values", "visibleValues": ["研发部"]}],
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter-view", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--name", new_name,
            "--range", new_range,
            "--criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("name") == new_name, f"name 应为 {new_name}: {data}"


class TestFilterViewUpdateError:
    """dws sheet filter-view update — 错误路径"""

    def test_update_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "update",
            "--sheet-id", "Sheet1",
            "--filter-view-id", "FV_ID",
            "--name", "test",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_update_missing_sheet_id(self, dws):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "update",
            "--node", "SOME_NODE",
            "--filter-view-id", "FV_ID",
            "--name", "test",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"

    def test_update_missing_filter_view_id(self, dws):
        """缺少必填 --filter-view-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "update",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--name", "test",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --filter-view-id 应报错: {result.stdout[:200]}"

    def test_update_no_update_field(self, dws, sheet_node_id, sheet_id):
        """不传任何更新字段应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", "SOME_FV_ID",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"不传更新字段应报错: {result.stdout[:200]}"

    def test_update_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "update",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--filter-view-id", "FV_ID",
            "--name", "test",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    def test_update_invalid_filter_view_id(self, dws, sheet_node_id, sheet_id):
        """无效 filterViewId 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", "NONEXISTENT_FV_99999",
            "--name", "test",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 filterViewId 应报错: {result.stdout[:200]}"

    def test_update_invalid_criteria_json(self, dws, sheet_node_id, sheet_id):
        """--criteria 传非法 JSON 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", "SOME_FV_ID",
            "--criteria", "NOT_VALID_JSON",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"非法 JSON 应报错: {result.stdout[:200]}"
