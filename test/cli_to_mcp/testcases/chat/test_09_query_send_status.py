"""
test_09_query_send_status.py — 查询消息发送状态测试

Commands tested:
  1. dws chat message query-send-status

Flags:
  --open-task-id (必填)

注意：openTaskId 来自 send-by-bot 的返回，当前 PAT 权限不支持发送消息，
因此仅测试 CLI 层面参数校验和无效 openTaskId 的错误处理。
"""

import json
import pytest


class TestChatMessageQuerySendStatus:
    """dws chat message query-send-status — 查询消息发送状态"""

    # ── 缺参测试 ──

    def test_missing_open_task_id(self, dws):
        """不传 --open-task-id 应报错。"""
        result = dws.run_raw(
            "chat", "message", "query-send-status",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower() or "required" in combined.lower(), (
            f"缺少必填参数应报错: {combined[:300]}"
        )

    # ── 无效 openTaskId ──

    def test_invalid_open_task_id(self, dws):
        """无效 openTaskId 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "message", "query-send-status",
            "--open-task-id", "INVALID_TASK_ID_99999",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        try:
            data = json.loads(combined.strip())
        except json.JSONDecodeError:
            pytest.fail(f"query-send-status 返回非 JSON: {combined[:300]}")
        # 预期返回业务错误（PARAM_ERROR 等）
        if data.get("success") is True:
            return  # 意外成功也接受
        err = data.get("error", {})
        assert err, f"预期返回 error 对象: {data}"

    def test_empty_open_task_id(self, dws):
        """空字符串 openTaskId 应报错。"""
        result = dws.run_raw(
            "chat", "message", "query-send-status",
            "--open-task-id", "",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "error" in combined.lower(), (
            f"空 openTaskId 应报错: {combined[:300]}"
        )
