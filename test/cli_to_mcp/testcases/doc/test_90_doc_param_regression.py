"""doc 高频错误参数回归用例。

注: --keyword 是 --query 的隐藏别名, --title 是 --name 的隐藏别名,
    均为合法参数, 此处验证其可正常工作且 CLI 不崩溃。
"""


def _assert_regression_ok(result):
    """回归用例统一判定: CLI 不崩溃即可。"""
    combined = ((result.stdout or "") + "\n" + (result.stderr or "")).strip()
    assert len(combined) > 0, "命令未产生任何输出"

class TestDocParamRegression:
    def test_search_keyword_alias(self, dws):
        """--keyword 是 --query 的别名, 应正常返回结果。"""
        result = dws.run_raw("doc", "search", "--keyword", "测试")
        _assert_regression_ok(result)

    def test_create_title_alias(self, dws):
        """--title 是 --name 的别名, 应正常创建文档。"""
        result = dws.run_raw("doc", "create", "--title", "回归测试别名")
        _assert_regression_ok(result)

    def test_read_wrong_url_flag(self, dws):
        """dws doc read 不支持 --url (正确为 --node)。"""
        result = dws.run_raw("doc", "read", "--url", "https://invalid.example.com")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_read_wrong_id_flag(self, dws):
        """dws doc read 不支持 --id (正确为 --node)。"""
        result = dws.run_raw("doc", "read", "--id", "INVALID")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()


    # ── copy/move/rename 回归用例 ──

    def test_copy_missing_node(self, dws):
        """copy 缺少必填参数 --node 应报错"""
        result = dws.run_raw("doc", "copy", "--folder", "FAKE_FOLDER")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_move_missing_node(self, dws):
        """move 缺少必填参数 --node 应报错"""
        result = dws.run_raw("doc", "move", "--folder", "FAKE_FOLDER")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_rename_missing_name(self, dws):
        """rename 缺少必填参数 --name 应报错"""
        result = dws.run_raw("doc", "rename", "--node", "FAKE_NODE")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_rename_missing_node(self, dws):
        """rename 缺少必填参数 --node 应报错"""
        result = dws.run_raw("doc", "rename", "--name", "新名称")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_copy_with_workspace(self, dws):
        """copy --workspace 参数应被正常接受（不崩溃）"""
        result = dws.run_raw("doc", "copy", "--node", "FAKE_NODE", "--workspace", "FAKE_WS")
        _assert_regression_ok(result)

    def test_move_with_workspace(self, dws):
        """move --workspace 参数应被正常接受（不崩溃）"""
        result = dws.run_raw("doc", "move", "--node", "FAKE_NODE", "--workspace", "FAKE_WS")
        _assert_regression_ok(result)

    def test_copy_with_folder_and_workspace(self, dws):
        """copy 同时传 --folder 和 --workspace 应被正常接受"""
        result = dws.run_raw("doc", "copy", "--node", "FAKE_NODE", "--folder", "FAKE_FOLDER", "--workspace", "FAKE_WS")
        _assert_regression_ok(result)

    def test_move_with_folder_and_workspace(self, dws):
        """move 同时传 --folder 和 --workspace 应被正常接受"""
        result = dws.run_raw("doc", "move", "--node", "FAKE_NODE", "--folder", "FAKE_FOLDER", "--workspace", "FAKE_WS")
        _assert_regression_ok(result)

    def test_rename_title_alias(self, dws):
        """--title 是 --name 的隐藏别名，rename 应正常接受"""
        result = dws.run_raw("doc", "rename", "--node", "FAKE_NODE", "--title", "新标题")
        _assert_regression_ok(result)

    def test_copy_node_url_alias(self, dws):
        """copy --url 是 --node 的隐藏别名，应正常接受"""
        result = dws.run_raw("doc", "copy", "--url", "https://alidocs.dingtalk.com/i/nodes/FAKE", "--folder", "FAKE_FOLDER")
        _assert_regression_ok(result)

    def test_move_node_id_alias(self, dws):
        """move --id 是 --node 的隐藏别名，应正常接受"""
        result = dws.run_raw("doc", "move", "--id", "FAKE_NODE", "--folder", "FAKE_FOLDER")
        _assert_regression_ok(result)

class TestDocPermissionParamRegression:
    """dws doc permission {add|update|list} 参数回归。"""

    # ── 子命令缺失 ──

    def test_permission_no_subcommand(self, dws):
        """`dws doc permission` 不带子命令应输出帮助/报错。"""
        result = dws.run_raw("doc", "permission")
        # cobra 默认行为：未指定子命令时打印 help 并退出码 0；
        # 此处只断言有输出，验证子命令组已注册。
        _assert_regression_ok(result)

    def test_permission_unknown_subcommand(self, dws):
        """未知子命令 `permission delete` 应被 cobra 拒绝。"""
        result = dws.run_raw("doc", "permission", "delete", "--node", "FAKE")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── add 参数回归 ──

    def test_add_invalid_role(self, dws):
        """--role 取值非法（如 unknown-role）应被业务层拒绝。"""
        result = dws.run_raw(
            "doc", "permission", "add",
            "--node", "FAKE_NODE",
            "--user", "FAKE_USER",
            "--role", "unknown-role",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_add_org_flag_not_exposed(self, dws):
        """--org 已从 CLI 移除（避免误授权企业全员），应被 cobra 拒绝。"""
        result = dws.run_raw(
            "doc", "permission", "add",
            "--node", "FAKE_NODE",
            "--user", "FAKE_USER",
            "--role", "READER",
            "--org",
        )
        assert result.returncode != 0 or "unknown flag" in (result.stdout + result.stderr).lower()

    def test_add_wrong_users_flag(self, dws):
        """错误参数名 --users（应为 --user）应被 cobra 拒绝。"""
        result = dws.run_raw(
            "doc", "permission", "add",
            "--node", "FAKE_NODE",
            "--users", "FAKE_USER",
            "--role", "reader",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_add_sticky_node_flag(self, dws):
        """粘连参数 `--nodeXXX` 应被 cobra 拒绝。"""
        result = dws.run_raw(
            "doc", "permission", "add",
            "--nodeFAKE",
            "--user", "FAKE_USER",
            "--role", "reader",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── update 参数回归 ──

    def test_update_no_required_flags(self, dws):
        """update 不传任何参数应报错。"""
        result = dws.run_raw("doc", "permission", "update")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_update_wrong_uid_flag(self, dws):
        """错误参数名 --uid（应为 --user）应被 cobra 拒绝。"""
        result = dws.run_raw(
            "doc", "permission", "update",
            "--node", "FAKE_NODE",
            "--uid", "FAKE_USER",
            "--role", "reader",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── list 参数回归 ──

    def test_list_no_required_flags(self, dws):
        """list 不传 --node 应报错。"""
        result = dws.run_raw("doc", "permission", "list")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_list_wrong_maxresults_flag(self, dws):
        """错误参数名 --maxresults（应为 --max-results）应被 cobra 拒绝。"""
        result = dws.run_raw(
            "doc", "permission", "list",
            "--node", "FAKE_NODE",
            "--maxresults", "10",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_list_old_pagesize_flag_removed(self, dws):
        """旧的 --page-size 参数已被删除（迁移到 --max-results），应被 cobra 拒绝。"""
        result = dws.run_raw(
            "doc", "permission", "list",
            "--node", "FAKE_NODE",
            "--page-size", "10",
        )
        assert result.returncode != 0 or "unknown flag" in (result.stdout + result.stderr).lower()

    def test_list_with_filter_role(self, dws):
        """--filter-role 是合法参数，应被正常接受（不崩溃）。"""
        result = dws.run_raw(
            "doc", "permission", "list",
            "--node", "FAKE_NODE",
            "--filter-role", "MANAGER,EDITOR",
        )
        _assert_regression_ok(result)


class TestDocMediaInsertParamRegression:
    """dws doc media insert 参数回归。"""

    def test_insert_sticky_node_flag(self, dws):
        """--nodeXXX 粘连参数应被拒绝。"""
        result = dws.run_raw("doc", "media", "insert", "--nodeXXX")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_insert_sticky_file_flag(self, dws):
        """--file/path 粘连参数应被拒绝。"""
        result = dws.run_raw("doc", "media", "insert", "--file/path")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_insert_wrong_mime_flag(self, dws):
        """错误的参数名 --mimetype（应为 --mime-type）应被拒绝。"""
        result = dws.run_raw(
            "doc", "media", "insert",
            "--node", "FAKE", "--file", "/tmp", "--mimetype", "text/plain",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_insert_no_flags_at_all(self, dws):
        """不传任何参数应报错。"""
        result = dws.run_raw("doc", "media", "insert")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()


class TestDocUpdateModeRequired:
    """doc update --mode 必填校验。"""

    def test_update_without_mode_should_fail(self, dws):
        """doc update 不传 --mode 应报错。"""
        result = dws.run_raw(
            "doc", "update",
            "--node", "FAKE_NODE",
            "--content", "test content",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0, f"expected non-zero exit code, got 0. output: {combined}"
        assert "mode" in combined.lower(), f"expected error message to mention 'mode', got: {combined}"

    def test_update_with_mode_append_accepted(self, dws):
        """doc update --mode append 应被正常接受（不因缺少 mode 报错）。"""
        result = dws.run_raw(
            "doc", "update",
            "--node", "FAKE_NODE",
            "--content", "test content",
            "--mode", "append",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        # 不应因 --mode 缺失报错，可能因 FAKE_NODE 无效而报其他错误
        assert "required flag" not in combined.lower() or "mode" not in combined.lower()

    def test_update_with_mode_overwrite_accepted(self, dws):
        """doc update --mode overwrite 应被正常接受（不因缺少 mode 报错）。"""
        result = dws.run_raw(
            "doc", "update",
            "--node", "FAKE_NODE",
            "--content", "test content",
            "--mode", "overwrite",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "required flag" not in combined.lower() or "mode" not in combined.lower()


class TestDocCrossProductAlias:
    """跨产品 alias 兼容性：doc 命令应接受 drive 常用的参数名。"""

    # ── --file-id 是 --node 的跨产品别名 ──

    def test_info_accepts_file_id_alias(self, dws):
        """doc info --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("doc", "info", "--file-id", "FAKE_NODE_ID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_read_accepts_file_id_alias(self, dws):
        """doc read --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("doc", "read", "--file-id", "FAKE_NODE_ID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_download_accepts_file_id_alias(self, dws):
        """doc download --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("doc", "download", "--file-id", "FAKE_NODE_ID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_delete_accepts_file_id_alias(self, dws):
        """doc delete --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("doc", "delete", "--file-id", "FAKE_NODE_ID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    # ── --parent-id 是 --folder 的跨产品别名 ──

    def test_list_accepts_parent_id_alias(self, dws):
        """doc list --parent-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("doc", "list", "--parent-id", "FAKE_FOLDER_ID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_create_accepts_parent_id_alias(self, dws):
        """doc create --parent-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("doc", "create", "--name", "test", "--parent-id", "FAKE_FOLDER_ID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_copy_accepts_parent_id_alias(self, dws):
        """doc copy --parent-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("doc", "copy", "--node", "FAKE_NODE", "--parent-id", "FAKE_FOLDER_ID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_move_accepts_parent_id_alias(self, dws):
        """doc move --parent-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("doc", "move", "--node", "FAKE_NODE", "--parent-id", "FAKE_FOLDER_ID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    # ── --file-id alias 补充：media/comment/permission/export/update/block ──

    def test_update_accepts_file_id_alias(self, dws):
        """doc update --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("doc", "update", "--file-id", "FAKE_NODE_ID", "--content", "test", "--mode", "append")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_media_insert_accepts_file_id_alias(self, dws):
        """doc media insert --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("doc", "media", "insert", "--file-id", "FAKE_NODE_ID", "--file", "/tmp/nonexist.txt")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_comment_list_accepts_file_id_alias(self, dws):
        """doc comment list --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("doc", "comment", "list", "--file-id", "FAKE_NODE_ID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_permission_add_accepts_file_id_alias(self, dws):
        """doc permission add --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("doc", "permission", "add", "--file-id", "FAKE_NODE_ID", "--user", "FAKE_USER", "--role", "READER")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_export_accepts_file_id_alias(self, dws):
        """doc export --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("doc", "export", "--file-id", "FAKE_NODE_ID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_block_list_accepts_file_id_alias(self, dws):
        """doc block list --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("doc", "block", "list", "--file-id", "FAKE_NODE_ID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined
