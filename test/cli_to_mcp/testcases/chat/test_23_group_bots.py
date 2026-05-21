"""
test_23_group_bots.py — 查看群内所有机器人测试

Commands tested:
  1. dws chat group bots  (list_group_bots — 拉取群内所有机器人列表)

Setup 流程（按 5.14/5.18 评测要求）：
  搜测试群（searched_chat_id）→ 在群内 add-bot → bots 列出 → 清理 remove-bot。
  add 与 cleanup 失败时仅 skip，不 fail（避免环境污染影响 list 命令本身的验证）。
"""

import time

import pytest

from conftest import _parse_json, skip_if_backend_tool_missing


@pytest.fixture(scope="function")
def searched_chat_with_bot(dws, searched_chat_id, robot_code):
    """搜索测试群 + 加入指定机器人；teardown 时清理移除。

    yield: {"group_id": str, "robot_code": str, "open_bot_id": str | None}
    open_bot_id 取自 chat group bots 返回，list_group_bots 失败时为 None
    （不影响其他用例对 add-bot / remove-bot 自身的验证）。
    """
    gid = searched_chat_id
    add_proc = dws.run_raw(
        "chat", "group", "members", "add-bot",
        "--robot-code", robot_code,
        "--id", gid,
    )
    add_data = _parse_json(add_proc)
    if add_data is None or add_data.get("success") is not True:
        # 群里可能已有该机器人，加不上时直接进入测试，不再 fail
        print(f"\n[fixture] add-bot 未成功（可能机器人已在群内），继续: "
              f"{(add_data or {}).get('error') or (add_data or {}).get('message')}")
    else:
        time.sleep(1)

    open_bot_id = None
    list_proc = dws.run_raw("chat", "group", "bots", "--group", gid)
    list_data = _parse_json(list_proc)
    if list_data and list_data.get("success"):
        bots = (
            list_data.get("result", {}).get("bots")
            or list_data.get("result", {}).get("robots")
            or list_data.get("result", {}).get("botList")
            or list_data.get("bots")
            or []
        )
        if bots:
            first = bots[0]
            open_bot_id = (
                first.get("openBotId")
                or first.get("open_bot_id")
                or first.get("botId")
            )

    yield {"group_id": gid, "robot_code": robot_code, "open_bot_id": open_bot_id}

    if open_bot_id:
        dws.run_raw(
            "chat", "group", "members", "remove-bot",
            "--id", gid,
            "--bot-id", open_bot_id,
        )


class TestChatGroupBots:
    """dws chat group bots"""

    def test_group_bots_basic(self, dws, searched_chat_with_bot):
        """搜群 + 加机器人后，列出群内机器人 — 应 success=true。"""
        gid = searched_chat_with_bot["group_id"]
        proc = dws.run_raw(
            "chat", "group", "bots",
            "--group", gid,
        )
        data = _parse_json(proc)
        skip_if_backend_tool_missing(data)
        assert data is not None and data.get("success") is True, f"success 应为 True: {data}"

    def test_group_bots_missing_group(self, dws):
        """不传 --group 应报错（必填）。"""
        result = dws.run_raw(
            "chat", "group", "bots",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_group_bots_invalid_group(self, dws):
        """无效 group 应返回业务错误。"""
        result = dws.run_raw(
            "chat", "group", "bots",
            "--group", "INVALID_CONV_99999",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )
