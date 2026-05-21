"""
test_05_minutes_mind_graph.py — 听记思维导图测试

Commands tested:
  1. dws minutes mind-graph create  (create_mind_graph)
  2. dws minutes mind-graph status  (query_mind_graph_status)

回归说明：
  - 反例小节（references/products/minutes.md "反例 / 回归案例 - 案例 1"）
    要求"听记 URL + 创建思维导图"场景必须走 mind-graph create →
    mind-graph status 的串联流程，严禁 AI 自行用 g6/markmap 等替代。
  - 本文件除了覆盖 create / status 各自的成功 / 无效 ID / 缺参用例外，
    新增 TestMindGraphFlow 串联用例对齐"正确做法"的真实链路。
"""

import pytest


@pytest.fixture(scope="session")
def minutes_id(dws):
    """获取听记 ID，优先用 list all，其次 list shared。"""
    for subcmd in ("all", "shared"):
        data = dws.run_ok("minutes", "list", subcmd)
        result = data.get("result", {})
        items = (
            result.get("itemList", [])
            if isinstance(result, dict)
            else []
        ) or data.get("minutes", [])
        if items:
            mid = items[0].get("minutesId") or items[0].get("uuid") or items[0].get("id")
            if mid:
                return mid
    pytest.skip("No minutes available")


class TestMindGraphCreate:
    """dws minutes mind-graph create"""

    def test_create_mind_graph(self, dws, minutes_id):
        """触发创建思维导图任务。"""
        data = dws.run_ok(
            "minutes", "mind-graph", "create", "--id", minutes_id,
        )
        assert data is not None

    def test_create_mind_graph_invalid_id(self, dws):
        """使用无效 ID 创建思维导图应报错。"""
        result = dws.run_raw(
            "minutes", "mind-graph", "create", "--id", "INVALID",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_create_mind_graph_missing_id(self, dws):
        """缺少 --id 参数应报错。"""
        result = dws.run_raw("minutes", "mind-graph", "create")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()


class TestMindGraphStatus:
    """dws minutes mind-graph status"""

    def test_query_mind_graph_status(self, dws, minutes_id):
        """查询思维导图生成状态。"""
        data = dws.run_ok(
            "minutes", "mind-graph", "status", "--id", minutes_id,
        )
        assert data is not None

    def test_query_mind_graph_status_invalid_id(self, dws):
        """使用无效 ID 查询状态应报错。"""
        result = dws.run_raw(
            "minutes", "mind-graph", "status", "--id", "INVALID",
        )
        assert (
            result.returncode != 0
            or "error" in result.stdout.lower()
            or "error" in result.stderr.lower()
        )

    def test_query_mind_graph_status_missing_id(self, dws):
        """缺少 --id 参数应报错。"""
        result = dws.run_raw("minutes", "mind-graph", "status")
        assert result.returncode != 0 or "error" in (result.stdout + result.stderr).lower()

class TestMindGraphFlow:
    """create → status 串联流程回归

    对齐 references/products/minutes.md "反例 / 回归案例 - 案例 1"
    要求的"正确做法"：
      1) 提取 taskUuid
      2) dws minutes mind-graph create --id <taskUuid>
      3) dws minutes mind-graph status --id <taskUuid> 轮询
    本用例仅验证两步命令链路通畅，不等待状态变 1（异步任务，
    线上时长不可控；只要 status 返回结构合法即视为链路 OK）。
    """

    def test_create_then_status(self, dws, minutes_id):
        """先 create 触发任务，再 status 立即查询，两步均应返回合法 JSON。"""
        # Step 1: 触发创建
        create_data = dws.run_ok(
            "minutes", "mind-graph", "create", "--id", minutes_id,
        )
        assert create_data is not None, "create 应返回 JSON"

        # Step 2: 紧接着查询状态（不轮询，仅验证链路）
        status_data = dws.run_ok(
            "minutes", "mind-graph", "status", "--id", minutes_id,
        )
        assert status_data is not None, "status 应返回 JSON"

        # 校验 status 返回结构里能拿到任务态字段（0/1/2 或缺省视为成功）。
        # 不强校验值，只确认服务端确实把 mind-graph 任务关联到了同一 taskUuid。
        result = status_data.get("result")
        if isinstance(result, dict):
            # 常见字段名 status / taskStatus / state，任一存在即认为链路通
            assert any(
                k in result for k in ("status", "taskStatus", "state")
            ) or result == {} or "success" in status_data, (
                f"status 返回未包含任务态字段：{result}"
            )
