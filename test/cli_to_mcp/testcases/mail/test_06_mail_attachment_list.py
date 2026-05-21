"""
test_06_mail_attachment_list.py — 邮件附件列举测试

环境变量: DINGTALK_MAIL_EMAIL

Commands tested:
  1. dws mail attachment list  (list_mail_attachments)

前置依赖：
  需要先通过 dws mail message search 获取一封含附件的邮件 ID。
"""

import os
import pytest


@pytest.fixture(scope="session")
def email_addr():
    return os.environ.get("DINGTALK_MAIL_EMAIL", "1i2-hgmfj0xkbl@dingtalk.com")


@pytest.fixture(scope="session")
def message_with_attachment(dws, email_addr):
    """尝试搜索一封含附件的邮件，返回其 messageId；若找不到则跳过。"""
    data = dws.run(
        "mail", "message", "search",
        "--email", email_addr,
        "--query", "hasAttachments:true",
        "--size", "1",
    )
    messages = data.get("messages", [])
    if not messages:
        pytest.skip("未找到含附件的邮件，跳过附件列举测试")
    return messages[0].get("id") or messages[0].get("messageId")


class TestMailAttachmentList:
    """dws mail attachment list

    响应结构：{"attachments": [...]}
    attachment 对象包含：id, name, contentType, size
    """

    def test_list_returns_attachments_field(self, dws, email_addr, message_with_attachment):
        """正常查询：应返回 attachments 列表。"""
        data = dws.run_ok(
            "mail", "attachment", "list",
            "--email", email_addr,
            "--id", message_with_attachment,
        )
        assert "attachments" in data, f"响应缺少 attachments 字段: {data}"
        assert isinstance(data["attachments"], list), (
            f"attachments 应为列表: {data}"
        )

    def test_attachment_has_required_fields(self, dws, email_addr, message_with_attachment):
        """附件条目应包含 id 和 name 字段（如有结果）。"""
        data = dws.run_ok(
            "mail", "attachment", "list",
            "--email", email_addr,
            "--id", message_with_attachment,
        )
        attachments = data.get("attachments", [])
        if not attachments:
            pytest.skip("该邮件附件列表为空，跳过字段校验")
        first = attachments[0]
        assert "id" in first, f"附件条目缺少 id 字段: {first}"
        assert "name" in first, f"附件条目缺少 name 字段: {first}"

    def test_attachment_has_size_and_content_type(self, dws, email_addr, message_with_attachment):
        """附件条目应包含 size 字段；contentType 为可选字段（如有结果）。"""
        data = dws.run_ok(
            "mail", "attachment", "list",
            "--email", email_addr,
            "--id", message_with_attachment,
        )
        attachments = data.get("attachments", [])
        if not attachments:
            pytest.skip("该邮件附件列表为空，跳过字段校验")
        first = attachments[0]
        assert "size" in first, f"附件条目缺少 size 字段: {first}"
        # contentType 为可选字段，部分附件类型（如 .eml）可能不包含该字段
        if "contentType" in first:
            assert isinstance(first["contentType"], str), (
                f"contentType 应为字符串: {first}"
            )

    def test_attachment_size_is_positive(self, dws, email_addr, message_with_attachment):
        """附件大小应为正整数。"""
        data = dws.run_ok(
            "mail", "attachment", "list",
            "--email", email_addr,
            "--id", message_with_attachment,
        )
        attachments = data.get("attachments", [])
        if not attachments:
            pytest.skip("该邮件附件列表为空，跳过大小校验")
        for att in attachments:
            size = att.get("size", 0)
            assert isinstance(size, (int, float)) and size > 0, (
                f"附件 size 应为正数，实际: {size}, 附件: {att}"
            )


class TestMailAttachmentListParamValidation:
    """参数校验测试（不依赖具体邮件数据）。"""

    def test_missing_email(self, dws):
        """缺少必填参数 --email 应报错。"""
        result = dws.run_raw(
            "mail", "attachment", "list",
            "--id", "fake_message_id",
        )
        assert result.returncode != 0, "缺少 --email 参数应返回非零退出码"

    def test_missing_id(self, dws, email_addr):
        """缺少必填参数 --id 应报错。"""
        result = dws.run_raw(
            "mail", "attachment", "list",
            "--email", email_addr,
        )
        assert result.returncode != 0, "缺少 --id 参数应返回非零退出码"

    def test_missing_both_params(self, dws):
        """缺少所有必填参数应报错。"""
        result = dws.run_raw(
            "mail", "attachment", "list",
        )
        assert result.returncode != 0, "缺少所有必填参数应返回非零退出码"

    def test_invalid_email(self, dws):
        """无效邮箱地址应报错。"""
        result = dws.run_raw(
            "mail", "attachment", "list",
            "--email", "invalid@nowhere.test",
            "--id", "fake_message_id",
        )
        combined = (result.stdout + result.stderr).lower()
        assert (
            result.returncode != 0
            or "error" in combined
        ), f"无效邮箱应报错，实际 returncode={result.returncode}"

    def test_invalid_message_id(self, dws, email_addr):
        """无效邮件 ID 应报错或返回空列表。"""
        result = dws.run_raw(
            "mail", "attachment", "list",
            "--email", email_addr,
            "--id", "nonexistent_message_id_xyz",
        )
        combined = (result.stdout + result.stderr).lower()
        assert (
            result.returncode != 0
            or "error" in combined
            or "attachments" in combined
        ), (
            f"无效邮件 ID 应报错或返回空附件列表, "
            f"returncode={result.returncode}"
        )

    def test_help_shows_attachment_list(self, dws):
        """help 输出应展示 attachment list 子命令信息。"""
        result = dws.run_raw(
            "mail", "attachment", "list", "--help",
        )
        help_text = result.stdout + result.stderr
        assert "--email" in help_text, (
            f"--email 未出现在 help 输出中:\n{help_text[:500]}"
        )
        assert "--id" in help_text, (
            f"--id 未出现在 help 输出中:\n{help_text[:500]}"
        )
