"""
test_11_replace.py — 全局查找替换测试

依赖 conftest.py 提供的 replace_sheet fixture（function 级别），
每个测试用例都在独立的工作表中运行，避免测试间数据污染。

种子数据（由 replace_sheet fixture 写入）：
  A1:D1  姓名 | 部门 | 金额 | 状态
  A2:D2  张三 | 销售部 | 50000 | 完成
  A3:D3  李四 | 市场部 | 38000 | 待处理
  A4:D4  王五 | 销售部 | 62000 | 完成
  A5:D5  Test_User | 研发部 | 99000 | pending

Commands tested:
  1. dws sheet replace --node NODE_ID --sheet-id SHEET_ID --find TEXT --replacement TEXT
  2. dws sheet replace ... --range RANGE
  3. dws sheet replace ... --match-case
  4. dws sheet replace ... --match-entire-cell
  5. dws sheet replace ... --use-regexp
  6. dws sheet replace ... --include-hidden
"""

import json


class TestSheetReplaceBasic:
    """dws sheet replace — 基本替换功能"""

    def test_replace_basic(self, dws, replace_sheet):
        """替换种子数据中的"销售部"→"营销部"，应命中 2 个。"""
        node_id = replace_sheet["node_id"]
        sheet_id = replace_sheet["sheet_id"]
        # 种子数据 B2="销售部", B4="销售部"，应命中 2 个
        data = dws.run(
            "sheet", "replace",
            "--node", node_id,
            "--sheet-id", sheet_id,
            "--find", "销售部",
            "--replacement", "营销部",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert "replaceCount" in data, f"响应缺少 replaceCount: {data}"
        assert data["replaceCount"] >= 2, f"销售部应至少命中 2 个单元格: {data}"

    def test_replace_to_empty_and_restore(self, dws, replace_sheet):
        """写入临时数据到空闲区，替换为空（删除）。"""
        node_id = replace_sheet["node_id"]
        sheet_id = replace_sheet["sheet_id"]
        # 在空闲区域 G1:G2 写入临时数据
        dws.run(
            "sheet", "range", "update",
            "--node", node_id,
            "--sheet-id", sheet_id,
            "--range", "G1:G2",
            "--values", json.dumps([["TEMP_RPL"], ["TEMP_RPL"]], ensure_ascii=False),
        )
        data = dws.run(
            "sheet", "replace",
            "--node", node_id,
            "--sheet-id", sheet_id,
            "--find", "TEMP_RPL",
            "--replacement", "",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("replaceCount", 0) >= 2, f"应至少替换 2 个单元格: {data}"

    def test_replace_no_match(self, dws, replace_sheet):
        """查找不存在的文本，replaceCount 应为 0。"""
        node_id = replace_sheet["node_id"]
        sheet_id = replace_sheet["sheet_id"]
        data = dws.run(
            "sheet", "replace",
            "--node", node_id,
            "--sheet-id", sheet_id,
            "--find", "__NO_MATCH_REPLACE_99999__",
            "--replacement", "whatever",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("replaceCount") == 0, f"replaceCount 应为 0: {data}"


class TestSheetReplaceOptions:
    """dws sheet replace — 可选参数"""

    def test_replace_with_range(self, dws, replace_sheet):
        """--range 限定替换范围：只在 B2:B3 内替换"销售部"。"""
        node_id = replace_sheet["node_id"]
        sheet_id = replace_sheet["sheet_id"]
        # 种子数据 B2="销售部", B3="市场部", B4="销售部"
        # 限定 B2:B3 只应命中 B2 的"销售部"，B4 不在范围内
        data = dws.run(
            "sheet", "replace",
            "--node", node_id,
            "--sheet-id", sheet_id,
            "--find", "销售部",
            "--replacement", "技术部",
            "--range", "B2:B3",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("replaceCount", 0) == 1, (
            f"限定 B2:B3 范围应只替换 1 个: {data}"
        )

    def test_replace_match_entire_cell(self, dws, replace_sheet):
        """--match-entire-cell 完整单元格匹配"完成"。"""
        node_id = replace_sheet["node_id"]
        sheet_id = replace_sheet["sheet_id"]
        # 种子数据 D2="完成", D3="待处理", D4="完成"
        # 精确匹配"完成"应命中 2 个，不会匹配"待处理"
        data = dws.run(
            "sheet", "replace",
            "--node", node_id,
            "--sheet-id", sheet_id,
            "--find", "完成",
            "--replacement", "已完成",
            "--match-entire-cell",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("replaceCount", 0) == 2, (
            f"精确匹配'完成'应替换 2 个单元格: {data}"
        )

    def test_replace_match_case(self, dws, replace_sheet):
        """--match-case 区分大小写替换"Test_User"。"""
        node_id = replace_sheet["node_id"]
        sheet_id = replace_sheet["sheet_id"]
        # 种子数据 A5="Test_User"，区分大小写应命中 1 个
        data = dws.run(
            "sheet", "replace",
            "--node", node_id,
            "--sheet-id", sheet_id,
            "--find", "Test_User",
            "--replacement", "Test_Admin",
            "--match-case",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("replaceCount", 0) == 1, (
            f"区分大小写应只替换 1 个单元格: {data}"
        )

    def test_replace_include_hidden(self, dws, replace_sheet):
        """--include-hidden 包含隐藏行列的替换。"""
        node_id = replace_sheet["node_id"]
        sheet_id = replace_sheet["sheet_id"]
        data = dws.run(
            "sheet", "replace",
            "--node", node_id,
            "--sheet-id", sheet_id,
            "--find", "销售部",
            "--replacement", "运营部",
            "--include-hidden",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert "replaceCount" in data, f"响应缺少 replaceCount: {data}"
        assert data["replaceCount"] >= 2, (
            f"include-hidden 应至少命中 2 个单元格: {data}"
        )

    def test_replace_combined_options(self, dws, replace_sheet):
        """--match-case + --range 组合使用。"""
        node_id = replace_sheet["node_id"]
        sheet_id = replace_sheet["sheet_id"]
        # 种子数据 A5="Test_User"，在 A1:A10 范围内区分大小写应命中 1 个
        data = dws.run(
            "sheet", "replace",
            "--node", node_id,
            "--sheet-id", sheet_id,
            "--find", "Test_User",
            "--replacement", "Test_Staff",
            "--match-case",
            "--range", "A1:A10",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("replaceCount", 0) == 1, (
            f"组合选项应只替换 1 个单元格: {data}"
        )

    def test_replace_use_regexp(self, dws, replace_sheet):
        """--use-regexp 正则表达式替换：匹配"张"或"王"开头的姓名。"""
        node_id = replace_sheet["node_id"]
        sheet_id = replace_sheet["sheet_id"]
        # 种子数据 A2="张三", A4="王五"，正则应命中 2 个
        data = dws.run(
            "sheet", "replace",
            "--node", node_id,
            "--sheet-id", sheet_id,
            "--find", "^[张王].+",
            "--replacement", "REPLACED",
            "--use-regexp",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("replaceCount", 0) >= 2, (
            f"正则应至少替换 2 个单元格: {data}"
        )


class TestSheetReplaceError:
    """dws sheet replace — 错误路径"""

    def test_replace_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "replace",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--find", "test",
            "--replacement", "new",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    def test_replace_missing_find(self, dws):
        """缺少必填 --find 参数应报错。"""
        result = dws.run_raw(
            "sheet", "replace",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--replacement", "new",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --find 应报错: {result.stdout[:200]}"

    def test_replace_missing_replacement(self, dws):
        """缺少必填 --replacement 参数应报错。"""
        result = dws.run_raw(
            "sheet", "replace",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--find", "test",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --replacement 应报错: {result.stdout[:200]}"

    def test_replace_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "replace",
            "--sheet-id", "Sheet1",
            "--find", "test",
            "--replacement", "new",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_replace_missing_sheet_id(self, dws):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "replace",
            "--node", "SOME_NODE",
            "--find", "test",
            "--replacement", "new",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"
