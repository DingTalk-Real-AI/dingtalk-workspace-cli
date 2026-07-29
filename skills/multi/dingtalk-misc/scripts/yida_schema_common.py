"""
宜搭 schema 公共工具模块

提供 form / report / custom-page 三类 builder 共用的基础函数：
  - next_node_id / generate_field_id — ID 生成
  - i18n / build_yida_i18n — 国际化包装
  - build_option_data_source — 选项 dataSource 构造 + 语义色彩
  - build_components_map — componentsMap 构造
  - FIELD_TYPE_ALIAS — 字段类型别名映射
"""
from __future__ import annotations

import random
import string
import time
from typing import Any, Optional

# ---------------------------------------------------------------------------
# 常量
# ---------------------------------------------------------------------------

COMPONENT_PACKAGE = "@ali/vc-deep-yida"
COMPONENT_VERSION = "1.5.169"

REPORT_COMPONENT_PACKAGE = "@/components/vc-yida-report"
REPORT_COMPONENT_VERSION = "1.0.6"

UTILS_LEGAO_BUILTIN = {
    "name": "legaoBuiltin",
    "type": "npm",
    "content": {
        "package": "@ali/vu-legao-builtin",
        "version": "3.0.0",
        "exportName": "legaoBuiltin",
    },
}

UTILS_YIDA_PLUGIN = {
    "name": "yidaPlugin",
    "type": "npm",
    "content": {
        "package": "@ali/vu-yida-plugin",
        "version": "1.0.13",
        "exportName": "yidaPlugin",
    },
}

# 字段类型别名 → 标准 componentName
FIELD_TYPE_ALIAS: dict[str, str] = {
    "text": "TextField",
    "textfield": "TextField",
    "textarea": "TextareaField",
    "textareafield": "TextareaField",
    "number": "NumberField",
    "numberfield": "NumberField",
    "rate": "RateField",
    "ratefield": "RateField",
    "date": "DateField",
    "datefield": "DateField",
    "cascadedate": "CascadeDateField",
    "cascadedatefield": "CascadeDateField",
    "daterange": "CascadeDateField",
    "radio": "RadioField",
    "radiofield": "RadioField",
    "select": "SelectField",
    "selectfield": "SelectField",
    "checkbox": "CheckboxField",
    "checkboxfield": "CheckboxField",
    "multiselect": "MultiSelectField",
    "multiselectfield": "MultiSelectField",
    "country": "CountrySelectField",
    "countryselectfield": "CountrySelectField",
    "address": "AddressField",
    "addressfield": "AddressField",
    "attachment": "AttachmentField",
    "attachmentfield": "AttachmentField",
    "image": "ImageField",
    "imagefield": "ImageField",
    "employee": "EmployeeField",
    "employeefield": "EmployeeField",
    "department": "DepartmentSelectField",
    "departmentselectfield": "DepartmentSelectField",
    "table": "TableField",
    "tablefield": "TableField",
    "association": "AssociationFormField",
    "associationformfield": "AssociationFormField",
    "serialnumber": "SerialNumberField",
    "serialnumberfield": "SerialNumberField",
    "divider": "Divider",
}

DATA_SOURCE_FIT_COMPILED = (
    "'use strict';\n\nvar __preParser__ = function fit(response) {\n"
    "  var content = response.content !== undefined ? response.content : response;\n"
    "  var error = {\n"
    "    message: response.errorMsg || response.errors && response.errors[0] && response.errors[0].msg || response.content || '远程数据源请求出错，success is false'\n"
    "  };\n"
    "  var success = true;\n"
    "  if (response.success !== undefined) {\n"
    "    success = response.success;\n"
    "  } else if (response.hasError !== undefined) {\n"
    "    success = !response.hasError;\n"
    "  }\n"
    "  return {\n"
    "    content: content,\n"
    "    success: success,\n"
    "    error: error\n"
    "  };\n"
    "};"
)

DATA_SOURCE_FIT_SOURCE = (
    "function fit(response) {\r\n"
    "  const content = (response.content !== undefined) ? response.content : response;\r\n"
    "  const error = {\r\n"
    "    message: response.errorMsg ||\r\n"
    "      (response.errors && response.errors[0] && response.errors[0].msg) ||\r\n"
    "      response.content || '远程数据源请求出错，success is false',\r\n"
    "  };\r\n"
    "  let success = true;\r\n"
    "  if (response.success !== undefined) {\r\n"
    "    success = response.success;\r\n"
    "  } else if (response.hasError !== undefined) {\r\n"
    "    success = !response.hasError;\r\n"
    "  }\r\n"
    "  return {\r\n"
    "    content,\r\n"
    "    success,\r\n"
    "    error,\r\n"
    "  };\r\n"
    "}"
)

SUPPORTED_FIELD_TYPES = {
    "TextField", "TextareaField", "NumberField", "RateField",
    "DateField", "CascadeDateField",
    "RadioField", "SelectField", "CheckboxField", "MultiSelectField",
    "CountrySelectField", "AddressField",
    "AttachmentField", "ImageField",
    "EmployeeField", "DepartmentSelectField",
    "TableField", "AssociationFormField", "SerialNumberField",
    "Divider",
}

OPTION_FIELD_TYPES = {"RadioField", "SelectField", "CheckboxField", "MultiSelectField"}

# ---------------------------------------------------------------------------
# ID 生成
# ---------------------------------------------------------------------------

_node_counter = 0


def _random_chars(n: int = 6) -> str:
    return "".join(random.choices(string.ascii_letters + string.digits, k=n))


def unique_id(prefix: str = "", separator: str = "") -> str:
    timestamp = str(int(time.time() * 1000000))[-6:]
    random_part = _random_chars(6)
    return f"{prefix}{separator}{timestamp}{random_part}"


def next_node_id() -> str:
    global _node_counter
    _node_counter += 1
    ts = int(time.time() * 1000)
    base36 = ""
    n = ts
    while n > 0:
        n, r = divmod(n, 36)
        base36 = "0123456789abcdefghijklmnopqrstuvwxyz"[r] + base36
    return f"node_oc{base36}{_node_counter}"


def generate_field_id(component_name: str) -> str:
    if not component_name:
        return unique_id()
    first_char_lower = component_name[0].lower()
    rest_of_name = component_name[1:]
    return f"{first_char_lower}{rest_of_name}_{unique_id()}"


# ---------------------------------------------------------------------------
# 国际化
# ---------------------------------------------------------------------------


def i18n(text: str, en_text: Optional[str] = None) -> dict[str, str]:
    return {
        "type": "i18n",
        "zh_CN": text,
        "en_US": en_text or text,
    }


def build_yida_i18n(text: str, translations: Optional[dict[str, str]] = None) -> dict[str, str]:
    result: dict[str, str] = {"type": "i18n", "zh_CN": text, "en_US": text}
    if translations:
        result.update(translations)
    return result


# ---------------------------------------------------------------------------
# 语义色彩
# ---------------------------------------------------------------------------

_NEGATIVE_KEYWORDS = ['不通过', '拒绝', '失败', '错误', '否', '不同意', '驳回', '取消', '删除', '禁止', '异常', '警告', '危险']
_POSITIVE_KEYWORDS = ['通过', '同意', '成功', '完成', '是', '正常', '确认', '批准', '接受', '优秀', '合格']
_PROCESSING_KEYWORDS = ['处理中', '进行中', '待审核', '审核中', '处理', '待定', '等待']
_WARNING_KEYWORDS = ['注意', '提醒', '待办', '紧急', '重要']
_PAUSE_KEYWORDS = ['暂停', '挂起', '冻结', '停用']

COLOR_PALETTE = [
    '#e0f0ff', '#e0f4e6', '#fff2e0', '#ffece6', '#eee9fe',
    '#e0f2f2', '#fff7e0', '#fde5ec', '#f6e4ff', '#e8ebfc',
    '#f6f6f7', '#bbddff', '#bbe7c8', '#ffe2bb', '#ffcfd8',
    '#d9cefd', '#bbe3e3', '#ffeebb', '#fac4d4', '#ebc4ff',
    '#cbd2f8', '#edeeef',
    '#007fff', '#00a532', '#fd9100', '#f2510c', '#704af7',
    '#009595', '#fdbd00', '#e9235d', '#b421fd', '#3954e4',
    '#76787a', '#0058b1', '#007423', '#b16600', '#a62700',
    '#4f34af', '#006868', '#b18500', '#a41841', '#7e17b1',
    '#2a3d9f', '#181c1f',
]


def get_semantic_color(text: str) -> Optional[str]:
    text_lower = text.lower().strip()
    if any(k in text_lower for k in _NEGATIVE_KEYWORDS):
        return '#FF4D4F'
    if any(k in text_lower for k in _POSITIVE_KEYWORDS):
        return '#52C41A'
    if any(k in text_lower for k in _PROCESSING_KEYWORDS):
        return '#1890FF'
    if any(k in text_lower for k in _WARNING_KEYWORDS):
        return '#FA8C16'
    if any(k in text_lower for k in _PAUSE_KEYWORDS):
        return '#8C8C8C'
    return None


def build_option_data_source(
    options: list[str],
    *,
    is_checkbox: bool = False,
    used_colors: Optional[set[str]] = None,
) -> tuple[list[dict[str, Any]], set[str]]:
    if used_colors is None:
        used_colors = set()
    color_idx = 0
    data_source: list[dict[str, Any]] = []
    for idx, opt_name in enumerate(options):
        semantic = get_semantic_color(opt_name)
        if semantic:
            color = semantic
        else:
            while color_idx < len(COLOR_PALETTE) and COLOR_PALETTE[color_idx] in used_colors:
                color_idx += 1
            color = COLOR_PALETTE[color_idx % len(COLOR_PALETTE)]
            color_idx += 1
        used_colors.add(color)
        data_source.append({
            "text": i18n(opt_name),
            "value": opt_name,
            "defaultChecked": False if is_checkbox else (idx == 0),
            "color": color,
        })
    return data_source, used_colors


# ---------------------------------------------------------------------------
# componentsMap
# ---------------------------------------------------------------------------


def build_components_map(component_names: list[str]) -> list[dict[str, str]]:
    seen: set[str] = set()
    result: list[dict[str, str]] = []
    for name in component_names:
        if name not in seen:
            seen.add(name)
            result.append({
                "package": COMPONENT_PACKAGE,
                "version": COMPONENT_VERSION,
                "componentName": name,
            })
    return result


_REPORT_LOWCODE_COMPONENTS = {"YoushuSelect"}


def build_report_components_map(component_names: list[str]) -> list[dict[str, str]]:
    seen: set[str] = set()
    result: list[dict[str, str]] = []
    for name in component_names:
        if name not in seen:
            seen.add(name)
            if name in _REPORT_LOWCODE_COMPONENTS:
                result.append({
                    "devMode": "lowcode",
                    "componentName": name,
                })
            else:
                result.append({
                    "package": REPORT_COMPONENT_PACKAGE,
                    "version": REPORT_COMPONENT_VERSION,
                    "componentName": name,
                })
    return result


def collect_component_names(field_nodes: list[dict[str, Any]]) -> list[str]:
    names: list[str] = []

    def _walk(node: dict[str, Any]) -> None:
        cn = node.get("componentName", "")
        if cn:
            names.append(cn)
        for child in node.get("children", []):
            _walk(child)

    for node in field_nodes:
        _walk(node)
    return names


# ---------------------------------------------------------------------------
# 类型解析
# ---------------------------------------------------------------------------


def normalize_field_type(raw_type: str) -> str:
    lower = raw_type.lower().replace("_", "").replace("-", "")
    if lower in FIELD_TYPE_ALIAS:
        return FIELD_TYPE_ALIAS[lower]
    if raw_type in SUPPORTED_FIELD_TYPES:
        return raw_type
    raise ValueError(f"不支持的字段类型: {raw_type}")
