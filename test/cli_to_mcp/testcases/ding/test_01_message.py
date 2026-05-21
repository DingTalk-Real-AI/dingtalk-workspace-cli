"""
test_ding.py — DING 消息测试 (2 commands × 3 cases)

环境变量: DINGTALK_DING_ROBOT_CODE

Commands tested:
  1. dws ding message send    (send_ding_message)
  2. dws ding message recall  (recall_ding_message)
"""

import os
import pytest
import time



@pytest.fixture(scope="session")
def robot_code():
    return os.environ.get("DINGTALK_DING_ROBOT_CODE", "dingzrrsoiuh5adawoux")


@pytest.fixture(scope="session")
def ding_target_user():
    return os.environ.get("DINGTALK_DING_TARGET_USER", "035666020404868955453")


class TestDingMessageSend:
    """dws ding message send"""

    def test_send_app_ding(self, dws, robot_code, ding_target_user):
        """发送应用内 DING。"""
        data = dws.run(
            "ding", "message", "send",
            "--robot-code", robot_code,
            "--users", ding_target_user,
            "--content", f"CLI自动化DING {int(time.time())}",
        )
        assert data.get("success") is True
        assert data.get("result", {}).get("openDingId")

    def test_send_with_type(self, dws, robot_code, ding_target_user):
        """指定 type=app 发送。"""
        data = dws.run(
            "ding", "message", "send",
            "--robot-code", robot_code,
            "--type", "app",
            "--users", ding_target_user,
            "--content", "类型测试",
        )
        assert data.get("success") is True
        assert data.get("result", {}).get("openDingId")

    def test_send_invalid_robot(self, dws, ding_target_user):
        """无效 robot-code 应报错。"""
        result = dws.run_raw(
            "ding", "message", "send",
            "--robot-code", "INVALID",
            "--users", ding_target_user,
            "--content", "X",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestDingMessageRecall:
    """dws ding message recall"""

    def test_recall_sent(self, dws, robot_code, ding_target_user):
        """发送后撤回 DING。"""
        send = dws.run(
            "ding", "message", "send",
            "--robot-code", robot_code,
            "--users", ding_target_user,
            "--content", f"待撤回 {int(time.time())}",
        )
        ding_id = send.get("result", {}).get("openDingId", "")
        if not ding_id:
            pytest.skip("No openDingId returned")
        dws.run_ok(
            "ding", "message", "recall",
            "--robot-code", robot_code,
            "--id", ding_id,
        )

    def test_recall_invalid_id(self, dws, robot_code):
        """撤回无效 DING ID。"""
        result = dws.run_raw(
            "ding", "message", "recall",
            "--robot-code", robot_code,
            "--id", "INVALID_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_recall_missing_robot(self, dws):
        """缺少 robot-code 应报错。"""
        result = dws.run_raw(
            "ding", "message", "recall",
            "--id", "SOME_ID",
        )
        assert (
            result.returncode != 0
            or "error" in result.stderr.lower()
            or "robotCode" in result.stderr.lower()
        )
