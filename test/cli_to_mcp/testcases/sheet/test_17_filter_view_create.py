"""
test_17_filter_view_create.py — 创建筛选视图测试

依赖 conftest.py 自建的测试表格。

Commands tested:
  1. dws sheet filter-view create --node NODE_ID --sheet-id SHEET_ID --name NAME --range RANGE
  2. dws sheet filter-view create ... --criteria CRITERIA_JSON
"""

import json

from test_utils import unique_name


class TestFilterViewCreateBasic:
    """dws sheet filter-view create — 基本创建"""

    def test_create_basic(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """创建不带条件的筛选视图，验证返回 id、name、range。"""
        fv_name = unique_name("BasicFV")
        data = dws.run(
            "sheet", "filter-view", "create",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--name", fv_name,
            "--range", "A1:D10",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("id"), f"响应缺少 id: {data}"
        filter_view_cleanup(data["id"])
        assert data.get("name") == fv_name, f"name 应为 {fv_name}: {data}"
        assert data.get("range"), f"响应缺少 range: {data}"

    def test_create_single_column_range(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """创建单列范围的筛选视图。"""
        fv_name = unique_name("SingleCol")
        data = dws.run(
            "sheet", "filter-view", "create",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--name", fv_name,
            "--range", "A1:A100",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("id"), f"响应缺少 id: {data}"
        filter_view_cleanup(data["id"])

    def test_create_large_range(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """创建大范围的筛选视图（A1:Z100，覆盖 26 列 × 100 行）。"""
        fv_name = unique_name("LargeRange")
        data = dws.run(
            "sheet", "filter-view", "create",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--name", fv_name,
            "--range", "A1:Z100",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("id"), f"响应缺少 id: {data}"
        filter_view_cleanup(data["id"])

    def test_create_in_new_sheet(self, dws, sheet_node_id, filter_view_cleanup):
        """在新建工作表中创建筛选视图。"""
        sheet_name = unique_name("FVCreate")
        dws.run("sheet", "new", "--node", sheet_node_id, "--name", sheet_name)

        list_data = dws.run("sheet", "list", "--node", sheet_node_id)
        sheets = list_data.get("sheets") or list_data.get("data", {}).get("sheets") or []
        new_sheet_id = None
        for s in sheets:
            name = s.get("name") or s.get("title") or ""
            if sheet_name in name:
                new_sheet_id = s.get("sheetId") or s.get("id")
                break
        assert new_sheet_id, f"新建工作表不在 list 中: {sheets}"

        fv_name = unique_name("NewSheetFV")
        data = dws.run(
            "sheet", "filter-view", "create",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--name", fv_name,
            "--range", "A1:E20",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("id"), f"响应缺少 id: {data}"
        filter_view_cleanup(data["id"])


class TestFilterViewCreateWithCriteria:
    """dws sheet filter-view create — 带筛选条件"""

    def test_create_with_values_criteria(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """创建带按值筛选条件的视图。"""
        fv_name = unique_name("ValuesFV")
        criteria = json.dumps(
            [{"column": 0, "filterType": "values", "visibleValues": ["销售部"]}],
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter-view", "create",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--name", fv_name,
            "--range", "A1:D10",
            "--criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("id"), f"响应缺少 id: {data}"
        filter_view_cleanup(data["id"])

    def test_create_with_condition_criteria(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """创建带按条件筛选的视图。"""
        fv_name = unique_name("CondFV")
        criteria = json.dumps(
            [{
                "column": 2,
                "filterType": "condition",
                "conditions": [{"operator": "greater", "value": "50000"}],
            }],
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter-view", "create",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--name", fv_name,
            "--range", "A1:D10",
            "--criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("id"), f"响应缺少 id: {data}"
        filter_view_cleanup(data["id"])

    def test_create_with_multi_column_criteria(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """创建带多列筛选条件的视图。"""
        fv_name = unique_name("MultiColFV")
        criteria = json.dumps(
            [
                {"column": 0, "filterType": "values", "visibleValues": ["销售部", "市场部"]},
                {"column": 2, "filterType": "condition", "conditions": [{"operator": "greater", "value": "30000"}]},
            ],
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter-view", "create",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--name", fv_name,
            "--range", "A1:D10",
            "--criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("id"), f"响应缺少 id: {data}"
        filter_view_cleanup(data["id"])

    def test_create_with_empty_criteria(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """传空数组 criteria 创建视图。"""
        fv_name = unique_name("EmptyCrit")
        data = dws.run(
            "sheet", "filter-view", "create",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--name", fv_name,
            "--range", "A1:D10",
            "--criteria", "[]",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("id"), f"响应缺少 id: {data}"
        filter_view_cleanup(data["id"])


class TestFilterViewCreateError:
    """dws sheet filter-view create — 错误路径"""

    def test_create_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "create",
            "--sheet-id", "Sheet1",
            "--name", "test",
            "--range", "A1:D10",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_create_missing_sheet_id(self, dws):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "create",
            "--node", "SOME_NODE",
            "--name", "test",
            "--range", "A1:D10",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"

    def test_create_missing_name(self, dws):
        """缺少必填 --name 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "create",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--range", "A1:D10",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --name 应报错: {result.stdout[:200]}"

    def test_create_missing_range(self, dws):
        """缺少必填 --range 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "create",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--name", "test",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --range 应报错: {result.stdout[:200]}"

    def test_create_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "create",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--name", "test",
            "--range", "A1:D10",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    def test_create_invalid_criteria_json(self, dws, sheet_node_id, sheet_id):
        """--criteria 传非法 JSON 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "create",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--name", "test",
            "--range", "A1:D10",
            "--criteria", "NOT_VALID_JSON",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"非法 JSON 应报错: {result.stdout[:200]}"
