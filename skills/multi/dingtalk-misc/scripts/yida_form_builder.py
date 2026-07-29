"""
宜搭表单 schema 构造 — 字段组件装配进 Page > FormContainer 骨架。

主入口:
  build_form_schema(form_title, fields, form_uuid, corp_id, app_type)
  apply_changes_to_schema(schema, changes) — 增量操作
"""
from __future__ import annotations

import copy
import json
import re
from typing import Any, Optional

from yida_schema_common import (
    build_components_map,
    collect_component_names,
    DATA_SOURCE_FIT_COMPILED,
    DATA_SOURCE_FIT_SOURCE,
    generate_field_id,
    i18n,
    next_node_id,
    normalize_field_type,
    UTILS_LEGAO_BUILTIN,
    UTILS_YIDA_PLUGIN,
)
from yida_form_fields import build_field_component


# ---------------------------------------------------------------------------
# 骨架常量
# ---------------------------------------------------------------------------

_FORM_ACTIONS = {
    "module": {
        "source": (
            '/**\n* 尊敬的用户，你好：页面 JS 面板是高阶用法。\n*/\n\n'
            'export function didMount() {\n'
            '  console.log(`「页面 JS」：当前页面地址 ${location.href}`);\n'
            '}'
        ),
        "compiled": (
            '"use strict";\n\nexports.__esModule = true;\nexports.didMount = didMount;\n'
            'function didMount() {\n'
            '  console.log("\\u300C\\u9875\\u9762 JS\\u300D\\uFF1A\\u5F53\\u524D\\u9875\\u9762\\u5730\\u5740 " + location.href);\n'
            '}\n'
        ),
    },
    "type": "FUNCTION",
    "list": [{"id": "didMount", "title": "didMount"}],
}


_CONSTRUCTOR_SOURCE = (
    "function constructor() {\n"
    "var module = { exports: {} };\n"
    "var _this = this;\n"
    "this.__initMethods__(module.exports, module);\n"
    "Object.keys(module.exports).forEach(function(item) {\n"
    "  if(typeof module.exports[item] === 'function'){\n"
    "    _this[item] = module.exports[item];\n"
    "  }\n"
    "});\n\n"
    "}"
)


# ---------------------------------------------------------------------------
# 主入口
# ---------------------------------------------------------------------------


def build_form_schema(
    form_title: str,
    fields: list[dict[str, Any]],
    form_uuid: str = "",
    corp_id: str = "",
    app_type: str = "",
    *,
    label_align: str = "top",
) -> dict[str, Any]:
    """
    构造完整的表单 schema。

    Args:
        form_title: 表单标题
        fields: 字段定义数组（每项含 type + label + ...）
        form_uuid: 表单 UUID
        corp_id: 企业 ID（流水号需要）
        app_type: 应用编码
        label_align: 标签对齐方式 top/left
    """
    # 1. 构造所有字段节点
    field_nodes: list[dict[str, Any]] = []
    used_colors: set[str] = set()
    for field in fields:
        node, used_colors = build_field_component(
            field,
            used_colors=used_colors,
            app_type=app_type,
            form_uuid=form_uuid,
            corp_id=corp_id,
        )
        field_nodes.append(node)

    # 2. 后处理：解析 @label: 引用
    _resolve_field_id_references(field_nodes)

    # 3. 后处理：补全流水号 formula（此时 corp_id/app_type/form_uuid 确定）
    _fill_serial_number_formulas(field_nodes, corp_id, app_type, form_uuid)

    # 4. 收集 componentsMap
    all_component_names = ["Page", "RootHeader", "RootContent", "RootFooter", "FormContainer"]
    all_component_names.extend(collect_component_names(field_nodes))
    components_map = build_components_map(all_component_names)

    # 5. 拼装骨架
    schema: dict[str, Any] = {
        "schemaType": "superform",
        "schemaVersion": "5.0",
        "pages": [
            {
                "utils": [UTILS_LEGAO_BUILTIN, UTILS_YIDA_PLUGIN],
                "componentsMap": components_map,
                "componentsTree": [
                    {
                        "componentName": "Page",
                        "id": next_node_id(),
                        "props": {
                            "templateVersion": "1.0.0",
                            "pageStyle": {"backgroundColor": "#f2f3f5"},
                            "titleName": i18n(form_title),
                            "titleDesc": i18n(""),
                            "titleColor": "light",
                            "titleBg": "https://img.alicdn.com/imgextra/i2/O1CN0143ATPP1wIa9TrVvzN_!!6000000006285-2-tps-3360-400.png_.webp",
                            "backgroundColorCustom": "#f1f2f3",
                            "sizePc": "medium",
                            "labelAlignPc": label_align,
                            "labelWidthPc": "130px",
                            "labelWeightPc": "normal",
                            "contentMargin": "12",
                            "contentPadding": "20",
                            "contentBgColor": "white",
                            "showTitle": True,
                            "labelAlignMobile": "left",
                            "labelWidthMobile": "100px",
                            "labelWeightMobile": "bold",
                            "contentMarginMobile": "12",
                            "contentPaddingMobile": "0",
                            "contentBgColorMobile": "white",
                            "className": "page_m8o991i5",
                        },
                        "dataSource": {
                            "offline": [],
                            "globalConfig": {
                                "fit": {
                                    "compiled": DATA_SOURCE_FIT_COMPILED,
                                    "source": DATA_SOURCE_FIT_SOURCE,
                                    "type": "js",
                                    "error": {},
                                },
                            },
                            "online": [],
                            "list": [],
                            "sync": True,
                        },
                        "methods": {
                            "__initMethods__": {
                                "type": "js",
                                "source": "function (exports, module) { /*set actions code here*/ }",
                                "compiled": "function (exports, module) { /*set actions code here*/ }",
                            },
                        },
                        "lifeCycles": {
                            "componentDidMount": {
                                "id": "didMount",
                                "name": "didMount",
                                "params": {},
                                "type": "actionRef",
                            },
                            "componentWillUnmount": "",
                            "constructor": {
                                "type": "js",
                                "compiled": _CONSTRUCTOR_SOURCE,
                                "source": _CONSTRUCTOR_SOURCE,
                            },
                        },
                        "hidden": False,
                        "title": "",
                        "isLocked": False,
                        "condition": True,
                        "conditionGroup": "",
                        "children": [
                            {
                                "componentName": "RootHeader",
                                "id": next_node_id(),
                                "props": {},
                                "hidden": False,
                                "title": "",
                                "isLocked": False,
                                "condition": True,
                                "conditionGroup": "",
                            },
                            {
                                "componentName": "RootContent",
                                "id": next_node_id(),
                                "props": {},
                                "hidden": False,
                                "title": "",
                                "isLocked": False,
                                "condition": True,
                                "conditionGroup": "",
                                "children": [
                                    {
                                        "componentName": "FormContainer",
                                        "id": next_node_id(),
                                        "props": {
                                            "columns": 1,
                                            "labelAlign": label_align,
                                            "submitText": i18n("提交", "Submit"),
                                            "fieldId": generate_field_id("formContainer"),
                                            "aiFormConfig": {
                                                "systemPrompt": "",
                                                "model": "qwen",
                                            },
                                            "beforeSubmit": False,
                                            "afterSubmit": False,
                                        },
                                        "hidden": False,
                                        "title": "",
                                        "isLocked": False,
                                        "condition": True,
                                        "conditionGroup": "",
                                        "children": field_nodes,
                                    },
                                ],
                            },
                            {
                                "componentName": "RootFooter",
                                "id": next_node_id(),
                                "props": {},
                                "hidden": False,
                                "title": "",
                                "isLocked": False,
                                "condition": True,
                                "conditionGroup": "",
                            },
                        ],
                        "css": "body{background-color:#f2f3f5}",
                    },
                ],
                "componentAlias": {"items": []},
                "id": form_uuid or "xxxx",
                "connectComponent": [],
            },
        ],
        "actions": copy.deepcopy(_FORM_ACTIONS),
        "config": {"connectComponent": []},
    }

    return schema


# ---------------------------------------------------------------------------
# 增量修改
# ---------------------------------------------------------------------------


def apply_changes_to_schema(
    schema: dict[str, Any],
    changes: list[dict[str, Any]],
    *,
    corp_id: str = "",
    app_type: str = "",
    form_uuid: str = "",
) -> dict[str, Any]:
    """
    在已有 schema 上执行增量 changes（add/update/delete）。

    返回修改后的 schema（原地修改）。
    """
    form_container = _find_form_container(schema)
    if form_container is None:
        raise ValueError("schema 中找不到 FormContainer 节点")

    children: list[dict[str, Any]] = form_container.get("children", [])
    used_colors = _collect_existing_colors(children)

    for change in changes:
        action = change.get("action", "")
        if action == "add":
            field_def = change.get("field", {})
            node, used_colors = build_field_component(
                field_def,
                used_colors=used_colors,
                app_type=app_type,
                form_uuid=form_uuid,
                corp_id=corp_id,
            )
            after_label = change.get("after")
            before_label = change.get("before")
            insert_idx = len(children)

            if after_label:
                idx = _find_field_index_by_label(children, after_label)
                if idx is not None:
                    insert_idx = idx + 1
            elif before_label:
                idx = _find_field_index_by_label(children, before_label)
                if idx is not None:
                    insert_idx = idx

            children.insert(insert_idx, node)

        elif action == "update":
            label = change.get("label", "")
            table_label = change.get("tableLabel")
            patches = change.get("changes", {})

            target_list = children
            if table_label:
                table_node = _find_field_by_label(children, table_label)
                if table_node and table_node.get("componentName") == "TableField":
                    target_list = table_node.get("children", [])

            target = _find_field_by_label(target_list, label)
            if target:
                _apply_field_patches(target, patches, used_colors)

        elif action == "delete":
            label = change.get("label", "")
            table_label = change.get("tableLabel")

            target_list = children
            if table_label:
                table_node = _find_field_by_label(children, table_label)
                if table_node and table_node.get("componentName") == "TableField":
                    target_list = table_node.get("children", [])

            idx = _find_field_index_by_label(target_list, label)
            if idx is not None:
                target_list.pop(idx)

    # 后处理
    all_fields = form_container.get("children", [])
    _resolve_field_id_references(all_fields)
    _fill_serial_number_formulas(all_fields, corp_id, app_type, form_uuid)

    # 更新 componentsMap
    all_names = ["Page", "RootHeader", "RootContent", "RootFooter", "FormContainer"]
    all_names.extend(collect_component_names(all_fields))
    schema["pages"][0]["componentsMap"] = build_components_map(all_names)

    return schema


# ---------------------------------------------------------------------------
# 后处理
# ---------------------------------------------------------------------------


def _resolve_field_id_references(field_nodes: list[dict[str, Any]]) -> None:
    """解析 @label:字段名 引用为真实 fieldId。"""
    label_to_field_id: dict[str, str] = {}

    def _collect(nodes: list[dict[str, Any]]) -> None:
        for node in nodes:
            props = node.get("props", {})
            label_obj = props.get("label", {})
            label_text = label_obj.get("zh_CN", "") if isinstance(label_obj, dict) else str(label_obj)
            field_id = props.get("fieldId", "")
            if label_text and field_id:
                label_to_field_id[label_text] = field_id
            for child in node.get("children", []):
                _collect([child])

    _collect(field_nodes)

    def _resolve(nodes: list[dict[str, Any]]) -> None:
        for node in nodes:
            props = node.get("props", {})
            filling_rules = props.get("dataFillingRules", {})
            if isinstance(filling_rules, dict):
                main_rules = filling_rules.get("mainRules", [])
                for rule in main_rules:
                    for key in ("source", "sourceFieldId", "target", "targetFieldId"):
                        val = rule.get(key, "")
                        if isinstance(val, str) and val.startswith("@label:"):
                            name = val[7:]
                            if name in label_to_field_id:
                                rule[key] = label_to_field_id[name]
            for child in node.get("children", []):
                _resolve([child])

    _resolve(field_nodes)


def _fill_serial_number_formulas(
    field_nodes: list[dict[str, Any]],
    corp_id: str,
    app_type: str,
    form_uuid: str,
) -> None:
    """确保 SerialNumberField 的 formula 包含正确的 corp_id/app_type/form_uuid。"""
    if not corp_id or not app_type or not form_uuid:
        return

    def _walk(nodes: list[dict[str, Any]]) -> None:
        for node in nodes:
            if node.get("componentName") == "SerialNumberField":
                props = node.get("props", {})
                field_id = props.get("fieldId", "")
                serial_rule = props.get("serialNumberRule", [])
                if serial_rule and field_id:
                    rule_json = json.dumps({"type": "custom", "value": serial_rule}).replace('"', '\\"')
                    props["formula"] = {
                        "expression": f'SERIALNUMBER("{corp_id}", "{app_type}", "{form_uuid}", "{field_id}", "{rule_json}")'
                    }
            for child in node.get("children", []):
                _walk([child])

    _walk(field_nodes)


# ---------------------------------------------------------------------------
# 辅助函数
# ---------------------------------------------------------------------------


def _find_form_container(schema: dict[str, Any]) -> Optional[dict[str, Any]]:
    pages = schema.get("pages", [])
    if not pages:
        return None
    tree = pages[0].get("componentsTree", [])
    if not tree:
        return None

    def _search(node: dict[str, Any]) -> Optional[dict[str, Any]]:
        if node.get("componentName") == "FormContainer":
            return node
        for child in node.get("children", []):
            found = _search(child)
            if found:
                return found
        return None

    return _search(tree[0])


def _find_field_by_label(nodes: list[dict[str, Any]], label: str) -> Optional[dict[str, Any]]:
    for node in nodes:
        props = node.get("props", {})
        label_obj = props.get("label", {})
        label_text = label_obj.get("zh_CN", "") if isinstance(label_obj, dict) else str(label_obj)
        if label_text == label:
            return node
    return None


def _find_field_index_by_label(nodes: list[dict[str, Any]], label: str) -> Optional[int]:
    for idx, node in enumerate(nodes):
        props = node.get("props", {})
        label_obj = props.get("label", {})
        label_text = label_obj.get("zh_CN", "") if isinstance(label_obj, dict) else str(label_obj)
        if label_text == label:
            return idx
    return None


def _collect_existing_colors(nodes: list[dict[str, Any]]) -> set[str]:
    colors: set[str] = set()
    for node in nodes:
        props = node.get("props", {})
        for item in props.get("dataSource", []):
            c = item.get("color")
            if c:
                colors.add(c)
        for child in node.get("children", []):
            colors.update(_collect_existing_colors([child]))
    return colors


def _apply_field_patches(node: dict[str, Any], patches: dict[str, Any], used_colors: set[str]) -> None:
    props = node.get("props", {})

    if "label" in patches:
        props["label"] = i18n(patches["label"])
    if "required" in patches:
        validation = props.get("validation", [])
        has_req = any(v.get("type") == "required" for v in validation)
        if patches["required"] and not has_req:
            validation.append({"type": "required"})
            props["validation"] = validation
        elif not patches["required"] and has_req:
            props["validation"] = [v for v in validation if v.get("type") != "required"]
    if "behavior" in patches:
        props["behavior"] = patches["behavior"]
    if "options" in patches:
        from yida_schema_common import build_option_data_source
        component_name = node.get("componentName", "")
        is_checkbox = component_name in ("CheckboxField", "MultiSelectField")
        ds, _ = build_option_data_source(patches["options"], is_checkbox=is_checkbox, used_colors=used_colors)
        props["dataSource"] = ds
        props["isUseDataSourceColor"] = True
    if "placeholder" in patches:
        props["placeholder"] = i18n(patches["placeholder"])
    if "suffix" in patches:
        props["innerAfter"] = i18n(patches["suffix"])
    if "prefix" in patches:
        props["innerBefore"] = i18n(patches["prefix"])
    if "format" in patches:
        props["format"] = patches["format"]
    if "multiple" in patches or "multi" in patches:
        val = patches.get("multiple") or patches.get("multi")
        props["multiple"] = val
        if val:
            props["mode"] = "multiple"


def is_empty_skeleton(schema: dict[str, Any]) -> bool:
    """判断 schema 是否是空骨架（无字段）。"""
    fc = _find_form_container(schema)
    if fc is None:
        return True
    children = fc.get("children", [])
    return len(children) == 0
