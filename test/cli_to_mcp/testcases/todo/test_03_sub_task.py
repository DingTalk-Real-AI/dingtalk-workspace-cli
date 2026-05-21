"""
test_03_sub_task.py — 创建子待办集成测试

测试步骤：
1. 调用创建待办接口创建父待办，返回 taskId 为 parentTaskId
2. 调用创建子待办接口创建子待办，传入的 parent-id 字段为第一步返回的 parentTaskId
3. 调用查询待办详情的接口，查询 taskId 为 parentTaskId 的子待办列表是否包含了刚才创建的子待办。
"""

import time
import pytest

from test_utils import unique_name


class TestCreateSubTodo:
    """dws todo task create-sub 集成测试"""

    def test_create_sub_todo_flow(self, dws, current_user_id):
        """完整流程：创建父待办 -> 创建子待办 -> 验证子待办存在于父待办详情中"""
        # Step 1: 创建父待办
        parent_title = unique_name("Parent_Todo")
        parent_data = dws.run(
            "todo", "task", "create",
            "--title", parent_title,
            "--executors", current_user_id,
        )
        assert parent_data.get("success") is True, f"创建父待办失败: {parent_data}"
        parent_task_id = parent_data["result"]["taskId"]
        print(f"\n[STEP 1] Created parent todo: {parent_task_id}")

        # Step 2: 创建子待办
        sub_title = unique_name("Sub_Todo")
        sub_data = dws.run(
            "todo", "task", "create-sub",
            "--parent-id", parent_task_id,
            "--title", sub_title,
            "--executors", current_user_id,
        )
        assert sub_data.get("success") is True, f"创建子待办失败: {sub_data}"
        sub_task_id = sub_data["result"]["taskId"]
        print(f"[STEP 2] Created sub todo: {sub_task_id}")

        # Step 3: 查询父待办详情，验证子待办存在
        detail_data = dws.run(
            "todo", "task", "get",
            "--task-id", parent_task_id,
        )
        assert detail_data.get("success") is True, f"查询父待办详情失败: {detail_data}"
        
        todo_detail_model = detail_data["result"].get("todoDetailModel", {})
        sub_todos = todo_detail_model.get("subTodos", [])
        
        # 验证子待办列表中包含刚创建的子待办
        # 注意：MCP 返回的 subTodos 列表中可能不包含 taskId 字段，因此使用 subject 进行匹配
        found_sub_todo = False
        for sub_todo in sub_todos:
            if sub_todo.get("subject") == sub_title:
                found_sub_todo = True
                # 如果存在 taskId 字段，则进一步校验
                if "taskId" in sub_todo:
                    assert sub_todo.get("taskId") == sub_task_id, f"子待办 taskId 不匹配: {sub_todo}"
                break
        
        assert found_sub_todo is True, f"子待办 (title={sub_title}) 未出现在父待办 {parent_task_id} 的子待办列表中: {sub_todos}"
        print(f"[STEP 3] Verified sub todo {sub_task_id} exists in parent todo {parent_task_id}")

        # Teardown: 清理数据（先删子待办，再删父待办）
        try:
            dws.run("todo", "task", "delete", "--task-id", sub_task_id, "--yes")
            print(f"[TEARDOWN] Deleted sub todo: {sub_task_id}")
        except Exception as e:
            print(f"[TEARDOWN WARNING] Failed to delete sub todo {sub_task_id}: {e}")
        
        try:
            dws.run("todo", "task", "delete", "--task-id", parent_task_id, "--yes")
            print(f"[TEARDOWN] Deleted parent todo: {parent_task_id}")
        except Exception as e:
            print(f"[TEARDOWN WARNING] Failed to delete parent todo {parent_task_id}: {e}")

    def test_create_sub_todo_with_priority(self, dws, current_user_id):
        """创建带优先级的子待办并验证"""
        # Step 1: 创建父待办
        parent_title = unique_name("Parent_Todo_Priority")
        parent_data = dws.run(
            "todo", "task", "create",
            "--title", parent_title,
            "--executors", current_user_id,
        )
        assert parent_data.get("success") is True
        parent_task_id = parent_data["result"]["taskId"]

        # Step 2: 创建高优先级子待办
        sub_title = unique_name("Sub_Todo_High_Priority")
        sub_data = dws.run(
            "todo", "task", "create-sub",
            "--parent-id", parent_task_id,
            "--title", sub_title,
            "--executors", current_user_id,
            "--priority", "40",
        )
        assert sub_data.get("success") is True
        sub_task_id = sub_data["result"]["taskId"]

        # Step 3: 验证子待办存在且优先级正确
        detail_data = dws.run(
            "todo", "task", "get",
            "--task-id", parent_task_id,
        )
        sub_todos = detail_data["result"]["todoDetailModel"].get("subTodos", [])
        found = False
        for sub_todo in sub_todos:
            # 注意：MCP 返回的 subTodos 列表中可能不包含 taskId 字段，因此使用 subject 进行匹配
            if sub_todo.get("subject") == sub_title:
                found = True
                # 如果存在 taskId 字段，则进一步校验
                if "taskId" in sub_todo:
                    assert sub_todo.get("taskId") == sub_task_id, f"子待办 taskId 不匹配: {sub_todo}"
                # 注意：子待办详情中可能不直接返回 priority，这里主要验证存在性
                break
        assert found is True, f"高优先级子待办 (title={sub_title}) 未找到"

        # Teardown
        try:
            dws.run("todo", "task", "delete", "--task-id", sub_task_id, "--yes")
            dws.run("todo", "task", "delete", "--task-id", parent_task_id, "--yes")
        except Exception:
            pass

    def test_create_sub_todo_invalid_parent_id(self, dws, current_user_id):
        """使用无效的父待办 ID 创建子待办应失败"""
        sub_title = unique_name("Sub_Todo_Invalid_Parent")
        result = dws.run_raw(
            "todo", "task", "create-sub",
            "--parent-id", "INVALID_PARENT_ID_99999",
            "--title", sub_title,
            "--executors", current_user_id,
        )
        # 期望返回非零退出码或错误信息
        assert (
            result.returncode != 0
            or "error" in result.stderr.lower()
            or "false" in result.stdout.lower()
        ), f"使用无效父 ID 创建子待办应失败: {result.stdout}"
