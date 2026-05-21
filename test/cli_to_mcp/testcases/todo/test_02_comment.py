"""
test_02_comment.py — 待办评论测试 (3 commands × 3+ cases)

Commands tested:
  1. dws todo comment add     (add_todo_comment)
  2. dws todo comment list    (list_todo_comment)
  3. dws todo comment delete  (delete_todo_comment)

依赖：
  - test_task_id fixture (todo/conftest.py 提供)
  - dws / current_user_id (root conftest.py 提供)

注意：
  - add 使用 --task-id + --content
  - list 支持 --page / --size 分页
  - delete 需 --task-id + --comment-id，并加 --yes 跳过二次确认
"""

import time
import pytest


def _extract_comment_id(data: dict):
    """从 add/list 返回结构中尽力提取 commentId，兼容多种返回层级。"""
    if not isinstance(data, dict):
        return None
    result = data.get("result")
    if isinstance(result, dict):
        for key in ("commentId", "id"):
            if result.get(key):
                return result[key]
        # list 风格：result.comments / result.commentList
        for key in ("comments", "commentList", "commentCards"):
            arr = result.get(key)
            if isinstance(arr, list) and arr:
                first = arr[0]
                if isinstance(first, dict):
                    return first.get("commentId") or first.get("id")
    if isinstance(result, list) and result:
        first = result[0]
        if isinstance(first, dict):
            return first.get("commentId") or first.get("id")
    return None


class TestCommentAdd:
    """dws todo comment add"""

    def test_add_returns_success(self, dws, test_task_id):
        """新增评论应成功返回。"""
        data = dws.run_ok(
            "todo", "comment", "add",
            "--task-id", "task5635986a3060144670e65b2a5c080e89",
            "--content", f"CLI_Comment_{int(time.time())}",
        )
        assert data is not None

    def test_add_chinese_content(self, dws, test_task_id):
        """中文评论内容应被正确处理。"""
        data = dws.run_ok(
            "todo", "comment", "add",
            "--task-id", "task5635986a3060144670e65b2a5c080e89",
            "--content", "中文评论内容-测试",
        )
        assert data is not None

    def test_add_missing_content_should_fail(self, dws, test_task_id):
        """缺少 --content 应被 CLI 校验拒绝。"""
        result = dws.run_raw(
            "todo", "comment", "add",
            "--task-id", test_task_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), "missing --content should be rejected"

    def test_add_to_invalid_task(self, dws):
        """向无效 taskId 添加评论应报错或返回失败。"""
        result = dws.run_raw(
            "todo", "comment", "add",
            "--task-id", "INVALID_TASK_99999",
            "--content", "should fail",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
            or "false" in result.stdout.lower()
        )


class TestCommentList:
    """dws todo comment list"""

    def test_list_returns_data(self, dws, test_task_id):
        """查询评论列表应成功返回。"""
        # 先确保至少有一条评论
        dws.run_ok(
            "todo", "comment", "add",
            "--task-id", test_task_id,
            "--content", f"For_List_{int(time.time())}",
        )
        data = dws.run_ok(
            "todo", "comment", "list",
            "--task-id", test_task_id,
        )
        assert data is not None

    def test_list_with_pagination(self, dws, test_task_id):
        """指定 --page / --size 应成功返回。"""
        data = dws.run_ok(
            "todo", "comment", "list",
            "--task-id", test_task_id,
            "--page", "1",
            "--size", "20",
        )
        assert data is not None

    def test_list_invalid_task(self, dws):
        """无效 taskId 查询评论应报错或返回失败。"""
        result = dws.run_raw(
            "todo", "comment", "list",
            "--task-id", "INVALID_TASK_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
            or "false" in result.stdout.lower()
        )


class TestCommentDelete:
    """dws todo comment delete"""

    def test_add_then_delete(self, dws, test_task_id):
        """新增一条评论后能成功删除。"""
        add_data = dws.run_ok(
            "todo", "comment", "add",
            "--task-id", test_task_id,
            "--content", f"To_Be_Deleted_{int(time.time())}",
        )
        comment_id = _extract_comment_id(add_data)
        if not comment_id:
            # 兜底：从 list 中拿最新一条
            list_data = dws.run_ok(
                "todo", "comment", "list",
                "--task-id", test_task_id,
            )
            comment_id = _extract_comment_id(list_data)
        if not comment_id:
            pytest.skip(f"无法从 add/list 返回中提取 commentId: add={add_data}")

        data = dws.run_ok(
            "todo", "comment", "delete",
            "--task-id", test_task_id,
            "--comment-id", comment_id,
            "--yes",
        )
        assert data is not None

    def test_delete_invalid_comment(self, dws, test_task_id):
        """删除不存在的 commentId 应报错或返回失败。"""
        result = dws.run_raw(
            "todo", "comment", "delete",
            "--task-id", test_task_id,
            "--comment-id", "INVALID_COMMENT_99999",
            "--yes",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
            or "false" in result.stdout.lower()
        )

    def test_delete_missing_comment_id(self, dws, test_task_id):
        """缺少 --comment-id 应被 CLI 校验拒绝。"""
        result = dws.run_raw(
            "todo", "comment", "delete",
            "--task-id", test_task_id,
            "--yes",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), "missing --comment-id should be rejected"

    def test_delete_from_invalid_task(self, dws):
        """无效 taskId 下删除评论应报错。"""
        result = dws.run_raw(
            "todo", "comment", "delete",
            "--task-id", "INVALID_TASK_99999",
            "--comment-id", "INVALID_COMMENT_99999",
            "--yes",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
            or "false" in result.stdout.lower()
        )
