"""
test_94_field_edge_cases.py — 字段边界行为验证

验证 aitable-field-properties.md 和 aitable-cell-value.md 中描述的边界行为。

覆盖范围:
- singleSelect 写入不存在的 option name → 是否自动创建新选项
- multipleSelect 写入不存在的 option name → 是否自动创建
- options 更新是否真的全量覆盖（旧选项消失验证）
- progress 写入 75（非 0.75）→ 验证行为
- rating 写入超出 max 范围 → 验证行为
- filters 根节点必须 and/or（缺失时行为验证）
- filters 操作符验证: eq/contain/gt/is_empty
- sort direction vs order 验证
"""

import json
import time

import pytest


@pytest.fixture(scope="module")
def edge_table(dws, test_base_id):
    """Create a table with fields for edge case testing."""
    ts = int(time.time())
    fields = [
        {"fieldName": "标题", "type": "text"},
        {"fieldName": "单选边界", "type": "singleSelect", "config": {"options": [{"name": "已有A"}, {"name": "已有B"}]}},
        {"fieldName": "多选边界", "type": "multipleSelect", "config": {"options": [{"name": "标签1"}, {"name": "标签2"}]}},
        {"fieldName": "进度边界", "type": "progress", "config": {"formatter": "PERCENT"}},
        {"fieldName": "评分边界", "type": "rating", "config": {"min": 1, "max": 5, "icon": "star"}},
        {"fieldName": "数字筛选", "type": "number", "config": {"formatter": "INT"}},
    ]
    data = dws.run(
        "aitable", "table", "create",
        "--base-id", test_base_id,
        "--name", f"EdgeCaseTest_{ts}",
        "--fields", json.dumps(fields, ensure_ascii=False),
    )
    table_id = data["data"]["tableId"]

    # Get field map
    table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
    field_map = {f["fieldName"]: f["fieldId"] for f in table_data["data"]["tables"][0].get("fields", [])}

    # Pre-populate some records for filter tests
    title_fld = field_map["标题"]
    num_fld = field_map["数字筛选"]
    select_fld = field_map["单选边界"]
    records = [
        {"cells": {title_fld: "记录1", num_fld: 10, select_fld: "已有A"}},
        {"cells": {title_fld: "记录2", num_fld: 20, select_fld: "已有B"}},
        {"cells": {title_fld: "记录3", num_fld: 30, select_fld: "已有A"}},
        {"cells": {title_fld: "特殊记录", num_fld: 100, select_fld: "已有B"}},
    ]
    dws.run(
        "aitable", "record", "create",
        "--base-id", test_base_id,
        "--table-id", table_id,
        "--records", json.dumps(records, ensure_ascii=False),
    )

    return table_id, field_map


# ─── singleSelect: 自动创建新选项 ──────────────────────────

class TestSelectAutoCreate:
    """验证写入不存在的 option name 时是否自动创建。"""

    def test_single_select_new_option_auto_created(self, dws, test_base_id, edge_table):
        """写入不存在的选项名 → 应自动创建该选项。"""
        table_id, fm = edge_table
        new_option_name = f"新选项_{int(time.time()) % 10000}"

        # 写入不存在的选项
        create_data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", json.dumps([{"cells": {fm["单选边界"]: new_option_name}}]),
        )
        body = create_data["data"]
        rec_id = body.get("newRecordIds", [None])[0] or body.get("records", [{}])[0].get("recordId")
        assert rec_id, f"record should be created, got: {create_data}"

        # 读取确认值写入成功
        query_data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--record-ids", rec_id,
            "--field-ids", fm["单选边界"],
        )
        records = query_data["data"].get("records", [])
        val = records[0].get("cells", {}).get(fm["单选边界"])
        print(f"  [singleSelect auto-create] wrote '{new_option_name}', read back: {val!r}")

        # 验证字段配置中新选项已出现
        field_data = dws.run(
            "aitable", "field", "get",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--field-ids", fm["单选边界"],
        )
        fields = field_data["data"].get("fields", [])
        options = fields[0].get("config", {}).get("options", []) if fields else []
        option_names = [o.get("name") for o in options]
        print(f"  [singleSelect auto-create] options after write: {option_names}")
        assert new_option_name in option_names, \
            f"expect new option '{new_option_name}' auto-created in config, got: {option_names}"

    def test_multiple_select_new_option_auto_created(self, dws, test_base_id, edge_table):
        """multipleSelect 写入不存在的选项名 → 应自动创建。"""
        table_id, fm = edge_table
        new_opt = f"新标签_{int(time.time()) % 10000}"

        create_data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", json.dumps([{"cells": {fm["多选边界"]: ["标签1", new_opt]}}]),
        )
        body = create_data["data"]
        rec_id = body.get("newRecordIds", [None])[0] or body.get("records", [{}])[0].get("recordId")
        assert rec_id

        # 验证配置中新标签已出现
        field_data = dws.run(
            "aitable", "field", "get",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--field-ids", fm["多选边界"],
        )
        fields = field_data["data"].get("fields", [])
        options = fields[0].get("config", {}).get("options", []) if fields else []
        option_names = [o.get("name") for o in options]
        print(f"  [multipleSelect auto-create] options after write: {option_names}")
        assert new_opt in option_names


# ─── options 更新全量覆盖验证 ──────────────────────────────

class TestOptionsOverwrite:
    """验证 field update options 是全量覆盖而非追加。"""

    def test_options_update_is_overwrite(self, dws, test_base_id, edge_table):
        """更新 options 只传新选项 → 旧选项应消失。"""
        table_id, fm = edge_table

        # 创建一个新单选字段用于此测试（避免影响其他测试）
        ts = int(time.time()) % 10000
        field_name = f"OverwriteTest_{ts}"
        dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--name", field_name,
            "--type", "singleSelect",
            "--config", json.dumps({"options": [{"name": "原A"}, {"name": "原B"}, {"name": "原C"}]}),
        )

        # 获取新字段 ID
        table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
        new_fld = next(
            (f for f in table_data["data"]["tables"][0]["fields"] if f.get("fieldName") == field_name),
            None,
        )
        assert new_fld, f"field '{field_name}' not found after create"
        field_id = new_fld["fieldId"]

        # 更新 options: 只传两个新选项（不传原有选项）
        dws.run(
            "aitable", "field", "update",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--field-id", field_id,
            "--config", json.dumps({"options": [{"name": "新X"}, {"name": "新Y"}]}),
        )

        # 验证: 原选项应该消失
        field_data = dws.run(
            "aitable", "field", "get",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--field-ids", field_id,
        )
        fields = field_data["data"].get("fields", [])
        options = fields[0].get("config", {}).get("options", []) if fields else []
        option_names = [o.get("name") for o in options]
        print(f"  [options overwrite] after update: {option_names}")
        assert "新X" in option_names
        assert "新Y" in option_names
        # 关键验证: 原选项应消失（全量覆盖）
        assert "原A" not in option_names, \
            f"expect '原A' removed after overwrite, but still exists: {option_names}"
        assert "原B" not in option_names
        assert "原C" not in option_names


# ─── progress 边界 ─────────────────────────────────────────

class TestProgressEdge:
    """验证 progress 值域行为。"""

    def test_progress_write_75_behavior(self, dws, test_base_id, edge_table):
        """写入 75（非 0.75）→ 记录行为（可能报错或被截断为 1）。"""
        table_id, fm = edge_table

        # 尝试写入 75
        try:
            create_data = dws.run(
                "aitable", "record", "create",
                "--base-id", test_base_id,
                "--table-id", table_id,
                "--records", json.dumps([{"cells": {fm["进度边界"]: 75}}]),
            )
            # 如果没报错，看读取回来是什么
            body = create_data["data"]
            rec_id = body.get("newRecordIds", [None])[0] or body.get("records", [{}])[0].get("recordId")
            if rec_id:
                query_data = dws.run(
                    "aitable", "record", "query",
                    "--base-id", test_base_id,
                    "--table-id", table_id,
                    "--record-ids", rec_id,
                    "--field-ids", fm["进度边界"],
                )
                records = query_data["data"].get("records", [])
                val = records[0].get("cells", {}).get(fm["进度边界"]) if records else None
                print(f"  [progress edge] wrote 75, read back: {val!r}")
                print(f"  RESULT: writing 75 did NOT error — stored as {val}")
            else:
                print(f"  [progress edge] wrote 75, create succeeded but no recordId")
        except Exception as e:
            print(f"  [progress edge] wrote 75, got error: {e}")
            print(f"  RESULT: writing 75 DOES cause error (as expected)")


# ─── rating 边界 ───────────────────────────────────────────

class TestRatingEdge:
    """验证 rating 超出 max 范围的行为。"""

    def test_rating_over_max_rejected(self, dws, test_base_id, edge_table):
        """rating 写入 6（超出 max=5）→ 应被服务端拒绝（返回 error）。"""
        table_id, fm = edge_table

        # 使用 expect_success=False 允许 API 返回错误
        create_data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", json.dumps([{"cells": {fm["评分边界"]: 6}}]),
            expect_success=False,
        )
        # 验证: 超出范围应返回错误
        is_error = (
            create_data.get("status") == "error"
            or (create_data.get("error") and create_data.get("error") != {})
        )
        print(f"  [rating edge] wrote 6 (max=5), response status: {create_data.get('status')}")
        print(f"  RESULT: writing over max {'DOES' if is_error else 'does NOT'} cause error")
        assert is_error, \
            f"expect rating=6 (max=5) to be rejected, but got: {create_data}"


# ─── filters 行为验证 ──────────────────────────────────────

class TestFilters:
    """验证 aitable-filter-sort.md 中描述的 filters 行为。"""

    def test_filter_eq_by_name(self, dws, test_base_id, edge_table):
        """filters 用 singleSelect name 过滤 (eq)。"""
        table_id, fm = edge_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "eq", "operands": [fm["单选边界"], "已有A"]}],
        })
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--filters", filters,
        )
        records = data["data"].get("records", [])
        print(f"  [filter eq] filtered by '已有A', got {len(records)} records")
        # 我们预置了2条 "已有A" + 可能有自动创建测试的额外记录
        assert len(records) >= 2, f"expect at least 2 records with '已有A', got {len(records)}"

    def test_filter_gt(self, dws, test_base_id, edge_table):
        """filters 数值 gt 操作符。"""
        table_id, fm = edge_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "gt", "operands": [fm["数字筛选"], "25"]}],
        })
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--filters", filters,
        )
        records = data["data"].get("records", [])
        print(f"  [filter gt] filtered by > 25, got {len(records)} records")
        # 预置: 30, 100 → 至少 2 条
        assert len(records) >= 2

    def test_filter_contain_text(self, dws, test_base_id, edge_table):
        """filters 文本 contain 操作符。"""
        table_id, fm = edge_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "contain", "operands": [fm["标题"], "特殊"]}],
        })
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--filters", filters,
        )
        records = data["data"].get("records", [])
        print(f"  [filter contain] filtered by contain '特殊', got {len(records)} records")
        assert len(records) >= 1

    def test_filter_or_logic(self, dws, test_base_id, edge_table):
        """filters 根节点 or 逻辑。"""
        table_id, fm = edge_table
        filters = json.dumps({
            "operator": "or",
            "operands": [
                {"operator": "eq", "operands": [fm["数字筛选"], "10"]},
                {"operator": "eq", "operands": [fm["数字筛选"], "100"]},
            ],
        })
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--filters", filters,
        )
        records = data["data"].get("records", [])
        print(f"  [filter or] 10 or 100, got {len(records)} records")
        assert len(records) >= 2

    def test_filter_missing_root_operator_returns_all(self, dws, test_base_id, edge_table):
        """缺失根节点 and/or 时（错误格式）→ API 应忽略 filter 返回全表。"""
        table_id, fm = edge_table
        # 错误格式: 直接用 eq 作根节点
        bad_filters = json.dumps({"operator": "eq", "operands": [fm["数字筛选"], "10"]})
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--filters", bad_filters,
        )
        records = data["data"].get("records", [])
        print(f"  [filter bad root] no and/or root, got {len(records)} records")
        # 应该返回全部（至少4条预置 + 可能的额外记录）
        assert len(records) >= 4, \
            f"expect all records returned when filter root is invalid, got {len(records)}"


# ─── sort 验证 ─────────────────────────────────────────────

class TestSort:
    """验证 sort 参数行为。"""

    def test_sort_direction_desc(self, dws, test_base_id, edge_table):
        """sort 用 direction=desc 降序排列。

        注意：记录写入后排序索引可能有短暂延迟，带重试验证。
        """
        import time
        table_id, fm = edge_table
        sort_param = json.dumps([{"fieldId": fm["数字筛选"], "direction": "desc"}])

        for attempt in range(3):
            if attempt > 0:
                time.sleep(2)
            data = dws.run(
                "aitable", "record", "query",
                "--base-id", test_base_id,
                "--table-id", table_id,
                "--sort", sort_param,
                "--field-ids", fm["数字筛选"],
            )
            records = data["data"].get("records", [])
            nums = []
            for r in records:
                v = r.get("cells", {}).get(fm["数字筛选"])
                if v is not None:
                    nums.append(float(str(v)))
            print(f"  [sort desc attempt {attempt+1}] numbers: {nums[:10]}")
            if len(nums) >= 2 and nums[0] >= nums[1]:
                break
        # 最终验证
        if len(nums) >= 2:
            assert nums[0] >= nums[1], f"expect descending, first={nums[0]}, second={nums[1]}"

    def test_sort_direction_asc(self, dws, test_base_id, edge_table):
        """sort 用 direction=asc 升序排列。"""
        table_id, fm = edge_table
        sort_param = json.dumps([{"fieldId": fm["数字筛选"], "direction": "asc"}])
        data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--sort", sort_param,
            "--field-ids", fm["数字筛选"],
        )
        records = data["data"].get("records", [])
        nums = []
        for r in records:
            v = r.get("cells", {}).get(fm["数字筛选"])
            if v is not None:
                nums.append(float(str(v)))
        print(f"  [sort asc] numbers: {nums[:10]}")
        if len(nums) >= 2:
            assert nums[0] <= nums[1], f"expect ascending, first={nums[0]}, second={nums[1]}"
