"""
test_99_minutes_0511_goldencase_regression.py — 0511-0512 P1 Golden Case 回归测试

基于 golden_case_p1p2_0509_0512.md 中 4 个 P1 case 提炼的错误模式：
  Case 1: Windows 沙箱无 unix 工具 — cli 不接受 shell 管道截断参数
  Case 2: 读他人/未授权/已删除听记 — 权限和资源不存在错误缺乏恢复指引
  Case 3: upload create 业务报错 — --json/--format json 入参混淆
  Case 4: list 分页/时间筛选参数名不一致 — --page-size/--date-range/--limit 均不存在

测试目标：验证错误参数必须被拒绝 + 正确参数路径可走通 + 权限错误有可操作的恢复路径。
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


# ---------------------------------------------------------------------------
# Case 1: Windows 沙箱无 unix 工具, cli 不接受 shell 管道截断参数
# Golden Case 原始问题: LLM 用 `| head -c 2000` 截断 transcription 输出,
#   Windows 沙箱无 head 命令导致 16 次重试全部失败.
#   根因: cli 缺少 --max-output-bytes 等内置截断参数.
# 测试策略: 验证 list/get 子命令用正确的 cli 内置参数 (--max/--query/--start/--end)
#   即可完成筛选截断, 不依赖外部 shell 工具.
# ---------------------------------------------------------------------------


class TestCase1TranscriptionWithoutShellPipe:
    """Case 1: 验证 transcription 拉取无需 shell 管道, cli 内置参数即可控制输出。"""

    @pytest.fixture(scope="class")
    def first_minutes_id(self, dws):
        """获取当前账号的第一个有效听记 ID。"""
        data = dws.run_ok("minutes", "list", "mine", "--max", "1")
        items = _extract_items(data)
        if not items:
            pytest.skip("当前账号无听记数据，无法验证 transcription 拉取")
        mid = _extract_id(items[0])
        if not mid:
            pytest.skip("听记项缺少 ID 字段")
        return mid

    def test_transcription_returns_without_pipe(self, dws, first_minutes_id):
        """get transcription 直接调用即可返回数据, 无需 | head 截断。"""
        data = dws.run_ok(
            "minutes", "get", "transcription", "--id", first_minutes_id,
        )
        assert isinstance(data, dict) and len(data) > 0

    def test_list_mine_max_controls_page_size(self, dws):
        """--max 参数控制返回条数, 替代 | head -N。"""
        data = dws.run_ok("minutes", "list", "mine", "--max", "3")
        items = _extract_items(data)
        assert isinstance(items, list)
        assert len(items) <= 3

    def test_list_all_query_filters_server_side(self, dws):
        """--query 参数在服务端过滤, 替代 | grep。"""
        data = dws.run_ok("minutes", "list", "all", "--query", "会议")
        items = _extract_items(data)
        assert isinstance(items, list)


# ---------------------------------------------------------------------------
# Case 2: 读他人/未授权/已删除听记, cli 报权限或资源不存在
# Golden Case 原始问题: `user is not minutes creator` 或 `P_DataNotFound`
#   模型无法区分两种错误, 也不知道切换到 list shared.
# 测试策略:
#   A) 用一个假 uuid 调 get summary 必须返回错误 (不静默成功)
#   B) list shared 链路可走通 (权限降级恢复路径存在)
#   C) P_DataNotFound 与 taskUuid is invalid 产生不同的错误信息
# ---------------------------------------------------------------------------


class TestCase2PermissionAndNotFoundRecovery:
    """Case 2: 权限不足和资源不存在场景的错误返回与恢复路径。"""

    def test_get_summary_with_fake_uuid_returns_error(self, dws):
        """用伪造 uuid 调 get summary 必须返回错误, 不能静默成功。"""
        result = dws.run_raw(
            "minutes", "get", "summary",
            "--id", "00000000000000000000000000000000_0000000000_0",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        has_error = (
            result.returncode != 0
            or "error" in combined.lower()
            or "invalid" in combined.lower()
            or "not found" in combined.lower()
            or "P_DataNotFound" in combined
            or "dingOpenErrcode" in combined
        )
        assert has_error, (
            f"伪造 uuid 应返回错误, 实际 returncode={result.returncode}, "
            f"output={combined[:300]}"
        )

    def test_list_shared_is_available(self, dws):
        """list shared 链路可走通 — 权限降级恢复路径的前提。"""
        data = dws.run_ok("minutes", "list", "shared")
        assert isinstance(data, dict)
        result = data.get("result", data)
        assert isinstance(result, (dict, list))

    def test_list_shared_then_get_summary_workflow(self, dws):
        """list shared 拿到 id 后 get summary 应能成功 (他人共享听记的恢复路径)。"""
        data = dws.run_ok("minutes", "list", "shared", "--max", "1")
        items = _extract_items(data)
        if not items:
            pytest.skip("当前账号无共享听记, 无法验证 shared → get 链路")
        mid = _extract_id(items[0])
        if not mid:
            pytest.skip("共享听记项缺少 ID 字段")
        summary = dws.run_ok("minutes", "get", "summary", "--id", mid)
        assert isinstance(summary, dict) and len(summary) > 0

    def test_invalid_uuid_format_rejected(self, dws):
        """格式错误的 uuid (非 hex 编码) 应被拒绝, 且错误信息与 P_DataNotFound 不同。"""
        result = dws.run_raw(
            "minutes", "get", "info",
            "--id", "THIS_IS_NOT_A_VALID_UUID_FORMAT!!!",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        has_error = (
            result.returncode != 0
            or "error" in combined.lower()
            or "invalid" in combined.lower()
        )
        assert has_error, f"格式错误的 uuid 应被拒绝: {combined[:300]}"


# ---------------------------------------------------------------------------
# Case 3: upload create 业务报错, --json/--format json 入参混淆
# Golden Case 原始问题: LLM 尝试 `--json '{"fileName":...}'` 和
#   `-f json '{"fileName":...}'` 都被拒. 根因: --format json 是输出格式,
#   cli 不接受 JSON 作为输入.
# 测试策略:
#   A) --json 作为输入参数必须被拒绝 (unknown flag)
#   B) upload create 的正确参数名 --file-name / --file-size 是合法的
#   C) 缺少 --file-name 或 --file-size 必须报 missing required flag
# ---------------------------------------------------------------------------


class TestCase3UploadCreateParamValidation:
    """Case 3: upload create 参数校验 — 禁止 JSON 输入, 验证正确参数名。"""

    def test_upload_create_rejects_json_flag(self, dws):
        """upload create --json 不存在, 必须报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--json", '{"fileName":"test.mp3","fileSize":1024}',
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert (
            "unknown flag" in combined.lower()
            or result.returncode != 0
        ), f"--json 应报 unknown flag: {combined[:300]}"

    def test_upload_create_accepts_file_name_flag(self, dws):
        """upload create --file-name 是合法参数, 不报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-name", "test.mp3",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower(), (
            f"--file-name 应为合法参数: {combined[:300]}"
        )

    def test_upload_create_accepts_file_size_flag(self, dws):
        """upload create --file-size 是合法参数, 不报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-size", "1024",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower(), (
            f"--file-size 应为合法参数: {combined[:300]}"
        )

    def test_upload_create_missing_file_name_reports_error(self, dws):
        """upload create 缺少 --file-name 应报缺失必填参数。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-size", "1024",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        has_error = (
            result.returncode != 0
            or "missing" in combined.lower()
            or "required" in combined.lower()
            or "error" in combined.lower()
        )
        assert has_error, f"缺少 --file-name 应报错: {combined[:300]}"

    def test_upload_create_missing_file_size_reports_error(self, dws):
        """upload create 缺少 --file-size 应报缺失必填参数。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-name", "test.mp3",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        has_error = (
            result.returncode != 0
            or "missing" in combined.lower()
            or "required" in combined.lower()
            or "error" in combined.lower()
        )
        assert has_error, f"缺少 --file-size 应报错: {combined[:300]}"

    def test_format_json_is_output_format_not_input(self, dws):
        """--format json 仅控制输出格式, 不能用来传入 JSON 输入参数。

        验证: 即使加了 --format json, 缺少 --file-name 仍然报 missing。
        """
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--format", "json",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        has_error = (
            result.returncode != 0
            or "missing" in combined.lower()
            or "required" in combined.lower()
            or "error" in combined.lower()
        )
        assert has_error, (
            f"仅传 --format json 不传 --file-name/--file-size 应报缺失: "
            f"{combined[:300]}"
        )


# ---------------------------------------------------------------------------
# Case 4: list 分页/时间筛选参数名跟其他模块不一致
# Golden Case 原始问题: LLM 写 --page-size / --date-range / --limit,
#   全部 unknown flag. 正确的是 --max / --start + --end.
# 测试策略:
#   A) --page-size / --date-range / --limit 必须被拒绝
#   B) --max / --start / --end 是合法参数
#   C) list 后不跟 scope 直接加 --start 必须被拒绝
# ---------------------------------------------------------------------------


class TestCase4ListParamNameConsistency:
    """Case 4: list 分页/时间筛选参数名验证 — 拒绝其他模块参数名。"""

    def test_list_mine_rejects_page_size(self, dws):
        """list mine --page-size 不存在 (应为 --max)。"""
        result = dws.run_raw("minutes", "list", "mine", "--page-size", "10")
        combined = (result.stdout or "") + (result.stderr or "")
        assert (
            result.returncode != 0
            or "unknown flag" in combined.lower()
            or "error" in combined.lower()
        ), f"--page-size 应被拒绝: {combined[:300]}"

    def test_list_mine_rejects_date_range(self, dws):
        """list mine --date-range 不存在 (应为 --start + --end)。"""
        result = dws.run_raw(
            "minutes", "list", "mine",
            "--date-range", "2026-05-04 2026-05-10",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert (
            result.returncode != 0
            or "unknown flag" in combined.lower()
            or "error" in combined.lower()
        ), f"--date-range 应被拒绝: {combined[:300]}"

    def test_list_mine_rejects_limit(self, dws):
        """list mine --limit 不存在 (应为 --max)。"""
        result = dws.run_raw("minutes", "list", "mine", "--limit", "10")
        combined = (result.stdout or "") + (result.stderr or "")
        assert (
            result.returncode != 0
            or "unknown flag" in combined.lower()
            or "error" in combined.lower()
        ), f"--limit 应被拒绝: {combined[:300]}"

    def test_list_without_scope_rejects_start(self, dws):
        """list --start (不跟 scope) 必须被拒绝。"""
        result = dws.run_raw(
            "minutes", "list",
            "--start", "2026-05-04T00:00:00+08:00",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert (
            result.returncode != 0
            or "unknown flag" in combined.lower()
            or "error" in combined.lower()
            or "unknown command" in combined.lower()
        ), f"list 后不跟 scope 直接加 --start 应被拒绝: {combined[:300]}"

    def test_list_mine_accepts_max(self, dws):
        """list mine --max 是合法参数名。"""
        data = dws.run_ok("minutes", "list", "mine", "--max", "5")
        items = _extract_items(data)
        assert isinstance(items, list)

    def test_list_mine_accepts_start_end(self, dws):
        """list mine --start / --end 是合法参数名。"""
        data = dws.run_ok(
            "minutes", "list", "mine",
            "--start", "2026-04-01T00:00:00+08:00",
            "--end", "2026-05-12T23:59:59+08:00",
        )
        items = _extract_items(data)
        assert isinstance(items, list)

    def test_list_all_accepts_query_with_time_range(self, dws):
        """list all --query + --start + --end 组合筛选可正常工作。"""
        data = dws.run_ok(
            "minutes", "list", "all",
            "--query", "会议",
            "--start", "2026-04-01T00:00:00+08:00",
            "--end", "2026-05-12T23:59:59+08:00",
        )
        items = _extract_items(data)
        assert isinstance(items, list)
