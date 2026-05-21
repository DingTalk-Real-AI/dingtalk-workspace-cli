"""
test_08_minutes_replace_text.py — 听记文本替换测试 (1 command × 4 cases)

Commands tested:
  1. dws minutes replace-text  (replace_minutes_text)
"""

import pytest


@pytest.fixture(scope="session")
def minutes_id(dws):
    """获取听记 ID，优先用 list all，其次 list shared。"""
    for subcmd in ("all", "shared"):
        data = dws.run_ok("minutes", "list", subcmd)
        result = data.get("result", {})
        items = (
            result.get("itemList", [])
            if isinstance(result, dict)
            else []
        ) or data.get("minutes", [])
        if items:
            mid = items[0].get("minutesId") or items[0].get("uuid") or items[0].get("id")
            if mid:
                return mid
    pytest.skip("No minutes available")


class TestReplaceText:
    """dws minutes replace-text"""

    def test_replace_text(self, dws, minutes_id):
        """查找替换文本（使用不存在的文字，验证命令语法正确）。"""
        result = dws.run_raw(
            "minutes", "replace-text",
            "--id", minutes_id,
            "--search", "不存在的旧文字_XYZ",
            "--replace", "新文字",
        )
        # 命令语法应正确，不应报 unknown flag
        assert "unknown flag" not in result.stderr

    def test_replace_text_chinese(self, dws, minutes_id):
        """查找替换中文文本。"""
        result = dws.run_raw(
            "minutes", "replace-text",
            "--id", minutes_id,
            "--search", "测试旧文字",
            "--replace", "测试新文字",
        )
        assert "unknown flag" not in result.stderr

    def test_replace_text_invalid_id(self, dws):
        """使用无效 ID 替换文本应报错。"""
        result = dws.run_raw(
            "minutes", "replace-text",
            "--id", "INVALID",
            "--search", "A",
            "--replace", "B",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_replace_text_missing_search(self, dws, minutes_id):
        """缺少 --search 参数应报错。"""
        result = dws.run_raw(
            "minutes", "replace-text",
            "--id", minutes_id,
            "--replace", "B",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_replace_text_missing_replace(self, dws, minutes_id):
        """缺少 --replace 参数应报错。"""
        result = dws.run_raw(
            "minutes", "replace-text",
            "--id", minutes_id,
            "--search", "A",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()
