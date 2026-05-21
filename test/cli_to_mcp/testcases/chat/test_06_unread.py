"""
test_06_unread.py — 未读会话列表测试

Commands tested:
  1. dws chat message list-unread-conversations
"""


class TestChatMessageListUnreadConversations:
    """dws chat message list-unread-conversations — 获取未读会话列表"""

    def test_list_unread_conversations(self, dws):
        """默认参数获取未读会话列表。"""
        data = dws.run_ok("chat", "message", "list-unread-conversations")
        assert data is not None

    def test_list_unread_conversations_with_count(self, dws):
        """指定 count 获取未读会话列表。"""
        data = dws.run_ok("chat", "message", "list-unread-conversations", "--count", "20")
        assert data is not None

    def test_list_unread_conversations_invalid_count(self, dws):
        """count 传非数字应报参数错误。"""
        result = dws.run_raw("chat", "message", "list-unread-conversations", "--count", "invalid")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()
