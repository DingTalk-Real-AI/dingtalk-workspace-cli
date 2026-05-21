"""
test_97_advanced_fields.py — 高级字段操作验证

覆盖 aitable-formula-guide.md / aitable-field.md / aitable-best-practices.md 中的声明：
- lookup 字段创建（关联引用）
- filterUp 字段创建（查找引用）
- formula 字段更新
- AI 字段创建（--ai-config + fieldRef prompt）
- field delete 操作
- field update rename
"""

import json
import time

import pytest


@pytest.fixture(scope="module")
def adv_tables(dws, test_base_id):
    """Create two tables for advanced field tests: main + target."""
    ts = int(time.time())

    # Target table (for link/lookup/filterUp)
    target_fields = [
        {"fieldName": "项目名", "type": "text"},
        {"fieldName": "预算", "type": "number", "config": {"formatter": "FLOAT_2"}},
        {"fieldName": "状态", "type": "singleSelect", "config": {"options": [{"name": "进行中"}, {"name": "已完成"}]}},
    ]
    target_data = dws.run(
        "aitable", "table", "create",
        "--base-id", test_base_id,
        "--name", f"AdvTarget_{ts}",
        "--fields", json.dumps(target_fields, ensure_ascii=False),
    )
    target_table_id = target_data["data"]["tableId"]

    # Get target field map
    target_info = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", target_table_id)
    target_fm = {f["fieldName"]: f["fieldId"] for f in target_info["data"]["tables"][0].get("fields", [])}

    # Insert some records in target table
    records = [
        {"cells": {target_fm["项目名"]: "Alpha", target_fm["预算"]: 10000, target_fm["状态"]: "进行中"}},
        {"cells": {target_fm["项目名"]: "Beta", target_fm["预算"]: 20000, target_fm["状态"]: "已完成"}},
    ]
    dws.run(
        "aitable", "record", "create",
        "--base-id", test_base_id,
        "--table-id", target_table_id,
        "--records", json.dumps(records, ensure_ascii=False),
    )

    # Main table
    main_fields = [
        {"fieldName": "任务名", "type": "text"},
        {"fieldName": "单价", "type": "number", "config": {"formatter": "FLOAT_2"}},
        {"fieldName": "数量", "type": "number", "config": {"formatter": "INT"}},
    ]
    main_data = dws.run(
        "aitable", "table", "create",
        "--base-id", test_base_id,
        "--name", f"AdvMain_{ts}",
        "--fields", json.dumps(main_fields, ensure_ascii=False),
    )
    main_table_id = main_data["data"]["tableId"]

    # Get main field map
    main_info = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", main_table_id)
    main_fm = {f["fieldName"]: f["fieldId"] for f in main_info["data"]["tables"][0].get("fields", [])}

    return {
        "main_table_id": main_table_id,
        "main_fm": main_fm,
        "target_table_id": target_table_id,
        "target_fm": target_fm,
    }


# ═══════════════════════════════════════════════════════════════
# formula update
# ═══════════════════════════════════════════════════════════════

class TestFormulaUpdate:
    """验证 formula 字段创建后可以更新公式。"""

    @pytest.mark.xfail(reason="API 已知限制：formula 字段创建后短时间内 field update 返回 ResourceNotFound，服务端索引延迟较长")
    def test_formula_create_and_update(self, dws, test_base_id, adv_tables):
        """创建 formula 字段后，用 field update 修改公式表达式。"""
        main_id = adv_tables["main_table_id"]

        # Create formula field
        create_data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", main_id,
            "--name", "总价",
            "--type", "formula",
            "--config", json.dumps({"formula": "[单价] * [数量]"}),
        )
        # Extract field id
        body = create_data.get("data", {})
        field_id = body.get("fieldId") or (body.get("results", [{}])[0].get("fieldId") if body.get("results") else None)
        assert field_id, f"formula field should be created, got: {create_data}"

        # 等待服务端索引生效（formula 字段创建后异步注册）
        time.sleep(3)

        # Update formula（带重试，服务端可能有短暂延迟）
        for attempt in range(3):
            update_data = dws.run(
                "aitable", "field", "update",
                "--base-id", test_base_id,
                "--table-id", main_id,
                "--field-id", field_id,
                "--config", json.dumps({"formula": "[单价] * [数量] * 1.1"}),
                expect_success=False,
            )
            err = update_data.get("error", {})
            if not err or err == {} or update_data.get("status") != "error":
                break
            if "NotFound" in str(err) and attempt < 2:
                time.sleep(2)
                continue
            pytest.fail(f"formula field update failed after retries: {update_data}")
        print(f"  [OK] formula field {field_id} updated successfully")


# ═══════════════════════════════════════════════════════════════
# lookup 字段
# ═══════════════════════════════════════════════════════════════

class TestLookupField:
    """验证 lookup（关联引用）字段创建。"""

    def test_lookup_field_creation(self, dws, test_base_id, adv_tables):
        """创建双向关联字段 → 再基于该关联创建 lookup 字段。"""
        main_id = adv_tables["main_table_id"]
        target_id = adv_tables["target_table_id"]
        target_fm = adv_tables["target_fm"]

        # Step 1: Create bidirectionalLink field
        link_data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", main_id,
            "--name", "关联项目",
            "--type", "bidirectionalLink",
            "--config", json.dumps({"linkedTableId": target_id, "multiple": True}),
        )
        link_body = link_data.get("data", {})
        link_field_id = link_body.get("fieldId") or (link_body.get("results", [{}])[0].get("fieldId") if link_body.get("results") else None)
        assert link_field_id, f"bidirectionalLink field should be created, got: {link_data}"

        # Step 2: Create lookup field referencing the link
        lookup_data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", main_id,
            "--name", "项目预算汇总",
            "--type", "lookup",
            "--config", json.dumps({
                "associateField": link_field_id,
                "valuesField": target_fm["预算"],
                "aggregator": "SUM",
            }),
        )
        lookup_body = lookup_data.get("data", {})
        lookup_fid = lookup_body.get("fieldId") or (lookup_body.get("results", [{}])[0].get("fieldId") if lookup_body.get("results") else None)
        assert lookup_fid, f"lookup field should be created, got: {lookup_data}"
        print(f"  [OK] lookup field created: {lookup_fid}")


# ═══════════════════════════════════════════════════════════════
# filterUp 字段
# ═══════════════════════════════════════════════════════════════

class TestFilterUpField:
    """验证 filterUp（查找引用）字段创建。"""

    def test_filterup_field_creation(self, dws, test_base_id, adv_tables):
        """创建 filterUp 字段，直接从目标表按条件查找取值。"""
        main_id = adv_tables["main_table_id"]
        target_id = adv_tables["target_table_id"]
        target_fm = adv_tables["target_fm"]

        config = {
            "targetSheet": target_id,
            "filters": [
                {
                    "fieldId": target_fm["状态"],
                    "operator": "equal",
                    "value": "进行中",
                    "link": "AND",
                }
            ],
            "valuesField": target_fm["预算"],
            "aggregator": "SUM",
        }
        data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", main_id,
            "--name", "进行中项目预算",
            "--type", "filterUp",
            "--config", json.dumps(config, ensure_ascii=False),
        )
        body = data.get("data", {})
        fid = body.get("fieldId") or (body.get("results", [{}])[0].get("fieldId") if body.get("results") else None)
        assert fid, f"filterUp field should be created, got: {data}"
        print(f"  [OK] filterUp field created: {fid}")


# ═══════════════════════════════════════════════════════════════
# AI 字段（--ai-config）
# ═══════════════════════════════════════════════════════════════

class TestAIField:
    """验证 AI 字段创建（prompt 必须含 fieldRef）。"""

    def test_ai_field_with_field_ref(self, dws, test_base_id, adv_tables):
        """创建包含 fieldRef 的 AI 字段。"""
        main_id = adv_tables["main_table_id"]
        main_fm = adv_tables["main_fm"]

        ai_config = {
            "outputType": "text",
            "prompt": [
                {"type": "text", "value": "请用一句话描述以下任务："},
                {"type": "fieldRef", "fieldId": main_fm["任务名"]},
            ],
            "autoRecompute": False,
            "enableWebSearch": False,
        }
        data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", main_id,
            "--name", "AI摘要",
            "--type", "text",
            "--ai-config", json.dumps(ai_config, ensure_ascii=False),
        )
        body = data.get("data", {})
        fid = body.get("fieldId") or (body.get("results", [{}])[0].get("fieldId") if body.get("results") else None)
        assert fid, f"AI field should be created, got: {data}"
        print(f"  [OK] AI field created: {fid}")


# ═══════════════════════════════════════════════════════════════
# field update — rename
# ═══════════════════════════════════════════════════════════════

class TestFieldUpdateRename:
    """验证 field update 重命名。"""

    def test_field_rename(self, dws, test_base_id, adv_tables):
        """创建字段后修改名称。"""
        main_id = adv_tables["main_table_id"]

        # Create a disposable field
        create_data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", main_id,
            "--name", "临时字段",
            "--type", "text",
        )
        body = create_data.get("data", {})
        fid = body.get("fieldId") or (body.get("results", [{}])[0].get("fieldId") if body.get("results") else None)
        assert fid

        # Rename
        dws.run(
            "aitable", "field", "update",
            "--base-id", test_base_id,
            "--table-id", main_id,
            "--field-id", fid,
            "--name", "已重命名字段",
        )

        # Verify via table get
        table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", main_id)
        fields = table_data["data"]["tables"][0].get("fields", [])
        names = [f["fieldName"] for f in fields]
        assert "已重命名字段" in names, f"renamed field should appear, got: {names}"


# ═══════════════════════════════════════════════════════════════
# field delete
# ═══════════════════════════════════════════════════════════════

class TestFieldDelete:
    """验证 field delete 操作。"""

    def test_field_delete(self, dws, test_base_id, adv_tables):
        """创建字段后删除，验证字段消失。"""
        main_id = adv_tables["main_table_id"]

        # Create a disposable field
        create_data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", main_id,
            "--name", "待删除字段",
            "--type", "text",
        )
        body = create_data.get("data", {})
        fid = body.get("fieldId") or (body.get("results", [{}])[0].get("fieldId") if body.get("results") else None)
        assert fid

        # Delete
        dws.run(
            "aitable", "field", "delete",
            "--base-id", test_base_id,
            "--table-id", main_id,
            "--field-id", fid,
            "--yes",
        )

        # Verify field is gone
        table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", main_id)
        fields = table_data["data"]["tables"][0].get("fields", [])
        field_ids = [f["fieldId"] for f in fields]
        assert fid not in field_ids, f"deleted field should not appear, got: {field_ids}"
