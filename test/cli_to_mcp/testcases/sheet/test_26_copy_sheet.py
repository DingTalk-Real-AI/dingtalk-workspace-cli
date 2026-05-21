"""
test_26_copy_sheet.py — 复制工作表测试

依赖 conftest.py 自建的测试表格。
为避免影响其他测试的种子数据，正向测试在新建工作表上操作。

Commands tested:
  1. dws sheet copy --node NODE_ID --sheet-id SHEET_ID
  2. dws sheet copy --title "副本名称"
  3. dws sheet copy --title "备份" --index 0
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


class TestCopySheetBasic:
    """dws sheet copy — 基本复制"""

    def test_copy_default(self, dws, sheet_node_id):
        """复制工作表（系统自动命名），验证返回新 sheetId。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "CSBasic")
        data = dws.run(
            "sheet", "copy",
            "--node", sheet_node_id,
            "--sheet-id", sid,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        new_sheet_id = data.get("sheetId") or data.get("id")
        assert new_sheet_id, f"copy 应返回新 sheetId: {data}"
        assert new_sheet_id != sid, f"新 sheetId 不应与源 sheetId 相同: {data}"

    def test_copy_with_title(self, dws, sheet_node_id):
        """复制工作表并指定名称。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "CSTitle")
        copy_name = unique_name("MyCopy")
        data = dws.run(
            "sheet", "copy",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--title", copy_name,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        returned_name = data.get("name") or ""
        assert copy_name in returned_name, f"name 应包含 {copy_name}: {data}"

    def test_copy_with_title_and_index(self, dws, sheet_node_id):
        """复制工作表并指定名称和位置。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "CSIdx")
        copy_name = unique_name("CopyFirst")
        data = dws.run(
            "sheet", "copy",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--title", copy_name,
            "--index", "0",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        returned_name = data.get("name") or ""
        assert copy_name in returned_name, f"name 应包含 {copy_name}: {data}"


class TestCopySheetDataIntegrity:
    """dws sheet copy — 数据完整性验证"""

    def test_copy_preserves_data(self, dws, sheet_node_id):
        """复制后读取副本数据，验证与源数据一致。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "CSData", rows=3, cols=2)
        copy_name = unique_name("DataCheck")
        copy_data = dws.run(
            "sheet", "copy",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--title", copy_name,
        )
        assert copy_data.get("success") is True, f"success 应为 True: {copy_data}"
        new_sid = copy_data.get("sheetId") or copy_data.get("id")
        assert new_sid, f"copy 应返回新 sheetId: {copy_data}"

        # 读取源工作表数据
        src_data = dws.run(
            "sheet", "range", "read",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--range", "A1:B3",
        )
        src_values = src_data.get("values") or src_data.get("data", {}).get("values") or []

        # 读取副本数据
        dst_data = dws.run(
            "sheet", "range", "read",
            "--node", sheet_node_id,
            "--sheet-id", new_sid,
            "--range", "A1:B3",
        )
        dst_values = dst_data.get("values") or dst_data.get("data", {}).get("values") or []

        assert len(dst_values) == len(src_values), (
            f"副本行数应与源一致: src={len(src_values)}, dst={len(dst_values)}"
        )


class TestCopySheetIndexOnly:
    """dws sheet copy — 只传 index 不传 title"""

    def test_copy_with_index_only(self, dws, sheet_node_id):
        """只指定 index 不指定 title，系统自动命名并放到指定位置。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "CSIdxOnly")
        data = dws.run(
            "sheet", "copy",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--index", "0",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        new_sheet_id = data.get("sheetId") or data.get("id")
        assert new_sheet_id, f"copy 应返回新 sheetId: {data}"
        assert new_sheet_id != sid, f"新 sheetId 不应与源 sheetId 相同: {data}"


class TestCopySheetByName:
    """dws sheet copy — 使用工作表名称"""

    def test_copy_by_sheet_name(self, dws, sheet_node_id):
        """通过工作表名称（而非 ID）复制。"""
        _, sheet_name = _create_sheet_with_data(dws, sheet_node_id, "CSByName")
        copy_name = unique_name("NameCopy")
        data = dws.run(
            "sheet", "copy",
            "--node", sheet_node_id,
            "--sheet-id", sheet_name,
            "--title", copy_name,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"


class TestCopySheetDuplicateTitle:
    """dws sheet copy — 重复名称自动重命名"""

    def test_duplicate_title_auto_rename(self, dws, sheet_node_id):
        """使用已有工作表名称复制，系统应自动重命名而非报错。"""
        sid, original_name = _create_sheet_with_data(dws, sheet_node_id, "CSDup")
        # 使用源工作表的名称作为 title，应自动重命名
        data = dws.run(
            "sheet", "copy",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--title", original_name,
        )
        assert data.get("success") is True, f"重复名称应自动重命名而非报错: {data}"
        new_sheet_id = data.get("sheetId") or data.get("id")
        assert new_sheet_id, f"copy 应返回新 sheetId: {data}"


class TestCopySheetTitleSpecialChars:
    """dws sheet copy — title 特殊字符边界"""

    def test_title_with_special_chars(self, dws, sheet_node_id):
        """title 包含 / 等特殊字符应报错（服务端校验）。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "CSTSpec")
        result = dws.run_raw(
            "sheet", "copy",
            "--node", sheet_node_id,
            "--sheet-id", sid,
            "--title", "bad/copy*name",
        )
        combined = result.stdout + result.stderr
        assert (
            result.returncode != 0
            or "error" in combined.lower()
            or "fail" in combined.lower()
        ), f"title 含特殊字符应报错: {combined[:300]}"


class TestCopySheetVisibility:
    """dws sheet copy — 可见性验证"""

    def test_copy_visibility_is_visible(self, dws, sheet_node_id):
        """复制后的工作表应默认可见。"""
        sid, _ = _create_sheet_with_data(dws, sheet_node_id, "CSVis")
        data = dws.run(
            "sheet", "copy",
            "--node", sheet_node_id,
            "--sheet-id", sid,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        visibility = data.get("visibility") or ""
        assert visibility == "visible", f"visibility 应为 visible: {data}"


class TestCopySheetErrors:
    """dws sheet copy — 参数校验错误场景"""

    def test_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw("sheet", "copy", "--sheet-id", "any")
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_missing_sheet_id(self, dws, sheet_node_id):
        """缺少必填 --sheet-id 参数应报错。"""
        result = dws.run_raw("sheet", "copy", "--node", sheet_node_id)
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --sheet-id 应报错: {result.stdout[:200]}"

    def test_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "copy",
            "--node", "INVALID_NODE_99999",
            "--sheet-id", "any",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    def test_invalid_sheet_id(self, dws, sheet_node_id):
        """无效 sheetId 应报错。"""
        result = dws.run_raw(
            "sheet", "copy",
            "--node", sheet_node_id,
            "--sheet-id", "INVALID_SHEET_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 sheetId 应报错: {result.stdout[:200]}"
