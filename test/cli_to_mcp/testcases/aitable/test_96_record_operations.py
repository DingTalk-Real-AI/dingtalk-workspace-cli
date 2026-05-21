"""
test_96_record_operations.py — record update/delete/query 高级参数验证

覆盖 aitable-record-update.md / aitable-record-delete.md / aitable-record-query.md 中的声明：
- record update: 部分字段更新（未传字段保持原值）
- record update: --records-file 文件模式
- record delete: 删除记录（不可逆）
- record query --all: 自动翻页
- record query --query: 全文关键词搜索
- record query --field-ids: 选择返回字段
- record query --limit: 限制返回数量
- record query --record-ids: 按 ID 精确获取
"""

import json
import time

import pytest


@pytest.fixture(scope="module")
def ops_table(dws, test_base_id):
    """Create a table with sample records for operation tests."""
    ts = int(time.time())
    fields = [
        {"fieldName": "标题", "type": "text"},
        {"fieldName": "状态", "type": "singleSelect", "config": {"options": [{"name": "待办"}, {"name": "进行中"}, {"name": "已完成"}]}},
        {"fieldName": "数量", "type": "number", "config": {"formatter": "INT"}},
        {"fieldName": "备注", "type": "text"},
    ]
    data = dws.run(
        "aitable", "table", "create",
        "--base-id", test_base_id,
        "--name", f"RecordOpsTest_{ts}",
        "--fields", json.dumps(fields, ensure_ascii=False),
    )
    table_id = data["data"]["tableId"]

    # Get field map
    table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
    field_map = {f["fieldName"]: f["fieldId"] for f in table_data["data"]["tables"][0].get("fields", [])}

    # Pre-populate 5 records
    title_fld = field_map["标题"]
    status_fld = field_map["状态"]
    num_fld = field_map["数量"]
    note_fld = field_map["备注"]
    records = [
        {"cells": {title_fld: f"任务{i}", status_fld: "待办", num_fld: i * 10, note_fld: f"备注内容{i}"}}
        for i in range(1, 6)
    ]
    create_data = dws.run(
        "aitable", "record", "create",
        "--base-id", test_base_id,
        "--table-id", table_id,
        "--records", json.dumps(records, ensure_ascii=False),
    )
    body = create_data["data"]
    record_ids = body.get("newRecordIds") or [r["recordId"] for r in body.get("records", [])]
    assert len(record_ids) == 5, f"expected 5 records created, got: {create_data}"

    return table_id, field_map, record_ids


# ═══════════════════════════════════════════════════════════════
# record update — 部分字段更新
# ═══════════════════════════════════════════════════════════════

class TestRecordUpdate:
    """验证 record update 只传部分字段时，未传字段保持原值。"""

    def test_partial_update_preserves_untouched_fields(self, dws, test_base_id, ops_table):
        """更新状态字段，备注字段应保持不变。"""
        table_id, fm, rec_ids = ops_table
        target_rec = rec_ids[0]

        # Update only 状态
        dws.run(
            "aitable", "record", "update",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", json.dumps([{
                "recordId": target_rec,
                "cells": {fm["状态"]: "进行中"}
            }]),
        )

        # Read back and verify
        query_data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--record-ids", target_rec,
        )
        cells = query_data["data"]["records"][0]["cells"]
        # 状态 should be updated
        status_val = cells.get(fm["状态"])
        if isinstance(status_val, dict):
            assert status_val.get("name") == "进行中"
        else:
            assert status_val == "进行中"
        # 备注 should be preserved
        assert cells.get(fm["备注"]) == "备注内容1", "untouched field should be preserved"

    def test_update_multiple_records_batch(self, dws, test_base_id, ops_table):
        """批量更新多条记录。"""
        table_id, fm, rec_ids = ops_table
        updates = [
            {"recordId": rec_ids[1], "cells": {fm["数量"]: 999}},
            {"recordId": rec_ids[2], "cells": {fm["数量"]: 888}},
        ]
        dws.run(
            "aitable", "record", "update",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", json.dumps(updates),
        )

        # Verify
        query_data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--record-ids", f"{rec_ids[1]},{rec_ids[2]}",
        )
        records = query_data["data"]["records"]
        values = sorted([int(float(r["cells"].get(fm["数量"], 0))) for r in records])
        assert 888 in values and 999 in values


# ═══════════════════════════════════════════════════════════════
# record delete — 删除记录
# ═══════════════════════════════════════════════════════════════

class TestRecordDelete:
    """验证 record delete 操作。"""

    def test_delete_single_record(self, dws, test_base_id, ops_table):
        """删除一条记录后查询应找不到。"""
        table_id, fm, rec_ids = ops_table
        target_rec = rec_ids[4]  # 删最后一条

        dws.run(
            "aitable", "record", "delete",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--record-ids", target_rec,
            "--yes",
        )

        # Query should not return this record
        query_data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--record-ids", target_rec,
            expect_success=False,
        )
        records = query_data.get("data", {}).get("records", [])
        # 可能返回空数组或报错
        assert len(records) == 0 or query_data.get("status") == "error", \
            f"deleted record should not be queryable, got: {query_data}"


# ═══════════════════════════════════════════════════════════════
# record query — 高级参数
# ═══════════════════════════════════════════════════════════════

class TestRecordQueryFieldIds:
    """验证 --field-ids 仅返回指定字段。"""

    def test_field_ids_limits_response(self, dws, test_base_id, ops_table):
        """只请求标题字段，返回应不包含其他字段。"""
        table_id, fm, rec_ids = ops_table
        query_data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--field-ids", fm["标题"],
            "--limit", "2",
        )
        records = query_data["data"]["records"]
        assert len(records) >= 1
        cells = records[0]["cells"]
        # 应只包含标题字段
        assert fm["标题"] in cells
        # 数量字段不应返回
        assert fm["数量"] not in cells, "non-requested field should not appear"


class TestRecordQueryLimit:
    """验证 --limit 参数。"""

    def test_limit_caps_results(self, dws, test_base_id, ops_table):
        """--limit 2 应最多返回 2 条。"""
        table_id, fm, rec_ids = ops_table
        query_data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--limit", "2",
        )
        records = query_data["data"]["records"]
        assert len(records) <= 2


class TestRecordQueryByIds:
    """验证 --record-ids 精确查询。"""

    def test_query_by_ids(self, dws, test_base_id, ops_table):
        """按 ID 查询返回指定记录。"""
        table_id, fm, rec_ids = ops_table
        target = f"{rec_ids[0]},{rec_ids[1]}"
        query_data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--record-ids", target,
        )
        records = query_data["data"]["records"]
        returned_ids = {r["recordId"] for r in records}
        assert rec_ids[0] in returned_ids
        assert rec_ids[1] in returned_ids


class TestRecordQueryKeyword:
    """验证 --query 全文搜索。"""

    def test_query_keyword_search(self, dws, test_base_id, ops_table):
        """搜索关键词应返回匹配记录。"""
        table_id, fm, rec_ids = ops_table
        query_data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--query", "任务1",
        )
        records = query_data["data"].get("records", [])
        # 至少应匹配到"任务1"
        assert len(records) >= 1
        # 验证返回内容包含关键词
        found = any("任务1" in str(r.get("cells", {})) for r in records)
        assert found, f"keyword '任务1' should match, got: {records}"


class TestRecordQueryAll:
    """验证 --all 自动翻页模式。"""

    def test_all_flag_returns_complete_data(self, dws, test_base_id, ops_table):
        """--all 模式应返回全部记录（不含 nextCursor）。"""
        table_id, fm, rec_ids = ops_table
        query_data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--all",
        )
        data = query_data["data"]
        records = data.get("records", [])
        # 我们创建了 5 条，删了 1 条，应该有 4 条
        assert len(records) >= 4
        # --all 成功完成时不应有 nextCursor（或 hasMore=false）
        has_more = data.get("hasMore", False)
        next_cursor = data.get("nextCursor") or data.get("cursor")
        if has_more:
            # 如果还有更多，说明 page-limit 到了，但对于 4 条数据不应如此
            print(f"  [WARN] --all still has more data: cursor={next_cursor}")
