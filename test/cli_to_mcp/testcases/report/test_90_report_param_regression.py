"""report 参数兼容与错误参数回归用例。"""

from test_utils import iso8601_cn_offset


REPORT_SENT_COLUMNS = ["日期", "标题", "发送人", "状态", "日志内容", "钉钉链接"]
REPORT_LIST_COLUMNS = ["日期", "标题", "发送人", "状态", "钉钉链接"]
REPORT_LINK_MARKER = "[在钉钉中查看日志]("


class TestReportParamRegression:
    def _assert_markdown_table(self, data, columns):
        markdown = data.get("agentDisplayMarkdown", "")
        assert markdown.startswith("| " + " | ".join(columns) + " |")
        assert "钉钉链接" in markdown
        if data.get("count", 0) > 0:
            assert REPORT_LINK_MARKER in markdown

    def test_sent_sticky_cursor_and_size(self, dws):
        data = dws.run_ok(
            "report", "sent",
            "--cursor0", "--size20",
            "--start", iso8601_cn_offset(days=-7),
            "--end", iso8601_cn_offset(),
        )
        assert data.get("success") is True
        assert data.get("count", 0) <= 20
        assert data.get("agentDisplayContentIncluded") is True
        assert data.get("agentDisplayColumns") == REPORT_SENT_COLUMNS
        self._assert_markdown_table(data, REPORT_SENT_COLUMNS)

    def test_list_wrong_limit_flag(self, dws):
        data = dws.run_ok(
            "report", "list",
            "--start", iso8601_cn_offset(days=-7),
            "--end", iso8601_cn_offset(),
            "--limit", "20",
        )
        assert data.get("success") is True
        assert data.get("count", 0) <= 20
        assert data.get("agentDisplayContentIncluded") is False
        assert data.get("agentDisplayColumns") == REPORT_LIST_COLUMNS
        assert all("日志内容" not in item for item in data.get("result", []))
        self._assert_markdown_table(data, REPORT_LIST_COLUMNS)

    def test_inbox_wrong_date_flag(self, dws):
        result = dws.run_raw("report", "inbox", "--date", "2026-03-22")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()
