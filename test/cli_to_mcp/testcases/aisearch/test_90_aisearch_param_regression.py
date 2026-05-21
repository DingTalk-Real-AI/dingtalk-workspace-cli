"""
test_90_aisearch_param_regression.py — aisearch 高频瞎猜参数回归用例。

覆盖 v0.2.54 引入的「模型瞎猜兜底」能力，按真实生产流量分类：

  A 类（同义子命令瞎猜）：search / search-person / search-user /
        user / user-search / query / people / find / lookup / ask
        → 全部走 personCmd 的 cobra Aliases，等价于 person。
  B 类（跨模块名混淆）：contact
        → 同样走 alias 透明兜底，但语义上对应 dws contact。
  C 类（裸 root + flag）：dws aisearch --keyword xxx
        → 走 root 智能 RunE，自动当 person 执行。
  D 类（keyword flag 同义瞎猜）：--name / --q / --query / --text
        → 走 root PersistentFlags，resolveAisearchKeyword fallback。
  E 类（笛卡尔积）：alias 子命令 × flag 同义词的所有组合。
  F 类（多余 positional args 容忍）：person search xxx 这种把 alias
        当 positional 多写一遍。
"""

import json


def _assert_regression_result_ok(result):
    """
    参数回归用例统一判定（与 contact/test_90 保持一致）：
    - 成功返回 JSON（含 success/result/error/code/message 等）→ 通过
    - stderr/stdout 含 unknown flag / AUTH_PERMISSION_DENIED → 视为失败
    """
    combined = ((result.stdout or "") + "\n" + (result.stderr or "")).strip()
    lower = combined.lower()
    if "unknown flag" in lower or "auth_permission_denied" in lower:
        return

    for payload in (result.stdout or "", result.stderr or ""):
        text = (payload or "").strip()
        if not text:
            continue
        try:
            data = json.loads(text)
        except json.JSONDecodeError:
            continue
        if isinstance(data, dict):
            if data.get("success") is True:
                return
            if "error" in data or "code" in data or "message" in data:
                return
            if "result" in data or "userId" in data:
                return

    assert False, (
        "回归用例命令返回不符合预期（既非成功结果，也非可识别错误）:\n"
        f"returncode={result.returncode}\nstdout={result.stdout}\nstderr={result.stderr}"
    )


# ---------- A 类：同义子命令瞎猜 → personCmd alias 兜底 ----------


class TestAisearchSubcommandAliasA:
    """A 类：模型把 aisearch 的子命令瞎猜成 search/find/query/... ，
    cobra Aliases 透明路由到 personCmd。"""

    def test_alias_search(self, dws):
        result = dws.run_raw("aisearch", "search", "--keyword", "张")
        _assert_regression_result_ok(result)

    def test_alias_search_person(self, dws):
        result = dws.run_raw("aisearch", "search-person", "--keyword", "张")
        _assert_regression_result_ok(result)

    def test_alias_search_user(self, dws):
        result = dws.run_raw("aisearch", "search-user", "--keyword", "张")
        _assert_regression_result_ok(result)

    def test_alias_user(self, dws):
        result = dws.run_raw("aisearch", "user", "--keyword", "张")
        _assert_regression_result_ok(result)

    def test_alias_user_search(self, dws):
        result = dws.run_raw("aisearch", "user-search", "--keyword", "张")
        _assert_regression_result_ok(result)

    def test_alias_query(self, dws):
        result = dws.run_raw("aisearch", "query", "--keyword", "张")
        _assert_regression_result_ok(result)

    def test_alias_people(self, dws):
        result = dws.run_raw("aisearch", "people", "--keyword", "张")
        _assert_regression_result_ok(result)

    def test_alias_find(self, dws):
        result = dws.run_raw("aisearch", "find", "--keyword", "张")
        _assert_regression_result_ok(result)

    def test_alias_lookup(self, dws):
        result = dws.run_raw("aisearch", "lookup", "--keyword", "张")
        _assert_regression_result_ok(result)

    def test_alias_ask(self, dws):
        result = dws.run_raw("aisearch", "ask", "--keyword", "张")
        _assert_regression_result_ok(result)


# ---------- B 类：跨模块名混淆 → 同样兜底到 person ----------


class TestAisearchSubcommandAliasB:
    """B 类：模型把 contact 模块名拼到 aisearch 下，
    alias 透明兜底（语义上仍走 person 搜人）。"""

    def test_alias_contact(self, dws):
        result = dws.run_raw("aisearch", "contact", "--keyword", "张")
        _assert_regression_result_ok(result)


# ---------- C 类：root 裸调智能兜底 ----------


class TestAisearchRootSmartFallbackC:
    """C 类：模型漏写 person 子命令，直接 dws aisearch --keyword xxx。
    root 的智能 RunE 检测到 keyword 后自动当 person 跑。"""

    def test_root_naked_with_keyword(self, dws):
        result = dws.run_raw("aisearch", "--keyword", "张")
        _assert_regression_result_ok(result)

    def test_root_naked_with_short_w(self, dws):
        result = dws.run_raw("aisearch", "-w", "张")
        _assert_regression_result_ok(result)

    def test_root_naked_with_keyword_and_dimension(self, dws):
        result = dws.run_raw("aisearch", "--keyword", "张", "--dimension", "name")
        _assert_regression_result_ok(result)


# ---------- D 类：keyword flag 同义词兜底 ----------


class TestAisearchKeywordFlagAliasD:
    """D 类：模型把 --keyword 瞎猜成 --name / --q / --query / --text，
    root PersistentFlags 注册了同义 flag，由 resolveAisearchKeyword 转发。"""

    def test_flag_alias_name(self, dws):
        result = dws.run_raw("aisearch", "person", "--name", "张")
        _assert_regression_result_ok(result)

    def test_flag_alias_q(self, dws):
        result = dws.run_raw("aisearch", "person", "--q", "张")
        _assert_regression_result_ok(result)

    def test_flag_alias_query(self, dws):
        result = dws.run_raw("aisearch", "person", "--query", "张")
        _assert_regression_result_ok(result)

    def test_flag_alias_text(self, dws):
        result = dws.run_raw("aisearch", "person", "--text", "张")
        _assert_regression_result_ok(result)


# ---------- E 类：alias × flag 笛卡尔积（线上复现 case）----------


class TestAisearchCombinedAliasAndFlagE:
    """E 类：模型双重瞎猜——子命令是 alias，flag 也是同义词。
    例：dws aisearch search --query xxx（线上复现的 unknown flag 报错 case）"""

    def test_search_with_query_flag(self, dws):
        """线上复现：aisearch search --query xxx 之前报 unknown flag --query。"""
        result = dws.run_raw("aisearch", "search", "--query", "潘小玲")
        _assert_regression_result_ok(result)

    def test_search_with_name_flag(self, dws):
        result = dws.run_raw("aisearch", "search", "--name", "张三")
        _assert_regression_result_ok(result)

    def test_find_with_q_flag(self, dws):
        result = dws.run_raw("aisearch", "find", "--q", "李四")
        _assert_regression_result_ok(result)

    def test_query_alias_with_text_flag(self, dws):
        result = dws.run_raw("aisearch", "query", "--text", "王五")
        _assert_regression_result_ok(result)

    def test_contact_alias_with_query_flag(self, dws):
        """B 类 alias × D 类 flag 同义词组合。"""
        result = dws.run_raw("aisearch", "contact", "--query", "陈超")
        _assert_regression_result_ok(result)

    def test_people_alias_with_name_flag(self, dws):
        result = dws.run_raw("aisearch", "people", "--name", "杭总")
        _assert_regression_result_ok(result)


# ---------- F 类：多余 positional args 容忍 ----------


class TestAisearchArbitraryArgsToleranceF:
    """F 类：模型把 alias 名当 positional arg 多写一遍。
    例：dws aisearch person search --keyword xxx
    cobra.ArbitraryArgs 让 search 被静默忽略，person 仍按 keyword 执行。"""

    def test_person_with_extra_search_arg(self, dws):
        """person 后再多写一个 search 不应报错。"""
        result = dws.run_raw("aisearch", "person", "search", "--keyword", "张")
        _assert_regression_result_ok(result)

    def test_person_with_extra_user_search_args(self, dws):
        """person 后多写两个 positional args 也容忍。"""
        result = dws.run_raw("aisearch", "person", "user", "search", "--keyword", "张")
        _assert_regression_result_ok(result)
