"""
test_19_streaming_card.py — 流式卡片测试

Commands tested:
  1. dws chat message send-card   (create_and_send_card)
  2. dws chat message update-card (update_streaming_card)

send-card 和 update-card 必须搭配使用，最后一次 update 必须将 flow-status 设为 3（finish）。
flowStatus 枚举: 1=PROCESSING, 2=INPUTTING, 3=FINISH, 4=EXECUTING, 5=ERROR
"""

import json
import pytest


class TestMessageSendCard:
    """dws chat message send-card"""

    def test_send_card_to_group(self, dws, searched_chat_id):
        """向群聊创建卡片，验证返回 bizId。"""
        data = dws.run(
            "chat", "message", "send-card",
            "--group", searched_chat_id,
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        result = data.get("result", {})
        assert "bizId" in result, f"result 缺少 bizId: {result}"
        assert result["bizId"], f"bizId 不应为空: {result}"

    def test_send_card_missing_target(self, dws):
        """不传 group 和 receiver 应报错。"""
        result = dws.run_raw(
            "chat", "message", "send-card",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestMessageUpdateCard:
    """dws chat message update-card"""

    def test_send_and_update_card(self, dws, searched_chat_id):
        """创建卡片（不传 content）后通过 update-card 写入内容并完成（flow-status=3 FINISH）。"""
        send_data = dws.run(
            "chat", "message", "send-card",
            "--group", searched_chat_id,
        )
        assert send_data.get("success") is True, f"send-card 失败: {send_data}"
        biz_id = send_data.get("result", {}).get("bizId")
        assert biz_id, f"send-card 缺少 bizId: {send_data}"

        update_data = dws.run(
            "chat", "message", "update-card",
            "--biz-id", biz_id,
            "--content", "CI测试-更新后的卡片内容",
            "--flow-status", "3",
        )
        assert update_data.get("success") is True, f"update-card 失败: {update_data}"

    def test_update_card_streaming_then_finish(self, dws, searched_chat_id):
        """创建卡片（不传 content）→ 输入中(2 INPUTTING) → 完成(3 FINISH)，验证多步流程。"""
        send_data = dws.run(
            "chat", "message", "send-card",
            "--group", searched_chat_id,
        )
        biz_id = send_data.get("result", {}).get("bizId")
        assert biz_id, f"send-card 缺少 bizId: {send_data}"

        streaming_data = dws.run(
            "chat", "message", "update-card",
            "--biz-id", biz_id,
            "--content", "CI测试-流式更新中...",
            "--flow-status", "2",
        )
        assert streaming_data.get("success") is True, f"streaming update 失败: {streaming_data}"

        finish_data = dws.run(
            "chat", "message", "update-card",
            "--biz-id", biz_id,
            "--content", "CI测试-最终内容",
            "--flow-status", "3",
        )
        assert finish_data.get("success") is True, f"finish update 失败: {finish_data}"

    def test_update_card_missing_biz_id(self, dws):
        """不传 biz-id 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "message", "update-card",
            "--content", "测试",
            "--flow-status", "3",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_update_card_missing_flow_status(self, dws):
        """不传 flow-status 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "message", "update-card",
            "--biz-id", "FAKE_BIZ_ID",
            "--content", "测试",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_update_card_invalid_biz_id(self, dws):
        """无效 biz-id 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "message", "update-card",
            "--biz-id", "INVALID_BIZ_99999",
            "--content", "测试",
            "--flow-status", "3",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
