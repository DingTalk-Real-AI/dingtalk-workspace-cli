"""场景1回归: --task-uuid / --uuid / --url 别名不再报 unknown flag。

验证 LLM 常用的 OpenAPI 原生字段名（taskUuid/uuid）作为 flag 名时，
CLI 能正确解析而非返回 unknown flag 错误。
"""

import pytest


class TestGetSummaryFlagAliases:
    """dws minutes get summary 的 flag 别名兼容。"""

    @pytest.fixture(scope="class")
    def sample_id(self, dws):
        data = dws.run_ok("minutes", "list", "mine", "--max", "1")
        items = data.get("result", {}).get("itemList", [])
        if not items:
            pytest.skip("当前账号无听记数据")
        mid = items[0].get("taskUuid") or items[0].get("uuid") or items[0].get("id")
        if not mid:
            pytest.skip("听记项缺少 ID 字段")
        return mid

    def test_task_uuid_flag_accepted(self, dws, sample_id):
        """--task-uuid 应被接受，不报 unknown flag。"""
        result = dws.run_raw("minutes", "get", "summary", "--task-uuid", sample_id)
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower(), (
            f"--task-uuid 不应报 unknown flag: {combined[:300]}"
        )

    def test_uuid_flag_accepted(self, dws, sample_id):
        """--uuid 应被接受，不报 unknown flag。"""
        result = dws.run_raw("minutes", "get", "summary", "--uuid", sample_id)
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower(), (
            f"--uuid 不应报 unknown flag: {combined[:300]}"
        )

    def test_url_flag_accepted(self, dws, sample_id):
        """--url 应被接受，不报 unknown flag。"""
        result = dws.run_raw("minutes", "get", "summary", "--url", sample_id)
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower(), (
            f"--url 不应报 unknown flag: {combined[:300]}"
        )

    def test_id_flag_still_works(self, dws, sample_id):
        """--id（正式 flag）仍然正常工作。"""
        data = dws.run_ok("minutes", "get", "summary", "--id", sample_id)
        assert isinstance(data, dict) and len(data) > 0

class TestGetInfoFlagAliases:
    """dws minutes get info 的 flag 别名兼容。"""

    @pytest.fixture(scope="class")
    def sample_id(self, dws):
        data = dws.run_ok("minutes", "list", "mine", "--max", "1")
        items = data.get("result", {}).get("itemList", [])
        if not items:
            pytest.skip("当前账号无听记数据")
        mid = items[0].get("taskUuid") or items[0].get("uuid") or items[0].get("id")
        if not mid:
            pytest.skip("听记项缺少 ID 字段")
        return mid

    def test_task_uuid_flag_accepted(self, dws, sample_id):
        """get info --task-uuid 应被接受。"""
        result = dws.run_raw("minutes", "get", "info", "--task-uuid", sample_id)
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    def test_uuid_flag_accepted(self, dws, sample_id):
        """get info --uuid 应被接受。"""
        result = dws.run_raw("minutes", "get", "info", "--uuid", sample_id)
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

class TestGetTranscriptionFlagAliases:
    """dws minutes get transcription 的 flag 别名兼容。"""

    @pytest.fixture(scope="class")
    def sample_id(self, dws):
        data = dws.run_ok("minutes", "list", "mine", "--max", "1")
        items = data.get("result", {}).get("itemList", [])
        if not items:
            pytest.skip("当前账号无听记数据")
        mid = items[0].get("taskUuid") or items[0].get("uuid") or items[0].get("id")
        if not mid:
            pytest.skip("听记项缺少 ID 字段")
        return mid

    def test_task_uuid_flag_accepted(self, dws, sample_id):
        """get transcription --task-uuid 应被接受。"""
        result = dws.run_raw("minutes", "get", "transcription", "--task-uuid", sample_id)
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    def test_uuid_flag_accepted(self, dws, sample_id):
        """get transcription --uuid 应被接受。"""
        result = dws.run_raw("minutes", "get", "transcription", "--uuid", sample_id)
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

class TestMindGraphFlagAliases:
    """dws minutes mind-graph create/status 的 flag 别名兼容。"""

    def test_mind_graph_create_task_uuid_accepted(self, dws):
        """mind-graph create --task-uuid 应被接受，不报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "mind-graph", "create",
            "--task-uuid", "test_placeholder_uuid",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()

    def test_mind_graph_status_uuid_accepted(self, dws):
        """mind-graph status --uuid 应被接受，不报 unknown flag。"""
        result = dws.run_raw(
            "minutes", "mind-graph", "status",
            "--uuid", "test_placeholder_uuid",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert "unknown flag" not in combined.lower()
