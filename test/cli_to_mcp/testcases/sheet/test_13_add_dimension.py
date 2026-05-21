"""
test_13_add_dimension.py — 追加空行空列测试

依赖 conftest.py 自建的测试表格。

Commands tested:
  1. dws sheet add-dimension --node NODE_ID --sheet-id SHEET_ID --dimension ROWS --length N
  2. dws sheet add-dimension ... --dimension COLUMNS --length N
"""

from test_utils import unique_name


class TestSheetAddDimensionRows:
    """dws sheet add-dimension — 追加空行"""

    def test_add_rows(self, dws, sheet_node_id, sheet_id):
        """追加 3 行空行，验证成功。"""
        data = dws.run(
            "sheet", "add-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "ROWS",
            "--length", "3",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_add_single_row(self, dws, sheet_node_id, sheet_id):
        """追加 1 行空行。"""
        data = dws.run(
            "sheet", "add-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "ROWS",
            "--length", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_add_rows_verify_row_count(self, dws, sheet_node_id):
        """追加空行后，通过 info 验证行数增加。"""
        sheet_name = unique_name("AddRowVerify")
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

        # 获取当前行数
        info_before = dws.run(
            "sheet", "info",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
        )
        row_count_before = info_before.get("rowCount", 0)

        # 追加 5 行
        dws.run(
            "sheet", "add-dimension",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--dimension", "ROWS",
            "--length", "5",
        )

        # 验证行数增加
        info_after = dws.run(
            "sheet", "info",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
        )
        row_count_after = info_after.get("rowCount", 0)
        assert row_count_after >= row_count_before + 5, (
            f"行数应增加 5: before={row_count_before}, after={row_count_after}"
        )


class TestSheetAddDimensionColumns:
    """dws sheet add-dimension — 追加空列"""

    def test_add_columns(self, dws, sheet_node_id, sheet_id):
        """追加 2 列空列，验证成功。"""
        data = dws.run(
            "sheet", "add-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "COLUMNS",
            "--length", "2",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_add_single_column(self, dws, sheet_node_id, sheet_id):
        """追加 1 列空列。"""
        data = dws.run(
            "sheet", "add-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "COLUMNS",
            "--length", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_add_columns_verify_col_count(self, dws, sheet_node_id):
        """追加空列后，通过 info 验证列数增加。"""
        sheet_name = unique_name("AddColVerify")
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

        info_before = dws.run(
            "sheet", "info",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
        )
        col_count_before = info_before.get("columnCount", 0)

        dws.run(
            "sheet", "add-dimension",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--dimension", "COLUMNS",
            "--length", "3",
        )

        info_after = dws.run(
            "sheet", "info",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
        )
        col_count_after = info_after.get("columnCount", 0)
        assert col_count_after >= col_count_before + 3, (
            f"列数应增加 3: before={col_count_before}, after={col_count_after}"
        )

class TestSheetAddDimensionError:
    """dws sheet add-dimension — 错误路径"""

    def test_add_dimension_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "add-dimension",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--length", "1",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    def test_add_dimension_missing_dimension(self, dws):
        """缺少必填 --dimension 参数应报错。"""
        result = dws.run_raw(
            "sheet", "add-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--length", "1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --dimension 应报错: {result.stdout[:200]}"

    def test_add_dimension_missing_length(self, dws):
        """缺少必填 --length 参数应报错（默认值为 0，服务端应拒绝）。"""
        result = dws.run_raw(
            "sheet", "add-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --length 应报错: {result.stdout[:200]}"

    def test_add_dimension_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "add-dimension",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--length", "1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_add_dimension_missing_sheet_id(self, dws):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "add-dimension",
            "--node", "SOME_NODE",
            "--dimension", "ROWS",
            "--length", "1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"

    def test_add_dimension_zero_length(self, dws):
        """--length 为 0 应报错或被拒绝。"""
        result = dws.run_raw(
            "sheet", "add-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--length", "0",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"length=0 应报错: {result.stdout[:200]}"

    def test_add_dimension_negative_length(self, dws):
        """--length 为负数应报错或被拒绝。"""
        result = dws.run_raw(
            "sheet", "add-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--length", "-1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"length=-1 应报错: {result.stdout[:200]}"
