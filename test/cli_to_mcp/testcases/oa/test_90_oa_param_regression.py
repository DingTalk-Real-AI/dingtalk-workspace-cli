"""oa 高频错误参数回归用例。"""

from test_utils import iso8601_cn_offset

PROCESS_CODE = "PROC-C6A9BDC0-93BD-459D-91E9-DF26B6981ACA"


class TestOaParamRegression:
    def test_list_initiated_missing_process_code(self, dws):
        result = dws.run_raw(
            "oa", "approval", "list-initiated",
            "--process-code", PROCESS_CODE,
            "--start", iso8601_cn_offset(days=-7),
            "--end", iso8601_cn_offset(),
            "--next-token", "0",
            "--max-results", "20",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_list_initiated_wrong_start_format(self, dws):
        result = dws.run_raw(
            "oa", "approval", "list-initiated",
            "--process-code", PROCESS_CODE,
            "--start", "2026-03-23 00:00:00",
            "--end", "2026-03-23 23:59:59",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()
