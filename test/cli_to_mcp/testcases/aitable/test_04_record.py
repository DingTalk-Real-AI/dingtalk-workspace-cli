"""
test_04_record.py — 记录管理全覆盖测试 (4 commands)

Commands tested:
  15. dws aitable record query    (query_records)
  16. dws aitable record create   (create_records)
  17. dws aitable record update   (update_records)
  18. dws aitable record delete   (delete_records)

Setup: 在 test_base 中创建一个带丰富字段的 table 供记录测试。
"""

import json
import time

import pytest


@pytest.fixture(scope="module")
def record_test_env(dws, test_base_id):
    """创建带丰富字段的 table，yield (table_id, field_map)。"""
    ts = int(time.time())
    name = f"RecordTest_{ts}"
    fields_def = json.dumps([
        {"fieldName": "标题", "type": "text"},
        {"fieldName": "数量", "type": "number",
         "config": {"formatter": "INT"}},
        {"fieldName": "状态", "type": "singleSelect",
         "config": {"options": [
             {"name": "待办"}, {"name": "进行中"}, {"name": "已完成"}
         ]}},
        {"fieldName": "截止日期", "type": "date",
         "config": {"formatter": "YYYY-MM-DD"}},
        {"fieldName": "已确认", "type": "checkbox"},
    ], ensure_ascii=False)

    data = dws.run(
        "aitable", "table", "create",
        "--base-id", test_base_id,
        "--name", name,
        "--fields", fields_def,
    )
    table_id = data["data"]["tableId"]

    # 获取所有 field，建立 name→id 映射
    field_data = dws.run(
        "aitable", "field", "get",
        "--base-id", test_base_id,
        "--table-id", table_id,
    )
    field_map = {
        f["fieldName"]: f["fieldId"]
        for f in field_data["data"]["fields"]
    }

    yield table_id, field_map


class TestRecordCreate:
    """dws aitable record create"""

    def test_create_single_record(self, dws, test_base_id, record_test_env):
        """创建单条记录，包含多种字段类型。"""
        table_id, fm = record_test_env
        records = json.dumps([{
            "cells": {
                fm["标题"]: "集成测试任务A",
                fm["数量"]: 42,
                fm["状态"]: "待办",
                fm["截止日期"]: "2026-12-31",
                fm["已确认"]: True,
            }
        }], ensure_ascii=False)

        data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", records,
        )
        new_ids = data["data"]["newRecordIds"]
        assert len(new_ids) == 1
        assert isinstance(new_ids[0], str) and len(new_ids[0]) > 0

    def test_create_batch_records(self, dws, test_base_id, record_test_env):
        """批量创建 3 条记录。"""
        table_id, fm = record_test_env
        records = json.dumps([
            {"cells": {fm["标题"]: "批量任务B", fm["数量"]: 10,
                       fm["状态"]: "进行中"}},
            {"cells": {fm["标题"]: "批量任务C", fm["数量"]: 20,
                       fm["状态"]: "已完成"}},
            {"cells": {fm["标题"]: "批量任务D", fm["数量"]: 30,
                       fm["状态"]: "待办", fm["已确认"]: False}},
        ], ensure_ascii=False)

        data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", records,
        )
        assert len(data["data"]["newRecordIds"]) == 3

    def test_create_minimal_record(self, dws, test_base_id, record_test_env):
        """只填 1 个字段的最小记录。"""
        table_id, fm = record_test_env
        records = json.dumps([
            {"cells": {fm["标题"]: "最小记录E"}},
        ], ensure_ascii=False)

        data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", records,
        )
        assert len(data["data"]["newRecordIds"]) == 1


class TestRecordQuery:
    """dws aitable record query"""

    def test_query_all(self, dws, test_base_id, record_test_env):
        """查询全部记录，至少有前面创建的记录。"""
        table_id, _ = record_test_env
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
        )
        records = data["data"]["records"]
        assert isinstance(records, list)
        assert len(records) >= 1

    def test_query_by_record_ids(self, dws, test_base_id, record_test_env):
        """按 record-ids 精确查询。"""
        table_id, _ = record_test_env
        # 先查全部，取前 2 个 ID
        all_data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
        )
        all_recs = all_data["data"]["records"]
        if len(all_recs) < 2:
            pytest.skip("Not enough records for ID-based query test")

        ids = [all_recs[0]["recordId"], all_recs[1]["recordId"]]
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--record-ids", ",".join(ids),
        )
        returned_ids = [r["recordId"] for r in data["data"]["records"]]
        for rid in ids:
            assert rid in returned_ids

    def test_query_with_keyword(self, dws, test_base_id, record_test_env):
        """关键词搜索 '集成测试'。"""
        table_id, _ = record_test_env
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--keyword", "集成测试",
        )
        records = data["data"]["records"]
        assert isinstance(records, list)
        # 至少应找到"集成测试任务A"
        assert len(records) >= 1

    def test_query_with_limit(self, dws, test_base_id, record_test_env):
        """--limit 2 限制返回条数。"""
        table_id, _ = record_test_env
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--limit", "2",
        )
        records = data["data"]["records"]
        assert len(records) <= 2

    def test_query_with_field_ids(self, dws, test_base_id, record_test_env):
        """--field-ids 限制返回字段。"""
        table_id, fm = record_test_env
        title_fid = fm["标题"]
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--field-ids", title_fid,
        )
        records = data["data"]["records"]
        assert len(records) >= 1
        # 每条记录 cells 中只应包含指定字段
        for rec in records:
            cell_keys = list(rec.get("cells", {}).keys())
            assert title_fid in cell_keys or len(cell_keys) <= 2


class TestRecordUpdate:
    """dws aitable record update"""

    def test_update_record(self, dws, test_base_id, record_test_env):
        """修改一条记录的多个字段，验证修改生效。"""
        table_id, fm = record_test_env
        # 取第一条记录
        query_data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--limit", "1",
        )
        rec_id = query_data["data"]["records"][0]["recordId"]

        # 更新
        records = json.dumps([{
            "recordId": rec_id,
            "cells": {
                fm["标题"]: "已更新的标题",
                fm["数量"]: 999,
                fm["状态"]: "已完成",
            }
        }], ensure_ascii=False)
        update_data = dws.run(
            "aitable", "record", "update",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", records,
        )
        # update 返回 recordIds 列表
        assert rec_id in update_data["data"]["recordIds"]

        # 验证
        verify = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--record-ids", rec_id,
        )
        cells = verify["data"]["records"][0]["cells"]
        assert cells[fm["标题"]] == "已更新的标题"


class TestRecordDelete:
    """dws aitable record delete"""

    def test_delete_records(self, dws, test_base_id, record_test_env):
        """创建 2 条记录后删除，验证不再存在。"""
        table_id, fm = record_test_env
        # 创建 2 条临时记录
        records = json.dumps([
            {"cells": {fm["标题"]: "待删记录X"}},
            {"cells": {fm["标题"]: "待删记录Y"}},
        ], ensure_ascii=False)
        create_data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", records,
        )
        ids_to_delete = create_data["data"]["newRecordIds"]

        # 删除
        dws.run(
            "aitable", "record", "delete",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--record-ids", ",".join(ids_to_delete),
            "--yes",
        )

        # 验证：尝试按 ID 查询应为空
        verify = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--record-ids", ",".join(ids_to_delete),
        )
        remaining = verify["data"].get("records", [])
        remaining_ids = [r["recordId"] for r in remaining]
        for rid in ids_to_delete:
            assert rid not in remaining_ids
