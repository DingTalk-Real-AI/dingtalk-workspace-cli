"""
宜搭报表图表组件构造 — 按 chart.type 分发，产出 component dict + cube 查询模型。

支持 9 种图表: bar / line / pie / funnel / table / indicator / gauge / combo / pivot

dataSetModelMap 结构严格匹配宜搭报表引擎格式：
  - 外层 key: chartData (bar/line/pie/funnel/gauge) / table / youshuData (indicator)
              / dataSetName (combo/pivot)
  - 内层: dataViewQueryModel{cubeCode, fieldDefinitionList, fieldList, cubeTenantId}
        + 外层 fieldList (完整字段对象) + xField/yField 等角色字段
"""
from __future__ import annotations

import math
from typing import Any, Optional

from yida_schema_common import (
    i18n,
    next_node_id,
    generate_field_id,
)

# ---------------------------------------------------------------------------
# 图表类型 → componentName 映射
# ---------------------------------------------------------------------------

CHART_TYPE_MAP: dict[str, str] = {
    "bar": "YoushuGroupedBarChart",
    "line": "YoushuLineChart",
    "pie": "YoushuPieChart",
    "funnel": "YoushuFunnelChart",
    "table": "YoushuTable",
    "indicator": "YoushuSimpleIndicatorCard",
    "gauge": "YoushuGauge",
    "combo": "YoushuComboChart",
    "pivot": "YoushuCrossPivotTable",
}

def _compute_dynamic_layout(chart_type: str, chart: dict[str, Any]) -> dict[str, Any]:
    """根据图表内容动态计算布局尺寸。"""
    if chart_type == "table":
        page_size = chart.get("pageSize", 20)
        h = max(16, min(40, round(page_size * 0.9) + 6))
        return {"w": 6, "h": h, "minH": h, "maxH": h, "resizeHandles": ["w", "e"]}

    if chart_type == "indicator":
        kpi_count = len(chart.get("kpi", []))
        col = min(kpi_count, 4) or 1
        rows = math.ceil(kpi_count / col)
        h = max(11, 8 + rows * 5)
        w = min(2 * min(kpi_count, 3), 6)
        return {"w": w, "h": h, "minH": h, "maxH": h, "resizeHandles": ["w", "e"]}

    if chart_type in ("bar", "line"):
        y_raw = chart.get("yField", "")
        y_count = len(y_raw) if isinstance(y_raw, list) else 1
        h = 22 if y_count <= 3 else min(30, 22 + (y_count - 3) * 2)
        return {"w": 3, "h": h}

    if chart_type == "gauge":
        return {"w": 2, "h": 18}

    if chart_type == "combo":
        return {"w": 6, "h": 22}

    if chart_type == "pivot":
        return {"w": 6, "h": 30}

    return {"w": 3, "h": 22}

_VALUE_SUFFIX_PREFIXES = ("selectField_", "radioField_", "checkboxField_", "multiSelectField_", "employeeField_")

_CUBE_LABEL_SUFFIX: dict[str, str] = {
    "selectField_": "_值",
    "radioField_": "_值",
    "checkboxField_": "_值",
    "multiSelectField_": "_值",
    "employeeField_": "_名称",
    "departmentSelectField_": "_名称",
}

_DS_KEY_CHART = "chartData"
_DS_KEY_TABLE = "table"
_DS_KEY_INDICATOR = "youshuData"
_DS_KEY_DATASET = "dataSetName"

_TIME_GRAN_ID_SUFFIX: dict[str, str] = {
    "YEAR": "1", "QUARTER": "2", "MONTH": "3",
    "WEEK": "4", "DAY": "5", "HOUR": "6",
}
_TIME_GRAN_FORMAT: dict[str, str] = {
    "YEAR": "yyyy", "QUARTER": "yyyy-Q", "MONTH": "yyyy-MM",
    "WEEK": "yyyy-w", "DAY": "yyyy-MM-dd", "HOUR": "yyyy-MM-dd HH",
}

_AGGREGATE_TYPES = frozenset({"COUNT", "SUM", "AVG", "MAX", "MIN", "COUNT_DISTINCT"})


# ---------------------------------------------------------------------------
# 公共工具
# ---------------------------------------------------------------------------


def normalize_cube_code(code: str) -> str:
    """FORM-E604838A-3121-43D1-... → FORM_E604838A312143D1..."""
    if code.startswith(("FORM-", "FORM_")):
        return "FORM_" + code[5:].replace("-", "")
    return code.replace("-", "_")


_STRIP_SUFFIXES = ("_id", "_date")


def normalize_field_code(field_code: str) -> str:
    fc = field_code
    for sfx in _STRIP_SUFFIXES:
        if fc.endswith(sfx):
            fc = fc[:-len(sfx)]
            break
    if any(fc.startswith(p) for p in _VALUE_SUFFIX_PREFIXES):
        if not fc.endswith("_value"):
            return fc + "_value"
    return fc


def _apply_cube_label_suffix(field_code: str, text: str) -> str:
    """fieldCode 带 _value 后缀时，同步给显示标签追加中文后缀。

    cube 元数据中 selectField_xxx_value 显示名为 "原标签_值"，
    employeeField_xxx_value 显示名为 "原标签_名称"。
    不追加会导致引擎报"找不到配置"。
    """
    if not field_code.endswith("_value"):
        return text
    base = field_code[:-6]
    for prefix, suffix in _CUBE_LABEL_SUFFIX.items():
        if base.startswith(prefix):
            if not text.endswith(suffix):
                return text + suffix
            return text
    return text


def _derive_label(field_code: str) -> str:
    """从 fieldCode 推导可读标签：去前缀、去 _value 后缀。"""
    fc = field_code
    for p in _VALUE_SUFFIX_PREFIXES:
        if fc.startswith(p):
            fc = fc[len(p):]
            break
    else:
        for prefix in ("textField_", "numberField_", "dateField_", "textareaField_",
                        "countrySelectField_", "addressField_", "attachmentField_",
                        "imageField_", "departmentSelectField_", "tableField_",
                        "associationFormField_", "serialNumberField_", "rateField_",
                        "cascadeDateField_"):
            if fc.startswith(prefix):
                fc = fc[len(prefix):]
                break
    if fc.endswith("_value"):
        fc = fc[:-6]
    return fc or field_code


def _infer_data_type(field_code: str) -> str:
    if field_code.startswith("numberField_"):
        return "NUMBER"
    if field_code.startswith("dateField_"):
        return "DATE"
    if field_code.startswith(("employeeField_", "departmentSelectField_")):
        return "STRING" if field_code.endswith("_value") else "ARRAY"
    return "STRING"


_ARRAY_FIELD_PREFIXES = ("employeeField_", "departmentSelectField_")


def _fix_array_field_for_aggregate(field_code: str, data_type: str, aggregate_type: str) -> tuple[str, str]:
    """数组字段做聚合时保留 _value 后缀（cube 元数据只认 _value），仅将 ARRAY 类型转为 STRING。"""
    if aggregate_type == "NONE":
        return field_code, data_type
    if any(field_code.startswith(p) for p in _ARRAY_FIELD_PREFIXES):
        import warnings
        warnings.warn(
            f"⚠ {field_code} 是数组字段，不能做聚合（{aggregate_type}）——"
            f"cube SQL 会报 'aggregate cannot contain set-returning function'。"
            f"请改用非数组字段（如 numberField / textField）做 {aggregate_type}。",
            stacklevel=3,
        )
    if data_type == "ARRAY":
        return field_code, "STRING"
    return field_code, data_type


def _is_date_field(field_code: str) -> bool:
    return field_code.startswith("dateField_")


def _gen_alias() -> str:
    return f"field_{next_node_id()}"


# ---------------------------------------------------------------------------
# 字段解析：支持 string 或 rich field dict
# ---------------------------------------------------------------------------


def _resolve_field(raw: Any, cube_code: str) -> dict[str, Any]:
    """把 string 或 dict 输入统一解析为标准字段元数据。"""
    if isinstance(raw, dict):
        fc = normalize_field_code(raw.get("fieldCode", ""))
        if not fc:
            raise ValueError(
                f"fieldCode 不能为空。请先通过 `dws yida form components` 获取源表字段定义，"
                f"使用返回的 fieldCode 构造字段对象。收到: {raw!r}"
            )
        text = raw.get("text") or raw.get("label") or _derive_label(fc)
        text = _apply_cube_label_suffix(fc, text)
        return {
            "fieldCode": fc,
            "dataType": raw.get("dataType") or _infer_data_type(fc),
            "text": text,
            "id": raw.get("id") or fc,
            "classifiedCode": raw.get("classifiedCode") or cube_code,
            "timeGranularityType": raw.get("timeGranularityType"),
            "timeFormat": raw.get("timeFormat"),
        }
    fc = normalize_field_code(str(raw))
    if not fc:
        raise ValueError(
            "fieldCode 不能为空字符串。请先通过 `dws yida form components` 获取源表字段定义。"
        )
    text = _apply_cube_label_suffix(fc, _derive_label(fc))
    return {
        "fieldCode": fc,
        "dataType": _infer_data_type(fc),
        "text": text,
        "id": fc,
        "classifiedCode": cube_code,
        "timeGranularityType": None,
        "timeFormat": None,
    }


# ---------------------------------------------------------------------------
# 字段对象构造（匹配宜搭报表引擎格式）
# ---------------------------------------------------------------------------


def _build_query_field_def(
    alias: str,
    label: str,
    cube_code: str,
    field_code: str,
    data_type: str,
    aggregate_type: str = "NONE",
    time_gran: Optional[str] = None,
    classified_code: Optional[str] = None,
) -> dict[str, Any]:
    """dataViewQueryModel.fieldDefinitionList 中的单条定义。"""
    return {
        "classifiedCode": classified_code or cube_code,
        "cubeCode": cube_code,
        "fieldCode": field_code,
        "dataType": data_type,
        "isDim": False,
        "aggregateType": aggregate_type,
        "alias": alias,
        "aliasName": {"type": "i18n", "zh_CN": label, "en_US": label},
        "timeGranularityType": time_gran,
    }


def _build_full_field(
    alias: str,
    label: str,
    cube_code: str,
    field_code: str,
    data_type: str,
    aggregate_type: str = "NONE",
    time_gran: Optional[str] = None,
    classified_code: Optional[str] = None,
    field_id: Optional[str] = None,
    time_format: Optional[str] = None,
) -> dict[str, Any]:
    """外层 fieldList / xField / yField 中的完整字段对象。"""
    fid = field_id or field_code
    effective_dt = "DOUBLE" if aggregate_type in _AGGREGATE_TYPES else data_type
    result: dict[str, Any] = {
        "title": i18n(label),
        "classifiedCode": classified_code or cube_code,
        "cubeCode": cube_code,
        "fieldCode": field_code,
        "isDimension": "false",
        "dataType": effective_dt,
        "format": {"type": "NONE"},
        "link": [{"type": "NONE"}],
        "drillList": [],
        "aggregateType": aggregate_type,
        "orderBy": {"type": "NONE", "reference": alias},
        "fieldKey": alias,
        "visible": True,
        "beUsedTimes": 1,
        "isVisible": "y",
        "id": fid,
        "text": label,
    }
    if aggregate_type in _AGGREGATE_TYPES:
        result["measureType"] = "MEASURE_ATTRIBUTE"
    if time_gran:
        result["timeGranularityType"] = time_gran
        suffix = _TIME_GRAN_ID_SUFFIX.get(time_gran, "")
        if suffix:
            result["id"] = field_code + suffix
        fmt = time_format or _TIME_GRAN_FORMAT.get(time_gran)
        if fmt:
            result["timeFormat"] = fmt
    return result


def _wrap_dataset(
    ds_key: str,
    cube_code: str,
    cube_tenant_id: str,
    query_field_defs: list[dict[str, Any]],
    aliases: list[str],
    full_fields: list[dict[str, Any]],
    extra: Optional[dict[str, Any]] = None,
    extra_query: Optional[dict[str, Any]] = None,
) -> dict[str, Any]:
    """组装完整 dataSetModelMap（匹配宜搭报表引擎结构）。"""
    query_model: dict[str, Any] = {
        "cubeCode": cube_code,
        "cubeTenantId": cube_tenant_id,
        "fieldDefinitionList": query_field_defs,
        "fieldList": aliases,
        "filterList": [],
        "orderByList": [],
    }
    if extra_query:
        query_model.update(extra_query)

    ds: dict[str, Any] = {
        "cubeCodes": [cube_code],
        "dataViewQueryModel": query_model,
        "fieldList": full_fields,
        "youshuDataType": "real",
        "filterList": [],
        "limit": "",
        "mockData": [],
    }

    if extra:
        ds.update(extra)
    return {ds_key: ds}


# ---------------------------------------------------------------------------
# 图表 settings 默认值
# ---------------------------------------------------------------------------

_AXIS_LABEL_STYLE = {
    "labelType": "default",
    "color": "rgba(23,26,29,0.4)",
    "fontSize": 12,
    "limitLengthType": "percent",
    "percent": 30,
    "value": 100,
    "autoRotate": True,
    "rotate": "0",
    "autoHide": True,
}

_X_AXIS_DEFAULT = {
    "showXAxis": True,
    "showTitle": False,
    "title": {"type": "i18n", "zh_CN": "", "en_US": ""},
    "line": True,
    "tickLine": True,
    "grid": False,
    "label": True,
    "labelStyle": _AXIS_LABEL_STYLE,
    "values": {"type": "i18n", "zh_CN": "", "en_US": ""},
}

_Y_AXIS_DEFAULT = {
    "showYAxis": True,
    "showTitle": False,
    "title": {"type": "i18n", "zh_CN": "", "en_US": ""},
    "line": False,
    "tickLine": False,
    "grid": True,
    "label": True,
    "labelStyle": _AXIS_LABEL_STYLE,
    "min": None,
    "max": None,
    "tickCount": 5,
}

_CUSTOM_COLOR = "#5894FF,#394B76,#F7B900,#E55F24,#80D5F5,#9849B0,#3BC88A,#0E869D,#F4A49E,#80563C"

_LEGEND_DEFAULT = {"showLegend": True, "legendPosition": "top-left", "flipPage": True}


def _get_chart_settings(chart_type: str, chart: dict[str, Any]) -> dict[str, Any]:
    if chart_type == "bar":
        return {
            "container": {"height": 248},
            "style": {
                "mode": "group",
                "linkGroup": False,
                "transpose": False,
                "barStyle": "ai",
                "size": None,
                "maxSize": None,
                "minSize": None,
                "barBackground": None,
                "groupSpacing": 0,
                "radiusLeftTop": 4,
                "radiusRightTop": 4,
                "radiusRightBottom": 0,
                "radiusLeftBottom": 0,
                "colorType": "CUSTOM_COLOR",
                "chartColorsMode": "EHRColorsMode",
                "customColor": _CUSTOM_COLOR,
            },
            "countLabel": {"showCountLabel": False, "fontSize": 12, "color": "#000"},
            "axisType": "hz",
            "xAxis": _X_AXIS_DEFAULT,
            "yAxis": _Y_AXIS_DEFAULT,
            "legend": _LEGEND_DEFAULT,
            "label": {
                "showLabel": True,
                "labelShowStyle": "ai",
                "fontSize": 12,
                "autoColor": True,
                "color": "#000",
                "autoPosition": False,
                "position": "middle",
                "autoAdjust": True,
                "autoHide": True,
            },
            "slider": {"showSlider": False},
            "tooltip": {"showTooltip": True},
        }
    elif chart_type == "line":
        return {
            "container": {"height": 248},
            "style": {
                "mode": "none",
                "lineStyle": "ai",
                "lineSize": 2,
                "smooth": False,
                "showArea": False,
                "areaOpacity": 0.25,
                "showPoint": True,
                "pointSize": 4,
                "pointShape": "circle",
                "showLine": True,
                "lineWidth": 2,
                "colorType": "CUSTOM_COLOR",
                "chartColorsMode": "EHRColorsMode",
                "customColor": _CUSTOM_COLOR,
            },
            "axisType": "hz",
            "xAxis": _X_AXIS_DEFAULT,
            "yAxis": _Y_AXIS_DEFAULT,
            "legend": _LEGEND_DEFAULT,
            "label": {"showLabel": True, "fontSize": 12, "color": "#000", "autoOverlap": True},
            "slider": {"showSlider": False},
            "tooltip": {"showTooltip": True},
        }
    elif chart_type == "pie":
        return {
            "container": {"height": 248},
            "style": {
                "radius": 75,
                "isRing": False,
                "innerRadius": 0,
                "colorType": "CUSTOM_COLOR",
                "chartColorsMode": "EHRColorsMode",
                "customColor": _CUSTOM_COLOR,
            },
            "statistic": {"showStatistic": False},
            "label": {
                "showLabel": True,
                "showLine": True,
                "labelAlign": "outer",
                "labelSize": 12,
                "labelColor": "#404040",
                "labelFormatType": "NAME_PERCENT",
            },
            "legend": {
                "showLegend": True,
                "legendPosition": "right",
                "flipPage": True,
                "type": "item",
                "contentType": "NAME",
                "cardWidth": None,
                "ratio": 65,
                "layout": "vertical",
                "itemSpacing": 12,
            },
            "tooltip": {"showTooltip": True, "contentType": None},
            "percentDigits": 2,
        }
    elif chart_type == "funnel":
        return {
            "container": {"height": 248},
            "style": {
                "colorType": "CUSTOM_COLOR",
                "chartColorsMode": "EHRColorsMode",
                "customColor": _CUSTOM_COLOR,
            },
            "legend": _LEGEND_DEFAULT,
            "label": {"showLabel": True, "fontSize": 12, "color": "#000"},
            "tooltip": {"showTooltip": True},
        }
    elif chart_type == "gauge":
        return {
            "container": {"height": 248},
            "useSingleColor": False,
            "singleColor": "#0089FF",
            "color": [],
            "tick": {"showTick": True, "min": None, "max": None, "tickInterval": None},
            "assistValue": {"openAssistValue": True, "showCompare": False, "position": "bottom"},
            "style": {"rounded": True, "pivot": True, "rangeSize": 16, "radius": 95, "innerRadius": 90},
        }
    elif chart_type == "combo":
        return {
            "container": {"height": 248},
            "style": {
                "sync": False,
                "chartType": "bar-line",
                "bar": {
                    "size": None, "maxSize": None, "minSize": None, "mode": "group",
                    "barBackground": None,
                    "radiusLeftTop": 4, "radiusRightTop": 4,
                    "radiusRightBottom": 0, "radiusLeftBottom": 0,
                },
                "line": {
                    "size": 2, "smooth": False,
                    "showPoint": True, "pointSize": 4, "pointShape": "circle",
                },
                "autoAdjust": True,
                "colorType": "CUSTOM_COLOR",
                "chartColorsMode": "EHRColorsMode",
                "customColor": _CUSTOM_COLOR,
            },
            "xAxis": _X_AXIS_DEFAULT,
            "leftYAxis": {
                "showLeftYAxis": True, "showTitle": False,
                "title": {"type": "i18n", "zh_CN": "", "en_US": ""},
                "line": False, "tickLine": False, "grid": True, "label": True,
                "labelStyle": _AXIS_LABEL_STYLE,
                "min": None, "max": None, "tickCount": 5,
            },
            "rightYAxis": {
                "showRightYAxis": True, "showTitle": False,
                "title": {"type": "i18n", "zh_CN": "", "en_US": ""},
                "line": False, "tickLine": False, "grid": False, "label": True,
                "labelStyle": _AXIS_LABEL_STYLE,
                "min": None, "max": None, "tickCount": 5,
            },
            "legend": _LEGEND_DEFAULT,
            "leftLabel": {"showLabel": True, "fontSize": 12, "color": "#000"},
            "rightLabel": {"showLabel": True, "fontSize": 12, "color": "#000"},
            "slider": {"showSlider": False},
            "tooltip": {"showTooltip": True},
        }
    elif chart_type == "table":
        page_size = chart.get("pageSize", 20)
        return {
            "rglConfig": {"w": 6, "h": 21, "isHeightAuto": True},
            "size": "medium",
            "wordSize": "medium:14",
            "theme": "split",
            "mergeCell": False,
            "fixedHeader": False,
            "maxBodyHeight": "300",
            "fixedColumnIndex": 1,
            "isReverseTable": False,
            "showReversedHeader": False,
            "isUniqueRows": False,
            "pagination": {
                "isPagination": True,
                "pageSize": page_size,
                "pageShowCount": 5,
                "showPageSelect": False,
                "size": "small",
                "type": "normal",
            },
            "isTree": False,
            "idField": None,
            "pidField": None,
            "isLeaf": None,
            "drilldownFilterList": None,
            "defaultExpand": False,
            "rankStyle": False,
            "container": {"height": 472},
            "titleTip": False,
            "showCopyData": False,
            "enableFieldSelect": False,
            "defaultSelectedFields": "",
            "hasFullscreen": False,
            "copyAsImg": False,
            "height": None,
            "isHeightAuto": True,
        }
    elif chart_type == "indicator":
        kpi_count = len(chart.get("kpi", []))
        return {
            "showSideStyle": "NONE",
            "followTheme": False,
            "themeType": "dark",
            "showSideBorder": True,
            "sideBarColor": "#0089FF",
            "bgColorType": "single",
            "singleBgColor": "#F1F2F3",
            "colorType": "SCHEMA_COLOR",
            "multipleBgColor": "defaultColorsMode",
            "customColor": "#0089FF,#FF9200,#11AB4F,#FFD100,#7263EE,#67C5EB,#6B748C,#FF755A,#007E99,#FFA8A8",
            "size": "normal",
            "valueSize": "20px",
            "titleMaxRow": 0,
            "columnCount": min(kpi_count, 4) or 4,
            "columnCountForH5": min(kpi_count, 2) or 2,
            "popoverAlign": "b",
            "container": {"height": 72},
            "titleTip": False,
            "enableFieldSelect": False,
            "hasFullscreen": False,
            "copyAsImg": False,
            "height": None,
            "isHeightAuto": True,
        }
    elif chart_type == "pivot":
        return {
            "rglConfig": {"w": 6, "h": 21, "isHeightAuto": True},
            "maxBodyHeight": 500,
            "size": "normal",
            "rows": [],
            "columns": [],
            "measures": [],
            "details": [],
            "supportExport": False,
            "exportType": "XJZ",
            "dialogWidth": 850,
            "dialogPageSize": 10,
            "baseInfo": {
                "isShowSetter": True, "isShowFilter": False, "isShowReload": False,
                "isHideTitle": False, "isMeasureOrder": True, "isZebra": True,
                "rowMaxSize": 3000, "columnsMaxSize": 500, "columnWidth": 100,
                "dialogWidth": 850, "dialogPageSize": 10,
                "detailExportData": {"supportExport": False, "exportType": "BROWSER"},
            },
            "mode": "summary",
            "summaryInfo": {
                "isRowTotal": True, "rowTotalWidth": 130, "rowTotalPosition": "end",
                "isColumnTotal": True, "isSubTotal": False,
                "rowMaxSize": 3000, "columnsMaxSize": 500,
            },
            "paginationInfo": {
                "size": "small", "type": "normal", "pageShowCount": 5,
                "pageSize": 10, "showPageSelect": False,
            },
            "container": {"height": 232},
            "titleTip": False,
            "hasFullscreen": False,
            "copyAsImg": False,
            "height": None,
            "isHeightAuto": True,
        }
    return {}


# ---------------------------------------------------------------------------
# userConfig 构建（设计器数据配置面板）
# ---------------------------------------------------------------------------

def _setter(name: str, title: str, **extra_props: Any) -> dict[str, Any]:
    item: dict[str, Any] = {"setterName": "ColumnFieldSetter", "name": name, "title": title}
    if extra_props:
        item["setterProps"] = extra_props
    return item


def _build_user_config(chart_type: str) -> list[dict[str, Any]]:
    if chart_type in ("bar", "line"):
        return [{"name": _DS_KEY_CHART, "title": "配置数据", "items": [
            _setter("xField", "横轴", single=True, showFormatTab=True, showFormulaEditor=True,
                    showFieldInfo=True, showAggregateTab=False, showDrillTab=True, showEditTab=True, showSortTab=True),
            _setter("yField", "纵轴", showFormatTab=True, showFormulaEditor=True,
                    showFieldInfo=True, showEditTab=True, showSortTab=True, showDataLink=True),
            _setter("groupField", "分组", single=True, showFormatTab=True, showEditTab=True),
            _setter("annotationField", "参考线", showFormatTab=True, showEditTab=True),
        ]}]
    if chart_type == "pie":
        return [{"name": _DS_KEY_CHART, "title": "配置数据", "items": [
            _setter("xField", "分类字段", single=True, showAggregateTab=False, showDrillTab=True, showColorTab=True),
            _setter("yField", "数值字段", single=True, showDataLink=True),
            _setter("ratio", "趋势值字段"),
            _setter("totalValue", "总值字段"),
            _setter("totalRatio", "总趋势值字段"),
        ]}]
    if chart_type == "funnel":
        return [{"name": _DS_KEY_CHART, "title": "配置数据", "items": [
            _setter("xField", "横轴", single=True, showFormatTab=True, showAggregateTab=False,
                    showDrillTab=True, showEditTab=True, showSortTab=True),
            _setter("yField", "纵轴", showFormatTab=True, showEditTab=True, showSortTab=True, showDataLink=True),
        ]}]
    if chart_type == "combo":
        return [{"name": _DS_KEY_DATASET, "title": "配置数据", "items": [
            _setter("xField", "横轴", single=True),
            _setter("leftYFields", "左纵轴", showDataLink=True),
            _setter("rightYFields", "右纵轴", showDataLink=True),
            _setter("annotationField", "参考线"),
        ]}]
    if chart_type == "table":
        return [{"name": _DS_KEY_TABLE, "title": "配置数据", "items": [
            _setter("columnFields", "列", showFormulaEditor=True, showFieldInfo=True,
                    showDataLink=True, supportDynamicAlias=True, showBatchSet=True,
                    batchSetFields=["text", "title", "aggregateType", "format_type", "format_decimalDigit"]),
        ]}]
    if chart_type == "indicator":
        return [{"name": _DS_KEY_INDICATOR, "title": "指标数据", "items": [
            _setter("kpi", "指标", single=False, showDataLink=True, supportDynamicAlias=True,
                    showBatchSet=True,
                    batchSetFields=["text", "title", "titleTip", "aggregateType",
                                    "format_type", "format_decimalDigit", "unit"]),
            _setter("helpKpi", "辅助指标", single=False, showDataLink=True),
        ]}]
    if chart_type == "pivot":
        return [{"name": _DS_KEY_DATASET, "title": "配置数据", "items": [
            _setter("columnList", "列", showFormatTab=True, showEditTab=True),
        ]}]
    if chart_type == "gauge":
        return [{"name": _DS_KEY_CHART, "title": "配置数据", "items": [
            _setter("valueField", "指标值", single=True),
            _setter("assitValueField", "辅助值", single=True),
        ]}]
    return []


# ---------------------------------------------------------------------------
# mockData 构建（设计器预览数据）
# ---------------------------------------------------------------------------

def _build_mock_data(chart_type: str) -> list[dict[str, Any]]:
    if chart_type == "bar":
        return [{"name": _DS_KEY_CHART, "data": {"data": [
            {"month": "Jan.", "value": 18.9}, {"month": "Feb.", "value": 28.8},
            {"month": "Mar.", "value": 39.3}, {"month": "Apr.", "value": 81.4},
            {"month": "May", "value": 47},
        ], "meta": [
            {"fieldKey": "xField", "dataType": "STRING", "title": "month"},
            {"fieldKey": "yField", "dataType": "NUMBER", "title": "value"},
        ], "currentPage": 1, "totalCount": 5}}]

    if chart_type == "line":
        return [{"name": _DS_KEY_CHART, "data": {"data": [
            {"xField": "2020", "yField": 3}, {"xField": "2021", "yField": 4},
            {"xField": "2022", "yField": 3.5}, {"xField": "2023", "yField": 5},
            {"xField": "2024", "yField": 4.9},
        ], "meta": [
            {"fieldKey": "xField", "dataType": "STRING", "title": "xField"},
            {"fieldKey": "yField", "dataType": "NUMBER", "title": "yField"},
        ], "currentPage": 1, "totalCount": 5}}]

    if chart_type == "pie":
        return [{"name": _DS_KEY_CHART, "data": {"data": [
            {"xField": "分类A", "yField": 63, "ratio": 0.8, "totalValue": 202, "totalRatio": 0.32},
            {"xField": "分类B", "yField": 72, "ratio": 0.5, "totalValue": 202, "totalRatio": 0.36},
            {"xField": "分类C", "yField": 67, "ratio": 0.3, "totalValue": 202, "totalRatio": 0.33},
        ], "meta": [
            {"fieldKey": "xField", "dataType": "STRING", "title": "xField"},
            {"fieldKey": "yField", "dataType": "NUMBER", "title": "yField"},
            {"fieldKey": "ratio", "dataType": "NUMBER", "title": "ratio"},
            {"fieldKey": "totalValue", "dataType": "NUMBER", "title": "totalValue"},
            {"fieldKey": "totalRatio", "dataType": "NUMBER", "title": "totalRatio"},
        ], "currentPage": 1, "totalCount": 3}}]

    if chart_type == "funnel":
        return [{"name": _DS_KEY_CHART, "data": {"data": [
            {"xField": "展示", "yField": 100}, {"xField": "点击", "yField": 80},
            {"xField": "访问", "yField": 60}, {"xField": "咨询", "yField": 40},
            {"xField": "订单", "yField": 20},
        ], "meta": [
            {"fieldKey": "xField", "dataType": "STRING", "title": "xField"},
            {"fieldKey": "yField", "dataType": "NUMBER", "title": "yField"},
        ], "currentPage": 1, "totalCount": 5}}]

    if chart_type == "gauge":
        return [{"name": _DS_KEY_CHART, "data": {"data": [
            {"value": 75, "assitValue": 100},
        ], "meta": [
            {"fieldKey": "valueField", "dataType": "NUMBER", "title": "value"},
            {"fieldKey": "assitValueField", "dataType": "NUMBER", "title": "assitValue"},
        ], "currentPage": 1, "totalCount": 1}}]

    if chart_type == "combo":
        return [{"name": _DS_KEY_DATASET, "data": {"data": [
            {"xField": "Jan.", "leftY": 18.9, "rightY": 5},
            {"xField": "Feb.", "leftY": 28.8, "rightY": 8},
            {"xField": "Mar.", "leftY": 39.3, "rightY": 12},
            {"xField": "Apr.", "leftY": 81.4, "rightY": 15},
            {"xField": "May", "leftY": 47, "rightY": 10},
        ], "meta": [
            {"fieldKey": "xField", "dataType": "STRING", "title": "xField"},
            {"fieldKey": "leftYFields", "dataType": "NUMBER", "title": "leftY"},
            {"fieldKey": "rightYFields", "dataType": "NUMBER", "title": "rightY"},
        ], "currentPage": 1, "totalCount": 5}}]

    if chart_type == "table":
        return [{"name": _DS_KEY_TABLE, "data": {"data": [
            {"col1": "数据1", "col2": "数据2", "col3": 100},
            {"col1": "数据4", "col2": "数据5", "col3": 200},
            {"col1": "数据7", "col2": "数据8", "col3": 300},
        ], "meta": [
            {"fieldKey": "columnFields", "dataType": "STRING", "title": "col1"},
            {"fieldKey": "columnFields", "dataType": "STRING", "title": "col2"},
            {"fieldKey": "columnFields", "dataType": "NUMBER", "title": "col3"},
        ], "currentPage": 1, "totalCount": 3}}]

    if chart_type == "indicator":
        return [{"name": _DS_KEY_INDICATOR, "data": {"data": [
            {"kpi1": 23123, "kpi2": 7712},
        ], "meta": [
            {"fieldKey": "kpi1", "dataType": "NUMBER", "title": "指标1", "category": "kpi"},
            {"fieldKey": "kpi2", "dataType": "NUMBER", "title": "指标2", "category": "kpi"},
        ], "currentPage": 1, "totalCount": 1}}]

    if chart_type == "pivot":
        return [{"name": _DS_KEY_DATASET, "data": {"data": [
            {"col1": 74, "col2": 9, "col3": 79},
            {"col1": 85, "col2": 15, "col3": 62},
            {"col1": 93, "col2": 28, "col3": 44},
        ], "meta": [
            {"fieldKey": "columnList", "dataType": "NUMBER", "title": "col1"},
            {"fieldKey": "columnList", "dataType": "NUMBER", "title": "col2"},
            {"fieldKey": "columnList", "dataType": "NUMBER", "title": "col3"},
        ], "currentPage": 1, "totalCount": 3}}]

    return []


# ---------------------------------------------------------------------------
# afterFetch / exportData / link 构建
# ---------------------------------------------------------------------------

_AFTER_FETCH = {
    "type": "JSFunction",
    "value": "function afterFetch(data, extraInfo) { return data; }",
}

_EXPORT_DATA = {
    "supportExport": False,
    "passType": "NO_PASS",
    "exportType": "BROWSER",
}

_LINK = {
    "hasLink": False,
    "content": {"type": "i18n", "zh_CN": "更多", "en_US": "More"},
    "onlyIcon": True,
}


# ---------------------------------------------------------------------------
# 顶层字段属性提取
# ---------------------------------------------------------------------------

def _extract_top_level_field_props(chart_type: str, ds_model_map: dict[str, Any]) -> dict[str, Any]:
    """从 dataSetModelMap 提取需要写入 props 顶层的字段属性。"""
    result: dict[str, Any] = {}
    if chart_type in ("bar", "line", "pie", "funnel"):
        ds = ds_model_map.get(_DS_KEY_CHART, {})
        result["xField"] = ds.get("xField", [])
        result["yField"] = ds.get("yField", [])
        if chart_type not in ("pie", "funnel"):
            result["groupField"] = ds.get("groupField", [])
    elif chart_type == "gauge":
        ds = ds_model_map.get(_DS_KEY_CHART, {})
        result["valueField"] = ds.get("valueField", [])
        result["assitValueField"] = ds.get("assitValueField", [])
    elif chart_type == "indicator":
        ds = ds_model_map.get(_DS_KEY_INDICATOR, {})
        result["kpiField"] = ds.get("kpi", [])
        result["helpKpiField"] = ds.get("helpKpi", [])
    elif chart_type == "table":
        ds = ds_model_map.get(_DS_KEY_TABLE, {})
        result["columnField"] = ds.get("columnFields", [])
    elif chart_type == "combo":
        ds = ds_model_map.get(_DS_KEY_DATASET, {})
        result["xField"] = ds.get("xField", [])
        result["leftYFields"] = ds.get("leftYFields", [])
        result["rightYFields"] = ds.get("rightYFields", [])
    elif chart_type == "pivot":
        ds = ds_model_map.get(_DS_KEY_DATASET, {})
        result["columnList"] = ds.get("columnList", [])
    return result


# ---------------------------------------------------------------------------
# 主入口
# ---------------------------------------------------------------------------


def build_chart_component(
    chart: dict[str, Any],
    cube_tenant_id: str = "",
) -> tuple[dict[str, Any], str, dict[str, Any]]:
    """
    构造图表组件。

    Returns:
        (component_node, fieldId, default_layout)
    """
    chart_type = chart.get("type", "bar")
    if chart_type not in CHART_TYPE_MAP:
        raise ValueError(f"不支持的图表类型: {chart_type}（支持: {list(CHART_TYPE_MAP.keys())}）")

    component_name = CHART_TYPE_MAP[chart_type]
    title = chart.get("title", "图表")
    field_id = generate_field_id(component_name)
    node_id = next_node_id()

    cube_code = normalize_cube_code(chart.get("cubeCode", ""))
    if not cube_code:
        raise ValueError(f"图表 '{title}' 缺少 cubeCode")

    data_set_model_map = _build_data_set_model_map(chart, cube_code, cube_tenant_id)

    props: dict[str, Any] = {
        "fieldId": field_id,
        "cid": node_id,
        "showComponentTitle": True,
        "componentTitle": i18n(title),
        "componentTitleTextAlign": "LEFT",
        "titleTipContent": i18n(""),
        "titleTipIconName": "help",
        "headerSize": "medium",
        "link": _LINK,
        "exportData": _EXPORT_DATA,
        "openRefresh": True,
        "enabledCache": True,
        "auth": [],
        "afterFetch": _AFTER_FETCH,
        "__style__": {},
        "mockData": _build_mock_data(chart_type),
        "dataSetModelMap": data_set_model_map,
        "userConfig": _build_user_config(chart_type),
        "settings": _get_chart_settings(chart_type, chart),
        "titleTip": False,
        "hasFullscreen": False,
        "copyAsImg": False,
        "height": None,
        "isHeightAuto": True,
        "datasetModel": {"filterList": []},
    }

    top_level = _extract_top_level_field_props(chart_type, data_set_model_map)
    props.update(top_level)

    if chart_type in ("table", "indicator"):
        props["showFieldSelectIcon"] = True

    if chart_type == "table":
        props["pageSize"] = chart.get("pageSize", 20)

    node: dict[str, Any] = {
        "componentName": component_name,
        "id": node_id,
        "props": props,
    }

    computed = _compute_dynamic_layout(chart_type, chart)
    layout: dict[str, Any] = {
        "w": chart.get("w", computed["w"]),
        "h": chart.get("h", computed["h"]),
    }
    for k in ("minH", "maxH", "resizeHandles"):
        if k in computed:
            layout[k] = computed[k]

    return node, field_id, layout


# ---------------------------------------------------------------------------
# dataSetModelMap 构造
# ---------------------------------------------------------------------------


def _build_data_set_model_map(
    chart: dict[str, Any],
    cube_code: str,
    cube_tenant_id: str,
) -> dict[str, Any]:
    chart_type = chart.get("type", "bar")

    if chart_type in ("bar", "line", "pie", "funnel"):
        return _build_xy_data_model(chart, cube_code, cube_tenant_id)
    elif chart_type == "table":
        return _build_table_data_model(chart, cube_code, cube_tenant_id)
    elif chart_type == "indicator":
        return _build_indicator_data_model(chart, cube_code, cube_tenant_id)
    elif chart_type == "gauge":
        return _build_gauge_data_model(chart, cube_code, cube_tenant_id)
    elif chart_type == "combo":
        return _build_combo_data_model(chart, cube_code, cube_tenant_id)
    elif chart_type == "pivot":
        return _build_pivot_data_model(chart, cube_code, cube_tenant_id)
    return {}


def _build_xy_data_model(
    chart: dict[str, Any],
    cube_code: str,
    cube_tenant_id: str,
) -> dict[str, Any]:
    chart_type = chart.get("type", "bar")
    x_raw = chart.get("xField", "")
    x_resolved = _resolve_field(x_raw, cube_code)

    y_raw = chart.get("yField", "")
    if isinstance(y_raw, (str, dict)):
        y_raw = [y_raw]
    y_resolved_list = [_resolve_field(f, cube_code) for f in y_raw]

    agg_type = chart.get("aggregateType", "COUNT").upper()

    x_label = _apply_cube_label_suffix(
        x_resolved["fieldCode"], chart.get("xLabel", "") or x_resolved["text"]
    )
    y_labels_raw = chart.get("yLabel", [])
    if isinstance(y_labels_raw, str):
        y_labels_raw = [y_labels_raw]

    x_alias = _gen_alias()
    x_time_gran = x_resolved["timeGranularityType"] or (
        chart.get("timeGranularityType", "DAY") if _is_date_field(x_resolved["fieldCode"]) else None
    )

    query_defs = [_build_query_field_def(
        x_alias, x_label, cube_code, x_resolved["fieldCode"], x_resolved["dataType"],
        "NONE", x_time_gran, classified_code=x_resolved["classifiedCode"],
    )]
    x_full = _build_full_field(
        x_alias, x_label, cube_code, x_resolved["fieldCode"], x_resolved["dataType"],
        "NONE", x_time_gran, classified_code=x_resolved["classifiedCode"],
        field_id=x_resolved["id"], time_format=x_resolved["timeFormat"],
    )
    all_full = [x_full]

    y_aliases: list[str] = []
    y_fulls: list[dict[str, Any]] = []
    for i, yr in enumerate(y_resolved_list):
        y_label = _apply_cube_label_suffix(
            yr["fieldCode"], (y_labels_raw[i] if i < len(y_labels_raw) else "") or yr["text"]
        )
        y_alias = _gen_alias()
        y_aliases.append(y_alias)
        y_fc, y_dt = _fix_array_field_for_aggregate(yr["fieldCode"], yr["dataType"], agg_type)
        query_defs.append(_build_query_field_def(
            y_alias, y_label, cube_code, y_fc, y_dt,
            agg_type, classified_code=yr["classifiedCode"],
        ))
        y_full = _build_full_field(
            y_alias, y_label, cube_code, y_fc, y_dt,
            agg_type, classified_code=yr["classifiedCode"], field_id=yr["id"],
        )
        y_fulls.append(y_full)
        all_full.append(y_full)

    extra: dict[str, Any] = {"xField": [x_full], "yField": y_fulls}
    if chart_type == "pie":
        extra.update({"ratio": [], "totalValue": [], "totalRatio": [], "trailingIconField": []})
    elif chart_type != "funnel":
        extra.update({"groupField": [], "annotationField": []})

    limit_val = chart.get("limit")
    if limit_val is not None:
        extra["limit"] = int(limit_val)

    return _wrap_dataset(
        _DS_KEY_CHART, cube_code, cube_tenant_id,
        query_defs, [x_alias] + y_aliases, all_full,
        extra=extra,
    )


def _build_table_data_model(
    chart: dict[str, Any],
    cube_code: str,
    cube_tenant_id: str,
) -> dict[str, Any]:
    raw_columns = chart.get("columnFields", []) or chart.get("columns", []) or chart.get("fields", [])
    if not raw_columns:
        x_field = chart.get("xField", "")
        if x_field:
            raw_columns = [x_field]
    if not raw_columns:
        raise ValueError(
            "table 图表必须提供 columnFields（或 columns / fields / xField）。"
            "请先通过 `dws yida form components` 获取源表字段定义。"
        )

    column_labels_raw = chart.get("columnLabels", [])
    resolved_columns = [_resolve_field(col, cube_code) for col in raw_columns]

    query_defs: list[dict[str, Any]] = []
    aliases: list[str] = []
    full_fields: list[dict[str, Any]] = []

    for i, rc in enumerate(resolved_columns):
        label = _apply_cube_label_suffix(
            rc["fieldCode"], (column_labels_raw[i] if i < len(column_labels_raw) else "") or rc["text"]
        )
        alias = _gen_alias()
        aliases.append(alias)
        time_gran = rc["timeGranularityType"] or (
            chart.get("timeGranularityType", "DAY") if _is_date_field(rc["fieldCode"]) else None
        )
        query_defs.append(_build_query_field_def(
            alias, label, cube_code, rc["fieldCode"], rc["dataType"],
            "NONE", time_gran, classified_code=rc["classifiedCode"],
        ))
        full = _build_full_field(
            alias, label, cube_code, rc["fieldCode"], rc["dataType"],
            "NONE", time_gran, classified_code=rc["classifiedCode"],
            field_id=rc["id"], time_format=rc["timeFormat"],
        )
        full["sortable"] = False
        full["hidden"] = False
        full_fields.append(full)

    column_fields: list[dict[str, Any]] = []
    for i, full in enumerate(full_fields):
        col: dict[str, Any] = {
            "aggregateType": full["aggregateType"],
            "beUsedTimes": 1,
            "classifiedCode": full["classifiedCode"],
            "cubeCode": full["cubeCode"],
            "dataType": full["dataType"],
            "fieldCode": full["fieldCode"],
            "fieldKey": full["fieldKey"],
            "id": full["id"],
            "text": full["text"],
            "title": full["title"],
            "visible": True,
            "width": 120,
            "align": "left",
            "fixed": "left" if i == 0 else None,
            "timeGranularityType": full.get("timeGranularityType"),
            "timeFormat": full.get("timeFormat"),
            "orderBy": full["orderBy"],
        }
        column_fields.append(col)

    return _wrap_dataset(
        _DS_KEY_TABLE, cube_code, cube_tenant_id,
        query_defs, aliases, full_fields,
        extra={"columnFields": column_fields},
    )


def _build_indicator_data_model(
    chart: dict[str, Any],
    cube_code: str,
    cube_tenant_id: str,
) -> dict[str, Any]:
    kpi_list = chart.get("kpi", []) or chart.get("kpiField", []) or chart.get("yField", []) or chart.get("fields", [])
    if not kpi_list:
        raise ValueError("indicator 图表必须提供 kpi（或 kpiField / yField / fields）数组")

    query_defs: list[dict[str, Any]] = []
    aliases: list[str] = []
    full_fields: list[dict[str, Any]] = []

    kpi_rich: list[dict[str, Any]] = []
    for kpi in kpi_list:
        if isinstance(kpi, str):
            kpi = {"fieldCode": kpi}
        fc = normalize_field_code(kpi.get("fieldCode", ""))
        agg = kpi.get("aggregateType", "COUNT").upper()
        dt = kpi.get("dataType") or _infer_data_type(fc)
        fc, dt = _fix_array_field_for_aggregate(fc, dt, agg)
        label = kpi.get("aliasName", "") or kpi.get("text", "") or kpi.get("label", "") or _derive_label(fc)
        label = _apply_cube_label_suffix(fc, label)
        field_id = kpi.get("id") or fc
        classified_code = kpi.get("classifiedCode") or cube_code
        alias = _gen_alias()
        aliases.append(alias)
        query_defs.append(_build_query_field_def(
            alias, label, cube_code, fc, dt, agg, classified_code=classified_code,
        ))
        full = _build_full_field(
            alias, label, cube_code, fc, dt, agg,
            classified_code=classified_code, field_id=field_id,
        )
        full_fields.append(full)
        kpi_item: dict[str, Any] = {
            **full,
            "visible": True,
            "isVisible": "y",
        }
        kpi_rich.append(kpi_item)

    help_kpi_list = chart.get("helpKpi", [])
    help_kpi_rich: list[dict[str, Any]] = []
    for hk in help_kpi_list:
        if isinstance(hk, str):
            hk = {"fieldCode": hk}
        fc = normalize_field_code(hk.get("fieldCode", ""))
        agg = hk.get("aggregateType", "COUNT").upper()
        dt = hk.get("dataType") or _infer_data_type(fc)
        fc, dt = _fix_array_field_for_aggregate(fc, dt, agg)
        label = hk.get("aliasName", "") or hk.get("text", "") or hk.get("label", "") or _derive_label(fc)
        label = _apply_cube_label_suffix(fc, label)
        field_id = hk.get("id") or fc
        classified_code = hk.get("classifiedCode") or cube_code
        alias = _gen_alias()
        aliases.append(alias)
        query_defs.append(_build_query_field_def(
            alias, label, cube_code, fc, dt, agg, classified_code=classified_code,
        ))
        full = _build_full_field(
            alias, label, cube_code, fc, dt, agg,
            classified_code=classified_code, field_id=field_id,
        )
        full_fields.append(full)
        help_kpi_rich.append({**full, "visible": True, "isVisible": "y"})

    return _wrap_dataset(
        _DS_KEY_INDICATOR, cube_code, cube_tenant_id,
        query_defs, aliases, full_fields,
        extra={"kpi": kpi_rich, "helpKpi": help_kpi_rich},
    )


def _build_gauge_data_model(
    chart: dict[str, Any],
    cube_code: str,
    cube_tenant_id: str,
) -> dict[str, Any]:
    value_raw = chart.get("valueField") or chart.get("yField")
    if isinstance(value_raw, list):
        value_raw = value_raw[0] if value_raw else None
    if not value_raw:
        raise ValueError("gauge 图表必须提供 valueField（或 yField）")

    value_resolved = _resolve_field(value_raw, cube_code)
    agg_type = chart.get("aggregateType", "AVG").upper()

    v_alias = _gen_alias()
    v_label = _apply_cube_label_suffix(
        value_resolved["fieldCode"], chart.get("valueLabel", "") or value_resolved["text"]
    )
    v_fc, v_dt = _fix_array_field_for_aggregate(value_resolved["fieldCode"], value_resolved["dataType"], agg_type)

    query_defs = [_build_query_field_def(
        v_alias, v_label, cube_code, v_fc, v_dt, agg_type,
        classified_code=value_resolved["classifiedCode"],
    )]
    v_full = _build_full_field(
        v_alias, v_label, cube_code, v_fc, v_dt, agg_type,
        classified_code=value_resolved["classifiedCode"], field_id=value_resolved["id"],
    )
    all_full = [v_full]
    aliases = [v_alias]

    assit_fulls: list[dict[str, Any]] = []
    assit_raw = chart.get("assitValueField")
    if assit_raw:
        if isinstance(assit_raw, list):
            assit_raw = assit_raw[0] if assit_raw else None
        if assit_raw:
            assit_resolved = _resolve_field(assit_raw, cube_code)
            a_alias = _gen_alias()
            a_label = _apply_cube_label_suffix(
                assit_resolved["fieldCode"], chart.get("assitLabel", "") or assit_resolved["text"]
            )
            a_fc, a_dt = _fix_array_field_for_aggregate(
                assit_resolved["fieldCode"], assit_resolved["dataType"], agg_type
            )
            query_defs.append(_build_query_field_def(
                a_alias, a_label, cube_code, a_fc, a_dt, agg_type,
                classified_code=assit_resolved["classifiedCode"],
            ))
            a_full = _build_full_field(
                a_alias, a_label, cube_code, a_fc, a_dt, agg_type,
                classified_code=assit_resolved["classifiedCode"], field_id=assit_resolved["id"],
            )
            all_full.append(a_full)
            aliases.append(a_alias)
            assit_fulls.append(a_full)

    return _wrap_dataset(
        _DS_KEY_CHART, cube_code, cube_tenant_id,
        query_defs, aliases, all_full,
        extra={"valueField": [v_full], "assitValueField": assit_fulls},
    )


def _build_combo_data_model(
    chart: dict[str, Any],
    cube_code: str,
    cube_tenant_id: str,
) -> dict[str, Any]:
    x_raw = chart.get("xField", "")
    x_resolved = _resolve_field(x_raw, cube_code)

    left_raw = chart.get("leftYFields", [])
    if isinstance(left_raw, (str, dict)):
        left_raw = [left_raw]
    right_raw = chart.get("rightYFields", [])
    if isinstance(right_raw, (str, dict)):
        right_raw = [right_raw]

    if not left_raw and not right_raw:
        raise ValueError("combo 图表必须提供 leftYFields 或 rightYFields")

    left_resolved = [_resolve_field(f, cube_code) for f in left_raw]
    right_resolved = [_resolve_field(f, cube_code) for f in right_raw]

    left_agg = chart.get("leftAggregateType", "SUM").upper()
    right_agg = chart.get("rightAggregateType", "SUM").upper()

    x_label = _apply_cube_label_suffix(
        x_resolved["fieldCode"], chart.get("xLabel", "") or x_resolved["text"]
    )
    x_alias = _gen_alias()
    x_time_gran = x_resolved["timeGranularityType"] or (
        chart.get("timeGranularityType", "DAY") if _is_date_field(x_resolved["fieldCode"]) else None
    )

    query_defs = [_build_query_field_def(
        x_alias, x_label, cube_code, x_resolved["fieldCode"], x_resolved["dataType"],
        "NONE", x_time_gran, classified_code=x_resolved["classifiedCode"],
    )]
    x_full = _build_full_field(
        x_alias, x_label, cube_code, x_resolved["fieldCode"], x_resolved["dataType"],
        "NONE", x_time_gran, classified_code=x_resolved["classifiedCode"],
        field_id=x_resolved["id"], time_format=x_resolved["timeFormat"],
    )
    all_full = [x_full]
    all_aliases = [x_alias]

    left_fulls: list[dict[str, Any]] = []
    for lr in left_resolved:
        alias = _gen_alias()
        all_aliases.append(alias)
        label = _apply_cube_label_suffix(lr["fieldCode"], lr["text"])
        fc, dt = _fix_array_field_for_aggregate(lr["fieldCode"], lr["dataType"], left_agg)
        query_defs.append(_build_query_field_def(
            alias, label, cube_code, fc, dt, left_agg, classified_code=lr["classifiedCode"],
        ))
        full = _build_full_field(
            alias, label, cube_code, fc, dt, left_agg,
            classified_code=lr["classifiedCode"], field_id=lr["id"],
        )
        left_fulls.append(full)
        all_full.append(full)

    right_fulls: list[dict[str, Any]] = []
    for rr in right_resolved:
        alias = _gen_alias()
        all_aliases.append(alias)
        label = _apply_cube_label_suffix(rr["fieldCode"], rr["text"])
        fc, dt = _fix_array_field_for_aggregate(rr["fieldCode"], rr["dataType"], right_agg)
        query_defs.append(_build_query_field_def(
            alias, label, cube_code, fc, dt, right_agg, classified_code=rr["classifiedCode"],
        ))
        full = _build_full_field(
            alias, label, cube_code, fc, dt, right_agg,
            classified_code=rr["classifiedCode"], field_id=rr["id"],
        )
        right_fulls.append(full)
        all_full.append(full)

    return _wrap_dataset(
        _DS_KEY_DATASET, cube_code, cube_tenant_id,
        query_defs, all_aliases, all_full,
        extra={
            "xField": [x_full],
            "leftYFields": left_fulls,
            "rightYFields": right_fulls,
            "annotationField": [],
        },
    )


def _build_pivot_data_model(
    chart: dict[str, Any],
    cube_code: str,
    cube_tenant_id: str,
) -> dict[str, Any]:
    raw_columns = chart.get("columnList", []) or chart.get("columns", [])
    if not raw_columns:
        raise ValueError(
            "pivot 图表必须提供 columnList（或 columns）。"
            "请先通过 `dws yida form components` 获取源表字段定义。"
        )

    resolved_columns = [_resolve_field(col, cube_code) for col in raw_columns]

    query_defs: list[dict[str, Any]] = []
    aliases: list[str] = []
    full_fields: list[dict[str, Any]] = []

    for rc in resolved_columns:
        label = _apply_cube_label_suffix(rc["fieldCode"], rc["text"])
        alias = _gen_alias()
        aliases.append(alias)
        time_gran = rc["timeGranularityType"] or (
            chart.get("timeGranularityType", "DAY") if _is_date_field(rc["fieldCode"]) else None
        )
        query_defs.append(_build_query_field_def(
            alias, label, cube_code, rc["fieldCode"], rc["dataType"],
            "NONE", time_gran, classified_code=rc["classifiedCode"],
        ))
        full = _build_full_field(
            alias, label, cube_code, rc["fieldCode"], rc["dataType"],
            "NONE", time_gran, classified_code=rc["classifiedCode"],
            field_id=rc["id"], time_format=rc["timeFormat"],
        )
        full_fields.append(full)

    return _wrap_dataset(
        _DS_KEY_DATASET, cube_code, cube_tenant_id,
        query_defs, aliases, full_fields,
        extra={"columnList": full_fields},
        extra_query={"filterMode": "PROFESSIONAL"},
    )
