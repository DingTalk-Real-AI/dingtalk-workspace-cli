"""drive 高频错误参数回归用例。"""


class TestDriveParamRegression:
    def test_upload_info_wrong_size_flag(self, dws):
        result = dws.run_raw(
            "drive", "upload-info",
            "--file-name", "demo.txt",
            "--size", "50000",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_tree_list_wrong_path_flag(self, dws):
        result = dws.run_raw("drive", "tree", "list", "--path", "/")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_list_should_not_hit_parsefloat_panic(self, dws):
        result = dws.run_raw("drive", "list")
        combined = (result.stdout + result.stderr).lower()
        assert "strconv.parsefloat" not in combined

    # ── list-spaces 参数回归 ──

    def test_list_spaces_basic(self, dws):
        """list-spaces 无参数应正常执行（不 panic）。"""
        result = dws.run_raw("drive", "list-spaces")
        combined = (result.stdout + result.stderr).lower()
        assert "panic" not in combined

    def test_list_spaces_with_space_type_myspace(self, dws):
        """list-spaces --space-type mySpace 应正常执行。"""
        result = dws.run_raw("drive", "list-spaces", "--space-type", "mySpace")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_list_spaces_with_limit(self, dws):
        """list-spaces --limit 应被正确识别。"""
        result = dws.run_raw("drive", "list-spaces", "--limit", "10")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_list_spaces_with_max(self, dws):
        """list-spaces --max 应被正确识别（与 --limit 同义）。"""
        result = dws.run_raw("drive", "list-spaces", "--max", "10")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_list_spaces_with_next_token(self, dws):
        """list-spaces --next-token (alias of --cursor) 应被正确识别。"""
        result = dws.run_raw("drive", "list-spaces", "--next-token", "abc123")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined


class TestDriveCrossProductAlias:
    """跨产品 alias 兼容性：drive 命令应接受 doc/wiki 常用的参数名。"""

    # ── --file-id 是 --node 的跨产品别名（向后兼容） ──

    def test_info_accepts_node_alias(self, dws):
        """drive info --node 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("drive", "info", "--node", "FAKE_DENTRY_UUID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_download_accepts_node_alias(self, dws):
        """drive download --node 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("drive", "download", "--node", "FAKE_DENTRY_UUID", "--output", "/tmp/")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_delete_accepts_node_alias(self, dws):
        """drive delete --node 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("drive", "delete", "--node", "FAKE_DENTRY_UUID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    # ── --parent-id 是 --folder 的跨产品别名（向后兼容） ──

    def test_list_accepts_folder_alias(self, dws):
        """drive list --folder 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("drive", "list", "--folder", "FAKE_PARENT_UUID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_upload_accepts_folder_alias(self, dws):
        """drive upload --folder 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("drive", "upload", "--file", "/tmp/nonexist.txt", "--folder", "FAKE_PARENT_UUID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_mkdir_accepts_folder_alias(self, dws):
        """drive mkdir --folder 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("drive", "mkdir", "--name", "test-dir", "--folder", "FAKE_PARENT_UUID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    # ── --name alias 已从 upload 移除（不在 crossProductAliases 中） ──

    def test_upload_rejects_name_flag(self, dws):
        """drive upload --name 已移除（不再是 --file-name 的别名），应被 cobra 拒绝。"""
        result = dws.run_raw("drive", "upload", "--file", "/tmp/nonexist.txt", "--name", "report.pdf")
        assert result.returncode != 0 or "unknown flag" in (result.stdout + result.stderr).lower()
