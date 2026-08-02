"""
宜搭报表 schema 构造 — 多 chart 装配进 Page > RootContent 栅格布局。

主入口:
  build_report_schema_with_filters(report_title, charts, filters, report_id, corp_id)
  apply_chart_changes_to_schema(schema, changes, cube_tenant_id)
"""
from __future__ import annotations

import copy
from typing import Any, Optional

from yida_schema_common import (
    build_report_components_map,
    DATA_SOURCE_FIT_COMPILED,
    DATA_SOURCE_FIT_SOURCE,
    i18n,
    next_node_id,
    generate_field_id,
)
from yida_report_charts import (
    build_chart_component,
    CHART_TYPE_MAP,
    normalize_cube_code,
    normalize_field_code,
)

# ---------------------------------------------------------------------------
# 报表骨架常量
# ---------------------------------------------------------------------------

_PAGE_STYLE = ":root {\n  background-color: #f2f3f5;\n}\n"

_PAGE_PROPS = {
    "pageStyle": _PAGE_STYLE,
    "containerStyle": {},
    "userVariables": [
        {"text": "工号", "id": "varWorkNo"},
        {"text": "部门名称", "id": "varDeptName"},
        {"text": "所属公司编号", "id": "varCorpNo"},
        {"text": "部门编码", "id": "varDeptNo"},
    ],
    "templateVersion": "1.0.0",
    "className": "page_m9o7d9ml",
    "params": [],
}

_INIT_METHODS = {
    "__initMethods__": {
        "type": "js",
        "source": "function (exports, module) { /*set actions code here*/ }",
        "compiled": "function (exports, module) { /*set actions code here*/ }",
    }
}

_CONSTRUCTOR = {
    "type": "js",
    "compiled": (
        "function constructor() {\n"
        "var module = { exports: {} };\n"
        "var _this = this;\n"
        "this.__initMethods__(module.exports, module);\n"
        "Object.keys(module.exports).forEach(function(item) {\n"
        "  if(typeof module.exports[item] === 'function'){\n"
        "    _this[item] = module.exports[item];\n"
        "  }\n"
        "});\n"
        "}"
    ),
    "source": (
        "function constructor() {\n"
        "var module = { exports: {} };\n"
        "var _this = this;\n"
        "this.__initMethods__(module.exports, module);\n"
        "Object.keys(module.exports).forEach(function(item) {\n"
        "  if(typeof module.exports[item] === 'function'){\n"
        "    _this[item] = module.exports[item];\n"
        "  }\n"
        "});\n"
        "}"
    ),
}


# ---------------------------------------------------------------------------
# 布局计算
# ---------------------------------------------------------------------------


def _next_layout_position(layout: list[dict[str, Any]], w: int) -> tuple[int, int]:
    """Calculate next (x, y) for a new item in the 6-column react-grid-layout grid.

    Finds the last row, checks if ``w`` fits to the right of existing items;
    if yes returns (right_edge, row_y), otherwise wraps to a new row.
    """
    if not layout:
        return 0, 0
    max_y = max(li.get("y", 0) for li in layout)
    last_row = [li for li in layout if li.get("y", 0) == max_y]
    last_row_right = max(li.get("x", 0) + li.get("w", 0) for li in last_row)
    last_row_h = max(li.get("h", 0) for li in last_row)
    if last_row_right + w <= 6:
        return last_row_right, max_y
    return 0, max_y + last_row_h


# ---------------------------------------------------------------------------
# 主入口
# ---------------------------------------------------------------------------


def build_report_schema_with_filters(
    report_title: str,
    charts: list[dict[str, Any]],
    filters: Optional[list[dict[str, Any]]] = None,
    report_id: str = "",
    corp_id: str = "",
) -> dict[str, Any]:
    """构造完整报表 schema（含图表 + 筛选器）。"""
    if filters is None:
        filters = []

    # 构造所有图表节点
    chart_nodes: list[dict[str, Any]] = []
    chart_field_ids: list[str] = []
    layout_items: list[dict[str, Any]] = []
    component_names: set[str] = {"Page", "RootHeader", "RootContent", "RootFooter"}

    x = 0
    y = 0
    row_max_h = 0
    for chart in charts:
        node, field_id, default_layout = build_chart_component(chart, cube_tenant_id=corp_id)
        chart_nodes.append(node)
        chart_field_ids.append(field_id)
        component_names.add(node["componentName"])

        w = default_layout["w"]
        h = default_layout["h"]
        if x + w > 6:
            x = 0
            y += row_max_h
            row_max_h = 0
        item: dict[str, Any] = {"i": field_id, "x": x, "y": y, "w": w, "h": h,
                                 "moved": False, "static": False}
        for k in ("minH", "maxH", "resizeHandles"):
            if k in default_layout:
                item[k] = default_layout[k]
        layout_items.append(item)
        row_max_h = max(row_max_h, h)
        x += w
        if x >= 6:
            x = 0
            y += row_max_h
            row_max_h = 0

    # 筛选器组件
    filter_nodes: list[dict[str, Any]] = []
    for flt in filters:
        f_node = _build_select_filter(flt, corp_id, chart_nodes)
        if f_node:
            filter_nodes.append(f_node)
            component_names.add("YoushuSelect")

    # 筛选器 layout（顶部一行）
    if filter_nodes:
        filter_layout: list[dict[str, Any]] = []
        fx = 0
        for fn in filter_nodes:
            fn_field_id = fn.get("props", {}).get("fieldId", fn["id"])
            filter_layout.append({"i": fn_field_id, "x": fx, "y": 0, "w": 2, "h": 2})
            fx += 2
        # 把图表 layout 的 y 下移
        offset_y = 2
        for li in layout_items:
            li["y"] += offset_y
        layout_items = filter_layout + layout_items

    all_children = filter_nodes + chart_nodes
    components_map = build_report_components_map(sorted(component_names))

    schema: dict[str, Any] = {
        "schemaType": "superform",
        "schemaVersion": "5.0",
        "pages": [
            {
                "utils": [],
                "componentsMap": components_map,
                "componentsTree": [
                    {
                        "componentName": "Page",
                        "id": next_node_id(),
                        "props": copy.deepcopy(_PAGE_PROPS),
                        "css": "body {\n  background-color: #f2f3f5;\n}\n",
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
                        "methods": copy.deepcopy(_INIT_METHODS),
                        "lifeCycles": {
                            "constructor": copy.deepcopy(_CONSTRUCTOR),
                        },
                        "children": [
                            {
                                "componentName": "RootHeader",
                                "id": next_node_id(),
                                "props": {},
                            },
                            {
                                "componentName": "RootContent",
                                "id": next_node_id(),
                                "props": {
                                    "layout": layout_items,
                                    "rglSwitch": True,
                                    "contentBgColor": "transparent",
                                },
                                "children": all_children,
                            },
                            {
                                "componentName": "RootFooter",
                                "id": next_node_id(),
                                "props": {},
                            },
                        ],
                    },
                ],
                "id": report_id or next_node_id(),
                "connectComponent": [],
            },
        ],
        "actions": {
            "module": {"source": "", "compiled": ""},
            "list": [],
        },
    }

    return schema


# ---------------------------------------------------------------------------
# 增量修改
# ---------------------------------------------------------------------------


def apply_chart_changes_to_schema(
    schema: dict[str, Any],
    changes: list[dict[str, Any]],
    cube_tenant_id: str = "",
) -> dict[str, Any]:
    """增量修改报表 schema（add/remove/replace/update-props）。"""
    root_content = _find_root_content(schema)
    if root_content is None:
        raise ValueError("schema 中找不到 RootContent")

    children: list[dict[str, Any]] = root_content.get("children", [])
    layout: list[dict[str, Any]] = root_content.get("props", {}).get("layout", [])

    for change in changes:
        action = change.get("action", "")

        if action == "add":
            chart_def = change.get("chart", {})
            node, field_id, default_layout = build_chart_component(chart_def, cube_tenant_id=cube_tenant_id)

            after_title = change.get("after")
            insert_idx = len(children)
            if after_title:
                idx = _find_chart_index_by_title(children, after_title)
                if idx is not None:
                    insert_idx = idx + 1

            children.insert(insert_idx, node)
            nx, ny = _next_layout_position(layout, default_layout["w"])
            item: dict[str, Any] = {"i": field_id, "x": nx, "y": ny,
                                     "w": default_layout["w"], "h": default_layout["h"],
                                     "moved": False, "static": False}
            for lk in ("minH", "maxH", "resizeHandles"):
                if lk in default_layout:
                    item[lk] = default_layout[lk]
            layout.append(item)
            _ensure_component_in_map(schema, node["componentName"])

        elif action == "remove":
            title = change.get("title", "")
            idx = _find_chart_index_by_title(children, title)
            if idx is not None:
                removed = children.pop(idx)
                remove_keys = {removed.get("id"), removed.get("props", {}).get("fieldId")}
                layout[:] = [li for li in layout if li.get("i") not in remove_keys]

        elif action == "replace":
            title = change.get("title", "")
            chart_def = change.get("chart", {})
            idx = _find_chart_index_by_title(children, title)
            if idx is not None:
                old_node = children[idx]
                old_keys = {old_node.get("id"), old_node.get("props", {}).get("fieldId")}
                node, field_id, default_layout = build_chart_component(chart_def, cube_tenant_id=cube_tenant_id)
                for li in layout:
                    if li.get("i") in old_keys:
                        li["i"] = field_id
                        break
                children[idx] = node
                _ensure_component_in_map(schema, node["componentName"])

        elif action == "update-props":
            title = change.get("title", "")
            props_patch = change.get("props", {})
            idx = _find_chart_index_by_title(children, title)
            if idx is not None:
                children[idx].setdefault("props", {}).update(props_patch)

    root_content["children"] = children
    root_content.setdefault("props", {})["layout"] = layout
    return schema


# ---------------------------------------------------------------------------
# 筛选器
# ---------------------------------------------------------------------------


def auto_generate_filters(charts: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """从图表字段中自动提取 select-like 字段作为筛选器。"""
    seen: set[str] = set()
    filters: list[dict[str, Any]] = []
    prefixes = ("selectField_", "radioField_", "checkboxField_", "multiSelectField_")

    _SCAN_KEYS = ("xField", "yField", "columnFields", "kpi", "kpiField",
                   "leftYFields", "rightYFields", "columnList", "valueField")

    for chart in charts:
        for key in _SCAN_KEYS:
            val = chart.get(key, "")
            if not val:
                continue
            fields = val if isinstance(val, list) else [val]
            for f in fields:
                fc = f.get("fieldCode", "") if isinstance(f, dict) else str(f)
                if not fc:
                    continue
                base = fc.replace("_value", "") if fc.endswith("_value") else fc
                if any(base.startswith(p) for p in prefixes) and base not in seen:
                    seen.add(base)
                    filters.append({
                        "type": "select",
                        "cubeCode": chart.get("cubeCode", ""),
                        "fieldCode": fc if fc.endswith("_value") else normalize_field_code(fc),
                        "label": base,
                    })
    return filters


def _build_select_filter(
    flt: dict[str, Any],
    cube_tenant_id: str,
    chart_nodes: list[dict[str, Any]],
) -> Optional[dict[str, Any]]:
    if flt.get("type") != "select":
        return None

    node_id = next_node_id()
    field_code = normalize_field_code(flt.get("fieldCode", ""))
    cube_code = normalize_cube_code(flt.get("cubeCode", ""))
    label = flt.get("label", field_code)

    node: dict[str, Any] = {
        "componentName": "YoushuSelect",
        "id": node_id,
        "props": {
            "fieldId": generate_field_id("YoushuSelect"),
            "cid": node_id,
            "componentTitle": i18n(label),
            "dataSetModelMap": {
                "selectFilter": {
                    "dataViewQueryModel": {
                        "cubeCode": cube_code,
                        "fieldDefinitionList": [{
                            "classifiedCode": cube_code,
                            "cubeCode": cube_code,
                            "fieldCode": field_code,
                            "dataType": "VARCHAR",
                            "isDim": False,
                            "aggregateType": "NONE",
                            "alias": "filter_dim",
                            "aliasName": {"type": "i18n", "zh_CN": label, "en_US": label},
                            "timeGranularityType": None,
                        }],
                        "fieldList": ["filter_dim"],
                        "filterList": [],
                        "orderByList": [],
                        "cubeTenantId": cube_tenant_id,
                    },
                    "fieldList": [],
                    "youshuDataType": "real",
                    "cubeCodes": [cube_code] if cube_code else "",
                    "filterList": [],
                    "limit": "",
                }
            },
            "filterLinkage": _build_filter_linkage(flt, chart_nodes, field_code),
        },
    }
    return node


def _build_filter_linkage(
    flt: dict[str, Any],
    chart_nodes: list[dict[str, Any]],
    field_code: str,
) -> list[dict[str, Any]]:
    link_to = flt.get("linkTo")
    linkage: list[dict[str, Any]] = []

    targets = chart_nodes if link_to is None else [chart_nodes[i] for i in link_to if i < len(chart_nodes)]
    for target in targets:
        linkage.append({
            "targetComponentId": target.get("id", ""),
            "targetFieldCode": field_code,
        })
    return linkage


# ---------------------------------------------------------------------------
# 辅助
# ---------------------------------------------------------------------------


def _find_root_content(schema: dict[str, Any]) -> Optional[dict[str, Any]]:
    pages = schema.get("pages", [])
    if not pages:
        return None
    tree = pages[0].get("componentsTree", [])
    if not tree:
        return None

    for child in tree[0].get("children", []):
        if child.get("componentName") == "RootContent":
            return child
    return None


def _find_chart_index_by_title(nodes: list[dict[str, Any]], title: str) -> Optional[int]:
    for idx, node in enumerate(nodes):
        ct = node.get("props", {}).get("componentTitle", {})
        zh = ct.get("zh_CN", "") if isinstance(ct, dict) else str(ct)
        if zh == title:
            return idx
    return None


def _ensure_component_in_map(schema: dict[str, Any], component_name: str) -> None:
    pages = schema.get("pages", [])
    if not pages:
        return
    cm = pages[0].get("componentsMap", [])
    from yida_schema_common import REPORT_COMPONENT_PACKAGE, REPORT_COMPONENT_VERSION
    if not any(c.get("componentName") == component_name for c in cm):
        cm.append({
            "package": REPORT_COMPONENT_PACKAGE,
            "version": REPORT_COMPONENT_VERSION,
            "componentName": component_name,
        })


def parse_report_config(raw: Any) -> tuple[list[dict[str, Any]], Optional[list[dict[str, Any]]]]:
    """解析入参：支持 [charts] 数组 或 {charts, filters} 对象。"""
    if isinstance(raw, list):
        return raw, None
    if isinstance(raw, dict):
        return raw.get("charts", []), raw.get("filters")
    raise ValueError("charts 入参必须是数组或 {charts, filters} 对象")


def count_report_children(schema: dict[str, Any]) -> int:
    """Return children count in RootContent, or -1 if RootContent not found."""
    rc = _find_root_content(schema)
    if rc is None:
        return -1
    return len(rc.get("children", []))


def is_empty_report_skeleton(schema: dict[str, Any]) -> bool:
    return count_report_children(schema) == 0
