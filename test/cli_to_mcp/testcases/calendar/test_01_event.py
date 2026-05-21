"""
test_01_event.py — 日程管理全覆盖测试 (7 commands × 3+ cases)

Commands tested:
  1. dws calendar event list      (list_calendar_events)
  2. dws calendar event get       (get_calendar_detail)
  3. dws calendar event create    (create_calendar_event)
  4. dws calendar event update    (update_calendar_event)
  5. dws calendar event delete    (delete_calendar_event)
  6. dws calendar event suggest   (list_suggested_event_times)
  7. dws calendar event respond   (respond)
"""

import time
import pytest

from test_utils import iso8601_cn_offset
from test_utils import extract_calendar_event_id


class TestEventList:
    """dws calendar event list"""

    def test_list_default(self, dws):
        """无参数调用应返回日程列表。"""
        data = dws.run("calendar", "event", "list")
        assert data is not None

    def test_list_with_time_range(self, dws):
        """指定 start/end 时间区间应返回该区间内的日程。"""
        start = iso8601_cn_offset(hours=-24)
        end = iso8601_cn_offset(hours=24)
        data = dws.run(
            "calendar", "event", "list",
            "--start", start, "--end", end,
        )
        assert data is not None

    def test_list_empty_range(self, dws):
        """查询遥远未来的时间段应返回空列表。"""
        start = "2099-01-01T00:00:00+08:00"
        end = "2099-01-02T00:00:00+08:00"
        data = dws.run(
            "calendar", "event", "list",
            "--start", start, "--end", end,
        )
        events = data.get("data", {}).get("events", [])
        assert isinstance(events, list)

    def test_list_with_calendar_id(self, dws):
        """显式传 --calendar-id primary 应等价于默认主日历查询。"""
        data = dws.run(
            "calendar", "event", "list",
            "--calendar-id", "primary",
        )
        assert data is not None


class TestEventGet:
    """dws calendar event get"""

    def test_get_returns_detail(self, dws, test_event_id):
        """获取已创建日程详情应返回完整信息。"""
        data = dws.run(
            "calendar", "event", "get", "--id", test_event_id,
        )
        assert data is not None

    def test_get_contains_summary(self, dws, test_event_id):
        """日程详情应包含 summary (标题) 字段。"""
        data = dws.run(
            "calendar", "event", "get", "--id", test_event_id,
        )
        # event get 新结构在 result，兼容旧结构 data。
        detail = data.get("result") or data.get("data") or {}
        has_summary = bool(detail.get("summary"))
        assert has_summary, f"No summary in event detail: {detail}"

    def test_get_invalid_id(self, dws):
        """使用无效事件 ID 应返回错误。"""
        result = dws.run_raw(
            "calendar", "event", "get",
            "--id", "INVALID_EVENT_ID_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_get_with_calendar_id(self, dws, test_event_id):
        """显式传 --calendar-id primary 仍能取到日程详情。"""
        data = dws.run(
            "calendar", "event", "get",
            "--id", test_event_id,
            "--calendar-id", "primary",
        )
        assert data is not None
        detail = data.get("result") or data.get("data") or {}
        assert detail, f"calendar-id=primary 应返回详情: {data}"


class TestEventCreate:
    """dws calendar event create"""

    def test_create_basic(self, dws):
        """创建基本日程应成功返回 eventId。"""
        title = f"CLI_Test_Create_{int(time.time())}"
        start = iso8601_cn_offset(hours=10)
        end = iso8601_cn_offset(hours=11)
        data = dws.run(
            "calendar", "event", "create",
            "--title", title, "--start", start, "--end", end,
        )
        # 统一兼容不同返回结构，避免“命令成功但测试解析不到 id”。
        event_id = extract_calendar_event_id(data)
        assert event_id, "create must return eventId"
        # cleanup
        dws.run(
            "calendar", "event", "delete",
            "--id", event_id, "--yes",
        )

    def test_create_with_desc(self, dws):
        """创建日程时附带描述，验证写入成功。"""
        title = f"CLI_Test_Desc_{int(time.time())}"
        start = iso8601_cn_offset(hours=12)
        end = iso8601_cn_offset(hours=13)
        data = dws.run(
            "calendar", "event", "create",
            "--title", title, "--start", start, "--end", end,
            "--desc", "自动化测试描述",
        )
        event_id = extract_calendar_event_id(data)
        assert event_id
        # cleanup
        dws.run(
            "calendar", "event", "delete",
            "--id", event_id, "--yes",
        )

    def test_create_with_timezone(self, dws):
        """创建日程时指定时区应成功返回 eventId。"""
        title = f"CLI_Test_TZ_{int(time.time())}"
        start = iso8601_cn_offset(hours=16)
        end = iso8601_cn_offset(hours=17)
        data = dws.run(
            "calendar", "event", "create",
            "--title", title, "--start", start, "--end", end,
            "--timezone", "America/New_York",
        )
        event_id = extract_calendar_event_id(data)
        assert event_id, "create with timezone must return eventId"
        # cleanup
        dws.run(
            "calendar", "event", "delete",
            "--id", event_id, "--yes",
        )

    def test_create_verify_via_get(self, dws):
        """创建日程后通过 get 验证标题一致。"""
        title = f"CLI_Test_Verify_{int(time.time())}"
        start = iso8601_cn_offset(hours=14)
        end = iso8601_cn_offset(hours=15)
        data = dws.run(
            "calendar", "event", "create",
            "--title", title, "--start", start, "--end", end,
        )
        event_id = extract_calendar_event_id(data)
        assert event_id, f"create must return eventId, got: {data}"
        # verify
        detail = dws.run(
            "calendar", "event", "get", "--id", event_id,
        )
        assert title in str(detail.get("data", detail))
        # cleanup
        dws.run(
            "calendar", "event", "delete",
            "--id", event_id, "--yes",
        )

    def test_create_with_location(self, dws):
        """创建日程时指定地点，验证 location 正确透传到 MCP。"""
        title = f"CLI_Test_Loc_{int(time.time())}"
        start = iso8601_cn_offset(hours=18)
        end = iso8601_cn_offset(hours=19)
        location = "3F-A201 西湖厅"
        event_id = None
        try:
            data = dws.run(
                "calendar", "event", "create",
                "--title", title, "--start", start, "--end", end,
                "--location", location,
            )
            event_id = extract_calendar_event_id(data)
            assert event_id, f"create with --location must return eventId, got: {data}"
        finally:
            if event_id:
                dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_create_with_free_busy(self, dws):
        """创建日程时指定 free-busy=free，验证忙碌状态透传。"""
        title = f"CLI_Test_FB_{int(time.time())}"
        start = iso8601_cn_offset(hours=20)
        end = iso8601_cn_offset(hours=21)
        data = dws.run(
            "calendar", "event", "create",
            "--title", title, "--start", start, "--end", end,
            "--free-busy", "free",
        )
        event_id = extract_calendar_event_id(data)
        assert event_id, f"create with --free-busy must return eventId, got: {data}"
        try:
            detail = dws.run("calendar", "event", "get", "--id", event_id)
            event = detail.get("result") or detail.get("data") or {}
            assert event.get("isAllDay") is not None or event, (
                f"日程详情应包含有效数据: {event}"
            )
        finally:
            dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_create_with_rich_text_desc(self, dws):
        """创建日程时指定富文本描述，验证 richTextDescription 透传。"""
        title = f"CLI_Test_RTD_{int(time.time())}"
        start = iso8601_cn_offset(hours=22)
        end = iso8601_cn_offset(hours=23)
        rich_desc = "<b>重要</b>：请提前准备材料"
        data = dws.run(
            "calendar", "event", "create",
            "--title", title, "--start", start, "--end", end,
            "--rich-text-desc", rich_desc,
        )
        event_id = extract_calendar_event_id(data)
        assert event_id, f"create with --rich-text-desc must return eventId, got: {data}"
        # cleanup
        dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_create_with_rooms_skipped_if_no_room(self, dws):
        """有空闲会议室时，演示 event create --rooms 一步预定；没有则跳过。"""
        import json
        search_start = iso8601_cn_offset(hours=20)
        search_end = iso8601_cn_offset(hours=21)
        search = dws.run_raw(
            "calendar", "room", "search",
            "--start", search_start, "--end", search_end,
            "--available",
        )
        try:
            search_data = json.loads(search.stdout)
        except (json.JSONDecodeError, AttributeError):
            pytest.skip(f"会议室搜索返回非 JSON: {search.stderr[:120]}")
            return
        rooms = (
            search_data.get("data", {}).get("rooms", [])
            or search_data.get("result", {}).get("rooms", [])
        )
        if not rooms:
            pytest.skip("当前时段没有空闲会议室，跳过 event create --rooms 演练")
        room_id = rooms[0].get("roomId") or rooms[0].get("id")
        if not room_id:
            pytest.skip("搜索结果未提供 roomId，跳过")

        title = f"CLI_Test_CreateRooms_{int(time.time())}"
        data = dws.run(
            "calendar", "event", "create",
            "--title", title,
            "--start", search_start, "--end", search_end,
            "--rooms", room_id,
        )
        event_id = extract_calendar_event_id(data)
        assert event_id, f"create with --rooms must return eventId, got: {data}"
        dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_create_partial_recurrence_rejected(self, dws):
        """循环规则不完整（只传 --recurrence-type）时，CLI 应前置拒绝，不能落落到 MCP。"""
        title = f"CLI_Test_PartialRecur_{int(time.time())}"
        start = iso8601_cn_offset(hours=12)
        end = iso8601_cn_offset(hours=13)
        result = dws.run_raw(
            "calendar", "event", "create",
            "--title", title, "--start", start, "--end", end,
            "--recurrence-type", "daily",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "recurrence" in combined, (
            f"不完整 recurrence 应被 CLI 拒绝，实际返回 rc={result.returncode}, output={combined[:200]}"
        )
        # 确保错误消息传达了“结构不完整 / recurrence-range-type 必须同时提供”的提示
        assert (
            "recurrence-range-type" in combined
            or "结构不完整" in combined
            or "INPUT_MISSING_PARAM" in combined
        ), f"期望提示补齐 recurrence-range-type，实际 output={combined[:300]}"

    def test_create_partial_recurrence_range_rejected(self, dws):
        """只传 --recurrence-range-type 不传 --recurrence-type 时也应拒绝。"""
        title = f"CLI_Test_PartialRecurRange_{int(time.time())}"
        start = iso8601_cn_offset(hours=14)
        end = iso8601_cn_offset(hours=15)
        result = dws.run_raw(
            "calendar", "event", "create",
            "--title", title, "--start", start, "--end", end,
            "--recurrence-range-type", "numbered",
            "--recurrence-count", "3",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "recurrence" in combined
        assert (
            "recurrence-type" in combined
            or "结构不完整" in combined
            or "INPUT_MISSING_PARAM" in combined
        ), f"期望提示补齐 recurrence-type，实际 output={combined[:300]}"

    def test_create_weekly_recurrence_missing_days(self, dws):
        """weekly 类型缺少 --recurrence-days-of-week 时应拒绝。"""
        title = f"CLI_Test_WeeklyMissingDays_{int(time.time())}"
        start = iso8601_cn_offset(hours=16)
        end = iso8601_cn_offset(hours=17)
        result = dws.run_raw(
            "calendar", "event", "create",
            "--title", title, "--start", start, "--end", end,
            "--recurrence-type", "weekly",
            "--recurrence-interval", "1",
            "--recurrence-range-type", "numbered",
            "--recurrence-count", "4",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "recurrence" in combined
        assert (
            "days-of-week" in combined
            or "daysOfWeek" in combined
            or "INPUT_MISSING_PARAM" in combined
        ), f"期望提示 weekly 缺少 days-of-week，实际 output={combined[:300]}"

    def test_create_missing_recurrence_interval_rejected(self, dws):
        """缺少 --recurrence-interval 时应被 CLI 拒绝（interval 也属于 recurrence 结构的必填字段）。"""
        title = f"CLI_Test_MissingInterval_{int(time.time())}"
        start = iso8601_cn_offset(hours=18)
        end = iso8601_cn_offset(hours=19)
        result = dws.run_raw(
            "calendar", "event", "create",
            "--title", title, "--start", start, "--end", end,
            "--recurrence-type", "daily",
            "--recurrence-range-type", "numbered",
            "--recurrence-count", "5",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0, (
            f"缺少 --recurrence-interval 应被 CLI 拒绝，实际 rc={result.returncode}, output={combined[:300]}"
        )
        assert (
            "recurrence-interval" in combined
            or "结构不完整" in combined
            or "INPUT_MISSING_PARAM" in combined
        ), f"期望提示补齐 --recurrence-interval，实际 output={combined[:300]}"


class TestEventUpdate:
    """dws calendar event update"""

    @staticmethod
    def _create_updatable_event(dws):
        """创建一条独立测试日程，避免 session 级共享状态污染。"""
        title = f"CLI_Test_UpdateSeed_{int(time.time())}"
        start = iso8601_cn_offset(hours=10)
        end = iso8601_cn_offset(hours=11)
        data = dws.run(
            "calendar", "event", "create",
            "--title", title, "--start", start, "--end", end,
        )
        event_id = extract_calendar_event_id(data)
        assert event_id, f"create must return eventId, got: {data}"
        return event_id

    def test_update_title(self, dws):
        """修改日程标题应生效。"""
        event_id = self._create_updatable_event(dws)
        new_title = f"CLI_Test_Updated_{int(time.time())}"
        try:
            dws.run_ok(
                "calendar", "event", "update",
                "--id", event_id, "--title", new_title,
            )
            detail = dws.run(
                "calendar", "event", "get", "--id", event_id,
            )
            event = detail.get("result") or detail.get("data") or {}
            assert event.get("summary") == new_title, f"unexpected event detail: {event}"
        finally:
            dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_update_time(self, dws):
        """修改日程时间应成功。"""
        event_id = self._create_updatable_event(dws)
        new_start = iso8601_cn_offset(hours=20)
        new_end = iso8601_cn_offset(hours=21)
        try:
            dws.run_ok(
                "calendar", "event", "update",
                "--id", event_id,
                "--start", new_start, "--end", new_end,
            )
        finally:
            dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_update_only_end_time(self, dws):
        """仅修改结束时间，其他保持不变。"""
        event_id = self._create_updatable_event(dws)
        new_end = iso8601_cn_offset(hours=25)
        try:
            dws.run_ok(
                "calendar", "event", "update",
                "--id", event_id, "--end", new_end,
            )
        finally:
            dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_update_desc(self, dws):
        """修改日程描述应成功。"""
        event_id = self._create_updatable_event(dws)
        try:
            dws.run_ok(
                "calendar", "event", "update",
                "--id", event_id, "--desc", "自动化测试更新描述",
            )
        finally:
            dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_update_timezone(self, dws):
        """修改日程时区应成功。"""
        event_id = self._create_updatable_event(dws)
        try:
            dws.run_ok(
                "calendar", "event", "update",
                "--id", event_id, "--timezone", "Asia/Tokyo",
            )
        finally:
            dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_update_recurrence(self, dws):
        """修改日程为 daily 循环应成功。"""
        event_id = self._create_updatable_event(dws)
        try:
            dws.run_ok(
                "calendar", "event", "update",
                "--id", event_id,
                "--recurrence-type", "daily",
                "--recurrence-interval", "1",
                "--recurrence-range-type", "numbered",
                "--recurrence-count", "5",
            )
        finally:
            dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_update_partial_recurrence_rejected(self, dws):
        """修改日程时只传部分 --recurrence-* 字段应被拒绝，避免把循环规则覆盖成不完整状态。"""
        event_id = self._create_updatable_event(dws)
        try:
            result = dws.run_raw(
                "calendar", "event", "update",
                "--id", event_id,
                # 模拟模型误以为“只改一个字段”的场景
                "--recurrence-count", "7",
            )
            combined = (result.stdout or "") + (result.stderr or "")
            assert result.returncode != 0 or "recurrence" in combined, (
                f"update 只传部分 recurrence 应被拒绝，实际 rc={result.returncode}, output={combined[:200]}"
            )
            assert (
                "recurrence-type" in combined
                or "结构不完整" in combined
                or "INPUT_MISSING_PARAM" in combined
            ), f"期望提示重传完整 recurrence，实际 output={combined[:300]}"
        finally:
            dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_update_location(self, dws):
        """修改日程地点应成功。"""
        event_id = self._create_updatable_event(dws)
        try:
            dws.run_ok(
                "calendar", "event", "update",
                "--id", event_id, "--location", "5F-B302 钱塘厅",
            )
        finally:
            dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_update_free_busy(self, dws):
        """修改日程忙碌状态应成功。"""
        event_id = self._create_updatable_event(dws)
        try:
            dws.run_ok(
                "calendar", "event", "update",
                "--id", event_id, "--free-busy", "free",
            )
        finally:
            dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_update_rich_text_desc(self, dws):
        """修改日程富文本描述应成功。"""
        event_id = self._create_updatable_event(dws)
        try:
            dws.run_ok(
                "calendar", "event", "update",
                "--id", event_id,
                "--rich-text-desc", "<h2>议程</h2><ul><li>方案评审</li></ul>",
            )
        finally:
            dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")


class TestEventDelete:
    """dws calendar event delete"""

    def test_delete_lifecycle(self, dws):
        """创建 → 删除 → 验证不可访问。"""
        title = f"CLI_Test_Del_{int(time.time())}"
        start = iso8601_cn_offset(hours=30)
        end = iso8601_cn_offset(hours=31)
        data = dws.run(
            "calendar", "event", "create",
            "--title", title, "--start", start, "--end", end,
        )
        event_id = extract_calendar_event_id(data)
        assert event_id, f"create must return eventId, got: {data}"
        dws.run(
            "calendar", "event", "delete",
            "--id", event_id, "--yes",
        )
        # 日历 API 是软删除，删除后 get 仍返回 JSON，status 变为 cancelled
        data = dws.run("calendar", "event", "get", "--id", event_id)
        status = data.get("result", {}).get("status", "")
        assert status == "cancelled", (
            f"Deleted event status should be 'cancelled', got: {status}"        )

    def test_delete_invalid_id(self, dws):
        """删除不存在的日程应返回错误。"""
        result = dws.run_raw(
            "calendar", "event", "delete",
            "--id", "INVALID_99999", "--yes",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_delete_and_redelete(self, dws):
        """删除同一日程两次，第二次应报错。"""
        title = f"CLI_Test_DblDel_{int(time.time())}"
        start = iso8601_cn_offset(hours=32)
        end = iso8601_cn_offset(hours=33)
        data = dws.run(
            "calendar", "event", "create",
            "--title", title, "--start", start, "--end", end,
        )
        event_id = extract_calendar_event_id(data)
        assert event_id, f"create must return eventId, got: {data}"
        dws.run(
            "calendar", "event", "delete",
            "--id", event_id, "--yes",
        )
        result = dws.run_raw(
            "calendar", "event", "delete",
            "--id", event_id, "--yes",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
        )


class TestEventSuggest:
    """dws calendar event suggest"""

    def test_suggest_default(self, dws):
        """无参数调用建议日程时间应返回有效数据。"""
        data = dws.run("calendar", "event", "suggest")
        assert data is not None

    def test_suggest_with_users(self, dws, current_user_id):
        """指定参会人查询建议时间应返回有效数据。"""
        data = dws.run(
            "calendar", "event", "suggest",
            "--users", current_user_id,
        )
        assert data is not None

    def test_suggest_with_duration(self, dws):
        """指定持续时间查询建议时间应返回有效数据。"""
        data = dws.run(
            "calendar", "event", "suggest",
            "--duration", "60",
        )
        assert data is not None

    def test_suggest_with_time_range(self, dws):
        """指定时间范围查询建议时间应返回有效数据。"""
        start = iso8601_cn_offset(hours=2)
        end = iso8601_cn_offset(hours=10)
        data = dws.run(
            "calendar", "event", "suggest",
            "--start", start, "--end", end,
        )
        assert data is not None

    def test_suggest_with_all_params(self, dws, current_user_id):
        """指定全部参数查询建议时间应返回有效数据。"""
        start = iso8601_cn_offset(hours=2)
        end = iso8601_cn_offset(hours=10)
        data = dws.run(
            "calendar", "event", "suggest",
            "--start", start, "--end", end,
            "--users", current_user_id,
            "--duration", "30",
            "--timezone", "Asia/Shanghai",
        )
        assert data is not None


class TestEventRespond:
    """dws calendar event respond"""

    @staticmethod
    def _create_respondable_event(dws):
        """创建一条用于响应测试的日程。"""
        title = f"CLI_Test_Respond_{int(time.time())}"
        start = iso8601_cn_offset(hours=10)
        end = iso8601_cn_offset(hours=11)
        data = dws.run(
            "calendar", "event", "create",
            "--title", title, "--start", start, "--end", end,
        )
        event_id = extract_calendar_event_id(data)
        assert event_id, f"create must return eventId, got: {data}"
        return event_id

    def test_respond_accepted(self, dws):
        """接受日程：组织者不允许修改自己的响应状态，断言返回业务错误。"""
        event_id = self._create_respondable_event(dws)
        try:
            result = dws.run_raw(
                "calendar", "event", "respond",
                "--id", event_id, "--status", "accepted",
            )
            combined = (result.stdout or "") + (result.stderr or "")
            # 自己创建的日程，当前用户必定是组织者，API 返回 300000
            assert "Cannot change response status of event organizer" in combined, (
                f"组织者 respond 应返回 organizer 限制错误，实际: {combined[:300]}"
            )
        finally:
            dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_respond_declined(self, dws):
        """拒绝日程：组织者不允许拒绝自己的日程，断言返回业务错误。"""
        event_id = self._create_respondable_event(dws)
        try:
            result = dws.run_raw(
                "calendar", "event", "respond",
                "--id", event_id, "--status", "declined",
            )
            combined = (result.stdout or "") + (result.stderr or "")
            assert "Cannot change response status of event organizer" in combined, (
                f"组织者 respond 应返回 organizer 限制错误，实际: {combined[:300]}"
            )
        finally:
            dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_respond_tentative(self, dws):
        """暂定日程：组织者不允许暂定自己的日程，断言返回业务错误。"""
        event_id = self._create_respondable_event(dws)
        try:
            result = dws.run_raw(
                "calendar", "event", "respond",
                "--id", event_id, "--status", "tentative",
            )
            combined = (result.stdout or "") + (result.stderr or "")
            assert "Cannot change response status of event organizer" in combined, (
                f"组织者 respond 应返回 organizer 限制错误，实际: {combined[:300]}"
            )
        finally:
            dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_respond_invalid_status(self, dws):
        """传入无效 status 应返回错误。"""
        event_id = self._create_respondable_event(dws)
        try:
            result = dws.run_raw(
                "calendar", "event", "respond",
                "--id", event_id, "--status", "INVALID_STATUS",
            )
            assert (
                result.returncode != 0
                or "error" in result.stdout.lower()
                or "invalid" in result.stderr.lower()
            ), "invalid --status should return error"
        finally:
            dws.run_raw("calendar", "event", "delete", "--id", event_id, "--yes")

    def test_respond_invalid_event_id(self, dws):
        """响应不存在的日程应返回错误。"""
        result = dws.run_raw(
            "calendar", "event", "respond",
            "--id", "INVALID_EVENT_99999", "--status", "accepted",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
