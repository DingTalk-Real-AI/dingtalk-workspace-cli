"""
test_02_ding_query.py — DING 消息查询测试

Commands tested:
  1. dws ding message list              (list_ding_messages — 列出 DING 消息)
  2. dws ding message receiver-status   (list_ding_receiver_status — 查询 DING 接收状态)

注意:
  - ding message list / receiver-status 调用 im 空间 MCP 工具，预发环境可能未部署，
    DWSRunner 会自动 pytest.skip()（检测 "未找到指定工具" 等关键词）。
  - ding message list 必须传 --type（合法值: ALL/UNREAD/SEND/NEW_COMMENT/DELETED）；
    receiver-status 需要 --ding-id。
"""

import json as _json

import pytest


def _parse_json(proc):
    """从进程输出解析 JSON（与 conftest 同逻辑）。"""
    for src in (proc.stdout, proc.stderr):
        if not src or not src.strip():
            continue
        try:
            return _json.loads(src)
        except (ValueError, _json.JSONDecodeError):
            continue
    return None


class TestDingMessageList:
    """dws ding message list"""

    def test_ding_list_basic(self, dws):
        """列出 DING 消息 — 传 --type ALL，正常路径。"""
        data = dws.run("ding", "message", "list", "--type", "ALL")
        assert isinstance(data, dict), f"应返回 dict: {data}"

    def test_ding_list_with_cursor(self, dws):
        """列出 DING 消息 — 带 --cursor 分页参数。"""
        data = dws.run("ding", "message", "list", "--type", "ALL", "--cursor", "0")
        assert isinstance(data, dict), f"应返回 dict: {data}"

    def test_ding_list_with_type(self, dws):
        """列出 DING 消息 — 带 --type SEND 过滤已发送。"""
        data = dws.run("ding", "message", "list", "--type", "SEND")
        assert isinstance(data, dict), f"应返回 dict: {data}"

    def test_ding_list_invalid_cursor(self, dws):
        """无效 cursor 值 — 应返回错误或空结果。"""
        result = dws.run_raw(
            "ding", "message", "list", "--cursor", "NOT_A_NUMBER",
        )
        # 非数字 cursor，CLI 层或服务端应拒绝
        combined = (result.stdout or "") + (result.stderr or "")
        assert (
            result.returncode != 0
            or "error" in combined.lower()
            or "invalid" in combined.lower()
        ), f"无效 cursor 应报错: {combined[:300]}"


class TestDingMessageReceiverStatus:
    """dws ding message receiver-status"""

    @pytest.fixture()
    def ding_id(self, dws):
        """从 ding message list 获取一个有效 dingId。

        如果 list 为空（新组织无历史 DING 数据），自动发送一条 DING 消息造数据后重试。
        """
        import os
        import time

        def _fetch_ding_list(list_type="ALL"):
            proc = dws.run_raw("ding", "message", "list", "--type", list_type)
            data = _parse_json(proc)
            if data is None:
                return []
            return (
                data.get("result", {}).get("dingMessages")
                or data.get("result", {}).get("dingMessageList")
                or data.get("result", {}).get("records")
                or data.get("dingMessages")
                or data.get("dingMessageList")
                or []
            )

        def _extract_ding_id(items):
            for item in items:
                did = item.get("dingId") or item.get("openDingId")
                if did:
                    return str(did)
            return None

        # 优先从 SEND 类型获取（当前用户发的，避免 senderUid not match）
        for list_type in ("SEND", "ALL"):
            items = _fetch_ding_list(list_type)
            did = _extract_ding_id(items)
            if did:
                return did

        # list 为空 → 发一条 DING 消息造数据后重试
        robot_code = os.environ.get("DINGTALK_DING_ROBOT_CODE", "dingzrrsoiuh5adawoux")
        target_user = os.environ.get("DINGTALK_DING_TARGET_USER", "035666020404868955453")
        send_proc = dws.run_raw(
            "ding", "message", "send",
            "--robot-code", robot_code,
            "--users", target_user,
            "--content", f"自动造数据DING_{int(time.time())}",
        )
        send_data = _parse_json(send_proc)
        if send_data and send_data.get("success"):
            time.sleep(2)
            items = _fetch_ding_list("SEND")
            did = _extract_ding_id(items)
            if did:
                return did

        pytest.skip("无法获取可用 dingId（SEND/ALL 列表均为空且自动造数据失败）")

    def test_receiver_status_basic(self, dws, ding_id):
        """查询 DING 接收状态 — 正常路径。"""
        data = dws.run(
            "ding", "message", "receiver-status",
            "--ding-id", ding_id,
        )
        assert isinstance(data, dict), f"应返回 dict: {data}"

    def test_receiver_status_missing_ding_id(self, dws):
        """不传 --ding-id 应报错（必填）。"""
        result = dws.run_raw("ding", "message", "receiver-status")
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_receiver_status_invalid_ding_id(self, dws):
        """无效 ding-id 应返回业务错误。"""
        result = dws.run_raw(
            "ding", "message", "receiver-status",
            "--ding-id", "INVALID_DING_99999",
        )
        combined = (result.stdout or "") + (result.stderr or "")
        assert (
            result.returncode != 0
            or "error" in combined.lower()
        ), f"无效 ding-id 应报错: {combined[:300]}"
