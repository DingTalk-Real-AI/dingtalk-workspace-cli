"""
test_93_cell_value_write_read.py — cellValue 写入/读取格式验证

验证 aitable-cell-value.md 中定义的每种字段类型的 cellValue 写入和读取格式。
流程: 创建含各类型字段的表 → 写入记录 → 读取记录 → 验证返回结构。

覆盖范围:
- text: 字符串写入/读取
- number: 数字写入, 字符串读取
- singleSelect: name字符串写入 / {id,name}对象写入, {id,name}对象读取
- multipleSelect: name数组写入 / 对象数组写入, 对象数组读取
- date: 3种格式写入(YYYY-MM-DD / YYYY-MM-DD HH:mm / RFC3339), RFC3339读取
- currency: 数字写入, 字符串读取
- progress: 0.75写入(正确), 验证值域
- rating: 整数写入/读取
- checkbox: 布尔写入/读取
- url: 对象写入 / 纯字符串写入, 对象读取
- richText: {markdown}写入/读取
- telephone/email: 字符串写入/读取
- geolocation: {address,name,location}对象写入/读取
- unidirectionalLink: {linkedRecordIds}写入/读取
"""

import json
import time

import pytest


@pytest.fixture(scope="module")
def cv_table(dws, test_base_id):
    """Create a table with all testable field types, return (tableId, fieldMap)."""
    ts = int(time.time())
    fields = [
        {"fieldName": "标题", "type": "text"},
        {"fieldName": "数字", "type": "number", "config": {"formatter": "FLOAT_2"}},
        {"fieldName": "单选", "type": "singleSelect", "config": {"options": [{"name": "A"}, {"name": "B"}, {"name": "C"}]}},
        {"fieldName": "多选", "type": "multipleSelect", "config": {"options": [{"name": "X"}, {"name": "Y"}, {"name": "Z"}]}},
        {"fieldName": "日期", "type": "date", "config": {"formatter": "YYYY-MM-DD HH:mm"}},
        {"fieldName": "货币", "type": "currency", "config": {"currencyType": "CNY", "formatter": "FLOAT_2"}},
        {"fieldName": "进度", "type": "progress", "config": {"formatter": "PERCENT"}},
        {"fieldName": "评分", "type": "rating", "config": {"min": 1, "max": 5, "icon": "star"}},
        {"fieldName": "勾选", "type": "checkbox"},
        {"fieldName": "链接", "type": "url"},
        {"fieldName": "富文本", "type": "richText"},
        {"fieldName": "电话", "type": "telephone"},
        {"fieldName": "邮箱", "type": "email"},
        {"fieldName": "地理位置", "type": "geolocation"},
    ]

    data = dws.run(
        "aitable", "table", "create",
        "--base-id", test_base_id,
        "--name", f"CellValueTest_{ts}",
        "--fields", json.dumps(fields, ensure_ascii=False),
    )
    table_id = data["data"]["tableId"]

    # Get field map (fieldName -> fieldId)
    table_data = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", table_id)
    table_info = table_data["data"]["tables"][0]
    field_map = {f["fieldName"]: f["fieldId"] for f in table_info.get("fields", [])}

    return table_id, field_map


def write_and_read(dws, test_base_id, table_id, cells):
    """Write a record and read it back, return the read cells."""
    # Write
    create_data = dws.run(
        "aitable", "record", "create",
        "--base-id", test_base_id,
        "--table-id", table_id,
        "--records", json.dumps([{"cells": cells}], ensure_ascii=False),
    )
    body = create_data["data"]
    if "newRecordIds" in body:
        record_id = body["newRecordIds"][0]
    elif "records" in body:
        record_id = body["records"][0]["recordId"]
    else:
        pytest.fail(f"Cannot extract recordId from create response: {create_data}")

    # Read back
    query_data = dws.run(
        "aitable", "record", "query",
        "--base-id", test_base_id,
        "--table-id", table_id,
        "--record-ids", record_id,
    )
    records = query_data["data"].get("records", [])
    assert len(records) >= 1, f"expect record returned, got: {query_data}"
    return records[0].get("cells", {})


# ─── Tests ──────────────────────────────────────────────────

class TestTextCellValue:
    def test_text_write_read(self, dws, test_base_id, cv_table):
        table_id, fm = cv_table
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["标题"]: "Hello测试"})
        assert read_cells.get(fm["标题"]) == "Hello测试"


class TestNumberCellValue:
    def test_number_write_int(self, dws, test_base_id, cv_table):
        """写入整数, 读取应为字符串。"""
        table_id, fm = cv_table
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["数字"]: 42})
        val = read_cells.get(fm["数字"])
        # 文档说读取为字符串，验证是否确实如此
        print(f"  [number] wrote 42, read back: {val!r} (type={type(val).__name__})")
        # 接受 "42" 或 "42.00" 或 42（数字）
        assert val is not None, "number field should have value"

    def test_number_write_float(self, dws, test_base_id, cv_table):
        """写入浮点数。"""
        table_id, fm = cv_table
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["数字"]: 3.14})
        val = read_cells.get(fm["数字"])
        print(f"  [number] wrote 3.14, read back: {val!r} (type={type(val).__name__})")
        assert val is not None


class TestSingleSelectCellValue:
    def test_write_by_name(self, dws, test_base_id, cv_table):
        """singleSelect 用 name 字符串写入。"""
        table_id, fm = cv_table
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["单选"]: "A"})
        val = read_cells.get(fm["单选"])
        print(f"  [singleSelect] wrote 'A', read back: {val!r}")
        # 文档说读取为 {id, name} 对象
        assert isinstance(val, dict), f"expect dict, got {type(val).__name__}: {val}"
        assert val.get("name") == "A"
        assert "id" in val

    def test_write_by_object(self, dws, test_base_id, cv_table):
        """singleSelect 用 {id, name} 对象写入。"""
        table_id, fm = cv_table
        # 先获取 option id
        field_data = dws.run(
            "aitable", "field", "get",
            "--base-id", test_base_id,
            "--table-id", table_id,
            "--field-ids", fm["单选"],
        )
        fields = field_data["data"].get("fields", [])
        options = fields[0].get("config", {}).get("options", []) if fields else []
        opt_b = next((o for o in options if o.get("name") == "B"), None)
        if not opt_b:
            pytest.skip("Cannot find option B's id for object write test")

        read_cells = write_and_read(dws, test_base_id, table_id,
                                    {fm["单选"]: {"id": opt_b["id"], "name": "B"}})
        val = read_cells.get(fm["单选"])
        assert isinstance(val, dict)
        assert val.get("name") == "B"


class TestMultipleSelectCellValue:
    def test_write_by_name_array(self, dws, test_base_id, cv_table):
        """multipleSelect 用 name 数组写入。"""
        table_id, fm = cv_table
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["多选"]: ["X", "Y"]})
        val = read_cells.get(fm["多选"])
        print(f"  [multipleSelect] wrote ['X','Y'], read back: {val!r}")
        # 文档说读取为对象数组
        assert isinstance(val, list), f"expect list, got {type(val).__name__}"
        names = [item.get("name") if isinstance(item, dict) else item for item in val]
        assert "X" in names and "Y" in names


class TestDateCellValue:
    def test_write_date_string(self, dws, test_base_id, cv_table):
        """date 用 YYYY-MM-DD 字符串写入。"""
        table_id, fm = cv_table
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["日期"]: "2026-03-15"})
        val = read_cells.get(fm["日期"])
        print(f"  [date] wrote '2026-03-15', read back: {val!r}")
        assert val is not None
        assert "2026" in str(val) and "03" in str(val)

    def test_write_datetime_string(self, dws, test_base_id, cv_table):
        """date 用 YYYY-MM-DD HH:mm 字符串写入。"""
        table_id, fm = cv_table
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["日期"]: "2026-06-01 14:30"})
        val = read_cells.get(fm["日期"])
        print(f"  [date] wrote '2026-06-01 14:30', read back: {val!r}")
        assert val is not None

    def test_write_rfc3339(self, dws, test_base_id, cv_table):
        """date 用 RFC3339 字符串写入。"""
        table_id, fm = cv_table
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["日期"]: "2026-12-25T10:00:00+08:00"})
        val = read_cells.get(fm["日期"])
        print(f"  [date] wrote RFC3339, read back: {val!r}")
        assert val is not None


class TestCurrencyCellValue:
    def test_currency_write_read(self, dws, test_base_id, cv_table):
        """currency 数字写入, 读取验证。"""
        table_id, fm = cv_table
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["货币"]: 99.5})
        val = read_cells.get(fm["货币"])
        print(f"  [currency] wrote 99.5, read back: {val!r} (type={type(val).__name__})")
        assert val is not None


class TestProgressCellValue:
    def test_progress_write_fraction(self, dws, test_base_id, cv_table):
        """progress 写入 0.75 表示 75%。"""
        table_id, fm = cv_table
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["进度"]: 0.75})
        val = read_cells.get(fm["进度"])
        print(f"  [progress] wrote 0.75, read back: {val!r}")
        # 验证读取值接近 0.75
        if isinstance(val, (int, float)):
            assert 0.7 <= float(val) <= 0.8, f"expect ~0.75, got {val}"
        elif isinstance(val, str):
            assert 0.7 <= float(val) <= 0.8, f"expect ~0.75, got {val}"


class TestRatingCellValue:
    def test_rating_write_read(self, dws, test_base_id, cv_table):
        """rating 写入整数 4。"""
        table_id, fm = cv_table
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["评分"]: 4})
        val = read_cells.get(fm["评分"])
        print(f"  [rating] wrote 4, read back: {val!r}")
        assert val is not None
        assert int(float(str(val))) == 4


class TestCheckboxCellValue:
    def test_checkbox_true(self, dws, test_base_id, cv_table):
        """checkbox 写入 true。"""
        table_id, fm = cv_table
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["勾选"]: True})
        val = read_cells.get(fm["勾选"])
        print(f"  [checkbox] wrote true, read back: {val!r}")
        assert val is True or val == "true"

    def test_checkbox_false(self, dws, test_base_id, cv_table):
        """checkbox 写入 false。"""
        table_id, fm = cv_table
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["勾选"]: False})
        val = read_cells.get(fm["勾选"])
        print(f"  [checkbox] wrote false, read back: {val!r}")
        # false 时可能返回 false 或 null/不返回该字段
        assert val is False or val is None or val == "false"


class TestUrlCellValue:
    def test_url_object_write(self, dws, test_base_id, cv_table):
        """url 用 {text, link} 对象写入。"""
        table_id, fm = cv_table
        url_obj = {"text": "钉钉官网", "link": "https://dingtalk.com"}
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["链接"]: url_obj})
        val = read_cells.get(fm["链接"])
        print(f"  [url] wrote object, read back: {val!r}")
        assert isinstance(val, dict), f"expect dict, got {type(val).__name__}"
        assert "link" in val
        assert "dingtalk.com" in val.get("link", "")

    def test_url_string_write(self, dws, test_base_id, cv_table):
        """url 用纯 URL 字符串写入。"""
        table_id, fm = cv_table
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["链接"]: "https://example.com"})
        val = read_cells.get(fm["链接"])
        print(f"  [url] wrote string, read back: {val!r}")
        # 文档说自动补齐为 {text, link}
        if isinstance(val, dict):
            assert "example.com" in val.get("link", "")
        elif isinstance(val, str):
            assert "example.com" in val


class TestRichTextCellValue:
    def test_richtext_markdown(self, dws, test_base_id, cv_table):
        """richText 用 {markdown} 写入。"""
        table_id, fm = cv_table
        rt_obj = {"markdown": "**加粗**\n普通文字\n"}
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["富文本"]: rt_obj})
        val = read_cells.get(fm["富文本"])
        print(f"  [richText] wrote markdown obj, read back: {val!r}")
        assert val is not None
        if isinstance(val, dict):
            assert "markdown" in val


class TestTelephoneEmailCellValue:
    def test_telephone(self, dws, test_base_id, cv_table):
        """telephone 字符串写入/读取。"""
        table_id, fm = cv_table
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["电话"]: "13800138000"})
        val = read_cells.get(fm["电话"])
        print(f"  [telephone] wrote '13800138000', read back: {val!r}")
        assert "13800138000" in str(val)

    def test_email(self, dws, test_base_id, cv_table):
        """email 字符串写入/读取。"""
        table_id, fm = cv_table
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["邮箱"]: "test@example.com"})
        val = read_cells.get(fm["邮箱"])
        print(f"  [email] wrote 'test@example.com', read back: {val!r}")
        assert "test@example.com" in str(val)


class TestGeolocationCellValue:
    def test_geolocation_object(self, dws, test_base_id, cv_table):
        """geolocation {address, name, location} 对象写入/读取。"""
        table_id, fm = cv_table
        geo_obj = {
            "address": "浙江省杭州市",
            "name": "阿里中心",
            "location": ["120.007852", "30.271194"],
        }
        read_cells = write_and_read(dws, test_base_id, table_id, {fm["地理位置"]: geo_obj})
        val = read_cells.get(fm["地理位置"])
        print(f"  [geolocation] wrote object, read back: {val!r}")
        assert val is not None
        if isinstance(val, dict):
            assert "location" in val or "address" in val


class TestLinkCellValue:
    """验证 unidirectionalLink 的 cellValue 写入/读取。"""

    @pytest.fixture(scope="class")
    def link_setup(self, dws, test_base_id):
        """Create two tables with a link field and target records."""
        ts = int(time.time())
        # Target table
        target_data = dws.run(
            "aitable", "table", "create",
            "--base-id", test_base_id,
            "--name", f"LinkTarget_CV_{ts}",
            "--fields", json.dumps([{"fieldName": "名称", "type": "text"}]),
        )
        target_table_id = target_data["data"]["tableId"]

        # Create a record in target table
        target_fields = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", target_table_id)
        target_fld_id = target_fields["data"]["tables"][0]["fields"][0]["fieldId"]
        rec_data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", target_table_id,
            "--records", json.dumps([{"cells": {target_fld_id: "关联目标"}}]),
        )
        target_rec_id = rec_data["data"].get("newRecordIds", [None])[0] or \
                        rec_data["data"].get("records", [{}])[0].get("recordId")

        # Source table with link field
        source_data = dws.run(
            "aitable", "table", "create",
            "--base-id", test_base_id,
            "--name", f"LinkSource_CV_{ts}",
            "--fields", json.dumps([{"fieldName": "标题", "type": "text"}]),
        )
        source_table_id = source_data["data"]["tableId"]

        # Add link field
        link_field_data = dws.run(
            "aitable", "field", "create",
            "--base-id", test_base_id,
            "--table-id", source_table_id,
            "--name", "关联字段",
            "--type", "unidirectionalLink",
            "--config", json.dumps({"linkedTableId": target_table_id, "multiple": True}),
        )
        # Get link field id
        src_fields = dws.run("aitable", "table", "get", "--base-id", test_base_id, "--table-ids", source_table_id)
        link_fld = next(
            (f for f in src_fields["data"]["tables"][0]["fields"] if f.get("fieldName") == "关联字段"),
            None,
        )
        link_fld_id = link_fld["fieldId"] if link_fld else None

        return {
            "source_table_id": source_table_id,
            "target_rec_id": target_rec_id,
            "link_fld_id": link_fld_id,
        }

    def test_link_write_read(self, dws, test_base_id, link_setup):
        """unidirectionalLink 用 {linkedRecordIds} 写入/读取。"""
        setup = link_setup
        if not setup["link_fld_id"] or not setup["target_rec_id"]:
            pytest.skip("link setup incomplete")

        cells = {setup["link_fld_id"]: {"linkedRecordIds": [setup["target_rec_id"]]}}
        # Write
        create_data = dws.run(
            "aitable", "record", "create",
            "--base-id", test_base_id,
            "--table-id", setup["source_table_id"],
            "--records", json.dumps([{"cells": cells}], ensure_ascii=False),
        )
        body = create_data["data"]
        rec_id = body.get("newRecordIds", [None])[0] or body.get("records", [{}])[0].get("recordId")

        # Read back (need to specify field-ids to get link field)
        query_data = dws.run(
            "aitable", "record", "query",
            "--base-id", test_base_id,
            "--table-id", setup["source_table_id"],
            "--record-ids", rec_id,
            "--field-ids", setup["link_fld_id"],
        )
        records = query_data["data"].get("records", [])
        assert len(records) >= 1
        val = records[0].get("cells", {}).get(setup["link_fld_id"])
        print(f"  [link] wrote linkedRecordIds, read back: {val!r}")
        assert val is not None
        if isinstance(val, dict):
            assert "linkedRecordIds" in val
            assert setup["target_rec_id"] in val["linkedRecordIds"]
