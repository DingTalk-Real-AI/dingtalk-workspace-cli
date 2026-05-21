"""contact 高频错误参数回归用例。"""

import json


def _assert_regression_result_ok(result):
    """
    参数回归用例统一判定：
    - 成功返回 JSON（如 {"success": true} / 正常结果）视为通过；
    - 失败返回结构化错误（stderr 或 stdout 中含 error/code/message）视为失败；
    - 明确的 unknown flag / AUTH_PERMISSION_DENIED 文本视为失败。
    """
    combined = ((result.stdout or "") + "\n" + (result.stderr or "")).strip()
    lower = combined.lower()
    if "unknown flag" in lower or "auth_permission_denied" in lower:
        return

    for payload in (result.stdout or "", result.stderr or ""):
        text = (payload or "").strip()
        if not text:
            continue
        try:
            data = json.loads(text)
        except json.JSONDecodeError:
            continue
        if isinstance(data, dict):
            if data.get("success") is True:
                return
            if "error" in data or "code" in data or "message" in data:
                return
            if "result" in data or "userId" in data or "deptList" in data:
                return

    assert False, (
        "回归用例命令返回不符合预期（既非成功结果，也非可识别错误）:\n"
        f"returncode={result.returncode}\nstdout={result.stdout}\nstderr={result.stderr}"
    )


class TestContactParamRegression:
    def test_user_search_wrong_query_flag(self, dws):
        result = dws.run_raw("contact", "user", "search", "--query", "wukong01")
        _assert_regression_result_ok(result)

    def test_dept_search_wrong_query_flag(self, dws):
        result = dws.run_raw("contact", "dept", "search", "--query", "研发")
        _assert_regression_result_ok(result)

    def test_user_get_wrong_user_ids_flag(self, dws):
        result = dws.run_raw("contact", "user", "get", "--user-ids", "035665695811868955452")
        _assert_regression_result_ok(result)

    def test_user_get_sticky_ids_flag(self, dws):
        result = dws.run_raw("contact", "user", "get", "--ids", "035665695811868955452")
        _assert_regression_result_ok(result)

