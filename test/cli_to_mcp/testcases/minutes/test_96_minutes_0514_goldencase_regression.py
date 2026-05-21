"""
test_96_minutes_0514_goldencase_regression.py — 0514 P0/P1 Golden Case 回归测试

基于 听记_0514_P0P1.md 中 7 个 case 提炼的错误模式：
  Case 1 (P0): 多模块并行 & + wait（Windows 不支持）+ --start-time 跨模块误用
  Case 2 (P0): 多步流程中途停止（前置步骤成功后未继续执行后续步骤）
  Case 3 (P1): minutes get/info 子命令名不一致 + 诉求偏移
  Case 4 (P1): 多模块参数频繁错配（--task-uuid / --start 误用）
  Case 5 (P1): get transcription 空 error_msg + --uuid 在 get 层级不识别
  Case 6 (P1): minutes detail 幻觉子命令 + report inbox 不存在
  Case 7 (P1): 能力缺失（文档目录修改）— 无 CLI 测试点

测试目标：验证 0514 新发现的错误命令必须报错 + 正确链路可走通。
"""

from typing import Optional

import pytest


def _extract_items(data: dict) -> list:
    """从 list 返回的多种可能结构中健壮地提取 items 列表。"""
    result = data.get("result", {})
    if isinstance(result, dict):
        items = result.get("itemList", []) or result.get("minutes", []) or result.get("items", [])
        if items:
            return items
    if isinstance(result, list):
        return result
    return data.get("minutes", []) or data.get("itemList", []) or data.get("items", [])


def _extract_id(item: dict) -> Optional[str]:
    """从单个听记 item 中提取 ID。"""
    return item.get("taskUuid") or item.get("minutesId") or item.get("uuid") or item.get("id")


class TestDetailSubcommandRejected:
    """Case 6: 验证 'minutes detail' 幻觉子命令被拒绝。

    对应 Golden Case: minutes-0514-006 (P1)
    错误模式: LLM 编造 'dws minutes detail --id xxx'，实际不存在 detail 子命令。
    """

    def test_minutes_detail_is_not_valid_subcommand(self, dws):
        """'dws minutes detail --id xxx' 应报 unknown subcommand 或类似错误。"""
        result = dws.run_raw(
            "minutes", "detail",
            "--id", "76327569643239393530333232333539315f32333230333333355f35",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "unknown" in combined.lower() or "error" in combined.lower()

    def test_minutes_detail_with_uuid_also_rejected(self, dws):
        """'dws minutes detail --uuid xxx' 同样应报错。"""
        result = dws.run_raw(
            "minutes", "detail",
            "--uuid", "76327569643239393530333232333539315f32333230333333355f35",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "unknown" in combined.lower() or "error" in combined.lower()

    def test_correct_alternative_get_info(self, dws):
        """正确路径 'dws minutes get info --id xxx' 不报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "get", "info",
            "--id", "TEST_PLACEHOLDER_FLAG_CHECK",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()


class TestGetWithoutSubcommandAndTaskUuid:
    """Case 4/5/6: 验证 'get --task-uuid' 缺子命令时报错。

    对应 Golden Case: minutes-0514-004, 005, 006 (P1)
    错误模式: LLM 写 'dws minutes get --task-uuid xxx' 跳过子命令层级。
    """

    def test_get_task_uuid_without_subcommand_rejected(self, dws):
        """'dws minutes get --task-uuid xxx' 缺子命令应报错。"""
        result = dws.run_raw(
            "minutes", "get",
            "--task-uuid", "76327569643239393530333232333539315f32333230333333355f35",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower() or "unknown" in combined.lower()

    def test_get_uuid_without_subcommand_rejected(self, dws):
        """'dws minutes get --uuid xxx' 缺子命令应报错。"""
        result = dws.run_raw(
            "minutes", "get",
            "--uuid", "76327569643239393530333232333539315f32333230333333355f35",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower() or "unknown" in combined.lower()

    @pytest.mark.parametrize("subcommand", ["info", "summary", "transcription"])
    def test_get_subcommand_with_task_uuid_accepted(self, dws, subcommand):
        """'dws minutes get <subcommand> --task-uuid xxx' 在子命令层 --task-uuid 是合法别名。"""
        result = dws.run_raw(
            "minutes", "get", subcommand,
            "--task-uuid", "TEST_PLACEHOLDER_FLAG_CHECK",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()


class TestCrossModuleParamContamination:
    """Case 1/3/5: 验证跨模块参数名不可互用。

    对应 Golden Case: minutes-0514-001, 003, 005 (P0/P1)
    错误模式: --start-time(calendar) 被用到 minutes list 上。
    """

    def test_list_mine_start_time_is_invalid(self, dws):
        """'dws minutes list mine --start-time ...' 应报 unknown flag（正确参数是 --start）。"""
        result = dws.run_raw(
            "minutes", "list", "mine",
            "--start-time", "2026-05-11T00:00:00+08:00",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" in combined.lower() or result.returncode != 0

    def test_list_mine_end_time_is_invalid(self, dws):
        """'dws minutes list mine --end-time ...' 应报 unknown flag（正确参数是 --end）。"""
        result = dws.run_raw(
            "minutes", "list", "mine",
            "--end-time", "2026-05-17T23:59:59+08:00",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" in combined.lower() or result.returncode != 0

    def test_list_mine_start_end_correct_params(self, dws):
        """'dws minutes list mine --start ... --end ...' 正确参数不报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "list", "mine",
            "--start", "2026-05-11T00:00:00+08:00",
            "--end", "2026-05-17T23:59:59+08:00",
            "--max", "1",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    def test_list_mine_limit_is_invalid(self, dws):
        """'dws minutes list mine --limit 10' 应报 unknown flag（正确参数是 --max）。"""
        result = dws.run_raw(
            "minutes", "list", "mine",
            "--limit", "10",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" in combined.lower() or result.returncode != 0


class TestReportModuleBoundary0514:
    """Case 1/6: 验证 report 模块边界（0514 复现）。

    对应 Golden Case: minutes-0514-001, 006 (P0/P1)
    """

    def test_report_inbox_does_not_exist(self, dws):
        """'dws report inbox' 不存在，应报 unknown subcommand。"""
        result = dws.run_raw("report", "inbox", "--format", "json")
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "unknown" in combined.lower() or "error" in combined.lower()

    def test_report_list_requires_start(self, dws):
        """'dws report list' 不带 --start 应报 required flag。"""
        result = dws.run_raw("report", "list", "--format", "json")
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "required" in combined.lower() or "start" in combined.lower()

    def test_report_list_start_time_is_invalid(self, dws):
        """'dws report list --start-time ...' 应报 unknown flag（正确参数是 --start）。"""
        result = dws.run_raw(
            "report", "list",
            "--start-time", "2026-05-06T00:00:00+08:00",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" in combined.lower() or result.returncode != 0


class TestEndToEndListGetWorkflow0514:
    """Case 4/5: 验证 list → get 的完整链路（0514 场景）。

    多步流程中途不能停止：list 获取 id 后必须继续 get summary/transcription。
    """

    @pytest.fixture(scope="class")
    def valid_id(self, dws):
        """从 list mine 获取第一个有效 ID。"""
        data = dws.run_ok("minutes", "list", "mine", "--max", "1")
        items = _extract_items(data)
        if not items:
            pytest.skip("No minutes available for current user")
        first_id = _extract_id(items[0])
        if not first_id:
            pytest.skip("Cannot extract ID from first item")
        return first_id

    def test_list_then_get_info(self, dws, valid_id):
        """list → get info 链路完整可用。"""
        data = dws.run_ok("minutes", "get", "info", "--id", valid_id)
        assert data is not None

    def test_list_then_get_summary(self, dws, valid_id):
        """list → get summary 链路完整可用。"""
        data = dws.run_ok("minutes", "get", "summary", "--id", valid_id)
        assert data is not None

    def test_list_then_get_transcription(self, dws, valid_id):
        """list → get transcription 链路完整可用。"""
        data = dws.run_ok("minutes", "get", "transcription", "--id", valid_id)
        assert data is not None

    def test_list_with_time_range(self, dws):
        """list mine 带时间范围参数可正常执行。"""
        data = dws.run_ok(
            "minutes", "list", "mine",
            "--start", "2026-05-01T00:00:00+08:00",
            "--end", "2026-05-15T23:59:59+08:00",
            "--max", "5",
        )
        items = _extract_items(data)
        assert isinstance(items, list)
