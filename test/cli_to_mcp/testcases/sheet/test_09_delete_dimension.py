"""
test_09_delete_dimension.py — 删除指定位置的行或列测试

依赖 conftest.py 自建的测试表格。
为避免影响其他测试的种子数据，正向测试在新建工作表上操作。

Commands tested:
  1. dws sheet delete-dimension --node NODE_ID --sheet-id SHEET_ID --dimension ROWS --position "3" --length 2
  2. dws sheet delete-dimension --node NODE_ID --sheet-id SHEET_ID --dimension COLUMNS --position "A" --length 1
"""

import json

from test_utils import unique_name


# ─── 辅助函数 ──────────────────────────────────────────────

def _create_sheet_with_data(dws, node_id, name_prefix, rows, cols=2):
    """新建工作表并写入指定行数的数据，返回 (sheet_id, sheet_name)。"""
    sheet_name = unique_name(name_prefix)
    dws.run("sheet", "new", "--node", node_id, "--name", sheet_name)

    list_data = dws.run("sheet", "list", "--node", node_id)
    sheets = list_data.get("sheets") or list_data.get("data", {}).get("sheets") or []
    new_sheet_id = None
    for s in sheets:
        n = s.get("name") or s.get("title") or ""
        if sheet_name in n:
            new_sheet_id = s.get("sheetId") or s.get("id")
            break
    assert new_sheet_id, f"新建工作表 {sheet_name} 不在 list 中: {sheets}"

    # 生成数据：每行 cols 列，值为 "R{row}C{col}"
    values = [[f"R{r}C{c}" for c in range(1, cols + 1)] for r in range(1, rows + 1)]
    end_col = chr(ord("A") + cols - 1)
    range_addr = f"A1:{end_col}{rows}"
    dws.run(
        "sheet", "range", "update",
        "--node", node_id,
        "--sheet-id", new_sheet_id,
        "--range", range_addr,
        "--values", json.dumps(values, ensure_ascii=False),
    )
    return new_sheet_id, sheet_name


class TestSheetDeleteDimensionRows:
    """dws sheet delete-dimension (ROWS)"""

    def test_delete_rows_basic(self, dws, sheet_node_id):
        """删除 1 行，验证核心业务字段。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "DelRow", 5)
        data = dws.run(
            "sheet", "delete-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "ROWS",
            "--position", "2",
            "--length", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("dimension") == "ROWS", f"dimension 应为 ROWS: {data}"
        assert "a1Notation" in data, f"响应缺少 a1Notation: {data}"

    def test_delete_rows_multiple(self, dws, sheet_node_id):
        """删除多行（3 行），验证 length 字段。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "DelRows3", 6)
        data = dws.run(
            "sheet", "delete-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "ROWS",
            "--position", "2",
            "--length", "3",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("length") == 3, f"length 应为 3: {data}"

    def test_delete_rows_with_sheet_prefix(self, dws, sheet_node_id, sheet_id):
        """position 携带工作表前缀（如 Sheet1!3），验证成功。"""
        # 获取工作表名称用于前缀
        list_data = dws.run("sheet", "list", "--node", sheet_node_id)
        sheets = list_data.get("sheets") or list_data.get("data", {}).get("sheets") or []
        assert sheets, f"sheet list 返回空: {list_data}"

        # 在新工作表上操作，避免影响种子数据
        sid, sname = _create_sheet_with_data(dws, sheet_node_id, "DelRowPfx", 5)

        position_with_prefix = f"{sname}!3"
        data = dws.run(
            "sheet", "delete-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "ROWS",
            "--position", position_with_prefix,
            "--length", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_delete_rows_then_read(self, dws, sheet_node_id):
        """新建工作表 → 写入 5 行 → 删除 2 行 → 读取验证行数减少。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "DelRowRead", 5)

        # 删除第 2~3 行
        delete_data = dws.run(
            "sheet", "delete-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "ROWS",
            "--position", "2",
            "--length", "2",
        )
        assert delete_data.get("success") is True, f"delete 失败: {delete_data}"

        # 读取验证：原来 5 行 → 现在应有 3 行
        read_data = dws.run(
            "sheet", "range", "read",
            "--node", sheet_node_id,
            "--sheet-id", sid,
        )
        read_values = read_data.get("values") or []
        assert len(read_values) == 3, (
            f"删除 2 行后应有 3 行数据，实际 {len(read_values)} 行: {read_values}"
        )


class TestSheetDeleteDimensionColumns:
    """dws sheet delete-dimension (COLUMNS)"""

    def test_delete_columns_basic(self, dws, sheet_node_id):
        """从 A 列开始删除 1 列，验证核心业务字段。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "DelCol", 3, cols=4)
        data = dws.run(
            "sheet", "delete-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "COLUMNS",
            "--position", "A",
            "--length", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("dimension") == "COLUMNS", f"dimension 应为 COLUMNS: {data}"
        assert "a1Notation" in data, f"响应缺少 a1Notation: {data}"

    def test_delete_columns_multiple(self, dws, sheet_node_id):
        """从 B 列开始删除 2 列，验证 length 字段。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "DelCols2", 3, cols=5)
        data = dws.run(
            "sheet", "delete-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "COLUMNS",
            "--position", "B",
            "--length", "2",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("length") == 2, f"length 应为 2: {data}"

    def test_delete_columns_multi_letter(self, dws, sheet_node_id):
        """使用多字母列号（如 AB），验证成功。"""
        # 先插入足够多的列，确保 AB 列存在
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "DelColAB", 2, cols=2)
        # 插入 30 列确保 AB 列存在
        dws.run(
            "sheet", "insert-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "COLUMNS",
            "--position", "A",
            "--length", "30",
        )
        data = dws.run(
            "sheet", "delete-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "COLUMNS",
            "--position", "AB",
            "--length", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_delete_columns_then_read(self, dws, sheet_node_id):
        """新建工作表 → 写入 4 列 → 删除 2 列 → 读取验证列数减少。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "DelColRead", 3, cols=4)

        # 删除 B~C 列
        delete_data = dws.run(
            "sheet", "delete-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "COLUMNS",
            "--position", "B",
            "--length", "2",
        )
        assert delete_data.get("success") is True, f"delete 失败: {delete_data}"

        # 读取验证：原来 4 列 → 现在应有 2 列
        read_data = dws.run(
            "sheet", "range", "read",
            "--node", sheet_node_id,
            "--sheet-id", sid,
        )
        read_values = read_data.get("values") or []
        assert len(read_values) > 0, f"读取数据为空: {read_values}"
        first_row = read_values[0]
        assert len(first_row) == 2, (
            f"删除 2 列后应有 2 列数据，实际 {len(first_row)} 列: {first_row}"
        )


class TestSheetDeleteDimensionErrors:
    """delete-dimension 错误路径测试"""

    def test_invalid_dimension(self, dws, sheet_node_id, sheet_id):
        """--dimension 传入无效值应报错。"""
        result = dws.run_raw(
            "sheet", "delete-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "INVALID",
            "--position", "3",
            "--length", "1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"无效 dimension 应报错: {result.stdout[:200]}"

    def test_invalid_length_zero(self, dws, sheet_node_id, sheet_id):
        """--length 为 0 应报错。"""
        result = dws.run_raw(
            "sheet", "delete-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "ROWS",
            "--position", "1",
            "--length", "0",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"length=0 应报错: {result.stdout[:200]}"

    def test_invalid_length_negative(self, dws, sheet_node_id, sheet_id):
        """--length 为负数应报错。"""
        result = dws.run_raw(
            "sheet", "delete-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "ROWS",
            "--position", "1",
            "--length", "-1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"length=-1 应报错: {result.stdout[:200]}"

    def test_missing_dimension(self, dws, sheet_node_id, sheet_id):
        """缺少必填 --dimension 参数应报错。"""
        result = dws.run_raw(
            "sheet", "delete-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--position", "3",
            "--length", "1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --dimension 应报错: {result.stdout[:200]}"

    def test_missing_position(self, dws, sheet_node_id, sheet_id):
        """缺少必填 --position 参数应报错。"""
        result = dws.run_raw(
            "sheet", "delete-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "ROWS",
            "--length", "1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --position 应报错: {result.stdout[:200]}"

    def test_missing_length(self, dws, sheet_node_id, sheet_id):
        """缺少必填 --length 参数应报错。"""
        result = dws.run_raw(
            "sheet", "delete-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "ROWS",
            "--position", "3",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --length 应报错: {result.stdout[:200]}"

    def test_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "delete-dimension",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--position", "3",
            "--length", "1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "delete-dimension",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--position", "3",
            "--length", "1",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"
