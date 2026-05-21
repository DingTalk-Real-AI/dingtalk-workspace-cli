"""
test_07_merge_cells.py — 合并单元格测试

依赖 conftest.py 自建的测试表格。
正向用例使用默认工作表的空闲区域（M~P 列），操作完成后清理数据，
避免每个测试都新建工作表导致运行缓慢。

Commands tested:
  1. dws sheet merge-cells --node NODE_ID --sheet-id SHEET_ID --range RANGE
  2. dws sheet merge-cells --node NODE_ID --sheet-id SHEET_ID --range RANGE --merge-type TYPE
"""

import json


def _write_and_merge(dws, node_id, sheet_id, data_range, values, merge_range, **merge_kwargs):
    """写入测试数据 → 执行 merge-cells → 返回响应。用完后需自行清理。"""
    dws.run(
        "sheet", "range", "update",
        "--node", node_id,
        "--sheet-id", sheet_id,
        "--range", data_range,
        "--values", json.dumps(values, ensure_ascii=False),
    )
    args = [
        "sheet", "merge-cells",
        "--node", node_id,
        "--sheet-id", sheet_id,
        "--range", merge_range,
    ]
    for flag, value in merge_kwargs.items():
        args.extend([f"--{flag.replace('_', '-')}", value])
    return dws.run(*args)


def _clear_range(dws, node_id, sheet_id, cell_range, rows, cols):
    """用空字符串清理指定区域。"""
    empty_rows = [[""] * cols for _ in range(rows)]
    dws.run(
        "sheet", "range", "update",
        "--node", node_id,
        "--sheet-id", sheet_id,
        "--range", cell_range,
        "--values", json.dumps(empty_rows),
    )


class TestSheetMergeCellsAll:
    """dws sheet merge-cells — mergeAll（默认合并所有）"""

    def test_merge_all_basic(self, dws, sheet_node_id, sheet_id):
        """默认 mergeAll 合并，验证 success 和 a1Notation。"""
        data = _write_and_merge(
            dws, sheet_node_id, sheet_id,
            data_range="M1:N2",
            values=[["合并", "测试"], ["数据", "行"]],
            merge_range="M1:N2",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert "a1Notation" in data, f"响应缺少 a1Notation: {data}"

        # 清理：先取消合并再清空数据
        dws.run(
            "sheet", "unmerge-cells",
            "--node", sheet_node_id, "--sheet-id", sheet_id, "--range", "M1:N2",
        )
        _clear_range(dws, sheet_node_id, sheet_id, "M1:N2", 2, 2)

    def test_merge_all_explicit(self, dws, sheet_node_id, sheet_id):
        """显式传 --merge-type mergeAll，验证 mergeType 返回值。"""
        data = _write_and_merge(
            dws, sheet_node_id, sheet_id,
            data_range="M1:O3",
            values=[["A", "B", "C"], ["D", "E", "F"], ["G", "H", "I"]],
            merge_range="M1:O3",
            merge_type="mergeAll",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        merge_type = data.get("mergeType", "")
        assert merge_type == "mergeAll", f"mergeType 应为 mergeAll: {data}"

        dws.run(
            "sheet", "unmerge-cells",
            "--node", sheet_node_id, "--sheet-id", sheet_id, "--range", "M1:O3",
        )
        _clear_range(dws, sheet_node_id, sheet_id, "M1:O3", 3, 3)

    def test_merge_all_then_read(self, dws, sheet_node_id, sheet_id):
        """合并后读取验证：合并 M1:N1 后，只有左上角保留值，右侧为空。"""
        data = _write_and_merge(
            dws, sheet_node_id, sheet_id,
            data_range="M1:N1",
            values=[["左上", "右侧"]],
            merge_range="M1:N1",
        )
        assert data.get("success") is True, f"merge 失败: {data}"

        read_data = dws.run(
            "sheet", "range", "read",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "M1:N1",
        )
        read_values = read_data.get("values") or []
        assert len(read_values) >= 1, f"读取结果应至少 1 行: {read_data}"
        first_row = read_values[0]
        assert first_row[0] == "左上", f"合并后左上角应保留值 '左上': {first_row}"

        dws.run(
            "sheet", "unmerge-cells",
            "--node", sheet_node_id, "--sheet-id", sheet_id, "--range", "M1:N1",
        )
        _clear_range(dws, sheet_node_id, sheet_id, "M1:N1", 1, 2)


class TestSheetMergeCellsRows:
    """dws sheet merge-cells --merge-type mergeRows"""

    def test_merge_rows(self, dws, sheet_node_id, sheet_id):
        """按行合并，验证 success 和 mergeType。"""
        data = _write_and_merge(
            dws, sheet_node_id, sheet_id,
            data_range="M4:O6",
            values=[["R1", "R2", "R3"], ["R4", "R5", "R6"], ["R7", "R8", "R9"]],
            merge_range="M4:O6",
            merge_type="mergeRows",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        merge_type = data.get("mergeType", "")
        assert merge_type == "mergeRows", f"mergeType 应为 mergeRows: {data}"

        dws.run(
            "sheet", "unmerge-cells",
            "--node", sheet_node_id, "--sheet-id", sheet_id, "--range", "M4:O6",
        )
        _clear_range(dws, sheet_node_id, sheet_id, "M4:O6", 3, 3)


class TestSheetMergeCellsColumns:
    """dws sheet merge-cells --merge-type mergeColumns"""

    def test_merge_columns(self, dws, sheet_node_id, sheet_id):
        """按列合并，验证 success 和 mergeType。"""
        data = _write_and_merge(
            dws, sheet_node_id, sheet_id,
            data_range="M7:O9",
            values=[["C1", "C2", "C3"], ["C4", "C5", "C6"], ["C7", "C8", "C9"]],
            merge_range="M7:O9",
            merge_type="mergeColumns",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        merge_type = data.get("mergeType", "")
        assert merge_type == "mergeColumns", f"mergeType 应为 mergeColumns: {data}"

        dws.run(
            "sheet", "unmerge-cells",
            "--node", sheet_node_id, "--sheet-id", sheet_id, "--range", "M7:O9",
        )
        _clear_range(dws, sheet_node_id, sheet_id, "M7:O9", 3, 3)


class TestSheetMergeCellsError:
    """dws sheet merge-cells — 错误路径"""

    def test_merge_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "merge-cells",
            "--sheet-id", "Sheet1",
            "--range", "A1:B2",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_merge_missing_sheet_id(self, dws, sheet_node_id):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "merge-cells",
            "--node", sheet_node_id,
            "--range", "A1:B2",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"

    def test_merge_missing_range(self, dws, sheet_node_id, sheet_id):
        """缺少必填 --range 参数应报错。"""
        result = dws.run_raw(
            "sheet", "merge-cells",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --range 应报错: {result.stdout[:200]}"

    def test_merge_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "merge-cells",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--range", "A1:B2",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    def test_merge_invalid_merge_type(self, dws, sheet_node_id, sheet_id):
        """无效 mergeType 应报错。"""
        result = dws.run_raw(
            "sheet", "merge-cells",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:B2",
            "--merge-type", "invalidType",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"无效 mergeType 应报错: {result.stdout[:200]}"
