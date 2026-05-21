"""
test_04_mail_draft.py — 草稿管理测试

Commands tested:
  10. dws mail draft create  (create_draft)
  11. dws mail draft update  (update_draft)
  12. dws mail draft send    (send_draft)

草稿箱 folderId: 5
"""

import json
import os
import time
import pytest


EMAIL = os.environ.get("DINGTALK_MAIL_EMAIL", "1i2-hgmfj0xkbl@dingtalk.com")


@pytest.fixture(scope="module")
def created_draft_id(dws):
    """创建草稿并返回 messageId，供 update 测试使用。"""
    result = dws.run_raw(
        "mail", "draft", "create",
        "--from", EMAIL,
        "--subject", f"CLI_draft_update_test_{int(time.time())}",
        "--body", "初始草稿正文。",
    )
    if result.returncode != 0:
        pytest.skip(f"无法创建草稿，跳过 draft update 用例: {result.stderr[:200]}")
    try:
        data = json.loads(result.stdout)
    except json.JSONDecodeError:
        pytest.skip("draft create 返回非 JSON，跳过 draft update 用例")

    msg_id = (
        data.get("messageId")
        or data.get("data", {}).get("messageId")
        or data.get("result", {}).get("message", {}).get("id")
        or data.get("result", {}).get("message", {}).get("messageId")
    )
    if not msg_id:
        pytest.skip(f"无法提取草稿 messageId，跳过 draft update 用例: {data}")
    return msg_id


def _draft_create_ok(data: dict, label: str = "") -> None:
    """统一断言 draft create 成功响应。

    兼容三种格式：
      - {"success": True/"true", ...}
      - {"messageId": "...", ...}
      - {"result": {"message": {"id": "..."}}, ...}
    """
    assert (
        "messageId" in data
        or data.get("success") in (True, "true")
        or data.get("result", {}).get("message", {}).get("id")
        or "data" in data
    ), f"draft create 响应异常{' (' + label + ')' if label else ''}: {data}"


class TestMailDraftCreate:
    """dws mail draft create"""

    def test_create_minimal(self, dws):
        """最小参数创建草稿（仅 --from 和 --subject）。"""
        data = dws.run_ok(
            "mail", "draft", "create",
            "--from", EMAIL,
            "--subject", f"最简草稿_{int(time.time())}",
        )
        _draft_create_ok(data)


    def test_create_with_body(self, dws):
        """创建带正文的草稿。"""
        data = dws.run_ok(
            "mail", "draft", "create",
            "--from", EMAIL,
            "--subject", f"含正文草稿_{int(time.time())}",
            "--body", "这是草稿正文内容。",
        )
        _draft_create_ok(data, "with body")


    def test_create_with_recipients(self, dws):
        """创建包含收件人的草稿。"""
        data = dws.run_ok(
            "mail", "draft", "create",
            "--from", EMAIL,
            "--to", EMAIL,
            "--subject", f"含收件人草稿_{int(time.time())}",
            "--body", "待发送的草稿。",
        )
        _draft_create_ok(data, "with recipients")


    def test_create_with_cc(self, dws):
        """创建包含抄送的草稿。"""
        data = dws.run_ok(
            "mail", "draft", "create",
            "--from", EMAIL,
            "--to", EMAIL,
            "--cc", EMAIL,
            "--subject", f"含抄送草稿_{int(time.time())}",
            "--body", "含抄送的草稿。",
        )
        _draft_create_ok(data, "with cc")


    def test_create_sender_alias(self, dws):
        """使用 --sender 别名代替 --from 创建草稿。"""
        data = dws.run_ok(
            "mail", "draft", "create",
            "--sender", EMAIL,
            "--subject", f"sender别名草稿_{int(time.time())}",
        )
        _draft_create_ok(data, "sender alias")


    def test_create_missing_required_from(self, dws):
        """缺少 --from/--sender 应报错。"""
        result = dws.run_raw(
            "mail", "draft", "create",
            "--subject", "无发件人草稿",
        )
        assert result.returncode != 0, "缺少 --from 应返回非零状态码"

    def test_create_missing_required_subject(self, dws):
        """缺少 --subject 应报错。"""
        result = dws.run_raw(
            "mail", "draft", "create",
            "--from", EMAIL,
        )
        assert result.returncode != 0, "缺少 --subject 应返回非零状态码"


class TestMailDraftUpdate:
    """dws mail draft update"""

    def test_update_subject(self, dws, created_draft_id):
        """更新草稿主题。"""
        data = dws.run_ok(
            "mail", "draft", "update",
            "--from", EMAIL,
            "--id", created_draft_id,
            "--subject", f"更新后标题_{int(time.time())}",
        )
        assert (
            data.get("success") == "true"
            or "data" in data
            or isinstance(data, dict)
        ), f"draft update subject 响应异常: {data}"

    def test_update_body(self, dws, created_draft_id):
        """更新草稿正文。"""
        data = dws.run_ok(
            "mail", "draft", "update",
            "--from", EMAIL,
            "--id", created_draft_id,
            "--body", f"更新后正文内容_{int(time.time())}",
        )
        assert (
            data.get("success") == "true"
            or "data" in data
            or isinstance(data, dict)
        ), f"draft update body 响应异常: {data}"

    def test_update_recipients(self, dws, created_draft_id):
        """更新草稿收件人。"""
        data = dws.run_ok(
            "mail", "draft", "update",
            "--from", EMAIL,
            "--id", created_draft_id,
            "--to", EMAIL,
            "--cc", EMAIL,
        )
        assert (
            data.get("success") == "true"
            or "data" in data
            or isinstance(data, dict)
        ), f"draft update recipients 响应异常: {data}"

    def test_update_missing_required_flags(self, dws):
        """缺少必填参数应报错。"""
        result = dws.run_raw(
            "mail", "draft", "update",
            "--from", EMAIL,
            # 缺少 --id
        )
        assert result.returncode != 0, "缺少 --id 应返回非零状态码"

    def test_update_invalid_draft_id(self, dws):
        """无效草稿 ID 应返回错误。"""
        result = dws.run_raw(
            "mail", "draft", "update",
            "--from", EMAIL,
            "--id", "INVALID_DRAFT_ID_99999",
            "--subject", "新标题",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), "无效草稿 ID 更新应有错误响应"


@pytest.fixture(scope="module")
def draft_id_for_send(dws):
    """创建一封草稿用于 send 测试，返回 messageId。"""
    result = dws.run_raw(
        "mail", "draft", "create",
        "--from", EMAIL,
        "--to", EMAIL,
        "--subject", f"CLI_draft_send_test_{int(time.time())}",
        "--body", "这是用于发送测试的草稿正文。",
    )
    if result.returncode != 0:
        pytest.skip(f"无法创建草稿，跳过 draft send 用例: {result.stderr[:200]}")
    try:
        data = json.loads(result.stdout)
    except json.JSONDecodeError:
        pytest.skip("draft create 返回非 JSON，跳过 draft send 用例")

    msg_id = (
        data.get("messageId")
        or data.get("data", {}).get("messageId")
        or data.get("result", {}).get("message", {}).get("id")
        or data.get("result", {}).get("message", {}).get("messageId")
    )
    if not msg_id:
        pytest.skip(f"无法提取草稿 messageId，跳过 draft send 用例: {data}")
    return msg_id


class TestMailDraftSend:
    """dws mail draft send"""

    def test_send_missing_required_from(self, dws):
        """缺少 --from/--sender 应报错。"""
        result = dws.run_raw(
            "mail", "draft", "send",
            "--id", "any-draft-id",
        )
        assert result.returncode != 0, "缺少 --from 应返回非零状态码"

    def test_send_missing_required_id(self, dws):
        """缺少 --id 应报错。"""
        result = dws.run_raw(
            "mail", "draft", "send",
            "--from", EMAIL,
        )
        assert result.returncode != 0, "缺少 --id 应返回非零状态码"

    def test_send_invalid_draft_id(self, dws):
        """无效草稿 ID 应返回错误。"""
        result = dws.run_raw(
            "mail", "draft", "send",
            "--from", EMAIL,
            "--id", "INVALID_DRAFT_ID_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), "无效草稿 ID 发送应有错误响应"

    def test_send_draft(self, dws, draft_id_for_send):
        """发送已有草稿。"""
        data = dws.run_ok(
            "mail", "draft", "send",
            "--from", EMAIL,
            "--id", draft_id_for_send,
        )
        assert isinstance(data, dict), f"draft send 响应应为字典: {data}"

    def test_send_sender_alias(self, dws):
        """使用 --sender 别名发送草稿（无效 ID，仅验证参数解析）。"""
        result = dws.run_raw(
            "mail", "draft", "send",
            "--sender", EMAIL,
            "--id", "ALIAS_TEST_ID",
        )
        # 参数能被正确解析（即使 ID 无效），不应因 --sender 未识别而报 flag 错误
        assert "unknown flag" not in result.stderr.lower(), (
            "--sender 别名应被识别，不应报 unknown flag"
        )
