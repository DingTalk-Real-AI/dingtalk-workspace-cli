"""
test_08_insert_dimension.py — 在指定位置插入行或列测试

依赖 conftest.py 自建的测试表格。

Commands tested:
  1. dws sheet insert-dimension --node NODE_ID --sheet-id SHEET_ID --dimension ROWS --position "3" --length 2
  2. dws sheet insert-dimension --node NODE_ID --sheet-id SHEET_ID --dimension COLUMNS --position "A" --length 1
"""

from test_utils import unique_name


class TestSheetInsertDimensionRows:
    """dws sheet insert-dimension (ROWS)"""

    def test_insert_rows_basic(self, dws, sheet_node_id, sheet_id):
        """在第 2 行之前插入 1 行，验证核心业务字段。"""
        data = dws.run(
            "sheet", "insert-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "ROWS",
            "--position", "2",
            "--length", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("dimension") == "ROWS", f"dimension 应为 ROWS: {data}"
        assert "a1Notation" in data, f"响应缺少 a1Notation: {data}"

    def test_insert_rows_multiple(self, dws, sheet_node_id, sheet_id):
        """插入多行（3 行），验证 length 字段。"""
        data = dws.run(
            "sheet", "insert-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "ROWS",
            "--position", "5",
            "--length", "3",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("length") == 3, f"length 应为 3: {data}"

    def test_insert_rows_with_sheet_prefix(self, dws, sheet_node_id, sheet_id):
        """position 携带工作表前缀（如 Sheet1!3），验证成功。"""
        # 获取工作表名称用于前缀
        list_data = dws.run("sheet", "list", "--node", sheet_node_id)
        sheets = list_data.get("sheets") or list_data.get("data", {}).get("sheets") or []
        assert sheets, f"sheet list 返回空: {list_data}"
        sheet_name = sheets[0].get("name") or sheets[0].get("title")
        if not sheet_name:
            import pytest
            pytest.skip("无法从 list 响应中获取工作表名称")

        position_with_prefix = f"{sheet_name}!3"
        data = dws.run(
            "sheet", "insert-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "ROWS",
            "--position", position_with_prefix,
            "--length", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"


class TestSheetInsertDimensionColumns:
    """dws sheet insert-dimension (COLUMNS)"""

    def test_insert_columns_basic(self, dws, sheet_node_id, sheet_id):
        """在 A 列之前插入 1 列，验证核心业务字段。"""
        data = dws.run(
            "sheet", "insert-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "COLUMNS",
            "--position", "A",
            "--length", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("dimension") == "COLUMNS", f"dimension 应为 COLUMNS: {data}"
        assert "a1Notation" in data, f"响应缺少 a1Notation: {data}"

    def test_insert_columns_multiple(self, dws, sheet_node_id, sheet_id):
        """在 C 列之前插入 2 列，验证 length 字段。"""
        data = dws.run(
            "sheet", "insert-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "COLUMNS",
            "--position", "C",
            "--length", "2",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("length") == 2, f"length 应为 2: {data}"

    def test_insert_columns_multi_letter(self, dws, sheet_node_id, sheet_id):
        """使用多字母列号（如 AB），验证成功。"""
        data = dws.run(
            "sheet", "insert-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "COLUMNS",
            "--position", "AB",
            "--length", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"


class TestSheetInsertDimensionOnNewSheet:
    """在新建工作表上测试 insert-dimension，避免影响其他测试的种子数据。"""

    def test_insert_rows_then_read(self, dws, sheet_node_id):
        """新建工作表 → 写入数据 → 插入行 → 读取验证行数增加。"""
        import json

        # 1. 新建工作表
        sheet_name = unique_name("InsertDimTest")
        dws.run("sheet", "new", "--node", sheet_node_id, "--name", sheet_name)

        # 2. 获取新工作表 ID
        list_data = dws.run("sheet", "list", "--node", sheet_node_id)
        sheets = list_data.get("sheets") or list_data.get("data", {}).get("sheets") or []
        new_sheet_id = None
        for s in sheets:
            name = s.get("name") or s.get("title") or ""
            if sheet_name in name:
                new_sheet_id = s.get("sheetId") or s.get("id")
                break
        assert new_sheet_id, f"新建工作表 {sheet_name} 不在 list 中: {sheets}"

        # 3. 写入 3 行数据
        values = json.dumps(
            [["A", "B"], ["C", "D"], ["E", "F"]],
            ensure_ascii=False,
        )
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--range", "A1:B3",
            "--values", values,
        )

        # 4. 在第 2 行之前插入 2 行
        insert_data = dws.run(
            "sheet", "insert-dimension",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--dimension", "ROWS",
            "--position", "2",
            "--length", "2",
        )
        assert insert_data.get("success") is True, f"insert 失败: {insert_data}"

        # 5. 读取验证：原来 3 行 → 现在应有 5 行（含 2 空行）
        read_data = dws.run(
            "sheet", "range", "read",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
        )
        read_values = read_data.get("values") or []
        assert len(read_values) >= 5, (
            f"插入 2 行后应至少有 5 行数据，实际 {len(read_values)} 行: {read_values}"
        )


class TestSheetInsertDimensionErrors:
    """insert-dimension 错误路径测试"""

    def test_invalid_dimension(self, dws, sheet_node_id, sheet_id):
        """--dimension 传入无效值应报错。"""
        result = dws.run_raw(
            "sheet", "insert-dimension",
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
            "sheet", "insert-dimension",
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
            "sheet", "insert-dimension",
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
            "sheet", "insert-dimension",
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
            "sheet", "insert-dimension",
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
            "sheet", "insert-dimension",
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
            "sheet", "insert-dimension",
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
            "sheet", "insert-dimension",
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
