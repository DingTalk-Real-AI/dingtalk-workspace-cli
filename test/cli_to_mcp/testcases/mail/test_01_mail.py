"""
test_mail.py — 邮箱测试 (4 commands × 3 cases)

环境变量: DINGTALK_MAIL_EMAIL

Commands tested:
  1. dws mail mailbox list     (list_user_mailboxes)
  2. dws mail message search   (search_emails)
  3. dws mail message get      (get_email_by_message_id)
  4. dws mail message send     (send_email)
"""

import json
import os
import pytest
import subprocess
import time



@pytest.fixture(scope="session")
def email_addr():
    return os.environ.get("DINGTALK_MAIL_EMAIL", "1i2-hgmfj0xkbl@dingtalk.com")


class TestMailboxList:
    """dws mail mailbox list"""

    def test_list_returns_data(self, dws):
        """查询可用邮箱应返回数据。"""
        data = dws.run_ok("mail", "mailbox", "list")
        assert "emailAccounts" in data, f"响应缺少 emailAccounts 字段: {data}"
        assert data.get("success") == "true", f"success 字段不为 true: {data}"

    def test_list_idempotent(self, dws):
        """多次调用应一致。"""
        d1 = dws.run_ok("mail", "mailbox", "list")
        d2 = dws.run_ok("mail", "mailbox", "list")
        assert d1.get("emailAccounts") == d2.get("emailAccounts"), "两次调用 emailAccounts 结果不一致"

    def test_list_non_empty(self, dws):
        """邮箱列表应包含至少一个地址。"""
        data = dws.run_ok("mail", "mailbox", "list")
        accounts = data.get("emailAccounts", [])
        assert isinstance(accounts, list), f"emailAccounts 应为列表: {data}"
        assert len(accounts) > 0, f"emailAccounts 为空，当前用户未绑定邮箱: {data}"

    def test_list_consistent_with_get_self(self, dws):
        """mailbox list 应非空（当 get-self 有 orgAuthEmail 时用户已有企业邮箱）。

        orgAuthEmail 可能是 dingmail 域，而 mailbox list 返回 dingtalk.com 域，
        两者属于同一用户的不同邮箱地址，因此只校验 mailbox list 非空。
        """
        profile = dws.run("contact", "user", "get-self")
        results = profile.get("result", [])
        if not results:
            pytest.skip("get-self 未返回 result")
        emp = results[0].get("orgEmployeeModel", {})
        org_email = emp.get("orgAuthEmail", "")
        if not org_email:
            pytest.skip("当前用户 get-self 无 orgAuthEmail")

        mailbox = dws.run_ok("mail", "mailbox", "list")
        accounts = mailbox.get("emailAccounts", [])
        assert len(accounts) > 0, (
            f"get-self 返回 orgAuthEmail={org_email}，"
            f"但 mailbox list emailAccounts 为空: {accounts}"
        )


class TestMailMessageSearch:
    """dws mail message search"""

    def test_search_inbox(self, dws, email_addr):
        """搜索收件箱邮件。"""
        data = dws.run_ok(
            "mail", "message", "search",
            "--email", email_addr,
            "--query", "folderId:2",
            "--size", "5",
        )
        assert "messages" in data or "data" in data, f"响应缺少 messages/data 字段: {data}"

    def test_search_by_subject(self, dws, email_addr):
        """按主题搜索。"""
        data = dws.run_ok(
            "mail", "message", "search",
            "--email", email_addr,
            "--query", 'subject:"测试"',
            "--size", "5",
        )
        assert "messages" in data or "data" in data, f"响应缺少 messages/data 字段: {data}"

    def test_search_with_date(self, dws, email_addr):
        """按日期范围搜索。"""
        data = dws.run_ok(
            "mail", "message", "search",
            "--email", email_addr,
            "--query", "date>2026-01-01T00:00:00Z",
            "--size", "5",
        )
        assert "messages" in data or "data" in data, f"响应缺少 messages/data 字段: {data}"


class TestMailMessageGet:
    """dws mail message get"""

    def test_get_first_email(self, dws, email_addr):
        """获取第一封邮件详情。"""
        search = dws.run_ok(
            "mail", "message", "search",
            "--email", email_addr,
            "--query", "folderId:2",
            "--size", "1",
        )
        msgs = search.get("messages", [])
        if not msgs:
            pytest.skip("No messages in inbox")
        msg_id = msgs[0].get("messageId") or msgs[0].get("id")
        if not msg_id:
            pytest.skip("Cannot extract messageId")
        data = dws.run_ok(
            "mail", "message", "get",
            "--email", email_addr, "--id", msg_id,
        )
        assert isinstance(data, dict) and len(data) > 0, f"邮件详情应为非空字典: {data}"

    def test_get_invalid_id(self, dws, email_addr):
        """获取无效邮件 ID。"""
        result = dws.run_raw(
            "mail", "message", "get",
            "--email", email_addr,
            "--id", "INVALID_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_get_invalid_email(self, dws):
        """无效邮箱地址获取邮件。"""
        result = dws.run_raw(
            "mail", "message", "get",
            "--email", "invalid@nowhere.test",
            "--id", "X",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestMailMessageSend:
    """dws mail message send"""

    def test_send_to_self(self, dws, email_addr):
        """给自己发送邮件。"""
        data = dws.run(
            "mail", "message", "send",
            "--from", email_addr,
            "--to", email_addr,
            "--subject", f"CLI测试邮件_{int(time.time())}",
            "--body", "这是 CLI 自动化测试邮件。",
        )
        assert isinstance(data, dict) and len(data) > 0, f"发送结果应为非空字典: {data}"

    def test_send_with_cc(self, dws, email_addr):
        """发送邮件并抄送。"""
        data = dws.run(
            "mail", "message", "send",
            "--from", email_addr,
            "--to", email_addr,
            "--cc", email_addr,
            "--subject", f"抄送测试_{int(time.time())}",
            "--body", "抄送测试邮件。",
        )
        assert isinstance(data, dict) and len(data) > 0, f"发送结果应为非空字典: {data}"

    def test_send_invalid_from(self, dws, email_addr):
        """无效发件人应报错。"""
        result = dws.run_raw(
            "mail", "message", "send",
            "--from", "invalid@nowhere.test",
            "--to", email_addr,
            "--subject", "X",
            "--body", "X",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
