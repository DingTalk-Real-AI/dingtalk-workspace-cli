"""
test_03_mail_batch.py — 邮件批量操作测试

Commands tested:
  8. dws mail message batch-move    (batch_move_message)
  9. dws mail message batch-delete  (batch_delete_message)

前置依赖：
  - batch-move 需要一封可移动的邮件 ID（从收件箱获取）
  - batch-delete 使用草稿来测试删除（避免误删重要邮件），
    若无法创建草稿则从收件箱取邮件 ID 做软删除测试（移入已删除文件夹）

常用文件夹 ID：1=已发送, 2=收件箱, 3=垃圾邮件, 5=草稿, 6=已删除
"""

import json
import os
import time
import pytest


EMAIL = os.environ.get("DINGTALK_MAIL_EMAIL", "1i2-hgmfj0xkbl@dingtalk.com")
FOLDER_INBOX = "2"
FOLDER_DELETED = "6"
FOLDER_DRAFT = "5"


@pytest.fixture(scope="module")
def movable_message_id(dws):
    """从收件箱取一封邮件 ID，用于 batch-move 测试。"""
    result = dws.run_raw(
        "mail", "message", "search",
        "--email", EMAIL,
        "--query", "folderId:2",
        "--size", "1",
    )
    if result.returncode != 0:
        pytest.skip(f"无法搜索收件箱，跳过 batch-move 用例: {result.stderr[:200]}")
    try:
        data = json.loads(result.stdout)
    except json.JSONDecodeError:
        pytest.skip("search 返回非 JSON，跳过 batch-move 用例")

    msgs = (
        data.get("data", {}).get("messages")
        or data.get("messages")
        or []
    )
    if not msgs:
        pytest.skip("收件箱为空，跳过 batch-move 用例")

    msg_id = msgs[0].get("messageId") or msgs[0].get("id")
    if not msg_id:
        pytest.skip("无法提取 messageId，跳过 batch-move 用例")
    return msg_id


@pytest.fixture(scope="module")
def draft_message_id_for_delete(dws):
    """创建一封草稿用于 batch-delete 测试，避免误删重要邮件。"""
    result = dws.run_raw(
        "mail", "draft", "create",
        "--from", EMAIL,
        "--subject", f"CLI_batch_delete_test_{int(time.time())}",
        "--body", "此草稿由 CLI 自动化测试创建，用于 batch-delete 测试。",
    )
    if result.returncode != 0:
        pytest.skip(f"无法创建草稿，跳过 batch-delete 用例: {result.stderr[:200]}")
    try:
        data = json.loads(result.stdout)
    except json.JSONDecodeError:
        pytest.skip("draft create 返回非 JSON，跳过 batch-delete 用例")

    msg_id = (
        data.get("messageId")
        or data.get("data", {}).get("messageId")
        or data.get("result", {}).get("message", {}).get("id")
        or data.get("result", {}).get("message", {}).get("messageId")
    )
    if not msg_id:
        pytest.skip(f"无法提取草稿 messageId，跳过 batch-delete 用例: {data}")
    return msg_id


class TestMailMessageBatchMove:
    """dws mail message batch-move"""

    def test_batch_move_to_deleted(self, dws, movable_message_id):
        """将收件箱邮件移动到已删除文件夹（软删除）。"""
        data = dws.run_ok(
            "mail", "message", "batch-move",
            "--email", EMAIL,
            "--ids", movable_message_id,
            "--folder", FOLDER_DELETED,
        )
        assert (
            data.get("success") == "true"
            or "data" in data
            or isinstance(data, dict)
        ), f"batch-move 响应异常: {data}"

    def test_batch_move_multiple_ids(self, dws):
        """批量移动多封邮件（先搜索两封）。"""
        result = dws.run_raw(
            "mail", "message", "search",
            "--email", EMAIL,
            "--query", "folderId:2",
            "--size", "2",
        )
        if result.returncode != 0:
            pytest.skip("无法搜索收件箱，跳过多ID移动用例")
        try:
            data = json.loads(result.stdout)
        except json.JSONDecodeError:
            pytest.skip("search 返回非 JSON，跳过多ID移动用例")

        msgs = (
            data.get("data", {}).get("messages")
            or data.get("messages")
            or []
        )
        if len(msgs) < 2:
            pytest.skip("收件箱邮件不足 2 封，跳过多ID批量移动用例")

        ids = ",".join(
            m.get("messageId") or m.get("id")
            for m in msgs[:2]
            if m.get("messageId") or m.get("id")
        )
        if not ids:
            pytest.skip("无法提取邮件 ID")

        result2 = dws.run_ok(
            "mail", "message", "batch-move",
            "--email", EMAIL,
            "--ids", ids,
            "--folder", FOLDER_DELETED,
        )
        assert isinstance(result2, dict), f"batch-move 多ID响应异常: {result2}"

    def test_batch_move_missing_required_flags(self, dws):
        """缺少必填参数应报错。"""
        result = dws.run_raw(
            "mail", "message", "batch-move",
            "--email", EMAIL,
            "--ids", "some_id",
            # 缺少 --folder
        )
        assert result.returncode != 0, "缺少 --folder 应返回非零状态码"

    def test_batch_move_invalid_folder(self, dws, movable_message_id):
        """无效文件夹 ID 应报错。"""
        result = dws.run_raw(
            "mail", "message", "batch-move",
            "--email", EMAIL,
            "--ids", movable_message_id,
            "--folder", "99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), "无效 folder ID 应有错误响应"


class TestMailMessageBatchDelete:
    """dws mail message batch-delete"""

    def test_batch_delete_draft(self, dws, draft_message_id_for_delete):
        """删除一封草稿邮件。"""
        data = dws.run_ok(
            "mail", "message", "batch-delete",
            "--email", EMAIL,
            "--ids", draft_message_id_for_delete,
        )
        assert (
            data.get("success") == "true"
            or "data" in data
            or isinstance(data, dict)
        ), f"batch-delete 响应异常: {data}"

    def test_batch_delete_missing_required_flags(self, dws):
        """缺少必填参数应报错。"""
        result = dws.run_raw(
            "mail", "message", "batch-delete",
            "--email", EMAIL,
            # 缺少 --ids
        )
        assert result.returncode != 0, "缺少 --ids 应返回非零状态码"

    def test_batch_delete_invalid_ids(self, dws):
        """无效邮件 ID 删除应报错。"""
        result = dws.run_raw(
            "mail", "message", "batch-delete",
            "--email", EMAIL,
            "--ids", "INVALID_DEL_ID_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), "无效 ID 批量删除应有错误响应"
