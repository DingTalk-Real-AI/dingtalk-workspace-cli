"""
test_04_reminder.py — 待办提醒测试 (2 commands × 3+ cases)

Commands tested:
  1. dws todo task add-reminder    (add_todo_reminder)
  2. dws todo task reset-reminder  (reset_todo_reminder)

依赖：
  - test_task_id fixture (todo/conftest.py 提供，无截止时间)
  - test_task_id_with_due fixture (todo/conftest.py 提供，带截止时间)
  - dws / current_user_id (root conftest.py 提供)

注意：
  - add-reminder 有两种模式：
    - baseTime=dueTime: 基于截止时间偏移，待办必须有截止时间，需要 --due-date-offset
    - baseTime=customTime: 自定义提醒时间戳，需要 --reminder-time-stamp (ISO-8601)
  - reset-reminder 可选传 --reminder-rules (JSON 数组)，不传则清除所有提醒
"""

import json

from test_utils import iso8601_cn_offset


class TestAddReminderDueTime:
    """dws todo task add-reminder --base-time dueTime"""

    def test_add_reminder_due_time_basic(self, dws, test_task_id_with_due):
        """基于截止时间偏移添加提醒（提前 30 分钟）。"""
        data = dws.run_ok(
            "todo", "task", "add-reminder",
            "--task-id", test_task_id_with_due,
            "--base-time", "dueTime",
            "--due-date-offset", "-30",
        )
        result = data
        assert result.get("success") is True, (
            f"add-reminder (dueTime) 应返回 result.success=true: {data}"
        )

    def test_add_reminder_due_time_zero_offset(self, dws, test_task_id_with_due):
        """偏移量为 0（截止时刻提醒）。"""
        data = dws.run_ok(
            "todo", "task", "add-reminder",
            "--task-id", test_task_id_with_due,
            "--base-time", "dueTime",
            "--due-date-offset", "0",
        )
        result = data
        assert result.get("success") is True, (
            f"add-reminder (dueTime, offset=0) 应返回 result.success=true: {data}"
        )

    def test_add_reminder_due_time_invalid_task(self, dws):
        """无效 taskId 添加提醒应报错或返回失败。"""
        result = dws.run_raw(
            "todo", "task", "add-reminder",
            "--task-id", "INVALID_TASK_99999",
            "--base-time", "dueTime",
            "--due-date-offset", "-30",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
            or "false" in result.stdout.lower()
        )


class TestAddReminderCustomTime:
    """dws todo task add-reminder --base-time customTime"""

    def test_add_reminder_custom_time_basic(self, dws, test_task_id_with_due):
        """使用自定义时间戳添加提醒。"""
        reminder_time = iso8601_cn_offset(days=6, hours=23)
        data = dws.run_ok(
            "todo", "task", "add-reminder",
            "--task-id", test_task_id_with_due,
            "--base-time", "customTime",
            "--reminder-time-stamp", reminder_time,
        )
        result = data
        assert result.get("success") is True, (
            f"add-reminder (customTime) 应返回 result.success=true: {data}"
        )

    def test_add_reminder_custom_time_on_task_without_due(self, dws, test_task_id):
        """对无截止时间的待办使用 customTime 模式也应成功。"""
        reminder_time = iso8601_cn_offset(days=1)
        data = dws.run_ok(
            "todo", "task", "add-reminder",
            "--task-id", test_task_id,
            "--base-time", "customTime",
            "--reminder-time-stamp", reminder_time,
        )
        result = data
        assert result.get("success") is True, (
            f"add-reminder (customTime, no due) 应返回 result.success=true: {data}"
        )

    def test_add_reminder_custom_time_invalid_task(self, dws):
        """无效 taskId 添加自定义时间提醒应报错。"""
        reminder_time = iso8601_cn_offset(days=1)
        result = dws.run_raw(
            "todo", "task", "add-reminder",
            "--task-id", "INVALID_TASK_99999",
            "--base-time", "customTime",
            "--reminder-time-stamp", reminder_time,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
            or "false" in result.stdout.lower()
        )

    def test_add_reminder_missing_base_time(self, dws, test_task_id_with_due):
        """缺少 --base-time 应被 CLI 校验拒绝。"""
        result = dws.run_raw(
            "todo", "task", "add-reminder",
            "--task-id", test_task_id_with_due,
            "--due-date-offset", "-30",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), "missing --base-time should be rejected"


class TestResetReminder:
    """dws todo task reset-reminder"""

    def test_reset_reminder_clear_all(self, dws, test_task_id_with_due):
        """不传 --reminder-rules 则清除所有提醒。"""
        data = dws.run_ok(
            "todo", "task", "reset-reminder",
            "--task-id", test_task_id_with_due,
        )
        result = data
        assert result.get("success") is True, (
            f"reset-reminder (clear) 应返回 result.success=true: {data}"
        )

    def test_reset_reminder_with_due_time_rule(self, dws, test_task_id_with_due):
        """重置提醒并指定基于截止时间的规则。"""
        rules = json.dumps([{"dueDateOffset": -15, "baseTime": "dueTime"}])
        data = dws.run_ok(
            "todo", "task", "reset-reminder",
            "--task-id", test_task_id_with_due,
            "--reminder-rules", rules,
        )
        result = data
        assert result.get("success") is True, (
            f"reset-reminder (dueTime rule) 应返回 result.success=true: {data}"
        )

    def test_reset_reminder_with_custom_time_rule(self, dws, test_task_id_with_due):
        """重置提醒并指定自定义时间戳规则。"""
        reminder_time = iso8601_cn_offset(days=6)
        rules = json.dumps([
            {"reminderTimeStamp": reminder_time, "baseTime": "customTime"}
        ])
        data = dws.run_ok(
            "todo", "task", "reset-reminder",
            "--task-id", test_task_id_with_due,
            "--reminder-rules", rules,
        )
        result = data
        assert result.get("success") is True, (
            f"reset-reminder (customTime rule) 应返回 result.success=true: {data}"
        )

    def test_reset_reminder_with_mixed_rules(self, dws, test_task_id_with_due):
        """重置提醒并指定混合规则（dueTime + customTime）。"""
        reminder_time = iso8601_cn_offset(days=5)
        rules = json.dumps([
            {"dueDateOffset": -30, "baseTime": "dueTime"},
            {"reminderTimeStamp": reminder_time, "baseTime": "customTime"},
        ])
        data = dws.run_ok(
            "todo", "task", "reset-reminder",
            "--task-id", test_task_id_with_due,
            "--reminder-rules", rules,
        )
        result = data
        assert result.get("success") is True, (
            f"reset-reminder (mixed rules) 应返回 result.success=true: {data}"
        )

    def test_reset_reminder_invalid_task(self, dws):
        """无效 taskId 重置提醒应报错或返回失败。"""
        result = dws.run_raw(
            "todo", "task", "reset-reminder",
            "--task-id", "INVALID_TASK_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
            or "false" in result.stdout.lower()
        )

    def test_reset_reminder_missing_task_id(self, dws):
        """缺少 --task-id 应被 CLI 校验拒绝。"""
        result = dws.run_raw(
            "todo", "task", "reset-reminder",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), "missing --task-id should be rejected"
