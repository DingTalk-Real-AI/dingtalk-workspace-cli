#!/usr/bin/env python3
"""yida_jsx_pipeline.py — JSX 全链路处理流水线（transform + field_check + lint）。

三块能力一站式聚合（按 region 分段维护）：
  1) [region: transform]    JSX 源码 → React.createElement 调用（纯 Python，零依赖）。
  2) [region: field_check]  发布前字段 ID 对账（拉表单 schema 比对 JSX 引用）。
  3) [region: lint]         宜搭专属运行时静态检查（30 条规则）。

公开 API（被 yida_custom_page_update.py 直接 import）：
  - transform_jsx(source) -> str
  - field_check(code, app, *, schema_fetcher=None) -> dict
  - lint_yida_source(source, filename=None) -> dict
  - lint_check(source, filename=None) -> dict
  - extract_form_uuids / extract_field_ids / fetch_form_fields  （field_check 辅助）
  - EVENT_NAME_ALIASES / THEN_CALLBACK_LINE_LIMIT / CALLBACK_SCAN_LINE_LIMIT （lint 常量）

CLI：
  python yida_jsx_pipeline.py transform <jsx_file>
  python yida_jsx_pipeline.py field-check --app APP_X --code-file page.jsx
  python yida_jsx_pipeline.py lint page.jsx [--json] [--stdin]
"""
from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path
from typing import Callable, Iterable

# ===========================================================================
# region: transform —— JSX → React.createElement
# ===========================================================================

_VOID_ELEMENTS = frozenset([
    'area', 'base', 'br', 'col', 'embed', 'hr', 'img', 'input',
    'link', 'meta', 'param', 'source', 'track', 'wbr',
])

_JSX_TAG_START = re.compile(r'<([A-Za-z_][A-Za-z0-9_.]*|>)')


def transform_jsx(source):
    """将 JSX 源码中的 JSX 表达式转换为 React.createElement 调用。

    Args:
        source: 包含 JSX 的 JavaScript 源码字符串

    Returns:
        转换后不含 JSX 的 JavaScript 源码
    """
    transformer = _JsxTransformer(source)
    return transformer.transform()


class _JsxTransformer:
    """递归下降 JSX 转换器"""

    def __init__(self, source):
        self.source = source
        self.pos = 0
        self.output = []

    def transform(self):
        while self.pos < len(self.source):
            if self._at_jsx_start():
                jsx_code = self._parse_jsx_element()
                self.output.append(jsx_code)
            else:
                self.output.append(self.source[self.pos])
                self.pos += 1
        return ''.join(self.output)

    def _peek(self, offset=0):
        idx = self.pos + offset
        if idx < len(self.source):
            return self.source[idx]
        return ''

    def _remaining(self):
        return self.source[self.pos:]

    def _at_jsx_start(self):
        """判断当前 < 是否是 JSX 标签开始（而非比较运算符）"""
        if self._peek() != '<':
            return False
        next_char = self._peek(1)
        if next_char == '>' or next_char == '/':
            return self._is_jsx_context()
        if next_char.isalpha() or next_char == '_':
            return self._is_jsx_context()
        return False

    def _is_jsx_context(self):
        """通过前文判断当前 < 是否处于 JSX 可出现的上下文"""
        preceding = ''.join(self.output).rstrip()
        if not preceding:
            # 在递归调用中（如表达式容器 {<div>}），output 为空但来源是合法 JSX
            # 检查 source 前面是否刚从表达式容器开始
            if self.pos == 0:
                return True
            return False
        last_char = preceding[-1]
        # JSX 可出现在 return/( 之后、赋值/逗号/冒号/三元之后、逻辑运算符之后
        if last_char in ('(', ',', ':', '?', '=', '&', '|', '!', ';', '{', '[', '\n'):
            return True
        # return <xxx
        if preceding.endswith('return'):
            return True
        # 箭头函数 => <xxx
        if preceding.endswith('=>'):
            return True
        if last_char == '>':
            return False
        return False

    def _skip_whitespace(self):
        while self.pos < len(self.source) and self.source[self.pos] in ' \t\n\r':
            self.pos += 1

    def _parse_jsx_element(self):
        """解析一个完整的 JSX 元素，返回 createElement 调用字符串"""
        assert self.source[self.pos] == '<'
        self.pos += 1  # skip <

        # Fragment: <>...</>
        if self._peek() == '>':
            self.pos += 1  # skip >
            children = self._parse_jsx_children('')
            # expect </>
            if self.source[self.pos:self.pos + 3] == '</>':
                self.pos += 3
            return self._emit_create_element('React.Fragment', 'null', children)

        # 解析标签名
        tag_name = self._parse_tag_name()

        # 解析属性
        attrs = self._parse_attributes()

        # 自闭合 /> 或 >
        self._skip_whitespace()
        if self.source[self.pos:self.pos + 2] == '/>':
            self.pos += 2
            props_str = self._attrs_to_props(attrs)
            return self._emit_create_element(self._tag_ref(tag_name), props_str, [])

        assert self.source[self.pos] == '>', f"Expected > at pos {self.pos}, got: {self.source[self.pos:self.pos+20]}"
        self.pos += 1  # skip >

        # 解析子元素
        children = self._parse_jsx_children(tag_name)

        # 解析闭合标签 </tagName>
        self._parse_closing_tag(tag_name)

        props_str = self._attrs_to_props(attrs)
        return self._emit_create_element(self._tag_ref(tag_name), props_str, children)

    def _parse_tag_name(self):
        start = self.pos
        while self.pos < len(self.source) and (self.source[self.pos].isalnum() or self.source[self.pos] in '_.'):
            self.pos += 1
        return self.source[start:self.pos]

    def _parse_attributes(self):
        """解析 JSX 属性列表，返回 [(name, value_str), ...]"""
        attrs = []
        while self.pos < len(self.source):
            self._skip_whitespace()
            ch = self._peek()
            if ch == '/' or ch == '>':
                break
            attr_name = self._parse_attribute_name()
            if not attr_name:
                break

            self._skip_whitespace()
            if self._peek() == '=':
                self.pos += 1  # skip =
                self._skip_whitespace()
                value = self._parse_attribute_value()
                attrs.append((attr_name, value))
            else:
                # boolean attribute
                attrs.append((attr_name, 'true'))
        return attrs

    def _parse_attribute_name(self):
        start = self.pos
        while self.pos < len(self.source):
            ch = self.source[self.pos]
            if ch.isalnum() or ch in '_-':
                self.pos += 1
            else:
                break
        return self.source[start:self.pos]

    def _parse_attribute_value(self):
        """解析属性值，返回 JS 表达式字符串"""
        ch = self._peek()
        if ch == '"':
            return self._parse_string_literal('"')
        if ch == "'":
            return self._parse_string_literal("'")
        if ch == '{':
            return self._parse_jsx_expression()
        return 'true'

    def _parse_string_literal(self, quote):
        self.pos += 1  # skip opening quote
        result = [quote]
        while self.pos < len(self.source):
            ch = self.source[self.pos]
            if ch == '\\':
                result.append(ch)
                self.pos += 1
                if self.pos < len(self.source):
                    result.append(self.source[self.pos])
                    self.pos += 1
            elif ch == quote:
                result.append(ch)
                self.pos += 1
                break
            else:
                result.append(ch)
                self.pos += 1
        return ''.join(result)

    def _parse_jsx_expression(self):
        """解析 { ... } 表达式容器，返回内部表达式字符串（可能含嵌套 JSX）"""
        assert self.source[self.pos] == '{'
        self.pos += 1  # skip {
        content = self._read_balanced_braces()
        # 递归转换内部 JSX
        inner_transformed = transform_jsx(content)
        return inner_transformed

    def _read_balanced_braces(self):
        """从当前位置读取到匹配 } 为止的内容（处理嵌套 {} 和字符串）"""
        depth = 1
        result = []
        while self.pos < len(self.source) and depth > 0:
            ch = self.source[self.pos]
            if ch in ('"', "'", '`'):
                string_content = self._consume_string_in_expr(ch)
                result.append(string_content)
            elif ch == '{':
                depth += 1
                result.append(ch)
                self.pos += 1
            elif ch == '}':
                depth -= 1
                if depth > 0:
                    result.append(ch)
                self.pos += 1
            elif ch == '/' and self.pos + 1 < len(self.source):
                next_ch = self.source[self.pos + 1]
                if next_ch == '/':
                    comment = self._consume_line_comment()
                    result.append(comment)
                elif next_ch == '*':
                    comment = self._consume_block_comment()
                    result.append(comment)
                else:
                    result.append(ch)
                    self.pos += 1
            else:
                result.append(ch)
                self.pos += 1
        return ''.join(result)

    def _consume_string_in_expr(self, quote):
        """消费字符串字面量（含转义），返回完整字符串"""
        result = [quote]
        self.pos += 1
        if quote == '`':
            return self._consume_template_literal(result)
        while self.pos < len(self.source):
            ch = self.source[self.pos]
            result.append(ch)
            self.pos += 1
            if ch == '\\' and self.pos < len(self.source):
                result.append(self.source[self.pos])
                self.pos += 1
            elif ch == quote:
                break
        return ''.join(result)

    def _consume_template_literal(self, result):
        """消费模板字面量 `...${expr}...`"""
        while self.pos < len(self.source):
            ch = self.source[self.pos]
            result.append(ch)
            self.pos += 1
            if ch == '\\' and self.pos < len(self.source):
                result.append(self.source[self.pos])
                self.pos += 1
            elif ch == '`':
                break
            elif ch == '$' and self.pos < len(self.source) and self.source[self.pos] == '{':
                result.append('{')
                self.pos += 1
                # 读到匹配的 }
                depth = 1
                while self.pos < len(self.source) and depth > 0:
                    c = self.source[self.pos]
                    result.append(c)
                    self.pos += 1
                    if c == '{':
                        depth += 1
                    elif c == '}':
                        depth -= 1
        return ''.join(result)

    def _consume_line_comment(self):
        start = self.pos
        while self.pos < len(self.source) and self.source[self.pos] != '\n':
            self.pos += 1
        return self.source[start:self.pos]

    def _consume_block_comment(self):
        start = self.pos
        self.pos += 2  # skip /*
        while self.pos < len(self.source) - 1:
            if self.source[self.pos] == '*' and self.source[self.pos + 1] == '/':
                self.pos += 2
                break
            self.pos += 1
        else:
            self.pos = len(self.source)
        return self.source[start:self.pos]

    def _parse_jsx_children(self, parent_tag):
        """解析子元素直到遇到 </parent_tag> 或 </>"""
        children = []
        text_buf = []

        while self.pos < len(self.source):
            # 检查闭合标签
            if parent_tag and self.source[self.pos:self.pos + 2] == '</':
                break
            if not parent_tag and self.source[self.pos:self.pos + 3] == '</>':
                break

            ch = self.source[self.pos]

            if ch == '{':
                # 先 flush text
                if text_buf:
                    text = ''.join(text_buf).strip()
                    if text:
                        children.append(repr(text))
                    text_buf = []
                # 表达式子元素
                self.pos += 1
                expr_content = self._read_balanced_braces()
                expr_transformed = transform_jsx(expr_content)
                stripped = expr_transformed.strip()
                # 跳过纯 JSX 注释表达式 {/* ... */} 或 {// ...}
                # 否则会在 React.createElement 子节点列表里产生空位导致连续逗号 ,,
                comment_stripped = re.sub(r'/\*[\s\S]*?\*/', '', stripped)
                comment_stripped = re.sub(r'//[^\n]*', '', comment_stripped).strip()
                if not comment_stripped:
                    continue
                children.append(stripped)

            elif ch == '<':
                # 先 flush text
                if text_buf:
                    text = ''.join(text_buf).strip()
                    if text:
                        children.append(repr(text))
                    text_buf = []
                # 嵌套子元素
                child_element = self._parse_jsx_element()
                children.append(child_element)
            else:
                text_buf.append(ch)
                self.pos += 1

        # flush remaining text
        if text_buf:
            text = ''.join(text_buf).strip()
            if text:
                children.append(repr(text))

        return children

    def _parse_closing_tag(self, expected_tag):
        """解析 </tagName>"""
        if self.source[self.pos:self.pos + 2] != '</':
            return
        self.pos += 2  # skip </
        tag = self._parse_tag_name()
        self._skip_whitespace()
        if self.pos < len(self.source) and self.source[self.pos] == '>':
            self.pos += 1

    def _tag_ref(self, tag_name):
        """标签名转为 createElement 第一参数"""
        if tag_name[0].isupper():
            return tag_name  # 组件引用
        return repr(tag_name)  # HTML 元素用字符串

    def _attrs_to_props(self, attrs):
        """将属性列表转为 props 对象字符串"""
        if not attrs:
            return 'null'
        pairs = []
        for name, value in attrs:
            prop_name = self._normalize_attr_name(name)
            pairs.append(f'{prop_name}: {value}')
        return '{' + ', '.join(pairs) + '}'

    def _normalize_attr_name(self, name):
        """HTML 属性名 → JS 属性名"""
        mapping = {
            'class': 'className',
            'for': 'htmlFor',
            'tabindex': 'tabIndex',
            'readonly': 'readOnly',
            'maxlength': 'maxLength',
            'colspan': 'colSpan',
            'rowspan': 'rowSpan',
            'enctype': 'encType',
            'contenteditable': 'contentEditable',
            'crossorigin': 'crossOrigin',
            'accesskey': 'accessKey',
            'autocomplete': 'autoComplete',
            'autofocus': 'autoFocus',
            'autoplay': 'autoPlay',
        }
        normalized = mapping.get(name.lower(), name)
        # 带 - 的属性名需要引号
        if '-' in normalized and not normalized.startswith('data-') and not normalized.startswith('aria-'):
            return repr(normalized)
        if '-' in normalized:
            return repr(normalized)
        return normalized

    def _emit_create_element(self, tag, props, children):
        """生成 React.createElement 调用"""
        args = [tag, props]
        args.extend(children)
        return 'React.createElement(' + ', '.join(args) + ')'


# ===========================================================================
# region: field_check —— 发布前字段 ID 对账
# ===========================================================================

# JSX 里的表单引用：'FORM-XXX' 字面量
_FORM_UUID_RE = re.compile(r"['\"](FORM-[A-Z0-9]+)['\"]")

# 字段 ID 命名规范：以 Field/SelectField/... 结尾再接下划线 + 大小写数字
# 用宽松前缀（任何字母）+ 强约束的 fieldId 命名格式（见 yida-custom-page-codegen.md）
_FIELD_ID_LITERAL_RE = re.compile(r"['\"]([a-zA-Z][a-zA-Z0-9]*Field_[A-Za-z0-9]+)['\"]")


def extract_form_uuids(code: str) -> set[str]:
    """提取 JSX 中所有 'FORM-XXX' 字面量。"""
    return set(_FORM_UUID_RE.findall(code))


def extract_field_ids(code: str) -> set[str]:
    """提取 JSX 中所有疑似字段 ID 的字符串字面量（xxxField_yyyy 形式）。"""
    return set(_FIELD_ID_LITERAL_RE.findall(code))


def _run_dws(args: list[str]) -> tuple[object | None, str | None]:
    try:
        result = subprocess.run(["dws"] + args, capture_output=True, text=True, timeout=120)
    except FileNotFoundError:
        return None, "找不到 'dws' 命令（请确认已安装并在 PATH）"
    except subprocess.TimeoutExpired:
        return None, "dws 调用超时"
    if result.returncode != 0:
        err = result.stderr.strip() or result.stdout.strip()
        return None, f"dws 失败 (exit {result.returncode}): {err[:200]}"
    try:
        return json.loads(result.stdout), None
    except json.JSONDecodeError as e:
        return None, f"输出非 JSON: {e}"


_FIELD_ID_VALUE_RE = re.compile(r'\b[A-Za-z]+Field_[A-Za-z0-9]+\b')
_COMPONENT_LIST_KEYS = ("components", "fields", "data", "items", "result", "children", "list")


def _walk_values(obj):
    if isinstance(obj, dict):
        yield obj
        for value in obj.values():
            yield from _walk_values(value)
    elif isinstance(obj, list):
        for item in obj:
            yield from _walk_values(item)


def _find_nested_value(obj, keys):
    if isinstance(obj, dict):
        for key in keys:
            value = obj.get(key)
            if value not in (None, ""):
                return value
        for value in obj.values():
            found = _find_nested_value(value, keys)
            if found not in (None, ""):
                return found
    elif isinstance(obj, list):
        for item in obj:
            found = _find_nested_value(item, keys)
            if found not in (None, ""):
                return found
    return None


def _normalize_i18n_label(label):
    if isinstance(label, str):
        raw = label.strip()
        if raw.startswith("{") and raw.endswith("}"):
            try:
                parsed = json.loads(raw)
            except json.JSONDecodeError:
                return label
            if isinstance(parsed, dict):
                return (parsed.get("zh_CN") or parsed.get("zh-CN")
                        or parsed.get("text") or parsed.get("pureEn_US")
                        or parsed.get("en_US") or label)
        return label
    if isinstance(label, dict):
        return (label.get("zh_CN") or label.get("zh-CN") or label.get("text")
                or label.get("pureEn_US") or label.get("en_US") or "")
    return label or ""


def _find_field_id(obj):
    value = _find_nested_value(obj, ("fieldId", "fieldCode", "field_id", "fieldKey"))
    if isinstance(value, str):
        m = _FIELD_ID_VALUE_RE.search(value)
        if m:
            return m.group(0)
        return value

    key_value = _find_nested_value(obj, ("key", "name", "id"))
    if isinstance(key_value, str):
        m = _FIELD_ID_VALUE_RE.search(key_value)
        if m:
            return m.group(0)

    text = json.dumps(obj, ensure_ascii=False) if isinstance(obj, (dict, list)) else str(obj)
    m = _FIELD_ID_VALUE_RE.search(text)
    return m.group(0) if m else None


def _extract_component_items(data):
    if isinstance(data, dict):
        for key in _COMPONENT_LIST_KEYS:
            value = data.get(key)
            if isinstance(value, list):
                return value
            if isinstance(value, dict):
                nested = _extract_component_items(value)
                if nested:
                    return nested
    if isinstance(data, list):
        return data

    found = []
    seen = set()
    for item in _walk_values(data):
        if not isinstance(item, dict):
            continue
        fid = _find_field_id(item)
        if fid and fid not in seen:
            seen.add(fid)
            found.append(item)
    return found


def fetch_form_fields(app: str, form_uuid: str) -> tuple[list[dict] | None, str | None]:
    """调用 dws yida form components 拿表单字段。"""
    data, err = _run_dws(["yida", "form", "components", "--app", app,
                          "--form", form_uuid, "--format", "json"])
    if err:
        return None, err
    return _extract_component_items(data), None


def _normalize_field(comp: dict) -> dict:
    field_id = _find_field_id(comp)
    label = (_find_nested_value(comp, ("label", "title", "text", "displayName", "nameCn"))
             or "")
    label = _normalize_i18n_label(label)
    return {
        "fieldId": field_id,
        "label": label,
        "componentName": (_find_nested_value(comp, ("componentName", "type", "component"))
                          or ""),
    }


def field_check(code: str, app: str, *,
                schema_fetcher: Callable[[str], tuple[list[dict] | None, str | None]] | None = None) -> dict:
    """字段 ID 对账。

    Args:
        code: JSX 源码
        app: 应用 appType
        schema_fetcher: 可注入的字段拉取函数（测试用），签名 (form_uuid)->(fields, err)

    Returns:
        {
          ok: bool,
          errors: [{type, fieldId?, message}],
          warnings: [{type, message}],
          info: {checkedForms, referencedFieldCount, knownFieldCount, missing}
        }
    """
    fetcher = schema_fetcher or (lambda fu: fetch_form_fields(app, fu))

    form_uuids = extract_form_uuids(code)
    referenced = extract_field_ids(code)

    if not form_uuids:
        return {
            "ok": True, "errors": [], "warnings": [],
            "info": {"skipped": "no_form_uuid_in_jsx",
                     "referencedFieldCount": len(referenced)},
        }

    if not referenced:
        return {
            "ok": True, "errors": [], "warnings": [],
            "info": {"skipped": "no_field_id_referenced",
                     "checkedForms": sorted(form_uuids)},
        }

    all_known: dict[str, dict] = {}
    fetch_errors: list[dict] = []
    for fu in sorted(form_uuids):
        fields, err = fetcher(fu)
        if err or fields is None:
            fetch_errors.append({"form": fu, "error": err or "unknown"})
            continue
        for c in fields:
            f = _normalize_field(c)
            if f["fieldId"]:
                all_known[f["fieldId"]] = {**f, "form": fu}

    # 所有表单都拉不到 → 无法对账，作为 warning 放行（避免登录态过期把发布卡死）
    if fetch_errors and not all_known:
        return {
            "ok": True,
            "errors": [],
            "warnings": [{
                "type": "schema_fetch_all_failed",
                "message": ("无法拉取任何表单 schema，跳过字段对账。"
                            "可能原因：登录态过期 / 网络异常 / form_uuid 不属于当前应用。"
                            f"详情：{fetch_errors}"),
            }],
            "info": {"checkedForms": sorted(form_uuids),
                     "referencedFieldCount": len(referenced),
                     "knownFieldCount": 0},
        }

    missing = sorted(referenced - all_known.keys())
    errors: list[dict] = []
    if missing:
        forms_hint = ", ".join(sorted(form_uuids))
        for fid in missing:
            errors.append({
                "type": "field_id_not_found",
                "fieldId": fid,
                "message": (
                    f"字段 ID `{fid}` 在表单 schema 中不存在。\n"
                    f"        已检查的表单：{forms_hint}\n"
                    f"        修复建议：\n"
                    f"          1) 用 `python yida_form_inspector.py --action fields-snippet "
                    f"--app {app} --form <FORM-XXX>` 重新生成 FIELDS 常量\n"
                    f"          2) 或核对 var FORM_UUID 是否填错"
                ),
            })

    warnings: list[dict] = []
    for fe in fetch_errors:
        warnings.append({
            "type": "schema_fetch_partial",
            "message": (f"表单 {fe['form']} schema 拉取失败（部分对账已跳过）：{fe['error']}"),
        })

    return {
        "ok": len(errors) == 0,
        "errors": errors,
        "warnings": warnings,
        "info": {
            "checkedForms": sorted(form_uuids),
            "referencedFieldCount": len(referenced),
            "knownFieldCount": len(all_known),
            "missing": missing,
        },
    }


# ===========================================================================
# region: lint —— 宜搭专属运行时静态检查（30 条规则）
# ===========================================================================

THEN_CALLBACK_LINE_LIMIT = 50
CALLBACK_SCAN_LINE_LIMIT = 80

EVENT_NAME_ALIASES = {
    'onclick': 'onClick', 'onchange': 'onChange', 'oninput': 'onChange',
    'onsubmit': 'onSubmit', 'onkeydown': 'onKeyDown', 'onkeyup': 'onKeyUp',
    'onkeypress': 'onKeyPress', 'onfocus': 'onFocus', 'onblur': 'onBlur',
    'onmouseenter': 'onMouseEnter', 'onmouseleave': 'onMouseLeave',
    'onmousedown': 'onMouseDown', 'onmouseup': 'onMouseUp',
    'onmousemove': 'onMouseMove', 'oncompositionstart': 'onCompositionStart',
    'oncompositionend': 'onCompositionEnd',
}


# ---- helpers ----

def _is_in_comment_or_string(line: str, match_index: int) -> bool:
    """判断行内某偏移是否在 // 注释或字符串中（简化版）。"""
    if match_index is None or match_index < 0:
        return False
    before = line[:match_index]
    if '//' in before:
        return True
    quotes = sum(1 for ch in before if ch in ("'", '"'))
    return quotes % 2 != 0


def _strip_comments_and_strings(source: str) -> str:
    """剥离 /**/ // 块/行注释 和 ' " ` 字符串，便于纯结构扫描。"""
    out = re.sub(r'/\*[\s\S]*?\*/', '', source)
    out = re.sub(r'//.*$', '', out, flags=re.M)
    out = re.sub(r"'(?:\\.|[^'\\])*'", "''", out)
    out = re.sub(r'"(?:\\.|[^"\\])*"', '""', out)
    out = re.sub(r'`(?:\\.|[^`\\])*`', '``', out)
    return out


def _extract_function_body(lines: list[str], start_line_index: int, function_index: int) -> str:
    """从 lines[start_line_index] 起，找到第一个 `{` 并按 brace 深度抽出函数体（最多扫 80 行）。"""
    end_line_index = min(len(lines), start_line_index + CALLBACK_SCAN_LINE_LIMIT)
    source = '\n'.join(lines[start_line_index:end_line_index])
    open_brace_index = source.find('{', function_index)
    if open_brace_index < 0:
        return ''

    brace_depth = 0
    quote: str | None = None
    in_line_comment = False
    in_block_comment = False

    i = open_brace_index
    n = len(source)
    while i < n:
        ch = source[i]
        nxt = source[i + 1] if i + 1 < n else ''

        if in_line_comment:
            if ch == '\n':
                in_line_comment = False
            i += 1
            continue
        if in_block_comment:
            if ch == '*' and nxt == '/':
                in_block_comment = False
                i += 2
                continue
            i += 1
            continue
        if quote:
            if ch == '\\':
                i += 2
                continue
            if ch == quote:
                quote = None
            i += 1
            continue
        if ch == '/' and nxt == '/':
            in_line_comment = True
            i += 2
            continue
        if ch == '/' and nxt == '*':
            in_block_comment = True
            i += 2
            continue
        if ch in ("'", '"', '`'):
            quote = ch
            i += 1
            continue
        if ch == '{':
            brace_depth += 1
        elif ch == '}':
            brace_depth -= 1
            if brace_depth == 0:
                return source[open_brace_index + 1:i]
        i += 1
    return source[open_brace_index + 1:]


def _function_callback_uses_this(lines: list[str], line_index: int, match_index: int) -> bool:
    line = lines[line_index]
    fn_idx = line.find('function', match_index)
    if fn_idx < 0:
        return False
    body = _extract_function_body(lines, line_index, fn_idx)
    return bool(re.search(r'\bthis\b', _strip_comments_and_strings(body)))


def _extract_named_export_function_body(source: str, name: str) -> tuple[int, str] | None:
    """Extract body of `export function name(...) { ... }` from full source."""
    m = re.search(r'\bexport\s+function\s+' + re.escape(name) + r'\s*\(', source)
    if not m:
        return None
    open_brace = source.find('{', m.end())
    if open_brace < 0:
        return None

    brace_depth = 0
    quote: str | None = None
    in_line_comment = False
    in_block_comment = False
    i = open_brace
    n = len(source)
    while i < n:
        ch = source[i]
        nxt = source[i + 1] if i + 1 < n else ''
        if in_line_comment:
            if ch == '\n':
                in_line_comment = False
            i += 1
            continue
        if in_block_comment:
            if ch == '*' and nxt == '/':
                in_block_comment = False
                i += 2
                continue
            i += 1
            continue
        if quote:
            if ch == '\\':
                i += 2
                continue
            if ch == quote:
                quote = None
            i += 1
            continue
        if ch == '/' and nxt == '/':
            in_line_comment = True
            i += 2
            continue
        if ch == '/' and nxt == '*':
            in_block_comment = True
            i += 2
            continue
        if ch in ("'", '"', '`'):
            quote = ch
            i += 1
            continue
        if ch == '{':
            brace_depth += 1
        elif ch == '}':
            brace_depth -= 1
            if brace_depth == 0:
                start_line = source.count('\n', 0, open_brace) + 1
                return start_line, source[open_brace + 1:i]
        i += 1
    start_line = source.count('\n', 0, open_brace) + 1
    return start_line, source[open_brace + 1:]


# ---- disable map ----

_DISABLE_LINE_RE = re.compile(
    r'(?:dws|openyida)-lint-disable-line(?:\s+([a-z0-9_,\-\s]+))?', re.I)
_DISABLE_NEXT_RE = re.compile(
    r'(?:dws|openyida)-lint-disable-next-line(?:\s+([a-z0-9_,\-\s]+))?', re.I)


def _parse_disable_rules(raw: str | None) -> list[str]:
    if not raw or not raw.strip():
        return ['*']
    return [r.strip() for r in re.split(r'[,\s]+', raw) if r.strip()]


def _build_disable_map(lines: list[str]) -> dict[int, set[str]]:
    dmap: dict[int, set[str]] = {}
    for idx, line in enumerate(lines):
        line_no = idx + 1
        m = _DISABLE_LINE_RE.search(line)
        if m:
            dmap.setdefault(line_no, set()).update(_parse_disable_rules(m.group(1)))
        m2 = _DISABLE_NEXT_RE.search(line)
        if m2:
            dmap.setdefault(line_no + 1, set()).update(_parse_disable_rules(m2.group(1)))
    return dmap


def _is_rule_disabled(dmap: dict[int, set[str]], line: int, rule: str) -> bool:
    rules = dmap.get(line)
    if not rules:
        return False
    return '*' in rules or rule in rules


def _push_issue(lst: list[dict], line: int, rule: str, message: str,
                dmap: dict[int, set[str]]) -> None:
    if _is_rule_disabled(dmap, line, rule):
        return
    for it in lst:
        if it['line'] == line and it['rule'] == rule and it['message'] == message:
            return
    lst.append({'line': line, 'rule': rule, 'message': message})


# ---- aux detectors ----

_THEN_START_RE = re.compile(
    r'\.then\s*\(\s*(?:function\s*\(|(?:\([^)]*\)|[a-zA-Z_$]\w*)\s*=>)')


def _detect_large_then_callbacks(lines: list[str]) -> list[dict]:
    results: list[dict] = []
    in_then = False
    brace_depth = 0
    then_start_line = 0
    then_body_start_line = 0

    for i, line in enumerate(lines):
        trimmed = line.strip()
        if trimmed.startswith('//') or trimmed.startswith('*') or trimmed.startswith('/*'):
            continue
        if not in_then:
            m = _THEN_START_RE.search(line)
            if m and not _is_in_comment_or_string(line, m.start()):
                in_then = True
                then_start_line = i + 1
                brace_depth = 0
                after = line[m.start():]
                for ch in after:
                    if ch == '{':
                        brace_depth += 1
                    elif ch == '}':
                        brace_depth -= 1
                then_body_start_line = i + 1
        else:
            for ch in line:
                if ch == '{':
                    brace_depth += 1
                elif ch == '}':
                    brace_depth -= 1
            if brace_depth <= 0:
                callback_line_count = (i + 1) - then_body_start_line
                if callback_line_count > THEN_CALLBACK_LINE_LIMIT:
                    results.append({'line': then_start_line, 'lineCount': callback_line_count})
                in_then = False
    return results


_YIDA_CALL_RE = re.compile(r'this\.utils\.yida\.[A-Za-z_$][\w$]*\s*\(')


def _detect_yida_calls_without_catch(source: str, warnings: list[dict],
                                      dmap: dict[int, set[str]]) -> None:
    for m in _YIDA_CALL_RE.finditer(source):
        line = source.count('\n', 0, m.start()) + 1
        statement = source[m.start():m.start() + 600]
        if '.catch(' not in statement:
            _push_issue(warnings, line, 'yida-api-catch',
                        '调用 this.utils.yida.* 应跟 .catch() 防止 Promise 异常吞没', dmap)


_LABEL_FORMATTER_RE = re.compile(
    r'\blabel\s*:\s*\{[\s\S]{0,1200}?\bformatter\s*:\s*function\b')


def _detect_echarts_rich_label_formatter(source: str, warnings: list[dict],
                                          dmap: dict[int, set[str]]) -> None:
    for m in _LABEL_FORMATTER_RE.finditer(source):
        line = source.count('\n', 0, m.start()) + 1
        block = source[m.start():m.start() + 1600]
        uses_rich = bool(re.search(r'\brich\s*:', block) or
                         re.search(r"return\s+['\"`][\s\S]{0,300}?\{[A-Za-z0-9_]+\|", block))
        if uses_rich:
            _push_issue(warnings, line, 'echarts-rich-label-formatter',
                        'echarts label.formatter 使用 rich 模板时建议改用对象写法或 string formatter', dmap)


# ---- lifecycle scan (replaces AST ExportNamedDeclaration / ClassMethod) ----

_EXPORT_FN_RE = re.compile(r'\bexport\s+function\s+([A-Za-z][\w]*)\s*\(')
_LIFECYCLE_DECL_RE = re.compile(
    r'(?:^|[\s;{])(componentDidMount|componentWillUnmount)\s*\(')


def _scan_lifecycle(source: str, errors: list[dict], dmap: dict[int, set[str]]) -> None:
    for m in _EXPORT_FN_RE.finditer(source):
        name = m.group(1)
        line = source.count('\n', 0, m.start()) + 1
        if name.lower() == 'didmount' and name != 'didMount':
            _push_issue(errors, line, 'lifecycle-case',
                        f'生命周期函数大小写错误：{name} 应为 didMount', dmap)
        elif name.lower() == 'didunmount' and name != 'didUnmount':
            _push_issue(errors, line, 'lifecycle-case',
                        f'生命周期函数大小写错误：{name} 应为 didUnmount', dmap)
        if name in ('componentDidMount', 'componentWillUnmount'):
            expected = 'didMount' if name == 'componentDidMount' else 'didUnmount'
            _push_issue(errors, line, 'react-lifecycle-method',
                        f'宜搭页面不使用 React 生命周期：{name} 应为 {expected}', dmap)
    # 类方法 / 对象方法形式（避免重复行）
    for m in _LIFECYCLE_DECL_RE.finditer(source):
        name = m.group(1)
        line = source.count('\n', 0, m.start()) + 1
        expected = 'didMount' if name == 'componentDidMount' else 'didUnmount'
        _push_issue(errors, line, 'react-lifecycle-method',
                    f'宜搭页面不使用 React 生命周期：{name} 应为 {expected}', dmap)


# ---- jsx element scan (replaces AST JSXOpeningElement) ----

# JSX 起始标签匹配：必须自己处理 brace/quote 平衡，避免 `=>` 中的 `>` 被误当作
# 标签结束符（这是 [^<>] 简化正则的固有缺陷）。
_JSX_TAG_NAME_RE = re.compile(r'<([a-zA-Z][\w.\-]*)')
# 在属性串内找事件属性 onXxx=
_EVENT_ATTR_RE = re.compile(r'\b(on[A-Za-z]\w*)\s*=\s*(\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}|"[^"]*"|\'[^\']*\')')
_LOWER_EVENT_RE = re.compile(r'\bon[a-z]\w+\b')


def _iter_jsx_open_tags(source: str):
    """逐个 yield JSX 起始标签 `(start_pos, tag_name, attr_str)`。

    自己跟踪 `{}` 深度和字符串边界，避免 `() => x` 中的 `>` 被误当作标签结束。
    """
    n = len(source)
    i = 0
    while i < n:
        m = _JSX_TAG_NAME_RE.match(source, i)
        if not m:
            i += 1
            continue
        tag_name = m.group(1)
        j = m.end()
        brace = 0
        quote: str | None = None
        end_pos = -1
        while j < n:
            ch = source[j]
            nxt = source[j + 1] if j + 1 < n else ''
            if quote:
                if ch == '\\':
                    j += 2
                    continue
                if ch == quote:
                    quote = None
                j += 1
                continue
            if ch in ("'", '"', '`'):
                quote = ch
                j += 1
                continue
            if ch == '{':
                brace += 1
            elif ch == '}':
                brace -= 1
            elif ch == '<' and brace == 0:
                # 嵌套标签，提前结束当前扫描
                break
            elif ch == '>' and brace == 0:
                end_pos = j
                break
            j += 1
        if end_pos < 0:
            i = m.end()
            continue
        attr_str = source[m.end():end_pos]
        # 兼容自闭合：<tag .../> 的最后 / 落在 attr_str 末尾，去掉
        if attr_str.endswith('/'):
            attr_str = attr_str[:-1]
        yield (m.start(), tag_name, attr_str)
        i = end_pos + 1


def _attrs_split(attr_str: str) -> dict[str, str]:
    """粗略拆解属性串，返回 {name: rawValue}。仅用于 button 缺 handler 检测。"""
    if not attr_str:
        return {}
    result: dict[str, str] = {}
    # 匹配 name="..."  name='...'  name={...}  name (布尔属性)
    pattern = re.compile(
        r'([a-zA-Z][\w-]*)(?:\s*=\s*("[^"]*"|\'[^\']*\'|\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}))?')
    for m in pattern.finditer(attr_str):
        name = m.group(1)
        value = m.group(2) or ''
        result[name] = value
    return result


def _is_interactive_button(attrs: dict[str, str]) -> bool:
    if 'disabled' in attrs or 'aria-disabled' in attrs:
        return True
    btn_type = attrs.get('type', '').strip('"\'').lower()
    if btn_type == 'submit':
        return True
    for k in ('onClick', 'onMouseDown', 'onKeyDown'):
        if k in attrs:
            return True
    return False


def _canonical_event_name(name: str) -> str:
    if name in EVENT_NAME_ALIASES:
        return EVENT_NAME_ALIASES[name]
    if not name or len(name) <= 2:
        return name
    return 'on' + name[2].upper() + name[3:]


_BARE_HANDLER_IDENT_RE = re.compile(r'^(?:handle|on)[A-Z]\w*$')
_THIS_OR_SELF_MEMBER_RE = re.compile(r'^(?:this|self)\.[A-Za-z_$][\w$]*$')
_BIND_THIS_RE = re.compile(r'\.bind\s*\(\s*this\s*\)\s*$')


def _expr_is_bare_handler_ref(expr: str) -> bool:
    expr = expr.strip()
    if _THIS_OR_SELF_MEMBER_RE.match(expr):
        return True
    if _BARE_HANDLER_IDENT_RE.match(expr):
        return True
    return False


def _arrow_returns_bare_handler(value: str) -> bool:
    """识别 {() => this.foo} / {(a) => handleX} / {() => { this.foo; }} 这类无效箭头。"""
    inner = value.strip()
    if inner.startswith('{') and inner.endswith('}'):
        inner = inner[1:-1].strip()
    # 匹配箭头函数：(...) => body  或  ident => body
    m = re.match(r'(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*=>\s*(.+)$', inner, re.S)
    if not m:
        return False
    body = m.group(1).strip()
    if body.startswith('{') and body.endswith('}'):
        body = body[1:-1].strip().rstrip(';').strip()
        # 多语句：取最后一个表达式判断
        parts = [p.strip() for p in re.split(r';|\n', body) if p.strip()]
        return any(_expr_is_bare_handler_ref(p) for p in parts)
    return _expr_is_bare_handler_ref(body)


# 复用行扫描的 message 模板，确保和 _DIRECT_METHOD_RE/_BIND_INLINE_RE 触发的告警
# 内容完全一致 → _push_issue 的 (line, rule, message) 去重生效。
_MSG_DIRECT_METHOD = 'onXxx={this.foo} 会导致 this 上下文丢失，请改用箭头函数'
_MSG_BIND_THIS = 'onXxx={fn.bind(this)} 在宜搭运行时不稳定，请改用箭头函数'


def _scan_jsx_elements(source: str, errors: list[dict], warnings: list[dict],
                       dmap: dict[int, set[str]]) -> None:
    for start_pos, tag, attr_str in _iter_jsx_open_tags(source):
        line = source.count('\n', 0, start_pos) + 1
        is_lower_tag = tag[0].islower()
        attrs = _attrs_split(attr_str)

        if is_lower_tag and tag == 'button' and not _is_interactive_button(attrs):
            _push_issue(errors, line, 'button-missing-handler',
                        '<button> 必须挂载 onClick / onMouseDown / onKeyDown 之一，'
                        '或显式 disabled / type="submit"', dmap)

        # 事件属性扫描
        for evt_match in _EVENT_ATTR_RE.finditer(attr_str):
            attr_name = evt_match.group(1)
            attr_value = evt_match.group(2) or ''
            attr_line_offset = attr_str[:evt_match.start()].count('\n')
            attr_line = line + attr_line_offset

            # event-lowercase: onclick / onchange 等小写形式
            if _LOWER_EVENT_RE.fullmatch(attr_name) and attr_name[2:3].islower():
                canon = _canonical_event_name(attr_name)
                _push_issue(errors, attr_line, 'event-lowercase',
                            f'JSX 事件属性大小写错误：{attr_name} 应为 {canon}', dmap)
                continue

            # 仅处理 onXxx 形式
            if not re.match(r'^on[A-Z]', attr_name):
                continue

            # 字符串值（非大括号包裹）：event-call-result
            if attr_value.startswith('"') or attr_value.startswith("'"):
                _push_issue(errors, attr_line, 'event-call-result',
                            f'{attr_name} 必须传函数引用，不能是字符串', dmap)
                continue

            if not (attr_value.startswith('{') and attr_value.endswith('}')):
                continue

            inner = attr_value[1:-1].strip()
            # event-direct-method: this.xxx 或 self.xxx 单独成员引用
            if _THIS_OR_SELF_MEMBER_RE.match(inner):
                _push_issue(errors, attr_line, 'event-direct-method',
                            _MSG_DIRECT_METHOD, dmap)
                continue
            # event-bind-this: xxx.bind(this)
            if _BIND_THIS_RE.search(inner):
                _push_issue(errors, attr_line, 'event-bind-this',
                            _MSG_BIND_THIS, dmap)
                continue
            # event-call-result: 形如 fn(args) 的调用结果
            if (re.search(r'\)\s*$', inner) and '=>' not in inner
                    and not inner.startswith('function')
                    and not inner.startswith('(')):
                if re.match(r'^[A-Za-z_$][\w$.]*\s*\(', inner):
                    _push_issue(errors, attr_line, 'event-call-result',
                                f'{attr_name}={{fn()}} 是把调用结果当 handler，请去掉括号',
                                dmap)
                    continue
            # event-noop-arrow: () => this.foo  或 () => { this.foo }
            if '=>' in inner and _arrow_returns_bare_handler(attr_value):
                _push_issue(errors, attr_line, 'event-noop-arrow',
                            f'{attr_name}={{() => this.foo}} 没有真正调用 handler，'
                            f'请改成 () => this.foo()', dmap)


# ---- main lint ----

_RENDERJSX_RE = re.compile(r'export\s+function\s+renderJsx\s*\(')
_REACT_HOOKS_RE = re.compile(r'\buse(State|Effect|Memo|Callback|Ref|Reducer|Context)\s*\(')
_UNSUPPORTED_HOOKS_RE = re.compile(r'\buse(Memo|Callback|Ref|Reducer|Context)\s*\(')
_USE_STATE_RE = re.compile(r'\buseState\s*\(')
_SUPPORTED_USE_STATE_PREFIX_RE = re.compile(
    r'\b(?:var|let|const)\s+\[\s*[A-Za-z_$][\w$]*\s*,\s*[A-Za-z_$][\w$]*\s*\]\s*=\s*$')
_EXPORT_DEFAULT_RE = re.compile(r'export\s+default\b')
_EXPORT_DEFAULT_PAGE_RE = re.compile(r'export\s+default\s+function\s+Page\s*\(')
_HAS_JSX_TAG_RE = re.compile(r'<[A-Za-z][\w.-]*(\s|>)')

_IMPORT_REQUIRE_RE = re.compile(r'^\s*import\s+|\brequire\s*\(')
_SUPPORTED_MODERN_IMPORT_RE = re.compile(
    r'^\s*import\b[\s\S]*?\bfrom\s+[\'"](?:react|react-dom)[\'"]\s*;?\s*$'
    r'|^\s*import\s+[\'"](?:react|react-dom)[\'"]\s*;?\s*$')
_LEGACY_ECHARTS_MAP_RE = re.compile(
    r'(?:echarts(?:\.min)?\.js/map/js/china|echarts/map/js/china|map/js/china(?:\.js)?)',
    re.I)
_EVENT_FUNCTION_RE = re.compile(r'on[A-Z]\w+=\{function\b')
_DIRECT_METHOD_RE = re.compile(r'on[A-Z]\w+=\{this\.[A-Za-z_$][\w$]*\s*\}')
_BIND_INLINE_RE = re.compile(r'on[A-Z]\w+=\{[^}]*\.bind\(this\)[^}]*\}')
_CONST_LET_RE = re.compile(r'\b(const|let)\s+')
_COMPUTED_RE = re.compile(r'\{\s*\[[^\]]+\]\s*:')
_PAD_METHOD_RE = re.compile(r'\.(padStart|padEnd)\s*\(')
_MAP_FILTER_FN_RE = re.compile(r'\.(map|filter)\s*\(\s*function\b')
_FOREACH_FN_RE = re.compile(r'\.forEach\s*\(\s*function\b')
_CONTROLLED_INPUT_RE = re.compile(r'<input\b[^>]*\bvalue=')
_NATIVE_SELECT_RE = re.compile(r'<select\b')
_IFRAME_NAV_RE = re.compile(
    r'<a\b(?=[^>]*(?:aliwork\.com|yidaapps\.com|/preview/|/workbench))'
    r'(?![^>]*\btarget=([\'"]?)_top\1)'
    r'(?![^>]*\btarget=([\'"]?)_blank\2)[^>]*>',
    re.I)
_TOP_LOCATION_RE = re.compile(
    r'window\.location\.href\s*=\s*[^;\n]*(?:aliwork\.com|yidaapps\.com|/preview/|/workbench)',
    re.I)
_PAGE_SIZE_RE = re.compile(r'\bpageSize\s*:\s*(\d+)')
_CUSTOM_STATE_DECL_RE = re.compile(r'\bvar\s+_customState\b')
_CUSTOM_STATE_REF_RE = re.compile(r'(?<![.\w$])_customState\b')
_SELF_METHOD_ASSIGN_RE = re.compile(r'\b(?:self|this)\.[A-Za-z_$][\w$]*\s*=\s*function\s*\(')
_RENDER_YIDA_API_RE = re.compile(r'\b(?:Yida\.api|(?:self|this)\.utils\.yida)\b')
_DID_MOUNT_EMULATION_RE = re.compile(r'\bdidMountCalled\b')


def _has_unsupported_use_state_pattern(source: str) -> bool:
    """Return True if any useState call is not compiler-lowerable.

    The page compiler currently lowers only declaration-form hooks:
      var [state, setState] = useState(initialValue);
    It supports multiline initial values, but the declaration prefix itself must
    be on the same line as useState so that the generated state name is stable.
    """
    for match in _USE_STATE_RE.finditer(source):
        line_start = source.rfind('\n', 0, match.start()) + 1
        prefix = source[line_start:match.start()]
        if not _SUPPORTED_USE_STATE_PREFIX_RE.search(prefix):
            return True
    return False


def _scan_render_side_effects(source: str, errors: list[dict],
                              dmap: dict[int, set[str]]) -> None:
    render = _extract_named_export_function_body(source, 'renderJsx')
    if render is None:
        return
    start_line, body = render
    clean_body = _strip_comments_and_strings(body)

    for m in _SELF_METHOD_ASSIGN_RE.finditer(clean_body):
        line = start_line + clean_body.count('\n', 0, m.start())
        _push_issue(errors, line, 'method-defined-in-render',
                    '不要在 renderJsx 内写 self.xxx = function()。'
                    '宜搭运行时方法必须提升为 export function xxx()，'
                    'renderJsx 只负责渲染', dmap)

    for m in _DID_MOUNT_EMULATION_RE.finditer(clean_body):
        line = start_line + clean_body.count('\n', 0, m.start())
        _push_issue(errors, line, 'lifecycle-emulated-in-render',
                    '不要在 renderJsx 内用 didMountCalled 模拟生命周期。'
                    '初始化加载请写 export function didMount()', dmap)

    for m in _RENDER_YIDA_API_RE.finditer(clean_body):
        line = start_line + clean_body.count('\n', 0, m.start())
        _push_issue(errors, line, 'api-call-in-render',
                    'renderJsx 中不要直接调用 Yida API。'
                    '异步加载放到 didMount 或事件触发的 export function 中', dmap)


def lint_yida_source(source: str, filename: str | None = None) -> dict:
    """对 JSX 源码做静态检查，返回 {'errors': [...], 'warnings': [...]}。

    每条记录形如：{'line': int, 'rule': str, 'message': str}。
    rule 命名采用稳定字符串，便于错误码映射与 disable 注释引用。
    """
    errors: list[dict] = []
    warnings: list[dict] = []
    lines = source.split('\n')
    dmap = _build_disable_map(lines)

    has_render_jsx = bool(_RENDERJSX_RE.search(source))
    is_modern_authoring = bool(_EXPORT_DEFAULT_PAGE_RE.search(source))
    if not has_render_jsx and not is_modern_authoring:
        _push_issue(errors, 1, 'missing-render-jsx',
                    '页面入口必须为 export function renderJsx() {...}，'
                    '或使用可编译的 export default function Page() authoring 模式', dmap)
    if _UNSUPPORTED_HOOKS_RE.search(source):
        _push_issue(errors, 1, 'unsupported-hooks',
                    '当前 Python compiler 仅支持 useState 和 useEffect(..., [])，'
                    '不支持 useMemo/useCallback/useRef/useReducer/useContext', dmap)
    elif _REACT_HOOKS_RE.search(source) and not is_modern_authoring:
        _push_issue(errors, 1, 'react-hooks',
                    'Hooks 只能出现在 export default function Page() authoring 模式中，'
                    '发布前由 Python compiler 降级为 _customState + didMount', dmap)
    if is_modern_authoring and _has_unsupported_use_state_pattern(source):
        _push_issue(errors, 1, 'unsupported-use-state-pattern',
                    '当前 Python compiler 仅支持 var/let/const [state, setState] = useState(init) '
                    '这种解构声明写法', dmap)
    if _EXPORT_DEFAULT_RE.search(source) and not is_modern_authoring:
        _push_issue(errors, 1, 'export-default',
                    '仅允许 export default function Page() authoring 模式；'
                    '否则请使用具名 export function renderJsx()', dmap)
    if filename and filename.lower().endswith('.js') and has_render_jsx \
            and _HAS_JSX_TAG_RE.search(source):
        _push_issue(warnings, 1, 'jsx-extension',
                    '文件含 JSX 元素，建议把扩展名从 .js 改为 .jsx', dmap)

    code_without_strings = _strip_comments_and_strings(source)
    if _CUSTOM_STATE_REF_RE.search(code_without_strings) \
            and not _CUSTOM_STATE_DECL_RE.search(code_without_strings):
        _push_issue(errors, 1, 'missing-custom-state',
                    '代码引用了 _customState 但未声明 var _customState = {...}; '
                    '发布后会报 ReferenceError', dmap)
    _scan_render_side_effects(source, errors, dmap)

    for idx, line in enumerate(lines):
        line_no = idx + 1
        trimmed = line.strip()
        if trimmed.startswith('//') or trimmed.startswith('*') or trimmed.startswith('/*'):
            continue

        m = _IMPORT_REQUIRE_RE.search(line)
        if m and not _is_in_comment_or_string(line, m.start()) \
                and not (is_modern_authoring and _SUPPORTED_MODERN_IMPORT_RE.search(line)):
            _push_issue(errors, line_no, 'import-require',
                        '宜搭自定义页面不允许 import / require，'
                        '请直接使用 React / this.utils.* 等运行时全局对象', dmap)

        m = _LEGACY_ECHARTS_MAP_RE.search(line)
        if m:
            _push_issue(errors, line_no, 'echarts-legacy-map-china',
                        '禁用 echarts/map/js/china，请改用 echarts/map/json/* 数据加载方式', dmap)

        m = _EVENT_FUNCTION_RE.search(line)
        if m and not _is_in_comment_or_string(line, m.start()) \
                and _function_callback_uses_this(lines, idx, m.start()):
            _push_issue(errors, line_no, 'event-function',
                        'JSX 事件用 function(){...} 且内部含 this，会丢失 this 指向，'
                        '请改用箭头函数', dmap)

        m = _DIRECT_METHOD_RE.search(line)
        if m and not _is_in_comment_or_string(line, m.start()):
            _push_issue(errors, line_no, 'event-direct-method',
                        _MSG_DIRECT_METHOD, dmap)

        m = _BIND_INLINE_RE.search(line)
        if m and not _is_in_comment_or_string(line, m.start()):
            _push_issue(errors, line_no, 'event-bind-this',
                        _MSG_BIND_THIS, dmap)

        m = _CONST_LET_RE.search(line)
        if m and not _is_in_comment_or_string(line, m.start()):
            _push_issue(warnings, line_no, 'const-let',
                        '宜搭页面 Babel 输出 ES5，建议用 var 而非 const/let 以避免编译问题', dmap)

        m = _COMPUTED_RE.search(line)
        if m and not _is_in_comment_or_string(line, m.start()):
            _push_issue(errors, line_no, 'computed-property',
                        '对象不允许使用 ES6 computed key（{[xxx]: ...}），'
                        '请改用 var obj = {}; obj[xxx] = ...', dmap)

        m = _PAD_METHOD_RE.search(line)
        if m and not _is_in_comment_or_string(line, m.start()):
            _push_issue(warnings, line_no, 'pad-method',
                        f'{m.group(1)} 是 ES2017，宜搭运行时不一定支持，'
                        f'请改用手写补零', dmap)

        m = _MAP_FILTER_FN_RE.search(line)
        if m and not _is_in_comment_or_string(line, m.start()) \
                and _function_callback_uses_this(lines, idx, m.start()):
            _push_issue(errors, line_no, 'array-callback-function',
                        f'{m.group(1)}(function(){{...}}) 内部含 this 会丢失上下文，'
                        f'请改用箭头函数', dmap)

        m = _FOREACH_FN_RE.search(line)
        if m and not _is_in_comment_or_string(line, m.start()) \
                and _function_callback_uses_this(lines, idx, m.start()):
            _push_issue(warnings, line_no, 'foreach-callback-function',
                        'forEach(function(){...}) 内部含 this 建议改用箭头函数', dmap)

        m = _CONTROLLED_INPUT_RE.search(line)
        if m and not _is_in_comment_or_string(line, m.start()):
            _push_issue(errors, line_no, 'controlled-input',
                        '<input value={...}> 在宜搭运行时表现异常，'
                        '请改用 defaultValue 或宜搭 Input 组件', dmap)

        m = _NATIVE_SELECT_RE.search(line)
        if m and not _is_in_comment_or_string(line, m.start()):
            _push_issue(warnings, line_no, 'native-select-ui',
                        '原生 <select> 样式与宜搭风格不一致，建议用 Select 组件', dmap)

        m = _IFRAME_NAV_RE.search(line)
        if m and not _is_in_comment_or_string(line, m.start()):
            _push_issue(warnings, line_no, 'iframe-self-navigation',
                        '<a> 跳转宜搭内部链接时建议加 target="_top" 或 "_blank"，'
                        '否则在 iframe 内会卡住', dmap)

        m = _TOP_LOCATION_RE.search(line)
        if m and not _is_in_comment_or_string(line, m.start()) \
                and 'window.top.location' not in line:
            _push_issue(warnings, line_no, 'iframe-self-navigation',
                        'window.location.href 跳转宜搭内部链接时建议改用 window.top.location', dmap)

        m = _PAGE_SIZE_RE.search(line)
        if m and int(m.group(1)) > 100 \
                and not _is_in_comment_or_string(line, m.start()):
            _push_issue(errors, line_no, 'page-size-limit',
                        f'pageSize={m.group(1)} 超过宜搭限制 100，请分页拉取', dmap)

    for r in _detect_large_then_callbacks(lines):
        _push_issue(warnings, r['line'], 'large-then-callback',
                    f'.then() 回调超过 {THEN_CALLBACK_LINE_LIMIT} 行（实际 {r["lineCount"]} 行），'
                    f'建议拆分为独立函数', dmap)
    _detect_yida_calls_without_catch(source, warnings, dmap)
    _detect_echarts_rich_label_formatter(source, warnings, dmap)
    _scan_lifecycle(source, errors, dmap)
    _scan_jsx_elements(source, errors, warnings, dmap)

    errors.sort(key=lambda x: (x['line'], x['rule']))
    warnings.sort(key=lambda x: (x['line'], x['rule']))
    return {'errors': errors, 'warnings': warnings}


def lint_check(source: str, filename: str | None = None) -> dict:
    """与 field_check 同款返回结构。

    Returns: {'ok': bool, 'errors': [...], 'warnings': [...], 'info': {...}}
    """
    result = lint_yida_source(source, filename)
    return {
        'ok': len(result['errors']) == 0,
        'errors': result['errors'],
        'warnings': result['warnings'],
        'info': {
            'errorCount': len(result['errors']),
            'warningCount': len(result['warnings']),
        },
    }


# ===========================================================================
# region: unified CLI（transform / field-check / lint）
# ===========================================================================

def _print_lint_human(result: dict) -> int:
    errs = result['errors']
    warns = result['warnings']
    if not errs and not warns:
        print('[OK] lint 通过（30 条规则）')
        return 0
    if errs:
        print(f'[FAIL] {len(errs)} 个错误：', file=sys.stderr)
        for e in errs:
            print(f"  L{e['line']:>4} [{e['rule']}] {e['message']}", file=sys.stderr)
    if warns:
        print(f'[WARN] {len(warns)} 个警告：')
        for w in warns:
            print(f"  L{w['line']:>4} [{w['rule']}] {w['message']}")
    return 1 if errs else 0


def _cli_transform(args: argparse.Namespace) -> int:
    src = Path(args.source).expanduser().read_text(encoding='utf-8')
    out = transform_jsx(src)
    if args.output:
        Path(args.output).expanduser().write_text(out, encoding='utf-8')
    else:
        sys.stdout.write(out)
    return 0


def _cli_field_check(args: argparse.Namespace) -> int:
    if args.code_file:
        code = Path(args.code_file).expanduser().read_text(encoding='utf-8')
    elif args.code:
        code = args.code
    else:
        print('错误: 必须提供 --code-file 或 --code', file=sys.stderr)
        return 1
    result = field_check(code, args.app)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result['ok'] else 1


def _cli_lint(args: argparse.Namespace) -> int:
    if args.stdin:
        src = sys.stdin.read()
        fname = args.filename
    elif args.source:
        path = Path(args.source).expanduser().resolve()
        if not path.exists():
            print(f'错误: 文件不存在 {path}', file=sys.stderr)
            return 1
        src = path.read_text(encoding='utf-8')
        fname = str(path)
    else:
        print('错误: 必须提供 source 路径或 --stdin', file=sys.stderr)
        return 1
    result = lint_yida_source(src, fname)
    if args.json:
        print(json.dumps({'ok': len(result['errors']) == 0, **result},
                         ensure_ascii=False, indent=2))
        return 0 if not result['errors'] else 1
    return _print_lint_human(result)


def main() -> int:
    ap = argparse.ArgumentParser(
        description='宜搭自定义页面 JSX 流水线（transform + field-check + lint）')
    sub = ap.add_subparsers(dest='cmd', required=True)

    sp_t = sub.add_parser('transform', help='JSX → React.createElement')
    sp_t.add_argument('source', help='JSX 源文件路径')
    sp_t.add_argument('--output', help='输出文件（默认 stdout）')

    sp_fc = sub.add_parser('field-check', help='发布前字段 ID 对账')
    sp_fc.add_argument('--app', required=True, help='应用 appType')
    sp_fc.add_argument('--code-file', help='JSX 源文件')
    sp_fc.add_argument('--code', help='JSX 内联字符串')

    sp_l = sub.add_parser('lint', help='宜搭专属 30 条规则静态检查')
    sp_l.add_argument('source', nargs='?', help='JSX 源文件路径（与 --stdin 二选一）')
    sp_l.add_argument('--stdin', action='store_true', help='从 stdin 读取源码')
    sp_l.add_argument('--filename', help='用于 jsx-extension 规则的文件名（仅 --stdin 模式有用）')
    sp_l.add_argument('--json', action='store_true', help='输出 JSON 而非文本')

    args = ap.parse_args()
    if args.cmd == 'transform':
        return _cli_transform(args)
    if args.cmd == 'field-check':
        return _cli_field_check(args)
    if args.cmd == 'lint':
        return _cli_lint(args)
    ap.error(f'未知命令: {args.cmd}')
    return 1


if __name__ == '__main__':
    sys.exit(main())
