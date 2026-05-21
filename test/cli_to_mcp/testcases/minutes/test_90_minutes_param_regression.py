"""minutes 高频错误参数回归用例。"""


class TestMinutesParamRegression:
    def test_list_wrong_max_flag(self, dws):
        result = dws.run_raw("minutes", "list", "shared", "--max", "10")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_get_summary_wrong_task_uuid_flag(self, dws):
        result = dws.run_raw(
            "minutes", "get", "summary",
            "--task-uuid", "7632756964323339",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_get_info_wrong_url_flag(self, dws):
        result = dws.run_raw("minutes", "get", "info", "--url", "https://example.com")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── list all 参数回归 ──
    def test_list_all_wrong_limit_flag(self, dws):
        """list all 使用错误参数 --limit（应为 --max）。"""
        result = dws.run_raw("minutes", "list", "all", "--limit", "10")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_list_all_sticky_max(self, dws):
        """list all 参数粘连 --max20（应为 --max 20）。"""
        result = dws.run_raw("minutes", "list", "all", "--max20")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── update summary 参数回归 ──
    def test_update_summary_wrong_text_flag(self, dws):
        """update summary 使用错误参数 --text（应为 --content）。"""
        result = dws.run_raw(
            "minutes", "update", "summary",
            "--id", "7632756964323339", "--text", "test",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_update_summary_wrong_summary_flag(self, dws):
        """update summary 使用错误参数 --summary（应为 --content）。"""
        result = dws.run_raw(
            "minutes", "update", "summary",
            "--id", "7632756964323339", "--summary", "test",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── mind-graph 参数回归 ──
    def test_mind_graph_create_wrong_task_uuid_flag(self, dws):
        """mind-graph create 使用错误参数 --task-uuid（应为 --id）。"""
        result = dws.run_raw(
            "minutes", "mind-graph", "create",
            "--task-uuid", "7632756964323339",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── speaker 参数回归 ──
    def test_speaker_replace_wrong_source_flag(self, dws):
        """speaker replace 使用错误参数 --source（应为 --from）。"""
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", "7632756964323339", "--source", "A", "--to", "B",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_speaker_replace_wrong_target_flag(self, dws):
        """speaker replace 使用错误参数 --target（应为 --to）。"""
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", "7632756964323339", "--from", "A", "--target", "B",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── hot-word 参数回归 ──
    def test_hot_word_add_wrong_word_flag(self, dws):
        """hot-word add 使用错误参数 --word（应为 --words）。"""
        result = dws.run_raw("minutes", "hot-word", "add", "--word", "钉钉")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── replace-text 参数回归 ──
    def test_replace_text_wrong_find_flag(self, dws):
        """replace-text 使用错误参数 --find（应为 --search）。"""
        result = dws.run_raw(
            "minutes", "replace-text",
            "--id", "7632756964323339", "--find", "A", "--replace", "B",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_replace_text_wrong_old_flag(self, dws):
        """replace-text 使用错误参数 --old（应为 --search）。"""
        result = dws.run_raw(
            "minutes", "replace-text",
            "--id", "7632756964323339", "--old", "A", "--new", "B",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    # ── upload 参数回归 ──
    def test_upload_create_wrong_filename_flag(self, dws):
        """upload create 使用错误参数 --filename（应为 --file-name）。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--filename", "meeting.mp4", "--file-size", "1024",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_upload_create_wrong_size_flag(self, dws):
        """upload create 使用错误参数 --size（应为 --file-size）。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-name", "meeting.mp4", "--size", "1024",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_upload_complete_wrong_session_flag(self, dws):
        """upload complete 使用错误参数 --session（应为 --session-id）。"""
        result = dws.run_raw(
            "minutes", "upload", "complete",
            "--session", "abc123",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_upload_cancel_wrong_id_flag(self, dws):
        """upload cancel 使用错误参数 --id（应为 --session-id）。"""
        result = dws.run_raw(
            "minutes", "upload", "cancel",
            "--id", "abc123",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_upload_create_missing_all_required(self, dws):
        """upload create 不传任何参数应报错。"""
        result = dws.run_raw("minutes", "upload", "create")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_upload_complete_missing_session_id(self, dws):
        """upload complete 不传 --session-id 应报错。"""
        result = dws.run_raw("minutes", "upload", "complete")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_upload_cancel_missing_session_id(self, dws):
        """upload cancel 不传 --session-id 应报错。"""
        result = dws.run_raw("minutes", "upload", "cancel")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()
