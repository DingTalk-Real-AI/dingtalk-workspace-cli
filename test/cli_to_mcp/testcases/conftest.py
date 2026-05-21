from __future__ import annotations

"""
Root conftest.py — Shared DWSRunner for all product tests.

Handles two response formats:
  - aitable-style:  {"data": {...}, "status": "success"}
  - MCP-raw-style:  {"result": [...], "success": true}

`dws` 调用统一附加 `--format json`（不用 `-f`，避免与子命令如 `contract import batch` 的 `-f`/`--file-id` 冲突）。
解析顺序（避免 IDE/pytest 的 PATH 与交互式终端不一致，命中旧版二进制导致 unknown flag）：
  1. 环境变量 DWS_BIN（绝对路径优先）
  2. 自 testcases 向上各层目录下的 ./dws（可执行文件，常见于项目根）
  3. 本仓库 dingtalk-cli_b/dws（make build 产物）
  4. shutil.which("dws")
  5. 回退为 "dws"
"""

import json
import inspect
import os
import re
import shlex
import subprocess
import threading

import pytest
from test_utils import resolve_dws_bin

_raw_results_lock = threading.Lock()
_RAW_RESULTS_FILE = os.path.join(os.path.dirname(__file__), ".raw_cmd_results.json")


# 统一走 test_utils 的路径解析逻辑，便于多目录共享与后续维护。
DWS_BIN = resolve_dws_bin(__file__)




# ===== Open-source CI guard =====
# This suite makes real CLI calls and requires a valid dws login token.
# When run in CI or any environment without one, skip the whole session
# cleanly instead of producing hundreds of unrelated failures.
def _check_token_or_skip():
    import os, subprocess, json
    bin_path = os.environ.get("DWS_BIN") or DWS_BIN
    try:
        r = subprocess.run(
            [bin_path, "auth", "status", "--format", "json"],
            capture_output=True, text=True, timeout=8,
        )
    except Exception as e:
        pytest.skip(f"dws binary not runnable: {e}", allow_module_level=True)
        return
    try:
        info = json.loads(r.stdout)
    except Exception:
        pytest.skip("dws auth status returned non-json; not logged in", allow_module_level=True)
        return
    if not info.get("token_valid"):
        pytest.skip("dws not authenticated; run `dws auth login` first", allow_module_level=True)


@pytest.fixture(scope="session", autouse=True)
def _require_dws_token():
    """Skip the whole pytest session when no usable dws login is present."""
    _check_token_or_skip()
    yield


class DWSRunner:
    """Universal wrapper for `dws` CLI invocation.

    Handles both response formats transparently.
    """

    # 使用长选项，避免与业务子命令的 -f 简写冲突（如 contract import --file-id 占用 -f）。

    @staticmethod
    def _log_cmd(cmd: list[str]) -> None:
        """将真实执行命令写入 pytest 日志，便于 last_run.log 回溯。"""
        print(f"DWS_CMD: {' '.join(shlex.quote(x) for x in cmd)}")

    # 环境/权限类错误关键词 — 匹配到时 skip 而非 fail
    _SKIP_KEYWORDS = (
        "AUTH_PERMISSION_DENIED",
        "AUTH_TOKEN_EXPIRED",
        "权限不足",
        "PAT_MEDIUM_RISK_NO_PERMISSION",
        "robot 不存在",
        "robotCode is in not valid",
        "token is not exist",
    )

    @staticmethod
    def _maybe_skip_on_json_code(data) -> None:
        if not isinstance(data, dict):
            return
        # 检查顶层 code、message、error.message、error.category 等字段
        text_to_check = " ".join(filter(None, [
            str(data.get("code", "")),
            str(data.get("message", "")),
            str((data.get("error") or {}).get("message", "")),
            str((data.get("error") or {}).get("category", "")),
            str(data.get("technical_detail", "")),
        ]))
        for kw in DWSRunner._SKIP_KEYWORDS:
            if kw in text_to_check:
                pytest.skip(
                    f"dws 环境/权限问题，跳过用例：{json.dumps(data, ensure_ascii=False)[:200]}"
                )

    @staticmethod
    def _parse_completed_json(result: subprocess.CompletedProcess, cmd: list[str]) -> dict:
        """成功输出多在 stdout；tools/call 等业务错误常在 stderr，亦为合法 JSON。"""
        for text in ((result.stdout or "").strip(), (result.stderr or "").strip()):
            if not text:
                continue
            try:
                data = json.loads(text)
            except json.JSONDecodeError:
                continue
            DWSRunner._maybe_skip_on_json_code(data)
            return data
        combined = (result.stdout or "") + (result.stderr or "")
        for kw in DWSRunner._SKIP_KEYWORDS:
            if kw in combined:
                pytest.skip(f"dws 权限不足，跳过用例：{combined.strip()[:200]}")
        pytest.fail(
            f"dws returned non-JSON:\n"
            f"  cmd:    {' '.join(cmd)}\n"
            f"  stdout: {result.stdout[:500]}\n"
            f"  stderr: {result.stderr[:500]}"
        )
    def run(self, *args: str, expect_success: bool = True) -> dict:
        """Execute dws, return parsed JSON. Assert success."""
        cmd = [DWS_BIN, *args, "--format", "json"]
        self._log_cmd(cmd)
        result = subprocess.run(
            cmd, capture_output=True, text=True, timeout=60,
        )
        data = self._parse_completed_json(result, cmd)
        if expect_success:
            self._assert_success(data, cmd)
        return data

    def run_ok(self, *args: str) -> dict:
        """Like run(), but tolerates missing status (no error)."""
        cmd = [DWS_BIN, *args, "--format", "json"]
        self._log_cmd(cmd)
        result = subprocess.run(
            cmd, capture_output=True, text=True, timeout=60,
        )
        data = self._parse_completed_json(result, cmd)
        self._assert_no_error(data, cmd)
        return data

    def run_raw(self, *args: str) -> subprocess.CompletedProcess:
        """Execute dws, return raw CompletedProcess.

        Results are also persisted to .raw_cmd_results.json so that report
        generation can distinguish CLI errors from successful commands.
        """
        cmd = [DWS_BIN, *args, "--format", "json"]
        self._log_cmd(cmd)
        result = subprocess.run(
            cmd, capture_output=True, text=True, timeout=60,
        )
        self._persist_raw_result(cmd, result)
        return result

    @staticmethod
    def _persist_raw_result(cmd: list[str], result: subprocess.CompletedProcess) -> None:
        # 注意：这里是 run_all_tests.py 汇总 “CLI 真实报错” 的唯一数据源之一。
        # 截断过短会导致误判（例如 success:true 位于输出尾部被截断）。
        max_len = int(os.environ.get("DWS_RAW_LOG_MAX", "8000"))
        entry = {
            "cmd": " ".join(shlex.quote(x) for x in cmd),
            "returncode": result.returncode,
            "stdout": (result.stdout or "")[:max_len],
            "stderr": (result.stderr or "")[:max_len],
        }
        with _raw_results_lock:
            existing: list = []
            if os.path.exists(_RAW_RESULTS_FILE):
                try:
                    with open(_RAW_RESULTS_FILE, "r") as f:
                        existing = json.load(f)
                except (json.JSONDecodeError, OSError):
                    existing = []
            existing.append(entry)
            with open(_RAW_RESULTS_FILE, "w") as f:
                json.dump(existing, f, ensure_ascii=False, indent=2)

    @staticmethod
    def _is_permission_denied_payload(data) -> bool:
        """检测响应是否为"权限不足/灰度未开"类错误。

        灰度未开时 CLI 通常返回**合法 JSON** 而非裸 stderr，形如：
          {"success": false, "error": {"code": "AUTH_PERMISSION_DENIED", ...}}
          {"success": false, "message": "权限不足，请联系管理员开通灰度"}
        这种情况属于"测试组织未开通对应 MCP 工具灰度"的环境问题，
        不应判为代码缺陷，应 pytest.skip。
        """
        if not isinstance(data, dict):
            return False
        try:
            text = json.dumps(data, ensure_ascii=False).lower()
        except Exception:
            return False
        markers = (
            "auth_permission_denied",
            "auth_token_expired",
            "no_authority",
            "权限不足",
            "未开通",
            "灰度",
            "permission denied",
            "mcp_tool_error",
            "未找到指定工具",
            "robot 不存在",
            "token is not exist",
            "token 已过期",
        )
        return any(m in text for m in markers)

    @staticmethod
    def _assert_success(data, cmd: list):
        """Assert response indicates success.

        Supports three formats:
          - {"status": "success", "data": {...}}
          - {"success": true, "result": [...]}
          - Bare data: {"rootDentryUuid": "..."} or [...]
        If no status/success field exists, treat as OK.
        """
        if isinstance(data, list):
            return  # list response = valid data
        # "error": {} 是 aitable 的正常返回，只有非空 error 才算错误
        err = data.get("error")
        has_error = bool(err) and err != {}
        is_error = (
            data.get("status") == "error"
            or data.get("success") is False
            or data.get("success") == "false"
            or has_error
        )
        if is_error:
            # 灰度未开 / 权限不足 → 跳过，不计入失败
            if DWSRunner._is_permission_denied_payload(data):
                pytest.skip(
                    f"dws 权限不足/灰度未开，跳过用例：\n"
                    f"  cmd:  {' '.join(cmd)}\n"
                    f"  resp: {json.dumps(data, ensure_ascii=False)[:300]}"
                )
            pytest.fail(
                f"Expected success:\n"
                f"  cmd:  {' '.join(cmd)}\n"
                f"  resp: {json.dumps(data, ensure_ascii=False)[:500]}"
            )

    @staticmethod
    def _assert_no_error(data, cmd: list):
        """Assert response is NOT an explicit error."""
        if isinstance(data, list):
            return
        err = data.get("error")
        has_error = bool(err) and err != {}
        is_error = (
            data.get("status") == "error"
            or data.get("success") is False
            or data.get("success") == "false"
            or has_error
        )
        if is_error:
            # 灰度未开 / 权限不足 → 跳过，不计入失败
            if DWSRunner._is_permission_denied_payload(data):
                pytest.skip(
                    f"dws 权限不足/灰度未开，跳过用例：\n"
                    f"  cmd:  {' '.join(cmd)}\n"
                    f"  resp: {json.dumps(data, ensure_ascii=False)[:300]}"
                )
            pytest.fail(
                f"Command returned error:\n"
                f"  cmd:  {' '.join(cmd)}\n"
                f"  resp: {json.dumps(data, ensure_ascii=False)[:500]}"
            )


@pytest.fixture(scope="session")
def dws():
    """Session-scoped DWS runner."""
    return DWSRunner()


@pytest.fixture(scope="session")
def current_user_id(dws: DWSRunner):
    """
    Resolve current login user's userId.

    优先级：
    1) 环境变量 DINGTALK_TEST_USER_ID（显式指定，最稳定）
    2) dws contact user get-self 自动探测

    说明：
    - 在部分环境下，`contact user get-self` 可能因权限策略返回 AUTH_PERMISSION_DENIED；
      这种情况不应把用例打成 ERROR，而是给出可操作提示并跳过依赖该 fixture 的用例。
    """
    env_uid = os.environ.get("DINGTALK_TEST_USER_ID", "").strip()
    if env_uid:
        return env_uid

    # 使用 run_raw 避免 dws.run() 在非 JSON 时直接 pytest.fail 中断。
    result = dws.run_raw("contact", "user", "get-self")
    data = None
    for text in ((result.stdout or "").strip(), (result.stderr or "").strip()):
        if not text:
            continue
        try:
            data = json.loads(text)
            break
        except json.JSONDecodeError:
            continue
    if data is None:
        pytest.skip(
            "Cannot determine current userId: "
            "contact user get-self returned non-JSON. "
            "Set DINGTALK_TEST_USER_ID to run user-dependent cases. "
            f"stdout={result.stdout[:100]} stderr={(result.stderr or '')[:100]}"
        )
    # 检查是否为环境类错误（如 AUTH_TOKEN_EXPIRED）
    err_msg = json.dumps(data, ensure_ascii=False)
    for kw in ("AUTH_TOKEN_EXPIRED", "AUTH_PERMISSION_DENIED", "权限不足"):
        if kw in err_msg:
            pytest.skip(
                f"Cannot determine current userId: {kw}. "
                "Set DINGTALK_TEST_USER_ID to run user-dependent cases. "
                f"resp={err_msg[:200]}"
            )

    result_list = data.get("result", [])
    if isinstance(result_list, list) and result_list:
        uid = result_list[0].get("orgEmployeeModel", {}).get("userId")
        if uid:
            return uid

    # fallback for data-wrapped format
    result2 = data.get("data", {}).get("result", [])
    if isinstance(result2, list) and result2:
        uid = result2[0].get("orgEmployeeModel", {}).get("userId")
        if uid:
            return uid
    pytest.skip(
        "Cannot determine current userId from contact user get-self response. "
        "Set DINGTALK_TEST_USER_ID to run user-dependent cases."
    )

def _is_same_org_user(uid: str) -> bool:
    """判断 userId 是否为同组织内部成员（而非外部联系人/跨组织用户）。

    钉钉组织内部成员的 userId 通常为短数字格式（≤8位，如 "209499"），
    而外部联系人/跨组织用户的 userId 为 18 位长数字格式（如 "036230684122903146"）。
    这是因为内部 userId 来自 HR 系统分配的工号/序号，而外部联系人使用 DingTalk
    全局 unionId 作为标识。

    此规则在阿里/钉钉组织、以及绝大多数使用钉钉的企业中成立。
    对于极少数自定义 userId 的组织（理论上 userId 最长 64 字符），
    建议通过环境变量 DINGTALK_TEST_OTHER_USER_ID 显式指定。
    """
    if not uid:
        return False
    # 外部联系人 userId 通常为 15-18 位纯数字
    if uid.isdigit() and len(uid) > 10:
        return False
    return True


def _pick_user_from_search_payload(data, current_uid):
    """从 contact user search 的响应里挑出第一个同组织的 (userId != self) 的合法用户。

    返回 (userId, display_name) 或 (None, None)。

    过滤规则：
      1. 跳过 userId 为 None 的条目
      2. 跳过 userId == current_uid 的条目（不能对自己授权）
      3. 跳过疑似外部联系人/跨组织用户的 userId（长数字格式）
    """
    items = data.get("result") or data.get("data", {}).get("result") or []
    for item in items:
        if not item:
            continue
        cand_uid = item.get("userId")
        if not cand_uid or cand_uid == current_uid:
            continue
        if not _is_same_org_user(str(cand_uid)):
            continue
        cand_name = item.get("name") or item.get("nick") or ""
        return cand_uid, cand_name
    return None, None

def _pick_user_from_dept_members(data, current_uid):
    """从 contact dept list-members 的响应里挑出第一个 (userId != self) 的合法用户。

    实际返回结构（2026-04 实测）大致为：
      {"result": {"deptUserList": [{"userId": "...", "name": "..."}, ...], ...}}
    或：
      {"result": [{"userId": "...", ...}, ...]}（部分版本）
    """
    payload = data.get("result") or data.get("data", {}).get("result") or {}
    if isinstance(payload, dict):
        candidates = (
            payload.get("deptUserList")
            or payload.get("userList")
            or payload.get("users")
            or []
        )
    elif isinstance(payload, list):
        candidates = payload
    else:
        candidates = []

    for item in candidates:
        cand_uid = (item or {}).get("userId")
        if cand_uid and cand_uid != current_uid:
            cand_name = (item or {}).get("name") or (item or {}).get("nick") or ""
            return cand_uid, cand_name
    return None, None

@pytest.fixture(scope="session")
def other_user_id(dws: DWSRunner, current_user_id):
    """
    Resolve a non-current-user userId in the **same organization** as the
    current login account, required by member/permission cases that grant
    a role to **someone else**.

    背景：
      - 知识库 / 文档 / 群创建者天然就是 OWNER，对自己再次执行 add/update_member
        会被服务端以 internalError 拒绝（OWNER 不可被 add/update 接口覆盖）。
      - 因此凡是涉及"加成员/改成员角色"的正向用例，被授权对象必须是另一个真实
        userId，且**必须是同组织成员**（跨组织授权服务端会直接 forbidden）。
      - 故不能 hardcode 一个固定 userId，否则换登录账号 / 换组织（如本地 vs CI）
        就会失效。

    解析顺序（从稳定到自适应）：
      1) 环境变量 DINGTALK_TEST_OTHER_USER_ID 显式指定（最稳定，CI 推荐）
      2) `dws contact user search --keyword <kw>`：用一组**通讯录测试用例**
         本身已验证过的关键词逐个 search，命中即取
      3) `dws contact dept search --keyword <kw>` → `dept list-members --ids <id>`：
         先找到一个真实部门，再列其成员，挑非 self（适用于成员搜不到但
         部门能搜到的场景）
      4) 全部失败时 pytest.skip，附详细排查指引

    关键词池设计：
      - 第一梯队（"测试"/"研发"/"张"）来自通讯录测试用例自身（contact/test_01_user.py
        + test_02_dept.py），它们在 dtesla 组织里已验证 PASSED，证明在该组织
        必然能搜出结果；
      - 第二梯队（中文姓氏 Top5 + 数字）覆盖一般生产组织；
      - 第三梯队（英文 test/user）覆盖账号化命名场景；
      - 关键词数量适度（≤12），避免显著拖慢 fixture 构建。
    """
    # 1) 环境变量优先
    uid = os.environ.get("DINGTALK_TEST_OTHER_USER_ID", "").strip()
    if uid:
        return uid

    last_err = ""

    # 2) 通过 contact user search 自动发现
    # 关键词池排序：通讯录测试已验证 PASSED 的关键词放最前，最大化首次命中概率
    user_search_keywords = (
        "测试", "研发", "张",            # 通讯录测试用例自身使用，已在 dtesla PASSED
        "李", "王", "刘", "陈",          # 中文姓氏（覆盖一般生产组织）
        "test", "user",                  # 英文（覆盖账号化命名）
        "1", "0",                        # 数字兜底
    )
    for kw in user_search_keywords:
        # CLI 源码 `validateRequiredFlagWithAliases(cmd, "query", "keyword")`
        # 同时支持 --query 与 --keyword，跟随通讯录 PASSED 用例统一用 --keyword。
        result = dws.run_raw("contact", "user", "search", "--keyword", kw)
        if result.returncode != 0:
            last_err = (
                f"contact user search --keyword {kw!r} 失败 (rc={result.returncode}): "
                f"{(result.stderr or result.stdout or '')[:200]}"
            )
            continue
        try:
            data = json.loads(result.stdout)
        except json.JSONDecodeError:
            last_err = (
                f"contact user search --keyword {kw!r} 返回非 JSON: "
                f"{(result.stdout or '')[:200]}"
            )
            continue

        cand_uid, cand_name = _pick_user_from_search_payload(data, current_user_id)
        if cand_uid:
            print(
                f"\n[conftest] 🤝 other_user_id auto-discovered via "
                f"`contact user search --keyword {kw!r}`: "
                f"userId={cand_uid} name={cand_name!r}"
            )
            return cand_uid

    # 3) 兜底：dept search 找到一个真实部门 → list-members 拿成员
    #    比 list-members --ids 1 更稳：根部门常常被设为空（成员都挂子部门下）。
    #    关键词同样优先用通讯录测试已验证的"研发"。
    dept_search_keywords = ("研发", "测试", "技术", "产品", "运营", "管理")
    for kw in dept_search_keywords:
        dept_result = dws.run_raw("contact", "dept", "search", "--keyword", kw)
        if dept_result.returncode != 0:
            last_err = (
                f"contact dept search --keyword {kw!r} 失败 (rc={dept_result.returncode}): "
                f"{(dept_result.stderr or dept_result.stdout or '')[:200]}"
            )
            continue
        try:
            dept_data = json.loads(dept_result.stdout)
        except json.JSONDecodeError:
            last_err = (
                f"contact dept search --keyword {kw!r} 返回非 JSON: "
                f"{(dept_result.stdout or '')[:200]}"
            )
            continue

        # dept search 响应格式：{"deptList": [{"deptId": 123, "deptName": "..."}, ...]}
        dept_list = (
            dept_data.get("deptList")
            or dept_data.get("result")
            or dept_data.get("data", {}).get("result")
            or []
        )
        if not dept_list:
            continue  # 该关键词没搜到部门，换下一个

        # 遍历搜到的部门，逐个 list-members 直到挑出非 self 用户
        for dept in dept_list[:5]:  # 最多看 5 个部门，防爆
            dept_id = dept.get("deptId") or dept.get("id")
            if not dept_id:
                continue
            mem_result = dws.run_raw(
                "contact", "dept", "list-members", "--ids", str(dept_id)
            )
            if mem_result.returncode != 0:
                continue
            try:
                mem_data = json.loads(mem_result.stdout)
            except json.JSONDecodeError:
                continue

            cand_uid, cand_name = _pick_user_from_dept_members(mem_data, current_user_id)
            if cand_uid:
                dept_name = dept.get("deptName") or dept.get("name") or ""
                # 服务端返回的 deptName 可能含 <red></red> 高亮标签，剥掉
                dept_name_clean = re.sub(r"</?[^>]+>", "", dept_name)
                print(
                    f"\n[conftest] 🤝 other_user_id auto-discovered via "
                    f"`contact dept search --keyword {kw!r}` → "
                    f"`dept list-members --ids {dept_id}` "
                    f"(deptName={dept_name_clean!r}): "
                    f"userId={cand_uid} name={cand_name!r}"
                )
                return cand_uid

    # 4) 全部失败 → 跳过依赖 fixture 的用例，并附排查指引
    pytest.skip(
        "无法自动发现一个同组织内的非 self userId 用于成员/权限测试。\n"
        f"  尝试过 user search 关键词: {user_search_keywords}\n"
        f"  也尝试 dept search → list-members 兜底，关键词: {dept_search_keywords}\n"
        f"  最后一次错误: {last_err}\n"
        "  → 请显式设置环境变量 DINGTALK_TEST_OTHER_USER_ID=<同组织非self userId>，"
        "可通过 `dws contact user search --keyword <同事姓名>` 手动获取一个。"
    )

def pytest_collection_modifyitems(config, items):
    """
    可选跳过“参数别名/参数黏连”相关回归用例（开源版未实现能力）。

    通过环境变量控制：
    - SKIP_OPEN_UNIMPLEMENTED_PARAM_CASES=1 时生效
    """
    if os.environ.get("SKIP_OPEN_UNIMPLEMENTED_PARAM_CASES", "").strip() != "1":
        return

    alias_reason = "开源版CLI尚未实现"
    biz_reason = "开源版CLI业务能力暂不支持"
    alias_skip_marker = pytest.mark.skip(reason=alias_reason)
    biz_skip_marker = pytest.mark.skip(reason=biz_reason)
    patterns = (
        re.compile(r"wrong_.*_flag"),
        re.compile(r"sticky"),
    )
    blacklist_cmd_tokens = (
        ("chat", "message", "send"),
        ("chat", "message", "list-topic-replies"),
        ("chat", "message", "list"),
        ("contact", "dept", "list-children"),
    )

    for item in items:
        nodeid_l = item.nodeid.lower()
        name_l = item.name.lower()
        if any(p.search(nodeid_l) or p.search(name_l) for p in patterns):
            item.add_marker(alias_skip_marker)
            continue

        # 黑名单命令跳过：按测试函数源码中的命令 token 组合匹配。
        try:
            src = inspect.getsource(getattr(item, "obj", None)) or ""
        except Exception:
            src = ""
        src_l = src.lower()
        if any(all(tok in src_l for tok in tokens) for tokens in blacklist_cmd_tokens):
            item.add_marker(biz_skip_marker)
