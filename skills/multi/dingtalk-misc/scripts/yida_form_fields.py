"""
宜搭表单字段构造 — 按 type 分发，每种字段类型产出一个 component dict。

支持 19 种字段类型 + Divider。被 yida_form_builder.py 调用。
"""
from __future__ import annotations

import json
from typing import Any, Optional

from yida_schema_common import (
    generate_field_id,
    i18n,
    next_node_id,
    normalize_field_type,
    build_option_data_source,
    OPTION_FIELD_TYPES,
    SUPPORTED_FIELD_TYPES,
)


def build_field_component(
    field: dict[str, Any],
    *,
    used_colors: Optional[set[str]] = None,
    app_type: str = "",
    form_uuid: str = "",
    corp_id: str = "",
) -> tuple[dict[str, Any], set[str]]:
    """
    根据字段定义 dict 构造一个组件节点。

    返回 (component_node, updated_used_colors)。
    """
    if used_colors is None:
        used_colors = set()

    raw_type = field.get("type", "")
    component_name = normalize_field_type(raw_type)
    label = field.get("label", "")
    required = field.get("required", False)
    field_id = generate_field_id(component_name)

    node: dict[str, Any] = {
        "componentName": component_name,
        "id": next_node_id(),
        "props": {
            "label": i18n(label),
            "fieldId": field_id,
            "__category__": "form",
            "behavior": field.get("behavior", "NORMAL"),
            "visibility": ["PC", "MOBILE"],
            "submittable": "DEFAULT",
        },
    }

    props = node["props"]

    # required
    if required:
        props["validation"] = [{"type": "required"}]

    # placeholder
    if field.get("placeholder"):
        props["placeholder"] = i18n(field["placeholder"])

    # defaultValue
    if field.get("defaultValue") is not None:
        props["defaultValue"] = field["defaultValue"]

    # --- 按类型分发 ---
    if component_name == "TextareaField":
        _apply_textarea_props(props, field)
    elif component_name == "NumberField":
        _apply_number_props(props, field)
    elif component_name == "RateField":
        _apply_rate_props(props, field)
    elif component_name in ("DateField", "CascadeDateField"):
        _apply_date_props(props, field, component_name)
    elif component_name in OPTION_FIELD_TYPES:
        used_colors = _apply_option_props(props, field, component_name, used_colors)
    elif component_name in ("EmployeeField", "DepartmentSelectField"):
        _apply_people_props(props, field)
    elif component_name == "CountrySelectField":
        _apply_people_props(props, field)
    elif component_name == "AddressField":
        _apply_address_props(props, field)
    elif component_name == "AttachmentField":
        _apply_attachment_props(props, field)
    elif component_name == "ImageField":
        _apply_image_props(props, field)
    elif component_name == "SerialNumberField":
        _apply_serial_number_props(props, field, app_type, form_uuid, field_id, corp_id)
    elif component_name == "TableField":
        used_colors = _apply_table_props(node, field, used_colors, app_type, form_uuid, corp_id)
    elif component_name == "AssociationFormField":
        _apply_association_props(props, field, app_type)
    elif component_name == "Divider":
        _apply_divider_props(props, field, label)

    return node, used_colors


# ---------------------------------------------------------------------------
# 各类型 props 应用
# ---------------------------------------------------------------------------


def _apply_textarea_props(props: dict, field: dict) -> None:
    props["rows"] = field.get("rows", 4)
    props["htmlType"] = "textarea"


def _apply_number_props(props: dict, field: dict) -> None:
    fmt = field.get("format", "INT")
    if fmt == "FLOAT":
        props["precision"] = field.get("precision", 2)
        props["format"] = "money_w4"
    elif fmt == "PERCENT":
        props["precision"] = field.get("precision", 2)
        props["format"] = "percent"
    else:
        props["precision"] = 0
        props["format"] = "integer"

    if field.get("suffix"):
        props["innerAfter"] = i18n(field["suffix"])
    if field.get("prefix"):
        props["innerBefore"] = i18n(field["prefix"])
    if field.get("min") is not None:
        props["min"] = field["min"]
    if field.get("max") is not None:
        props["max"] = field["max"]


def _apply_rate_props(props: dict, field: dict) -> None:
    props["count"] = field.get("total", 5)
    if field.get("allowHalf"):
        props["allowHalf"] = True


def _apply_date_props(props: dict, field: dict, component_name: str) -> None:
    fmt = field.get("format", "yyyy-MM-dd")
    props["format"] = fmt
    if "HH" in fmt:
        props["showTime"] = True


def _apply_option_props(
    props: dict,
    field: dict,
    component_name: str,
    used_colors: set[str],
) -> set[str]:
    options = field.get("options", [])
    if not options:
        return used_colors

    is_checkbox = component_name in ("CheckboxField", "MultiSelectField")
    props["isUseDataSourceColor"] = True
    data_source, used_colors = build_option_data_source(
        options, is_checkbox=is_checkbox, used_colors=used_colors
    )
    props["dataSource"] = data_source
    if not is_checkbox and data_source:
        props["value"] = data_source[0]["value"]
    return used_colors


def _apply_people_props(props: dict, field: dict) -> None:
    multi = field.get("multi") or field.get("multiple")
    if multi:
        props["multiple"] = True
        props["mode"] = "multiple"


def _apply_address_props(props: dict, field: dict) -> None:
    level = field.get("level", "ADDRESS")
    props["addressType"] = level


def _apply_attachment_props(props: dict, field: dict) -> None:
    props["autoUpload"] = True
    props["maxFileSize"] = field.get("maxFileSize", 100)
    if field.get("maxFiles"):
        props["maxItems"] = field["maxFiles"]
    if field.get("fileTypes"):
        props["accept"] = field["fileTypes"]


def _apply_image_props(props: dict, field: dict) -> None:
    props["autoUpload"] = True
    if field.get("maxFiles"):
        props["maxItems"] = field["maxFiles"]


def _apply_serial_number_props(
    props: dict,
    field: dict,
    app_type: str,
    form_uuid: str,
    field_id: str,
    corp_id: str,
) -> None:
    # 流水号不允许 required
    if "validation" in props:
        props["validation"] = [v for v in props["validation"] if v.get("type") != "required"]

    serial_rule = field.get("serialNumberRule") or [
        {
            "__hide_delete__": False,
            "ruleType": "date",
            "content": "",
            "formField": "",
            "dateFormat": "yyyyMMdd",
            "timeZone": "+8",
            "digitCount": 4,
            "isFixed": True,
            "isFixedTips": "",
            "resetPeriod": "noClean",
            "resetPeriodTips": "",
            "initialValue": 1,
        },
        {
            "__hide_delete__": True,
            "ruleType": "autoCount",
            "content": "",
            "formField": "",
            "dateFormat": "yyyyMMdd",
            "timeZone": "+8",
            "digitCount": "4",
            "isFixed": True,
            "isFixedTips": "",
            "resetPeriod": "noClean",
            "resetPeriodTips": "",
            "initialValue": 1,
        },
    ]
    props["serialNumberRule"] = serial_rule

    serial_rule_json = json.dumps({"type": "custom", "value": serial_rule}).replace('"', '\\"')
    props["formula"] = {
        "expression": f'SERIALNUMBER("{corp_id}", "{app_type}", "{form_uuid}", "{field_id}", "{serial_rule_json}")'
    }


def _apply_table_props(
    node: dict,
    field: dict,
    used_colors: set[str],
    app_type: str,
    form_uuid: str,
    corp_id: str,
) -> set[str]:
    props = node["props"]
    props["layout"] = "TABLE"
    props["mobileLayout"] = "TILED"
    props["maxItems"] = field.get("maxItems", 50)

    children_defs = field.get("children", [])
    children_nodes: list[dict[str, Any]] = []
    for child_field in children_defs:
        child_node, used_colors = build_field_component(
            child_field,
            used_colors=used_colors,
            app_type=app_type,
            form_uuid=form_uuid,
            corp_id=corp_id,
        )
        children_nodes.append(child_node)

    node["children"] = children_nodes
    return used_colors


def _apply_association_props(props: dict, field: dict, app_type: str) -> None:
    source_app = field.get("sourceApp", app_type)
    source_form = field.get("sourceForm", "")
    display_field_code = field.get("displayFieldCode", "")

    if source_form:
        props["associationForm"] = {
            "appType": source_app,
            "formUuid": source_form,
            "formType": "receipt",
            "formTitle": "",
            "mainFieldId": display_field_code,
            "mainComponentName": "TextField",
            "mainFieldLabel": "",
        }
        props["dataFilterRules"] = {"instanceFieldId": None}

    filling_rules = field.get("dataFillingRules", [])
    if filling_rules:
        normalized: list[dict[str, Any]] = []
        for rule in filling_rules:
            src = rule.get("source", "")
            tgt = rule.get("target", "")
            normalized.append({
                "source": src,
                "sourceFieldId": src,
                "sourceType": rule.get("sourceType", "form"),
                "target": tgt,
                "targetFieldId": tgt,
                "targetType": rule.get("targetType", "form"),
            })
        props["dataFillingRules"] = {"mainRules": normalized}


def _apply_divider_props(props: dict, field: dict, label: str) -> None:
    props["title"] = i18n(label)
    props["type"] = field.get("dividerType", "multi-parallelograms-end")
    props.pop("validation", None)
    props["behavior"] = "NORMAL"
    props.pop("__category__", None)
