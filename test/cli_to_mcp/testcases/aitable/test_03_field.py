"""
test_03_field.py — 字段管理全覆盖测试 (4 commands)

Commands tested:
  11. dws aitable field get       (get_fields)
  12. dws aitable field create    (create_fields)
  13. dws aitable field update    (update_field)
  14. dws aitable field delete    (delete_field)

Setup: 在 test_base 中创建一个专用 table 供本文件测试。
"""

import json
import time

import pytest


@pytest.fixture(scope="module")
def field_test_table(dws, test_base_id):
    """Module-scoped: 创建一个用于 field 测试的 table。"""
    name = f"FieldTest_{int(time.time())}"
    fields = json.dumps([
        {"fieldName": "主字段", "type": "text"},
    ], ensure_ascii=False)
    data = dws.run(
        "aitable", "table", "create",
        "--base-id", test_base_id,
        "--name", name,
        "--fields", fields,
    )
    table_id = data["data"]["tableId"]
    yield table_id
    # 不删除，由 base 级别 teardown 负责清理


class TestFieldGet:
    """dws aitable field get"""

    def test_get_all_fields(self, dws, test_base_id, field_test_table):
        """获取所有字段，至少包含创建时的主字段。"""
        data = dws.run(
            "aitable", "field", "get",
            "--base-id", test_base_id,
            "--table-id", field_test_table,
        )
        fields = data["data"]["fields"]
        assert isinstance(fields, list)
        assert len(fields) >= 1
        # 验证字段结构
        f = fields[0]
        assert "fieldId" in f
        assert "fieldName" in f
        assert "type" in f

    def test_get_by_field_ids(self, dws, test_base_id, field_test_table):
        """指定 --field-ids 精确获取单个字段。"""
        all_data = dws.run(
            "aitable", "field", "get",
            "--base-id", test_base_id,
            "--table-id", field_test_table,
        )
        first_id = all_data["data"]["fields"][0]["fieldId"]

        data = dws.run(
            "aitable", "field", "get",
            "--base-id", test_base_id,
            "--table-id", field_test_table,
            "--field-ids", first_id,
        )
        fields = data["data"]["fields"]
        assert len(fields) == 1
        assert fields[0]["fieldId"] == first_id


class TestFieldCreate:
    """dws aitable field create"""

    def test_create_text_field(self, dws, test_base_id, field_test_table):
        """创建普通 text 字段。"""
        fields = json.dumps([
            {"fieldName": f"文本_{int(time.time())}", "type": "text"},
        ], ensure_ascii=False)
        data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", field_test_table,
            "--fields", fields,
        )
        results = data["data"]["results"]
        assert len(results) == 1

    def test_create_multiple_types(self, dws, test_base_id, field_test_table):
        """批量创建 number / singleSelect / date / checkbox 字段。"""
        ts = int(time.time())
        fields = json.dumps([
            {"fieldName": f"数字_{ts}", "type": "number",
             "config": {"formatter": "FLOAT_2"}},
            {"fieldName": f"单选_{ts}", "type": "singleSelect",
             "config": {"options": [
                 {"name": "待办"}, {"name": "进行中"}, {"name": "已完成"}
             ]}},
            {"fieldName": f"日期_{ts}", "type": "date",
             "config": {"formatter": "YYYY-MM-DD"}},
            {"fieldName": f"勾选_{ts}", "type": "checkbox"},
        ], ensure_ascii=False)

        data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", field_test_table,
            "--fields", fields,
        )
        results = data["data"]["results"]
        assert len(results) == 4
        # 统计成功数
        success_count = sum(
            1 for r in results if r.get("success", True)
        )
        assert success_count >= 3, (
            f"Expected at least 3 successes, got {success_count}"
        )

    def test_create_currency_field(self, dws, test_base_id, field_test_table):
        """创建 currency 类型字段。"""
        fields = json.dumps([
            {"fieldName": f"金额_{int(time.time())}", "type": "currency",
             "config": {"currencyType": "CNY", "formatter": "FLOAT_2"}},
        ], ensure_ascii=False)
        data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", field_test_table,
            "--fields", fields,
        )
        assert len(data["data"]["results"]) == 1

    def test_create_progress_field(self, dws, test_base_id, field_test_table):
        """创建 progress 类型字段。"""
        fields = json.dumps([
            {"fieldName": f"进度_{int(time.time())}", "type": "progress",
             "config": {"formatter": "PERCENT"}},
        ], ensure_ascii=False)
        data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", field_test_table,
            "--fields", fields,
        )
        assert len(data["data"]["results"]) == 1


class TestFieldUpdate:
    """dws aitable field update"""

    def test_update_field_name(self, dws, test_base_id, field_test_table):
        """重命名字段，验证生效。"""
        # 先创建一个专用字段
        old_name = f"待改名_{int(time.time())}"
        create_fields = json.dumps([
            {"fieldName": old_name, "type": "text"},
        ], ensure_ascii=False)
        create_data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", field_test_table,
            "--fields", create_fields,
        )
        field_id = create_data["data"]["results"][0]["fieldId"]

        # 重命名
        new_name = f"已改名_{int(time.time())}"
        dws.run(
            "aitable", "field", "update",
            "--base-id", test_base_id,
            "--table-id", field_test_table,
            "--field-id", field_id,
            "--name", new_name,
        )

        # 验证
        verify = dws.run(
            "aitable", "field", "get",
            "--base-id", test_base_id,
            "--table-id", field_test_table,
            "--field-ids", field_id,
        )
        assert verify["data"]["fields"][0]["fieldName"] == new_name


class TestFieldDelete:
    """dws aitable field delete"""

    def test_delete_field(self, dws, test_base_id, field_test_table):
        """创建字段后删除，验证不再存在。"""
        # 创建
        name = f"待删_{int(time.time())}"
        create_fields = json.dumps([
            {"fieldName": name, "type": "text"},
        ], ensure_ascii=False)
        create_data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", field_test_table,
            "--fields", create_fields,
        )
        field_id = create_data["data"]["results"][0]["fieldId"]

        # 删除
        dws.run(
            "aitable", "field", "delete",
            "--base-id", test_base_id,
            "--table-id", field_test_table,
            "--field-id", field_id,
            "--yes",
        )

        # 验证：不再出现
        all_fields = dws.run(
            "aitable", "field", "get",
            "--base-id", test_base_id,
            "--table-id", field_test_table,
        )
        remaining_ids = [
            f["fieldId"] for f in all_fields["data"]["fields"]
        ]
        assert field_id not in remaining_ids
