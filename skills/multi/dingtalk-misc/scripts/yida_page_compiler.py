#!/usr/bin/env python3
"""yida_page_compiler.py — 纯 Python 宜搭自定义页面编译管线。

两块能力按 region 分段维护：
  1) [region: page_compat]   现代 JSX (export default + useState/useEffect)
                             → 宜搭 runtime 兼容格式 (export function renderJsx
                             + _customState + didMount/didUnmount)
  2) [region: compiler]      page-compat → JSX→createElement → bind-this
                             → ESM→CJS → minify 的端到端编译。

公开 API（被 yida_custom_page_update.py 直接 import）：
  - compile_jsx(jsx_source, modern=None, minify=True) -> dict
  - compile_jsx_to_schema(jsx_source, form_uuid=None, ...) -> dict
  - build_page_source(source_code) -> dict
  - ensure_runtime_contract(source_code) -> dict
  - minify_js(source) -> str

外部依赖：无（纯 Python 标准库）。JSX 转换从 yida_jsx_pipeline.transform_jsx；
schema 外壳从 yida_page_schema.build_schema_content。
"""

import json
import re
import sys
from pathlib import Path

_SCRIPT_DIR = Path(__file__).resolve().parent
if str(_SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(_SCRIPT_DIR))

# JSX 转换与 schema 外壳从其它聚合文件 import（避免重复实现）
from yida_jsx_pipeline import transform_jsx
from yida_page_schema import build_schema_content


# ===========================================================================
# region: page_compat —— modern JSX → 宜搭 runtime 格式
# ===========================================================================
#
# 支持的转换：
#   - export default function Page() → export function renderJsx()
#   - var [x, setX] = useState(init)  → this.getCustomState('x') / this.setCustomState({x: val})
#   - setX(function(prev) { ... })     → setCustomState calls updater with previous value
#   - useEffect(fn, [])               → didMount 生命周期
#   - useEffect cleanup return        → didUnmount 生命周期
#   - 移除 React/react-dom import
#   - const/let → var
#   - 补齐 runtime contract exports
# ---------------------------------------------------------------------------

_REMOVABLE_IMPORTS = frozenset(['react', 'react-dom', 'lodash', 'moment'])

_RUNTIME_EXPORTS = {
    'getCustomState': (
        'export function getCustomState(key) {\n'
        '  if (typeof _customState === "undefined") {\n'
        '    return key ? undefined : {};\n'
        '  }\n'
        '  if (key) {\n'
        '    return _customState[key];\n'
        '  }\n'
        '  return Object.assign({}, _customState);\n'
        '}'
    ),
    'setCustomState': (
        'export function setCustomState(newState) {\n'
        '  if (typeof _customState === "undefined") {\n'
        '    return;\n'
        '  }\n'
        '  Object.keys(newState || {}).forEach(function(key) {\n'
        '    var value = newState[key];\n'
        '    if (typeof value === "function") {\n'
        '      value = value(_customState[key]);\n'
        '    }\n'
        '    _customState[key] = value;\n'
        '  });\n'
        '  this.forceUpdate();\n'
        '}'
    ),
    'forceUpdate': (
        'export function forceUpdate() {\n'
        '  this.setState({ timestamp: new Date().getTime() });\n'
        '}'
    ),
    'didMount': 'export function didMount() {}',
    'didUnmount': 'export function didUnmount() {}',
}

_TIMESTAMP_NODE = (
    "React.createElement('div', {style: {display: 'none'}}, "
    "this.state && this.state.timestamp)"
)

_CUSTOM_STATE_DECL_RE = re.compile(r'\bvar\s+_customState\b')


def _build_render_jsx_with_marker(render_body):
    """把 render_body 包进 IIFE，外层加 timestamp marker。

    产出：
        export function renderJsx() {
          var __yidaTs = this.state && this.state.timestamp;
          var __yidaContent = (function() {
            ...原 render_body...
          }).call(this);
          return React.createElement('div', null,
            React.createElement('div', {style: {display: 'none'}}, __yidaTs),
            __yidaContent
          );
        }

    作用：__yidaTs 本体在 IIFE 之外被读取使 setState({timestamp}) 能被 React diff、
    隐藏 div 把 __yidaTs 放到 JSX 树里使其参与重渲染；原 content 不动。
    多一层匿名 <div> 包裹，不影响布局，仅 body > div > xxx 选择器会失效。
    """
    if not render_body:
        render_body = '  return null;'
    indented = '\n'.join(
        ('    ' + line) if line else line
        for line in render_body.split('\n')
    )
    return (
        'export function renderJsx() {\n'
        '  var __yidaTs = this.state && this.state.timestamp;\n'
        '  var __yidaContent = (function() {\n'
        + indented + '\n'
        '  }).call(this);\n'
        "  return React.createElement('div', null,\n"
        "    React.createElement('div', {style: {display: 'none'}}, __yidaTs),\n"
        '    __yidaContent\n'
        '  );\n'
        '}'
    )


def build_page_source(source_code):
    """将现代 JSX 页面源码转换为宜搭 runtime 兼容格式。

    Args:
        source_code: 现代格式的 JSX 源码（export default function Page）

    Returns:
        dict: {
            'code': str,       转换后的代码
            'lint': {'errors': [], 'warnings': []},
            'fixes': [str],    应用的修复描述
            'errors': [{'code': str, 'message': str}],
        }
    """
    if not _should_transform(source_code):
        result = ensure_runtime_contract(source_code)
        return {
            'code': result['code'],
            'lint': {'errors': [], 'warnings': []},
            'fixes': result['fixes'],
            'errors': [],
        }

    fixes = []
    errors = []

    # Step 0: 剥离 JSX 表达式注释 {/* ... */}
    source_code = _strip_jsx_comments(source_code, fixes)

    lines = source_code.split('\n')

    # Step 1: 移除 React 等 import
    lines = _remove_imports(lines, fixes, errors)

    # Step 2: const/let → var
    lines = _fix_variable_declarations(lines, fixes)

    # Step 3: 提取 Page 函数体
    page_info = _extract_page_function(lines, errors)
    if page_info is None:
        code = '\n'.join(lines)
        result = ensure_runtime_contract(code)
        return {
            'code': result['code'],
            'lint': {'errors': errors, 'warnings': []},
            'fixes': fixes + result['fixes'],
            'errors': errors,
        }

    # Step 4: 从函数体中提取 useState 和 useEffect
    body_lines = page_info['body_lines']
    state_info = _extract_use_state(body_lines, fixes, errors)
    effect_info = _extract_use_effect(body_lines, fixes, errors)

    # Step 5: 构建 renderJsx 函数
    remaining_body = _remove_hooks_from_body(body_lines, state_info, effect_info)

    # Step 5.5: 分离函数体为三类
    separated = _separate_body_parts(remaining_body, state_info)

    # Step 6: 构建输出
    output_parts = []
    output_parts.append('\n'.join(page_info['before_lines']))

    # _customState 初始化
    state_inits = []
    for st in state_info['states']:
        state_inits.append(f"  {st['name']}: {st['init']},")
    if state_inits:
        output_parts.append('var _customState = {\n' + '\n'.join(state_inits) + '\n};')
    else:
        output_parts.append('var _customState = {};')

    # 模块顶层变量
    if separated['top_level_vars']:
        top_vars = _replace_state_references(separated['top_level_vars'], state_info)
        output_parts.append(top_vars)

    # 纯工具函数
    for fn_code in separated['utility_functions']:
        output_parts.append(fn_code.strip())

    # 导出函数声明（变为组件方法）
    function_names = separated['function_names']
    for fn_code in separated['functions']:
        fn_transformed = _replace_state_references(fn_code, state_info)
        fn_transformed = _add_bind_this_to_then_catch(fn_transformed)
        fn_transformed = _replace_fn_calls_with_this(fn_transformed, function_names)
        output_parts.append('export ' + fn_transformed.strip())

    # renderJsx
    render_body = _replace_state_references(separated['render_body'], state_info, receiver='self')
    render_body = _replace_fn_calls_with_this(render_body, function_names)
    if ('self.getCustomState' in render_body or 'self.setCustomState' in render_body) \
            and not re.search(r'\bvar\s+self\s*=\s*this\b', render_body):
        render_body = 'var self = this;\n' + render_body
    render_fn = _build_render_jsx_with_marker(render_body)
    output_parts.append(render_fn)
    fixes.append('Converted export default function Page → export function renderJsx (with re-render marker)')

    # didMount/didUnmount
    mount_code = _replace_state_references(effect_info['mount_code'], state_info) if effect_info['mount_code'] else ''
    unmount_code = _replace_state_references(effect_info['unmount_code'], state_info) if effect_info['unmount_code'] else ''

    if mount_code:
        mount_code = _replace_fn_calls_with_this(mount_code, function_names)
        mount_code = _add_bind_this_to_then_catch(mount_code)
    if unmount_code:
        unmount_code = _replace_fn_calls_with_this(unmount_code, function_names)
        unmount_code = _add_bind_this_to_then_catch(unmount_code)

    if mount_code:
        mount_fn = 'export function didMount() {\n' + mount_code + '\n}'
        output_parts.append(mount_fn)
        fixes.append('Extracted useEffect(fn, []) → didMount')
    else:
        output_parts.append('export function didMount() {}')

    if unmount_code:
        unmount_fn = 'export function didUnmount() {\n' + unmount_code + '\n}'
        output_parts.append(unmount_fn)
        fixes.append('Extracted useEffect cleanup → didUnmount')
    else:
        output_parts.append('export function didUnmount() {}')

    if page_info['after_lines']:
        output_parts.append('\n'.join(page_info['after_lines']))

    code = '\n\n'.join(p for p in output_parts if p.strip())
    result = ensure_runtime_contract(code)

    return {
        'code': result['code'],
        'lint': {'errors': errors, 'warnings': []},
        'fixes': fixes + result['fixes'],
        'errors': errors,
    }


def ensure_runtime_contract(source_code):
    """确保代码包含宜搭 runtime 必需的 export 函数。

    Args:
        source_code: 已转换的源码

    Returns:
        dict: {'code': str, 'fixes': [str]}
    """
    fixes = []
    append_parts = []

    exported_names = set(re.findall(r'export\s+function\s+(\w+)', source_code))

    if '_customState' in source_code and not _CUSTOM_STATE_DECL_RE.search(source_code):
        append_parts.insert(0, 'var _customState = {};')
        fixes.append('Inserted missing _customState store')

    if 'getCustomState' not in exported_names or 'setCustomState' not in exported_names:
        if '_customState' not in source_code:
            append_parts.insert(0, 'var _customState = {};')
            fixes.append('Inserted default _customState store')

    for name, body in _RUNTIME_EXPORTS.items():
        if name not in exported_names:
            append_parts.append(body)
            fixes.append(f'Inserted missing export function {name}')

    if not append_parts:
        return {'code': source_code, 'fixes': fixes}

    return {
        'code': source_code.rstrip() + '\n\n' + '\n\n'.join(append_parts) + '\n',
        'fixes': fixes,
    }


def _should_transform(source_code):
    """判断是否需要做 modern → yida 转换"""
    if re.search(r'export\s+function\s+renderJsx\s*\(', source_code):
        return False
    return bool(re.search(r'export\s+default\b', source_code))


def _strip_jsx_comments(source_code, fixes):
    """剥离 JSX 表达式容器注释 {/* ... */}（避免 ,, 双逗号）。"""
    if not source_code or '/*' not in source_code:
        return source_code
    original = source_code
    source_code = re.sub(
        r'(?m)^[ \t]*\{\s*/\*[\s\S]*?\*/\s*\}[ \t]*\r?\n',
        '',
        source_code,
    )
    source_code = re.sub(
        r'(>)\s*\{\s*/\*[\s\S]*?\*/\s*\}\s*(<)',
        r'\1\2',
        source_code,
    )
    if source_code != original:
        fixes.append('Stripped JSX expression comments (avoid ",," double-comma)')
    return source_code


def _remove_imports(lines, fixes, errors):
    """移除已知的 removable imports，对未知的报错"""
    result = []
    for line in lines:
        match = re.match(r"^\s*import\s+.*?from\s+['\"]([^'\"]+)['\"]", line)
        if not match:
            match = re.match(r"^\s*import\s+['\"]([^'\"]+)['\"]", line)
        if match:
            module_name = match.group(1).lower()
            if module_name in _REMOVABLE_IMPORTS:
                fixes.append(f'Removed {module_name} import')
                continue
            else:
                errors.append({
                    'code': 'UNSUPPORTED_IMPORT',
                    'message': f'Unsupported import "{match.group(1)}". '
                               'Use this.utils.loadScript for external libraries.',
                })
        result.append(line)
    return result


def _fix_variable_declarations(lines, fixes):
    """const/let → var"""
    result = []
    pattern = re.compile(r'^(\s*)(const|let)\b(.*)$')
    changed = False
    for line in lines:
        m = pattern.match(line)
        if m:
            result.append(f'{m.group(1)}var{m.group(3)}')
            changed = True
        else:
            result.append(line)
    if changed:
        fixes.append('Replaced const/let with var')
    return result


def _extract_page_function(lines, errors):
    """提取 export default function Page() { ... } 的位置和内容"""
    pattern = re.compile(r'^(\s*)export\s+default\s+function\s+\w*\s*\([^)]*\)\s*\{?\s*$')
    start_idx = None

    for i, line in enumerate(lines):
        if pattern.match(line):
            start_idx = i
            break
        if re.match(r'^\s*export\s+default\s+function\b', line):
            start_idx = i
            break

    if start_idx is None:
        errors.append({
            'code': 'NO_DEFAULT_EXPORT',
            'message': 'Cannot find export default function Page().',
        })
        return None

    brace_start = None
    text_from_start = '\n'.join(lines[start_idx:])
    for i, ch in enumerate(text_from_start):
        if ch == '{':
            brace_start = i
            break

    if brace_start is None:
        errors.append({'code': 'PARSE_ERROR', 'message': 'Cannot find opening brace of Page function.'})
        return None

    depth = 0
    all_text = '\n'.join(lines)
    char_pos = len('\n'.join(lines[:start_idx])) + (1 if start_idx > 0 else 0) + brace_start

    end_char = None
    pos = char_pos
    while pos < len(all_text):
        ch = all_text[pos]
        if ch in ('"', "'", '`'):
            pos = _skip_string_in_source(all_text, pos) + 1
            continue
        if ch == '{':
            depth += 1
        elif ch == '}':
            depth -= 1
            if depth == 0:
                end_char = pos
                break
        pos += 1

    if end_char is None:
        errors.append({'code': 'PARSE_ERROR', 'message': 'Cannot find closing brace of Page function.'})
        return None

    body_content = all_text[char_pos + 1:end_char]
    end_line = all_text[:end_char + 1].count('\n')

    before_lines = lines[:start_idx]
    after_lines = lines[end_line + 1:] if end_line + 1 < len(lines) else []
    body_lines = body_content.split('\n')

    return {
        'before_lines': before_lines,
        'body_lines': body_lines,
        'after_lines': after_lines,
    }


def _skip_string_in_source(text, pos):
    """跳过字符串字面量"""
    quote = text[pos]
    pos += 1
    while pos < len(text):
        ch = text[pos]
        if ch == '\\':
            pos += 2
            continue
        if ch == quote:
            return pos
        pos += 1
    return pos


def _extract_use_state(body_lines, fixes, errors):
    """提取所有 useState 声明（支持跨行初值）"""
    states = []
    body_text = '\n'.join(body_lines)
    decl_pattern = re.compile(
        r'(?m)^([ \t]*)var\s+\[(\w+)\s*,\s*(\w+)\]\s*=\s*useState\s*\('
    )
    for m in decl_pattern.finditer(body_text):
        name = m.group(2)
        setter = m.group(3)
        paren_start = m.end() - 1
        depth = 0
        pos = paren_start
        while pos < len(body_text):
            ch = body_text[pos]
            if ch in ('"', "'", '`'):
                pos = _skip_string_in_source(body_text, pos) + 1
                continue
            if ch == '(':
                depth += 1
            elif ch == ')':
                depth -= 1
                if depth == 0:
                    break
            pos += 1
        if depth != 0:
            errors.append({
                'code': 'UNSUPPORTED_USE_STATE',
                'message': f'Unbalanced parentheses in useState({name}) declaration.',
            })
            continue
        init_text = body_text[paren_start + 1:pos].strip()
        decl_end = pos + 1
        while decl_end < len(body_text) and body_text[decl_end] in ' \t':
            decl_end += 1
        if decl_end < len(body_text) and body_text[decl_end] == ';':
            decl_end += 1
        states.append({
            'name': name,
            'setter': setter,
            'init': init_text,
            'raw_text': body_text[m.start():decl_end],
        })

    for line in body_lines:
        hook_match = re.search(r'\buse([A-Z]\w*)\b', line)
        if hook_match:
            hook_name = 'use' + hook_match.group(1)
            if hook_name not in ('useState', 'useEffect'):
                errors.append({
                    'code': 'UNSUPPORTED_HOOK',
                    'message': f'{hook_name} is not supported.',
                })

    return {'states': states}


def _extract_use_effect(body_lines, fixes, errors):
    """提取 useEffect(fn, []) 调用"""
    mount_code = ''
    unmount_code = ''

    body_text = '\n'.join(body_lines)
    effect_pattern = re.compile(r'useEffect\s*\(\s*function\s*\(\)\s*\{')

    for match in effect_pattern.finditer(body_text):
        start = match.end()
        depth = 1
        pos = start
        while pos < len(body_text) and depth > 0:
            ch = body_text[pos]
            if ch in ('"', "'", '`'):
                pos = _skip_string_in_source(body_text, pos) + 1
                continue
            if ch == '{':
                depth += 1
            elif ch == '}':
                depth -= 1
            pos += 1

        fn_body = body_text[start:pos - 1]

        remaining_after_fn = body_text[pos:].lstrip()
        if not remaining_after_fn.startswith(','):
            errors.append({'code': 'UNSUPPORTED_EFFECT_DEPS', 'message': 'useEffect must have [], as second arg.'})
            continue
        deps_start = body_text.index(',', pos) + 1
        deps_text = body_text[deps_start:].lstrip()
        if not deps_text.startswith('[]') and not deps_text.startswith('[ ]'):
            errors.append({'code': 'UNSUPPORTED_EFFECT_DEPS', 'message': 'Only useEffect(fn, []) is supported.'})
            continue

        mount_lines, unmount_lines = _split_effect_body(fn_body)
        if mount_lines:
            mount_code += mount_lines + '\n'
        if unmount_lines:
            unmount_code += unmount_lines + '\n'

    return {
        'mount_code': mount_code.strip(),
        'unmount_code': unmount_code.strip(),
    }


def _split_effect_body(fn_body):
    """将 effect 函数体分为 mount 代码和 unmount（return 的 cleanup 函数）"""
    lines = fn_body.strip().split('\n')
    mount_lines = []
    unmount_lines = []

    return_idx = None
    for i in range(len(lines) - 1, -1, -1):
        if re.match(r'^\s*return\s+function\b', lines[i]):
            return_idx = i
            break
        if re.match(r'^\s*return\s*\(\s*function\b', lines[i]):
            return_idx = i
            break

    if return_idx is None:
        return '\n'.join(lines), ''

    mount_lines = lines[:return_idx]

    return_text = '\n'.join(lines[return_idx:])
    brace_match = return_text.find('{')
    if brace_match == -1:
        return '\n'.join(mount_lines), ''

    depth = 0
    end_pos = None
    for i in range(brace_match, len(return_text)):
        if return_text[i] == '{':
            depth += 1
        elif return_text[i] == '}':
            depth -= 1
            if depth == 0:
                end_pos = i
                break

    if end_pos is not None:
        cleanup_body = return_text[brace_match + 1:end_pos]
        unmount_lines = cleanup_body.strip()

    return '\n'.join(mount_lines), unmount_lines


def _remove_hooks_from_body(body_lines, state_info, effect_info):
    """从函数体中移除 useState 和 useEffect 声明行"""
    body_text = '\n'.join(body_lines)

    for st in state_info['states']:
        raw = st.get('raw_text') or st.get('line') or ''
        if raw:
            body_text = body_text.replace(raw, '')

    effect_pattern = re.compile(r'\s*useEffect\s*\(')
    while True:
        m = effect_pattern.search(body_text)
        if not m:
            break
        start = m.start()
        paren_start = body_text.index('(', m.start())
        depth = 0
        pos = paren_start
        while pos < len(body_text):
            ch = body_text[pos]
            if ch in ('"', "'", '`'):
                pos = _skip_string_in_source(body_text, pos) + 1
                continue
            if ch == '(':
                depth += 1
            elif ch == ')':
                depth -= 1
                if depth == 0:
                    break
            pos += 1
        end = pos + 1
        if end < len(body_text) and body_text[end] == ';':
            end += 1
        body_text = body_text[:start] + body_text[end:]

    body_text = re.sub(r'\n{3,}', '\n\n', body_text)
    return body_text.strip()


def _replace_state_references(body_text, state_info, receiver='this'):
    """将 state 变量引用替换为 this.getCustomState/setCustomState（跳过字符串）。"""
    for st in state_info['states']:
        name = st['name']
        setter = st['setter']
        body_text = _replace_setter_calls(body_text, setter, name, receiver=receiver)
        body_text = _replace_state_reads(body_text, name, receiver=receiver)
    return body_text


def _replace_state_reads(text, state_name, receiver='this'):
    """替换裸变量名引用为 this.getCustomState('name')，跳过字符串内容和对象属性 key。"""
    pattern = re.compile(r'(?<![.\w])' + re.escape(state_name) + r'(?!\w)')
    result = []
    pos = 0
    while pos < len(text):
        ch = text[pos]
        if ch in ('"', "'", '`'):
            end = _find_string_end(text, pos)
            result.append(text[pos:end + 1])
            pos = end + 1
            continue
        m = pattern.match(text, pos)
        if m:
            after_end = m.end()
            preceding = ''.join(result).rstrip()

            if preceding and preceding[-1] == '>':
                prev_prev = preceding[-2] if len(preceding) >= 2 else ''
                if prev_prev not in ('=', '!', '<', '>'):
                    result.append(text[pos:m.end()])
                    pos = m.end()
                    continue

            rest_after = text[after_end:].lstrip()
            if rest_after and rest_after[0] == ':' and not rest_after.startswith('::'):
                if preceding and preceding[-1] in ('{', ',', '\n'):
                    result.append(text[pos:m.end()])
                    pos = m.end()
                    continue
            result.append(f"{receiver}.getCustomState('{state_name}')")
            pos = m.end()
        else:
            result.append(ch)
            pos += 1
    return ''.join(result)


def _replace_setter_calls(text, setter_name, state_name, receiver='this'):
    """替换 setX(value) 为 this.setCustomState({'x': value})，跳过字符串内容。"""
    setter_pattern = re.compile(r'(?<![.\w])' + re.escape(setter_name) + r'\s*\(')
    result_parts = []
    search_start = 0

    while search_start < len(text):
        ch = text[search_start] if search_start < len(text) else ''
        if ch in ('"', "'", '`'):
            end = _find_string_end(text, search_start)
            result_parts.append(text[search_start:end + 1])
            search_start = end + 1
            continue

        m = setter_pattern.search(text, search_start)
        if not m:
            result_parts.append(text[search_start:])
            break

        result_parts.append(text[search_start:m.start()])

        paren_start = m.end() - 1
        depth = 0
        pos = paren_start
        while pos < len(text):
            c = text[pos]
            if c in ('"', "'", '`'):
                pos = _find_string_end(text, pos) + 1
                continue
            if c == '(':
                depth += 1
            elif c == ')':
                depth -= 1
                if depth == 0:
                    break
            pos += 1
        arg_content = text[paren_start + 1:pos]
        replacement = f"{receiver}.setCustomState({{ '{state_name}': {arg_content} }})"
        result_parts.append(replacement)
        search_start = pos + 1

    return ''.join(result_parts)


def _find_string_end(text, start):
    """找到从 start 位置开始的字符串字面量结束位置（含闭合引号）"""
    quote = text[start]
    pos = start + 1
    if quote == '`':
        while pos < len(text):
            ch = text[pos]
            if ch == '\\':
                pos += 2
                continue
            if ch == '`':
                return pos
            if ch == '$' and pos + 1 < len(text) and text[pos + 1] == '{':
                pos += 2
                depth = 1
                while pos < len(text) and depth > 0:
                    if text[pos] == '{':
                        depth += 1
                    elif text[pos] == '}':
                        depth -= 1
                    pos += 1
                continue
            pos += 1
        return len(text) - 1
    while pos < len(text):
        ch = text[pos]
        if ch == '\\':
            pos += 2
            continue
        if ch == quote:
            return pos
        pos += 1
    return len(text) - 1



def _consume_var_declaration(lines, start_index):
    """从 lines[start_index] 起读一条完整的 var 声明，允许初值跨多行。

    跟踪 {} [] () 括号深度（跳过字符串、模板字符串、单/多行注释），
    括号深度归零的那一行视为声明结束。返回 (end_index_inclusive, full_text)。
    """
    depth = 0
    in_string = False
    string_quote = ''
    n = len(lines)
    end = start_index
    i = start_index
    while i < n:
        line = lines[i]
        j = 0
        L = len(line)
        while j < L:
            ch = line[j]
            if in_string:
                if string_quote == '`':
                    if ch == '\\' and j + 1 < L:
                        j += 2
                        continue
                    if ch == '`':
                        in_string = False
                    j += 1
                    continue
                if ch == '\\' and j + 1 < L:
                    j += 2
                    continue
                if ch == string_quote:
                    in_string = False
                j += 1
                continue
            if ch == '/' and j + 1 < L and line[j + 1] == '/':
                break
            if ch == '/' and j + 1 < L and line[j + 1] == '*':
                end_block = line.find('*/', j + 2)
                if end_block == -1:
                    j = L
                    break
                j = end_block + 2
                continue
            if ch in ('"', "'", '`'):
                in_string = True
                string_quote = ch
                j += 1
                continue
            if ch in '{[(':
                depth += 1
            elif ch in '}])':
                depth -= 1
            j += 1
        if depth <= 0:
            end = i
            break
        i += 1
    else:
        end = n - 1
    return end, '\n'.join(lines[start_index:end + 1])


def _var_function_to_declaration(decl_text):
    """Convert `var name = function(args) { ... };` to `function name(args) { ... }`."""
    header = re.match(r'^\s*var\s+(\w+)\s*=\s*function\s*(\([^)]*\))\s*\{', decl_text, re.S)
    if not header:
        return None, None
    name = header.group(1)
    args = header.group(2)
    open_brace = decl_text.find('{', header.start())
    close_brace = decl_text.rfind('}')
    if open_brace < 0 or close_brace <= open_brace:
        return None, None
    body = decl_text[open_brace + 1:close_brace]
    return name, f"function {name}{args} {{{body}\n}}"


def _extract_declared_var_names(decl_text):
    """Return declared identifiers from a `var ...` declaration.

    The modern authoring splitter needs these names to keep chained render
    derivations together, e.g. `filtered` followed by `amount =
    filtered.reduce(...)`. This parser only handles normal identifiers because
    useState destructuring is removed before this phase.
    """
    m = re.match(r'^\s*var\s+([\s\S]*)$', decl_text.strip())
    if not m:
        return []
    tail = m.group(1)
    names = []
    pos = 0
    n = len(tail)
    while pos < n:
        while pos < n and tail[pos].isspace():
            pos += 1
        ident = re.match(r'[A-Za-z_$][\w$]*', tail[pos:])
        if ident:
            names.append(ident.group(0))
            pos += len(ident.group(0))
        depth = 0
        while pos < n:
            ch = tail[pos]
            if ch in ('"', "'", '`'):
                pos = _find_string_end(tail, pos) + 1
                continue
            if ch in '([{':
                depth += 1
            elif ch in ')]}':
                depth -= 1
            elif ch == ',' and depth == 0:
                pos += 1
                break
            pos += 1
        else:
            break
    return names


def _references_any_identifier(text, names):
    """Whether text references any identifier in names outside strings."""
    if not names:
        return False
    clean_parts = []
    pos = 0
    while pos < len(text):
        ch = text[pos]
        if ch in ('"', "'", '`'):
            end = _find_string_end(text, pos)
            clean_parts.append(' ')
            pos = end + 1
        else:
            clean_parts.append(ch)
            pos += 1
    clean = ''.join(clean_parts)
    return any(re.search(r'\b' + re.escape(name) + r'\b', clean) for name in names)


def _separate_body_parts(body_text, state_info):
    """将函数体分离为：需导出的方法、模块级工具函数、顶层常量、render 部分。

    分类规则：
    - 函数内含 setState/getState 调用 → 导出为组件方法（用 this.xxx() 调用）
    - 纯工具函数（不依赖 state）→ 模块级普通函数（直接调用）
    - var 声明依赖 state → 放入 renderJsx
    - var 声明不依赖 state → 模块顶层常量
    - return 语句及之后 → render_body
    """
    lines = body_text.split('\n')
    exported_functions = []
    exported_fn_names = []
    utility_functions = []
    utility_fn_names = []
    top_level_vars = []
    render_lines = []
    render_var_names = set()

    state_setters = set(st['setter'] for st in state_info['states'])
    state_names = set(st['name'] for st in state_info['states'])

    i = 0
    found_return = False
    while i < len(lines):
        line = lines[i]
        stripped = line.strip()

        if found_return:
            render_lines.append(line)
            i += 1
            continue

        if re.match(r'^\s*return\s*[\(]', stripped) or stripped == 'return (' or stripped == 'return(':
            found_return = True
            render_lines.append(line)
            i += 1
            continue

        fn_match = re.match(r'^(\s*)function\s+(\w+)\s*\(', line)
        if fn_match:
            fn_name = fn_match.group(2)
            fn_start = i
            brace_depth = 0
            fn_end = i
            for j in range(i, len(lines)):
                for ch in lines[j]:
                    if ch == '{':
                        brace_depth += 1
                    elif ch == '}':
                        brace_depth -= 1
                if brace_depth == 0 and j > i:
                    fn_end = j
                    break
                if brace_depth == 0 and '{' in lines[j]:
                    fn_end = j
                    break
            fn_code = '\n'.join(lines[fn_start:fn_end + 1])

            uses_state = (
                any(s + '(' in fn_code for s in state_setters)
                or any(re.search(r'\b' + re.escape(sn) + r'\b', fn_code) for sn in state_names)
                or bool(re.search(r'\bthis\.', fn_code))
            )
            if uses_state:
                exported_functions.append(fn_code)
                exported_fn_names.append(fn_name)
            else:
                utility_functions.append(fn_code)
                utility_fn_names.append(fn_name)
            i = fn_end + 1
            continue

        if re.match(r'^\s*var\s+\w+\s*=\s*function\s*\(', line):
            decl_end, decl_text = _consume_var_declaration(lines, i)
            fn_name, fn_code = _var_function_to_declaration(decl_text)
            if fn_name and fn_code:
                exported_functions.append(fn_code)
                exported_fn_names.append(fn_name)
            else:
                render_lines.extend(lines[i:decl_end + 1])
            i = decl_end + 1
            continue

        if stripped.startswith('var ') and not stripped.startswith('var ['):
            decl_end, decl_text = _consume_var_declaration(lines, i)
            decl_names = _extract_declared_var_names(decl_text)
            uses_state_var = _references_any_identifier(decl_text, state_names)
            uses_render_var = _references_any_identifier(decl_text, render_var_names)
            if uses_state_var or uses_render_var:
                render_lines.extend(lines[i:decl_end + 1])
                render_var_names.update(decl_names)
            else:
                top_level_vars.extend(lines[i:decl_end + 1])
            i = decl_end + 1
            continue

        if stripped:
            render_lines.append(line)
        i += 1

    return {
        'functions': exported_functions,
        'function_names': exported_fn_names,
        'utility_functions': utility_functions,
        'utility_fn_names': utility_fn_names,
        'top_level_vars': '\n'.join(top_level_vars),
        'render_body': '\n'.join(render_lines).strip(),
    }


def _replace_fn_calls_with_this(code, function_names):
    """将裸函数调用 xxx() 替换为 this.xxx()（仅对导出为组件方法的函数）。"""
    for fn_name in function_names:
        pattern = re.compile(r'(?<!function )(?<![.\w])' + re.escape(fn_name) + r'(?=\s*\()')
        code = pattern.sub('this.' + fn_name, code)
    return code


def _add_bind_this_to_then_catch(code):
    """在 .then(function(...) { ... }) 和 .catch(function(...) { ... }) 回调中，
    如果内部使用了 this.，则在闭合 } 后添加 .bind(this)。
    """
    result = []
    pos = 0
    text = code

    while pos < len(text):
        match = None
        for method in ('then', 'catch'):
            pattern = '.' + method + '(function'
            if text[pos:pos + len(pattern)] == pattern:
                match = method
                break

        if match is None:
            result.append(text[pos])
            pos += 1
            continue

        method_text = '.' + match + '('
        result.append(method_text)
        pos += len(method_text)

        brace_pos = text.find('{', pos)
        if brace_pos == -1:
            result.append(text[pos:])
            break

        result.append(text[pos:brace_pos + 1])
        pos = brace_pos + 1

        depth = 1
        body_start = pos
        while pos < len(text) and depth > 0:
            ch = text[pos]
            if ch in ('"', "'", '`'):
                pos = _find_string_end(text, pos) + 1
                continue
            if ch == '{':
                depth += 1
            elif ch == '}':
                depth -= 1
            pos += 1

        fn_body = text[body_start:pos - 1]
        result.append(fn_body)

        if 'this.' in fn_body:
            result.append('}.bind(this)')
        else:
            result.append('}')

    return ''.join(result)


# ===========================================================================
# region: compiler —— page-compat → JSX→createElement → bind-this → ESM→CJS → minify
# ===========================================================================

def compile_jsx(jsx_source, modern=None, minify=True):
    """编译 JSX 源码为宜搭 runtime 兼容的 JS。

    Args:
        jsx_source: JSX/JS 源码字符串
        modern: True 强制走 page-compat 转换；None 自动判断
        minify: 是否压缩（默认 True）

    Returns:
        dict: {
            'source_code': str,        原始输入
            'intermediate_code': str,   page-compat 改写后的中间产物
            'compiled_code': str,       JSX 转换 + minify 后的最终 JS
            'lint': {'errors': [], 'warnings': []},
            'ok': bool,
            'errors': [{'message': str, ...}],
        }
    """
    errors = []

    # Step 1: page-compat 转换
    compat_result = build_page_source(jsx_source)
    intermediate_code = compat_result['code']
    lint = compat_result.get('lint', {'errors': [], 'warnings': []})

    if compat_result.get('errors'):
        for err in compat_result['errors']:
            errors.append(err)

    if lint.get('errors'):
        return {
            'source_code': jsx_source,
            'intermediate_code': intermediate_code,
            'compiled_code': '',
            'lint': lint,
            'ok': False,
            'errors': errors + lint['errors'],
        }

    # Step 2: JSX → React.createElement
    try:
        transformed_code = transform_jsx(intermediate_code)
    except Exception as exc:
        errors.append({'code': 'JSX_TRANSFORM_ERROR', 'message': str(exc)})
        return {
            'source_code': jsx_source,
            'intermediate_code': intermediate_code,
            'compiled_code': '',
            'lint': lint,
            'ok': False,
            'errors': errors,
        }

    # Step 2.5: 给含 this. 的匿名函数加 .bind(this)
    transformed_code = _bind_this_to_anonymous_fns(transformed_code)

    # Step 2.6: ESM exports → CommonJS（宜搭 runtime 只认 CommonJS）
    transformed_code = _convert_esm_to_commonjs(transformed_code)

    # Step 3: minify
    if minify:
        compiled_code = minify_js(transformed_code)
    else:
        compiled_code = transformed_code

    ok = len(errors) == 0
    return {
        'source_code': jsx_source,
        'intermediate_code': intermediate_code,
        'compiled_code': compiled_code,
        'lint': lint,
        'ok': ok,
        'errors': errors,
    }


def compile_jsx_to_schema(jsx_source, form_uuid=None, existing_data_source=None,
                          modern=None, minify=True):
    """编译 JSX 源码，可选生成完整自定义页面 schema。"""
    result = compile_jsx(jsx_source, modern=modern, minify=minify)

    if form_uuid and result.get('ok'):
        result['schema'] = build_schema_content(
            result['source_code'],
            result['compiled_code'],
            form_uuid,
            existing_data_source=existing_data_source,
        )
    else:
        result['schema'] = None

    return result


def minify_js(source):
    """简单 JS minifier：移除注释、压缩连续空白、保留字符串内容。"""
    result = []
    pos = 0
    length = len(source)

    while pos < length:
        ch = source[pos]

        if ch in ('"', "'"):
            end = _consume_string(source, pos)
            result.append(source[pos:end + 1])
            pos = end + 1
            continue

        if ch == '`':
            end = _consume_template(source, pos)
            result.append(source[pos:end + 1])
            pos = end + 1
            continue

        if ch == '/' and pos + 1 < length and source[pos + 1] == '/':
            while pos < length and source[pos] != '\n':
                pos += 1
            continue

        if ch == '/' and pos + 1 < length and source[pos + 1] == '*':
            pos += 2
            while pos < length - 1:
                if source[pos] == '*' and source[pos + 1] == '/':
                    pos += 2
                    break
                pos += 1
            else:
                pos = length
            continue

        if ch in (' ', '\t', '\r'):
            while pos < length and source[pos] in (' ', '\t', '\r'):
                pos += 1
            if result and result[-1] and result[-1][-1:].isalnum() or (result and result[-1][-1:] == '_'):
                if pos < length and (source[pos].isalnum() or source[pos] == '_'):
                    result.append(' ')
            continue

        if ch == '\n':
            while pos < length and source[pos] in ('\n', '\r', ' ', '\t'):
                pos += 1
            if result and result[-1] and result[-1][-1:] not in ('{', '}', ';', ',', '(', '[', '\n', ''):
                result.append('\n')
            continue

        result.append(ch)
        pos += 1

    return ''.join(result)


def _consume_string(source, start):
    """找到字符串字面量的结束位置（含闭合引号）"""
    quote = source[start]
    pos = start + 1
    while pos < len(source):
        ch = source[pos]
        if ch == '\\':
            pos += 2
            continue
        if ch == quote:
            return pos
        pos += 1
    return len(source) - 1


def _consume_template(source, start):
    """找到模板字面量的结束位置（含闭合反引号）"""
    pos = start + 1
    while pos < len(source):
        ch = source[pos]
        if ch == '\\':
            pos += 2
            continue
        if ch == '`':
            return pos
        if ch == '$' and pos + 1 < len(source) and source[pos + 1] == '{':
            pos += 2
            depth = 1
            while pos < len(source) and depth > 0:
                if source[pos] == '{':
                    depth += 1
                elif source[pos] == '}':
                    depth -= 1
                elif source[pos] == '\\':
                    pos += 1
                pos += 1
            continue
        pos += 1
    return len(source) - 1


def _bind_this_to_anonymous_fns(code):
    """给含有 this. 引用的匿名函数表达式自动添加 .bind(this)。"""
    result = []
    pos = 0
    length = len(code)

    while pos < length:
        fn_idx = code.find('function', pos)
        if fn_idx == -1:
            result.append(code[pos:])
            break

        if fn_idx > 0 and (code[fn_idx - 1].isalnum() or code[fn_idx - 1] == '_'):
            result.append(code[pos:fn_idx + 8])
            pos = fn_idx + 8
            continue
        after_fn = fn_idx + 8
        if after_fn < length and (code[after_fn].isalnum() or code[after_fn] == '_'):
            pass

        result.append(code[pos:fn_idx])

        line_start = code.rfind('\n', 0, fn_idx)
        line_start = line_start + 1 if line_start != -1 else 0
        prefix = code[line_start:fn_idx].strip()
        is_declaration = (prefix == '' or prefix == 'export')

        scan = fn_idx + 8
        while scan < length and code[scan] in (' ', '\t'):
            scan += 1

        if scan < length and (code[scan].isalpha() or code[scan] == '_'):
            while scan < length and (code[scan].isalnum() or code[scan] == '_'):
                scan += 1
            while scan < length and code[scan] in (' ', '\t'):
                scan += 1

        if scan >= length or code[scan] != '(':
            result.append('function')
            pos = fn_idx + 8
            continue

        depth = 1
        scan += 1
        while scan < length and depth > 0:
            ch = code[scan]
            if ch == '(':
                depth += 1
            elif ch == ')':
                depth -= 1
            elif ch in ('"', "'", '`'):
                scan = _skip_str_for_bind(code, scan)
            scan += 1
        paren_end = scan

        while scan < length and code[scan] in (' ', '\t', '\n', '\r'):
            scan += 1

        if scan >= length or code[scan] != '{':
            result.append(code[fn_idx:paren_end])
            pos = paren_end
            continue

        brace_start = scan

        if is_declaration:
            result.append(code[fn_idx:brace_start + 1])
            pos = brace_start + 1
            continue

        depth = 1
        scan += 1
        while scan < length and depth > 0:
            ch = code[scan]
            if ch in ('"', "'", '`'):
                scan = _skip_str_for_bind(code, scan)
                scan += 1
                continue
            if ch == '{':
                depth += 1
            elif ch == '}':
                depth -= 1
            scan += 1
        brace_end = scan

        fn_body = code[brace_start + 1:brace_end - 1]
        result.append(code[fn_idx:brace_end])

        if 'this.' in fn_body:
            rest = code[brace_end:brace_end + 30]
            if not rest.lstrip(' \t').startswith('.bind(this)'):
                result.append('.bind(this)')

        pos = brace_end

    return ''.join(result)


def _skip_str_for_bind(code, pos):
    """跳过字符串/模板字面量，返回闭合引号的位置"""
    quote = code[pos]
    pos += 1
    while pos < len(code):
        ch = code[pos]
        if ch == '\\':
            pos += 2
            continue
        if quote == '`' and ch == '$' and pos + 1 < len(code) and code[pos + 1] == '{':
            pos += 2
            depth = 1
            while pos < len(code) and depth > 0:
                if code[pos] == '{':
                    depth += 1
                elif code[pos] == '}':
                    depth -= 1
                pos += 1
            continue
        if ch == quote:
            return pos
        pos += 1
    return pos - 1


def _convert_esm_to_commonjs(code):
    """将 export function xxx 转换为宜搭 runtime 能识别的格式。

    宜搭 runtime（render-engine）期望代码是一个包裹在 function 中的块，
    通过 Babel 编译后产出 CommonJS 风格：
      "use strict";
      Object.defineProperty(exports, "__esModule", { value: true });
      exports.renderJsx = renderJsx;
      exports.didMount = didMount;
      ...
      function renderJsx() { ... }

    但实际上宜搭更简单：它直接 eval 代码后从 exports 对象取函数。
    """
    lines = code.split('\n')
    result_lines = ['"use strict";', '']
    export_names = []

    for line in lines:
        m = re.match(r'^export\s+function\s+(\w+)', line)
        if m:
            fn_name = m.group(1)
            export_names.append(fn_name)
            result_lines.append(line.replace('export ', '', 1))
        else:
            result_lines.append(line)

    if export_names:
        result_lines.append('')
        for name in export_names:
            result_lines.append(f'exports.{name} = {name};')

    return '\n'.join(result_lines)
