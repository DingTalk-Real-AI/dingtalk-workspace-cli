"""record query --all 自动翻页集成测试。

验证场景：
1. 创建超过 100 条记录的表
2. --all 自动翻页获取全部记录
3. --all --page-limit 1 验证截断 + hasMore + cursor
4. --cursor 从断点续拉
"""

import json

import pytest
from test_utils import unique_name


@pytest.fixture(scope="module")
def pagination_base(dws):
    """Module-scoped base for pagination tests."""
    name = unique_name("Pagination_Test")
    data = dws.run("aitable", "base", "create", "--name", name)
    base_id = data["data"]["baseId"]
    yield base_id
    try:
        dws.run("aitable", "base", "delete", "--base-id", base_id, "--yes")
    except Exception:
        pass


@pytest.fixture(scope="module")
def pagination_table(dws, pagination_base):
    """Create a table with 150 records for pagination testing."""
    # Create table with a simple text field
    fields = json.dumps([
        {"fieldName": "序号", "type": "number"},
        {"fieldName": "内容", "type": "text"},
    ])
    data = dws.run(
        "aitable", "table", "create",
        "--base-id", pagination_base,
        "--name", "翻页测试表",
        "--fields", fields,
    )
    table_id = data["data"]["tableId"]

    # Get field IDs via table get
    import time
    time.sleep(1)  # wait for table to be ready
    table_data = dws.run(
        "aitable", "table", "get",
        "--base-id", pagination_base,
        "--table-ids", table_id,
    )
    field_map = {}
    for f in table_data["data"]["tables"][0]["fields"]:
        field_map[f["fieldName"]] = f["fieldId"]

    num_field = field_map["序号"]
    text_field = field_map["内容"]

    # Insert 150 records in 3 batches (50 + 50 + 50) to avoid API instability
    import time

    def make_batch(start, count):
        return json.dumps([
            {"cells": {num_field: i, text_field: f"record_{i}"}}
            for i in range(start, start + count)
        ])

    for batch_start in [1, 51, 101]:
        for attempt in range(3):
            try:
                dws.run(
                    "aitable", "record", "create",
                    "--base-id", pagination_base,
                    "--table-id", table_id,
                    "--records", make_batch(batch_start, 50),
                )
                break
            except Exception:
                if attempt == 2:
                    raise
                time.sleep(2)

    yield {"base_id": pagination_base, "table_id": table_id}


class TestRecordPagination:
    """record query --all 自动翻页测试。"""

    def test_all_fetches_all_records(self, dws, pagination_table):
        """--all should return all 150 records across multiple pages."""
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", pagination_table["base_id"],
            "--table-id", pagination_table["table_id"],
            "--all",
        )
        records = data["data"]["records"]
        total = data["data"]["totalCount"]
        has_more = data["data"]["hasMore"]

        assert total >= 150, f"expected >= 150 records, got {total}"
        assert len(records) >= 150, f"expected >= 150 records in array, got {len(records)}"
        assert has_more is False, "all data fetched, hasMore should be False"

    def test_page_limit_truncates(self, dws, pagination_table):
        """--all --page-limit 1 should return only first page (100 records)."""
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", pagination_table["base_id"],
            "--table-id", pagination_table["table_id"],
            "--all",
            "--page-limit", "1",
        )
        records = data["data"]["records"]
        has_more = data["data"]["hasMore"]
        cursor = data["data"].get("cursor", "")

        assert len(records) == 100, f"expected 100 records (1 page), got {len(records)}"
        assert has_more is True, "truncated, hasMore should be True"
        assert cursor != "", "truncated result should contain cursor for resume"

    def test_resume_from_cursor(self, dws, pagination_table):
        """After truncation, --cursor should resume from where we left off."""
        # First: get page 1 with cursor
        data1 = dws.run(
            "aitable", "record", "query",
            "--base-id", pagination_table["base_id"],
            "--table-id", pagination_table["table_id"],
            "--all",
            "--page-limit", "1",
        )
        cursor = data1["data"]["cursor"]
        page1_count = len(data1["data"]["records"])

        # Resume from cursor
        data2 = dws.run(
            "aitable", "record", "query",
            "--base-id", pagination_table["base_id"],
            "--table-id", pagination_table["table_id"],
            "--all",
            "--cursor", cursor,
        )
        page2_records = data2["data"]["records"]

        # Total should be 150
        total = page1_count + len(page2_records)
        assert total >= 150, f"page1({page1_count}) + page2({len(page2_records)}) = {total}, expected >= 150"

    def test_without_all_returns_single_page(self, dws, pagination_table):
        """Without --all, should return at most 100 records (single page)."""
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", pagination_table["base_id"],
            "--table-id", pagination_table["table_id"],
        )
        records = data["data"]["records"]
        assert len(records) <= 100, f"without --all, should return <= 100, got {len(records)}"
