"""sheet 高频错误参数回归用例。

覆盖命令：create / list / info / new / range read / range update / find /
          merge-cells / insert-dimension / delete-dimension / update-dimension / media-upload / write-image /
          replace / move-dimension / add-dimension / unmerge-cells /
          create-float-image / get-float-image / list-float-images / update-float-image / delete-float-image
"""


class TestSheetParamRegression:
    """参数粘连、错误参数名、必填参数缺失等回归测试。"""

    # ── create ──────────────────────────────────────────────

    def test_create_wrong_title_flag(self, dws):
        """--title 不是合法 flag（应为 --name）。"""
        result = dws.run_raw("sheet", "create", "--title", "test")
        assert result.returncode != 0 and "error" in (result.stdout + result.stderr).lower()

    # ── list ────────────────────────────────────────────────

    def test_list_sticky_node_flag(self, dws):
        """--nodeNODE_ID 粘连参数应被拒绝。"""
        result = dws.run_raw("sheet", "list", "--nodeNODE_ID")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_list_wrong_id_flag(self, dws):
        """--id 不是合法 flag（应为 --node）。"""
        result = dws.run_raw("sheet", "list", "--id", "SOME_NODE")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── info ────────────────────────────────────────────────

    def test_info_wrong_sheet_name_flag(self, dws):
        """--sheet-name 不是合法 flag（应为 --sheet-id）。"""
        result = dws.run_raw(
            "sheet", "info",
            "--node", "SOME_NODE",
            "--sheet-name", "Sheet1",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── new ─────────────────────────────────────────────────

    def test_new_sticky_name_flag(self, dws):
        """--name新工作表 粘连参数应被拒绝。"""
        result = dws.run_raw(
            "sheet", "new",
            "--node", "SOME_NODE",
            "--name新工作表",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_new_wrong_title_flag(self, dws):
        """--title 不是合法 flag（应为 --name）。"""
        result = dws.run_raw(
            "sheet", "new",
            "--node", "SOME_NODE",
            "--title", "Sheet2",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_new_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "new")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── range read ──────────────────────────────────────────

    def test_range_read_wrong_area_flag(self, dws):
        """--area 不是合法 flag（应为 --range）。"""
        result = dws.run_raw(
            "sheet", "range", "read",
            "--node", "SOME_NODE",
            "--area", "A1:D10",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── range update ────────────────────────────────────────

    def test_range_update_wrong_data_flag(self, dws):
        """--data 不是合法 flag（应为 --values）。"""
        result = dws.run_raw(
            "sheet", "range", "update",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--range", "A1",
            "--data", '[["test"]]',
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_range_update_wrong_links_flag(self, dws):
        """--links 不是合法 flag（应为 --hyperlinks）。"""
        result = dws.run_raw(
            "sheet", "range", "update",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--range", "A1",
            "--links", '[[]]',
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_range_update_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "range", "update")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── find ────────────────────────────────────────────────

    def test_find_sticky_find_flag(self, dws):
        """--find测试 粘连参数应被拒绝。"""
        result = dws.run_raw(
            "sheet", "find",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--find测试",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_find_sticky_node_flag(self, dws):
        """--nodeNODE_ID 粘连参数应被拒绝。"""
        result = dws.run_raw(
            "sheet", "find",
            "--nodeNODE_ID",
            "--sheet-id", "Sheet1",
            "--find", "test",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_find_wrong_text_flag(self, dws):
        """--text 不是合法 flag（应为 --find）。"""
        result = dws.run_raw(
            "sheet", "find",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--text", "test",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_find_keyword_alias_accepted(self, dws):
        """--keyword 是 --query 的 hidden alias，应被接受（不报 unknown flag）。
        注意：sheet find 的搜索参数 primary 是 --query，--keyword 通过 RegisterCrossProductAliases 注册。
        """
        result = dws.run_raw(
            "sheet", "find",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--keyword", "test",
        )
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined, (
            f"--keyword 应作为 --query 的 alias 被接受: stdout={result.stdout}, stderr={result.stderr}"
        )

    def test_find_wrong_ignore_case_flag(self, dws):
        """--ignore-case 不是合法 flag（应为 --match-case=false）。"""
        result = dws.run_raw(
            "sheet", "find",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--find", "test",
            "--ignore-case",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_find_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "find")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── merge-cells ─────────────────────────────────────────

    def test_merge_cells_sticky_range_flag(self, dws):
        """--rangeA1:B2 粘连参数应被拒绝。"""
        result = dws.run_raw(
            "sheet", "merge-cells",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--rangeA1:B2",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_merge_cells_wrong_type_flag(self, dws):
        """--type 不是合法 flag（应为 --merge-type）。"""
        result = dws.run_raw(
            "sheet", "merge-cells",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--range", "A1:B2",
            "--type", "mergeAll",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_merge_cells_wrong_area_flag(self, dws):
        """--area 不是合法 flag（应为 --range）。"""
        result = dws.run_raw(
            "sheet", "merge-cells",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--area", "A1:B2",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_merge_cells_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "merge-cells")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── insert-dimension ────────────────────────────────────

    def test_insert_dimension_wrong_type_flag(self, dws):
        """--type 不是合法 flag（应为 --dimension）。"""
        result = dws.run_raw(
            "sheet", "insert-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--type", "ROWS",
            "--position", "3",
            "--length", "1",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_insert_dimension_wrong_count_flag(self, dws):
        """--count 不是合法 flag（应为 --length）。"""
        result = dws.run_raw(
            "sheet", "insert-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--position", "3",
            "--count", "1",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_insert_dimension_sticky_length_flag(self, dws):
        """--length5 粘连参数应被拒绝。"""
        result = dws.run_raw(
            "sheet", "insert-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--position", "3",
            "--length5",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_insert_dimension_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "insert-dimension")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── delete-dimension ────────────────────────────────────

    def test_delete_dimension_wrong_type_flag(self, dws):
        """--type 不是合法 flag（应为 --dimension）。"""
        result = dws.run_raw(
            "sheet", "delete-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--type", "ROWS",
            "--position", "3",
            "--length", "1",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_delete_dimension_wrong_count_flag(self, dws):
        """--count 不是合法 flag（应为 --length）。"""
        result = dws.run_raw(
            "sheet", "delete-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--position", "3",
            "--count", "1",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_delete_dimension_sticky_length_flag(self, dws):
        """--length5 粘连参数应被拒绝。"""
        result = dws.run_raw(
            "sheet", "delete-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--position", "3",
            "--length5",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_delete_dimension_wrong_start_flag(self, dws):
        """--start 不是合法 flag（应为 --position）。"""
        result = dws.run_raw(
            "sheet", "delete-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--start", "3",
            "--length", "1",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_delete_dimension_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "delete-dimension")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── update-dimension ────────────────────────────────────

    def test_update_dimension_wrong_type_flag(self, dws):
        """--type 不是合法 flag（应为 --dimension）。"""
        result = dws.run_raw(
            "sheet", "update-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--type", "ROWS",
            "--start-index", "3",
            "--length", "1",
            "--hidden",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_update_dimension_wrong_count_flag(self, dws):
        """--count 不是合法 flag（应为 --length）。"""
        result = dws.run_raw(
            "sheet", "update-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--start-index", "3",
            "--count", "1",
            "--hidden",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_update_dimension_wrong_position_flag(self, dws):
        """--position 不是合法 flag（应为 --start-index）。"""
        result = dws.run_raw(
            "sheet", "update-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--position", "3",
            "--length", "1",
            "--hidden",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_update_dimension_wrong_size_flag(self, dws):
        """--size 不是合法 flag（应为 --pixel-size）。"""
        result = dws.run_raw(
            "sheet", "update-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--start-index", "3",
            "--length", "1",
            "--size", "40",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_update_dimension_sticky_length_flag(self, dws):
        """--length5 粘连参数应被拒绝。"""
        result = dws.run_raw(
            "sheet", "update-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--start-index", "3",
            "--length5",
            "--hidden",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_update_dimension_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "update-dimension")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── media-upload ────────────────────────────────────────

    def test_media_upload_sticky_node_flag(self, dws):
        """--nodeXXX 粘连参数应被拒绝。"""
        result = dws.run_raw(
            "sheet", "media-upload",
            "--nodevy20BglGWOdR042ZTGaBdRvDVA7depqY",
            "--file", "/tmp/test.txt",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "unknown flag" in combined

    def test_media_upload_wrong_document_flag(self, dws):
        """--document 不是合法 flag（应为 --node）。"""
        result = dws.run_raw(
            "sheet", "media-upload",
            "--document", "SOME_NODE",
            "--file", "/tmp/test.txt",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "unknown flag" in combined

    def test_media_upload_wrong_path_flag(self, dws):
        """--path 不是合法 flag（应为 --file）。"""
        result = dws.run_raw(
            "sheet", "media-upload",
            "--node", "SOME_NODE",
            "--path", "/tmp/test.txt",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "unknown flag" in combined

    def test_media_upload_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "media-upload")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── write-image ─────────────────────────────────────────

    def test_write_image_sticky_node_flag(self, dws):
        """--nodeXXX 粘连参数应被拒绝。"""
        result = dws.run_raw(
            "sheet", "write-image",
            "--nodevy20BglGWOdR042ZTGaBdRvDVA7depqY",
            "--sheet-id", "Sheet1",
            "--range", "A1:A1",
            "--file", "/tmp/test.png",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "unknown flag" in combined

    def test_write_image_sticky_range_flag(self, dws):
        """--rangeA1:A1 粘连参数应被拒绝。"""
        result = dws.run_raw(
            "sheet", "write-image",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--rangeA1:A1",
            "--file", "/tmp/test.png",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "unknown flag" in combined

    def test_write_image_wrong_sheetId_flag(self, dws):
        """--sheetId (驼峰) 不是合法 flag（应为 --sheet-id）。"""
        result = dws.run_raw(
            "sheet", "write-image",
            "--node", "SOME_NODE",
            "--sheetId", "Sheet1",
            "--range", "A1:A1",
            "--file", "/tmp/test.png",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "unknown flag" in combined

    def test_write_image_wrong_document_flag(self, dws):
        """--document 不是合法 flag（应为 --node）。"""
        result = dws.run_raw(
            "sheet", "write-image",
            "--document", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--range", "A1:A1",
            "--file", "/tmp/test.png",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "unknown flag" in combined

    def test_write_image_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "write-image")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── replace ─────────────────────────────────────────────

    def test_replace_wrong_search_flag(self, dws):
        """--search 不是合法 flag（应为 --find）。"""
        result = dws.run_raw(
            "sheet", "replace",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--search", "old",
            "--replacement", "new",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_replace_wrong_text_flag(self, dws):
        """--text 不是合法 flag（应为 --find）。"""
        result = dws.run_raw(
            "sheet", "replace",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--text", "old",
            "--replacement", "new",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_replace_wrong_replace_flag(self, dws):
        """--replace 不是合法 flag（应为 --replacement）。"""
        result = dws.run_raw(
            "sheet", "replace",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--find", "old",
            "--replace", "new",  # 故意传旧参数名，应报错
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_replace_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "replace")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── move-dimension ──────────────────────────────────────

    def test_move_dimension_wrong_type_flag(self, dws):
        """--type 不是合法 flag（应为 --dimension）。"""
        result = dws.run_raw(
            "sheet", "move-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--type", "ROWS",
            "--start-index", "0",
            "--end-index", "0",
            "--destination-index", "2",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_move_dimension_wrong_from_flag(self, dws):
        """--from 不是合法 flag（应为 --start-index）。"""
        result = dws.run_raw(
            "sheet", "move-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--from", "0",
            "--end-index", "0",
            "--destination-index", "2",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_move_dimension_wrong_to_flag(self, dws):
        """--to 不是合法 flag（应为 --destination-index）。"""
        result = dws.run_raw(
            "sheet", "move-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--start-index", "0",
            "--end-index", "0",
            "--to", "2",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_move_dimension_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "move-dimension")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── add-dimension ───────────────────────────────────────

    def test_add_dimension_wrong_count_flag(self, dws):
        """--count 不是合法 flag（应为 --length）。"""
        result = dws.run_raw(
            "sheet", "add-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--dimension", "ROWS",
            "--count", "5",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_add_dimension_wrong_type_flag(self, dws):
        """--type 不是合法 flag（应为 --dimension）。"""
        result = dws.run_raw(
            "sheet", "add-dimension",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--type", "ROWS",
            "--length", "5",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_add_dimension_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "add-dimension")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── unmerge-cells ───────────────────────────────────────

    def test_unmerge_cells_wrong_area_flag(self, dws):
        """--area 不是合法 flag（应为 --range）。"""
        result = dws.run_raw(
            "sheet", "unmerge-cells",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--area", "A1:B2",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_unmerge_cells_wrong_cells_flag(self, dws):
        """--cells 不是合法 flag（应为 --range）。"""
        result = dws.run_raw(
            "sheet", "unmerge-cells",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--cells", "A1:B2",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_unmerge_cells_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "unmerge-cells")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── create-float-image ───────────────────────────────────

    def test_create_float_image_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "create-float-image")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_create_float_image_wrong_image_flag(self, dws):
        """--image 不是合法 flag（应为 --src）。"""
        result = dws.run_raw(
            "sheet", "create-float-image",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--image", "https://example.com/img.png",
            "--range", "A1",
            "--width", "200",
            "--height", "150",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_create_float_image_wrong_url_flag(self, dws):
        """--url 不是合法 flag（应为 --src）。"""
        result = dws.run_raw(
            "sheet", "create-float-image",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--url", "https://example.com/img.png",
            "--range", "A1",
            "--width", "200",
            "--height", "150",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_create_float_image_zero_width(self, dws):
        """--width 为 0 应报错（必须为正整数）。"""
        result = dws.run_raw(
            "sheet", "create-float-image",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--src", "https://example.com/img.png",
            "--range", "A1",
            "--width", "0",
            "--height", "150",
        )
        assert result.returncode != 0

    def test_create_float_image_negative_height(self, dws):
        """--height 为负数应报错。"""
        result = dws.run_raw(
            "sheet", "create-float-image",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--src", "https://example.com/img.png",
            "--range", "A1",
            "--width", "200",
            "--height", "-1",
        )
        assert result.returncode != 0

    def test_create_float_image_negative_offset_x(self, dws):
        """--offset-x 为负数应报错。"""
        result = dws.run_raw(
            "sheet", "create-float-image",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--src", "https://example.com/img.png",
            "--range", "A1",
            "--width", "200",
            "--height", "150",
            "--offset-x", "-5",
        )
        assert result.returncode != 0

    # ── get-float-image ──────────────────────────────────────

    def test_get_float_image_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "get-float-image")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_get_float_image_wrong_id_flag(self, dws):
        """--id 不是合法 flag（应为 --float-image-id）。"""
        result = dws.run_raw(
            "sheet", "get-float-image",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--id", "fi12345678",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── list-float-images ────────────────────────────────────

    def test_list_float_images_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "list-float-images")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_list_float_images_wrong_sheetId_flag(self, dws):
        """--sheetId (驼峰) 不是合法 flag（应为 --sheet-id）。"""
        result = dws.run_raw(
            "sheet", "list-float-images",
            "--node", "SOME_NODE",
            "--sheetId", "Sheet1",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "unknown flag" in combined

    # ── update-float-image ───────────────────────────────────

    def test_update_float_image_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "update-float-image")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_update_float_image_no_update_field(self, dws):
        """只传必填标识但不传任何更新字段应报错。"""
        result = dws.run_raw(
            "sheet", "update-float-image",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--float-image-id", "fi12345678",
        )
        assert result.returncode != 0

    def test_update_float_image_negative_width(self, dws):
        """--width 为负数应报错。"""
        result = dws.run_raw(
            "sheet", "update-float-image",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--float-image-id", "fi12345678",
            "--width", "-10",
        )
        assert result.returncode != 0

    def test_update_float_image_wrong_id_flag(self, dws):
        """--image-id 不是合法 flag（应为 --float-image-id）。"""
        result = dws.run_raw(
            "sheet", "update-float-image",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--image-id", "fi12345678",
            "--width", "300",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── delete-float-image ───────────────────────────────────

    def test_delete_float_image_missing_all_required(self, dws):
        """不传任何必填参数应报错。"""
        result = dws.run_raw("sheet", "delete-float-image")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_delete_float_image_wrong_id_flag(self, dws):
        """--id 不是合法 flag（应为 --float-image-id）。"""
        result = dws.run_raw(
            "sheet", "delete-float-image",
            "--node", "SOME_NODE",
            "--sheet-id", "Sheet1",
            "--id", "fi12345678",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()


class TestSheetCrossProductAlias:
    """跨产品 alias 兼容性：sheet 命令应接受 drive 常用的参数名 --file-id。"""

    def test_info_accepts_file_id_alias(self, dws):
        """sheet info --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("sheet", "info", "--file-id", "FAKE_NODE_ID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_list_accepts_file_id_alias(self, dws):
        """sheet list --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("sheet", "list", "--file-id", "FAKE_NODE_ID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_find_accepts_file_id_alias(self, dws):
        """sheet find --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("sheet", "find", "--file-id", "FAKE_NODE_ID", "--sheet-id", "Sheet1", "--find", "test")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_append_accepts_file_id_alias(self, dws):
        """sheet append --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("sheet", "append", "--file-id", "FAKE_NODE_ID", "--sheet-id", "Sheet1", "--values", "[[1]]")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_range_read_accepts_file_id_alias(self, dws):
        """sheet range read --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("sheet", "range", "read", "--file-id", "FAKE_NODE_ID", "--sheet-id", "Sheet1", "--range", "A1:B2")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_export_accepts_file_id_alias(self, dws):
        """sheet export --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("sheet", "export", "--file-id", "FAKE_NODE_ID")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_new_accepts_file_id_alias(self, dws):
        """sheet new --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw("sheet", "new", "--file-id", "FAKE_NODE_ID", "--name", "TestSheet")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_range_update_accepts_file_id_alias(self, dws):
        """sheet range update --file-id 应被接受（不报 unknown flag）。"""
        result = dws.run_raw(
            "sheet", "range", "update",
            "--file-id", "FAKE_NODE_ID",
            "--sheet-id", "Sheet1",
            "--range", "A1",
            "--values", '[["test"]]',
        )
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined

    def test_create_accepts_parent_id_alias(self, dws):
        """sheet create --parent-id 应被接受（--folder 的跨产品 alias）。"""
        result = dws.run_raw("sheet", "create", "--name", "Test", "--parent-id", "FAKE_FOLDER")
        combined = (result.stdout + result.stderr).lower()
        assert "unknown flag" not in combined
