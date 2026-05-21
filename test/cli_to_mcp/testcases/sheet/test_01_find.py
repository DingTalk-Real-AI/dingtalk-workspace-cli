"""
test_01_find.py — 表格查找测试

依赖 conftest.py 自建的测试表格，种子数据：
  A1:D1  姓名 | 部门 | 金额 | 状态
  A2:D5  张三/李四/王五/Test_User 四行数据
  C7     =SUM(C2:C5) 公式

Commands tested:
  1. dws sheet find --node NODE_ID --sheet-id SHEET_ID --query TEXT
  2. dws sheet find ... --find TEXT           (--query 的隐藏别名)
  3. dws sheet find ... --range RANGE
  4. dws sheet find ... --match-case=false
  5. dws sheet find ... --match-entire-cell
  6. dws sheet find ... --use-regexp
  7. dws sheet find ... --match-formula
  8. dws sheet find ... --include-hidden
"""


class TestSheetFindBasic:
    """dws sheet find — 基本搜索功能"""

    def test_find_basic(self, dws, sheet_node_id, sheet_id):
        """搜索"销售部"，应命中张三和王五两行。"""
        data = dws.run(
            "sheet", "find",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--find", "销售部",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert "matchedCells" in data, f"响应缺少 matchedCells: {data}"
        assert isinstance(data["matchedCells"], list), f"matchedCells 应为 list: {data}"
        assert "totalCount" in data, f"响应缺少 totalCount: {data}"
        assert data["totalCount"] >= 2, f"销售部应至少命中 2 个单元格: {data}"

    def test_find_with_range(self, dws, sheet_node_id, sheet_id):
        """带 --range 参数限定搜索范围，在 A1:B3 内搜索"张三"。"""
        data = dws.run(
            "sheet", "find",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--find", "张三",
            "--range", "A1:B3",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert "matchedCells" in data, f"响应缺少 matchedCells: {data}"
        assert data.get("totalCount", 0) >= 1, f"张三应在 A1:B3 范围内命中: {data}"

    def test_find_no_match(self, dws, sheet_node_id, sheet_id):
        """搜索不存在的文本，应返回空结果。"""
        data = dws.run(
            "sheet", "find",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--find", "__NO_MATCH_99999__",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("totalCount") == 0, f"totalCount 应为 0: {data}"

    def test_find_with_query_alias(self, dws, sheet_node_id, sheet_id):
        """使用 --query 主参数搜索（与 --find 等价），应命中“销售部”。"""
        data = dws.run(
            "sheet", "find",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--query", "销售部",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert "matchedCells" in data, f"响应缺少 matchedCells: {data}"
        assert isinstance(data["matchedCells"], list), f"matchedCells 应为 list: {data}"
        assert data.get("totalCount", 0) >= 2, (
            f"--query 别名搜索销售部应至少命中 2 个单元格: {data}"
        )

    def test_find_query_alias_with_options(self, dws, sheet_node_id, sheet_id):
        """--query 别名可与其他可选参数组合使用，验证正则 + 范围。"""
        data = dws.run(
            "sheet", "find",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--query", "^[张王]",
            "--use-regexp",
            "--range", "A1:D10",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert "matchedCells" in data, f"响应缺少 matchedCells: {data}"
        assert data.get("totalCount", 0) >= 2, (
            f"--query 别名 + 正则应命中张三和王五: {data}"
        )


class TestSheetFindOptions:
    """dws sheet find — 可选参数搜索"""

    def test_find_ignore_case(self, dws, sheet_node_id, sheet_id):
        """--match-case=false 忽略大小写搜索 test_user。"""
        data = dws.run(
            "sheet", "find",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--find", "test_user",
            "--match-case=false",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert "matchedCells" in data, f"响应缺少 matchedCells: {data}"
        assert data.get("totalCount", 0) >= 1, (
            f"忽略大小写搜索 test_user 应命中 Test_User: {data}"
        )

    def test_find_match_entire_cell(self, dws, sheet_node_id, sheet_id):
        """--match-entire-cell 精确匹配"完成"。"""
        data = dws.run(
            "sheet", "find",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--find", "完成",
            "--match-entire-cell",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert "matchedCells" in data, f"响应缺少 matchedCells: {data}"
        assert data.get("totalCount", 0) >= 2, (
            f"精确匹配'完成'应命中至少 2 个单元格: {data}"
        )

    def test_find_with_regexp(self, dws, sheet_node_id, sheet_id):
        """--use-regexp 正则搜索以"张"或"王"开头的姓名。"""
        data = dws.run(
            "sheet", "find",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--find", "^[张王]",
            "--use-regexp",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert "matchedCells" in data, f"响应缺少 matchedCells: {data}"
        assert isinstance(data.get("totalCount"), (int, float)), (
            f"totalCount 应为数字: {data}"
        )
        assert data["totalCount"] >= 2, f"正则应命中张三和王五: {data}"

    def test_find_match_formula(self, dws, sheet_node_id, sheet_id):
        """--match-formula 搜索公式文本 SUM。"""
        data = dws.run(
            "sheet", "find",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--find", "SUM",
            "--match-formula",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert "matchedCells" in data, f"响应缺少 matchedCells: {data}"
        assert data.get("totalCount", 0) >= 1, (
            f"搜索公式 SUM 应至少命中 C7: {data}"
        )

    def test_find_include_hidden(self, dws, sheet_node_id, sheet_id):
        """--include-hidden 包含隐藏单元格搜索"待处理"。"""
        data = dws.run(
            "sheet", "find",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
            "--find", "待处理",
            "--include-hidden",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert "matchedCells" in data, f"响应缺少 matchedCells: {data}"
        assert data.get("totalCount", 0) >= 1, (
            f"搜索'待处理'应至少命中 1 个单元格: {data}"
        )


class TestSheetFindError:
    """dws sheet find — 错误路径（不依赖 fixture 创建的表格）"""

    def test_find_invalid_node(self, dws):
        """无效 node ID 应报错。"""
        result = dws.run_raw(
            "sheet", "find",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "Sheet1",
            "--find", "test",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: stdout={result.stdout[:200]}, stderr={result.stderr[:200]}"

    def test_find_missing_find_flag(self, dws):
        """同时缺少 --query 和 --find 参数应报错。"""
        result = dws.run_raw(
            "sheet", "find",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --query/--find 应报错: stdout={result.stdout[:200]}, stderr={result.stderr[:200]}"

    def test_find_missing_node_flag(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "find",
            "--sheet-id", "Sheet1",
            "--find", "test",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: stdout={result.stdout[:200]}, stderr={result.stderr[:200]}"

    def test_find_missing_sheet_id_flag(self, dws):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw(
            "sheet", "find",
            "--node", "SOME_NODE",
            "--find", "test",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: stdout={result.stdout[:200]}, stderr={result.stderr[:200]}"
