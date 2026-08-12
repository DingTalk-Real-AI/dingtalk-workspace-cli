#!/usr/bin/env python3
"""Generate stable Yida custom-page source from a small JSON spec.

The generator is intentionally pure Python and emits conservative Yida runtime
source: named exports plus React.createElement calls. This avoids the most
common failure modes in hand-written JSX: unsupported hooks, event binding
mistakes, computed object keys, and fragile JSX transforms.

Usage:
  python yida_page_generate.py product-homepage --spec page.json --output page.jsx --compile
  python yida_page_generate.py todo-mvc --output todo.jsx --title "团队待办"
"""
from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Any

_SCRIPT_DIR = Path(__file__).resolve().parent
if str(_SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(_SCRIPT_DIR))

from yida_jsx_pipeline import lint_check  # noqa: E402
from yida_page_compiler import compile_jsx_to_schema  # noqa: E402

IR_VERSION = "1.0"
KNOWN_TEMPLATES = ("product-homepage", "todo-mvc")


def _json(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, indent=2)


def _safe_filename_stem(value: str, fallback: str) -> str:
    text = re.sub(r"[^A-Za-z0-9_.-]+", "-", (value or "").strip()).strip(".-")
    return text or fallback


def _read_spec(path: str | None) -> dict[str, Any]:
    if not path:
        return {}
    spec_path = Path(path).expanduser().resolve()
    if not spec_path.exists():
        raise ValueError(f"spec 文件不存在: {spec_path}")
    try:
        data = json.loads(spec_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise ValueError(f"spec 不是合法 JSON: {exc}") from exc
    if not isinstance(data, dict):
        raise ValueError("spec 顶层必须是 JSON object")
    return data


def _merge_cli_vars(spec: dict[str, Any], args: argparse.Namespace) -> dict[str, Any]:
    merged = dict(spec)
    for key in ("title", "subtitle", "brand_name", "tagline"):
        value = getattr(args, key, None)
        if value:
            merged[key] = value
    if args.item:
        merged["items"] = [{"title": item, "text": ""} for item in args.item]
    return merged


def _list_of_dicts(value: Any, fallback: list[dict[str, Any]]) -> list[dict[str, Any]]:
    if not isinstance(value, list):
        return fallback
    result: list[dict[str, Any]] = []
    for item in value:
        if isinstance(item, dict):
            result.append(dict(item))
        elif item is not None:
            result.append({"title": str(item), "text": ""})
    return result or fallback


def _normalize_product_homepage(raw: dict[str, Any]) -> dict[str, Any]:
    features = _list_of_dicts(raw.get("features") or raw.get("items"), [
        {"title": "统一入口", "text": "把常用流程、表单和报表聚合到一个页面。"},
        {"title": "实时概览", "text": "用清晰的指标帮助团队快速判断当前状态。"},
        {"title": "低风险交付", "text": "使用稳定模板生成，减少运行时白屏概率。"},
    ])
    metrics = _list_of_dicts(raw.get("metrics"), [
        {"value": "3", "label": "核心模块"},
        {"value": "100%", "label": "纯 Python 生成"},
        {"value": "0", "label": "外部依赖"},
    ])
    actions = _list_of_dicts(raw.get("actions"), [
        {"title": "查看能力", "target": "features"},
        {"title": "查看指标", "target": "metrics"},
    ])
    return {
        "irVersion": IR_VERSION,
        "template": "product-homepage",
        "title": str(raw.get("title") or raw.get("brandName") or raw.get("brand_name") or "宜搭自定义页面"),
        "subtitle": str(raw.get("subtitle") or raw.get("tagline") or "稳定生成、可检查、可发布的自定义页面骨架"),
        "featuresTitle": str(raw.get("featuresTitle") or "核心能力"),
        "metricsTitle": str(raw.get("metricsTitle") or "关键指标"),
        "features": features[:8],
        "metrics": metrics[:6],
        "actions": actions[:4],
    }


def _normalize_todo_mvc(raw: dict[str, Any]) -> dict[str, Any]:
    todos = _list_of_dicts(raw.get("todos") or raw.get("items"), [
        {"content": "确认页面需求", "done": True},
        {"content": "生成稳定源码", "done": False},
        {"content": "发布前 dry-run 校验", "done": False},
    ])
    normalized_todos = []
    for item in todos[:20]:
        normalized_todos.append({
            "content": str(item.get("content") or item.get("title") or "未命名任务"),
            "done": bool(item.get("done")),
        })
    return {
        "irVersion": IR_VERSION,
        "template": "todo-mvc",
        "title": str(raw.get("title") or "团队待办"),
        "subtitle": str(raw.get("subtitle") or "验证状态、事件、列表渲染和本地交互的稳定模板"),
        "placeholder": str(raw.get("placeholder") or "输入任务后点击添加"),
        "todos": normalized_todos,
    }


def normalize_spec(template: str, raw: dict[str, Any]) -> dict[str, Any]:
    if template == "product-homepage":
        return _normalize_product_homepage(raw)
    if template == "todo-mvc":
        return _normalize_todo_mvc(raw)
    raise ValueError(f"未知模板: {template}")


def _common_runtime() -> str:
    return """function h(type, props) {
  var children = [];
  for (var i = 2; i < arguments.length; i++) {
    var child = arguments[i];
    if (child === null || typeof child === 'undefined' || child === false) {
      continue;
    }
    if (Array.isArray(child)) {
      for (var j = 0; j < child.length; j++) {
        if (child[j] !== null && typeof child[j] !== 'undefined' && child[j] !== false) {
          children.push(child[j]);
        }
      }
    } else {
      children.push(child);
    }
  }
  return React.createElement.apply(React, [type, props || null].concat(children));
}

function mergeStyle(base, extra) {
  var out = {};
  var key;
  for (key in (base || {})) {
    if (Object.prototype.hasOwnProperty.call(base, key)) {
      out[key] = base[key];
    }
  }
  for (key in (extra || {})) {
    if (Object.prototype.hasOwnProperty.call(extra, key)) {
      out[key] = extra[key];
    }
  }
  return out;
}

var _customState = {};

export function getCustomState(key) {
  if (key) {
    return _customState[key];
  }
  var out = {};
  var stateKey;
  for (stateKey in _customState) {
    if (Object.prototype.hasOwnProperty.call(_customState, stateKey)) {
      out[stateKey] = _customState[stateKey];
    }
  }
  return out;
}

export function setCustomState(newState) {
  var data = newState || {};
  Object.keys(data).forEach(function(key) {
    _customState[key] = data[key];
  });
  this.forceUpdate();
}

export function forceUpdate() {
  this.setState({ timestamp: new Date().getTime() });
}

export function didMount() {}

export function didUnmount() {}
"""


def render_product_homepage(ir: dict[str, Any]) -> str:
    return f"""/* Generated by dws yida_page_generate.py. Edit the spec/manifest first when possible. */
var PAGE_SPEC = {_json(ir)};

{_common_runtime()}

export function scrollToSection(id) {{
  if (!id || typeof document === 'undefined') {{
    return;
  }}
  var node = document.getElementById(id);
  if (node && node.scrollIntoView) {{
    node.scrollIntoView({{ behavior: 'smooth', block: 'start' }});
  }}
}}

export function renderJsx() {{
  var self = this;
  var features = [];
  var metrics = [];
  var actions = [];
  var i;
  var featureCardStyle = {{
    padding: 18,
    borderRadius: 8,
    backgroundColor: '#ffffff',
    border: '1px solid #e5e7eb',
    boxShadow: '0 1px 4px rgba(15, 23, 42, 0.06)'
  }};
  var actionStyle = {{
    height: 36,
    padding: '0 14px',
    borderRadius: 6,
    border: '1px solid #2563eb',
    backgroundColor: '#2563eb',
    color: '#ffffff',
    cursor: 'pointer'
  }};

  for (i = 0; i < PAGE_SPEC.features.length; i++) {{
    features.push(h('div', {{ key: 'feature-' + i, style: featureCardStyle }},
      h('div', {{ style: {{ fontSize: 16, fontWeight: 700, color: '#111827', marginBottom: 8 }} }}, PAGE_SPEC.features[i].title),
      h('div', {{ style: {{ fontSize: 13, lineHeight: 1.7, color: '#4b5563' }} }}, PAGE_SPEC.features[i].text)
    ));
  }}

  for (i = 0; i < PAGE_SPEC.metrics.length; i++) {{
    metrics.push(h('div', {{ key: 'metric-' + i, style: {{ minWidth: 120 }} }},
      h('div', {{ style: {{ fontSize: 26, fontWeight: 800, color: '#111827' }} }}, PAGE_SPEC.metrics[i].value),
      h('div', {{ style: {{ fontSize: 13, color: '#6b7280', marginTop: 4 }} }}, PAGE_SPEC.metrics[i].label)
    ));
  }}

  for (i = 0; i < PAGE_SPEC.actions.length; i++) {{
    actions.push(h('button', {{
      key: 'action-' + i,
      type: 'button',
      style: i === 0 ? actionStyle : mergeStyle(actionStyle, {{ backgroundColor: '#ffffff', color: '#2563eb' }}),
      onClick: function(target) {{
        return function(e) {{
          if (e && e.preventDefault) {{
            e.preventDefault();
          }}
          self.scrollToSection(target);
        }};
      }}(PAGE_SPEC.actions[i].target)
    }}, PAGE_SPEC.actions[i].title));
  }}

  return h('div', {{ style: {{ minHeight: '100vh', padding: 24, backgroundColor: '#f3f4f6', color: '#111827', boxSizing: 'border-box' }} }},
    h('div', {{ style: {{ display: 'none' }} }}, this.state && this.state.timestamp),
    h('section', {{ style: {{ maxWidth: 1120, margin: '0 auto', padding: '34px 0 22px' }} }},
      h('div', {{ style: {{ fontSize: 32, lineHeight: 1.25, fontWeight: 800, marginBottom: 12 }} }}, PAGE_SPEC.title),
      h('div', {{ style: {{ maxWidth: 720, fontSize: 15, lineHeight: 1.8, color: '#4b5563', marginBottom: 20 }} }}, PAGE_SPEC.subtitle),
      h('div', {{ style: {{ display: 'flex', flexWrap: 'wrap', gap: 10 }} }}, actions)
    ),
    h('section', {{ id: 'metrics', style: {{ maxWidth: 1120, margin: '0 auto 18px', padding: 20, borderRadius: 8, backgroundColor: '#ffffff', border: '1px solid #e5e7eb' }} }},
      h('div', {{ style: {{ fontSize: 15, fontWeight: 700, marginBottom: 16 }} }}, PAGE_SPEC.metricsTitle),
      h('div', {{ style: {{ display: 'flex', flexWrap: 'wrap', gap: 28 }} }}, metrics)
    ),
    h('section', {{ id: 'features', style: {{ maxWidth: 1120, margin: '0 auto', padding: '10px 0 28px' }} }},
      h('div', {{ style: {{ fontSize: 20, fontWeight: 800, margin: '10px 0 14px' }} }}, PAGE_SPEC.featuresTitle),
      h('div', {{ style: {{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 14 }} }}, features)
    )
  );
}}
"""


def render_todo_mvc(ir: dict[str, Any]) -> str:
    return f"""/* Generated by dws yida_page_generate.py. Edit the spec/manifest first when possible. */
var PAGE_SPEC = {_json(ir)};

{_common_runtime()}

_customState = {{
  todos: PAGE_SPEC.todos || [],
  draft: ''
}};

export function updateDraft(e) {{
  _customState.draft = e && e.target ? e.target.value : '';
}}

export function addTodo(e) {{
  if (e && e.preventDefault) {{
    e.preventDefault();
  }}
  var text = String(_customState.draft || '').replace(/^\\s+|\\s+$/g, '');
  if (!text) {{
    if (this.utils && this.utils.toast) {{
      this.utils.toast({{ title: '请输入待办内容', type: 'warning' }});
    }}
    return;
  }}
  var next = [];
  for (var i = 0; i < _customState.todos.length; i++) {{
    next.push(_customState.todos[i]);
  }}
  next.push({{ content: text, done: false }});
  this.setCustomState({{ todos: next, draft: '' }});
}}

export function toggleTodo(index) {{
  var next = [];
  for (var i = 0; i < _customState.todos.length; i++) {{
    var item = _customState.todos[i] || {{}};
    next.push({{ content: item.content, done: i === index ? !item.done : !!item.done }});
  }}
  this.setCustomState({{ todos: next }});
}}

export function clearDone() {{
  var next = [];
  for (var i = 0; i < _customState.todos.length; i++) {{
    if (!_customState.todos[i].done) {{
      next.push(_customState.todos[i]);
    }}
  }}
  this.setCustomState({{ todos: next }});
}}

export function renderJsx() {{
  var self = this;
  var todos = _customState.todos || [];
  var rows = [];
  var doneCount = 0;
  var i;

  for (i = 0; i < todos.length; i++) {{
    if (todos[i].done) {{
      doneCount += 1;
    }}
    rows.push(h('div', {{ key: 'todo-' + i, style: {{ display: 'flex', alignItems: 'center', gap: 10, padding: '12px 0', borderBottom: '1px solid #eef2f7' }} }},
      h('button', {{
        type: 'button',
        style: {{ width: 26, height: 26, borderRadius: 6, border: '1px solid #cbd5e1', backgroundColor: todos[i].done ? '#16a34a' : '#ffffff', color: '#ffffff', cursor: 'pointer' }},
        onClick: function(index) {{
          return function(e) {{
            if (e && e.preventDefault) {{
              e.preventDefault();
            }}
            self.toggleTodo(index);
          }};
        }}(i)
      }}, todos[i].done ? '✓' : ''),
      h('span', {{ style: {{ flex: 1, color: todos[i].done ? '#94a3b8' : '#111827', textDecoration: todos[i].done ? 'line-through' : 'none' }} }}, todos[i].content)
    ));
  }}

  return h('div', {{ style: {{ minHeight: '100vh', padding: 24, backgroundColor: '#f8fafc', color: '#111827', boxSizing: 'border-box' }} }},
    h('div', {{ style: {{ display: 'none' }} }}, this.state && this.state.timestamp),
    h('section', {{ style: {{ maxWidth: 760, margin: '0 auto', padding: 22, borderRadius: 8, backgroundColor: '#ffffff', border: '1px solid #e5e7eb' }} }},
      h('div', {{ style: {{ fontSize: 26, fontWeight: 800, marginBottom: 8 }} }}, PAGE_SPEC.title),
      h('div', {{ style: {{ fontSize: 14, color: '#64748b', marginBottom: 18 }} }}, PAGE_SPEC.subtitle),
      h('div', {{ style: {{ display: 'flex', gap: 10, marginBottom: 12 }} }},
        h('input', {{
          key: 'draft-' + todos.length + '-' + String(_customState.draft || '').length,
          defaultValue: _customState.draft || '',
          placeholder: PAGE_SPEC.placeholder,
          style: {{ flex: 1, height: 38, padding: '0 12px', borderRadius: 6, border: '1px solid #cbd5e1', outline: 'none' }},
          onChange: function(e) {{ self.updateDraft(e); }},
          onKeyDown: function(e) {{
            if (e && e.key === 'Enter') {{
              self.addTodo(e);
            }}
          }}
        }}),
        h('button', {{ type: 'button', style: {{ height: 38, padding: '0 14px', borderRadius: 6, border: '1px solid #2563eb', backgroundColor: '#2563eb', color: '#ffffff', cursor: 'pointer' }}, onClick: function(e) {{ self.addTodo(e); }} }}, '添加')
      ),
      h('div', {{ style: {{ display: 'flex', justifyContent: 'space-between', fontSize: 13, color: '#64748b', margin: '8px 0 4px' }} }},
        h('span', null, '共 ' + todos.length + ' 项'),
        h('span', null, '已完成 ' + doneCount + ' 项')
      ),
      h('div', null, rows.length ? rows : h('div', {{ style: {{ padding: 26, textAlign: 'center', color: '#94a3b8' }} }}, '暂无待办')),
      h('div', {{ style: {{ marginTop: 14, textAlign: 'right' }} }},
        h('button', {{ type: 'button', style: {{ height: 34, padding: '0 12px', borderRadius: 6, border: '1px solid #cbd5e1', backgroundColor: '#ffffff', color: '#334155', cursor: 'pointer' }}, onClick: function(e) {{ if (e && e.preventDefault) {{ e.preventDefault(); }} self.clearDone(); }} }}, '清除已完成')
      )
    )
  );
}}
"""


def render_source(ir: dict[str, Any]) -> str:
    template = ir.get("template")
    if template == "product-homepage":
        return render_product_homepage(ir)
    if template == "todo-mvc":
        return render_todo_mvc(ir)
    raise ValueError(f"未知模板: {template}")


def manifest_path_for(output: Path) -> Path:
    suffix = "".join(output.suffixes)
    stem = output.name[:-len(suffix)] if suffix else output.stem
    return output.with_name(f"{stem}.dws-yida-page.json")


def write_generated(output: Path, source: str, ir: dict[str, Any]) -> Path:
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(source, encoding="utf-8")
    manifest_path = manifest_path_for(output)
    manifest_path.write_text(_json(ir) + "\n", encoding="utf-8")
    return manifest_path


def generate(args: argparse.Namespace) -> dict[str, Any]:
    raw = _merge_cli_vars(_read_spec(args.spec), args)
    template = args.template or raw.get("template") or "product-homepage"
    if template not in KNOWN_TEMPLATES:
        raise ValueError(f"未知模板: {template}; 可选: {', '.join(KNOWN_TEMPLATES)}")
    ir = normalize_spec(template, raw)
    output = Path(args.output or raw.get("output") or f"{_safe_filename_stem(ir.get('title', ''), template)}.jsx")
    output = output.expanduser().resolve()
    source = render_source(ir)

    lint = lint_check(source, filename=str(output))
    if not lint.get("ok"):
        return {"ok": False, "stage": "lint", "errors": lint.get("errors", []), "warnings": lint.get("warnings", [])}

    compile_result: dict[str, Any] | None = None
    if args.compile:
        compile_result = compile_jsx_to_schema(source, form_uuid=args.form or "FORM-GENERATED-PREVIEW")
        if not compile_result.get("ok"):
            return {
                "ok": False,
                "stage": "compile",
                "errors": compile_result.get("errors", []),
                "warnings": compile_result.get("lint", {}).get("warnings", []),
            }

    manifest = write_generated(output, source, ir)
    result = {
        "ok": True,
        "template": template,
        "output": str(output),
        "manifest": str(manifest),
        "lint": lint.get("info", {}),
    }
    if compile_result is not None:
        result["compiledSize"] = len(compile_result.get("compiled_code", ""))
        result["schemaSize"] = len(compile_result.get("schema", ""))
    return result


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="纯 Python 宜搭自定义页面稳定生成器")
    parser.add_argument("template", nargs="?", choices=KNOWN_TEMPLATES, help="页面模板")
    parser.add_argument("--spec", help="页面 JSON spec")
    parser.add_argument("--output", help="输出源码路径，建议 pages/src/<name>.jsx")
    parser.add_argument("--compile", action="store_true", help="生成前先通过纯 Python 编译校验")
    parser.add_argument("--form", help="编译校验用 formUuid，默认 FORM-GENERATED-PREVIEW")
    parser.add_argument("--title", help="页面标题")
    parser.add_argument("--subtitle", help="页面副标题")
    parser.add_argument("--brand-name", dest="brand_name", help="品牌名，等价于 title")
    parser.add_argument("--tagline", help="标语，等价于 subtitle")
    parser.add_argument("--item", action="append", help="快速添加 feature/todo 项，可重复")
    parser.add_argument("--json", action="store_true", help="只输出 JSON 结果")
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    try:
        result = generate(args)
    except ValueError as exc:
        print(f"错误: {exc}", file=sys.stderr)
        return 1
    if args.json:
        print(json.dumps(result, ensure_ascii=False, indent=2))
    else:
        if result.get("ok"):
            print(f"[OK] 已生成: {result['output']}")
            print(f"[OK] Manifest: {result['manifest']}")
            if "compiledSize" in result:
                print(f"[OK] 编译校验通过: compiledSize={result['compiledSize']} schemaSize={result['schemaSize']}")
        else:
            print(json.dumps(result, ensure_ascii=False, indent=2), file=sys.stderr)
            return 1
    return 0 if result.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())
