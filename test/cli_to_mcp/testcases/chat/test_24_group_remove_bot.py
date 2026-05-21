"""
test_24_group_remove_bot.py — 从群内移除机器人测试

Commands tested:
  1. dws chat group members remove-bot  (remove_robot_in_group — 通过 openBotId 将机器人移出群)

Setup 流程（按 5.14/5.18 评测要求）：
  搜测试群（searched_chat_id）→ add-bot → list_group_bots 取 openBotId →
  remove-bot 验证正向路径。
  list_group_bots 拿不到 openBotId 时正向用例 skip，不 fail。
"""

import time

import pytest

from conftest import _parse_json, skip_if_backend_tool_missing


class TestChatGroupMembersRemoveBot:
    """dws chat group members remove-bot"""

    def test_remove_bot_full_flow(self, dws, searched_chat_id, robot_code):
        """搜群 → 取机器人 name → add-bot → list bots 按 name 匹配 openBotId → remove-bot。

        list_group_bots 返回结构里 bot 没有 robotCode 字段，只能通过 name 匹配；
        因此先用 chat bot search 拿到 robotCode 对应的 robotName，再按 name 比对。
        """
        gid = searched_chat_id

        # 0. 通过 chat bot search 拿 robotName（与 robot_code 配对）
        search_proc = dws.run_raw("chat", "bot", "search")
        sdata = _parse_json(search_proc)
        if not sdata or not sdata.get("success"):
            pytest.skip(f"chat bot search 失败，无法拿 robotName: {sdata}")
        robots = sdata.get("robotList") or sdata.get("result", {}).get("robotList") or []
        robot_name = None
        for r in robots:
            if r.get("robotCode") == robot_code:
                robot_name = r.get("robotName") or r.get("name")
                break
        if not robot_name:
            pytest.skip(f"chat bot search 未匹配到 robotCode={robot_code} 对应的 robotName")

        # 1. add-bot（已在群内时跳过 add 错误）
        add_proc = dws.run_raw(
            "chat", "group", "members", "add-bot",
            "--robot-code", robot_code,
            "--id", gid,
        )
        add_data = _parse_json(add_proc)
        if add_data is None:
            pytest.skip(f"add-bot 返回非 JSON: {add_proc.stdout[:200]}")
        time.sleep(1)

        # 2. list bots 按 name 拿 openBotId
        list_proc = dws.run_raw("chat", "group", "bots", "--group", gid)
        list_data = _parse_json(list_proc)
        skip_if_backend_tool_missing(list_data)
        if not list_data or not list_data.get("success"):
            pytest.skip(f"chat group bots 未成功: {list_data}")
        bots = list_data.get("result", {}).get("bots") or []
        target_bot_id = None
        for b in bots:
            if b.get("name") == robot_name:
                target_bot_id = b.get("openBotId")
                break
        if not target_bot_id:
            pytest.skip(f"群内未找到 name={robot_name!r} 的机器人，可能 add-bot 未生效或后端字段变化: bots={bots}")

        # 3. remove-bot
        remove_proc = dws.run_raw(
            "chat", "group", "members", "remove-bot",
            "--id", gid,
            "--bot-id", target_bot_id,
        )
        data = _parse_json(remove_proc)
        skip_if_backend_tool_missing(data)
        assert data is not None and data.get("success") is True, f"remove-bot 应成功: {data}"

    def test_remove_bot_invalid_bot_id(self, dws, searched_chat_id):
        """合法群 + 非法 bot-id，应返回业务错误。"""
        result = dws.run_raw(
            "chat", "group", "members", "remove-bot",
            "--id", searched_chat_id,
            "--bot-id", "BOGUS_BOT_ID_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_remove_bot_missing_id(self, dws):
        """不传 --id 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "group", "members", "remove-bot",
            "--bot-id", "BOGUS_BOT_ID_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_remove_bot_missing_bot_id(self, dws, searched_chat_id):
        """不传 --bot-id 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "group", "members", "remove-bot",
            "--id", searched_chat_id,
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_remove_bot_invalid_group_id(self, dws):
        """非法 group + 任意 bot-id，应返回业务错误。"""
        result = dws.run_raw(
            "chat", "group", "members", "remove-bot",
            "--id", "INVALID_CONV_99999",
            "--bot-id", "BOGUS_BOT_ID_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
