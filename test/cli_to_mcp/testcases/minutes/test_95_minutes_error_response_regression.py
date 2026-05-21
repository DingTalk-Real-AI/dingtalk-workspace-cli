"""场景2回归: 错误响应诊断 - 无效 ID 应返回明确错误而非空响应。

验证 get summary/info/transcription/todos 在传入无效 taskUuid 时，
CLI 返回可诊断的错误信息（而非 stdout 完全为空），帮助 LLM 快速判断
错误原因而非盲目重试。
"""

class TestInvalidIdErrorDiagnostics:
    """无效 taskUuid 应返回可识别的错误信息。"""

    INVALID_ID = "INVALID_TASK_UUID_00000000"

    def test_get_summary_invalid_id_has_error(self, dws):
        """get summary 无效 ID 应返回错误而非空响应。"""
        result = dws.run_raw("minutes", "get", "summary", "--id", self.INVALID_ID)
        combined = (result.stdout or "") + (result.stderr or "")
        # 至少应返回非空内容（错误信息或错误码）
        assert len(combined.strip()) > 0, "无效 ID 不应返回完全为空的响应"

    def test_get_info_invalid_id_has_error(self, dws):
        """get info 无效 ID 应返回错误而非空响应。"""
        result = dws.run_raw("minutes", "get", "info", "--id", self.INVALID_ID)
        combined = (result.stdout or "") + (result.stderr or "")
        assert len(combined.strip()) > 0, "无效 ID 不应返回完全为空的响应"

    def test_get_todos_invalid_id_has_error(self, dws):
        """get todos 无效 ID 应返回错误而非空响应。"""
        result = dws.run_raw("minutes", "get", "todos", "--id", self.INVALID_ID)
        combined = (result.stdout or "") + (result.stderr or "")
        assert len(combined.strip()) > 0, "无效 ID 不应返回完全为空的响应"

    def test_get_transcription_invalid_id_has_error(self, dws):
        """get transcription 无效 ID 应返回错误而非空响应。"""
        result = dws.run_raw("minutes", "get", "transcription", "--id", self.INVALID_ID)
        combined = (result.stdout or "") + (result.stderr or "")
        assert len(combined.strip()) > 0, "无效 ID 不应返回完全为空的响应"

    def test_get_keywords_invalid_id_has_error(self, dws):
        """get keywords 无效 ID 应返回错误而非空响应。"""
        result = dws.run_raw("minutes", "get", "keywords", "--id", self.INVALID_ID)
        combined = (result.stdout or "") + (result.stderr or "")
        assert len(combined.strip()) > 0, "无效 ID 不应返回完全为空的响应"

class TestMissingIdErrorMessage:
    """缺少必填 --id 参数应给出明确提示。"""

    def test_get_summary_missing_id(self, dws):
        """get summary 不传 --id 应报 required 错误。"""
        result = dws.run_raw("minutes", "get", "summary")
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "required" in combined.lower() or "error" in combined.lower()

    def test_get_info_missing_id(self, dws):
        """get info 不传 --id 应报 required 错误。"""
        result = dws.run_raw("minutes", "get", "info")
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "required" in combined.lower() or "error" in combined.lower()

    def test_get_todos_missing_id(self, dws):
        """get todos 不传 --id 应报 required 错误。"""
        result = dws.run_raw("minutes", "get", "todos")
        combined = (result.stdout or "") + (result.stderr or "")
        assert result.returncode != 0 or "required" in combined.lower() or "error" in combined.lower()
