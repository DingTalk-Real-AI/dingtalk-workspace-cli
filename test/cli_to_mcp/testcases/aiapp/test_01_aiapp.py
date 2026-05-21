"""
test_aiapp.py — AI 应用测试 (3 commands × 3 cases)

Commands tested:
  1. dws aiapp create  (create_ai_app)
  2. dws aiapp query   (query_ai_app)
  3. dws aiapp modify  (modify_ai_app)

Response schema (shared by create & modify):
  {
    "success": true,
    "result": {
      "status": "queued",
      "taskId": "<uuid>",
      "taskUrl": "<string>",
      "threadId": "<uuid>",
      "threadViewUrl": "<url>"
    }
  }

Query response schema:
  {
    "success": true,
    "result": {
      "taskId": "<uuid>",
      "status": "running" | "completed" | ...,
      "progress": { "completedSteps": int, "totalSteps": int, ... },
      "createdAt": "<iso8601>",
      "updatedAt": "<iso8601>",
      ...
    }
  }
"""

import json
import pytest
import time


def _assert_create_or_modify_response(data: dict, label: str = ""):
    """Validate the common response structure for create/modify commands."""
    assert isinstance(data, dict), f"{label} response should be dict, got {type(data)}"
    assert data.get("success") is True, f"{label} expected success=true, got: {data.get('success')}"
    result = data.get("result")
    assert isinstance(result, dict), f"{label} result should be dict, got: {result}"
    assert isinstance(result.get("taskId"), str) and result["taskId"], \
        f"{label} result.taskId should be non-empty string, got: {result.get('taskId')}"
    assert isinstance(result.get("threadId"), str) and result["threadId"], \
        f"{label} result.threadId should be non-empty string, got: {result.get('threadId')}"
    assert isinstance(result.get("status"), str), \
        f"{label} result.status should be string, got: {result.get('status')}"


def _assert_error_response(result, label: str = ""):
    """Validate that a run_raw result indicates an error."""
    stderr = (result.stderr or "").strip()
    stdout = (result.stdout or "").strip()
    # dws outputs error JSON to stdout, check both stdout and stderr
    has_error = False
    for output in (stdout, stderr):
        if not output:
            continue
        try:
            data = json.loads(output)
            if "error" in data:
                has_error = True
                break
        except json.JSONDecodeError:
            if "error" in output.lower():
                has_error = True
                break
    assert result.returncode != 0 or has_error, (
        f"{label} expected error response, but got returncode={result.returncode}, "
        f"stdout={stdout[:200]}, stderr={stderr[:200]}"
    )


@pytest.fixture(scope="session")
def test_ai_app(dws):
    """创建一个 AI 应用供后续测试使用。"""
    data = dws.run(
        "aiapp", "create",
        "--prompt", "创建一个天气查询助手，能回答全国各地天气",
    )
    _assert_create_or_modify_response(data, "fixture[test_ai_app]")
    result = data["result"]
    yield {"taskId": result["taskId"], "threadId": result["threadId"]}


class TestAiappCreate:
    """dws aiapp create"""

    def test_create_basic(self, dws):
        """创建基本 AI 应用。"""
        data = dws.run(
            "aiapp", "create",
            "--prompt", f"翻译助手_{int(time.time())}",
        )
        _assert_create_or_modify_response(data, "create_basic")

    def test_create_with_skills(self, dws):
        """创建并指定技能列表。"""
        data = dws.run(
            "aiapp", "create",
            "--prompt", "图片识别助手",
            "--skills", "skill1,skill2",
        )
        _assert_create_or_modify_response(data, "create_with_skills")

    def test_create_chinese_prompt(self, dws):
        """创建中文 prompt 的 AI 应用。"""
        data = dws.run(
            "aiapp", "create",
            "--prompt", "请创建一个能够解答数学问题的AI助手，支持四则运算和方程求解",
        )
        _assert_create_or_modify_response(data, "create_chinese_prompt")


class TestAiappQuery:
    """dws aiapp query"""

    def test_query_created_app(self, dws, test_ai_app):
        """查询已创建的 AI 应用，校验返回结构。"""
        data = dws.run_ok(
            "aiapp", "query",
            "--task-id", test_ai_app["taskId"],
        )
        assert data.get("success") is True, f"Expected success=true, got: {data.get('success')}"
        result = data.get("result")
        assert isinstance(result, dict), f"result should be dict, got: {result}"
        assert result.get("taskId") == test_ai_app["taskId"], \
            f"Queried taskId should match, expected {test_ai_app['taskId']}, got {result.get('taskId')}"

    def test_query_invalid_id(self, dws):
        """查询无效 taskId 应报错。"""
        result = dws.run_raw(
            "aiapp", "query", "--task-id", "INVALID_99999",
        )
        _assert_error_response(result, "query_invalid_id")

    def test_query_returns_status(self, dws, test_ai_app):
        """查询结果应含状态和进度信息。"""
        data = dws.run_ok(
            "aiapp", "query",
            "--task-id", test_ai_app["taskId"],
        )
        result = data.get("result", {})
        assert isinstance(result.get("status"), str) and result["status"], \
            f"result.status should be non-empty string, got: {result.get('status')}"
        assert "progress" in result, f"result should contain 'progress', keys: {list(result.keys())}"
        assert isinstance(result.get("createdAt"), str), \
            f"result.createdAt should be string, got: {result.get('createdAt')}"


class TestAiappModify:
    """dws aiapp modify"""

    def test_modify_prompt(self, dws, test_ai_app):
        """修改 AI 应用 prompt。"""
        if not test_ai_app.get("threadId"):
            pytest.skip("No threadId available")
        data = dws.run_ok(
            "aiapp", "modify",
            "--prompt", f"修改为日程管理助手_{int(time.time())}",
            "--thread-id", test_ai_app["threadId"],
        )
        _assert_create_or_modify_response(data, "modify_prompt")

    def test_modify_with_skills(self, dws, test_ai_app):
        """修改并指定新技能。"""
        if not test_ai_app.get("threadId"):
            pytest.skip("No threadId available")
        data = dws.run_ok(
            "aiapp", "modify",
            "--prompt", "日程管理助手",
            "--thread-id", test_ai_app["threadId"],
            "--skills", "skill_a,skill_b",
        )
        _assert_create_or_modify_response(data, "modify_with_skills")

    def test_modify_invalid_thread(self, dws):
        """修改无效 threadId 应报错。"""
        result = dws.run_raw(
            "aiapp", "modify",
            "--prompt", "X",
            "--thread-id", "INVALID_99999",
        )
        _assert_error_response(result, "modify_invalid_thread")
