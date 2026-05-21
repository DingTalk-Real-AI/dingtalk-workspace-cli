"""
test_93_minutes_badcase_regression.py — 听记基础评测集 16 例 badcase 回归测试

本文件固化 evalrun_4ab46f8da846 中 16 个"未调用 dws 技能"的失败 query 对应的
CLI 层正确行为。每个测试验证：当用户提出某类听记/纪要请求时，dws CLI 能够
返回正确的结构化数据（而非报错或空响应），从而证明 CLI 侧能力完备，问题
出在 Agent 策略层未正确路由到 dws 工具。

Badcase 分类（见 minutes.md 案例 8）：
  A) 模糊/省略型 query → AI 直接反问要细节，不主动 list
  B) 用 session_search/memory_search 搜索历史会话替代 dws
  C) 用 activity:search/web 搜索把"找听记"做成"搜网页"
  D) 钉钉听记/文档 URL 走 browser_use/read_file 而非 dws
  E) 多源数据生成日报/汇报，听记侧 0 调用

测试目标：验证 dws CLI 对各类听记查询命令的正确响应，确保 Agent 有可用的
工具链路可调用。
"""

from typing import Optional

import pytest


def _extract_items(data: dict) -> list:
    """从 list 返回的多种可能结构中健壮地提取 items 列表。"""
    result = data.get("result", {})
    if isinstance(result, dict):
        items = result.get("itemList", []) or result.get("minutes", []) or result.get("items", [])
        if items:
            return items
    if isinstance(result, list):
        return result
    return data.get("minutes", []) or data.get("itemList", []) or data.get("items", [])


def _extract_id(item: dict) -> Optional[str]:
    """从单个听记 item 中提取 ID。"""
    return item.get("taskUuid") or item.get("minutesId") or item.get("uuid") or item.get("id")


class TestMinutesListMineBasic:
    """模式 A/B/E 的基础前置：list mine 必须能返回有效数据。

    对应 query：
    - 0049 按关键词搜索我的听记
    - 0057 查列表+看摘要
    - 0063 周会回顾整理
    - 0064 评测工作复盘
    - 0003 帮我查一下最近一次会议的纪要内容
    - 0017 总结下我的会议
    - 0020 我昨天那个会的重点帮我提炼一下
    - 0027 把最近一次会议的待办整理出来
    """

    def test_list_mine_returns_valid_structure(self, dws):
        """dws minutes list mine 应返回有效结构（含 items 列表）。"""
        data = dws.run_ok("minutes", "list", "mine")
        # 兼容多种返回结构：success 字段可能不存在
        items = _extract_items(data)
        assert isinstance(items, list), f"无法从返回数据中提取 items 列表: {list(data.keys())}"

    def test_list_mine_items_have_required_fields(self, dws):
        """list mine 返回的每项应含 taskUuid/uuid 等关键字段。"""
        data = dws.run_ok("minutes", "list", "mine")
        items = _extract_items(data)
        if not items:
            pytest.skip("当前账号无听记数据，跳过字段校验")
        first = items[0]
        # taskUuid 是后续 get summary/get todos 等的必需入参
        has_id = bool(_extract_id(first))
        assert has_id, f"听记项缺少 ID 字段: {first.keys()}"

    def test_list_mine_with_query_keyword(self, dws):
        """list mine --query 支持关键词筛选。对应 0049/0025/0060。"""
        data = dws.run_ok("minutes", "list", "mine", "--query", "周会")
        assert data.get("success") is True

    def test_list_mine_with_max_limit(self, dws):
        """list mine --max 限制返回数量。对应 0057/0063。"""
        data = dws.run_ok("minutes", "list", "mine", "--max", "5")
        items = _extract_items(data)
        assert len(items) <= 5


class TestMinutesListAllWithQuery:
    """模式 C 的正确做法：用 list all --query 而非 web 搜索。

    对应 query：
    - 0025 调取某某项目讨论的两个听记内容
    - 0060 搜索+摘要+关键词
    """

    def test_list_all_with_query_keyword(self, dws):
        """dws minutes list all --query 支持全局关键词搜索。"""
        data = dws.run_ok("minutes", "list", "all", "--query", "周会")
        # 只需验证命令执行成功并返回了有效数据结构
        items = _extract_items(data)
        assert isinstance(items, list)

    def test_list_all_with_time_range(self, dws):
        """list all 支持 --start/--end 时间范围筛选。对应 0020/0063。"""
        data = dws.run_ok(
            "minutes", "list", "all",
            "--start", "2026-05-01T00:00:00+08:00",
            "--end", "2026-05-31T23:59:59+08:00",
        )
        items = _extract_items(data)
        assert isinstance(items, list)


class TestMinutesGetSummaryFromId:
    """模式 B/D 的正确做法：从 list 拿到 taskUuid 后 get summary。

    对应 query：
    - 0003 帮我查一下最近一次会议的纪要内容
    - 0017 总结下我的会议
    - 0020 我昨天那个会的重点帮我提炼一下
    - 0045/0046/0047 URL 解析后获取摘要
    """

    @pytest.fixture(scope="class")
    def sample_minutes_id(self, dws):
        """获取一个有效的听记 ID 供后续测试使用。"""
        data = dws.run_ok("minutes", "list", "mine", "--max", "1")
        items = _extract_items(data)
        if not items:
            pytest.skip("当前账号无听记数据，无法进行 get summary 测试")
        mid = _extract_id(items[0])
        if not mid:
            pytest.skip("听记项缺少 ID 字段")
        return mid

    def test_get_summary_returns_valid_data(self, dws, sample_minutes_id):
        """dws minutes get summary --id <uuid> 应返回有效摘要。"""
        data = dws.run_ok("minutes", "get", "summary", "--id", sample_minutes_id)
        assert isinstance(data, dict) and len(data) > 0

    def test_get_summary_invalid_id_returns_error(self, dws):
        """无效 taskUuid 应返回错误而非静默成功。对应 0045/0046/0047 的边界处理。"""
        result = dws.run_raw("minutes", "get", "summary", "--id", "INVALID_TASK_UUID")
        # CLI 应对无效 ID 给出明确错误提示
        combined = (result.stdout or "") + (result.stderr or "")
        assert (
            result.returncode != 0
            or "error" in combined.lower()
        )


class TestMinutesGetTodosFromId:
    """模式 B 的正确做法：用 get todos 而非从历史会话复述。

    对应 query：
    - 0027 把最近一次会议的待办整理出来
    """

    def test_get_todos_returns_valid_data(self, dws):
        """dws minutes get todos --id <uuid> 应返回待办列表。"""
        data = dws.run_ok("minutes", "list", "mine", "--max", "1")
        items = _extract_items(data)
        if not items:
            pytest.skip("当前账号无听记数据，无法进行 get todos 测试")
        mid = _extract_id(items[0])
        if not mid:
            pytest.skip("听记项缺少 ID 字段")
        data = dws.run_ok("minutes", "get", "todos", "--id", mid)
        assert isinstance(data, dict)


class TestMinutesListShared:
    """模式 A 的正确做法：共享听记用 list shared 而非反问。

    对应 query：
    - 0061 共享听记+看摘要（虽不属于 16 例未调用 dws，但属于同类模式）
    """

    def test_list_shared_returns_valid_structure(self, dws):
        """dws minutes list shared 应返回有效结构。"""
        data = dws.run_ok("minutes", "list", "shared")
        items = _extract_items(data)
        assert isinstance(items, list)


class TestMinutesDocRead:
    """模式 D 的正确做法：钉钉文档 URL 走 dws doc read 而非 read_file/browser_use。

    对应 query：
    - 0046 https://alidocs.dingtalk.com/i/nodes/sampleDocNode01 帮我读取这个文档内容
    """

    def test_doc_read_command_exists(self, dws):
        """验证 dws doc read 命令存在且能返回帮助信息（不依赖具体文档权限）。"""
        result = dws.run_raw("doc", "read", "--help")
        # --help 应正常返回（returncode 可能为 0 或非 0，但不应报 unknown command）
        output = ((result.stdout or "") + (result.stderr or "")).lower()
        assert "unknown" not in output or result.returncode == 0


class TestMinutesWorkflowIntegration:
    """端到端工作流验证：list → get summary 的完整链路。

    对应所有 16 个 query 的正确执行路径：
    1. 先 list mine/all/shared 拿到 taskUuid
    2. 再 get summary/get todos/get transcription 获取具体内容
    """

    def test_list_then_get_summary_workflow(self, dws):
        """验证 list mine → get summary 的完整工作流。"""
        # Step 1: list mine 获取最近听记
        list_data = dws.run_ok("minutes", "list", "mine", "--max", "1")
        items = _extract_items(list_data)
        if not items:
            pytest.skip("当前账号无听记数据，无法验证完整工作流")
        mid = _extract_id(items[0])
        if not mid:
            pytest.skip("听记项缺少 ID 字段")

        # Step 2: get summary 获取摘要
        summary_data = dws.run_ok("minutes", "get", "summary", "--id", mid)
        assert isinstance(summary_data, dict) and len(summary_data) > 0

    def test_list_with_query_then_get_summary_workflow(self, dws):
        """验证 list all --query → get summary 的工作流。对应 0025/0060。"""
        # Step 1: list all with query
        list_data = dws.run_ok("minutes", "list", "all", "--query", "周会", "--max", "1")
        items = _extract_items(list_data)
        if not items:
            pytest.skip("无匹配'周会'关键词的听记，跳过此测试")
        mid = _extract_id(items[0])
        if not mid:
            pytest.skip("听记项缺少 ID 字段")

        # Step 2: get summary
        summary_data = dws.run_ok("minutes", "get", "summary", "--id", mid)
        assert isinstance(summary_data, dict) and len(summary_data) > 0
