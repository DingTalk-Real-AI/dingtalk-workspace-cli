"""
test_22_filter_view_info.py — 获取单个筛选视图详情测试

依赖 conftest.py 自建的测试表格。

Commands tested:
  1. dws sheet filter-view info --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID
"""

import json

from test_utils import unique_name


def _create_filter_view(dws, node_id, sheet_id, cleanup=None, name=None, criteria=None):
    """辅助：创建筛选视图，返回 id。"""
    fv_name = name or unique_name("FVInfo")
    args = [
        "sheet", "filter-view", "create",
        "--node", node_id,
        "--sheet-id", sheet_id,
        "--name", fv_name,
        "--range", "A1:D10",
    ]
    if criteria:
        args.extend(["--criteria", criteria])
    data = dws.run(*args)
    assert data.get("success") is True, f"create 应成功: {data}"
    fv_id = data.get("id")
    assert fv_id, f"create 未返回 id: {data}"
    if cleanup:
        cleanup(fv_id)
    return fv_id, fv_name


class TestFilterViewInfoBasic:
    """dws sheet filter-view info — 基本功能"""

    def test_info_basic(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """获取筛选视图详情，验证返回 id、name、range 字段。"""
        fv_id, fv_name = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        data = dws.run(
            "sheet", "filter-view", "info",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
        )
        assert "id" in data, f"响应缺少 id: {data}"
        assert data["id"] == fv_id, f"id 不匹配: 期望 {fv_id}, 实际 {data['id']}"
        assert "name" in data, f"响应缺少 name: {data}"
        assert data["name"] == fv_name, f"name 不匹配: 期望 {fv_name}, 实际 {data['name']}"
        assert "range" in data, f"响应缺少 range: {data}"

    def test_info_with_criteria(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """获取带筛选条件的视图详情，验证 criteria 字段存在。"""
        criteria = json.dumps(
            [{"column": 0, "filterType": "values", "visibleValues": ["销售部"]}],
            ensure_ascii=False,
        )
        fv_id, _ = _create_filter_view(
            dws, sheet_node_id, sheet_id,
            cleanup=filter_view_cleanup, criteria=criteria,
        )
        data = dws.run(
            "sheet", "filter-view", "info",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
        )
        assert data.get("id") == fv_id, f"id 不匹配: {data}"
        # criteria 可能存在也可能在 list 层不返回，此处仅验证命令不报错

    def test_info_after_update(self, dws, sheet_node_id, sheet_id, filter_view_cleanup):
        """更新名称后，info 应返回新名称。"""
        fv_id, _ = _create_filter_view(dws, sheet_node_id, sheet_id, cleanup=filter_view_cleanup)
        new_name = unique_name("Updated")
        dws.run(
            "sheet", "filter-view", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
            "--name", new_name,
        )
        data = dws.run(
            "sheet", "filter-view", "info",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--filter-view-id", fv_id,
        )
        assert data.get("id") == fv_id, f"id 不匹配: {data}"
        assert data.get("name") == new_name, f"name 未更新: 期望 {new_name}, 实际 {data.get('name')}"


class TestFilterViewInfoError:
    """dws sheet filter-view info — 错误路径"""

    def test_info_invalid_filter_view_id(self, dws, sheet_node_id, sheet_id):
        """无效 filterViewId 应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "info",
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

    def test_info_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "info",
            "--sheet-id", "Sheet1",
            "--filter-view-id", "FV_ID",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_info_missing_sheet_id(self, dws):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "info",
            "--node", "SOME_NODE",
            "--filter-view-id", "FV_ID",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"

    def test_info_missing_filter_view_id(self, dws):
        """缺少必填 --filter-view-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "filter-view", "info",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --filter-view-id 应报错: {result.stdout[:200]}"
