"""conftest.py — Chat-specific fixtures.
DWSRunner/dws/current_user_id come from root conftest.py.

Fixtures:
  - robot_code: 机器人 Code（session 级）
  - chat_id: 群聊 openConversationId（**session 级**，整个测试 session 只建 1 个共享群）
  - transient_group_id: 一次性独立群（**function 级**），仅给会改群所有权等
    "破坏性独占"用例使用（如 chat group transfer-owner）
  - webhook_token: Webhook Token（session 级）
  - msg_id: 用于测试的消息 openMsgId（**session 级**，复用 chat_id 群发一次消息后所有用例共享）
  - current_open_dingtalk_id: 当前用户 openDingTalkId
  - text_emotion: 文字表情（function 级）

设计说明：
  早期版本 chat_id / msg_id 是 function 级（每个用例独立建群+发消息），
  会让钉钉会话列表被几十个 CI 测试群淹没。改造后整个 session 只建 1 个
  共享群，所有"在群里发消息/读信息/加 emoji/置顶/免打扰"的用例共用此群。
  对于真正会改变群本身（换群主等）的用例，使用 transient_group_id 创建
  独立的一次性群，避免污染共享群。
"""

import json as _json
import os
import time

import pytest


def _parse_json(proc):
    """从进程输出解析 JSON。先尝试 stdout，再回退 stderr。

    DWS CLI 在权限不足等情况下可能将 JSON 错误信息输出到 stderr，
    此辅助函数统一处理两种场景。
    """
    for src in (proc.stdout, proc.stderr):
        if not src or not src.strip():
            continue
        try:
            return _json.loads(src)
        except (ValueError, _json.JSONDecodeError):
            continue
    return None


def skip_if_backend_tool_missing(data):
    """当属于"环境/灰度问题"而非 CLI 缺陷时跳过用例。

    覆盖两类：
      1. 后端 MCP 服务未部署对应工具:
         "technical_detail": "Tool metadata API error: PARAM_ERROR - 未找到指定工具"
      2. 测试账号缺少对应工具的灰度权限:
         "message": "[AUTH_PERMISSION_DENIED] Permission denied"
    """
    if not isinstance(data, dict):
        return
    err = (data or {}).get("error") or {}
    detail = err.get("technical_detail") or ""
    if "未找到指定工具" in detail:
        pytest.skip(f"后端 MCP 工具尚未部署，跳过用例: {detail}")
    text = " ".join(filter(None, [
        str(err.get("message", "")),
        str(err.get("category", "")),
        str(detail),
    ]))
    if "AUTH_PERMISSION_DENIED" in text or "权限不足" in text or "Permission denied" in text:
        pytest.skip(f"测试账号无该工具灰度权限，跳过用例: {text[:200]}")


@pytest.fixture(scope="session")
def robot_code(dws):
    """机器人 Code — 动态获取当前组织可用的 bot，避免跨组织硬编码失效。

    优先级：环境变量 TEST_ROBOT_CODE → chat bot search 第一个结果 → skip。
    """
    env_code = os.environ.get("TEST_ROBOT_CODE", "")
    if env_code:
        return env_code
    proc = dws.run_raw("chat", "bot", "search")
    data = _parse_json(proc)
    if data and data.get("success") is not False:
        robots = data.get("robotList") or data.get("result", {}).get("robotList") or []
        if robots:
            code = robots[0].get("robotCode")
            if code:
                print(f"\n[conftest] 🤖 robot_code 自动获取: {code} ({robots[0].get('robotName', '?')})")
                return code
    pytest.skip("当前组织无可用机器人（chat bot search 返回空），跳过 bot 相关用例")


@pytest.fixture(scope="session")
def chat_id(dws, current_user_id):
    """群聊 openConversationId — **session 级共享群**。

    整个测试 session 只创建 1 个群，所有"在群里发消息/读信息/加 emoji/
    置顶/免打扰"的用例都共用此群，避免污染钉钉会话列表。

    会改变群本身（换群主等）的用例请使用 transient_group_id。
    """
    name = f"CI_共享测试群_{int(time.time())}_{id(object()) % 10000}"
    proc = dws.run_raw(
        "chat", "group", "create",
        "--name", name,
        "--users", current_user_id,
    )
    data = _parse_json(proc)
    if data is None:
        pytest.skip(
            f"chat group create 返回非 JSON，跳过: "
            f"stdout={proc.stdout[:200]} stderr={(proc.stderr or '')[:200]}"
        )
    if data.get("success") is not True:
        pytest.skip(
            f"chat group create 未成功，跳过: "
            f"{_json.dumps(data, ensure_ascii=False)[:200]}"
        )
    cid = data.get("result", {}).get("openConversationId")
    if not cid:
        pytest.skip(f"chat group create 返回缺少 openConversationId: {data}")
    print(f"\n[conftest] 🏠 chat_id session 共享群已创建: {cid} (name={name})")
    return cid


@pytest.fixture(scope="session")
def searched_chat_id(dws):
    """通过 chat search 搜索已有群 — **不依赖建群权限**。

    适用于可逆操作（invite-url、update-icon、update-settings、send-card 等），
    不会改变群的所有权或成员关系。当 PAT 缺少 chat.group:create 权限时，
    此 fixture 仍可正常工作。
    """
    data = dws.run("chat", "search", "--keyword", "测试", "--limit", "5")
    groups = (data.get("result") or {}).get("groups") or []
    if not groups:
        pytest.skip("chat search 未搜索到包含'测试'的群，跳过")
    cid = groups[0].get("openConversationId")
    if not cid:
        pytest.skip(f"搜索到的群缺少 openConversationId: {groups[0]}")
    title = groups[0].get("title", "未知")
    print(f"\n[conftest] 🔍 searched_chat_id 已选群: {cid} (title={title})")
    return cid


@pytest.fixture(scope="session")
def searched_chat_msg_id(dws, searched_chat_id):
    """在 searched_chat_id（搜索到的测试群）里发一条消息，返回 (group_id, msg_id)。

    适用于：set-history / combine-forward 等需要"先有一条消息再操作"的用例
    （5.14 / 5.18 新工具评测要求）。session 级共享，整个测试 session 只发 1 条。
    """
    proc = dws.run_raw(
        "chat", "message", "send",
        "--group", searched_chat_id,
        "--title", f"CI_搜索群消息_{int(time.time())}",
        f"自动化测试搜索群消息_{int(time.time())}_供 set-history/combine-forward 等用例使用",
    )
    send_data = _parse_json(proc)
    if send_data is None or send_data.get("success") is not True:
        pytest.skip(
            f"在 searched_chat_id 群里发消息失败，跳过: "
            f"{_json.dumps(send_data or {}, ensure_ascii=False)[:200]}"
        )
    time.sleep(2)

    # 优先用 search-advanced 拿 msgId，失败回退 chat message list
    mid = None
    proc2 = dws.run_raw(
        "chat", "message", "search-advanced",
        "--conversation-ids", searched_chat_id,
        "--limit", "5",
    )
    sd = _parse_json(proc2)
    if sd and sd.get("success"):
        msgs_list = sd.get("result", {}).get("conversationMessagesList", [])
        if msgs_list:
            inner = msgs_list[0].get("messages", []) or []
            if inner:
                mid = inner[0].get("openMessageId") or inner[0].get("openMsgId")
    if not mid:
        list_time = time.strftime("%Y-%m-%d 00:00:00")
        proc3 = dws.run_raw(
            "chat", "message", "list",
            "--conversation-id", searched_chat_id,
            "--time", list_time,
            "--limit", "5",
        )
        ld = _parse_json(proc3)
        if ld and ld.get("success"):
            messages = ld.get("result", {}).get("messages", [])
            if messages:
                mid = messages[0].get("openMessageId") or messages[0].get("openMsgId")
    if not mid:
        pytest.skip("无法在 searched_chat_id 群里取得 openMessageId")
    print(f"\n[conftest] 📨 searched_chat_msg_id: group={searched_chat_id} msg={mid}")
    return {"group_id": searched_chat_id, "msg_id": mid}


@pytest.fixture(scope="function")
def transient_group_id(dws, current_user_id):
    """一次性独立群 — **每次调用创建新群**。

    仅给会改变群本身（如 transfer-owner 换群主）的"破坏性独占"用例使用，
    避免污染 chat_id 共享群。
    """
    name = f"CI_一次性群_{int(time.time())}_{id(object()) % 10000}"
    proc = dws.run_raw(
        "chat", "group", "create",
        "--name", name,
        "--users", current_user_id,
    )
    data = _parse_json(proc)
    if data is None:
        pytest.skip(
            f"chat group create 返回非 JSON，跳过: "
            f"stdout={proc.stdout[:200]} stderr={(proc.stderr or '')[:200]}"
        )
    if data.get("success") is not True:
        pytest.skip(
            f"chat group create 未成功，跳过: "
            f"{_json.dumps(data, ensure_ascii=False)[:200]}"
        )
    cid = data.get("result", {}).get("openConversationId")
    if not cid:
        pytest.skip(f"chat group create 返回缺少 openConversationId: {data}")
    return cid


@pytest.fixture(scope="session")
def webhook_token():
    """Webhook Token，可从环境变量或使用默认値。"""
    return os.environ.get("TEST_WEBHOOK_TOKEN", "8710dbbfa8b8b63a02ddfe41ae428b838d2f5e998e0ec446196981d97e7d5350")


@pytest.fixture(scope="session")
def msg_id(dws, chat_id):
    """用于测试的消息 openMsgId — **session 级共享**。

    流程：在 chat_id 共享群里发 1 条消息 → search-advanced 拿 msgId。
    所有 add-emoji / remove-emoji 等不修改消息本身的用例共用此 msgId。
    使用 chat message send（当前用户身份）而非 send-by-bot，避免依赖机器人。
    """
    proc = dws.run_raw(
        "chat", "message", "send",
        "--group", chat_id,
        "--title", f"CI_共享消息_{int(time.time())}",
        f"自动化测试共享消息_{int(time.time())}_供emoji/reply等用例使用",
    )
    send_data = _parse_json(proc)
    assert send_data is not None, (
        f"send 返回非 JSON: stdout={proc.stdout[:200]} "
        f"stderr={(proc.stderr or '')[:200]}"
    )
    assert send_data.get("success") is True, (
        f"send 未成功: {_json.dumps(send_data, ensure_ascii=False)[:200]}"
    )
    # 等待消息落库后搜索
    time.sleep(2)

    # 策略 1: search-advanced（部分组织可能未开通搜索索引）
    mid = None
    proc2 = dws.run_raw(
        "chat", "message", "search-advanced",
        "--conversation-ids", chat_id,
        "--limit", "5",
    )
    search_data = _parse_json(proc2)
    if search_data and search_data.get("success"):
        msgs_list = search_data.get("result", {}).get("conversationMessagesList", [])
        if msgs_list:
            inner_msgs = msgs_list[0].get("messages", []) or []
            if inner_msgs:
                mid = inner_msgs[0].get("openMessageId") or inner_msgs[0].get("openMsgId")

    # 策略 2: 回退到 chat message list（按时间拉取最新消息）
    if not mid:
        print("\n[conftest] search-advanced 返回空，回退到 chat message list")
        list_time = time.strftime("%Y-%m-%d 00:00:00")
        proc3 = dws.run_raw(
            "chat", "message", "list",
            "--conversation-id", chat_id,
            "--time", list_time,
            "--limit", "5",
        )
        list_data = _parse_json(proc3)
        assert list_data is not None, (
            f"chat message list 返回非 JSON: stdout={proc3.stdout[:200]} "
            f"stderr={(proc3.stderr or '')[:200]}"
        )
        assert list_data.get("success") is True, (
            f"chat message list 未成功: "
            f"{_json.dumps(list_data, ensure_ascii=False)[:200]}"
        )
        messages = list_data.get("result", {}).get("messages", [])
        assert messages, (
            f"chat message list 返回空消息列表: "
            f"{_json.dumps(list_data, ensure_ascii=False)[:200]}"
        )
        mid = messages[0].get("openMessageId") or messages[0].get("openMsgId")

    assert mid, "无法获取 openMessageId（search-advanced 和 chat message list 均失败）"
    print(f"\n[conftest] 💬 msg_id session 共享消息已发: {mid}")
    return mid


@pytest.fixture(scope="session")
def current_open_dingtalk_id(dws, current_user_id):
    """当前登录用户的 openDingTalkId。

    解析顺序：
    1) 环境变量 TEST_OPEN_DINGTALK_ID（最稳定）
    2) contact user get-self 拿到当前用户姓名 → contact user search 搜出 openDingTalkId
       （contact user get 不返回 openDingTalkId，必须用 search）
    """
    v = os.environ.get("TEST_OPEN_DINGTALK_ID", "")
    if v:
        return v

    # 1) 拿到当前用户的姓名（用于后续 search 的关键词）
    self_proc = dws.run_raw("contact", "user", "get-self")
    self_data = _parse_json(self_proc)
    assert self_data is not None, f"contact user get-self 返回非 JSON: stdout={self_proc.stdout[:200]}"
    items = self_data.get("result", []) or []
    assert items, f"contact user get-self 返回空: {self_data}"
    name = items[0].get("orgEmployeeModel", {}).get("orgUserName") or ""
    assert name, f"contact user get-self 缺少 orgUserName: {items[0]}"

    # 2) 用姓名 search，挑出 userId 匹配当前用户的那一条
    search_proc = dws.run_raw("contact", "user", "search", "--keyword", name)
    search_data = _parse_json(search_proc)
    assert search_data is not None, f"contact user search 返回非 JSON: stdout={search_proc.stdout[:200]}"
    for u in search_data.get("result", []) or []:
        if u.get("userId") == current_user_id:
            oid = u.get("openDingTalkId")
            assert oid, f"search 命中当前用户但缺少 openDingTalkId: {u}"
            return oid
    raise AssertionError(
        f"contact user search --keyword {name!r} 未匹配到 userId={current_user_id}: {search_data}"
    )


@pytest.fixture(scope="function")
def text_emotion(dws, chat_id):  # chat_id 用作环境门控
    """创建一个 function 级文字表情，用于 add/remove-text-emotion 测试。"""
    proc = dws.run_raw(
        "chat", "message", "create-text-emotion",
        "--emotion-name", f"CLI测试赞_{int(time.time()) % 10000}",
        "--text", "nice",
    )
    data = _parse_json(proc)
    if data is None:
        pytest.skip(f"create-text-emotion 返回非 JSON，跳过: stdout={proc.stdout[:200]} stderr={(proc.stderr or '')[:200]}")
    if data.get("success") is not True:
        pytest.skip(f"create-text-emotion 未成功，跳过: {_json.dumps(data, ensure_ascii=False)[:200]}")
    result = data.get("result", {})
    eid = result.get("emotionId")
    bid = result.get("backgroundId")
    if not eid:
        pytest.skip(f"create-text-emotion 返回缺少 emotionId，跳过: {result}")
    return {
        "emotionId": eid,
        "backgroundId": bid,
        "emotionName": f"CLI测试赞_{int(time.time()) % 10000}",
        "text": "nice",
    }


# ─── Session-scoped fixtures (im_0429: reply/forward/set-top/mute/admin) ───


@pytest.fixture(scope="session")
def session_group_id(dws, current_user_id):
    """Session-scoped 群聊 — 用于可逆操作(置顶/群禁言/reply/forward)。"""
    name = f"CI_Session群_{int(time.time())}_{id(object()) % 10000}"
    proc = dws.run_raw(
        "chat", "group", "create",
        "--name", name,
        "--users", current_user_id,
    )
    data = _parse_json(proc)
    if data is None:
        pytest.skip(f"chat group create 返回非 JSON: stdout={proc.stdout[:200]}")
    if data.get("success") is not True:
        pytest.skip(f"chat group create 未成功: {_json.dumps(data, ensure_ascii=False)[:200]}")
    cid = data.get("result", {}).get("openConversationId")
    if not cid:
        pytest.skip(f"chat group create 缺少 openConversationId: {data}")
    return cid


# 严格白名单：仅在这三个固定账号中按顺序挑选，命中即用，绝不"随机选人"。
_FIND_OTHER_USER_KEYWORDS = ("wukong02", "汀葻")

def _get_open_dingtalk_id_by_user_id(dws, user_id):
    """已知 userId，尝试反查 openDingTalkId。

    流程（依赖 dws contact user get --ids 返回姓名字段）：
      1) dws contact user get --ids <userId> 拿到姓名
      2) dws contact user search --keyword <姓名> 搜索
      3) 在搜索结果中匹配 userId == user_id 的那一条，返回其 openDingTalkId

    实测注意：
      当前 dws contact user get --ids 返回的 orgEmployeeModel 字段为 null，
      拿不到姓名，因此该函数大概率返回 None；保留实现以便未来 dws 修复
      get 接口后自动可用。

    返回 openDingTalkId 字符串或 None。
    """
    if not user_id:
        return None
    proc = dws.run_raw("contact", "user", "get", "--ids", str(user_id))
    data = _parse_json(proc)
    if not data or not data.get("success"):
        print(
            f"\n[conftest] _get_open_dingtalk_id_by_user_id: "
            f"contact user get --ids {user_id} 失败或返回非 JSON，跳过该路径。"
        )
        return None
    items = data.get("result", []) or []
    if not items:
        print(
            f"\n[conftest] _get_open_dingtalk_id_by_user_id: "
            f"contact user get --ids {user_id} 返回空列表，跳过该路径。"
        )
        return None
    emp = items[0].get("orgEmployeeModel", {}) or {}
    name = (
        emp.get("orgUserName")
        or emp.get("name")
        or items[0].get("name")
        or ""
    )
    if not name:
        print(
            f"\n[conftest] _get_open_dingtalk_id_by_user_id: "
            f"contact user get --ids {user_id} 未返回姓名字段（orgEmployeeModel 可能为空），"
            f"跳过该路径。原始返回: {_json.dumps(items[0], ensure_ascii=False)[:200]}"
        )
        return None

    search_proc = dws.run_raw("contact", "user", "search", "--keyword", name)
    search_data = _parse_json(search_proc)
    if not search_data or not search_data.get("success"):
        print(
            f"\n[conftest] _get_open_dingtalk_id_by_user_id: "
            f"contact user search --keyword {name!r} 失败，跳过该路径。"
        )
        return None
    for u in search_data.get("result", []) or []:
        if str(u.get("userId") or "") == str(user_id):
            odid = u.get("openDingTalkId")
            if odid:
                return odid
    print(
        f"\n[conftest] _get_open_dingtalk_id_by_user_id: "
        f"contact user search --keyword {name!r} 未匹配到 userId={user_id}，跳过该路径。"
    )
    return None

def _find_other_org_user(dws, current_user_id):
    """按白名单顺序严格搜索固定账号，命中即返回；不做随机选人。

    依次搜索: wukong01 → wukong02 → 汀葻
      - 直接 dws contact user search --keyword <账号>
      - 接受条件：result 非空 + userId 非空 + userId != current_user_id + openDingTalkId 非空
      - 命中即停，返回 (userId, openDingTalkId)
      - 三个都搜不到 → 返回 (None, None)
    """
    for kw in _FIND_OTHER_USER_KEYWORDS:
        proc = dws.run_raw("contact", "user", "search", "--keyword", kw)
        data = _parse_json(proc)
        if not data or not data.get("success"):
            print(
                f"\n[conftest] _find_other_org_user: "
                f"contact user search --keyword {kw!r} 失败或返回非 JSON。"
            )
            continue
        results = data.get("result", []) or []
        if not results:
            print(
                f"\n[conftest] _find_other_org_user: "
                f"contact user search --keyword {kw!r} 返回空，尝试下一个。"
            )
            continue
        for u in results:
            uid = u.get("userId")
            odid = u.get("openDingTalkId")
            if uid and odid and str(uid) != str(current_user_id):
                print(
                    f"\n[conftest] 🤝 _find_other_org_user 命中关键词 {kw!r}: "
                    f"userId={uid} openDingTalkId={odid} name={u.get('name')!r}"
                )
                return uid, odid
        print(
            f"\n[conftest] _find_other_org_user: "
            f"contact user search --keyword {kw!r} 命中但无可用记录"
            f"（userId/openDingTalkId 缺失或为 self），尝试下一个。"
        )
    return None, None

@pytest.fixture(scope="session")
def _chat_other_user_pair(request, dws, current_user_id):
    """内部 fixture：解析 (userId, openDingTalkId) 二元组并缓存到 session。

    chat_other_user_id 与 chat_other_open_dingtalk_id 共享本结果，
    避免双倍调用 dws CLI。

    解析优先级（严格白名单优先，避免"随便选人"）：
      1) 环境变量同时配齐
         （DINGTALK_TEST_OTHER_USER_ID + DINGTALK_TEST_OTHER_OPEN_DINGTALK_ID）
      2) _find_other_org_user：白名单严格搜索 wukong01/wukong02/汀葻
         （这是首选自动发现路径，确保只在指定账号中挑选）
      3) 兜底：复用根 conftest 的 other_user_id（仅 userId），通过
         _get_open_dingtalk_id_by_user_id 反查 openDingTalkId（依赖
         dws contact user get --ids 返回姓名字段；当前 dws 实现下大概率
         拿不到，仅在白名单完全落空时才会触达）
      4) 全部失败 → 返回 (None, None)，由调用方 pytest.skip
    """
    # 1) 环境变量同时齐全 → 直接用，无需任何 dws 调用
    env_uid = os.environ.get("DINGTALK_TEST_OTHER_USER_ID", "").strip()
    env_odid = os.environ.get("DINGTALK_TEST_OTHER_OPEN_DINGTALK_ID", "").strip()
    if env_uid and env_odid:
        return env_uid, env_odid

    # 2) 白名单严格搜索（优先于根 conftest，避免"随便选人"）
    uid, odid = _find_other_org_user(dws, current_user_id)
    if uid and odid:
        return uid, odid

    # 3) 兜底：复用根 conftest 的 other_user_id（lazy 取，避免根 fixture 报错时直接挂掉）
    root_other_uid = None
    try:
        root_other_uid = request.getfixturevalue("other_user_id")
    except Exception as exc:  # 根 fixture pytest.skip / 解析失败均视为不可用
        print(
            f"\n[conftest] _chat_other_user_pair: "
            f"白名单搜索全部落空后尝试根 conftest other_user_id 也失败"
            f"（{type(exc).__name__}）。"
        )

    if root_other_uid:
        odid2 = _get_open_dingtalk_id_by_user_id(dws, root_other_uid)
        if odid2:
            print(
                f"\n[conftest] 🤝 _chat_other_user_pair 兜底复用根 conftest "
                f"other_user_id: userId={root_other_uid} openDingTalkId={odid2}"
            )
            return root_other_uid, odid2
        print(
            f"\n[conftest] _chat_other_user_pair: "
            f"根 conftest other_user_id={root_other_uid} 无法反查 openDingTalkId，"
            f"放弃。"
        )

    return None, None

@pytest.fixture(scope="session")
def chat_other_user_id(_chat_other_user_pair):
    """同组织内的另一个 userId（用于建双人群、转让群主等需要 userId 的命令）。

    解析顺序详见 _chat_other_user_pair。
    """
    uid, _ = _chat_other_user_pair
    if uid:
        return uid
    pytest.skip(
        "无法解析同组织非 self userId。\n"
        f"  尝试过白名单关键词: {_FIND_OTHER_USER_KEYWORDS}\n"
        "  → 可显式设置 DINGTALK_TEST_OTHER_USER_ID=<同组织用户 userId>"
        " 与 DINGTALK_TEST_OTHER_OPEN_DINGTALK_ID=<对应 openDingTalkId>"
    )

@pytest.fixture(scope="session")
def chat_other_open_dingtalk_id(_chat_other_user_pair):
    """同组织内的另一个 openDingTalkId（用于 group-mute-member / set-admin 的 --users 参数）。

    解析顺序详见 _chat_other_user_pair。
    """
    _, odid = _chat_other_user_pair
    if odid:
        return odid
    pytest.skip(
        "无法解析同组织非 self openDingTalkId。\n"
        f"  尝试过白名单关键词: {_FIND_OTHER_USER_KEYWORDS}\n"
        "  → 可显式设置 DINGTALK_TEST_OTHER_OPEN_DINGTALK_ID=<同组织用户 openDingTalkId>"
    )

@pytest.fixture(scope="session")
def multi_user_group_id(dws, current_user_id, chat_other_user_id):
    """Session-scoped 双人群 — 用于 mute-member/set-admin 等需要第二个用户的测试。

    使用 chat_other_user_id（来自已存在群的同组织成员，userId 非空保证同组织）。
    """
    name = f"CI_双人群_{int(time.time())}_{id(object()) % 10000}"
    proc = dws.run_raw(
        "chat", "group", "create",
        "--name", name,
        "--users", f"{current_user_id},{chat_other_user_id}",
    )
    data = _parse_json(proc)
    assert data is not None, f"chat group create 返回非 JSON: stdout={proc.stdout[:200]}"
    assert data.get("success") is True, f"chat group create 未成功: {_json.dumps(data, ensure_ascii=False)[:200]}"
    cid = data.get("result", {}).get("openConversationId")
    assert cid, f"chat group create 缺少 openConversationId: {data}"
    return cid


@pytest.fixture(scope="session")
def session_msg_id(msg_id):
    """Session-scoped 消息 openMsgId — 历史命名兼容，**直接复用 msg_id**。

    旧用例（reply/forward）以 session_msg_id 命名引用消息，现统一复用
    msg_id，避免在共享群里多发一条无意义的消息。
    """
    return msg_id
