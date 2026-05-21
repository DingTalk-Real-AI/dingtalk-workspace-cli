"""
test_02_minutes_detail.py — 闪记详情和更新测试 (6 commands × 3 cases)

Commands tested:
  5. dws minutes get keywords      (get_minutes_keywords)
  6. dws minutes get transcription (get_minutes_transcription)
  7. dws minutes get todos         (get_minutes_todos)
  8. dws minutes get batch         (batch_get_minutes_headers)
  9. dws minutes update title      (update_minutes_title)
 10. dws minutes get audio         (query_minutes_audio_url)
"""

import pytest


import time



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


class TestMinutesGetKeywords:
    """dws minutes get keywords"""

    def test_get_keywords(self, dws, minutes_id):
        """获取听记关键词。"""
        data = dws.run_ok(
            "minutes", "get", "keywords", "--id", minutes_id,
        )
        assert isinstance(data, dict) and len(data) > 0

    def test_keywords_structure(self, dws, minutes_id):
        """关键词应为有效数据。"""
        data = dws.run_ok(
            "minutes", "get", "keywords", "--id", minutes_id,
        )
        assert isinstance(data, dict) and len(data) > 0

    def test_keywords_invalid(self, dws):
        """无效 ID 获取关键词。"""
        result = dws.run_raw(
            "minutes", "get", "keywords", "--id", "INVALID",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestMinutesGetTranscription:
    """dws minutes get transcription"""

    def test_get_transcription(self, dws, minutes_id):
        """获取听记转录文本。"""
        data = dws.run_ok(
            "minutes", "get", "transcription",
            "--id", minutes_id,
        )
        assert isinstance(data, dict) and len(data) > 0

    def test_transcription_structure(self, dws, minutes_id):
        """转录内容应为有效数据。"""
        data = dws.run_ok(
            "minutes", "get", "transcription",
            "--id", minutes_id,
        )
        assert isinstance(data, dict) and len(data) > 0

    def test_transcription_invalid(self, dws):
        """无效 ID 获取转录。"""
        result = dws.run_raw(
            "minutes", "get", "transcription",
            "--id", "INVALID",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestMinutesGetTodos:
    """dws minutes get todos"""

    def test_get_todos(self, dws, minutes_id):
        """获取听记待办。"""
        data = dws.run_ok(
            "minutes", "get", "todos", "--id", minutes_id,
        )
        assert isinstance(data, dict) and len(data) > 0

    def test_todos_structure(self, dws, minutes_id):
        """待办应为有效数据。"""
        data = dws.run_ok(
            "minutes", "get", "todos", "--id", minutes_id,
        )
        assert isinstance(data, dict) and len(data) > 0

    def test_todos_invalid(self, dws):
        """无效 ID 获取待办。"""
        result = dws.run_raw(
            "minutes", "get", "todos", "--id", "INVALID",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestMinutesGetBatch:
    """dws minutes get batch"""

    def test_batch_single(self, dws, minutes_id):
        """批量获取单个听记头信息。"""
        data = dws.run_ok(
            "minutes", "get", "batch", "--ids", minutes_id,
        )
        assert isinstance(data, dict) and len(data) > 0

    def test_batch_multiple(self, dws, minutes_id):
        """批量获取多个（含重复）。"""
        ids = f"{minutes_id},{minutes_id}"
        data = dws.run_ok(
            "minutes", "get", "batch", "--ids", ids,
        )
        assert isinstance(data, dict) and len(data) > 0

    def test_batch_invalid(self, dws):
        """批量获取无效 ID。"""
        data = dws.run_ok(
            "minutes", "get", "batch",
            "--ids", "INVALID_1,INVALID_2",
        )
        assert isinstance(data, dict)


class TestMinutesGetAudio:
    """dws minutes get audio"""

    def test_get_audio(self, dws, minutes_id):
        """获取听记音频/视频地址。"""
        data = dws.run_ok(
            "minutes", "get", "audio", "--id", minutes_id,
        )
        assert isinstance(data, dict) and len(data) > 0

    def test_audio_structure(self, dws, minutes_id):
        """音频地址应为有效数据。"""
        data = dws.run_ok(
            "minutes", "get", "audio", "--id", minutes_id,
        )
        assert isinstance(data, dict) and len(data) > 0

    def test_audio_invalid(self, dws):
        """无效 ID 获取音频地址。"""
        result = dws.run_raw(
            "minutes", "get", "audio", "--id", "INVALID",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

class TestMinutesUpdateTitle:
    """dws minutes update title"""

    def test_update_title(self, dws, minutes_id):
        """修改听记标题。"""
        new_title = f"CLI_Test_Renamed_{int(time.time())}"
        dws.run_ok(
            "minutes", "update", "title",
            "--id", minutes_id, "--title", new_title,
        )

    def test_update_chinese_title(self, dws, minutes_id):
        """修改为中文标题。"""
        dws.run_ok(
            "minutes", "update", "title",
            "--id", minutes_id,
            "--title", f"中文标题_{int(time.time())}",
        )

    def test_update_title_invalid(self, dws):
        """修改无效 ID 标题。"""
        result = dws.run_raw(
            "minutes", "update", "title",
            "--id", "INVALID",
            "--title", "X",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
