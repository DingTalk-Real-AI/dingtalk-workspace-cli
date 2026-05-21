"""
test_07_minutes_hot_word.py — 听记个人热词管理测试 (1 command × 3 cases)

Commands tested:
  1. dws minutes hot-word add  (add_personal_hot_word)
"""

import pytest


class TestHotWordAdd:
    """dws minutes hot-word add"""

    def test_add_single_hot_word(self, dws):
        """添加单个热词。"""
        data = dws.run_ok(
            "minutes", "hot-word", "add", "--words", "钉钉",
        )
        assert data is not None

    def test_add_multiple_hot_words(self, dws):
        """添加多个热词（逗号分隔）。"""
        data = dws.run_ok(
            "minutes", "hot-word", "add", "--words", "OKR,钉钉,Copilot",
        )
        assert data is not None

    def test_add_hot_word_missing_words(self, dws):
        """缺少 --words 参数应报错。"""
        result = dws.run_raw("minutes", "hot-word", "add")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()
