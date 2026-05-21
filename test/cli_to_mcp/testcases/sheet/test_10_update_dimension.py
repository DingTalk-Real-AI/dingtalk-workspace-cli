"""
test_10_update_dimension.py — 更新指定范围行/列属性测试

依赖 conftest.py 自建的测试表格。
为避免影响其他测试的种子数据，正向测试在新建工作表上操作。

Commands tested:
  1. dws sheet update-dimension --dimension ROWS --start-index "3" --length 2 --hidden
  2. dws sheet update-dimension --dimension COLUMNS --start-index "A" --length 1 --pixel-size 200
  3. dws sheet update-dimension --dimension ROWS --start-index "1" --length 5 --pixel-size 40 --hidden
"""

import json

from test_utils import unique_name


# ─── 辅助函数 ──────────────────────────────────────────────

def _create_sheet_with_data(dws, node_id, name_prefix, rows=5, cols=3):
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


class TestSheetUpdateDimensionHiddenRows:
    """dws sheet update-dimension --dimension ROWS --hidden"""

    def test_hide_rows_basic(self, dws, sheet_node_id):
        """隐藏第 2~3 行，验证核心业务字段。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "UDHideR")
        data = dws.run(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "ROWS",
            "--start-index", "2",
            "--length", "2",
            "--hidden",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("dimension") == "ROWS", f"dimension 应为 ROWS: {data}"
        assert "a1Notation" in data, f"响应缺少 a1Notation: {data}"
        assert data.get("hidden") is True, f"hidden 应为 True: {data}"

    def test_show_rows(self, dws, sheet_node_id):
        """先隐藏再显示行，验证 hidden=false 生效。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "UDShowR")
        # 先隐藏
        dws.run(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "ROWS",
            "--start-index", "2",
            "--length", "1",
            "--hidden",
        )
        # 再显示
        data = dws.run(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "ROWS",
            "--start-index", "2",
            "--length", "1",
            "--hidden=false",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("hidden") is False, f"hidden 应为 False: {data}"

    def test_hide_rows_with_sheet_prefix(self, dws, sheet_node_id):
        """start-index 携带工作表前缀（如 Sheet1!3），验证成功。"""
        sid, sname = _create_sheet_with_data(dws, sheet_node_id, "UDHideRPfx")
        position_with_prefix = f"{sname}!3"
        data = dws.run(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "ROWS",
            "--start-index", position_with_prefix,
            "--length", "1",
            "--hidden",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"


class TestSheetUpdateDimensionHiddenColumns:
    """dws sheet update-dimension --dimension COLUMNS --hidden"""

    def test_hide_columns_basic(self, dws, sheet_node_id):
        """隐藏 A~B 列，验证核心业务字段。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "UDHideC")
        data = dws.run(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "COLUMNS",
            "--start-index", "A",
            "--length", "2",
            "--hidden",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("dimension") == "COLUMNS", f"dimension 应为 COLUMNS: {data}"
        assert "a1Notation" in data, f"响应缺少 a1Notation: {data}"
        assert data.get("hidden") is True, f"hidden 应为 True: {data}"

    def test_show_columns(self, dws, sheet_node_id):
        """先隐藏再显示列，验证 hidden=false 生效。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "UDShowC")
        # 先隐藏
        dws.run(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "COLUMNS",
            "--start-index", "B",
            "--length", "1",
            "--hidden",
        )
        # 再显示
        data = dws.run(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "COLUMNS",
            "--start-index", "B",
            "--length", "1",
            "--hidden=false",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("hidden") is False, f"hidden 应为 False: {data}"


class TestSheetUpdateDimensionPixelSize:
    """dws sheet update-dimension --pixel-size"""

    def test_set_row_height(self, dws, sheet_node_id):
        """设置第 1~3 行行高为 40px，验证 pixelSize 字段。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "UDRowH")
        data = dws.run(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "ROWS",
            "--start-index", "1",
            "--length", "3",
            "--pixel-size", "40",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("pixelSize") == 40, f"pixelSize 应为 40: {data}"

    def test_set_column_width(self, dws, sheet_node_id):
        """设置 A~B 列列宽为 200px，验证 pixelSize 字段。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "UDColW")
        data = dws.run(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "COLUMNS",
            "--start-index", "A",
            "--length", "2",
            "--pixel-size", "200",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("pixelSize") == 200, f"pixelSize 应为 200: {data}"

    def test_set_pixel_size_with_hidden(self, dws, sheet_node_id):
        """同时设置行高和隐藏，验证两个字段都生效。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "UDBoth")
        data = dws.run(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "ROWS",
            "--start-index", "2",
            "--length", "2",
            "--pixel-size", "50",
            "--hidden",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("pixelSize") == 50, f"pixelSize 应为 50: {data}"
        assert data.get("hidden") is True, f"hidden 应为 True: {data}"

    def test_set_column_width_multi_letter(self, dws, sheet_node_id):
        """使用多字母列号（如 AB）设置列宽，验证成功。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "UDColAB")
        # 插入足够多的列确保 AB 列存在
        dws.run(
            "sheet", "insert-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "COLUMNS",
            "--position", "A",
            "--length", "30",
        )
        data = dws.run(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--dimension", "COLUMNS",
            "--start-index", "AB",
            "--length", "1",
            "--pixel-size", "100",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"


class TestSheetUpdateDimensionErrors:
    """update-dimension 错误路径测试"""

    def test_invalid_dimension(self, dws, sheet_node_id, sheet_id):
        """--dimension 传入无效值应报错。"""
        result = dws.run_raw(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "INVALID",
            "--start-index", "3",
            "--length", "1",
            "--hidden",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"无效 dimension 应报错: {result.stdout[:200]}"

    def test_invalid_length_zero(self, dws, sheet_node_id, sheet_id):
        """--length 为 0 应报错。"""
        result = dws.run_raw(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "ROWS",
            "--start-index", "1",
            "--length", "0",
            "--hidden",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"length=0 应报错: {result.stdout[:200]}"

    def test_invalid_length_negative(self, dws, sheet_node_id, sheet_id):
        """--length 为负数应报错。"""
        result = dws.run_raw(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "ROWS",
            "--start-index", "1",
            "--length", "-1",
            "--hidden",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"length=-1 应报错: {result.stdout[:200]}"

    def test_missing_hidden_and_pixel_size(self, dws, sheet_node_id, sheet_id):
        """--hidden 和 --pixel-size 都不传应报错。"""
        result = dws.run_raw(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "ROWS",
            "--start-index", "1",
            "--length", "1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --hidden 和 --pixel-size 应报错: {result.stdout[:200]}"

    def test_missing_dimension(self, dws, sheet_node_id, sheet_id):
        """缺少必填 --dimension 参数应报错。"""
        result = dws.run_raw(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--start-index", "3",
            "--length", "1",
            "--hidden",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --dimension 应报错: {result.stdout[:200]}"

    def test_missing_start_index(self, dws, sheet_node_id, sheet_id):
        """缺少必填 --start-index 参数应报错。"""
        result = dws.run_raw(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "ROWS",
            "--length", "1",
            "--hidden",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --start-index 应报错: {result.stdout[:200]}"

    def test_missing_length(self, dws, sheet_node_id, sheet_id):
        """缺少必填 --length 参数应报错。"""
        result = dws.run_raw(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "ROWS",
            "--start-index", "3",
            "--hidden",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --length 应报错: {result.stdout[:200]}"

    def test_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "update-dimension",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--start-index", "3",
            "--length", "1",
            "--hidden",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "update-dimension",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--start-index", "3",
            "--length", "1",
            "--hidden",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    def test_negative_pixel_size(self, dws, sheet_node_id, sheet_id):
        """--pixel-size 为负数应报错。"""
        result = dws.run_raw(
            "sheet", "update-dimension",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--dimension", "ROWS",
            "--start-index", "1",
            "--length", "1",
            "--pixel-size", "-1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"pixel-size=-1 应报错: {result.stdout[:200]}"
