"""minutes 录音控制命令测试。"""

import json

import pytest


def _extract_uuid(payload):
    """尽量从响应体中提取听记 uuid/taskUuid。"""
    if isinstance(payload, dict):
        for key in ("taskUuid", "uuid", "minutesId", "id"):
            value = payload.get(key)
            if isinstance(value, str) and value:
                return value
        for value in payload.values():
            found = _extract_uuid(value)
            if found:
                return found
    elif isinstance(payload, list):
        for item in payload:
            found = _extract_uuid(item)
            if found:
                return found
    return ""


@pytest.fixture()
def active_record_uuid(dws):
    """创建一个进行中的听记并提取 uuid。"""
    result = dws.run_raw("minutes", "record", "start")
    if "unknown flag" in (result.stderr or "").lower():
        pytest.fail(f"record start 出现未知参数错误: {result.stderr[:200]}")
    try:
        data = json.loads(result.stdout or "{}")
    except json.JSONDecodeError:
        pytest.skip("record start 返回非 JSON，无法提取 uuid")
    record_uuid = _extract_uuid(data)
    if not record_uuid:
        pytest.skip("record start 未返回可用 uuid/taskUuid")
    return record_uuid


class TestMinutesRecordStart:
    """dws minutes record start"""

    def test_record_start(self, dws):
        """发起听记。"""
        data = dws.run_ok("minutes", "record", "start")
        assert data is not None

    def test_record_start_with_session_id(self, dws):
        """带 session-id 发起听记，至少应通过 CLI 参数解析。"""
        result = dws.run_raw(
            "minutes", "record", "start", "--session-id", "test-session-id",
        )
        assert "unknown flag" not in (result.stderr or "").lower()

    def test_record_start_idempotent(self, dws):
        """重复调用应保持稳定（至少无未知参数错误）。"""
        r1 = dws.run_raw("minutes", "record", "start")
        r2 = dws.run_raw("minutes", "record", "start")
        assert "unknown flag" not in (r1.stderr or "").lower()
        assert "unknown flag" not in (r2.stderr or "").lower()


class TestMinutesRecordPause:
    """dws minutes record pause"""

    def test_record_pause(self, dws, active_record_uuid):
        """暂停听记。"""
        result = dws.run_raw("minutes", "record", "pause", "--id", active_record_uuid)
        assert "unknown flag" not in (result.stderr or "").lower()

    def test_record_pause_invalid_id(self, dws):
        """无效 ID 暂停听记。"""
        result = dws.run_raw("minutes", "record", "pause", "--id", "INVALID")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_record_pause_missing_id(self, dws):
        """缺少必填 ID。"""
        result = dws.run_raw("minutes", "record", "pause")
        assert result.returncode != 0
        combined = ((result.stdout or "") + (result.stderr or "")).lower()
        assert "required" in combined or "flag" in combined


class TestMinutesRecordResume:
    """dws minutes record resume"""

    def test_record_resume(self, dws, active_record_uuid):
        """恢复听记。"""
        result = dws.run_raw("minutes", "record", "resume", "--id", active_record_uuid)
        assert "unknown flag" not in (result.stderr or "").lower()

    def test_record_resume_invalid_id(self, dws):
        """无效 ID 恢复听记。"""
        result = dws.run_raw("minutes", "record", "resume", "--id", "INVALID")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_record_resume_missing_id(self, dws):
        """缺少必填 ID。"""
        result = dws.run_raw("minutes", "record", "resume")
        assert result.returncode != 0
        combined = ((result.stdout or "") + (result.stderr or "")).lower()
        assert "required" in combined or "flag" in combined


class TestMinutesRecordStop:
    """dws minutes record stop"""

    def test_record_stop(self, dws, active_record_uuid):
        """结束听记。"""
        result = dws.run_raw("minutes", "record", "stop", "--id", active_record_uuid)
        assert "unknown flag" not in (result.stderr or "").lower()

    def test_record_stop_invalid_id(self, dws):
        """无效 ID 结束听记。"""
        result = dws.run_raw("minutes", "record", "stop", "--id", "INVALID")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_record_stop_missing_id(self, dws):
        """缺少必填 ID。"""
        result = dws.run_raw("minutes", "record", "stop")
        assert result.returncode != 0
        combined = ((result.stdout or "") + (result.stderr or "")).lower()
        assert "required" in combined or "flag" in combined
