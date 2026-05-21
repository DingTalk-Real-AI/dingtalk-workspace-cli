"""
test_04_range.py — 工作表数据读写测试

依赖 conftest.py 自建的测试表格，种子数据：
  A1:D1  姓名 | 部门 | 金额 | 状态
  A2:D5  张三/李四/王五/Test_User 四行数据
  C7     =SUM(C2:C5) 公式

Commands tested:
  1. dws sheet range read --node NODE_ID [--sheet-id SHEET_ID] [--range RANGE]
  2. dws sheet range update --node NODE_ID --sheet-id SHEET_ID --range RANGE --values JSON
  3. dws sheet range update ... --hyperlinks JSON
  4. dws sheet range get  （read 的别名）
"""

import json


class TestSheetRangeRead:
    """dws sheet range read"""

    def test_read_all(self, dws, sheet_node_id, sheet_id):
        """不传 --range 读取全部数据。"""
        data = dws.run(
            "sheet", "range", "read",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        values = data.get("values")
        assert isinstance(values, list), f"values 应为 list: {data}"
        assert len(values) >= 2, f"应至少有表头+1行数据: {data}"

    def test_read_specific_range(self, dws, sheet_node_id, sheet_id):
        """读取指定范围 A1:D1（表头行）。"""
        data = dws.run(
            "sheet", "range", "read",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:D1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        values = data.get("values")
        assert isinstance(values, list), f"values 应为 list: {data}"
        assert len(values) == 1, f"A1:D1 应只有 1 行: {data}"

    def test_read_data_content(self, dws, sheet_node_id, sheet_id):
        """读取 A2:A5 验证写入的姓名数据。"""
        data = dws.run(
            "sheet", "range", "read",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A2:A5",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        values = data.get("values") or []
        # 展平为一维列表
        flat = [cell for row in values for cell in row if cell]
        flat_str = [str(v) for v in flat]
        assert any("张三" in v for v in flat_str), f"A2:A5 应包含张三: {flat_str}"

    def test_read_default_sheet(self, dws, sheet_node_id):
        """不传 --sheet-id 默认读取第一个工作表。"""
        data = dws.run(
            "sheet", "range", "read",
            "--node", sheet_node_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        values = data.get("values")
        assert isinstance(values, list), f"values 应为 list: {data}"

    def test_read_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "range", "read",
            "--node", "INVALID_NODE_99999",
        )
        assert result.returncode != 0, (
            f"无效 nodeId 应报错: "
            f"returncode={result.returncode}, "
            f"stdout={result.stdout[:200]}, "
            f"stderr={result.stderr[:200]}"
        )

    def test_read_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw("sheet", "range", "read")
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"


class TestSheetRangeUpdate:
    """dws sheet range update"""

    def test_update_values(self, dws, sheet_node_id, sheet_id):
        """写入值到空区域，验证成功。"""
        values = json.dumps([["update_test_1", "update_test_2"]], ensure_ascii=False)
        data = dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "E1:F1",
            "--values", values,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_update_and_read_back(self, dws, sheet_node_id, sheet_id):
        """写入后读回验证数据一致性。"""
        write_val = "readback_test"
        values = json.dumps([[write_val]], ensure_ascii=False)
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "E2",
            "--values", values,
        )
        # 读回
        read_data = dws.run(
            "sheet", "range", "read",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "E2",
        )
        read_values = read_data.get("values") or []
        flat = [str(cell) for row in read_values for cell in row if cell]
        assert write_val in flat, f"读回数据应包含 {write_val}: {flat}"

    def test_update_formula(self, dws, sheet_node_id, sheet_id):
        """写入公式。"""
        values = json.dumps([["=1+1"]], ensure_ascii=False)
        data = dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "E3",
            "--values", values,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_update_null_clears_cell(self, dws, sheet_node_id, sheet_id):
        """写入 null 清空单元格。"""
        # 先写入值
        dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "F2",
            "--values", '[["to_be_cleared"]]',
        )
        # 再用 null 清空
        data = dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "F2",
            "--values", '[[null]]',
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_update_hyperlink(self, dws, sheet_node_id, sheet_id):
        """写入超链接。"""
        hyperlinks = json.dumps(
            [[{"type": "path", "link": "https://dingtalk.com", "text": "钉钉"}]],
            ensure_ascii=False,
        )
        data = dws.run(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "F3",
            "--hyperlinks", hyperlinks,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_update_missing_range(self, dws, sheet_node_id, sheet_id):
        """缺少必填 --range 参数应报错。"""
        result = dws.run_raw(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--values", '[["test"]]',
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --range 应报错: {result.stdout[:200]}"

    def test_update_missing_values_and_hyperlinks(self, dws, sheet_node_id, sheet_id):
        """--values 和 --hyperlinks 都不传应报错。"""
        result = dws.run_raw(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1",
        )
        assert result.returncode != 0, (
            f"缺少 --values 和 --hyperlinks 应报错: "
            f"returncode={result.returncode}, "
            f"stdout={result.stdout[:200]}, "
            f"stderr={result.stderr[:200]}"
        )

    def test_update_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "range", "update",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--range", "A1",
            "--values", '[["test"]]',
        )
        assert result.returncode != 0, (
            f"无效 nodeId 应报错: "
            f"returncode={result.returncode}, "
            f"stdout={result.stdout[:200]}, "
            f"stderr={result.stderr[:200]}"
        )

    def test_update_values_size_mismatch(self, dws, sheet_node_id, sheet_id):
        """values 形状与 range 尺寸不符时，应有明确的错误指示。

        场景：range=A1:C1（1 行 3 列），但只传入 2 列数据 [["a", "b"]]。
        期望：命令失败，且 stdout/stderr 中包含能指向"尺寸/维度/行列不匹配"的关键字。
        """
        mismatched = json.dumps([["a", "b"]], ensure_ascii=False)
        result = dws.run_raw(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:C1",
            "--values", mismatched,
        )
        # 调试辅助：无论通过与否都把服务端原始返回打出来，便于比对明确/泛化两类 message
        print(f"\n[size_mismatch] returncode={result.returncode}")
        print(f"[size_mismatch] stderr: {result.stderr[:1000]}")
        combined = (result.stdout + result.stderr).lower()
        # 首先应当命令失败
        assert result.returncode != 0 or '"success": false' in combined or '"success":false' in combined, (
            f"values 尺寸与 range 不符时应报错: "
            f"returncode={result.returncode}, "
            f"stdout={result.stdout[:500]}, "
            f"stderr={result.stderr[:500]}"
        )
        # 错误信息应包含能指向维度/尺寸不匹配的关键字，便于用户定位
        expected_keywords = [
            "range", "size", "shape", "dimension", "column", "row",
            "mismatch", "match", "length", "count",
            "列", "行", "数量", "尺寸", "维度", "不匹配", "不一致", "不符",
        ]
        assert any(k.lower() in combined for k in expected_keywords), (
            f"错误信息应包含尺寸/维度不匹配的明确指示，实际输出: "
            f"stdout={result.stdout[:500]}, stderr={result.stderr[:500]}"
        )

    def test_update_values_rows_mismatch(self, dws, sheet_node_id, sheet_id):
        """values 行数与 range 行数不符时，应有明确的错误指示。

        场景：range=A1:B2（2 行 2 列），但只传入 1 行 [["a", "b"]]。
        """
        mismatched = json.dumps([["a", "b"]], ensure_ascii=False)
        result = dws.run_raw(
            "sheet", "range", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:B2",
            "--values", mismatched,
        )
        # 调试辅助：无论通过与否都把服务端原始返回打出来，便于比对明确/泛化两类 message
        print(f"\n[rows_mismatch] returncode={result.returncode}")
        print(f"[rows_mismatch] stderr: {result.stderr[:1000]}")
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or '"success": false' in combined or '"success":false' in combined, (
            f"values 行数与 range 不符时应报错: "
            f"returncode={result.returncode}, "
            f"stdout={result.stdout[:500]}, "
            f"stderr={result.stderr[:500]}"
        )
        expected_keywords = [
            "range", "size", "shape", "dimension", "column", "row",
            "mismatch", "match", "length", "count",
            "列", "行", "数量", "尺寸", "维度", "不匹配", "不一致", "不符",
        ]
        assert any(k.lower() in combined for k in expected_keywords), (
            f"错误信息应包含尺寸/维度不匹配的明确指示，实际输出: "
            f"stdout={result.stdout[:500]}, stderr={result.stderr[:500]}"
        )


class TestSheetRangeGetAlias:
    """dws sheet range get —— read 的别名，行为应与 read 完全一致。"""

    def test_get_alias_read_all(self, dws, sheet_node_id, sheet_id):
        """使用 get 别名读取全部数据。"""
        data = dws.run(
            "sheet", "range", "get",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        values = data.get("values")
        assert isinstance(values, list), f"values 应为 list: {data}"
        assert len(values) >= 2, f"应至少有表头+1行数据: {data}"

    def test_get_alias_read_specific_range(self, dws, sheet_node_id, sheet_id):
        """使用 get 别名读取指定范围 A1:D1（表头行）。"""
        data = dws.run(
            "sheet", "range", "get",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:D1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        values = data.get("values")
        assert isinstance(values, list), f"values 应为 list: {data}"
        assert len(values) == 1, f"A1:D1 应只有 1 行: {data}"

    def test_get_alias_default_sheet(self, dws, sheet_node_id):
        """使用 get 别名且不传 --sheet-id，默认读取第一个工作表。"""
        data = dws.run(
            "sheet", "range", "get",
            "--node", sheet_node_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        values = data.get("values")
        assert isinstance(values, list), f"values 应为 list: {data}"

    def test_get_alias_equivalent_to_read(self, dws, sheet_node_id, sheet_id):
        """get 与 read 在同一范围下应返回相同的 values。"""
        read_data = dws.run(
            "sheet", "range", "read",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:D1",
        )
        get_data = dws.run(
            "sheet", "range", "get",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--range", "A1:D1",
        )
        assert read_data.get("success") is True, f"read success 应为 True: {read_data}"
        assert get_data.get("success") is True, f"get success 应为 True: {get_data}"
        assert read_data.get("values") == get_data.get("values"), (
            f"get 与 read 返回的 values 应一致: "
            f"read={read_data.get('values')}, get={get_data.get('values')}"
        )

    def test_get_alias_invalid_node(self, dws):
        """使用 get 别名传入无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "range", "get",
            "--node", "INVALID_NODE_99999",
        )
        assert result.returncode != 0, (
            f"无效 nodeId 应报错: "
            f"returncode={result.returncode}, "
            f"stdout={result.stdout[:200]}, "
            f"stderr={result.stderr[:200]}"
        )
