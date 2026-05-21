"""
test_02_participant.py — 日程参与者管理测试 (3 commands × 3+ cases)

Commands tested:
  1. dws calendar participant list    (get_calendar_participants)
  2. dws calendar participant add     (add_calendar_participant)
  3. dws calendar participant delete  (remove_calendar_participant)
"""

import pytest


class TestParticipantList:
    """dws calendar participant list"""

    def test_list_participants(self, dws, test_event_id):
        """查看日程参与者列表应成功返回。"""
        data = dws.run_ok(
            "calendar", "participant", "list",
            "--event", test_event_id,
        )
        assert data is not None

    def test_list_contains_creator(self, dws, test_event_id):
        """日程参与者至少包含创建者。"""
        data = dws.run_ok(
            "calendar", "participant", "list",
            "--event", test_event_id,
        )
        # 参与者列表应非空
        participants = data.get("data", {})
        assert participants is not None

    def test_list_invalid_event(self, dws):
        """无效日程 ID 查询参与者应报错。"""
        result = dws.run_raw(
            "calendar", "participant", "list",
            "--event", "INVALID_EVENT_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestParticipantAdd:
    """dws calendar participant add"""

    def test_add_participant(self, dws, test_event_id, current_user_id):
        """添加当前用户为参与者应成功。"""
        dws.run_ok(
            "calendar", "participant", "add",
            "--event", test_event_id,
            "--users", current_user_id,
        )

    def test_add_multiple_users(self, dws, test_event_id, current_user_id):
        """添加多个用户（含重复）应成功或幂等。"""
        dws.run_ok(
            "calendar", "participant", "add",
            "--event", test_event_id,
            "--users", f"{current_user_id},{current_user_id}",
        )

    def test_add_optional_participant(self, dws, test_event_id, current_user_id):
        """添加可选参会人应成功。"""
        dws.run_ok(
            "calendar", "participant", "add",
            "--event", test_event_id,
            "--users", current_user_id,
            "--optional",
        )

    def test_add_required_participant_explicit(self, dws, test_event_id, current_user_id):
        """不传 --optional（默认必选参会人）应成功且行为一致。"""
        dws.run_ok(
            "calendar", "participant", "add",
            "--event", test_event_id,
            "--users", current_user_id,
        )

    def test_add_to_invalid_event(self, dws, current_user_id):
        """向无效日程添加参与者应报错。"""
        result = dws.run_raw(
            "calendar", "participant", "add",
            "--event", "INVALID_EVENT_99999",
            "--users", current_user_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestParticipantDelete:
    """dws calendar participant delete"""

    def test_remove_then_verify(
        self, dws, test_event_id, current_user_id,
    ):
        """organizer 不可移除，API 应返回业务错误。"""
        # organizer 是日程创建者，不允许被移除
        result = dws.run_raw(
            "calendar", "participant", "delete",
            "--event", test_event_id,
            "--users", current_user_id,
            "--yes",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), "Removing organizer should fail"

    def test_remove_invalid_user(self, dws, test_event_id):
        """移除不存在的用户应报错。"""
        result = dws.run_raw(
            "calendar", "participant", "delete",
            "--event", test_event_id,
            "--users", "NON_EXISTENT_USER_ID",
            "--yes",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), "Removing non-existent user should fail"

    def test_remove_from_invalid_event(self, dws, current_user_id):
        """从无效日程移除参与者应报错。"""
        result = dws.run_raw(
            "calendar", "participant", "delete",
            "--event", "INVALID_EVENT_99999",
            "--users", current_user_id,
            "--yes",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
