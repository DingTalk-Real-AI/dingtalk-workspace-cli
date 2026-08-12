#!/usr/bin/env python3
"""
yida_form_inspector.py — 跨表单元数据巡查工具

为「自定义页面联动其他表单」场景提供元数据采集能力，让 AI 在生成 JSX 前就能
拿到目标应用下所有表单的 formUuid + 字段 ID，避免硬编码错误。

【调用前必读】references/yida-custom-page-codegen.md §9
    跨表单联动 5 种模式（只读聚合 / 提交其他表 / Master-Detail / 多表 Dashboard /
    跨表搬运）。本脚本只负责取字段元数据，JSX 写法以 codegen.md 为准——不要
    看到 --help 就直接拼 JSX。

用法:
    # 列出应用下全部表单（含 formUuid / formType / title）
    python yida_form_inspector.py --action list-forms --app APP_X

    # 查看单张表单的字段（fieldId / dataType / label）
    python yida_form_inspector.py --action fields --app APP_X --form FORM-XXX

    # 一次性导出多张表单的字段汇总（推荐：跨表整合页面用）
    python yida_form_inspector.py --action bundle --app APP_X --forms FORM-A,FORM-B,FORM-C --output ./forms.json

    # 直接生成可粘贴到 JSX 的 FIELDS 常量代码
    python yida_form_inspector.py --action fields-snippet --app APP_X --form FORM-XXX
"""
from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path
from typing import Any


def _run_dws(args):
    cmd = ["dws"] + args
    try:
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
    except FileNotFoundError:
        print("[FAIL] 找不到 'dws' 命令，请确认已安装并在 PATH 中", file=sys.stderr)
        return None
    except subprocess.TimeoutExpired:
        print("[FAIL] dws 调用超时", file=sys.stderr)
        return None
    if result.returncode != 0:
        err = result.stderr.strip() or result.stdout.strip()
        print(f"[FAIL] dws 执行失败 (exit {result.returncode}): {err}", file=sys.stderr)
        return None
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        print(f"[FAIL] 输出非 JSON: {exc}\n{result.stdout[:300]}", file=sys.stderr)
        return None


def _list_forms(app, form_types=None):
    args = ["yida", "app", "list-forms", "--app", app, "--format", "json"]
    if form_types:
        args.extend(["--form-types", form_types])
    data = _run_dws(args)
    if data is None:
        return None
    if isinstance(data, dict):
        for key in ("forms", "data", "items", "list"):
            if isinstance(data.get(key), list):
                return data[key]
        return [data]
    if isinstance(data, list):
        return data
    return []


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
        match = _FIELD_ID_VALUE_RE.search(value)
        return match.group(0) if match else value
    key_value = _find_nested_value(obj, ("key", "name", "id"))
    if isinstance(key_value, str):
        match = _FIELD_ID_VALUE_RE.search(key_value)
        if match:
            return match.group(0)
    text = json.dumps(obj, ensure_ascii=False) if isinstance(obj, (dict, list)) else str(obj)
    match = _FIELD_ID_VALUE_RE.search(text)
    return match.group(0) if match else None


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
        field_id = _find_field_id(item)
        if field_id and field_id not in seen:
            seen.add(field_id)
            found.append(item)
    return found


def _components(app, form):
    data = _run_dws(["yida", "form", "components", "--app", app,
                     "--form", form, "--format", "json"])
    if data is None:
        return None
    return _extract_component_items(data)


def _normalize_field(comp):
    field_id = _find_field_id(comp)
    label = (_find_nested_value(comp, ("label", "title", "text", "displayName", "nameCn"))
             or "")
    label = _normalize_i18n_label(label)
    return {
        "fieldId": field_id,
        "label": label,
        "dataType": _find_nested_value(comp, ("dataType", "valueType")) or "",
        "componentName": _find_nested_value(comp, ("componentName", "type", "component")) or "",
    }


def _camel_safe(text, fallback):
    if not text:
        text = fallback
    cleaned = re.sub(r"[^A-Za-z0-9]+", " ", text).strip()
    if not cleaned:
        return fallback
    parts = cleaned.split()
    return parts[0].lower() + "".join(p.capitalize() for p in parts[1:])


def _action_list_forms(args):
    forms = _list_forms(args.app, args.form_types)
    if forms is None:
        return 1
    summary = []
    for f in forms:
        summary.append({
            "formUuid": f.get("formUuid") or f.get("formId") or f.get("uuid"),
            "title": f.get("title") or f.get("name"),
            "formType": f.get("formType") or f.get("type"),
            "appType": f.get("appType") or args.app,
        })
    print(json.dumps({"ok": True, "app": args.app, "count": len(summary),
                      "forms": summary}, ensure_ascii=False, indent=2))
    return 0


def _action_fields(args):
    if not args.form:
        print("错误: --action fields 需要 --form", file=sys.stderr)
        return 1
    comps = _components(args.app, args.form)
    if comps is None:
        return 1
    fields = [_normalize_field(c) for c in comps]
    print(json.dumps({"ok": True, "app": args.app, "form": args.form,
                      "count": len(fields), "fields": fields},
                     ensure_ascii=False, indent=2))
    return 0


def _action_fields_snippet(args):
    if not args.form:
        print("错误: --action fields-snippet 需要 --form", file=sys.stderr)
        return 1
    comps = _components(args.app, args.form)
    if comps is None:
        return 1
    lines = ["// 由 yida_form_inspector.py 自动生成",
             f"// 表单: {args.form}",
             "var FIELDS = {"]
    used = set()
    for idx, comp in enumerate(comps):
        f = _normalize_field(comp)
        if not f["fieldId"]:
            continue
        var_name = _camel_safe(f["label"], f"field{idx}")
        base = var_name
        n = 2
        while var_name in used:
            var_name = f"{base}{n}"
            n += 1
        used.add(var_name)
        comment = f"  // {f['label']} ({f['dataType']})" if f["label"] else ""
        lines.append(f"  {var_name}: '{f['fieldId']}',{comment}")
    lines.append("};")
    print("\n".join(lines))
    return 0


def _action_bundle(args):
    if not args.forms:
        print("错误: --action bundle 需要 --forms FORM-A,FORM-B,...", file=sys.stderr)
        return 1
    form_ids = [s.strip() for s in args.forms.split(",") if s.strip()]
    bundle = {"ok": True, "app": args.app, "forms": {}}
    for fid in form_ids:
        comps = _components(args.app, fid)
        if comps is None:
            bundle["forms"][fid] = {"ok": False, "error": "fetch_failed"}
            continue
        bundle["forms"][fid] = {
            "ok": True,
            "fields": [_normalize_field(c) for c in comps],
        }
    out = json.dumps(bundle, ensure_ascii=False, indent=2)
    if args.output:
        Path(args.output).write_text(out, encoding="utf-8")
        print(f"[OK] 已写入 {args.output}（{len(form_ids)} 张表单）")
    else:
        print(out)
    return 0


def main():
    ap = argparse.ArgumentParser(
        description=(
            "跨表单元数据巡查 / FIELDS 常量生成 "
            "【调用前必读】references/yida-custom-page-codegen.md §9"
            "（跨表单联动 5 种模式），不要只看 --help 就拼 JSX"
        ),
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    ap.add_argument("--action", required=True,
                    choices=["list-forms", "fields", "fields-snippet", "bundle"])
    ap.add_argument("--app", required=True, help="应用编码 appType")
    ap.add_argument("--form", help="单表 formUuid（fields / fields-snippet 用）")
    ap.add_argument("--forms", help="多表 formUuid 逗号分隔（bundle 用）")
    ap.add_argument("--form-types",
                    help="list-forms 过滤：receipt / process / report / display 等")
    ap.add_argument("--output", help="bundle 写入文件")
    args = ap.parse_args()

    handlers = {
        "list-forms": _action_list_forms,
        "fields": _action_fields,
        "fields-snippet": _action_fields_snippet,
        "bundle": _action_bundle,
    }
    return handlers[args.action](args)


if __name__ == "__main__":
    sys.exit(main())
