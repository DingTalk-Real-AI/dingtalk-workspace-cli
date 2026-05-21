"""
test_25_bot_find.py — 搜索机器人测试

Commands tested:
  1. dws chat bot find  (search_bots — 按关键词搜索当前用户可用的机器人，支持游标分页)

与 dws chat bot search 的区别：
  - search → search_my_robots：仅返回我创建的机器人
  - find   → search_bots：返回当前用户可用的全部机器人
"""

import pytest

from conftest import _parse_json, skip_if_backend_tool_missing


class TestChatBotFind:
    """dws chat bot find"""

    def test_find_basic(self, dws):
        """关键词搜索机器人 — 正常路径。"""
        proc = dws.run_raw(
            "chat", "bot", "find",
            "--keyword", "钉",
        )
        data = _parse_json(proc)
        skip_if_backend_tool_missing(data)
        assert data is not None and data.get("success") is True, f"success 应为 True: {data}"

    def test_find_with_pagination(self, dws):
        """分页 — 先调一次拿真实 nextCursor，再翻页。

        注意 search_bots 的 cursor 是 String 类型，不能传 \"0\"（网关会被 auto-coerce
        回 Integer 触发 backend pojo 类型不匹配）；首次调用不传 cursor，翻页用
        上次返回的 nextCursor。
        """
        # 首次：不传 cursor
        proc1 = dws.run_raw(
            "chat", "bot", "find",
            "--keyword", "钉",
            "--limit", "3",
        )
        data1 = _parse_json(proc1)
        skip_if_backend_tool_missing(data1)
        assert data1 is not None and data1.get("success") is True, f"首次调用应成功: {data1}"
        next_cursor = (data1.get("result") or {}).get("nextCursor")
        if not next_cursor:
            pytest.skip("首次调用未返回 nextCursor，跳过翻页测试")

        # 翻页：用真实 nextCursor（非数字字符串）
        proc2 = dws.run_raw(
            "chat", "bot", "find",
            "--keyword", "钉",
            "--limit", "3",
            "--cursor", next_cursor,
        )
        data2 = _parse_json(proc2)
        skip_if_backend_tool_missing(data2)
        assert data2 is not None and data2.get("success") is True, f"翻页应成功: {data2}"

    def test_find_missing_keyword(self, dws):
        """不传 --keyword 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "bot", "find",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_find_empty_keyword_match(self, dws):
        """关键词不太可能命中任何机器人时，仍应 success=true（业务上返回空列表）。"""
        proc = dws.run_raw(
            "chat", "bot", "find",
            "--keyword", "_NONEXIST_BOT_KEYWORD_98765_",
            "--limit", "10",
        )
        data = _parse_json(proc)
        skip_if_backend_tool_missing(data)
        assert data is not None and data.get("success") is True, f"success 应为 True: {data}"
