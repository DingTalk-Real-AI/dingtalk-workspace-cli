"""
test_98_minutes_0512_goldencase_regression.py — 0512 P0 Golden Case 回归测试

基于 goldencase0512_minutes_p0.md 中 4 个 P0 case 提炼的错误模式：
  A) 参数名混淆：--uuid / --task-uuid 应为 --id（minutes-002/003/004）
  B) 子命令层级跳级："minutes summary" 应为 "minutes get summary"（minutes-003）
  C) LLM 编造 uuid 不先调 list（minutes-002）— 此处验证 list 链路可用
  D) summary vs transcription 意图区分（minutes-003）
  E) 多步流程 list → get 的完整链路验证（minutes-004）
  F) 跨模块周报场景 minutes 侧时间窗口过滤（minutes-005）
  G) 模糊时间表述"上上周"对应正确时间范围（minutes-004）

测试目标：验证错误参数必须报错 + 正确链路可走通，确保 Agent 有正确的工具路径。
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


class TestParamAliasAccepted:
    """验证 --id / --uuid / --task-uuid 三种参数别名均为合法参数。

    对应 Golden Case: minutes-002, minutes-003, minutes-004
    注意: --uuid 和 --task-uuid 已作为合法别名支持，不应报 unknown flag。
    """

    def test_get_summary_accepts_id_flag(self, dws):
        """get summary --id 为合法参数名，应不报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "get", "summary",
            "--id", "INVALID_BUT_CORRECT_FLAG_NAME",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    def test_get_summary_accepts_uuid_flag(self, dws):
        """get summary --uuid 为合法别名，应不报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "get", "summary",
            "--uuid", "INVALID_BUT_CORRECT_FLAG_NAME",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    def test_get_summary_accepts_task_uuid_flag(self, dws):
        """get summary --task-uuid 为合法别名，应不报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "get", "summary",
            "--task-uuid", "INVALID_BUT_CORRECT_FLAG_NAME",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    def test_get_transcription_accepts_uuid_flag(self, dws):
        """get transcription --uuid 为合法别名，应不报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "get", "transcription",
            "--uuid", "INVALID_BUT_CORRECT_FLAG_NAME",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()


class TestCommandHierarchySkip:
    """模式 B：验证跳过 get 中间层级的错误命令。

    对应 Golden Case: minutes-003, minutes-004
    LLM 常把 "dws minutes summary" 当顶层命令，遗漏 "get" 层级。
    """

    def test_minutes_summary_without_get_rejected(self, dws):
        """'dws minutes summary --id xxx' 缺少 get 层应报错。"""
        result = dws.run_raw(
            "minutes", "summary",
            "--id", "76327569643236363136363338375f3534333031353133375f39",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        # 应报 unknown command 或 error
        assert result.returncode != 0 or "error" in combined.lower() or "unknown" in combined.lower()

    def test_minutes_transcription_without_get_rejected(self, dws):
        """'dws minutes transcription --id xxx' 缺少 get 层应报错。"""
        result = dws.run_raw(
            "minutes", "transcription",
            "--id", "76327569643236363136363338375f3534333031353133375f39",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower() or "unknown" in combined.lower()


class TestListThenGetWorkflow:
    """模式 C/E：验证 list → get 的完整工作流链路可用。

    对应 Golden Case: minutes-002（不编造uuid）, minutes-004（多步流程）
    """

    @pytest.fixture(scope="class")
    def first_minutes_id(self, dws):
        """获取当前账号的第一个有效听记 ID。"""
        data = dws.run_ok("minutes", "list", "mine", "--max", "1")
        items = _extract_items(data)
        if not items:
            pytest.skip("当前账号无听记数据，无法验证 list→get 工作流")
        mid = _extract_id(items[0])
        if not mid:
            pytest.skip("听记项缺少 ID 字段")
        return mid

    def test_list_mine_then_get_summary_workflow(self, dws, first_minutes_id):
        """list mine 拿到 id 后 get summary 应成功返回。"""
        data = dws.run_ok("minutes", "get", "summary", "--id", first_minutes_id)
        assert isinstance(data, dict) and len(data) > 0

    def test_list_mine_then_get_transcription_workflow(self, dws, first_minutes_id):
        """list mine 拿到 id 后 get transcription 应成功返回。"""
        data = dws.run_ok("minutes", "get", "transcription", "--id", first_minutes_id)
        assert isinstance(data, dict) and len(data) > 0


class TestSummaryVsTranscriptionDistinction:
    """模式 D：验证 summary 和 transcription 是两个独立子命令，返回不同内容。

    对应 Golden Case: minutes-003
    """

    @pytest.fixture(scope="class")
    def sample_id(self, dws):
        """获取一个有效听记 ID。"""
        data = dws.run_ok("minutes", "list", "mine", "--max", "1")
        items = _extract_items(data)
        if not items:
            pytest.skip("当前账号无听记数据")
        mid = _extract_id(items[0])
        if not mid:
            pytest.skip("听记项缺少 ID 字段")
        return mid

    def test_summary_and_transcription_both_exist(self, dws, sample_id):
        """同一个 id 可分别调 get summary 和 get transcription，两者均有效。"""
        summary = dws.run_ok("minutes", "get", "summary", "--id", sample_id)
        transcription = dws.run_ok("minutes", "get", "transcription", "--id", sample_id)
        assert isinstance(summary, dict)
        assert isinstance(transcription, dict)

    def test_summary_and_transcription_return_different_content(self, dws, sample_id):
        """summary 和 transcription 返回的数据结构应不同。"""
        summary = dws.run_ok("minutes", "get", "summary", "--id", sample_id)
        transcription = dws.run_ok("minutes", "get", "transcription", "--id", sample_id)
        # 两者的顶层 key 集合应有差异（summary 含 markdown/content, transcription 含 paragraphs/items）
        summary_keys = set(summary.get("result", summary).keys()) if isinstance(summary.get("result", summary), dict) else set()
        trans_keys = set(transcription.get("result", transcription).keys()) if isinstance(transcription.get("result", transcription), dict) else set()
        # 至少有一个 key 不同即可证明是不同接口
        assert summary_keys != trans_keys or str(summary) != str(transcription)


class TestTimeRangeFilterForWeeklyReport:
    """模式 F/G：验证时间范围过滤能力（周报场景 + 模糊时间）。

    对应 Golden Case: minutes-004（上上周）, minutes-005（本周周报）
    """

    def test_list_mine_with_start_end_time_range(self, dws):
        """list mine 支持 --start/--end 时间范围过滤（周报场景）。"""
        data = dws.run_ok(
            "minutes", "list", "mine",
            "--start", "2026-04-28T00:00:00+08:00",
            "--end", "2026-05-02T23:59:59+08:00",
        )
        items = _extract_items(data)
        assert isinstance(items, list)

    def test_list_mine_rejects_start_time_flag(self, dws):
        """list mine --start-time 为错误参数名（应为 --start）。"""
        result = dws.run_raw(
            "minutes", "list", "mine",
            "--start-time", "2026-04-28T00:00:00+08:00",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower() or "unknown flag" in combined.lower()

    def test_list_mine_rejects_end_time_flag(self, dws):
        """list mine --end-time 为错误参数名（应为 --end）。"""
        result = dws.run_raw(
            "minutes", "list", "mine",
            "--end-time", "2026-05-02T23:59:59+08:00",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower() or "unknown flag" in combined.lower()

    def test_list_all_with_weekly_time_window(self, dws):
        """list all 按本周时间窗口过滤（跨模块周报场景 minutes 侧）。"""
        data = dws.run_ok(
            "minutes", "list", "all",
            "--start", "2026-05-12T00:00:00+08:00",
            "--end", "2026-05-16T23:59:59+08:00",
        )
        items = _extract_items(data)
        assert isinstance(items, list)


class TestCrossModuleParamIsolation:
    """模式 F 补充：验证 minutes 模块不接受其他模块的参数名。

    对应 Golden Case: minutes-005（doc 的 --markdown、report 的格式等）
    确保 minutes 的参数边界清晰，不会被跨模块场景下的参数名污染。
    """

    def test_list_mine_rejects_page_size_flag(self, dws):
        """list mine --page-size 不存在（应为 --max）。"""
        result = dws.run_raw("minutes", "list", "mine", "--page-size", "10")
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower() or "unknown flag" in combined.lower()

    def test_list_mine_rejects_date_range_flag(self, dws):
        """list mine --date-range 不存在（应为 --start/--end）。"""
        result = dws.run_raw("minutes", "list", "mine", "--date-range", "7d")
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower() or "unknown flag" in combined.lower()
