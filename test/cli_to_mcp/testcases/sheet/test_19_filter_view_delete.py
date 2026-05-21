"""
test_19_filter_view_delete.py — 删除筛选视图测试

依赖 conftest.py 自建的测试表格。

Commands tested:
  1. dws sheet filter-view delete --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID
"""

from test_utils import unique_name


def _create_filter_view(dws, node_id, sheet_id, cleanup=None, name=None, range_addr="A1:D10"):
    """辅助：创建筛选视图并返回 id。

    Args:
        cleanup: filter_view_cleanup fixture 回调，传入后自动注册 teardown 删除。
                 即使测试自行删除了 filter view，cleanup 的 best-effort 删除也不会报错。
    """
    fv_name = name or unique_name("FVDel")
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


class TestFilterViewDeleteBasic:
    """dws sheet filter-view delete — 基本删除"""

    def test_delete_basic(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """创建后删除筛选视图，验证 success。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        data = dws.run(
            "sheet", "filter-view", "delete",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("id") == fv_id, f"返回的 id 应为 {fv_id}: {data}"

    def test_delete_then_list_not_found(self, dws, sheet_node_id, filter_view_cleanup):
        """删除后 list 不应包含已删除的视图。"""
        sheet_name = unique_name("FVDelVerify")
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

        # 创建并删除
        fv_id = _create_filter_view(dws, sheet_node_id, new_sheet_id, cleanup=filter_view_cleanup)
        dws.run(
            "sheet", "filter-view", "delete",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--filter-view-id", fv_id,
        )

        # list 应不包含已删除视图
        fv_list = dws.run(
            "sheet", "filter-view", "list",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
        )
        filter_views = fv_list.get("filterViews", [])
        found_ids = [fv.get("id") for fv in filter_views]
        assert fv_id not in found_ids, (
            f"已删除的筛选视图 {fv_id} 不应在 list 中: {found_ids}"
        )

    def test_delete_multiple(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """创建多个筛选视图后逐个删除。"""
        fv_ids = []
        for i in range(3):
            fv_id = _create_filter_view(
                dws, sheet_node_id, sheet_id,
                cleanup=filter_view_cleanup,
                name=unique_name(f"Multi{i}"),
            )
            fv_ids.append(fv_id)

        for fv_id in fv_ids:
            data = dws.run(
                "sheet", "filter-view", "delete",
                "--node", sheet_node_id,
                "--sheet-id", sheet_id,
                "--filter-view-id", fv_id,
            )
            assert data.get("success") is True, f"删除 {fv_id} 应成功: {data}"


class TestFilterViewDeleteError:
    """dws sheet filter-view delete — 错误路径"""

    def test_delete_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "delete",
            "--sheet-id", "Sheet1",
            "--filter-view-id", "FV_ID",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_delete_missing_sheet_id(self, dws):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "delete",
            "--node", "SOME_NODE",
            "--filter-view-id", "FV_ID",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"

    def test_delete_missing_filter_view_id(self, dws):
        """缺少必填 --filter-view-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "delete",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --filter-view-id 应报错: {result.stdout[:200]}"

    def test_delete_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "delete",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--filter-view-id", "FV_ID",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    def test_delete_nonexistent_filter_view(self, dws, sheet_node_id, sheet_id):
        """删除不存在的 filterViewId 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "delete",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", "NONEXISTENT_FV_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"不存在的 filterViewId 应报错: {result.stdout[:200]}"

    def test_delete_already_deleted(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """重复删除同一个筛选视图应报错。"""
        fv_id = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)

        # 第一次删除应成功
        data = dws.run(
            "sheet", "filter-view", "delete",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
        )
        assert data.get("success") is True, f"首次删除应成功: {data}"

        # 第二次删除应报错
        result = dws.run_raw(
            "sheet", "filter-view", "delete",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"重复删除应报错: {result.stdout[:200]}"
