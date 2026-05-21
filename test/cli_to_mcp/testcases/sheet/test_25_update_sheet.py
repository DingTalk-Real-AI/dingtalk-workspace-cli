"""
test_25_update_sheet.py — 更新工作表属性测试

依赖 conftest.py 自建的测试表格。
为避免影响其他测试的种子数据，正向测试在新建工作表上操作。

Commands tested:
  1. dws sheet update --title "新名称"
  2. dws sheet update --index 0
  3. dws sheet update --hidden / --hidden=false
  4. dws sheet update --frozen-row-count N --frozen-column-count M
  5. dws sheet update --title "汇总" --index 0 (多属性同时更新)
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


class TestUpdateSheetTitle:
    """dws sheet update --title"""

    def test_rename_sheet(self, dws, sheet_node_id):
        """重命名工作表，验证返回的 name 字段。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "USTitle")
        new_name = unique_name("Renamed")
        data = dws.run(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--title", new_name,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        returned_name = data.get("name") or ""
        assert new_name in returned_name, f"name 应包含 {new_name}: {data}"

    def test_rename_sheet_chinese(self, dws, sheet_node_id):
        """使用中文名称重命名工作表。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "USChinese")
        new_name = unique_name("数据汇总")
        data = dws.run(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--title", new_name,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"


class TestUpdateSheetTitleSpecialChars:
    """dws sheet update --title 特殊字符边界"""

    def test_title_with_slash(self, dws, sheet_node_id):
        """title 包含 / 应报错（服务端校验）。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "USTSlash")
        result = dws.run_raw(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--title", "bad/name",
        )
        combined = result.stdout + result.stderr
        assert (
            result.returncode != 0
            or "error" in combined.lower()
            or "fail" in combined.lower()
        ), f"title 含 / 应报错: {combined[:300]}"

    def test_title_with_backslash(self, dws, sheet_node_id):
        """title 包含 \\ 应报错（服务端校验）。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "USTBSlash")
        result = dws.run_raw(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--title", "bad\\name",
        )
        combined = result.stdout + result.stderr
        assert (
            result.returncode != 0
            or "error" in combined.lower()
            or "fail" in combined.lower()
        ), f"title 含 \\\\ 应报错: {combined[:300]}"

    def test_title_with_brackets(self, dws, sheet_node_id):
        """title 包含 [ ] 应报错（服务端校验）。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "USTBracket")
        result = dws.run_raw(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--title", "bad[name]",
        )
        combined = result.stdout + result.stderr
        assert (
            result.returncode != 0
            or "error" in combined.lower()
            or "fail" in combined.lower()
        ), f"title 含 [] 应报错: {combined[:300]}"


class TestUpdateSheetIndex:
    """dws sheet update --index"""

    def test_move_to_first(self, dws, sheet_node_id):
        """移动工作表到第一个位置。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "USIdx")
        data = dws.run(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--index", "0",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_move_to_middle(self, dws, sheet_node_id):
        """移动工作表到中间位置（index=1）。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "USIdxMid")
        data = dws.run(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--index", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"


class TestUpdateSheetHidden:
    """dws sheet update --hidden"""

    def test_hide_sheet(self, dws, sheet_node_id):
        """隐藏工作表，验证 visibility 字段。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "USHide")
        data = dws.run(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--hidden",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        visibility = data.get("visibility") or ""
        assert visibility == "hidden", f"visibility 应为 hidden: {data}"

    def test_show_sheet(self, dws, sheet_node_id):
        """先隐藏再显示工作表，验证 hidden=false 生效。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "USShow")
        # 先隐藏
        dws.run(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--hidden",
        )
        # 再显示
        data = dws.run(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--hidden=false",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        visibility = data.get("visibility") or ""
        assert visibility == "visible", f"visibility 应为 visible: {data}"


class TestUpdateSheetFrozen:
    """dws sheet update --frozen-row-count / --frozen-column-count"""

    def test_freeze_rows(self, dws, sheet_node_id):
        """冻结前 2 行，验证 frozenRowCount 字段。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "USFrzR")
        data = dws.run(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--frozen-row-count", "2",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("frozenRowCount") == 2, f"frozenRowCount 应为 2: {data}"

    def test_freeze_columns(self, dws, sheet_node_id):
        """冻结前 1 列，验证 frozenColumnCount 字段。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "USFrzC")
        data = dws.run(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--frozen-column-count", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("frozenColumnCount") == 1, f"frozenColumnCount 应为 1: {data}"

    def test_freeze_rows_and_columns(self, dws, sheet_node_id):
        """同时冻结行和列。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "USFrzRC")
        data = dws.run(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--frozen-row-count", "2",
            "--frozen-column-count", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("frozenRowCount") == 2, f"frozenRowCount 应为 2: {data}"
        assert data.get("frozenColumnCount") == 1, f"frozenColumnCount 应为 1: {data}"

    def test_unfreeze(self, dws, sheet_node_id):
        """先冻结再取消冻结，验证冻结行列数归零。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "USUnfrz")
        # 先冻结
        dws.run(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--frozen-row-count", "2",
            "--frozen-column-count", "1",
        )
        # 取消冻结
        data = dws.run(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--frozen-row-count", "0",
            "--frozen-column-count", "0",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        assert data.get("frozenRowCount") == 0, f"frozenRowCount 应为 0: {data}"
        assert data.get("frozenColumnCount") == 0, f"frozenColumnCount 应为 0: {data}"


class TestUpdateSheetByName:
    """dws sheet update — 通过工作表名称操作"""

    def test_update_by_sheet_name(self, dws, sheet_node_id):
        """通过工作表名称（而非 ID）更新属性。"""
        _, sheet_name = _create_sheet_with_data(dws, sheet_node_id, "USByName")
        new_name = unique_name("ByNameRenamed")
        data = dws.run(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_name,
            "--title", new_name,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"


class TestUpdateSheetMultipleProps:
    """dws sheet update — 同时更新多个属性"""

    def test_title_and_index(self, dws, sheet_node_id):
        """同时修改名称和位置。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "USMulti")
        new_name = unique_name("MultiUpdate")
        data = dws.run(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--title", new_name,
            "--index", "0",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        returned_name = data.get("name") or ""
        assert new_name in returned_name, f"name 应包含 {new_name}: {data}"

    def test_title_hidden_and_frozen(self, dws, sheet_node_id):
        """同时修改名称、隐藏和冻结（三个属性同时更新）。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "USTriple")
        new_name = unique_name("TripleUpdate")
        data = dws.run(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--title", new_name,
            "--hidden",
            "--frozen-row-count", "1",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"


class TestUpdateSheetErrors:
    """dws sheet update — 参数校验错误场景"""

    def test_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw("sheet", "update", "--sheet-id", "any", "--title", "test")
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_missing_sheet_id(self, dws, sheet_node_id):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw("sheet", "update", "--node", sheet_node_id, "--title", "test")
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"

    def test_no_update_property(self, dws, sheet_node_id, sheet_id):
        """不提供任何可选更新属性应报错。"""
        result = dws.run_raw(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"不提供更新属性应报错: {result.stdout[:200]}"

    def test_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "update",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "any",
            "--title", "test",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    def test_invalid_sheet_id(self, dws, sheet_node_id):
        """无效 sheetId 应报错。"""
        result = dws.run_raw(
            "sheet", "update",
            "--node", sheet_node_id,
            "--sheet-id", "INVALID_SHEET_99999",
            "--title", "test",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 sheetId 应报错: {result.stdout[:200]}"
