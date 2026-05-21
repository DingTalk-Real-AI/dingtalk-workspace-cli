"""
test_03_mail_attachment_download.py — 邮件附件下载测试

环境变量: DINGTALK_MAIL_EMAIL

Commands tested:
  1. dws mail attachment download  (编排: list_user_mailboxes → create_download_session → HTTP GET 保存到本地)

Note:
  端到端下载测试需要 MCP Server 实现 create_download_session tool。
  在后端就绪前，端到端用例标记 @pytest.mark.skip。
"""

import os
import tempfile

import pytest


@pytest.fixture(scope="session")
def email_addr():
    email = os.environ.get("DINGTALK_MAIL_EMAIL")
    if not email:
        pytest.skip("未设置 DINGTALK_MAIL_EMAIL 环境变量，跳过邮件相关用例")
    return email


class TestMailAttachmentDownloadParamValidation:
    """附件下载参数校验测试（不依赖后端 MCP tool，可立即运行）。"""

    def test_download_help_contains_flags(self, dws):
        """--help 应包含所有必填和可选 flags。"""
        result = dws.run_raw("mail", "attachment", "download", "--help")
        help_text = result.stdout + result.stderr
        for flag in ("--email", "--message-id", "--attachment-id", "--name", "--output"):
            assert flag in help_text, (
                f"{flag} 未出现在 attachment download --help 输出中:\n{help_text[:500]}"
            )

    def test_download_missing_email(self, dws):
        """缺少 --email 应报错。"""
        result = dws.run_raw(
            "mail", "attachment", "download",
            "--message-id", "msg_abc123",
            "--attachment-id", "att_001",
            "--name", "report.pdf",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "required" in combined, (
            f"缺少 --email 应报错, returncode={result.returncode}"
        )

    def test_download_missing_message_id(self, dws, email_addr):
        """缺少 --message-id 应报错。"""
        result = dws.run_raw(
            "mail", "attachment", "download",
            "--email", email_addr,
            "--attachment-id", "att_001",
            "--name", "report.pdf",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "required" in combined, (
            f"缺少 --message-id 应报错, returncode={result.returncode}"
        )

    def test_download_missing_attachment_id(self, dws, email_addr):
        """缺少 --attachment-id 应报错。"""
        result = dws.run_raw(
            "mail", "attachment", "download",
            "--email", email_addr,
            "--message-id", "msg_abc123",
            "--name", "report.pdf",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "required" in combined, (
            f"缺少 --attachment-id 应报错, returncode={result.returncode}"
        )

    def test_download_missing_name(self, dws, email_addr):
        """缺少 --name 应报错。"""
        result = dws.run_raw(
            "mail", "attachment", "download",
            "--email", email_addr,
            "--message-id", "msg_abc123",
            "--attachment-id", "att_001",
        )
        combined = (result.stdout + result.stderr).lower()
        assert result.returncode != 0 or "error" in combined or "required" in combined, (
            f"缺少 --name 应报错, returncode={result.returncode}"
        )


@pytest.fixture(scope="module")
def real_attachment_info(dws, email_addr):
    """动态查找一封含附件的邮件，返回 (message_id, attachment_id, attachment_name)。

    流程：
      1. message search 搜索含附件邮件（hasAttachments:true）
      2. attachment list 取第一个附件信息
    找不到则 skip。
    """
    import json

    result = dws.run_raw(
        "mail", "message", "search",
        "--email", email_addr,
        "--query", "hasAttachments:true",
        "--size", "1",
    )
    if result.returncode != 0:
        pytest.skip(f"无法搜索含附件邮件，跳过下载 E2E 用例: {result.stderr[:200]}")
    try:
        data = json.loads(result.stdout)
    except json.JSONDecodeError:
        pytest.skip("search 返回非 JSON，跳过下载 E2E 用例")

    msgs = data.get("messages", [])
    if not msgs:
        pytest.skip("未找到含附件的邮件，跳过下载 E2E 用例")

    msg_id = msgs[0].get("id") or msgs[0].get("messageId")
    if not msg_id:
        pytest.skip("无法提取 messageId，跳过下载 E2E 用例")

    # 再查附件列表
    att_result = dws.run_raw(
        "mail", "attachment", "list",
        "--email", email_addr,
        "--id", msg_id,
    )
    if att_result.returncode != 0:
        pytest.skip(f"无法查询附件列表，跳过下载 E2E 用例: {att_result.stderr[:200]}")
    try:
        att_data = json.loads(att_result.stdout)
    except json.JSONDecodeError:
        pytest.skip("attachment list 返回非 JSON，跳过下载 E2E 用例")

    attachments = att_data.get("attachments", [])
    if not attachments:
        pytest.skip("该邮件附件列表为空，跳过下载 E2E 用例")

    att = attachments[0]
    att_id = att.get("id") or att.get("attachmentId")
    att_name = att.get("name", "download_test_file")
    if not att_id:
        pytest.skip("无法提取 attachmentId，跳过下载 E2E 用例")

    return msg_id, att_id, att_name


class TestMailAttachmentDownloadE2E:
    """端到端附件下载测试（后端 MCP Server 就绪后运行）。

    fixture real_attachment_info 会自动搜索含附件的邮件并提取真实 ID，无需手动指定。
    """

    def test_download_attachment_to_default_dir(self, dws, email_addr, real_attachment_info, tmp_path):
        """下载附件到指定目录（使用 --output）。"""
        msg_id, att_id, att_name = real_attachment_info
        output_dir = str(tmp_path)
        result = dws.run_raw(
            "mail", "attachment", "download",
            "--email", email_addr,
            "--message-id", msg_id,
            "--attachment-id", att_id,
            "--name", att_name,
            "--output", output_dir,
        )
        combined = result.stdout + result.stderr
        assert result.returncode == 0, (
            f"下载附件失败, returncode={result.returncode}, output={combined[:500]}"
        )
        dest = os.path.join(output_dir, att_name)
        assert os.path.exists(dest), (
            f"附件文件未保存到指定目录 {dest}, output={combined[:300]}"
        )
        assert os.path.getsize(dest) > 0, f"下载的附件文件大小为 0: {dest}"

    def test_download_attachment_to_custom_dir(self, dws, email_addr, real_attachment_info, tmp_path):
        """再次下载同一附件到另一个临时目录，验证 --output 可重复使用。"""
        msg_id, att_id, att_name = real_attachment_info
        output_dir = str(tmp_path / "subdir")
        os.makedirs(output_dir, exist_ok=True)
        result = dws.run_raw(
            "mail", "attachment", "download",
            "--email", email_addr,
            "--message-id", msg_id,
            "--attachment-id", att_id,
            "--name", att_name,
            "--output", output_dir,
        )
        combined = result.stdout + result.stderr
        assert result.returncode == 0, (
            f"下载附件失败, returncode={result.returncode}, output={combined[:500]}"
        )
        dest = os.path.join(output_dir, att_name)
        assert os.path.exists(dest), (
            f"附件文件未保存到子目录 {dest}, output={combined[:300]}"
        )
        assert os.path.getsize(dest) > 0, f"下载的附件文件大小为 0: {dest}"
