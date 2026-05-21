"""
test_04_minutes_update_summary.py — 听记更新纪要内容测试 (1 command × 3 cases)

Commands tested:
  1. dws minutes update summary  (update_minutes_summary)
"""

import time
from typing import Optional

import pytest


def _extract_items_from_list(data: dict) -> list:
    """从 list 返回的多种可能结构中提取 items 列表。"""
    result = data.get("result", {})
    if isinstance(result, dict):
        items = result.get("itemList", []) or result.get("minutes", []) or result.get("items", [])
        if items:
            return items
    if isinstance(result, list):
        return result
    return data.get("minutes", []) or data.get("itemList", []) or data.get("items", [])


def _extract_id_from_item(item: dict) -> Optional[str]:
    """从单个听记 item 中提取 ID。"""
    return (
        item.get("taskUuid")
        or item.get("minutesId")
        or item.get("uuid")
        or item.get("id")
    )


@pytest.fixture(scope="session")
def minutes_id(dws):
    """获取听记 ID，优先用 list all，其次 list shared。"""
    for subcmd in ("all", "shared"):
        data = dws.run_ok("minutes", "list", subcmd)
        items = _extract_items_from_list(data)
        if items:
            mid = _extract_id_from_item(items[0])
            if mid:
                return mid
    pytest.skip("No minutes available")


class TestMinutesUpdateSummary:
    """dws minutes update summary"""

    def test_update_summary(self, dws, minutes_id):
        """更新纪要内容为纯文本。"""
        new_content = f"CLI_Test_Summary_{int(time.time())}"
        dws.run_ok(
            "minutes", "update", "summary",
            "--id", minutes_id, "--content", new_content,
        )

    def test_update_summary_chinese(self, dws, minutes_id):
        """更新纪要内容为中文。"""
        dws.run_ok(
            "minutes", "update", "summary",
            "--id", minutes_id,
            "--content", f"中文纪要内容_{int(time.time())}",
        )

    def test_update_summary_invalid_id(self, dws):
        """使用无效 ID 更新纪要应报错。"""
        result = dws.run_raw(
            "minutes", "update", "summary",
            "--id", "INVALID",
            "--content", "test",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_update_summary_missing_content(self, dws, minutes_id):
        """缺少 --content 参数应报错。"""
        result = dws.run_raw(
            "minutes", "update", "summary",
            "--id", minutes_id,
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_update_summary_missing_id(self, dws):
        """缺少 --id 参数应报错。"""
        result = dws.run_raw(
            "minutes", "update", "summary",
            "--content", "test",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()
