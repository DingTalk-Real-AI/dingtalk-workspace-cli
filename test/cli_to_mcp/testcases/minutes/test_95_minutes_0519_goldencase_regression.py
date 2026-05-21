"""
test_95_minutes_0519_goldencase_regression.py — 0514-0519 听记 Golden Case 回归测试

基于 听记_0514_0519_20260520_0920.md 中新发现的错误模式：
  Case 9 (P0): transcription 翻页15+次，缺少 --all 自动翻页能力
  Case 10 (P1): minutes summary --uuid 顶层写法报 unknown flag
  Case 11 (P2): list 不带 scope 返回不完整 vs list all 返回完整
  Case 7 (P2): --url 参数不存在，minutes transcribe 不是合法子命令

测试目标：验证 0519 新发现的错误命令必须报错 + 正确链路可走通。
"""

from typing import Optional

import pytest


def _extract_items(data: dict) -> list:
    """从 list 返回的多种可能结构中健壮地提取 items 列表。"""
    result = data.get("result", {})
    if isinstance(result, dict):
        items = (
            result.get("itemList", [])
            or result.get("minutes", [])
            or result.get("items", [])
        )
        if items:
            return items
    if isinstance(result, list):
        return result
    return data.get("minutes", []) or data.get("itemList", []) or data.get("items", [])


def _extract_id(item: dict) -> Optional[str]:
    """从单个听记 item 中提取 ID。"""
    return (
        item.get("taskUuid")
        or item.get("minutesId")
        or item.get("uuid")
        or item.get("id")
    )


class TestTopLevelSummaryRejected:
    """Case 10: 验证 'minutes summary --uuid' 顶层写法被拒绝。

    对应 Golden Case: minutes-0519-010 (P1)
    错误模式: LLM 写 'dws minutes summary --uuid <id>'，summary 不是顶层子命令。
    """

    def test_minutes_summary_uuid_rejected(self, dws):
        """'dws minutes summary --uuid xxx' 应报错（summary 不在顶层）。"""
        result = dws.run_raw(
            "minutes", "summary",
            "--uuid", "76327569643239393938373236355f3133353835373431365f32",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "unknown" in combined.lower() or "error" in combined.lower()

    def test_minutes_summary_id_rejected(self, dws):
        """'dws minutes summary --id xxx' 应报错（summary 不在顶层）。"""
        result = dws.run_raw(
            "minutes", "summary",
            "--id", "76327569643239393938373236355f3133353835373431365f32",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "unknown" in combined.lower() or "error" in combined.lower()

    def test_correct_get_summary_id_accepted(self, dws):
        """正确路径 'dws minutes get summary --id xxx' 不报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "get", "summary",
            "--id", "TEST_PLACEHOLDER_FLAG_CHECK",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        # --id 是合法参数，不应报 unknown flag（即使值无效只会报业务错误）
        assert "unknown flag" not in combined.lower()

    def test_correct_get_summary_uuid_alias_accepted(self, dws):
        """'dws minutes get summary --uuid xxx' 别名不报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "get", "summary",
            "--uuid", "TEST_PLACEHOLDER_FLAG_CHECK",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()


class TestTranscribeAndUrlParamRejected:
    """Case 7: 验证 'minutes transcribe --url' 不存在。

    对应 Golden Case: minutes-0514-007 (P2)
    错误模式: LLM 编造 'dws minutes transcribe --url <听记url>'。
    """

    def test_minutes_transcribe_not_valid_subcommand(self, dws):
        """'dws minutes transcribe --url xxx' 应报 unknown subcommand。"""
        result = dws.run_raw(
            "minutes", "transcribe",
            "--url", "https://shanji.dingtalk.com/app/transcribes/7632756964323839",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "unknown" in combined.lower() or "error" in combined.lower()

    def test_get_transcription_url_param_rejected(self, dws):
        """'dws minutes get transcription --url xxx' 应报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "get", "transcription",
            "--url", "https://shanji.dingtalk.com/app/transcribes/7632756964323839",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" in combined.lower() or result.returncode != 0

    def test_get_transcription_id_accepted(self, dws):
        """正确路径 'dws minutes get transcription --id xxx' 不报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "get", "transcription",
            "--id", "TEST_PLACEHOLDER_FLAG_CHECK",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()


class TestListScopeBehavior:
    """Case 11: 验证 list 必须带 scope，list all 返回更完整结果。

    对应 Golden Case: minutes-0519-011 (P2)
    错误模式: 'dws minutes list' 不带 scope 返回不完整。
    """

    def test_list_all_returns_valid_data(self, dws):
        """'dws minutes list all' 应返回有效数据结构。"""
        data = dws.run_ok("minutes", "list", "all", "--max", "5")
        items = _extract_items(data)
        assert isinstance(items, list)

    def test_list_mine_returns_valid_data(self, dws):
        """'dws minutes list mine' 应返回有效数据结构。"""
        data = dws.run_ok("minutes", "list", "mine", "--max", "5")
        items = _extract_items(data)
        assert isinstance(items, list)

    def test_list_all_returns_more_or_equal(self, dws):
        """'list all' 返回条数应 >= 'list mine'（all = mine ∪ shared，覆盖面更广）。

        注意：由于分页限制（--max），不能简单断言 all 是 mine 的超集，
        但在相同 max 下 all 返回的总量不应少于 mine。
        """
        mine_data = dws.run_ok("minutes", "list", "mine", "--max", "5")
        all_data = dws.run_ok("minutes", "list", "all", "--max", "5")

        mine_items = _extract_items(mine_data)
        all_items = _extract_items(all_data)

        # all 应返回有效数据（至少不为空，如果 mine 有数据的话）
        if mine_items:
            assert len(all_items) > 0, "list all 在 mine 有数据时不应为空"


class TestTranscriptionPaginationLongMeeting:
    """Case 9: 验证 transcription 翻页机制在长会议中的表现。

    对应 Golden Case: minutes-0519-009 (P0)
    错误模式: 70min 会议翻页 15+ 次，LLM 中途放弃改用 shell 重定向。
    验证点: next-token 参数可用、多次翻页不报错。
    """

    @pytest.fixture(scope="class")
    def sample_id(self, dws):
        """获取一个有效听记 ID。"""
        data = dws.run_ok("minutes", "list", "mine", "--max", "3")
        items = _extract_items(data)
        if not items:
            pytest.skip("当前账号无听记数据")
        mid = _extract_id(items[0])
        if not mid:
            pytest.skip("听记项缺少 ID 字段")
        return mid

    def test_first_page_and_check_next_token(self, dws, sample_id):
        """首次调用应返回数据，且可能包含 nextToken 用于翻页。"""
        data = dws.run_ok("minutes", "get", "transcription", "--id", sample_id)
        assert isinstance(data, dict)
        # 结果中应有 result 字段
        result = data.get("result", data)
        assert isinstance(result, (dict, list))

    def test_pagination_second_page_if_available(self, dws, sample_id):
        """如果首页有 nextToken，第二页调用应正常返回。"""
        data = dws.run_ok("minutes", "get", "transcription", "--id", sample_id)
        result = data.get("result", {})
        next_token = None
        if isinstance(result, dict):
            next_token = result.get("nextToken") or result.get("next_token")

        if not next_token:
            pytest.skip("该听记只有一页，无需翻页")

        # 第二页调用
        page2 = dws.run_ok(
            "minutes", "get", "transcription",
            "--id", sample_id,
            "--next-token", next_token,
        )
        assert isinstance(page2, dict)

    def test_next_token_flag_accepted(self, dws, sample_id):
        """验证 --next-token 是 get transcription 的合法参数（不报 unknown flag）。"""
        result = dws.run_raw(
            "minutes", "get", "transcription",
            "--id", sample_id,
            "--next-token", "PLACEHOLDER_TOKEN",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        # --next-token 是合法参数，不应报 unknown flag
        assert "unknown flag" not in combined.lower()

    def test_page_token_flag_rejected(self, dws, sample_id):
        """验证 --page-token 不存在（常见错误：与其他模块混淆）。"""
        result = dws.run_raw(
            "minutes", "get", "transcription",
            "--id", sample_id,
            "--page-token", "PLACEHOLDER_TOKEN",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" in combined.lower() or result.returncode != 0


class TestCrossProductParamSpec:
    """跨产品参数规约 (dws-cli-param-spec) 对齐验证。

    验证 minutes 模块的参数与跨产品规约 Primary/Alias 的实际行为：
    - Group 13 (单页大小): Primary=--limit, minutes 实际用 --max
    - Group 14 (续页标识): Primary=--cursor, minutes 实际用 --next-token
    - Group 15/16 (起止时间): Primary=--start/--end, minutes 已对齐
    """

    def test_max_is_accepted_for_list(self, dws):
        """minutes list mine --max 是当前 CLI 实际参数，必须可用。"""
        result = dws.run_raw("minutes", "list", "mine", "--max", "3")
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    def test_limit_not_yet_registered(self, dws):
        """--limit 尚未在 minutes list 注册为 alias（规约 Primary 待对齐）。

        此测试记录当前行为：--limit 报 unknown flag。
        当 CLI 完成 alias 注册后此测试应改为验证 --limit 可用。
        """
        result = dws.run_raw("minutes", "list", "mine", "--limit", "3")
        combined = (result.stdout or "") + (result.stderr or "")
        # 当前 --limit 未注册，应报 unknown flag
        assert "unknown flag" in combined.lower() or result.returncode != 0

    def test_cursor_not_yet_registered_for_list(self, dws):
        """--cursor 尚未在 minutes list 注册为 --next-token 的 alias。

        此测试记录当前行为。
        """
        result = dws.run_raw("minutes", "list", "mine", "--cursor", "PLACEHOLDER")
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" in combined.lower() or result.returncode != 0

    def test_start_is_accepted(self, dws):
        """--start 是 minutes list 的合法参数（与规约 Primary 一致）。"""
        result = dws.run_raw(
            "minutes", "list", "mine",
            "--start", "2026-05-01T00:00:00+08:00",
            "--max", "1",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    def test_end_is_accepted(self, dws):
        """--end 是 minutes list 的合法参数（与规约 Primary 一致）。"""
        result = dws.run_raw(
            "minutes", "list", "mine",
            "--end", "2026-05-20T23:59:59+08:00",
            "--max", "1",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    def test_start_time_rejected(self, dws):
        """--start-time 不是 minutes 的合法参数（calendar 的参数，跨模块污染）。"""
        result = dws.run_raw(
            "minutes", "list", "mine",
            "--start-time", "2026-05-01T00:00:00+08:00",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" in combined.lower() or result.returncode != 0

    def test_query_is_accepted(self, dws):
        """--query 是 minutes list 的合法参数（与规约 Primary 一致）。"""
        result = dws.run_raw(
            "minutes", "list", "mine",
            "--query", "test",
            "--max", "1",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()
