"""
test_92_field_properties.py — 字段类型 config 结构验证

验证 aitable-field-properties.md 中定义的每种字段类型的 config 结构是否正确。
每个测试用例创建一个字段，确认 MCP 接受该 config 并成功创建。

覆盖范围:
- number: INT/FLOAT_1/FLOAT_2/FLOAT_3/FLOAT_4/THOUSAND/THOUSAND_FLOAT/PERCENT/PERCENT_FLOAT
- singleSelect: options 结构
- multipleSelect: options 结构
- date: 5 种 formatter
- currency: currencyType + formatter
- progress: 默认 + 自定义 range
- rating: min/max/icon
- user/department/group: multiple true/false
- formula: 公式表达式
- unidirectionalLink / bidirectionalLink: linkedTableId + multiple
- 无 config 类型: text/checkbox/url/richText/telephone/email/attachment/geolocation
"""

import json
import time

import pytest


# ─── Helpers ────────────────────────────────────────────────

def create_field(dws, base_id, table_id, field_name, field_type, config=None):
    """Create a single field and return the response data."""
    args = [
        "aitable", "field", "create",
        "--base-id", base_id,
        "--table-id", table_id,
        "--name", field_name,
        "--type", field_type,
    ]
    if config:
        args += ["--config", json.dumps(config, ensure_ascii=False)]
    return dws.run(*args)


def create_field_batch(dws, base_id, table_id, fields_list):
    """Create fields via --fields batch mode."""
    return dws.run(
        "aitable", "field", "create",
        "--base-id", base_id,
        "--table-id", table_id,
        "--fields", json.dumps(fields_list, ensure_ascii=False),
    )


def assert_field_created(data, expected_name=None):
    """Assert field creation succeeded."""
    body = data.get("data", {})
    # 批量模式
    if "results" in body:
        assert body.get("successCount", 0) >= 1, f"expect at least 1 field created, got: {data}"
        if expected_name:
            names = [r.get("fieldName") for r in body["results"] if r.get("success")]
            assert expected_name in names, f"expect '{expected_name}' in created fields, got: {names}"
    # 单字段模式 (可能返回 fieldId 或 results)
    elif "fieldId" in body:
        assert body["fieldId"], f"expect fieldId, got: {data}"
    else:
        # 兜底: 没有 error 就算成功
        assert data.get("status") != "error", f"field create failed: {data}"


# ─── Fixtures ───────────────────────────────────────────────

@pytest.fixture(scope="module")
def prop_table_id(dws, test_base_id):
    """Create a dedicated table for field property tests."""
    ts = int(time.time())
    data = dws.run(
        "aitable", "table", "create",
        "--base-id", test_base_id,
        "--name", f"FieldPropTest_{ts}",
        "--fields", json.dumps([{"fieldName": "标题", "type": "text"}]),
    )
    table_id = data["data"].get("tableId")
    assert table_id, f"table create must return tableId, got: {data}"
    return table_id


@pytest.fixture(scope="module")
def linked_table_id(dws, test_base_id):
    """Create a second table for link field tests."""
    ts = int(time.time())
    data = dws.run(
        "aitable", "table", "create",
        "--base-id", test_base_id,
        "--name", f"LinkTarget_{ts}",
        "--fields", json.dumps([{"fieldName": "名称", "type": "text"}]),
    )
    table_id = data["data"].get("tableId")
    assert table_id, f"table create must return tableId, got: {data}"
    return table_id


# ─── Tests: number formatter ───────────────────────────────

class TestNumberFormatter:
    """验证 number 类型所有 formatter 值。"""

    @pytest.mark.parametrize("formatter", [
        "INT", "FLOAT_1", "FLOAT_2", "FLOAT_3", "FLOAT_4",
        "THOUSAND", "THOUSAND_FLOAT", "PERCENT", "PERCENT_FLOAT",
    ])
    def test_number_formatter(self, dws, test_base_id, prop_table_id, formatter):
        """number 字段各 formatter 均可成功创建。"""
        name = f"Num_{formatter}_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "number",
                            config={"formatter": formatter})
        assert_field_created(data, name)


# ─── Tests: singleSelect / multipleSelect ──────────────────

class TestSelectFields:
    """验证 singleSelect/multipleSelect 的 options 结构。"""

    def test_single_select_with_options(self, dws, test_base_id, prop_table_id):
        """singleSelect + options 数组可成功创建。"""
        name = f"SS_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "singleSelect",
                            config={"options": [{"name": "待办"}, {"name": "进行中"}, {"name": "已完成"}]})
        assert_field_created(data, name)

    def test_multiple_select_with_options(self, dws, test_base_id, prop_table_id):
        """multipleSelect + options 数组可成功创建。"""
        name = f"MS_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "multipleSelect",
                            config={"options": [{"name": "标签A"}, {"name": "标签B"}, {"name": "标签C"}]})
        assert_field_created(data, name)


# ─── Tests: date formatter ─────────────────────────────────

class TestDateFormatter:
    """验证 date 类型所有 formatter 值。"""

    @pytest.mark.parametrize("formatter", [
        "YYYY-MM-DD",
        "YYYY-MM-DD HH:mm",
        "YYYY-MM-DD HH:mm:ss",
        "YYYY/MM/DD",
        "YYYY/MM/DD HH:mm",
    ])
    def test_date_formatter(self, dws, test_base_id, prop_table_id, formatter):
        """date 字段各 formatter 均可成功创建。"""
        # 用 formatter 首尾字符做短名避免重名
        short = formatter.replace("/", "s").replace(":", "c").replace(" ", "_")[:10]
        name = f"Date_{short}_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "date",
                            config={"formatter": formatter})
        assert_field_created(data, name)


# ─── Tests: currency ───────────────────────────────────────

class TestCurrencyField:
    """验证 currency 类型的 currencyType + formatter。"""

    @pytest.mark.parametrize("currency_type", [
        "CNY", "USD", "EUR", "JPY", "GBP", "HKD",
    ])
    def test_currency_types(self, dws, test_base_id, prop_table_id, currency_type):
        """常用 currencyType 均可成功创建。"""
        name = f"Cur_{currency_type}_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "currency",
                            config={"currencyType": currency_type, "formatter": "FLOAT_2"})
        assert_field_created(data, name)

    def test_currency_int_formatter(self, dws, test_base_id, prop_table_id):
        """currency + INT formatter（无小数位）。"""
        name = f"Cur_INT_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "currency",
                            config={"currencyType": "CNY", "formatter": "INT"})
        assert_field_created(data, name)


# ─── Tests: progress ───────────────────────────────────────

class TestProgressField:
    """验证 progress 类型 config。"""

    def test_progress_default(self, dws, test_base_id, prop_table_id):
        """progress 默认 config（formatter=PERCENT）。"""
        name = f"Prog_Def_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "progress",
                            config={"formatter": "PERCENT"})
        assert_field_created(data, name)

    def test_progress_custom_range(self, dws, test_base_id, prop_table_id):
        """progress 自定义 range（customizeRange=true, min=0, max=1）。"""
        name = f"Prog_Custom_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "progress",
                            config={"formatter": "PERCENT", "customizeRange": True, "min": 0, "max": 1})
        assert_field_created(data, name)


# ─── Tests: rating ─────────────────────────────────────────

class TestRatingField:
    """验证 rating 类型 config。"""

    def test_rating_default(self, dws, test_base_id, prop_table_id):
        """rating 默认 config（min=1, max=5, icon=star）。"""
        name = f"Rate_Def_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "rating",
                            config={"min": 1, "max": 5, "icon": "star"})
        assert_field_created(data, name)

    def test_rating_max_10(self, dws, test_base_id, prop_table_id):
        """rating max=10 边界。"""
        name = f"Rate_Max10_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "rating",
                            config={"min": 1, "max": 10, "icon": "star"})
        assert_field_created(data, name)


# ─── Tests: user / department / group ──────────────────────

class TestPersonFields:
    """验证 user/department/group 的 multiple config。"""

    def test_user_single(self, dws, test_base_id, prop_table_id):
        """user multiple=false。"""
        name = f"User_S_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "user",
                            config={"multiple": False})
        assert_field_created(data, name)

    def test_user_multiple(self, dws, test_base_id, prop_table_id):
        """user multiple=true（默认）。"""
        name = f"User_M_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "user",
                            config={"multiple": True})
        assert_field_created(data, name)

    def test_department_multiple(self, dws, test_base_id, prop_table_id):
        """department multiple=true。"""
        name = f"Dept_M_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "department",
                            config={"multiple": True})
        assert_field_created(data, name)

    def test_group_single(self, dws, test_base_id, prop_table_id):
        """group multiple=false。"""
        name = f"Group_S_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "group",
                            config={"multiple": False})
        assert_field_created(data, name)


# ─── Tests: formula ────────────────────────────────────────

class TestFormulaField:
    """验证 formula 类型 config。"""

    def test_formula_simple(self, dws, test_base_id, prop_table_id):
        """formula 字段可成功创建（简单表达式）。"""
        name = f"Formula_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "formula",
                            config={"formula": "[标题]"})
        assert_field_created(data, name)


# ─── Tests: link fields ────────────────────────────────────

class TestLinkFields:
    """验证 unidirectionalLink / bidirectionalLink config。"""

    def test_unidirectional_link(self, dws, test_base_id, prop_table_id, linked_table_id):
        """unidirectionalLink + linkedTableId + multiple=true。"""
        name = f"UniLink_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "unidirectionalLink",
                            config={"linkedTableId": linked_table_id, "multiple": True})
        assert_field_created(data, name)

    def test_bidirectional_link(self, dws, test_base_id, prop_table_id, linked_table_id):
        """bidirectionalLink + linkedTableId + multiple=true。"""
        name = f"BiLink_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, "bidirectionalLink",
                            config={"linkedTableId": linked_table_id, "multiple": True})
        assert_field_created(data, name)


# ─── Tests: no-config types ────────────────────────────────

class TestNoConfigTypes:
    """验证不需要 config 的字段类型均可无 config 创建成功。"""

    @pytest.mark.parametrize("field_type", [
        "text", "checkbox", "url", "richText",
        "telephone", "email", "attachment", "geolocation",
    ])
    def test_no_config_field(self, dws, test_base_id, prop_table_id, field_type):
        """无 config 字段类型创建成功。"""
        name = f"NC_{field_type}_{int(time.time()) % 10000}"
        data = create_field(dws, test_base_id, prop_table_id, name, field_type)
        assert_field_created(data, name)
