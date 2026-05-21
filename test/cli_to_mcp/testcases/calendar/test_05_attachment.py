"""
test_05_attachment.py — 日程附件管理测试 (1 command × 3+ cases)

Commands tested:
  1. dws calendar attachment add  (add_attachments)

注意：本用例只覆盖 CLI 解析与服务端的错误路径，**不**走真实钉盘上传，
避免在测试账号下污染钉盘文件。
"""

import pytest


class TestAttachmentAdd:
    """dws calendar attachment add"""

    def test_add_invalid_file_should_fail(self, dws, test_event_id):
        """传伪 fileId 应在服务端被拒绝（fileId 不存在或无权限）。"""
        result = dws.run_raw(
            "calendar", "attachment", "add",
            "--event", test_event_id,
            "--files", "FAKE_FILE_ID_99999:demo.pdf",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), "fake fileId should be rejected by server"

    def test_add_to_invalid_event(self, dws):
        """向无效日程添加附件应报错。"""
        result = dws.run_raw(
            "calendar", "attachment", "add",
            "--event", "INVALID_EVENT_99999",
            "--files", "FAKE_FILE_ID_99999:demo.pdf",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_add_malformed_files_flag(self, dws, test_event_id):
        """--files 元素必须形如 <fileId>:<name>，缺冒号应被 CLI 校验拒绝。"""
        result = dws.run_raw(
            "calendar", "attachment", "add",
            "--event", test_event_id,
            "--files", "FAKE_FILE_ID_NO_COLON",
        )
        assert (
            result.returncode != 0
            or "invalid" in result.stdout.lower()
            or "invalid" in result.stderr.lower()
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        ), "malformed --files (no colon) should be rejected"

    def test_add_multiple_files_parsed(self, dws, test_event_id):
        """多附件以逗号分隔；CLI 不应报 unknown flag，由服务端决定结果。"""
        result = dws.run_raw(
            "calendar", "attachment", "add",
            "--event", test_event_id,
            "--files", "FAKE_ID_A:a.pdf,FAKE_ID_B:b.pdf",
        )
        assert "unknown flag" not in result.stderr
        assert result.returncode is not None
