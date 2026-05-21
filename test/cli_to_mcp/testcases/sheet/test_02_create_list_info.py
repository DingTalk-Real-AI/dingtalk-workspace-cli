"""
test_02_create_list_info.py — 表格文档创建、工作表列表、工作表详情测试

Commands tested:
  1. dws sheet create --name NAME
  2. dws sheet list --node NODE_ID
  3. dws sheet info --node NODE_ID [--sheet-id SHEET_ID]

create 测试独立创建/验证/清理，不依赖 conftest 的 test_sheet_info fixture。
list / info 测试复用 conftest 自建的表格。
"""

from test_utils import unique_name


class TestSheetCreate:
    """dws sheet create"""

    def test_create_basic(self, dws):
        """创建表格文档，校验返回 nodeId。"""
        name = unique_name("CLI_Test_Sheet_Create")
        data = dws.run("sheet", "create", "--name", name)
        node_id = data.get("nodeId") or data.get("data", {}).get("nodeId")
        assert node_id, f"create 应返回 nodeId: {data}"
        assert isinstance(node_id, str), f"nodeId 应为字符串: {node_id}"

    def test_create_chinese_name(self, dws):
        """创建中文名称的表格文档。"""
        name = unique_name("测试表格")
        data = dws.run("sheet", "create", "--name", name)
        node_id = data.get("nodeId") or data.get("data", {}).get("nodeId")
        assert node_id, f"中文名称 create 应返回 nodeId: {data}"

    def test_create_missing_name(self, dws):
        """缺少必填 --name 参数应报错。"""
        result = dws.run_raw("sheet", "create")
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --name 应报错: {result.stdout[:200]}"


class TestSheetList:
    """dws sheet list"""

    def test_list_returns_sheets(self, dws, sheet_node_id):
        """列出工作表，校验返回 sheets 数组。"""
        data = dws.run("sheet", "list", "--node", sheet_node_id)
        sheets = data.get("sheets") or data.get("data", {}).get("sheets") or []
        assert isinstance(sheets, list), f"sheets 应为 list: {data}"
        assert len(sheets) >= 1, f"至少应有 1 个工作表: {data}"

    def test_list_sheet_has_id_and_name(self, dws, sheet_node_id):
        """工作表条目应包含 ID 和名称字段。"""
        data = dws.run("sheet", "list", "--node", sheet_node_id)
        sheets = data.get("sheets") or data.get("data", {}).get("sheets") or []
        assert len(sheets) >= 1, f"至少应有 1 个工作表: {data}"
        first = sheets[0]
        has_id = first.get("sheetId") or first.get("id")
        has_name = first.get("name") or first.get("title")
        assert has_id, f"工作表条目缺少 ID 字段: {first}"
        assert has_name, f"工作表条目缺少名称字段: {first}"

    def test_list_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw("sheet", "list", "--node", "INVALID_NODE_99999")
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    def test_list_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw("sheet", "list")
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"


class TestSheetInfo:
    """dws sheet info"""

    def test_info_default_sheet(self, dws, sheet_node_id):
        """不传 --sheet-id 时返回第一个工作表详情。"""
        data = dws.run("sheet", "info", "--node", sheet_node_id)
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_info_with_sheet_id(self, dws, sheet_node_id, sheet_id):
        """指定 --sheet-id 返回对应工作表详情。"""
        data = dws.run(
            "sheet", "info",
            "--node", sheet_node_id,
            "--sheet-id", sheet_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_info_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw("sheet", "info", "--node", "INVALID_NODE_99999")
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"

    def test_info_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw("sheet", "info")
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"
