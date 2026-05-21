"""
test_05_text_emotion.py — 文字表情回应测试

Commands tested:
  1. dws chat message create-text-emotion  (创建文字表情，获取 emotionId)
  2. dws chat message add-text-emotion     (对消息添加文字表情回应)
  3. dws chat message remove-text-emotion  (移除消息的文字表情回应)

NOTE: add/remove-text-emotion 依赖真实的 openConversationId 和 openMsgId，
      通过环境变量 TEST_OPENCONVERSATION_ID / TEST_MSG_ID 注入。
      create-text-emotion 不依赖会话，可独立运行。
"""

import pytest
from test_utils import unique_name


class TestCreateTextEmotion:
    """dws chat message create-text-emotion"""

    def test_create_basic(self, dws, chat_id):  # chat_id 用作环境门控
        """创建最简文字表情，验证返回 emotionId 和 backgroundId。"""
        data = dws.run(
            "chat", "message", "create-text-emotion",
            "--emotion-name", unique_name("赞"),
            "--text", "nice",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        result = data.get("result", {})
        assert "emotionId" in result, f"result 缺少 emotionId: {result}"
        assert "backgroundId" in result, f"result 缺少 backgroundId: {result}"
        assert result["emotionId"], f"emotionId 不应为空: {result}"

    def test_create_with_background_id(self, dws, chat_id):  # chat_id 用作环境门控
        """指定 backgroundId 创建文字表情。"""
        data = dws.run(
            "chat", "message", "create-text-emotion",
            "--emotion-name", unique_name("感谢"),
            "--text", "感谢",
            "--background-id", "im_bg_5",
        )
        assert data.get("success") is True, f"success 应为 True: {data}"
        result = data.get("result", {})
        assert "emotionId" in result, f"result 缺少 emotionId: {result}"

    def test_create_missing_emotion_name(self, dws):
        """不传 emotion-name 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "message", "create-text-emotion",
            "--text", "nice",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_create_missing_text(self, dws):
        """不传 text 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "message", "create-text-emotion",
            "--emotion-name", "赞",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestAddTextEmotion:
    """dws chat message add-text-emotion"""

    def test_add_basic(self, dws, chat_id, msg_id, text_emotion):
        """对消息添加文字表情回应。"""
        data = dws.run(
            "chat", "message", "add-text-emotion",
            "--conversation-id", chat_id,
            "--msg-id", msg_id,
            "--emotion-id", text_emotion["emotionId"],
            "--emotion-name", text_emotion["emotionName"],
            "--text", text_emotion["text"],
            "--background-id", text_emotion["backgroundId"],
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_add_with_group_alias(self, dws, chat_id, msg_id, text_emotion):
        """使用 --group 别名替代 --conversation-id。"""
        data = dws.run(
            "chat", "message", "add-text-emotion",
            "--group", chat_id,
            "--msg-id", msg_id,
            "--emotion-id", text_emotion["emotionId"],
            "--emotion-name", text_emotion["emotionName"],
            "--text", text_emotion["text"],
            "--background-id", text_emotion["backgroundId"],
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_add_missing_conversation_id(self, dws, msg_id, text_emotion):
        """不传 conversation-id 应报错。"""
        result = dws.run_raw(
            "chat", "message", "add-text-emotion",
            "--msg-id", msg_id,
            "--emotion-id", text_emotion["emotionId"],
            "--emotion-name", text_emotion["emotionName"],
            "--text", text_emotion["text"],
            "--background-id", text_emotion["backgroundId"],
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )


class TestRemoveTextEmotion:
    """dws chat message remove-text-emotion"""

    def test_remove_basic(self, dws, chat_id, msg_id, text_emotion):
        """移除消息上的文字表情回应。"""
        data = dws.run(
            "chat", "message", "remove-text-emotion",
            "--conversation-id", chat_id,
            "--msg-id", msg_id,
            "--emotion-id", text_emotion["emotionId"],
            "--emotion-name", text_emotion["emotionName"],
            "--text", text_emotion["text"],
            "--background-id", text_emotion["backgroundId"],
        )
        assert data.get("success") is True, f"success 应为 True: {data}"

    def test_remove_missing_msg_id(self, dws, chat_id, text_emotion):
        """不传 msg-id 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "message", "remove-text-emotion",
            "--conversation-id", chat_id,
            "--emotion-id", text_emotion["emotionId"],
            "--emotion-name", text_emotion["emotionName"],
            "--text", text_emotion["text"],
            "--background-id", text_emotion["backgroundId"],
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_remove_invalid_msg_id(self, dws, chat_id, text_emotion):
        """无效 msg-id 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "message", "remove-text-emotion",
            "--conversation-id", chat_id,
            "--msg-id", "INVALID_MSG_99999",
            "--emotion-id", text_emotion["emotionId"],
            "--emotion-name", text_emotion["emotionName"],
            "--text", text_emotion["text"],
            "--background-id", text_emotion["backgroundId"],
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
