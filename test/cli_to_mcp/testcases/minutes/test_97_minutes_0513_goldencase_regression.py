"""
test_97_minutes_0513_goldencase_regression.py — 0513 P0/P1 Golden Case 回归测试

基于 golden_case_p0p1_0513.md 中 7 个 case 提炼的错误模式：
  Case 1 (P0): --id/--uuid/--task-uuid 参数名统一 — get 各子命令均接受三种别名
  Case 2 (P0): URL 不能直接传 --id — 必须先提取 taskUuid hex
  Case 3 (P0): 共享/他人听记权限报错处理 — list shared 链路可用
  Case 4 (P0): upload create 参数校验 + cancel 必须带 session-id
  Case 5 (P0): shell 管道/并行写法在 dws 层面无影响（验证命令本身无管道依赖）
  Case 6 (P0): get transcription 分页 --next-token 是合法参数
  Case 7 (P1): report list 必须带 --start + report inbox 不存在

测试目标：验证错误参数必须报错 + 正确链路可走通 + 分页参数可用。
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


class TestParamAliasAllSubcommands:
    """Case 1: 验证 get info / get summary / get transcription 均接受 --id / --uuid / --task-uuid。

    对应 Golden Case: minutes-001 (P0)
    错误模式: LLM 在不同子命令间猜测 --id / --uuid / --task-uuid，被 unknown flag 拒绝。
    """

    @pytest.mark.parametrize("subcommand", ["info", "summary", "transcription"])
    def test_get_subcommand_accepts_id(self, dws, subcommand):
        """get <subcommand> --id 为合法参数。"""
        result = dws.run_raw(
            "minutes", "get", subcommand,
            "--id", "TEST_PLACEHOLDER_FLAG_CHECK",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    @pytest.mark.parametrize("subcommand", ["info", "summary", "transcription"])
    def test_get_subcommand_accepts_uuid(self, dws, subcommand):
        """get <subcommand> --uuid 为合法别名。"""
        result = dws.run_raw(
            "minutes", "get", subcommand,
            "--uuid", "TEST_PLACEHOLDER_FLAG_CHECK",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    @pytest.mark.parametrize("subcommand", ["info", "summary", "transcription"])
    def test_get_subcommand_accepts_task_uuid(self, dws, subcommand):
        """get <subcommand> --task-uuid 为合法别名。"""
        result = dws.run_raw(
            "minutes", "get", subcommand,
            "--task-uuid", "TEST_PLACEHOLDER_FLAG_CHECK",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    def test_get_without_subcommand_rejected(self, dws):
        """'dws minutes get --id xxx' 缺少子命令（info/summary/transcription）应报错。"""
        result = dws.run_raw(
            "minutes", "get",
            "--id", "76327569643237393730383837365f323733363637393038355f30",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        # get 后必须跟子命令
        assert result.returncode != 0 or "error" in combined.lower() or "unknown" in combined.lower()


class TestCommandHierarchyEnforced:
    """Case 1 补充: 验证跳过 get 层级的错误命令被拒绝。

    LLM 常写 'dws minutes summary --uuid xxx' 或 'dws minutes info --task-uuid xxx'。
    """

    @pytest.mark.parametrize("wrong_subcommand", ["summary", "info", "transcription"])
    def test_top_level_subcommand_rejected(self, dws, wrong_subcommand):
        """'dws minutes <subcommand> --id xxx' 缺少 get 层应报错。"""
        result = dws.run_raw(
            "minutes", wrong_subcommand,
            "--id", "76327569643237393730383837365f323733363637393038355f30",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower() or "unknown" in combined.lower()


class TestURLCannotBePassedAsId:
    """Case 2: 验证完整 URL 不能直接作为 --id 值传入（应先提取 taskUuid）。

    对应 Golden Case: minutes-002 (P0)
    """

    SAMPLE_URL = "https://shanji.dingtalk.com/app/transcribes/76327569643238393333333331395f363533313432353937365f32"

    def test_url_as_id_returns_error_not_unknown_flag(self, dws):
        """传入完整 URL 作为 --id，应返回业务错误（非 unknown flag），说明参数被接受但值无效。"""
        result = dws.run_raw(
            "minutes", "get", "info",
            "--id", self.SAMPLE_URL,
        )
        combined = (result.stdout or "") + (result.stderr or "")
        # --id 参数本身合法（不报 unknown flag），但值不合法应报业务错误
        assert "unknown flag" not in combined.lower()
        # 应该有某种错误返回（业务报错或参数值不合法）
        assert result.returncode != 0 or "error" in combined.lower()

    def test_extracted_uuid_from_url_accepted(self, dws):
        """从 URL 中提取的 hex taskUuid 传入 --id，不应报 unknown flag。"""
        task_uuid = "76327569643238393333333331395f363533313432353937365f32"
        result = dws.run_raw(
            "minutes", "get", "info",
            "--id", task_uuid,
        )
        combined = (result.stdout or "") + (result.stderr or "")
        # 参数名正确，不报 unknown flag（可能报权限或其他业务错误，但不是 flag 问题）
        assert "unknown flag" not in combined.lower()


class TestPermissionErrorHandling:
    """Case 3: 验证权限错误是业务错误而非参数问题。

    对应 Golden Case: minutes-003 (P0)
    访问他人听记时应返回权限相关错误，而非 unknown flag。
    """

    # 使用一个不太可能属于测试账号的 ID
    FOREIGN_ID = "7632756964323838383731343831303432375f363533303130313637355f35"

    def test_permission_denied_is_not_flag_error(self, dws):
        """访问他人听记时返回权限错误，不是 unknown flag。"""
        result = dws.run_raw(
            "minutes", "get", "summary",
            "--id", self.FOREIGN_ID,
        )
        combined = (result.stdout or "") + (result.stderr or "")
        # 不应是 unknown flag 错误
        assert "unknown flag" not in combined.lower()
        # 应有错误（权限或业务层面）
        assert result.returncode != 0 or "error" in combined.lower() or "permission" in combined.lower()

    def test_list_shared_is_valid_command(self, dws):
        """list shared 是合法命令入口（用于获取他人共享给我的听记）。"""
        result = dws.run_raw("minutes", "list", "shared", "--max", "1")
        combined = (result.stdout or "") + (result.stderr or "")
        # 命令本身合法，不报 unknown subcommand
        assert "unknown" not in combined.lower() or "subcommand" not in combined.lower()

    def test_list_all_is_valid_command(self, dws):
        """list all 是合法命令入口（查询所有有权限听记）。"""
        data = dws.run_ok("minutes", "list", "all", "--max", "1")
        items = _extract_items(data)
        assert isinstance(items, list)


class TestUploadCreateParams:
    """Case 4: 验证 upload create / cancel 参数校验。

    对应 Golden Case: minutes-004 (P0)
    """

    def test_upload_cancel_requires_session_id(self, dws):
        """upload cancel 不带 --session-id 应报 missing required flag。"""
        result = dws.run_raw("minutes", "upload", "cancel")
        combined = (result.stdout or "") + (result.stderr or "")
        assert "required" in combined.lower() or "session-id" in combined.lower() or result.returncode != 0

    def test_upload_create_requires_file_name(self, dws):
        """upload create 不带 --file-name 应报错。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-size", "1000",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "required" in combined.lower() or "file-name" in combined.lower()

    def test_upload_create_requires_file_size(self, dws):
        """upload create 不带 --file-size 应报错。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-name", "test.m4a",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "required" in combined.lower() or "file-size" in combined.lower()

    def test_upload_complete_requires_session_id(self, dws):
        """upload complete 不带 --session-id 应报 missing required flag。"""
        result = dws.run_raw("minutes", "upload", "complete")
        combined = (result.stdout or "") + (result.stderr or "")
        assert "required" in combined.lower() or "session-id" in combined.lower() or result.returncode != 0


class TestTranscriptionPagination:
    """Case 6: 验证 get transcription 的 --next-token 是合法参数。

    对应 Golden Case: minutes-006 (P0)
    错误模式: LLM 写 --next-token 被报 unknown flag（实际应该是合法的）。
    """

    def test_next_token_is_valid_flag(self, dws):
        """get transcription --next-token 不应报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "get", "transcription",
            "--id", "TEST_ID_PLACEHOLDER",
            "--next-token", "FAKE_TOKEN_12345",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    def test_page_token_is_invalid_flag(self, dws):
        """get transcription --page-token 不存在，应报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "get", "transcription",
            "--id", "TEST_ID_PLACEHOLDER",
            "--page-token", "FAKE_TOKEN_12345",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" in combined.lower() or result.returncode != 0

    def test_direction_is_valid_flag(self, dws):
        """get transcription --direction 是合法参数。"""
        result = dws.run_raw(
            "minutes", "get", "transcription",
            "--id", "TEST_ID_PLACEHOLDER",
            "--direction", "1",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    @pytest.fixture(scope="class")
    def first_id(self, dws):
        """获取第一个有效听记 ID 用于分页测试。"""
        data = dws.run_ok("minutes", "list", "mine", "--max", "1")
        items = _extract_items(data)
        if not items:
            pytest.skip("当前账号无听记数据")
        mid = _extract_id(items[0])
        if not mid:
            pytest.skip("听记项缺少 ID 字段")
        return mid

    def test_transcription_returns_pagination_info(self, dws, first_id):
        """get transcription 默认返回应包含分页信息结构。"""
        data = dws.run_ok("minutes", "get", "transcription", "--id", first_id)
        result = data.get("result", data)
        # 应包含 paragraphList/items 和可能的 hasNext/nextToken
        assert isinstance(result, dict)
        # 至少有某种列表字段
        has_list = any(
            isinstance(result.get(k), list)
            for k in ["paragraphList", "items", "paragraphs", "records"]
        )
        # 或者至少成功返回了数据
        assert has_list or len(result) > 0


class TestReportModuleBoundary:
    """Case 7: 验证 report 模块的命令边界（minutes 场景联动时高频踩坑）。

    对应 Golden Case: minutes-007 (P1)
    """

    def test_report_inbox_does_not_exist(self, dws):
        """'dws report inbox' 不存在，应报 unknown subcommand。"""
        result = dws.run_raw("report", "inbox", "--format", "json")
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "unknown" in combined.lower() or "error" in combined.lower()

    def test_report_list_without_start_reports_error(self, dws):
        """'dws report list' 不带 --start 应报 flag required。"""
        result = dws.run_raw("report", "list", "--format", "json")
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "required" in combined.lower() or "--start" in combined.lower()

    def test_report_list_with_start_and_end_accepted(self, dws):
        """'dws report list --start <iso> --end <iso>' 是合法写法（需同时传 start+end）。"""
        result = dws.run_raw(
            "report", "list",
            "--start", "2026-05-06T00:00:00+08:00",
            "--end", "2026-05-13T23:59:59+08:00",
            "--format", "json",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        # 不应报 unknown flag
        assert "unknown flag" not in combined.lower()
        # 不应报 "flag --xxx is required" 类错误（注意：成功返回的 JSON 数据中可能含 "required" 字样）
        assert "is required" not in combined.lower()


class TestEndToEndListGetWorkflow:
    """综合: 验证 list → get 完整工作流（覆盖 Case 1/3/6 的正确路径）。"""

    @pytest.fixture(scope="class")
    def valid_id(self, dws):
        """获取一个有效的听记 ID。"""
        data = dws.run_ok("minutes", "list", "mine", "--max", "1")
        items = _extract_items(data)
        if not items:
            pytest.skip("当前账号无听记数据")
        mid = _extract_id(items[0])
        if not mid:
            pytest.skip("听记项缺少 ID 字段")
        return mid

    def test_list_then_get_info(self, dws, valid_id):
        """list → get info 完整链路。"""
        data = dws.run_ok("minutes", "get", "info", "--id", valid_id)
        assert isinstance(data, dict) and len(data) > 0

    def test_list_then_get_summary(self, dws, valid_id):
        """list → get summary 完整链路。"""
        data = dws.run_ok("minutes", "get", "summary", "--id", valid_id)
        assert isinstance(data, dict) and len(data) > 0

    def test_list_then_get_transcription(self, dws, valid_id):
        """list → get transcription 完整链路。"""
        data = dws.run_ok("minutes", "get", "transcription", "--id", valid_id)
        assert isinstance(data, dict) and len(data) > 0


class TestSpeakerSummaryParams:
    """验证新增的 speaker summary create/get 子命令参数。

    对应 MCP tool: create_speaker_summary / get_speaker_summary
    """

    def test_speaker_summary_create_accepts_ids(self, dws):
        """speaker summary create --ids 为合法参数。"""
        result = dws.run_raw(
            "minutes", "speaker", "summary", "create",
            "--ids", "PLACEHOLDER_UUID",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()
        assert "unknown command" not in combined.lower()

    def test_speaker_summary_create_accepts_task_uuids(self, dws):
        """speaker summary create --task-uuids 为合法别名。"""
        result = dws.run_raw(
            "minutes", "speaker", "summary", "create",
            "--task-uuids", "PLACEHOLDER_UUID",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    def test_speaker_summary_create_requires_ids(self, dws):
        """speaker summary create 不传 --ids 应报 required。"""
        result = dws.run_raw(
            "minutes", "speaker", "summary", "create",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "required" in combined.lower()

    def test_speaker_summary_get_accepts_ids(self, dws):
        """speaker summary get --ids 为合法参数。"""
        result = dws.run_raw(
            "minutes", "speaker", "summary", "get",
            "--ids", "PLACEHOLDER_UUID",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()
        assert "unknown command" not in combined.lower()

    def test_speaker_summary_get_accepts_task_uuids(self, dws):
        """speaker summary get --task-uuids 为合法别名。"""
        result = dws.run_raw(
            "minutes", "speaker", "summary", "get",
            "--task-uuids", "PLACEHOLDER_UUID",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    def test_speaker_summary_get_requires_ids(self, dws):
        """speaker summary get 不传 --ids 应报 required。"""
        result = dws.run_raw(
            "minutes", "speaker", "summary", "get",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "required" in combined.lower()
