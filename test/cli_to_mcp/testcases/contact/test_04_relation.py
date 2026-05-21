"""
test_04_relation.py — 人员关系查询测试 (contact relation)

Commands tested:
  1. dws contact relation list-my-followings  (Args: cobra.NoArgs)

语义要点（见 contact.md IMPORTANT 段落）:
  - list-my-followings 只返回"我特别关注的人员列表"（一组 openDingTalkId）；
  - 绝不返回任何消息内容（发/说/聊/讲、消息/聊天/动态/最新内容）；
  - 这是 contact.md 明确的"易混淆硬规则"，也是本测试文件的主要回归目标。
"""


# ── 返回结构取字段的容错解析 ─────────────────────────────────────────────

_PERSON_ID_KEYS = ("openDingTalkId", "openDingtalkId", "openId", "userId")
# 反向保护：下列字段一旦出现在结果里，说明返回被污染成消息体
_FORBIDDEN_MESSAGE_KEYS = (
    "messageContent", "messageId", "content", "msgContent", "msgId",
    "text", "messages", "message",
)


def _iter_items(result):
    """兼容 result 为 list 或 dict(.followings/.items/.list) 的情况。"""
    if isinstance(result, list):
        return result
    if isinstance(result, dict):
        for k in ("followings", "items", "list", "userList", "personList"):
            v = result.get(k)
            if isinstance(v, list):
                return v
    return []


class TestContactRelationListMyFollowings:
    """dws contact relation list-my-followings"""

    def test_returns_success_structure(self, dws):
        """命令应返回 success=true 的标准结构（允许空列表）。"""
        data = dws.run_ok("contact", "relation", "list-my-followings")
        assert data.get("success") is True, f"响应 success 应为 True: {data}"
        assert "result" in data, f"响应缺少 result 字段: {list(data.keys())}"

    def test_result_is_list_or_dict(self, dws):
        """result 应为 list 或 dict 结构，允许空。"""
        data = dws.run_ok("contact", "relation", "list-my-followings")
        result = data["result"]
        assert isinstance(result, (list, dict)), (
            f"result 应为 list 或 dict, 实为: {type(result)}"
        )

    def test_items_carry_person_identifier(self, dws):
        """非空时，每项至少包含一个人员标识字段（openDingTalkId / userId 等）。"""
        data = dws.run_ok("contact", "relation", "list-my-followings")
        items = _iter_items(data["result"])
        if not items:
            return  # 当前账号无特别关注，跳过非空断言但本条不 skip（保持用例计数）
        for item in items:
            assert isinstance(item, dict), f"每项应为 dict: {item}"
            has_id = any(k in item for k in _PERSON_ID_KEYS)
            assert has_id, (
                f"关注项应至少含人员标识字段 {_PERSON_ID_KEYS}, 实际 keys: "
                f"{list(item.keys())}"
            )

    def test_result_does_not_contain_message_fields(self, dws):
        """核心反向保护：结果中不应包含任何消息体字段。

        此命令与 `chat message list-focused` 易混淆；若未来返回结构被错误改造
        （例如把特别关注人的"最近消息"拼入），会破坏 contact.md IMPORTANT 段
        定义的「终点=人员列表」硬规则。本用例在结构层面强制阻止该漂移。
        """
        data = dws.run_ok("contact", "relation", "list-my-followings")
        items = _iter_items(data["result"])
        for item in items:
            if not isinstance(item, dict):
                continue
            polluted = [k for k in _FORBIDDEN_MESSAGE_KEYS if k in item]
            assert not polluted, (
                f"list-my-followings 结果不应含消息类字段 {polluted}，"
                f"否则与 chat message list-focused 语义混淆，item keys="
                f"{list(item.keys())}"
            )

    def test_rejects_unexpected_args(self, dws):
        """Args: cobra.NoArgs — 传入多余位置参数应报错（非 0 退出）。"""
        result = dws.run_raw(
            "contact", "relation", "list-my-followings", "unexpected-positional",
        )
        assert result.returncode != 0, (
            f"list-my-followings 不应接受位置参数，但退出码为 0: "
            f"stdout={result.stdout}\nstderr={result.stderr}"
        )
