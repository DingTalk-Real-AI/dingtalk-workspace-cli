"""
test_01_task.py — 待办任务测试 (6 commands × 3+ cases)

实际返回格式:
  todo task create: {"arguments":[], "result":{"subject","taskId"}, "success":true}
  todo task list:   {"result":{"todoCards":[{taskId,subject,...}]}}
  todo task get:    MCP 工具在 pre 环境未注册 (PARAM_ERROR)
  todo task update: {"result":{"success":true/false}}
  todo task done:   {"result":{"success":true/false}}
  todo task delete: {"result":{"success":true/false}}

注意:
  - list 使用 --status (非 --done)
  - get 在 pre 环境不可用
  - update/done/delete 无顶层 success 字段
"""

import time
import pytest

from test_utils import iso8601_cn_offset


class TestTaskCreate:
    """dws todo task create"""

    def test_create_returns_taskId(self, dws, current_user_id):
        """创建待办应返回 taskId。"""
        ts = int(time.time())
        data = dws.run(
            "todo", "task", "create",
            "--title", f"Strict_Create_{ts}",
            "--executors", current_user_id,
        )
        assert data.get("success") is True
        result = data["result"]
        assert isinstance(result, dict)
        assert "taskId" in result, f"result 缺少 taskId: {result}"
        assert "subject" in result
        assert result["subject"] == f"Strict_Create_{ts}"

    def test_create_with_priority(self, dws, current_user_id):
        """创建含优先级的待办。"""
        data = dws.run(
            "todo", "task", "create",
            "--title", f"Priority_{int(time.time())}",
            "--executors", current_user_id,
            "--priority", "40",
        )
        assert data.get("success") is True
        assert "taskId" in data["result"]

    def test_create_with_due(self, dws, current_user_id):
        """创建含截止时间的待办（--due 为 ISO-8601）。"""
        due = iso8601_cn_offset(days=1)
        data = dws.run(
            "todo", "task", "create",
            "--title", f"Due_{int(time.time())}",
            "--executors", current_user_id,
            "--due", due,
        )
        assert data.get("success") is True
        assert "taskId" in data["result"]


class TestTaskList:
    """dws todo task list — 返回 {"result":{"todoCards":[...]}}"""

    def test_list_returns_todoCards(self, dws):
        """列出待办应返回 todoCards 列表。"""
        data = dws.run_ok("todo", "task", "list")
        result = data.get("result", {})
        assert isinstance(result, dict)
        assert "todoCards" in result, f"result 缺少 todoCards: {result.keys()}"
        assert isinstance(result["todoCards"], list)

    def test_list_todoCards_have_fields(self, dws):
        """todoCards 每项应包含 taskId 和 subject。"""
        data = dws.run_ok("todo", "task", "list")
        cards = data["result"]["todoCards"]
        if len(cards) > 0:
            card = cards[0]
            assert "taskId" in card, f"todoCard 缺少 taskId: {card.keys()}"
            assert "subject" in card

    def test_list_with_status_filter(self, dws):
        """列出未完成待办（--status false）。"""
        data = dws.run_ok(
            "todo", "task", "list",
            "--status", "false",
        )
        result = data.get("result", {})
        assert "todoCards" in result


class TestTaskGet:
    """dws todo task get"""

    def test_get_returns_detail(self, dws, test_task_id):
        """获取待办详情。"""
        data = dws.run(
            "todo", "task", "get",
            "--task-id", test_task_id,
        )
        result = data.get("result", {})
        assert isinstance(result, dict)
        assert "todoDetailModel" in result, f"result 缺少 todoDetailModel: {result.keys()}"

    def test_get_contains_subject(self, dws, test_task_id):
        """待办详情应包含 subject 字段。"""
        data = dws.run(
            "todo", "task", "get",
            "--task-id", test_task_id,
        )
        detail = data.get("result", {}).get("todoDetailModel", {})
        assert "subject" in detail, f"todoDetailModel 缺少 subject: {detail.keys()}"

    def test_get_invalid_id(self, dws):
        """获取无效 taskId 应报错。"""
        result = dws.run_raw(
            "todo", "task", "get",
            "--task-id", "INVALID_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stderr.lower()
        )


class TestTaskUpdate:
    """dws todo task update — 返回 {"result":{"success":true}}"""

    def test_update_title(self, dws, test_task_id):
        """修改待办标题应返回 result.success=true。"""
        new_title = f"Updated_{int(time.time())}"
        data = dws.run_ok(
            "todo", "task", "update",
            "--task-id", test_task_id,
            "--title", new_title,
        )
        result = data.get("result", {})
        assert result.get("success") is True, (
            f"update 应返回 result.success=true: {data}"
        )

    def test_update_priority(self, dws, test_task_id):
        """修改待办优先级。"""
        data = dws.run_ok(
            "todo", "task", "update",
            "--task-id", test_task_id,
            "--priority", "60",
        )
        result = data.get("result", {})
        assert result.get("success") is True

    def test_update_invalid_id(self, dws):
        """修改无效 taskId 应报错或返回 false。"""
        result = dws.run_raw(
            "todo", "task", "update",
            "--task-id", "INVALID_99999",
            "--title", "nope",
        )
        assert (
            result.returncode != 0
            or "error" in result.stderr.lower()
            or "false" in result.stdout.lower()
        )


class TestTaskDone:
    """dws todo task done — 返回 {"result":{"success":true}}"""

    def test_mark_done_and_undone(self, dws, test_task_id):
        """标记完成并恢复未完成。"""
        d1 = dws.run_ok(
            "todo", "task", "done",
            "--task-id", test_task_id, "--status", "true",
        )
        assert d1.get("result", {}).get("success") is True

        d2 = dws.run_ok(
            "todo", "task", "done",
            "--task-id", test_task_id, "--status", "false",
        )
        assert d2.get("result", {}).get("success") is True

    def test_done_invalid_task(self, dws):
        """完成无效 task 应返回 result.success=false。"""
        data = dws.run_ok(
            "todo", "task", "done",
            "--task-id", "INVALID_99999",
            "--status", "true",
        )
        result = data.get("result", {})
        assert result.get("success") is False, (
            f"无效 task done 应返回 result.success=false: {result}"
        )
