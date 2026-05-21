"""
test_09_minutes_upload.py — 听记文件上传测试 (3 commands × N cases)

Commands tested:
  1. dws minutes upload create   (create_upload_session)
  2. dws minutes upload complete (complete_upload_session)
  3. dws minutes upload cancel   (cancel_upload_session)

上传完整流程：
  1. create  → 获取 presignedUrl + sessionId
  2. HTTP PUT presignedUrl 上传文件（集成测试中跳过实际上传）
  3. complete → 创建听记（需要真实上传，此处仅验证命令语法）
  4. cancel  → 取消会话（使用 create 返回的 sessionId）
"""

import pytest


class TestUploadCreate:
    """dws minutes upload create — create_upload_session"""

    def test_create_upload_session_required_flags(self, dws):
        """验证 create 命令接受 --file-name 和 --file-size 参数（dry-run 模式）。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-name", "meeting.mp4",
            "--file-size", "102400",
            "--dry-run",
        )
        # dry-run 模式下不应报 unknown flag 或参数解析错误
        assert "unknown flag" not in result.stderr
        assert "unknown flag" not in result.stdout

    def test_create_upload_session_with_title(self, dws):
        """验证 create 命令接受可选 --title 参数。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-name", "meeting.mp4",
            "--file-size", "102400",
            "--title", "周会录音",
            "--dry-run",
        )
        assert "unknown flag" not in result.stderr

    def test_create_upload_session_with_input_language(self, dws):
        """验证 create 命令接受可选 --input-language 参数。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-name", "meeting.mp4",
            "--file-size", "102400",
            "--input-language", "zh",
            "--dry-run",
        )
        assert "unknown flag" not in result.stderr

    def test_create_upload_session_with_enable_message_card(self, dws):
        """验证 create 命令接受可选 --enable-message-card 参数。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-name", "meeting.mp4",
            "--file-size", "102400",
            "--enable-message-card",
            "--dry-run",
        )
        assert "unknown flag" not in result.stderr

    def test_create_upload_session_with_template_id(self, dws):
        """验证 create 命令接受可选 --template-id 参数。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-name", "meeting.mp4",
            "--file-size", "102400",
            "--template-id", "tmpl_001",
            "--dry-run",
        )
        assert "unknown flag" not in result.stderr

    def test_create_upload_session_missing_file_name(self, dws):
        """缺少 --file-name 参数应报错。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-size", "102400",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_create_upload_session_missing_file_size(self, dws):
        """缺少 --file-size 参数应报错（file-size <= 0）。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-name", "meeting.mp4",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_create_upload_session_zero_file_size(self, dws):
        """--file-size 为 0 应报错。"""
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-name", "meeting.mp4",
            "--file-size", "0",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_create_upload_session_live(self, dws):
        """实际调用 create_upload_session，验证返回 sessionId 和 presignedUrl。"""
        import json as _json
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-name", "test_meeting.mp4",
            "--file-size", "1024",
        )
        if result.returncode != 0:
            pytest.skip(f"upload create returned non-zero: {result.returncode}")
        try:
            data = _json.loads(result.stdout or "{}")
        except _json.JSONDecodeError:
            pytest.skip("upload create returned non-JSON response")
        if "error" in data:
            pytest.skip(f"upload create returned server error: {data['error'].get('message', '')[:100]}")
        assert data is not None
        # 返回结果应包含 sessionId 或 presignedUrl 字段
        result_str = str(data)
        assert "sessionId" in result_str or "presignedUrl" in result_str or "uploadUrl" in result_str

    def test_create_upload_session_url_not_escaped(self, dws):
        """验证 presignedUrl 中的 & 没有被转义为 \\u0026。

        修复背景：Go 的 json.Encoder 默认开启 HTML 转义，会将 & 转义为 \\u0026，
        导致返回的 presignedUrl 无法直接用于 curl -X PUT 上传。
        upload 命令使用 callMCPToolUnescaped 输出，应保留原始 &。
        """
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-name", "url_escape_test.mp4",
            "--file-size", "1024",
        )
        # 跳过 API 调用失败的情况（如未登录、权限不足等）
        if result.returncode != 0:
            pytest.skip("API call failed, skipping URL escape test")
        stdout = result.stdout
        if "error" in stdout.lower() and "presignedUrl" not in stdout:
            pytest.skip("API returned error, skipping URL escape test")

        # 核心断言：原始输出中不应包含 \u0026
        assert "\\u0026" not in stdout, (
            f"presignedUrl should not contain \\u0026 (escaped &), got:\n{stdout[:500]}"
        )
        # 如果包含 presignedUrl，验证其中有原始的 &
        if "presignedUrl" in stdout:
            assert "&" in stdout, (
                f"presignedUrl should contain raw & for query params, got:\n{stdout[:500]}"
            )


class TestUploadComplete:
    """dws minutes upload complete — complete_upload_session"""

    def test_complete_upload_session_missing_session_id(self, dws):
        """缺少 --session-id 参数应报错。"""
        result = dws.run_raw("minutes", "upload", "complete")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_complete_upload_session_invalid_session_id(self, dws):
        """使用无效 sessionId 应报错。"""
        result = dws.run_raw(
            "minutes", "upload", "complete",
            "--session-id", "INVALID_SESSION_ID_XYZ",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_complete_upload_session_dry_run(self, dws):
        """dry-run 模式下验证命令语法正确。"""
        result = dws.run_raw(
            "minutes", "upload", "complete",
            "--session-id", "test_session_id",
            "--dry-run",
        )
        assert "unknown flag" not in result.stderr


class TestUploadCancel:
    """dws minutes upload cancel — cancel_upload_session"""

    def test_cancel_upload_session_missing_session_id(self, dws):
        """缺少 --session-id 参数应报错。"""
        result = dws.run_raw("minutes", "upload", "cancel")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

    def test_cancel_upload_session_invalid_session_id(self, dws):
        """使用无效 sessionId 应报错。"""
        result = dws.run_raw(
            "minutes", "upload", "cancel",
            "--session-id", "INVALID_SESSION_ID_XYZ",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_cancel_upload_session_dry_run(self, dws):
        """dry-run 模式下验证命令语法正确。"""
        result = dws.run_raw(
            "minutes", "upload", "cancel",
            "--session-id", "test_session_id",
            "--dry-run",
        )
        assert "unknown flag" not in result.stderr

    def test_cancel_upload_session_live(self, dws):
        """先 create 再 cancel，验证取消流程。"""
        # Step 1: create session（使用 run_raw 避免服务端异常时直接 fail）
        import json
        result = dws.run_raw(
            "minutes", "upload", "create",
            "--file-name", "cancel_test.mp4",
            "--file-size", "1024",
        )
        if result.returncode != 0:
            pytest.skip(f"create_upload_session returned non-zero: {result.returncode}")
        try:
            create_data = json.loads(result.stdout or "{}")
        except json.JSONDecodeError:
            pytest.skip("create_upload_session returned non-JSON response")
        if "error" in create_data:
            pytest.skip(f"create_upload_session returned error: {create_data['error'].get('message', '')[:100]}")
        if create_data is None:
            pytest.skip("create_upload_session failed, skipping cancel test")

        # 提取 sessionId（优先从顶层取，其次从 result 嵌套对象中取）
        session_id = None
        if isinstance(create_data, dict):
            session_id = create_data.get("sessionId")
            if not session_id and isinstance(create_data.get("result"), dict):
                session_id = create_data["result"].get("sessionId")
        if not session_id:
            pytest.skip("Cannot extract sessionId from create response")

        # Step 2: cancel session
        data = dws.run_ok(
            "minutes", "upload", "cancel",
            "--session-id", session_id,
        )
        assert data is not None
