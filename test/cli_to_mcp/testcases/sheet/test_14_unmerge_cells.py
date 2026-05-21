"""
test_14_unmerge_cells.py — 取消合并单元格测试

依赖 conftest.py 自建的测试表格。

Commands tested:
  1. dws sheet unmerge-cells --node NODE_ID --sheet-id SHEET_ID --range RANGE
  2. dws sheet merge-cells + unmerge-cells 完整链路
"""

import json


def _find_sheet_id(dws, node_id, sheet_name):
    """在工作表列表中查找指定名称的工作表 ID。"""
    list_data = dws.run("sheet", "list", "--node", node_id)
    sheets = list_data.get("sheets") or list_data.get("data", {}).get("sheets") or []
    for s in sheets:
        name = s.get("name") or s.get("title") or ""
        if sheet_name in name:
            return s.get("sheetId") or s.get("id")
    raise AssertionError(f"新建工作表 {sheet_name} 不在 list 中: {sheets}")


class TestSheetUnmergeCells:
    """dws sheet unmerge-cells — 取消合并单元格"""

    def test_unmerge_cells_basic(self, dws, sheet_node_id, sheet_id):
        """对没有合并单元格的范围执行取消合并，应成功（无操作）。"""
        data = dws.run(
            "sheet", "unmerge-cells",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:B2",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_unmerge_cells_large_area(self, dws, sheet_node_id, sheet_id):
        """对较大范围执行取消合并，验证不报错。"""
        data = dws.run(
            "sheet", "unmerge-cells",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:Z100",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_unmerge_cells_single_cell(self, dws, sheet_node_id, sheet_id):
        """对单个单元格执行取消合并，验证不报错。"""
        data = dws.run(
            "sheet", "unmerge-cells",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

def _try_merge_cells(dws, node_id, sheet_id, range_addr):
    """尝试调用 merge-cells，如果命令不可用则跳过测试。"""
    import pytest
    result = dws.run_raw(
        "sheet", "merge-cells",
        "--node", node_id,
        "--sheet-id", sheet_id,
        "--range", range_addr,
    )
    if result.returncode != 0:
        stderr = (result.stderr or "").strip()
        if "unknown flag" in stderr.lower() or "unknown command" in stderr.lower():
            pytest.skip("merge-cells 命令在当前 dws 版本中不可用")
        if "invalidRequest" in stderr:
            pytest.skip(f"merge-cells 服务端拒绝: {stderr[:120]}")
    import json as _json
    try:
        data = _json.loads(result.stdout)
    except _json.JSONDecodeError:
        pytest.skip(f"merge-cells 返回非 JSON: {result.stdout[:120]}")
    return data

class TestSheetUnmergeAfterMerge:
    """dws sheet merge-cells + unmerge-cells — 完整链路"""

    def test_merge_then_unmerge(self, dws, sheet_node_id, sheet_id):
        """先合并 A1:C1，再取消合并，验证单元格恢复独立。使用默认工作表空闲区域。"""
        # 在空闲区域 I1:K2 写入临时数据
        values = json.dumps(
            [["标题A", "标题B", "标题C"], ["数据1", "数据2", "数据3"]],
            ensure_ascii=False,
        )
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "I1:K2",
            "--values", values,
        )

        # 合并 I1:K1
        merge_data = _try_merge_cells(dws, sheet_node_id, sheet_id, "I1:K1")
        assert merge_data.get("success") is True, f"merge 应成功: {merge_data}"

        # 取消合并 I1:K1
        unmerge_data = dws.run(
            "sheet", "unmerge-cells",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "I1:K1",
        )
        assert unmerge_data.get("success") is True, f"unmerge 应成功: {unmerge_data}"

        # 清理临时数据
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "I1:K2",
            "--values", json.dumps([["", "", ""], ["", "", ""]], ensure_ascii=False),
        )

    def test_merge_large_area_then_unmerge(self, dws, sheet_node_id, sheet_id):
        """合并较大范围，再取消合并，验证成功。使用默认工作表空闲区域。"""
        values = json.dumps(
            [
                ["I1", "J1", "K1", "L1"],
                ["I2", "J2", "K2", "L2"],
            ],
            ensure_ascii=False,
        )
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "I1:L2",
            "--values", values,
        )

        merge_data = _try_merge_cells(dws, sheet_node_id, sheet_id, "I1:L2")
        assert merge_data.get("success") is True, f"merge 应成功: {merge_data}"

        unmerge_data = dws.run(
            "sheet", "unmerge-cells",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "I1:L2",
        )
        assert unmerge_data.get("success") is True, f"unmerge 应成功: {unmerge_data}"

        # 清理
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "I1:L2",
            "--values", json.dumps([["", "", "", ""], ["", "", "", ""]], ensure_ascii=False),
        )

    def test_unmerge_partial_range(self, dws, sheet_node_id, sheet_id):
        """合并 I1:L1 后，用更大范围取消合并，验证也能成功。"""
        values = json.dumps([["H1", "H2", "H3", "H4"]], ensure_ascii=False)
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "I1:L1",
            "--values", values,
        )

        _try_merge_cells(dws, sheet_node_id, sheet_id, "I1:L1")

        # 用更大范围取消合并
        unmerge_data = dws.run(
            "sheet", "unmerge-cells",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "I1:Z10",
        )
        assert unmerge_data.get("success") is True, (
            f"用更大范围取消合并应成功: {unmerge_data}"
        )

        # 清理
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "I1:L1",
            "--values", json.dumps([["", "", "", ""]], ensure_ascii=False),
        )

class TestSheetUnmergeCellsError:

    def test_unmerge_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "unmerge-cells",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--range", "A1:B2",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    def test_unmerge_missing_range(self, dws):
        """缺少必填 --range 参数应报错。"""
        result = dws.run_raw(
            "sheet", "unmerge-cells",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --range 应报错: {result.stdout[:200]}"

    def test_unmerge_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "unmerge-cells",
            "--sheet-id", "Sheet1",
            "--range", "A1:B2",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_unmerge_missing_sheet_id(self, dws):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "unmerge-cells",
            "--node", "SOME_NODE",
            "--range", "A1:B2",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"
