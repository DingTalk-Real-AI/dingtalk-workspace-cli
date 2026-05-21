"""
test_12_move_dimension.py — 移动行列测试

依赖 conftest.py 自建的测试表格。

Commands tested:
  1. dws sheet move-dimension --node NODE_ID --sheet-id SHEET_ID
       --dimension ROWS --start-index N --end-index N --destination-index N
  2. dws sheet move-dimension ... --dimension COLUMNS ...
"""

import json

from test_utils import unique_name


def _find_sheet_id(dws, node_id, sheet_name):
    """在工作表列表中查找指定名称的工作表 ID。"""
    list_data = dws.run("sheet", "list", "--node", node_id)
    sheets = list_data.get("sheets") or list_data.get("data", {}).get("sheets") or []
    for s in sheets:
        name = s.get("name") or s.get("title") or ""
        if sheet_name in name:
            return s.get("sheetId") or s.get("id")
    raise AssertionError(f"新建工作表 {sheet_name} 不在 list 中: {sheets}")


def _read_column(dws, node_id, sheet_id, col, row_count):
    """读取指定列的数据，返回扁平列表（如 ['行1', '行2', '行3']）。"""
    range_addr = f"{col}1:{col}{row_count}"
    read_data = dws.run(
        "sheet", "range", "read",
        "--node", node_id,
        "--sheet-id", sheet_id,
        "--range", range_addr,
    )
    values = read_data.get("values") or read_data.get("data", {}).get("values") or []
    return [row[0] if row else None for row in values]


def _read_row(dws, node_id, sheet_id, row, col_count):
    """读取指定行的数据，返回扁平列表（如 ['ColA', 'ColB', 'ColC']）。"""
    end_col = chr(ord("A") + col_count - 1)
    range_addr = f"A{row}:{end_col}{row}"
    read_data = dws.run(
        "sheet", "range", "read",
        "--node", node_id,
        "--sheet-id", sheet_id,
        "--range", range_addr,
    )
    values = read_data.get("values") or read_data.get("data", {}).get("values") or []
    return values[0] if values else []


class TestSheetMoveDimensionRows:
    """dws sheet move-dimension — 移动行"""

    def test_move_row_down(self, dws, sheet_node_id):
        """将第 1 行（索引 0）移到末尾，验证数据顺序变为 [行2, 行3, 行1]。"""
        sheet_name = unique_name("MoveRowDown")
        dws.run("sheet", "new", "--node", sheet_node_id, "--name", sheet_name)
        new_sheet_id = _find_sheet_id(dws, sheet_node_id, sheet_name)

        values = json.dumps(
            [["行1", "A"], ["行2", "B"], ["行3", "C"]],
            ensure_ascii=False,
        )
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--range", "A1:B3",
            "--values", values,
        )

        # 将第 1 行（索引 0）移到第 3 行之后 → destination-index=2
        data = dws.run(
            "sheet", "move-dimension",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--dimension", "ROWS",
            "--start-index", "0",
            "--end-index", "0",
            "--destination-index", "2",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

        col_a = _read_column(dws, sheet_node_id, new_sheet_id, "A", 3)
        assert col_a == ["行2", "行3", "行1"], (
            f"移动后 A 列应为 [行2, 行3, 行1]，实际: {col_a}"
        )

    def test_move_row_up(self, dws, sheet_node_id):
        """将第 3 行（索引 2）移到第 1 行位置，验证数据顺序变为 [行3, 行1, 行2]。"""
        sheet_name = unique_name("MoveRowUp")
        dws.run("sheet", "new", "--node", sheet_node_id, "--name", sheet_name)
        new_sheet_id = _find_sheet_id(dws, sheet_node_id, sheet_name)

        values = json.dumps(
            [["行1"], ["行2"], ["行3"]],
            ensure_ascii=False,
        )
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--range", "A1:A3",
            "--values", values,
        )

        data = dws.run(
            "sheet", "move-dimension",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--dimension", "ROWS",
            "--start-index", "2",
            "--end-index", "2",
            "--destination-index", "0",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

        col_a = _read_column(dws, sheet_node_id, new_sheet_id, "A", 3)
        assert col_a == ["行3", "行1", "行2"], (
            f"移动后 A 列应为 [行3, 行1, 行2]，实际: {col_a}"
        )

    def test_move_row_to_middle(self, dws, sheet_node_id):
        """将第 1 行移到第 4 行位置（中间），验证 destination-index = 目标行号 - 1。
        数据 [R1,R2,R3,R4,R5]，start=0,dest=3 → [R2,R3,R4,R1,R5]。"""
        sheet_name = unique_name("MoveRowMid")
        dws.run("sheet", "new", "--node", sheet_node_id, "--name", sheet_name)
        new_sheet_id = _find_sheet_id(dws, sheet_node_id, sheet_name)

        values = json.dumps(
            [["R1"], ["R2"], ["R3"], ["R4"], ["R5"]],
            ensure_ascii=False,
        )
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--range", "A1:A5",
            "--values", values,
        )

        # 将第 1 行（索引 0）移到第 4 行位置 → destination-index = 4 - 1 = 3
        data = dws.run(
            "sheet", "move-dimension",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--dimension", "ROWS",
            "--start-index", "0",
            "--end-index", "0",
            "--destination-index", "3",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

        col_a = _read_column(dws, sheet_node_id, new_sheet_id, "A", 5)
        assert col_a == ["R2", "R3", "R4", "R1", "R5"], (
            f"移动后 A 列应为 [R2, R3, R4, R1, R5]，实际: {col_a}"
        )

    def test_move_row_up_to_middle(self, dws, sheet_node_id):
        """将第 5 行移到第 2 行位置（向上移到中间），验证 destination-index = 目标行号 - 1。
        数据 [R1,R2,R3,R4,R5]，start=4,dest=1 → [R1,R5,R2,R3,R4]。"""
        sheet_name = unique_name("MoveRowUpMid")
        dws.run("sheet", "new", "--node", sheet_node_id, "--name", sheet_name)
        new_sheet_id = _find_sheet_id(dws, sheet_node_id, sheet_name)

        values = json.dumps(
            [["R1"], ["R2"], ["R3"], ["R4"], ["R5"]],
            ensure_ascii=False,
        )
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--range", "A1:A5",
            "--values", values,
        )

        # 将第 5 行（索引 4）移到第 2 行位置 → destination-index = 2 - 1 = 1
        data = dws.run(
            "sheet", "move-dimension",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--dimension", "ROWS",
            "--start-index", "4",
            "--end-index", "4",
            "--destination-index", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

        col_a = _read_column(dws, sheet_node_id, new_sheet_id, "A", 5)
        assert col_a == ["R1", "R5", "R2", "R3", "R4"], (
            f"移动后 A 列应为 [R1, R5, R2, R3, R4]，实际: {col_a}"
        )

    def test_move_multiple_rows(self, dws, sheet_node_id):
        """移动多行（第 2~3 行移到第 1 行之前），验证顺序变为 [R2, R3, R1, R4]。"""
        sheet_name = unique_name("MoveMultiRows")
        dws.run("sheet", "new", "--node", sheet_node_id, "--name", sheet_name)
        new_sheet_id = _find_sheet_id(dws, sheet_node_id, sheet_name)

        values = json.dumps(
            [["R1"], ["R2"], ["R3"], ["R4"]],
            ensure_ascii=False,
        )
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--range", "A1:A4",
            "--values", values,
        )

        data = dws.run(
            "sheet", "move-dimension",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--dimension", "ROWS",
            "--start-index", "1",
            "--end-index", "2",
            "--destination-index", "0",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

        col_a = _read_column(dws, sheet_node_id, new_sheet_id, "A", 4)
        assert col_a == ["R2", "R3", "R1", "R4"], (
            f"移动后 A 列应为 [R2, R3, R1, R4]，实际: {col_a}"
        )


class TestSheetMoveDimensionColumns:
    """dws sheet move-dimension — 移动列"""

    def test_move_column(self, dws, sheet_node_id):
        """将 B 列（索引 1）移到 D 列位置，验证列顺序变为 [ColA, ColC, ColD, ColB]。"""
        sheet_name = unique_name("MoveCol")
        dws.run("sheet", "new", "--node", sheet_node_id, "--name", sheet_name)
        new_sheet_id = _find_sheet_id(dws, sheet_node_id, sheet_name)

        values = json.dumps(
            [["ColA", "ColB", "ColC", "ColD"]],
            ensure_ascii=False,
        )
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--range", "A1:D1",
            "--values", values,
        )

        data = dws.run(
            "sheet", "move-dimension",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--dimension", "COLUMNS",
            "--start-index", "1",
            "--end-index", "1",
            "--destination-index", "3",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

        row1 = _read_row(dws, sheet_node_id, new_sheet_id, 1, 4)
        assert row1 == ["ColA", "ColC", "ColD", "ColB"], (
            f"移动后第 1 行应为 [ColA, ColC, ColD, ColB]，实际: {row1}"
        )


    def test_move_multiple_columns(self, dws, sheet_node_id):
        """移动多列（B~C 列移到末尾），验证列顺序变为 [ColA, ColD, ColB, ColC]。"""
        sheet_name = unique_name("MoveMultiCols")
        dws.run("sheet", "new", "--node", sheet_node_id, "--name", sheet_name)
        new_sheet_id = _find_sheet_id(dws, sheet_node_id, sheet_name)

        values = json.dumps(
            [["ColA", "ColB", "ColC", "ColD"]],
            ensure_ascii=False,
        )
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--range", "A1:D1",
            "--values", values,
        )

        # 将 B~C 列（索引 1~2）移到 D 列之后 → destination-index=3
        data = dws.run(
            "sheet", "move-dimension",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--dimension", "COLUMNS",
            "--start-index", "1",
            "--end-index", "2",
            "--destination-index", "3",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

        # 移动 2 列后，数据可能分布在 5 列（服务端扩展了列空间）
        row1 = _read_row(dws, sheet_node_id, new_sheet_id, 1, 5)
        # 过滤掉空值，只保留非空数据的顺序
        non_empty = [v for v in row1 if v]
        assert non_empty == ["ColA", "ColD", "ColB", "ColC"], (
            f"移动后非空数据应为 [ColA, ColD, ColB, ColC]，实际: {row1}"
        )

    def test_move_and_move_back(self, dws, sheet_node_id):
        """移动行后再移回（可逆性验证），数据恢复原始顺序。"""
        sheet_name = unique_name("MoveBack")
        dws.run("sheet", "new", "--node", sheet_node_id, "--name", sheet_name)
        new_sheet_id = _find_sheet_id(dws, sheet_node_id, sheet_name)

        values = json.dumps(
            [["R1"], ["R2"], ["R3"]],
            ensure_ascii=False,
        )
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--range", "A1:A3",
            "--values", values,
        )

        # 将第 1 行（索引 0）移到末尾 → destination-index=2，变为 [R2, R3, R1]
        dws.run(
            "sheet", "move-dimension",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--dimension", "ROWS",
            "--start-index", "0",
            "--end-index", "0",
            "--destination-index", "2",
        )

        # 再将末尾行（索引 2）移回第 1 行位置 → destination-index=0，恢复 [R1, R2, R3]
        data = dws.run(
            "sheet", "move-dimension",
            "--node", sheet_node_id,
            "--sheet-id", new_sheet_id,
            "--dimension", "ROWS",
            "--start-index", "2",
            "--end-index", "2",
            "--destination-index", "0",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

        col_a = _read_column(dws, sheet_node_id, new_sheet_id, "A", 3)
        assert col_a == ["R1", "R2", "R3"], (
            f"移动后再移回，顺序应恢复为 [R1, R2, R3]，实际: {col_a}"
        )

class TestSheetMoveDimensionError:
    """dws sheet move-dimension — 错误路径"""

    def test_move_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "move-dimension",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--start-index", "0",
            "--end-index", "0",
            "--destination-index", "2",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    def test_move_missing_dimension(self, dws):
        """缺少必填 --dimension 参数应报错。"""
        result = dws.run_raw(
            "sheet", "move-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--start-index", "0",
            "--end-index", "0",
            "--destination-index", "2",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --dimension 应报错: {result.stdout[:200]}"

    def test_move_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "move-dimension",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--start-index", "0",
            "--end-index", "0",
            "--destination-index", "2",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_move_missing_sheet_id(self, dws):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "move-dimension",
            "--node", "SOME_NODE",
            "--dimension", "ROWS",
            "--start-index", "0",
            "--end-index", "0",
            "--destination-index", "2",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"
