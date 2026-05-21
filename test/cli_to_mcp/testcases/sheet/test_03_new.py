"""
test_03_new.py — 新建工作表测试

Commands tested:
  1. dws sheet new --node NODE_ID --name NAME
"""

from test_utils import unique_name


class TestSheetNew:
    """dws sheet new"""

    def test_new_basic(self, dws, sheet_node_id):
        """新建工作表，校验返回成功。"""
        name = unique_name("TestSheet")
        data = dws.run(
            "sheet", "new",
            "--node", sheet_node_id,
            "--name", name,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_new_chinese_name(self, dws, sheet_node_id):
        """新建中文名称的工作表。"""
        name = unique_name("测试工作表")
        data = dws.run(
            "sheet", "new",
            "--node", sheet_node_id,
            "--name", name,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_new_verify_in_list(self, dws, sheet_node_id):
        """新建工作表后，list 中应能看到新工作表。"""
        name = unique_name("VerifyNew")
        data = dws.run(
            "sheet", "new",
            "--node", sheet_node_id,
            "--name", name,
        )
        assert data.get("success") is True, f"新建工作表应成功: {data}"
        # 验证 list 中包含新建的工作表
        list_data = dws.run("sheet", "list", "--node", sheet_node_id)
        sheets = list_data.get("sheets") or list_data.get("data", {}).get("sheets") or []
        sheet_names = [
            s.get("name") or s.get("title") or ""
            for s in sheets
        ]
        # 系统可能自动重命名，检查是否有包含 VerifyNew 前缀的工作表
        assert any("VerifyNew" in n for n in sheet_names), (
            f"新建的工作表应出现在 list 中: names={sheet_names}"
        )

    def test_new_missing_name(self, dws, sheet_node_id):
        """缺少必填 --name 参数应报错。"""
        result = dws.run_raw(
            "sheet", "new",
            "--node", sheet_node_id,
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --name 应报错: {result.stdout[:200]}"

    def test_new_missing_node(self, dws):
        """缺少必填 --node 参数应报错。"""
        result = dws.run_raw(
            "sheet", "new",
            "--name", "ShouldFail",
        )
        assert (
            result.returncode != 0
            or "error" in (result.stdout + result.stderr).lower()
        ), f"缺少 --node 应报错: {result.stdout[:200]}"

    def test_new_invalid_node(self, dws):
        """无效 nodeId 应报错。"""
        result = dws.run_raw(
            "sheet", "new",
            "--node", "INVALID_NODE_99999",
            "--name", "ShouldFail",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 nodeId 应报错: {result.stdout[:200]}"
