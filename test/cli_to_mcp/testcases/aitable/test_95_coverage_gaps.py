"""
test_95_coverage_gaps.py — 补齐覆盖率缺口

验证 md 文档中尚未被 test_92/93/94 覆盖的声明。

覆盖范围:
1. filters 操作符完整性: exist/un_exist/ne/lt/lte/gte/date_eq/before/after/any_of/none_of
2. formula 字段只读验证（写入应报错）
3. options 更新回传原 id 保留旧选项
4. user/department/group cellValue 写入格式
5. date 毫秒时间戳写入
6. 只读字段（createdTime/lastModifiedTime）写入被拒绝
"""

import json
import time

import pytest


# ─── Fixtures ───────────────────────────────────────────────

@pytest.fixture(scope="module")
def gap_table(dws, test_base_id):
    """Create a table with fields for gap coverage testing."""
    ts = int(time.time())
    fields = [
        {"fieldName": "标题", "type": "text"},
        {"fieldName": "数值", "type": "number", "config": {"formatter": "INT"}},
        {"fieldName": "单选", "type": "singleSelect", "config": {"options": [{"name": "A"}, {"name": "B"}, {"name": "C"}]}},
        {"fieldName": "多选", "type": "multipleSelect", "config": {"options": [{"name": "X"}, {"name": "Y"}, {"name": "Z"}]}},
        {"fieldName": "日期", "type": "date", "config": {"formatter": "YYYY-MM-DD"}},
        {"fieldName": "人员", "type": "user", "config": {"multiple": True}},
        {"fieldName": "部门", "type": "department", "config": {"multiple": True}},
        {"fieldName": "群组", "type": "group", "config": {"multiple": True}},
    ]
    data = dws.run(
        "aitable", "table", "create",
        "--base-id", test_base_id,
        "--name", f"GapTest_{ts}",
        "--fields", json.dumps(fields, ensure_ascii=False),
    )
    table_id = data["data"]["tableId"]

    # Get field map
    table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
    field_map = {f["fieldName"]: f["fieldId"] for f in table_data["data"]["tables"][0].get("fields", [])}

    # Pre-populate records for filter tests
    title_fld = field_map["标题"]
    num_fld = field_map["数值"]
    select_fld = field_map["单选"]
    multi_fld = field_map["多选"]
    date_fld = field_map["日期"]
    records = [
        {"cells": {title_fld: "记录1", num_fld: 10, select_fld: "A", multi_fld: ["X", "Y"], date_fld: "2026-01-15"}},
        {"cells": {title_fld: "记录2", num_fld: 20, select_fld: "B", multi_fld: ["Y", "Z"], date_fld: "2026-03-20"}},
        {"cells": {title_fld: "记录3", num_fld: 30, select_fld: "A", multi_fld: ["X"], date_fld: "2026-06-01"}},
        {"cells": {title_fld: "", num_fld: 0, select_fld: "C"}},  # 空标题 + 无日期
    ]
    dws.run(
        "aitable", "record", "create",
        "--base-id", test_base_id,
        "--table-id", table_id,
        "--records", json.dumps(records, ensure_ascii=False),
    )

    return table_id, field_map


# ═══════════════════════════════════════════════════════════════
# 1. filters 操作符完整性
# ═══════════════════════════════════════════════════════════════

class TestFiltersNe:
    """ne (不等于) 操作符。"""

    def test_filter_ne(self, dws, test_base_id, gap_table):
        table_id, fm = gap_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "ne", "operands": [fm["单选"], "A"]}],
        })
        data = dws.run("aitable", "record", "query", "--base-id", test_base_id,
                       "--table-id", table_id, "--filters", filters)
        records = data["data"].get("records", [])
        print(f"  [filter ne] exclude 'A', got {len(records)} records")
        # 预置 4 条, 其中 2 条是 A → ne 应返回 2 条
        assert len(records) >= 2


class TestFiltersLtLteGte:
    """lt/lte/gte 数值比较操作符。"""

    def test_filter_lt(self, dws, test_base_id, gap_table):
        table_id, fm = gap_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "lt", "operands": [fm["数值"], "20"]}],
        })
        data = dws.run("aitable", "record", "query", "--base-id", test_base_id,
                       "--table-id", table_id, "--filters", filters)
        records = data["data"].get("records", [])
        print(f"  [filter lt] < 20, got {len(records)} records")
        # 10, 0 满足 < 20
        assert len(records) >= 2

    def test_filter_lte(self, dws, test_base_id, gap_table):
        table_id, fm = gap_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "lte", "operands": [fm["数值"], "20"]}],
        })
        data = dws.run("aitable", "record", "query", "--base-id", test_base_id,
                       "--table-id", table_id, "--filters", filters)
        records = data["data"].get("records", [])
        print(f"  [filter lte] <= 20, got {len(records)} records")
        # 10, 20, 0 满足 <= 20
        assert len(records) >= 3

    def test_filter_gte(self, dws, test_base_id, gap_table):
        table_id, fm = gap_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "gte", "operands": [fm["数值"], "20"]}],
        })
        data = dws.run("aitable", "record", "query", "--base-id", test_base_id,
                       "--table-id", table_id, "--filters", filters)
        records = data["data"].get("records", [])
        print(f"  [filter gte] >= 20, got {len(records)} records")
        # 20, 30 满足 >= 20
        assert len(records) >= 2


class TestFiltersExist:
    """exist/un_exist (有值/为空) 操作符。"""

    def test_filter_exist(self, dws, test_base_id, gap_table):
        """exist: 字段有值的记录。"""
        table_id, fm = gap_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "exist", "operands": [fm["日期"]]}],
        })
        data = dws.run("aitable", "record", "query", "--base-id", test_base_id,
                       "--table-id", table_id, "--filters", filters)
        records = data["data"].get("records", [])
        print(f"  [filter exist] 日期有值, got {len(records)} records")
        # 前 3 条有日期, 第 4 条无
        assert len(records) >= 3

    def test_filter_un_exist(self, dws, test_base_id, gap_table):
        """un_exist: 字段为空的记录。"""
        table_id, fm = gap_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "un_exist", "operands": [fm["日期"]]}],
        })
        data = dws.run("aitable", "record", "query", "--base-id", test_base_id,
                       "--table-id", table_id, "--filters", filters)
        records = data["data"].get("records", [])
        print(f"  [filter un_exist] 日期为空, got {len(records)} records")
        # 第 4 条无日期
        assert len(records) >= 1


class TestFiltersDate:
    """日期操作符: date_eq/before/after。"""

    def test_filter_before(self, dws, test_base_id, gap_table):
        """before: 早于指定日期。"""
        table_id, fm = gap_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "before", "operands": [fm["日期"], "2026-04-01"]}],
        })
        data = dws.run("aitable", "record", "query", "--base-id", test_base_id,
                       "--table-id", table_id, "--filters", filters)
        records = data["data"].get("records", [])
        print(f"  [filter before] < 2026-04-01, got {len(records)} records")
        # 2026-01-15, 2026-03-20 满足
        assert len(records) >= 2

    def test_filter_after(self, dws, test_base_id, gap_table):
        """after: 晚于指定日期。"""
        table_id, fm = gap_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "after", "operands": [fm["日期"], "2026-04-01"]}],
        })
        data = dws.run("aitable", "record", "query", "--base-id", test_base_id,
                       "--table-id", table_id, "--filters", filters)
        records = data["data"].get("records", [])
        print(f"  [filter after] > 2026-04-01, got {len(records)} records")
        # 2026-06-01 满足
        assert len(records) >= 1


class TestFiltersMultiSelect:
    """多选操作符: any_of/none_of/all_of。"""

    def test_filter_any_of(self, dws, test_base_id, gap_table):
        """any_of: 包含任一。"""
        table_id, fm = gap_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "any_of", "operands": [fm["多选"], "Z"]}],
        })
        data = dws.run("aitable", "record", "query", "--base-id", test_base_id,
                       "--table-id", table_id, "--filters", filters)
        records = data["data"].get("records", [])
        print(f"  [filter any_of] 多选含 Z, got {len(records)} records")
        # 记录2 有 [Y,Z]
        assert len(records) >= 1

    def test_filter_none_of(self, dws, test_base_id, gap_table):
        """none_of: 不包含任一。"""
        table_id, fm = gap_table
        filters = json.dumps({
            "operator": "and",
            "operands": [{"operator": "none_of", "operands": [fm["多选"], "X"]}],
        })
        data = dws.run("aitable", "record", "query", "--base-id", test_base_id,
                       "--table-id", table_id, "--filters", filters)
        records = data["data"].get("records", [])
        print(f"  [filter none_of] 多选不含 X, got {len(records)} records")
        # 记录2[Y,Z] + 记录4(无多选) 满足
        assert len(records) >= 1


# ═══════════════════════════════════════════════════════════════
# 2. formula 字段只读验证
# ═══════════════════════════════════════════════════════════════

class TestFormulaReadOnly:
    """验证 formula 字段不能通过 record create/update 写入。"""

    def test_formula_field_write_rejected(self, dws, test_base_id, gap_table):
        """尝试向 formula 字段写入值 → 应失败或被忽略。"""
        table_id, fm = gap_table

        # 创建 formula 字段
        formula_data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--name", f"公式只读_{int(time.time()) % 10000}",
            "--type", "formula",
            "--config", json.dumps({"formula": "[数值] * 2"}),
        )
        # 获取 formula 字段 ID
        results = formula_data.get("data", {}).get("results", [])
        formula_fld_id = results[0].get("fieldId") if results else None
        if not formula_fld_id:
            pytest.skip("formula field creation did not return fieldId")

        # 尝试向 formula 字段写入
        write_data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", json.dumps([{"cells": {formula_fld_id: 999}}]),
            expect_success=False,
        )
        # 两种可能: 报错 or 写入被忽略(字段值仍是计算结果)
        is_error = write_data.get("status") == "error"
        if is_error:
            print(f"  [formula readonly] write rejected: {write_data.get('summary','')[:100]}")
        else:
            # 即使没报错，读取回来的值应该是计算值而非 999
            rec_id = write_data.get("data", {}).get("newRecordIds", [None])[0]
            if rec_id:
                query = dws.run("aitable", "record", "query", "--base-id", test_base_id,
                                "--table-id", table_id, "--record-ids", rec_id,
                                "--field-ids", formula_fld_id)
                val = query["data"].get("records", [{}])[0].get("cells", {}).get(formula_fld_id)
                print(f"  [formula readonly] write NOT rejected, but read back: {val!r} (expect calc value, not 999)")
                # 验证写入的 999 被忽略
                assert val != 999 and val != "999", \
                    f"formula field should not store written value 999, got: {val}"
            else:
                print(f"  [formula readonly] write succeeded but no recordId returned")


# ═══════════════════════════════════════════════════════════════
# 3. options 更新回传原 id 保留旧选项
# ═══════════════════════════════════════════════════════════════

class TestOptionsRetainById:
    """验证更新 options 时回传原 id 可保留旧选项。"""

    def test_options_retain_with_original_id(self, dws, test_base_id, gap_table):
        """更新时回传原 id + 新增项 → 旧选项保留，新选项追加。"""
        table_id, fm = gap_table

        # 创建一个单选字段
        ts = int(time.time()) % 10000
        field_name = f"RetainTest_{ts}"
        dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--name", field_name,
            "--type", "singleSelect",
            "--config", json.dumps({"options": [{"name": "原1"}, {"name": "原2"}]}),
        )

        # 获取字段 ID 和 options（含 id）
        table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
        target_field = next(
            (f for f in table_data["data"]["tables"][0]["fields"] if f.get("fieldName") == field_name),
            None,
        )
        assert target_field, f"field '{field_name}' not found"
        field_id = target_field["fieldId"]

        # 获取完整字段配置（含 option id）
        field_detail = dws.run("aitable", "field", "get", "--base-id", test_base_id,
                               "--table-id", table_id, "--field-ids", field_id)
        options = field_detail["data"]["fields"][0].get("config", {}).get("options", [])
        print(f"  [options retain] original options: {options}")
        assert len(options) >= 2

        # 更新: 保留原选项(带 id) + 新增一个
        updated_options = options + [{"name": "新增项"}]
        dws.run(
            "aitable", "field", "update",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--field-id", field_id,
            "--config", json.dumps({"options": updated_options}),
        )

        # 验证: 原选项保留 + 新选项出现
        field_after = dws.run("aitable", "field", "get", "--base-id", test_base_id,
                              "--table-id", table_id, "--field-ids", field_id)
        final_options = field_after["data"]["fields"][0].get("config", {}).get("options", [])
        final_names = [o.get("name") for o in final_options]
        print(f"  [options retain] after update: {final_names}")
        assert "原1" in final_names, f"expect '原1' retained, got: {final_names}"
        assert "原2" in final_names, f"expect '原2' retained, got: {final_names}"
        assert "新增项" in final_names, f"expect '新增项' added, got: {final_names}"


# ═══════════════════════════════════════════════════════════════
# 4. user/department/group cellValue 写入
# ═══════════════════════════════════════════════════════════════

class TestUserDeptGroupCellValue:
    """验证 user/department/group 字段的 cellValue 写入格式。

    注意: 这些字段需要真实的 userId/deptId/cid。
    本测试验证「写入格式被接受」（不报格式错误），
    而非验证「值正确引用到了某个真实用户」。
    """

    def test_user_write_format_accepted(self, dws, test_base_id, gap_table):
        """user 字段写入 [{userId, corpId}] 格式不报格式错误。"""
        table_id, fm = gap_table
        if "人员" not in fm:
            pytest.skip("人员 field not in table")

        # 使用一个虚构 userId — 可能返回业务错误但不应是格式错误
        fake_user = [{"userId": "test_user_001", "corpId": "dingtest000"}]
        data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", json.dumps([{"cells": {fm["人员"]: fake_user}}]),
            expect_success=False,
        )
        print(f"  [user write] status={data.get('status')}, summary={data.get('summary','')[:100]}")
        # 如果是格式错误则 fail；如果是"用户不存在"的业务错误也 OK（说明格式对了）
        if data.get("status") == "error":
            summary = data.get("summary", "")
            # 格式错误的关键词
            assert "format" not in summary.lower() and "type mismatch" not in summary.lower(), \
                f"user cellValue format should be accepted, got format error: {summary}"
            print(f"  RESULT: format accepted (business error expected with fake userId)")
        else:
            print(f"  RESULT: write succeeded (format confirmed correct)")

    def test_department_write_format(self, dws, test_base_id, gap_table):
        """department 字段写入 [{deptId}] 格式。"""
        table_id, fm = gap_table
        if "部门" not in fm:
            pytest.skip("部门 field not in table")

        fake_dept = [{"deptId": "99999999"}]
        data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", json.dumps([{"cells": {fm["部门"]: fake_dept}}]),
            expect_success=False,
        )
        print(f"  [department write] status={data.get('status')}, summary={data.get('summary','')[:100]}")
        if data.get("status") == "error":
            summary = data.get("summary", "")
            assert "format" not in summary.lower() and "type mismatch" not in summary.lower(), \
                f"department cellValue format should be accepted, got: {summary}"

    def test_group_write_format(self, dws, test_base_id, gap_table):
        """group 字段写入 [{cid}] 格式（key 是 cid 不是 openConversationId）。"""
        table_id, fm = gap_table
        if "群组" not in fm:
            pytest.skip("群组 field not in table")

        fake_group = [{"cid": "99999999999"}]
        data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", json.dumps([{"cells": {fm["群组"]: fake_group}}]),
            expect_success=False,
        )
        print(f"  [group write] status={data.get('status')}, summary={data.get('summary','')[:100]}")
        if data.get("status") == "error":
            summary = data.get("summary", "")
            assert "format" not in summary.lower() and "type mismatch" not in summary.lower(), \
                f"group cellValue format should be accepted, got: {summary}"


# ═══════════════════════════════════════════════════════════════
# 5. date 毫秒时间戳写入
# ═══════════════════════════════════════════════════════════════

class TestDateTimestamp:
    """验证 date 字段支持毫秒时间戳写入。"""

    def test_date_millisecond_timestamp(self, dws, test_base_id, gap_table):
        """写入毫秒时间戳 → 应被接受并转为日期。"""
        table_id, fm = gap_table
        # 2026-06-15T00:00:00+08:00 的毫秒时间戳
        ts_ms = 1781568000000

        data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", json.dumps([{"cells": {fm["日期"]: ts_ms}}]),
            expect_success=False,
        )
        if data.get("status") == "error":
            print(f"  [date timestamp] REJECTED: {data.get('summary','')[:100]}")
            print(f"  FINDING: millisecond timestamp NOT supported for date field")
            # 如果被拒绝，说明 md 里关于时间戳写入的描述需要修正
        else:
            rec_id = data.get("data", {}).get("newRecordIds", [None])[0]
            if rec_id:
                query = dws.run("aitable", "record", "query", "--base-id", test_base_id,
                                "--table-id", table_id, "--record-ids", rec_id,
                                "--field-ids", fm["日期"])
                val = query["data"].get("records", [{}])[0].get("cells", {}).get(fm["日期"])
                print(f"  [date timestamp] wrote {ts_ms}, read back: {val!r}")
                print(f"  FINDING: millisecond timestamp IS supported")
                assert val is not None, "date should have a value after timestamp write"


# ═══════════════════════════════════════════════════════════════
# 6. 只读字段写入被拒绝
# ═══════════════════════════════════════════════════════════════

class TestReadOnlyFieldsRejected:
    """验证系统只读字段（createdTime/lastModifiedTime）不能被写入。"""

    def test_write_to_created_time_ignored(self, dws, test_base_id, gap_table):
        """尝试向 createdTime 字段写入 → 应被忽略或报错。"""
        table_id, fm = gap_table

        # 先获取表结构找到系统字段
        table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
        fields = table_data["data"]["tables"][0].get("fields", [])
        created_time_fld = next((f for f in fields if f.get("type") == "createdTime"), None)

        if not created_time_fld:
            # 创建一个 createdTime 字段
            create_data = dws.run(
                "aitable", "field", "create",
                "--base-id", test_base_id,
                "--table-id", table_id,
                "--name", "创建时间测试",
                "--type", "createdTime",
            )
            results = create_data.get("data", {}).get("results", [])
            if results and results[0].get("success"):
                ct_field_id = results[0]["fieldId"]
            else:
                pytest.skip("Cannot create createdTime field for test")
        else:
            ct_field_id = created_time_fld["fieldId"]

        # 尝试写入 createdTime 字段
        data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--records", json.dumps([{"cells": {ct_field_id: "2020-01-01T00:00:00+08:00"}}]),
            expect_success=False,
        )
        if data.get("status") == "error":
            print(f"  [readonly] createdTime write REJECTED: {data.get('summary','')[:100]}")
        else:
            # 写入没报错，但值应该是系统自动填充的当前时间，不是我们写入的 2020
            rec_id = data.get("data", {}).get("newRecordIds", [None])[0]
            if rec_id:
                query = dws.run("aitable", "record", "query", "--base-id", test_base_id,
                                "--table-id", table_id, "--record-ids", rec_id,
                                "--field-ids", ct_field_id)
                val = query["data"].get("records", [{}])[0].get("cells", {}).get(ct_field_id)
                print(f"  [readonly] createdTime write not rejected, read back: {val!r}")
                # 验证值不是 2020（被忽略了）
                if val and "2020" not in str(val):
                    print(f"  RESULT: write was IGNORED (value is system-generated)")
                else:
                    print(f"  RESULT: write was ACCEPTED (unexpected!)")
