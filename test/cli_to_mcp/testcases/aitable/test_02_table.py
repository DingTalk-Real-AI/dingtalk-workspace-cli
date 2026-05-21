"""
test_02_table.py — 数据表管理全覆盖测试 (4 commands)

Commands tested:
  7. dws aitable table get       (get_tables)
  8. dws aitable table create    (create_table)
  9. dws aitable table update    (update_table)
 10. dws aitable table delete    (delete_table)

Depends on: test_base_id (from conftest.py)
"""

import json
import time

import pytest


# ─── Helper: 初始字段定义 ────────────────────────────────────

BASIC_FIELDS = json.dumps([
    {"fieldName": "任务名称", "type": "text"},
    {"fieldName": "优先级", "type": "singleSelect", "config": {
        "options": [{"name": "高"}, {"name": "中"}, {"name": "低"}]
    }},
    {"fieldName": "截止日期", "type": "date", "config": {
        "formatter": "YYYY-MM-DD"
    }},
    {"fieldName": "预算", "type": "number", "config": {
        "formatter": "FLOAT_2"
    }},
    {"fieldName": "已完成", "type": "checkbox"},
], ensure_ascii=False)


@pytest.fixture(scope="module")
def seed_table_id(dws, test_base_id):
    """确保 base 中至少有一个 table 供 get 测试使用（带重试应对网络抖动）。"""
    fields = json.dumps([{"fieldName": "seed", "type": "text"}],
                        ensure_ascii=False)
    last_err = None
    for _attempt in range(3):
        try:
            data = dws.run(
                "aitable", "table", "create",
                "--base-id", test_base_id,
                "--name", f"SeedTable_{int(time.time())}",
                "--fields", fields,
            )
            return data["data"]["tableId"]
        except Exception as e:
            last_err = e
            time.sleep(2)
    pytest.fail(f"seed_table_id fixture failed after 3 retries: {last_err}")


class TestTableGet:
    """dws aitable table get"""

    def test_get_all_tables(self, dws, test_base_id, seed_table_id):
        """不传 table-ids，返回 base 下所有 table。"""
        data = dws.run(
            "aitable", "table", "get", "--base-id", test_base_id
        )
        tables = data["data"]["tables"]
        assert isinstance(tables, list)
        assert len(tables) >= 1
        # 不传 table-ids 时返回精简结构，至少有 tableId 和 tableName
        tbl = tables[0]
        assert "tableId" in tbl
        assert "tableName" in tbl

    def test_get_by_specific_ids(self, dws, test_base_id, seed_table_id):
        """传入 --table-ids 精确获取指定 table。"""
        data = dws.run(
            "aitable", "table", "get",
            "--base-id", test_base_id,
            "--table-ids", seed_table_id,
        )
        tables = data["data"]["tables"]
        assert len(tables) == 1
        assert tables[0]["tableId"] == seed_table_id


class TestTableCreate:
    """dws aitable table create"""

    def test_create_with_multiple_field_types(self, dws, test_base_id):
        """创建包含 5 种字段类型的表，验证返回结构。"""
        name = f"测试表_{int(time.time())}"
        data = dws.run(
            "aitable", "table", "create",
            "--base-id", test_base_id,
            "--name", name,
            "--fields", BASIC_FIELDS,
        )
        tbl = data["data"]
        assert "tableId" in tbl, "create_table must return tableId"
        assert tbl.get("tableName") == name

        # 验证字段确实被创建
        verify = dws.run(
            "aitable", "table", "get",
            "--base-id", test_base_id,
            "--table-ids", tbl["tableId"],
        )
        fields = verify["data"]["tables"][0]["fields"]
        field_names = [f["fieldName"] for f in fields]
        for expected in ["任务名称", "优先级", "截止日期", "预算", "已完成"]:
            assert expected in field_names, (
                f"Field '{expected}' not found in {field_names}"
            )

    def test_create_minimal_table(self, dws, test_base_id):
        """只含 1 个 text 字段的最小表。"""
        name = f"最小表_{int(time.time())}"
        fields = json.dumps([{"fieldName": "标题", "type": "text"}],
                            ensure_ascii=False)
        data = dws.run(
            "aitable", "table", "create",
            "--base-id", test_base_id,
            "--name", name,
            "--fields", fields,
        )
        assert data["data"]["tableId"]


class TestTableUpdate:
    """dws aitable table update"""

    def test_rename_table(self, dws, test_base_id):
        """重命名表，验证新名称生效。"""
        # 创建临时表
        name = f"待改名表_{int(time.time())}"
        fields = json.dumps([{"fieldName": "col1", "type": "text"}],
                            ensure_ascii=False)
        create = dws.run(
            "aitable", "table", "create",
            "--base-id", test_base_id,
            "--name", name,
            "--fields", fields,
        )
        table_id = create["data"]["tableId"]

        # 重命名
        new_name = f"已改名表_{int(time.time())}"
        dws.run(
            "aitable", "table", "update",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--name", new_name,
        )

        # 验证
        verify = dws.run(
            "aitable", "table", "get",
            "--base-id", test_base_id,
            "--table-ids", table_id,
        )
        assert verify["data"]["tables"][0]["tableName"] == new_name


class TestTableDelete:
    """dws aitable table delete"""

    def test_delete_table(self, dws, test_base_id):
        """创建临时表并删除，验证删除后不可获取。"""
        # 创建
        name = f"待删表_{int(time.time())}"
        fields = json.dumps([{"fieldName": "x", "type": "text"}],
                            ensure_ascii=False)
        create = dws.run(
            "aitable", "table", "create",
            "--base-id", test_base_id,
            "--name", name,
            "--fields", fields,
        )
        table_id = create["data"]["tableId"]

        # 删除
        dws.run(
            "aitable", "table", "delete",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--yes",
        )

        # 验证：该 table 不应出现在列表中
        all_tables = dws.run(
            "aitable", "table", "get", "--base-id", test_base_id
        )
        remaining_ids = [
            t["tableId"] for t in all_tables["data"]["tables"]
        ]
        assert table_id not in remaining_ids, (
            "Deleted table should not appear in table list"
        )
