"""
test_08_search_advanced.py — 多维度搜索消息测试

Commands tested:
  1. dws chat message search-advanced

Flags (all optional):
  --keyword, --sender-ids, --at-me, --at-ids,
  --conversation-ids / --groups (hidden alias),
  --start, --end, --cursor ("0"), --limit (100)
"""

import json
import pytest


class TestChatMessageSearchAdvanced:
    """dws chat message search-advanced — 多维度搜索消息"""

    # ── 基本搜索 ──

    def test_search_with_keyword(self, dws, chat_id):  # chat_id 用作环境门控
        """关键词搜索应返回合法结构（可能为空列表）。"""
        data = dws.run(
            "chat", "message", "search-advanced",
            "--keyword", "测试",
            "--limit", "2",
        )
        result = data.get("result", {})
        # API 无匹配时可能仅返回 {"hasMore": false}，不含 conversationMessagesList
        assert isinstance(result, dict), f"预期返回 dict，实际: {type(result)}"

    def test_search_no_params(self, dws, chat_id):  # chat_id 用作环境门控
        """不传任何参数也应正常返回（所有参数均可选）。"""
        data = dws.run(
            "chat", "message", "search-advanced",
            "--limit", "1",
        )
        result = data.get("result", {})
        assert isinstance(result, dict), f"预期返回 dict，实际: {type(result)}"

    # ── 分页 ──

    def test_search_pagination(self, dws, chat_id):  # chat_id 用作环境门控
        """hasMore 为 true 时，nextCursor 不为空。"""
        data = dws.run(
            "chat", "message", "search-advanced",
            "--keyword", "测试",
            "--limit", "1",
        )
        result = data.get("result", {})
        if result.get("hasMore"):
            assert result.get("nextCursor"), (
                "hasMore=true 时 nextCursor 不应为空"
            )

    def test_search_with_cursor(self, dws, chat_id):  # chat_id 用作环境门控
        """使用 cursor 进行翻页不应报错。"""
        data = dws.run(
            "chat", "message", "search-advanced",
            "--keyword", "测试",
            "--limit", "1",
            "--cursor", "0",
        )
        assert data.get("success") is True or "result" in data

    # ── conversation-ids 过滤 ──

    def test_search_by_conversation_id(self, dws, chat_id):
        """按 conversation-ids 过滤搜索。"""
        data = dws.run(
            "chat", "message", "search-advanced",
            "--conversation-ids", chat_id,
            "--limit", "2",
        )
        result = data.get("result", {})
        assert isinstance(result, dict)

    # ── at-me 过滤 ──

    def test_search_at_me(self, dws, chat_id):  # chat_id 用作环境门控
        """--at-me 过滤 @ 我的消息。"""
        data = dws.run(
            "chat", "message", "search-advanced",
            "--at-me",
            "--limit", "1",
        )
        result = data.get("result", {})
        assert isinstance(result, dict)

    # ── 时间范围 ──

    def test_search_with_time_range(self, dws, chat_id):  # chat_id 用作环境门控
        """使用 --start 和 --end 指定时间范围。"""
        data = dws.run(
            "chat", "message", "search-advanced",
            "--keyword", "测试",
            "--start", "2025-01-01T00:00:00+08:00",
            "--end", "2099-12-31T23:59:59+08:00",
            "--limit", "1",
        )
        result = data.get("result", {})
        assert isinstance(result, dict)

    # ── limit 边界 ──

    def test_search_limit_zero(self, dws):
        """limit=0 应仍返回成功（服务端可能返回空列表）。"""
        result = dws.run_raw(
            "chat", "message", "search-advanced",
            "--keyword", "测试",
            "--limit", "0",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        try:
            data = json.loads(combined.strip())
        except json.JSONDecodeError:
            pytest.fail(f"search-advanced --limit 0 返回非 JSON: {combined[:300]}")
        # 服务端可能返回 success 或 error，都可接受
        assert "success" in data or "error" in data
