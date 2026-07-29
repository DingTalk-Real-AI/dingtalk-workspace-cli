"""
yida_page_schema.py — 宜搭自定义页面 schema 处理（builder + extractor）。

公开 API:
  Builder:
    - build_schema_content(source_code, compiled_code, form_uuid, existing_data_source=None) -> str
    - build_default_page_data_source(form_uuid) -> dict
    - merge_page_data_source(existing_data_source, generated_data_source) -> dict
  Extractor:
    - extract_source_code(schema) -> str
    - extract_compiled_code(schema) -> str
    - inject_source_code(schema, new_source, new_compiled) -> str

支持三种 schema 格式：
  1. 标准格式：actions.module.source / actions.module.compiled
  2. YidaCodeCanvas 格式：pages[0].componentsTree[*].props.code / runtimeCode
  3. YidaAICanvas 格式：同 YidaCodeCanvas 布局
"""

import copy
import json
import random
import time


# region: schema_builder
# ---------------------------------------------------------------------------
# 宜搭页面 schema JSON 外壳构建
# ---------------------------------------------------------------------------


def _create_node_id_generator():
    counter = [0]

    def next_node_id():
        counter[0] += 1
        ts = _base36(int(time.time() * 1000))
        return 'node_oc' + ts + _base36(counter[0])

    return next_node_id


def _base36(n):
    if n == 0:
        return '0'
    chars = '0123456789abcdefghijklmnopqrstuvwxyz'
    result = []
    while n > 0:
        result.append(chars[n % 36])
        n //= 36
    return ''.join(reversed(result))


def _generate_suffix():
    ts = _base36(int(time.time() * 1000))
    rand = _base36(random.randint(0, 36 ** 6))
    return ts + rand.ljust(6, '0')[:6]


def _get_global_data_source_fit_config():
    fit_compiled = (
        "'use strict';\n\nvar __preParser__ = function fit(response) {\n"
        "  var content = response.content !== undefined ? response.content : response;\n"
        "  var error = {\n"
        "    message: response.errorMsg || response.errors && response.errors[0] "
        "&& response.errors[0].msg || response.content || "
        "'远程数据源请求出错，success is false'\n"
        "  };\n  var success = true;\n"
        "  if (response.success !== undefined) {\n    success = response.success;\n"
        "  } else if (response.hasError !== undefined) {\n    success = !response.hasError;\n"
        "  }\n  return {\n    content: content,\n    success: success,\n    error: error\n  };\n};"
    )
    fit_source = (
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
    return {
        'fit': {
            'compiled': fit_compiled,
            'source': fit_source,
            'type': 'js',
            'error': {},
        },
    }


def build_default_page_data_source(form_uuid):
    url_params = {
        'id': 'VCB660714833IBHEOXK376TA7XJH2AXUWR8MMW',
        'name': 'urlParams',
        'description': '当前页面地址的参数：如 aliwork.com/APP_XXX/workbench?id=1&name=宜搭，'
                        '可通过 this.state.urlParams.name 获取到宜搭',
        'formUuid': form_uuid,
        'protocal': 'URI',
        'isReadonly': True,
    }
    timestamp = {
        'id': '',
        'name': 'timestamp',
        'description': '',
        'formUuid': form_uuid,
        'protocal': 'VALUE',
        'initialData': '',
    }
    return {
        'offline': [],
        'globalConfig': _get_global_data_source_fit_config(),
        'online': [url_params, timestamp],
        'list': [url_params, timestamp],
        'sync': True,
    }


def _is_built_in_page_data_source(item):
    if not item or not isinstance(item, dict):
        return False
    return item.get('name') in ('urlParams', 'timestamp')


def _get_data_source_identity(item):
    if not item or not isinstance(item, dict):
        return ''
    if _is_built_in_page_data_source(item):
        return 'builtin:' + item['name']
    if item.get('id'):
        return 'id:' + item['id']
    if item.get('name') and item.get('protocal'):
        return 'name:' + item['name'] + '|protocal:' + item['protocal']
    if item.get('name'):
        return 'name:' + item['name']
    return json.dumps(item, ensure_ascii=False, sort_keys=True)


def _merge_data_source_array(existing_items, generated_items):
    merged = copy.deepcopy(existing_items) if isinstance(existing_items, list) else []
    seen = set()
    for item in merged:
        identity = _get_data_source_identity(item)
        if identity:
            seen.add(identity)

    for item in (generated_items if isinstance(generated_items, list) else []):
        identity = _get_data_source_identity(item)
        if not identity or identity not in seen:
            merged.append(copy.deepcopy(item))
            if identity:
                seen.add(identity)
    return merged


def merge_page_data_source(existing_data_source, generated_data_source):
    if not existing_data_source or not isinstance(existing_data_source, dict):
        return copy.deepcopy(generated_data_source)

    existing = copy.deepcopy(existing_data_source)
    generated = copy.deepcopy(generated_data_source) if generated_data_source else {}
    merged = {**generated, **existing}

    merged['offline'] = _merge_data_source_array(existing.get('offline'), generated.get('offline'))
    merged['online'] = _merge_data_source_array(existing.get('online'), generated.get('online'))
    merged['list'] = _merge_data_source_array(existing.get('list'), generated.get('list'))
    merged['globalConfig'] = {
        **(generated.get('globalConfig') or {}),
        **(existing.get('globalConfig') or {}),
    }
    merged['sync'] = existing.get('sync') if existing.get('sync') is not None else generated.get('sync')
    return merged


def build_schema_content(source_code, compiled_code, form_uuid, existing_data_source=None):
    """把 sourceCode + compiledCode 包装成完整的宜搭自定义页面 schema JSON 字符串。"""
    next_node_id = _create_node_id_generator()

    constructor_code = (
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

    page_data_source = merge_page_data_source(
        existing_data_source,
        build_default_page_data_source(form_uuid),
    )

    schema = {
        'schemaType': 'superform',
        'schemaVersion': '5.0',
        'pages': [
            {
                'utils': [
                    {
                        'name': 'legaoBuiltin',
                        'type': 'npm',
                        'content': {
                            'package': '@ali/vu-legao-builtin',
                            'version': '3.0.0',
                            'exportName': 'legaoBuiltin',
                        },
                    },
                    {
                        'name': 'yidaPlugin',
                        'type': 'npm',
                        'content': {
                            'package': '@ali/vu-yida-plugin',
                            'version': '1.1.0',
                            'exportName': 'yidaPlugin',
                        },
                    },
                ],
                'componentsMap': [
                    {'package': '@ali/vc-deep-yida', 'version': '1.5.169', 'componentName': 'RootHeader'},
                    {'package': '@ali/vc-deep-yida', 'version': '1.5.169', 'componentName': 'Jsx'},
                    {'package': '@ali/vc-deep-yida', 'version': '1.5.169', 'componentName': 'RootContent'},
                    {'package': '@ali/vc-deep-yida', 'version': '1.5.169', 'componentName': 'RootFooter'},
                    {'package': '@ali/vc-deep-yida', 'version': '1.5.169', 'componentName': 'Page'},
                ],
                'componentsTree': [
                    {
                        'componentName': 'Page',
                        'id': next_node_id(),
                        'props': {
                            'contentBgColor': 'white',
                            'pageStyle': {'backgroundColor': '#f2f3f5'},
                            'contentMargin': '0',
                            'contentPadding': '0',
                            'showTitle': False,
                            'contentPaddingMobile': '0',
                            'templateVersion': '1.0.0',
                            'contentMarginMobile': '0',
                            'className': 'page_' + _generate_suffix(),
                            'contentBgColorMobile': 'white',
                        },
                        'condition': True,
                        'css': (
                            'body{background-color:#f2f3f5}'
                            '.vc-page-yida-page{--yida-form-content-padding:0;'
                            '--yida-form-content-margin:0;--yida-layout-padding:0}'
                            '.vc-deep-container-entry.vc-rootcontent{padding:0!important;'
                            'margin-top:0!important;margin-right:0!important;'
                            'margin-bottom:0!important;margin-left:0!important}'
                        ),
                        'methods': {
                            '__initMethods__': {
                                'type': 'js',
                                'source': 'function (exports, module) { /*set actions code here*/ }',
                                'compiled': 'function (exports, module) { /*set actions code here*/ }',
                            },
                        },
                        'dataSource': page_data_source,
                        'lifeCycles': {
                            'constructor': {
                                'type': 'js',
                                'compiled': constructor_code,
                                'source': constructor_code,
                            },
                            'componentWillUnmount': {
                                'name': 'didUnmount',
                                'id': 'didUnmount',
                                'type': 'actionRef',
                                'params': {},
                            },
                            'componentDidMount': {
                                'name': 'didMount',
                                'id': 'didMount',
                                'params': {},
                                'type': 'actionRef',
                            },
                        },
                        'hidden': False,
                        'title': '',
                        'isLocked': False,
                        'conditionGroup': '',
                        'children': [
                            {
                                'componentName': 'RootHeader',
                                'id': next_node_id(),
                                'props': {},
                                'condition': True,
                                'hidden': False,
                                'title': '',
                                'isLocked': False,
                                'conditionGroup': '',
                            },
                            {
                                'componentName': 'RootContent',
                                'id': next_node_id(),
                                'props': {},
                                'condition': True,
                                'hidden': False,
                                'title': '',
                                'isLocked': False,
                                'conditionGroup': '',
                                'children': [
                                    {
                                        'componentName': 'Jsx',
                                        'id': next_node_id(),
                                        'props': {
                                            'render': {
                                                'type': 'js',
                                                'compiled': (
                                                    'function main(){\n    \n    "use strict";\n\n'
                                                    'var __compiledFunc__ = function render() {\n'
                                                    '  return this.renderJsx();\n};\n'
                                                    '    return __compiledFunc__.apply(this, arguments);\n  }'
                                                ),
                                                'source': (
                                                    'function render() {\n'
                                                    '  return this.renderJsx();\n}'
                                                ),
                                                'error': {},
                                            },
                                            '__style__': {},
                                            'fieldId': 'jsx_' + _generate_suffix(),
                                        },
                                        'condition': True,
                                        'hidden': False,
                                        'title': '',
                                        'isLocked': False,
                                        'conditionGroup': '',
                                    },
                                ],
                            },
                            {
                                'componentName': 'RootFooter',
                                'id': next_node_id(),
                                'props': {},
                                'condition': True,
                                'hidden': False,
                                'title': '',
                                'isLocked': False,
                                'conditionGroup': '',
                            },
                        ],
                    },
                ],
                'id': form_uuid,
                'connectComponent': [],
            },
        ],
        'actions': {
            'module': {
                'compiled': compiled_code,
                'source': source_code,
            },
            'type': 'FUNCTION',
            'list': [
                {'id': 'getCustomState', 'title': 'getCustomState'},
                {'id': 'setCustomState', 'title': 'setCustomState'},
                {'id': 'forceUpdate', 'title': 'forceUpdate'},
                {'id': 'didMount', 'title': 'didMount'},
                {'id': 'didUnmount', 'title': 'didUnmount'},
                {'id': 'renderJsx', 'title': 'renderJsx'},
            ],
        },
        'config': {
            'connectComponent': [],
        },
    }

    return json.dumps(schema, ensure_ascii=False)


# endregion


# region: schema_extractor
# ---------------------------------------------------------------------------
# 从 schema 中提取 / 注入源码（兼容标准 / YidaCodeCanvas / YidaAICanvas）
# ---------------------------------------------------------------------------


def _parse_schema(schema):
    if isinstance(schema, str):
        return json.loads(schema)
    if isinstance(schema, dict):
        return schema
    raise TypeError(f"schema must be dict or JSON string, got {type(schema).__name__}")


def _find_code_canvas_node(data: dict):
    """在 componentsTree 中查找 YidaCodeCanvas 或 YidaAICanvas 节点"""
    try:
        root = data['pages'][0]['componentsTree'][0]
        found = _search_canvas_recursive(root)
        if found:
            return found
    except (KeyError, IndexError, TypeError):
        pass
    return None


def _search_canvas_recursive(node: dict):
    if node.get('componentName') in ('YidaCodeCanvas', 'YidaAICanvas'):
        return node
    for child in node.get('children', []):
        found = _search_canvas_recursive(child)
        if found:
            return found
    return None


def _detect_format(data: dict) -> str:
    """检测 schema 格式：'standard' 或 'code_canvas'"""
    canvas = _find_code_canvas_node(data)
    if canvas and canvas.get('props', {}).get('code'):
        return 'code_canvas'

    try:
        if data['actions']['module']['source']:
            return 'standard'
    except (KeyError, TypeError):
        pass

    raise ValueError(
        "Cannot detect schema format: neither actions.module.source "
        "nor YidaCodeCanvas/YidaAICanvas.props.code found."
    )


def extract_source_code(schema) -> str:
    """从宜搭自定义页面 schema 提取原始 JSX 源码。"""
    data = _parse_schema(schema)
    fmt = _detect_format(data)

    if fmt == 'code_canvas':
        canvas = _find_code_canvas_node(data)
        code = canvas.get('props', {}).get('code')
        if not code or not isinstance(code, str):
            raise ValueError("YidaCodeCanvas/YidaAICanvas.props.code is empty or not a string")
        return code

    try:
        source = data['actions']['module']['source']
    except (KeyError, TypeError):
        raise ValueError("Cannot find actions.module.source")
    if not isinstance(source, str):
        raise ValueError(f"actions.module.source is not a string, got {type(source).__name__}")
    return source


def extract_compiled_code(schema) -> str:
    """从宜搭自定义页面 schema 提取编译后代码。"""
    data = _parse_schema(schema)
    fmt = _detect_format(data)

    if fmt == 'code_canvas':
        canvas = _find_code_canvas_node(data)
        runtime_code = canvas.get('props', {}).get('runtimeCode')
        if not runtime_code or not isinstance(runtime_code, str):
            raise ValueError("YidaCodeCanvas/YidaAICanvas.props.runtimeCode is empty or not a string")
        return runtime_code

    try:
        compiled = data['actions']['module']['compiled']
    except (KeyError, TypeError):
        raise ValueError("Cannot find actions.module.compiled")
    if not isinstance(compiled, str):
        raise ValueError(f"actions.module.compiled is not a string, got {type(compiled).__name__}")
    return compiled


def inject_source_code(schema, new_source: str, new_compiled: str) -> str:
    """将新的源码和编译产物注入 schema，返回更新后的 JSON 字符串。"""
    data = _parse_schema(schema)
    data = copy.deepcopy(data)
    fmt = _detect_format(data)

    if fmt == 'code_canvas':
        canvas = _find_code_canvas_node(data)
        if not canvas:
            raise ValueError("Cannot find YidaCodeCanvas/YidaAICanvas node for injection")
        canvas['props']['code'] = new_source
        canvas['props']['runtimeCode'] = new_compiled
    else:
        if 'actions' not in data or not isinstance(data['actions'], dict):
            raise ValueError("Invalid schema: missing 'actions' key")
        if 'module' not in data['actions'] or not isinstance(data['actions']['module'], dict):
            raise ValueError("Invalid schema: missing 'actions.module' key")
        data['actions']['module']['source'] = new_source
        data['actions']['module']['compiled'] = new_compiled

    return json.dumps(data, ensure_ascii=False)


# endregion


__all__ = [
    'build_schema_content',
    'build_default_page_data_source',
    'merge_page_data_source',
    'extract_source_code',
    'extract_compiled_code',
    'inject_source_code',
]
