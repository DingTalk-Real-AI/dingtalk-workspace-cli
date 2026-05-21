"""
test_28_filter.py — 全局筛选（filter）完整测试

覆盖命令：
  - dws sheet filter get
  - dws sheet filter create
  - dws sheet filter delete
  - dws sheet filter update
  - dws sheet filter clear-criteria
  - dws sheet filter sort

依赖 conftest.py 自建的测试表格。
"""

import json

from test_utils import unique_name


# ─── 共用辅助函数 ─────────────────────────────────────────────────────────────


def _new_sheet(dws, node_id, prefix="Flt"):
    """创建新工作表并返回 sheet_id。"""
    sheet_name = unique_name(prefix)
    dws.run("sheet", "new", "--node", node_id, "--name", sheet_name)

    list_data = dws.run("sheet", "list", "--node", node_id)
    sheets = list_data.get("sheets") or list_data.get("data", {}).get("sheets") or []
    for s in sheets:
        name = s.get("name") or s.get("title") or ""
        if sheet_name in name:
            return s.get("sheetId") or s.get("id")
    raise AssertionError(f"新建工作表 {sheet_name} 不在 list 中: {sheets}")


def _new_sheet_with_filter(dws, node_id, prefix="Flt", range_addr="A1:E20"):
    """创建新工作表并创建空筛选，返回 sheet_id。"""
    sid = _new_sheet(dws, node_id, prefix)
    data = dws.run(
        "sheet", "filter", "create",
        "--node", node_id,
        "--sheet-id", sid,
        "--range", range_addr,
    )
    assert data.get("success") is True, f"filter create 应成功: {data}"
    return sid


def _new_sheet_with_criteria(dws, node_id, prefix="Flt"):
    """创建新工作表、创建筛选并设置条件，返回 sheet_id。"""
    sid = _new_sheet(dws, node_id, prefix)
    criteria = json.dumps(
        [{"column": 0, "filterType": "values", "visibleValues": ["A", "B"]}],
        ensure_ascii=False,
    )
    data = dws.run(
        "sheet", "filter", "create",
        "--node", node_id,
        "--sheet-id", sid,
        "--range", "A1:E20",
        "--criteria", criteria,
    )
    assert data.get("success") is True, f"filter create 应成功: {data}"
    return sid


# ─── filter get ───────────────────────────────────────────────────────────────


class TestFilterGet:
    """dws sheet filter get"""

    def test_get_no_filter(self, dws, sheet_node_id):
        """无筛选时 get 应返回成功但筛选信息为空。"""
        sid = _new_sheet(dws, sheet_node_id, "FGetE")
        data = dws.run(
            "sheet", "filter", "get",
            "--node", sheet_node_id,
            "--sheet-id", sid,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_get_with_filter(self, dws, sheet_node_id):
        """创建筛选后 get 应返回筛选范围。"""
        sid = _new_sheet_with_filter(dws, sheet_node_id, "FGetF")
        data = dws.run(
            "sheet", "filter", "get",
            "--node", sheet_node_id,
            "--sheet-id", sid,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("range"), f"应返回筛选范围: {data}"

    def test_get_missing_required_flags(self, dws):
        """缺少必填参数应报错。"""
        result = dws.run_raw("sheet", "filter", "get", "--sheet-id", "Sheet1")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

        result = dws.run_raw("sheet", "filter", "get", "--node", "SOME_NODE")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()


# ─── filter create ────────────────────────────────────────────────────────────


class TestFilterCreate:
    """dws sheet filter create"""

    def test_create_empty_filter(self, dws, sheet_node_id):
        """创建不带条件的空筛选。"""
        sid = _new_sheet(dws, sheet_node_id, "FCreE")
        data = dws.run(
            "sheet", "filter", "create",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--range", "A1:D10",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_create_with_values_criteria(self, dws, sheet_node_id):
        """创建带按值筛选条件的筛选。"""
        sid = _new_sheet(dws, sheet_node_id, "FCreV")
        criteria = json.dumps(
            [{"column": 0, "filterType": "values", "visibleValues": ["销售部"]}],
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter", "create",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--range", "A1:D10",
            "--criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_create_with_condition_criteria(self, dws, sheet_node_id):
        """创建带按条件筛选的筛选。"""
        sid = _new_sheet(dws, sheet_node_id, "FCreC")
        criteria = json.dumps(
            [{"column": 1, "filterType": "condition", "conditions": [{"operator": "greater", "value": "100"}]}],
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter", "create",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--range", "A1:E20",
            "--criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_create_verify_by_get(self, dws, sheet_node_id):
        """创建后通过 filter get 验证。"""
        sid = _new_sheet(dws, sheet_node_id, "FCreG")
        dws.run(
            "sheet", "filter", "create",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--range", "A1:F50",
        )
        data = dws.run(
            "sheet", "filter", "get",
            "--node", sheet_node_id,
            "--sheet-id", sid,
        )
        assert data.get("success") is True
        assert data.get("range"), f"应返回筛选范围: {data}"

    def test_create_duplicate_filter(self, dws, sheet_node_id):
        """同一工作表重复创建筛选应报错。"""
        sid = _new_sheet_with_filter(dws, sheet_node_id, "FCreD")
        result = dws.run_raw(
            "sheet", "filter", "create",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--range", "A1:D10",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"重复创建筛选应报错: {result.stdout[:200]}"

    def test_create_invalid_criteria_json(self, dws, sheet_node_id, sheet_id):
        """--criteria 传非法 JSON 应报错。"""
        result = dws.run_raw(
            "sheet", "filter", "create",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:D10",
            "--criteria", "NOT_VALID_JSON",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        )

    def test_create_missing_range(self, dws):
        """缺少必填 --range 应报错。"""
        result = dws.run_raw(
            "sheet", "filter", "create",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()


# ─── filter delete ────────────────────────────────────────────────────────────


class TestFilterDelete:
    """dws sheet filter delete"""

    def test_delete_basic(self, dws, sheet_node_id):
        """创建筛选后删除，验证 success。"""
        sid = _new_sheet_with_filter(dws, sheet_node_id, "FDelB")
        data = dws.run(
            "sheet", "filter", "delete",
            "--node", sheet_node_id,
            "--sheet-id", sid,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_delete_then_get_empty(self, dws, sheet_node_id):
        """删除后 filter get 应返回空筛选信息。"""
        sid = _new_sheet_with_filter(dws, sheet_node_id, "FDelV")
        dws.run("sheet", "filter", "delete", "--node", sheet_node_id, "--sheet-id", sid)
        data = dws.run("sheet", "filter", "get", "--node", sheet_node_id, "--sheet-id", sid)
        assert data.get("success") is True
        assert not data.get("range"), f"删除后不应有 range: {data}"

    def test_delete_no_filter_exists(self, dws, sheet_node_id):
        """工作表没有筛选时删除应报错。"""
        sid = _new_sheet(dws, sheet_node_id, "FDelN")
        result = dws.run_raw(
            "sheet", "filter", "delete",
            "--node", sheet_node_id,
            "--sheet-id", sid,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_delete_already_deleted(self, dws, sheet_node_id):
        """重复删除应报错。"""
        sid = _new_sheet_with_filter(dws, sheet_node_id, "FDelR")
        dws.run("sheet", "filter", "delete", "--node", sheet_node_id, "--sheet-id", sid)
        result = dws.run_raw(
            "sheet", "filter", "delete",
            "--node", sheet_node_id,
            "--sheet-id", sid,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


# ─── filter update ────────────────────────────────────────────────────────────


class TestFilterUpdate:
    """dws sheet filter update"""

    def test_update_values(self, dws, sheet_node_id):
        """更新单列按值筛选条件。"""
        sid = _new_sheet_with_filter(dws, sheet_node_id, "FUpdV")
        criteria = json.dumps(
            [{"column": 0, "filterType": "values", "visibleValues": ["A", "B"]}],
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_update_condition(self, dws, sheet_node_id):
        """更新单列按条件筛选。"""
        sid = _new_sheet_with_filter(dws, sheet_node_id, "FUpdC")
        criteria = json.dumps(
            [{"column": 1, "filterType": "condition", "conditions": [{"operator": "greater", "value": "100"}]}],
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_update_multi_column(self, dws, sheet_node_id):
        """同时更新多列筛选条件。"""
        sid = _new_sheet_with_filter(dws, sheet_node_id, "FUpdM")
        criteria = json.dumps(
            [
                {"column": 0, "filterType": "values", "visibleValues": ["X"]},
                {"column": 2, "filterType": "condition", "conditions": [{"operator": "less-equal", "value": "200"}]},
            ],
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_update_color_filter(self, dws, sheet_node_id):
        """更新按颜色筛选条件。"""
        sid = _new_sheet_with_filter(dws, sheet_node_id, "FUpdCo")
        criteria = json.dumps(
            [{"column": 0, "filterType": "color", "backgroundColor": "#FF0000"}],
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "filter", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--criteria", criteria,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_update_invalid_criteria_json(self, dws, sheet_node_id, sheet_id):
        """--criteria 传非法 JSON 应报错。"""
        result = dws.run_raw(
            "sheet", "filter", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--criteria", "INVALID_JSON",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        )

    def test_update_no_filter_exists(self, dws, sheet_node_id):
        """工作表没有筛选时 update 应报错。"""
        sid = _new_sheet(dws, sheet_node_id, "FUpdN")
        criteria = json.dumps(
            [{"column": 0, "filterType": "values", "visibleValues": ["A"]}],
            ensure_ascii=False,
        )
        result = dws.run_raw(
            "sheet", "filter", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--criteria", criteria,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


# ─── filter clear-criteria ────────────────────────────────────────────────────


class TestFilterClearCriteria:
    """dws sheet filter clear-criteria"""

    def test_clear_existing_criteria(self, dws, sheet_node_id):
        """清除已设置条件的列。"""
        sid = _new_sheet_with_criteria(dws, sheet_node_id, "FClrE")
        data = dws.run(
            "sheet", "filter", "clear-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--column", "0",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_clear_no_criteria_column(self, dws, sheet_node_id):
        """清除未设置条件的列（幂等，不报错）。"""
        sid = _new_sheet_with_criteria(dws, sheet_node_id, "FClrN")
        data = dws.run(
            "sheet", "filter", "clear-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--column", "3",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_clear_then_verify(self, dws, sheet_node_id):
        """清除后筛选本身仍存在。"""
        sid = _new_sheet_with_criteria(dws, sheet_node_id, "FClrV")
        dws.run(
            "sheet", "filter", "clear-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--column", "0",
        )
        data = dws.run("sheet", "filter", "get", "--node", sheet_node_id, "--sheet-id", sid)
        assert data.get("success") is True
        assert data.get("range"), f"筛选本身应仍存在: {data}"

    def test_clear_no_filter_exists(self, dws, sheet_node_id):
        """工作表没有筛选时 clear-criteria 应报错。"""
        sid = _new_sheet(dws, sheet_node_id, "FClrF")
        result = dws.run_raw(
            "sheet", "filter", "clear-criteria",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--column", "0",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


# ─── filter sort ──────────────────────────────────────────────────────────────


class TestFilterSort:
    """dws sheet filter sort"""

    def test_sort_ascending(self, dws, sheet_node_id):
        """按第一列升序排序。"""
        sid = _new_sheet_with_filter(dws, sheet_node_id, "FSrtA")
        data = dws.run(
            "sheet", "filter", "sort",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--column", "0",
            "--ascending",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_sort_descending(self, dws, sheet_node_id):
        """按第一列降序排序。"""
        sid = _new_sheet_with_filter(dws, sheet_node_id, "FSrtD")
        data = dws.run(
            "sheet", "filter", "sort",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--column", "0",
            "--ascending=false",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_sort_different_column(self, dws, sheet_node_id):
        """按第 3 列排序。"""
        sid = _new_sheet_with_filter(dws, sheet_node_id, "FSrtC")
        data = dws.run(
            "sheet", "filter", "sort",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--column", "2",
            "--ascending",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_sort_no_filter_exists(self, dws, sheet_node_id):
        """工作表没有筛选时 sort 应报错。"""
        sid = _new_sheet(dws, sheet_node_id, "FSrtN")
        result = dws.run_raw(
            "sheet", "filter", "sort",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--column", "0",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
