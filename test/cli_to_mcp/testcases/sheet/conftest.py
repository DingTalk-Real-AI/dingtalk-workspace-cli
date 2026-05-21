"""
conftest.py — Sheet (钉钉表格) 测试共享 fixture。
DWSRunner/dws/current_user_id 由根 conftest.py 提供。

采用"自建自销"模式：
  1. session 开始时 create 一个测试表格文档
  2. 写入已知测试数据，供 find 等用例搜索
  3. session 结束后仅打印提示（当前 dws 无文档删除命令，需定期手动清理测试账号下的残留文档）

实际 API 格式 (2026-04):
  sheet create:       {"success": true, "nodeId": "...", ...}
  sheet list:         {"success": true, "sheets": [...], ...}
  sheet info:         {"success": true, ...}
  sheet new:          {"success": true, ...}
  sheet range read:   {"success": true, "values": [[...]], ...}
  sheet range update: {"success": true, ...}
  sheet find:         {"success": true, "matchedCells": [...], "totalCount": N, ...}
"""

import json

import pytest
from test_utils import unique_name


# ─── 测试数据常量 ──────────────────────────────────────────
# 这些值会被写入测试表格，find 用例中的搜索关键词必须与此对应。

SEED_HEADER = ["姓名", "部门", "金额", "状态"]
SEED_ROWS = [
    ["张三", "销售部", 50000, "完成"],
    ["李四", "市场部", 38000, "待处理"],
    ["王五", "销售部", 62000, "完成"],
    ["Test_User", "研发部", 99000, "pending"],
]


# ─── Sheet 文档生命周期 ─────────────────────────────────────

@pytest.fixture(scope="session")
def test_sheet_info(dws):
    """创建测试表格文档，写入种子数据，返回 nodeId + sheetId。

    注意：当前 dws 无文档删除命令，teardown 仅打印资源信息便于手动清理。
    测试账号下的残留文档需定期人工清理。
    """
    # 1. 创建表格文档
    doc_name = unique_name("CLI_Test_Sheet")
    create_data = dws.run("sheet", "create", "--name", doc_name)
    node_id = (
        create_data.get("nodeId")
        or create_data.get("data", {}).get("nodeId")
    )
    assert node_id, f"sheet create 未返回 nodeId: {create_data}"
    print(f"\n[SETUP] Created test sheet doc: {node_id} ({doc_name})")

    # 2. 获取默认工作表 ID
    list_data = dws.run("sheet", "list", "--node", node_id)
    sheets = (
        list_data.get("sheets")
        or list_data.get("data", {}).get("sheets")
        or []
    )
    assert sheets, f"sheet list 返回空工作表列表: {list_data}"
    sheet_id = sheets[0].get("sheetId") or sheets[0].get("id") or sheets[0].get("name")
    assert sheet_id, f"无法提取 sheetId: {sheets[0]}"
    print(f"[SETUP] Default sheet: {sheet_id}")

    # 3. 写入表头
    header_json = json.dumps([SEED_HEADER], ensure_ascii=False)
    dws.run(
        "sheet", "range", "update",
        "--node", node_id,
        "--sheet-id", sheet_id,
        "--range", f"A1:D1",
        "--values", header_json,
    )

    # 4. 写入数据行
    row_count = len(SEED_ROWS)
    values_json = json.dumps(SEED_ROWS, ensure_ascii=False)
    dws.run(
        "sheet", "range", "update",
        "--node", node_id,
        "--sheet-id", sheet_id,
        "--range", f"A2:D{1 + row_count}",
        "--values", values_json,
    )

    # 5. 写入一个公式单元格，供 --match-formula 测试
    dws.run(
        "sheet", "range", "update",
        "--node", node_id,
        "--sheet-id", sheet_id,
        "--range", "C7",
        "--values", '[["=SUM(C2:C5)"]]',
    )
    print(f"[SETUP] Seed data written: {row_count} rows + 1 formula cell")

    yield {
        "node_id": node_id,
        "sheet_id": sheet_id,
        "doc_name": doc_name,
    }

    # Teardown: sheet 无 delete 命令，仅打印提示便于手动清理
    print(f"\n[TEARDOWN] Test sheet doc: {node_id} ({doc_name})")
    print("[TEARDOWN] Sheet 无 delete 命令，如需清理请手动删除该文档")


@pytest.fixture(scope="session")
def sheet_node_id(test_sheet_info):
    """测试用表格文档 nodeId。"""
    return test_sheet_info["node_id"]

@pytest.fixture(scope="session")
def sheet_id(test_sheet_info):
    """测试用工作表 ID。"""
    return test_sheet_info["sheet_id"]

# ─── Replace 测试专用 fixture ──────────────────────────────────

@pytest.fixture(scope="function")
def replace_sheet(dws, test_sheet_info):
    """为每个 replace 测试用例在测试前重新写入干净种子数据。

    注意：replace_all 在服务端对新建工作表存在已知问题（replaceCount 始终为 0），
    因此必须复用 session 级别的默认工作表，而不是新建工作表。
    通过在每次测试前用 range update 覆盖写入种子数据来实现数据隔离。

    种子数据布局：
      A1:D1  姓名 | 部门 | 金额 | 状态
      A2:D2  张三 | 销售部 | 50000 | 完成
      A3:D3  李四 | 市场部 | 38000 | 待处理
      A4:D4  王五 | 销售部 | 62000 | 完成
      A5:D5  Test_User | 研发部 | 99000 | pending
    """
    node_id = test_sheet_info["node_id"]
    sheet_id = test_sheet_info["sheet_id"]

    # 先用空字符串清空足够大的区域，消除其他测试在各列留下的残留数据
    # replace_all 是全工作表替换，必须确保整个工作表干净
    # 注意：null 在服务端会被忽略，必须用 "" 才能清空单元格
    clear_rows = [[""] * 10 for _ in range(25)]
    dws.run(
        "sheet", "range", "update",
        "--node", node_id,
        "--sheet-id", sheet_id,
        "--range", "A1:J25",
        "--values", json.dumps(clear_rows, ensure_ascii=False),
    )

    # 再写入干净的种子数据
    all_rows = [SEED_HEADER] + SEED_ROWS
    dws.run(
        "sheet", "range", "update",
        "--node", node_id,
        "--sheet-id", sheet_id,
        "--range", f"A1:D{len(all_rows)}",
        "--values", json.dumps(all_rows, ensure_ascii=False),
    )

    yield {
        "node_id": node_id,
        "sheet_id": sheet_id,
    }


# ─── Filter View 自动清理 fixture ──────────────────────────────

@pytest.fixture(scope="function")
def filter_view_cleanup(dws, sheet_node_id, sheet_id):
    """每个测试用例结束后自动删除本用例创建的所有筛选视图。

    用法：测试中调用 `filter_view_cleanup(fv_id)` 注册需要清理的 filterViewId，
    fixture teardown 阶段会逐个调用 `filter-view delete` 清理。

    即使删除失败也不会让测试报错（best-effort 清理），仅打印警告。
    """
    created_filter_view_ids: list[str] = []

    def _register(fv_id: str) -> str:
        """注册一个 filterViewId，teardown 时自动删除。返回原 fv_id 方便链式调用。"""
        created_filter_view_ids.append(fv_id)
        return fv_id

    yield _register

    # Teardown：逆序删除，best-effort
    for fv_id in reversed(created_filter_view_ids):
        try:
            dws.run(
                "sheet", "filter-view", "delete",
                "--node", sheet_node_id,
                "--sheet-id", sheet_id,
                "--filter-view-id", fv_id,
                expect_success=False,
            )
        except Exception as exc:
            print(f"[CLEANUP] Failed to delete filter view {fv_id}: {exc}")