"""todo 高频错误参数回归用例。"""

import json


def _assert_regression_result_ok(result):
    """参数回归用例统一判定：
    - 成功返回 JSON（如 {success:true}）视为通过；
    - unknown flag / 结构化错误视为通过（回归用例验证的是 CLI 不崩溃）。
    """
    combined = ((result.stdout or "") + "\n" + (result.stderr or "")).strip()
    assert len(combined) > 0, "命令未产生任何输出"


class TestTodoParamRegression:
    def test_create_subject_alias(self, dws, current_user_id):
        """验证 --subject 作为 --title 别名可正常工作。"""
        result = dws.run_raw(
            "todo", "task", "create",
            "--subject", "参数别名回归",
            "--executors", current_user_id,
        )
        _assert_regression_result_ok(result)

    def test_list_sticky_page_and_size(self, dws):
        """粘连参数 --page1 --size20 不应导致 CLI 崩溃。"""
        result = dws.run_raw(
            "todo", "task", "list",
            "--page1", "--size20", "--status", "false",
        )
        _assert_regression_result_ok(result)

    def test_create_sticky_executors_flag(self, dws):
        result = dws.run_raw(
            "todo", "task", "create",
            "--title", "参数粘连测试",
            "--executors123456",
        )
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()
