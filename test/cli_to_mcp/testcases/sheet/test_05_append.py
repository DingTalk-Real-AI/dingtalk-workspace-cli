"""
test_05_append.py — 工作表末尾追加数据测试

依赖 conftest.py 自建的测试表格。

Commands tested:
  1. dws sheet append --node NODE_ID --sheet-id SHEET_ID --values JSON
"""

import json

from test_utils import unique_name


class TestSheetAppend:
    """dws sheet append"""

    def test_append_single_row(self, dws, sheet_node_id, sheet_id):
        """追加单行数据，验证成功。"""
        values = json.dumps([["追加测试A", "销售部", 10000, "完成"]], ensure_ascii=False)
        data = dws.run(
            "sheet", "append",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--values", values,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_append_multiple_rows(self, dws, sheet_node_id, sheet_id):
        """追加多行数据，验证成功。"""
        values = json.dumps(
            [
                ["追加测试B", "市场部", 20000, "待处理"],
                ["追加测试C", "研发部", 30000, "完成"],
            ],
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "append",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--values", values,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_append_then_read_verify(self, dws, sheet_node_id):
        """在新工作表中追加数据后读取验证。"""
        # 创建一个干净的工作表
        sheet_name = unique_name("AppendVerify")
        dws.run(
            "sheet", "new",
            "--node", sheet_node_id,
            "--name", sheet_name,
        )

        # 获取新工作表 ID
        list_data = dws.run("sheet", "list", "--node", sheet_node_id)
        sheets = list_data.get("sheets") or list_data.get("data", {}).get("sheets") or []
        new_sheet_id = None
        for s in sheets:
            name = s.get("name") or s.get("title") or ""
            if sheet_name in name:
                new_sheet_id = s.get("sheetId") or s.get("id")
                break
        assert new_sheet_id, f"新建工作表 {sheet_name} 不在 list 中: {sheets}"

        # 追加数据
        values = json.dumps(
            [["产品", "数量"], ["苹果", 50], ["香蕉", 80]],
            ensure_ascii=False,
        )
        dws.run(
            "sheet", "append",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--values", values,
        )

        # 读取验证
        read_data = dws.run(
            "sheet", "range", "read",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
        )
        read_values = read_data.get("values") or []
        assert len(read_values) >= 3, f"应至少有 3 行数据: {read_values}"
        flat_first = [str(c) for c in read_values[0] if c]
        assert "产品" in flat_first, f"第一行应包含 '产品': {flat_first}"

    def test_append_mixed_types(self, dws, sheet_node_id, sheet_id):
        """追加包含字符串、数字、布尔值的混合类型行。"""
        values = json.dumps([["混合类型", 42, True, "2026-01-01"]], ensure_ascii=False)
        data = dws.run(
            "sheet", "append",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--values", values,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_append_by_sheet_name(self, dws, sheet_node_id, sheet_id):
        """使用工作表名称（而非 ID）作为 --sheet-id 追加数据。"""
        # 获取默认工作表名称
        list_data = dws.run("sheet", "list", "--node", sheet_node_id)
        sheets = list_data.get("sheets") or list_data.get("data", {}).get("sheets") or []
        assert sheets, f"sheet list 返回空: {list_data}"

        sheet_name = sheets[0].get("name") or sheets[0].get("title")
        if not sheet_name:
            import pytest
            pytest.skip("无法从 list 响应中获取工作表名称")

        values = json.dumps([["通过名称追加", 999]], ensure_ascii=False)
        data = dws.run(
            "sheet", "append",
            "--node", sheet_node_id,
            "--sheet-id", sheet_name,
            "--values", values,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_append_missing_values(self, dws, sheet_node_id, sheet_id):
        """缺少必填 --values 参数应报错。"""
        result = dws.run_raw(
            "sheet", "append",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --values 应报错: {result.stdout[:200]}"

    def test_append_missing_sheet_id(self, dws, sheet_node_id):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "append",
            "--node", sheet_node_id,
            "--values", '[["test"]]',
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"

    def test_append_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "append",
            "--sheet-id", "Sheet1",
            "--values", '[["test"]]',
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_append_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "append",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--values", '[["test"]]',
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"
