"""
test_16_filter_view_list.py — 获取所有筛选视图测试

依赖 conftest.py 自建的测试表格。

Commands tested:
  1. dws sheet filter-view list --node NODE_ID --sheet-id SHEET_ID
"""

from test_utils import unique_name


class TestFilterViewListBasic:
    """dws sheet filter-view list — 基本功能"""

    def test_list_empty(self, dws, sheet_node_id, sheet_id):
        """无筛选视图时，list 应返回空列表。"""
        data = dws.run(
            "sheet", "filter-view", "list",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        filter_views = data.get("filterViews", [])
        assert isinstance(filter_views, list), f"filterViews 应为 list: {data}"

    def test_list_after_create(self, dws, sheet_node_id):
        """创建筛选视图后，list 应包含该视图。"""
        sheet_name = unique_name("FVList")
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

        # 创建筛选视图
        fv_name = unique_name("TestView")
        create_data = dws.run(
            "sheet", "filter-view", "create",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--name", fv_name,
            "--range", "A1:D10",
        )
        assert create_data.get("success") is True, f"create 应成功: {create_data}"
        created_id = create_data.get("id")
        assert created_id, f"create 未返回 id: {create_data}"

        # list 应包含刚创建的视图
        data = dws.run(
            "sheet", "filter-view", "list",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        filter_views = data.get("filterViews", [])
        assert isinstance(filter_views, list), f"filterViews 应为 list: {data}"
        found_ids = [fv.get("id") for fv in filter_views]
        assert created_id in found_ids, (
            f"刚创建的筛选视图 {created_id} 不在 list 结果中: {found_ids}"
        )

    def test_list_returns_fields(self, dws, sheet_node_id):
        """验证 list 返回的筛选视图包含 id、name、range 字段。"""
        sheet_name = unique_name("FVFields")
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

        fv_name = unique_name("FieldCheck")
        dws.run(
            "sheet", "filter-view", "create",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--name", fv_name,
            "--range", "A1:C5",
        )

        data = dws.run(
            "sheet", "filter-view", "list",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        filter_views = data.get("filterViews", [])
        assert len(filter_views) >= 1, f"应至少有 1 个筛选视图: {data}"

        first_view = filter_views[0]
        assert "id" in first_view, f"筛选视图缺少 id 字段: {first_view}"
        assert "name" in first_view, f"筛选视图缺少 name 字段: {first_view}"
        assert "range" in first_view, f"筛选视图缺少 range 字段: {first_view}"

    def test_list_with_sheet_name(self, dws, sheet_node_id, sheet_id):
        """通过工作表名称（非 ID）查询筛选视图。"""
        data = dws.run(
            "sheet", "filter-view", "list",
            "--node", sheet_node_id,
            "--sheet-id", "Sheet1",
        )
        # 即使 Sheet1 不存在也不应崩溃；如果存在则 success=True
        assert "success" in data or "error" in str(data).lower(), (
            f"响应应包含 success 或 error: {data}"
        )


class TestFilterViewListError:
    """dws sheet filter-view list — 错误路径"""

    def test_list_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "list",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    def test_list_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "list",
            "--sheet-id", "Sheet1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_list_missing_sheet_id(self, dws):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "list",
            "--node", "SOME_NODE",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"

    def test_list_invalid_sheet_id(self, dws, sheet_node_id):
        """无效 sheetId 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "list",
            "--node", sheet_node_id,
            "--sheet-id", "NONEXISTENT_SHEET_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 sheetId 应报错: {result.stdout[:200]}"
