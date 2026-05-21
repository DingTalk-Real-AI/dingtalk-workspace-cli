"""
test_02_thread_folder_tag.py — 邮件会话/文件夹/标签测试

环境变量: DINGTALK_MAIL_EMAIL

Commands tested:
  1. dws mail thread get      (get_conversation)
  2. dws mail folder list     (list_folders)
  3. dws mail tag list        (list_mail_tags)
"""

import os
import pytest


@pytest.fixture(scope="session")
def email_addr():
    return os.environ.get("DINGTALK_MAIL_EMAIL", "1i2-hgmfj0xkbl@dingtalk.com")


@pytest.fixture(scope="session")
def first_conversation_id(dws, email_addr):
    """从收件箱搜索第一封邮件，提取 conversationId 供 thread get 使用。

    message search 响应结构：{"messages": [{"conversationId": "...", ...}], "total": "N", "nextCursor": "..."}
    """
    search = dws.run_ok(
        "mail", "message", "search",
        "--email", email_addr,
        "--query", "folderId:2",
        "--size", "1",
    )
    msgs = search.get("messages", [])
    if not msgs:
        pytest.skip("收件箱无邮件，无法提取 conversationId")
    cid = msgs[0].get("conversationId")
    if not cid:
        pytest.skip("邮件响应中无 conversationId 字段")
    return cid


@pytest.fixture(scope="session")
def first_folder_id(dws, email_addr):
    """获取顶层文件夹列表，提取第一个文件夹 ID 供子文件夹测试使用。

    folder list 响应结构：{"folders": [{"id": "...", "displayName": "...", ...}]}
    """
    data = dws.run_ok(
        "mail", "folder", "list",
        "--email", email_addr,
    )
    folders = data.get("folders", [])
    if not folders:
        pytest.skip("顶层文件夹列表为空，无法提取 folderId")
    fid = folders[0].get("id")
    if not fid:
        pytest.skip("文件夹响应中无 id 字段")
    return fid


# ──────────────────────────────────────────────────────────
# dws mail thread get
# ──────────────────────────────────────────────────────────

class TestMailThreadGet:
    """dws mail thread get

    响应结构：{"conversation": {"id": "...", "subject": "...", "messageCount": N, ...}}
    """

    def test_get_thread_basic(self, dws, email_addr, first_conversation_id):
        """正常获取会话详情，校验外层 conversation key 及核心字段。"""
        data = dws.run_ok(
            "mail", "thread", "get",
            "--email", email_addr,
            "--id", first_conversation_id,
        )
        assert "conversation" in data, f"响应缺少 conversation 字段: {data}"
        conversation = data["conversation"]
        assert "id" in conversation, f"conversation 缺少 id 字段: {conversation}"
        assert "subject" in conversation, f"conversation 缺少 subject 字段: {conversation}"

    def test_get_thread_returns_message_count(self, dws, email_addr, first_conversation_id):
        """会话详情应包含 messageCount 字段。"""
        data = dws.run_ok(
            "mail", "thread", "get",
            "--email", email_addr,
            "--id", first_conversation_id,
        )
        assert "conversation" in data, f"响应缺少 conversation 字段: {data}"
        conversation = data["conversation"]
        assert "messageCount" in conversation, f"conversation 缺少 messageCount 字段: {conversation}"

    def test_get_thread_invalid_id(self, dws, email_addr):
        """无效 conversationId 应报错。"""
        result = dws.run_raw(
            "mail", "thread", "get",
            "--email", email_addr,
            "--id", "INVALID_CONVERSATION_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效 conversationId 应报错，实际 returncode={result.returncode}"

    def test_get_thread_missing_id(self, dws, email_addr):
        """缺少必填参数 --id 应报错。"""
        result = dws.run_raw(
            "mail", "thread", "get",
            "--email", email_addr,
        )
        assert result.returncode != 0, "缺少 --id 参数应返回非零退出码"

    def test_get_thread_missing_email(self, dws, first_conversation_id):
        """缺少必填参数 --email 应报错。"""
        result = dws.run_raw(
            "mail", "thread", "get",
            "--id", first_conversation_id,
        )
        assert result.returncode != 0, "缺少 --email 参数应返回非零退出码"


# ──────────────────────────────────────────────────────────
# dws mail folder list
# ──────────────────────────────────────────────────────────

class TestMailFolderList:
    """dws mail folder list

    响应结构：{"folders": [{"id": "...", "displayName": "...", "parentFolderId": "...",
                            "childFolderCount": N, "totalItemCount": N, "unreadItemCount": N}]}
    """

    def test_list_top_level_folders(self, dws, email_addr):
        """不传 --folder-id，返回顶层文件夹列表，且列表非空。"""
        data = dws.run_ok(
            "mail", "folder", "list",
            "--email", email_addr,
        )
        assert "folders" in data, f"响应缺少 folders 字段: {data}"
        folders = data["folders"]
        assert isinstance(folders, list), f"folders 应为列表: {data}"
        assert len(folders) > 0, f"顶层文件夹列表不应为空: {data}"

    def test_list_folders_fields(self, dws, email_addr):
        """文件夹条目应包含 id 和 displayName 字段。"""
        data = dws.run_ok(
            "mail", "folder", "list",
            "--email", email_addr,
        )
        folders = data.get("folders", [])
        assert len(folders) > 0, f"顶层文件夹列表为空: {data}"
        first = folders[0]
        assert "id" in first, f"文件夹条目缺少 id 字段: {first}"
        assert "displayName" in first, f"文件夹条目缺少 displayName 字段: {first}"

    def test_list_sub_folders(self, dws, email_addr, first_folder_id):
        """传入 --folder-id，返回该文件夹的子文件夹列表（可为空列表，但结构必须正确）。"""
        data = dws.run_ok(
            "mail", "folder", "list",
            "--email", email_addr,
            "--folder-id", first_folder_id,
        )
        assert "folders" in data, f"响应缺少 folders 字段: {data}"
        assert isinstance(data["folders"], list), f"folders 应为列表: {data}"

    def test_list_folders_missing_email(self, dws):
        """缺少必填参数 --email 应报错。"""
        result = dws.run_raw("mail", "folder", "list")
        assert result.returncode != 0, "缺少 --email 参数应返回非零退出码"

    def test_list_folders_invalid_email(self, dws):
        """无效邮箱地址应报错。"""
        result = dws.run_raw(
            "mail", "folder", "list",
            "--email", "invalid@nowhere.test",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效邮箱应报错，实际 returncode={result.returncode}"


# ──────────────────────────────────────────────────────────
# dws mail tag list
# ──────────────────────────────────────────────────────────

class TestMailTagList:
    """dws mail tag list

    响应结构：{"tags": [{"id": "...", "name": "...", "parentId": "...",
                         "totalItemCount": N, "unreadItemCount": N}]}
    """

    def test_list_tags_basic(self, dws, email_addr):
        """正常列举邮件标签，校验响应包含 tags 字段且为列表。"""
        data = dws.run_ok(
            "mail", "tag", "list",
            "--email", email_addr,
        )
        assert "tags" in data, f"响应缺少 tags 字段: {data}"
        assert isinstance(data["tags"], list), f"tags 应为列表: {data}"

    def test_list_tags_idempotent(self, dws, email_addr):
        """多次调用结果应一致。"""
        d1 = dws.run_ok("mail", "tag", "list", "--email", email_addr)
        d2 = dws.run_ok("mail", "tag", "list", "--email", email_addr)
        assert d1.get("tags") == d2.get("tags"), (
            f"两次调用 tags 结果不一致: {d1.get('tags')} vs {d2.get('tags')}"
        )

    def test_list_tags_fields(self, dws, email_addr):
        """标签条目应包含 id 和 name 字段（如有标签）。"""
        data = dws.run_ok(
            "mail", "tag", "list",
            "--email", email_addr,
        )
        tags = data.get("tags", [])
        if not tags:
            pytest.skip("当前邮箱无标签，跳过字段校验")
        first = tags[0]
        assert "id" in first, f"标签条目缺少 id 字段: {first}"
        assert "name" in first, f"标签条目缺少 name 字段: {first}"

    def test_list_tags_missing_email(self, dws):
        """缺少必填参数 --email 应报错。"""
        result = dws.run_raw("mail", "tag", "list")
        assert result.returncode != 0, "缺少 --email 参数应返回非零退出码"

    def test_list_tags_invalid_email(self, dws):
        """无效邮箱地址应报错。"""
        result = dws.run_raw(
            "mail", "tag", "list",
            "--email", "invalid@nowhere.test",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), f"无效邮箱应报错，实际 returncode={result.returncode}"
