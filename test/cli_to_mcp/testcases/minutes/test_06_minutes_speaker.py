"""
test_06_minutes_speaker.py — 听记发言人管理测试 (1 command × 4 cases)

Commands tested:
  1. dws minutes speaker replace  (replace_speaker)
"""

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


class TestSpeakerReplace:
    """dws minutes speaker replace"""

    def test_replace_speaker(self, dws, minutes_id):
        """替换发言人（使用不存在的发言人名称，验证命令语法正确）。"""
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", minutes_id,
            "--from", "不存在的发言人",
            "--to", "新发言人",
        )
        # 命令语法应正确，即使发言人不存在也不应报 unknown flag
        assert "unknown flag" not in result.stderr

    def test_replace_speaker_with_target_uid(self, dws, minutes_id):
        """替换发言人并指定 target-uid。"""
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", minutes_id,
            "--from", "不存在的发言人",
            "--to", "新发言人",
            "--target-uid", "test_uid_123",
        )
        assert "unknown flag" not in result.stderr

    def test_replace_speaker_invalid_id(self, dws):
        """使用无效 ID 替换发言人应报错。"""
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", "INVALID",
            "--from", "A",
            "--to", "B",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_replace_speaker_missing_required(self, dws, minutes_id):
        """缺少必填参数 --from 应报错。"""
        result = dws.run_raw(
            "minutes", "speaker", "replace",
            "--id", minutes_id,
            "--to", "B",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()
