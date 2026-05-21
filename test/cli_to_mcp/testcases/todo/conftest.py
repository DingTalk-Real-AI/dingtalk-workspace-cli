"""
conftest.py — Todo-specific fixtures.
DWSRunner/dws/current_user_id come from root conftest.py.
"""

import pytest
from test_utils import extract_todo_task_id, iso8601_cn_offset, unique_name


@pytest.fixture(scope="session")
def test_task_id(dws, current_user_id):
    """Create a test todo; yield taskId; delete on teardown."""
    title = unique_name("CLI_Test_Todo")
    data = dws.run(
        "todo", "task", "create",
        "--title", title,
        "--executors", current_user_id,
    )
    # 不同返回结构下 taskId 位置略有差异，统一用公共提取函数。
    task_id = extract_todo_task_id(data)
    assert task_id, f"Todo create must return taskId, got: {data}"
    print(f"\n[SETUP] Created test todo: {task_id}")

    yield task_id

    print(f"\n[TEARDOWN] Deleting test todo: {task_id}")
    try:
        dws.run("todo", "task", "delete", "--task-id", task_id, "--yes")
    except Exception as e:
        print(f"[TEARDOWN WARNING] {e}")


@pytest.fixture(scope="session")
def test_task_id_with_due(dws, current_user_id):
    """Create a test todo with due date; yield taskId; delete on teardown.

    Needed for add-reminder tests with baseTime=dueTime.
    """
    title = unique_name("CLI_Test_Todo_Due")
    due = iso8601_cn_offset(days=7)
    data = dws.run(
        "todo", "task", "create",
        "--title", title,
        "--executors", current_user_id,
        "--due", due,
    )
    task_id = extract_todo_task_id(data)
    assert task_id, f"Todo create (with due) must return taskId, got: {data}"
    print(f"\n[SETUP] Created test todo with due: {task_id}")

    yield task_id

    print(f"\n[TEARDOWN] Deleting test todo with due: {task_id}")
    try:
        dws.run("todo", "task", "delete", "--task-id", task_id, "--yes")
    except Exception as e:
        print(f"[TEARDOWN WARNING] {e}")
