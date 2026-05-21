"""
test_02_mail_reply.py — 邮件回复与转发测试

Commands tested:
  5. dws mail message reply      (reply_message)
  6. dws mail message reply-all  (reply_all)
  7. dws mail message forward    (forward_message)

前置依赖：需要从收件箱搜索到一封邮件 ID，作为回复/转发的目标。
若收件箱为空则跳过相关用例。
"""

import os
import time
import pytest


EMAIL = os.environ.get("DINGTALK_MAIL_EMAIL", "1i2-hgmfj0xkbl@dingtalk.com")


@pytest.fixture(scope="module")
def inbox_message_id(dws):
    """从收件箱中取第一封邮件的 ID，供 reply/forward 测试使用。"""
    result = dws.run_raw(
        "mail", "message", "search",
        "--email", EMAIL,
        "--query", "folderId:2",
        "--size", "1",
    )
    if result.returncode != 0:
        pytest.skip(f"无法搜索收件箱，跳过依赖用例: {result.stderr[:200]}")

    import json
    try:
        data = json.loads(result.stdout)
    except json.JSONDecodeError:
        pytest.skip("search 返回非 JSON，跳过依赖用例")

    msgs = (
        data.get("data", {}).get("messages")
        or data.get("messages")
        or []
    )
    if not msgs:
        pytest.skip("收件箱为空，跳过 reply/forward 用例")

    msg_id = msgs[0].get("messageId") or msgs[0].get("id")
    if not msg_id:
        pytest.skip("无法提取 messageId，跳过 reply/forward 用例")
    return msg_id


class TestMailMessageReply:
    """dws mail message reply"""

    def test_reply_basic(self, dws, inbox_message_id):
        """回复一封收件箱邮件（仅回复发件人）。"""
        data = dws.run_ok(
            "mail", "message", "reply",
            "--from", EMAIL,
            "--to", EMAIL,
            "--id", inbox_message_id,
            "--subject", f"Re: CLI自动化测试_{int(time.time())}",
            "--body", "这是 CLI 自动化回复测试。",
        )
        # 成功回复应返回新邮件 ID
        assert (
            "messageId" in data
            or data.get("success") in (True, "true")
            or "data" in data
        ), f"reply 响应缺少 messageId/success: {data}"

    def test_reply_sender_alias(self, dws, inbox_message_id):
        """使用 --sender 别名代替 --from 回复邮件。"""
        data = dws.run_ok(
            "mail", "message", "reply",
            "--sender", EMAIL,
            "--to", EMAIL,
            "--id", inbox_message_id,
            "--subject", f"Re: alias测试_{int(time.time())}",
            "--body", "sender alias 回复测试。",
        )
        assert (
            "messageId" in data
            or data.get("success") in (True, "true")
            or "data" in data
        ), f"--sender alias reply 响应异常: {data}"

    def test_reply_missing_required_flags(self, dws):
        """缺少必填参数 --id 应报错。"""
        result = dws.run_raw(
            "mail", "message", "reply",
            "--from", EMAIL,
            # 缺少 --id
        )
        assert result.returncode != 0, "缺少必填参数 --id 应返回非零状态码"

    def test_reply_optional_flags(self, dws, inbox_message_id):
        """提供必填参数及 --to, --subject, --body 应能成功回复。"""
        data = dws.run_ok(
            "mail", "message", "reply",
            "--from", EMAIL,
            "--to", EMAIL,
            "--id", inbox_message_id,
            "--subject", f"Re: optional测试_{int(time.time())}",
            "--body", "reply optional flags 测试。",
        )
        assert (
            "messageId" in data
            or data.get("success") in (True, "true")
            or "data" in data
        ), f"reply 响应异常: {data}"

    def test_reply_invalid_message_id(self, dws):
        """无效邮件 ID 应返回错误。"""
        result = dws.run_raw(
            "mail", "message", "reply",
            "--from", EMAIL,
            "--to", EMAIL,
            "--id", "INVALID_MSG_ID_99999",
            "--subject", "Re: test",
            "--body", "test",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), "无效 messageId 应有错误响应"


class TestMailMessageReplyAll:
    """dws mail message reply-all"""

    def test_reply_all_basic(self, dws, inbox_message_id):
        """回复所有人。"""
        data = dws.run_ok(
            "mail", "message", "reply-all",
            "--from", EMAIL,
            "--to", EMAIL,
            "--id", inbox_message_id,
            "--subject", f"Re: 全员回复_{int(time.time())}",
            "--body", "这是 CLI 自动化全员回复测试。",
        )
        assert (
            "messageId" in data
            or data.get("success") in (True, "true")
            or "data" in data
        ), f"reply-all 响应缺少 messageId/success: {data}"

    def test_reply_all_missing_required_flags(self, dws):
        """缺少必填参数 --id 应报错。"""
        result = dws.run_raw(
            "mail", "message", "reply-all",
            "--from", EMAIL,
            # 缺少 --id
        )
        assert result.returncode != 0, "缺少必填参数 --id 应返回非零状态码"

    def test_reply_all_optional_flags(self, dws, inbox_message_id):
        """提供必填参数及 --to, --subject, --body 应能成功回复全部。"""
        data = dws.run_ok(
            "mail", "message", "reply-all",
            "--from", EMAIL,
            "--to", EMAIL,
            "--id", inbox_message_id,
            "--subject", f"Re: reply-all optional测试_{int(time.time())}",
            "--body", "reply-all optional flags 测试。",
        )
        assert (
            "messageId" in data
            or data.get("success") in (True, "true")
            or "data" in data
        ), f"reply-all 响应异常: {data}"


class TestMailMessageForward:
    """dws mail message forward"""

    def test_forward_basic(self, dws, inbox_message_id):
        """转发一封邮件（无附言）。"""
        data = dws.run_ok(
            "mail", "message", "forward",
            "--from", EMAIL,
            "--to", EMAIL,
            "--id", inbox_message_id,
            "--subject", f"Fwd: CLI自动化测试_{int(time.time())}",
        )
        assert (
            "messageId" in data
            or data.get("success") in (True, "true")
            or "data" in data
        ), f"forward 响应缺少 messageId/success: {data}"

    def test_forward_with_body(self, dws, inbox_message_id):
        """转发邮件并附加附言。"""
        data = dws.run_ok(
            "mail", "message", "forward",
            "--from", EMAIL,
            "--to", EMAIL,
            "--id", inbox_message_id,
            "--subject", f"Fwd: 含附言_{int(time.time())}",
            "--body", "附言：请查阅上方邮件内容。",
        )
        assert (
            "messageId" in data
            or data.get("success") in (True, "true")
            or "data" in data
        ), f"forward with body 响应异常: {data}"

    def test_forward_missing_required_flags(self, dws):
        """缺少必填参数 --id 应报错。"""
        result = dws.run_raw(
            "mail", "message", "forward",
            "--from", EMAIL,
            # 缺少 --id
        )
        assert result.returncode != 0, "缺少必填参数 --id 应返回非零状态码"

    def test_forward_optional_flags(self, dws, inbox_message_id):
        """提供必填参数及 --to, --subject 应能成功转发。"""
        data = dws.run_ok(
            "mail", "message", "forward",
            "--from", EMAIL,
            "--to", EMAIL,
            "--id", inbox_message_id,
            "--subject", f"Fwd: optional测试_{int(time.time())}",
        )
        assert (
            "messageId" in data
            or data.get("success") in (True, "true")
            or "data" in data
        ), f"forward 响应异常: {data}"

    def test_forward_invalid_message_id(self, dws):
        """无效邮件 ID 转发应报错。"""
        result = dws.run_raw(
            "mail", "message", "forward",
            "--from", EMAIL,
            "--to", EMAIL,
            "--id", "INVALID_FWD_ID_99999",
            "--subject", "Fwd: test",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), "无效 messageId 转发应有错误响应"
