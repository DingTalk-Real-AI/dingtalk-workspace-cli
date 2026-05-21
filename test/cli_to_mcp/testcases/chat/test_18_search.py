"""
test_18_search.py — 群聊搜索测试

Commands tested:
  1. dws chat search (search_groups，支持分页参数 --limit / --cursor)
"""

import json
import pytest


def _run_search(dws, *args):
    """执行 chat search 并容忍后端偶发的 '系统繁忙' 错误。"""
    proc = dws.run_raw("chat", "search", *args)
    combined = (proc.stdout or "") + (proc.stderr or "")
    try:
        data = json.loads(combined.strip())
    except json.JSONDecodeError:
        pytest.fail(f"chat search 返回非 JSON: {combined[:300]}")
    err = data.get("error", {})
    if err.get("server_error_code") == "1001" or "系统繁忙" in str(err.get("message", "")):
        pytest.skip(f"后端偶发错误(系统繁忙)，跳过: {err.get('message', '')}")
    if data.get("success") is not True and err:
        pytest.fail(f"chat search 失败: {json.dumps(data, ensure_ascii=False)[:300]}")
    return data


class TestChatSearch:
    """dws chat search"""

    def test_search_basic(self, dws):
        """基本搜索，验证返回列表。"""
        data = _run_search(dws, "--keyword", "测试")
        assert data.get("success") is True, f"success 应为 True: {data}"
        result = data.get("result", {})
        groups = result.get("groups") or result.get("items") or []
        assert isinstance(groups, list), f"搜索结果应为列表: {result}"

    def test_search_with_limit(self, dws):
        """带 limit 参数搜索。"""
        data = _run_search(dws, "--keyword", "测试", "--limit", "3")
        assert data.get("success") is True, f"success 应为 True: {data}"
        result = data.get("result", {})
        groups = result.get("groups") or result.get("items") or []
        assert isinstance(groups, list), f"搜索结果应为列表: {result}"
        assert len(groups) <= 3, f"结果数应 <= 3，实际 {len(groups)}"

    def test_search_with_cursor(self, dws):
        """带 cursor 参数翻页。"""
        data = _run_search(
            dws,
            "--keyword", "测试",
            "--limit", "2",
            "--cursor", "0",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        result = data.get("result", {})
        groups = result.get("groups") or result.get("items") or []
        assert isinstance(groups, list), f"搜索结果应为列表: {result}"

    def test_search_missing_keyword(self, dws):
        """不传 keyword 应报错（必填）。"""
        result = dws.run_raw("chat", "search")
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_search_empty_keyword(self, dws):
        """空 keyword 应返回错误或空结果。"""
        result = dws.run_raw("chat", "search", "--keyword", "")
        combined = (result.stdout or "") + (result.stderr or "")
        assert (
            result.returncode != 0
            or "error" in combined.lower()
            or '"items": []' in combined
            or '"groupList": []' in combined
        )
