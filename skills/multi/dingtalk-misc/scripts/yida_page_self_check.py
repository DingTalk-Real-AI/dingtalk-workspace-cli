#!/usr/bin/env python3
"""Pure-Python self checks for Yida custom-page scripts.

This file lives under scripts/ because packaged workspace zips include scripts
but may exclude tests/. It intentionally does not touch remote Yida resources.

Usage:
  python yida_page_self_check.py
"""
from __future__ import annotations

import json
import re
import shutil
import sys
import tempfile
from argparse import Namespace
from pathlib import Path

_SCRIPT_DIR = Path(__file__).resolve().parent
if str(_SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(_SCRIPT_DIR))

import yida_custom_page_update as custom_update  # noqa: E402
import yida_form_inspector as form_inspector  # noqa: E402
import yida_jsx_pipeline as jsx_pipeline  # noqa: E402
from yida_jsx_pipeline import lint_check  # noqa: E402
from yida_page_compiler import build_page_source, compile_jsx_to_schema  # noqa: E402
from yida_page_generate import generate, manifest_path_for  # noqa: E402


def _args(template: str, output: Path, spec: str | None = None) -> Namespace:
    return Namespace(
        template=template,
        spec=spec,
        output=str(output),
        compile=True,
        form="FORM-SELF-CHECK",
        title=None,
        subtitle=None,
        brand_name=None,
        tagline=None,
        item=None,
        json=True,
    )


def _assert_generated_ok(template: str, output: Path, spec: dict | None = None) -> None:
    spec_path = None
    if spec is not None:
        spec_path = output.with_suffix(".spec.json")
        spec_path.write_text(json.dumps(spec, ensure_ascii=False), encoding="utf-8")

    result = generate(_args(template, output, str(spec_path) if spec_path else None))
    if not result.get("ok"):
        raise AssertionError(f"{template} generation failed: {result}")

    manifest = manifest_path_for(output)
    if not output.exists() or not manifest.exists():
        raise AssertionError(f"{template} did not write output/manifest")

    manifest_data = json.loads(manifest.read_text(encoding="utf-8"))
    if manifest_data.get("template") != template:
        raise AssertionError(f"{template} manifest template mismatch: {manifest_data}")

    source = output.read_text(encoding="utf-8")
    lint = lint_check(source, filename=str(output))
    if not lint.get("ok"):
        raise AssertionError(f"{template} lint failed: {lint}")

    compiled = compile_jsx_to_schema(source, form_uuid="FORM-SELF-CHECK")
    if not compiled.get("ok") or not compiled.get("schema"):
        raise AssertionError(f"{template} compile failed: {compiled.get('errors')}")


def check_page_generator() -> None:
    tmp = Path(tempfile.mkdtemp(prefix="dws-yida-self-check-"))
    try:
        _assert_generated_ok("product-homepage", tmp / "home.jsx", {
            "title": "解决方案中心",
            "subtitle": "稳定生成的宜搭自定义页",
            "features": [
                {"title": "客户洞察", "text": "聚合客户和拜访信息。"},
                {"title": "方案资产", "text": "沉淀可复用材料。"},
            ],
            "metrics": [
                {"value": "12", "label": "客户"},
                {"value": "5", "label": "方案"},
            ],
        })
        _assert_generated_ok("todo-mvc", tmp / "todo.jsx", {
            "title": "交付待办",
            "todos": [
                {"content": "确认字段", "done": True},
                {"content": "发布页面", "done": False},
            ],
        })
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def check_missing_custom_state_guard() -> None:
    bad_source = """export function renderJsx() {
  return <div>{_customState.title}</div>;
}
"""
    lint = lint_check(bad_source, filename="bad.jsx")
    rules = {item.get("rule") for item in lint.get("errors", [])}
    if "missing-custom-state" not in rules:
        raise AssertionError(f"missing-custom-state guard did not fire: {lint}")


def check_compiler_injects_custom_state() -> None:
    source = """export function renderJsx() {
  return React.createElement('div', null, _customState.title || 'ok');
}
"""
    result = build_page_source(source)
    if "var _customState = {};" not in result.get("code", ""):
        raise AssertionError("compiler did not inject missing _customState store")


def check_modern_hooks_authoring() -> None:
    source = """import React, { useState, useEffect } from 'react';
export default function Page() {
  var [count, setCount] = useState(0);
  useEffect(function() {
    setCount(1);
  }, []);
  return <div>{count}</div>;
}
"""
    lint = lint_check(source, filename="modern.jsx")
    if not lint.get("ok"):
        raise AssertionError(f"modern hooks authoring lint failed: {lint}")

    result = compile_jsx_to_schema(source, form_uuid="FORM-SELF-CHECK")
    if not result.get("ok") or not result.get("schema"):
        raise AssertionError(f"modern hooks authoring compile failed: {result.get('errors')}")

    compiled = result.get("compiled_code", "")
    if "useState" in compiled or "useEffect" in compiled or "import React" in compiled:
        raise AssertionError("modern hooks authoring was not lowered before publish")
    if "_customState" not in compiled or "exports.renderJsx" not in compiled:
        raise AssertionError("compiled hooks output is missing runtime contract")


def check_modern_helper_functions_are_bound() -> None:
    source = """import React, { useState, useEffect } from 'react';
export default function Page() {
  var [dataList, setDataList] = useState([]);
  useEffect(function() {
    loadData();
  }, []);
  var loadData = function() {
    setDataList([{ id: '1', title: 'ok' }]);
  };
  var renderListItem = function(item) {
    return <button onClick={function() { setDataList([]); }}>{item.title}</button>;
  };
  var renderListView = function() {
    if (!dataList || dataList.length === 0) {
      return <div>empty</div>;
    }
    return <div>{dataList.map(function(item) { return renderListItem(item); })}</div>;
  };
  return <div>{renderListView()}</div>;
}
"""
    result = compile_jsx_to_schema(source, form_uuid="FORM-SELF-CHECK", minify=False)
    if not result.get("ok") or not result.get("schema"):
        raise AssertionError(f"modern helper compile failed: {result.get('errors')}")

    intermediate = result.get("intermediate_code", "")
    required = [
        "export function loadData()",
        "export function renderListView()",
        "this.loadData();",
        "this.renderListView()",
        "this.renderListItem(item)",
    ]
    missing = [text for text in required if text not in intermediate]
    if missing:
        raise AssertionError(f"modern helper functions were not bound: missing={missing}\n{intermediate}")


def check_modern_functional_state_updater() -> None:
    source = """import React, { useState } from 'react';
export default function Page() {
  var [items, setItems] = useState([]);
  function addItem() {
    setItems(function(prev) {
      return prev.concat(['next']);
    });
  }
  return <button onClick={addItem}>{items.length}</button>;
}
"""
    result = build_page_source(source)
    code = result.get("code", "")
    if result.get("errors"):
        raise AssertionError(f"functional updater compile failed: {result.get('errors')}")
    if "setCustomState({ 'items': function(prev)" not in code:
        raise AssertionError(f"functional updater call was not preserved:\n{code}")
    if 'if (typeof value === "function")' not in code:
        raise AssertionError(f"setCustomState does not execute functional updaters:\n{code}")


def check_modern_render_derived_vars_stay_in_render() -> None:
    source = """import React, { useState } from 'react';
export default function Page() {
  var [items, setItems] = useState([]);
  function toRows(list) {
    return list.map(function(item) { return { label: item, value: item.length }; });
  }
  var rows = toRows(items);
  var maxValue = Math.max.apply(null, rows.map(function(row) { return row.value; }).concat([1]));
  return <div>{rows.map((row, idx) => <span key={idx}>{row.label}:{maxValue}</span>)}</div>;
}
"""
    result = compile_jsx_to_schema(source, form_uuid="FORM-SELF-CHECK", minify=False)
    if not result.get("ok") or not result.get("schema"):
        raise AssertionError(f"modern derived vars compile failed: {result.get('errors')}")
    compiled = result.get("compiled_code", "")
    render_idx = compiled.find("function renderJsx()")
    rows_idx = compiled.find("var rows =")
    max_idx = compiled.find("var maxValue =")
    if render_idx < 0 or rows_idx < render_idx or max_idx < render_idx:
        raise AssertionError(f"derived render vars escaped renderJsx:\n{compiled}")
    module_prefix = compiled[:render_idx]
    if "var maxValue =" in module_prefix or "var rows =" in module_prefix:
        raise AssertionError(f"derived render vars leaked to module scope:\n{compiled}")


def check_jsx_arrow_expression_body_is_transformed() -> None:
    source = """export function renderJsx() {
  var names = ['A'];
  return <select>{names.map((name, idx) => <option key={idx} value={name}>{name}</option>)}</select>;
}
"""
    result = compile_jsx_to_schema(source, form_uuid="FORM-SELF-CHECK", minify=False)
    if not result.get("ok") or not result.get("schema"):
        raise AssertionError(f"arrow JSX expression compile failed: {result.get('errors')}")
    compiled = result.get("compiled_code", "")
    if "<option" in compiled or "</option>" in compiled:
        raise AssertionError(f"arrow expression JSX was not transformed:\n{compiled}")
    if "React.createElement('option'" not in compiled:
        raise AssertionError(f"compiled option createElement missing:\n{compiled}")


def check_lint_allows_jsx_array_expression() -> None:
    source = """export function renderJsx() {
  return <div>{[['客户数', '1']].map((item, idx) => <span key={idx}>{item[0]}</span>)}</div>;
}
"""
    lint = lint_check(source, filename="array-expression.jsx")
    computed = [item for item in lint.get("errors", []) if item.get("rule") == "computed-property"]
    if computed:
        raise AssertionError(f"JSX array expression was misreported as computed property: {lint}")


def check_components_result_payload() -> None:
    payload = {
        "success": True,
        "result": [
            {
                "componentName": "Page",
                "label": "{\"en_US\":\"\",\"pureEn_US\":\"\",\"type\":\"i18n\",\"zh_CN\":\"\"}",
            },
            {
                "componentName": "FormContainer",
                "key": "formContainer_x",
                "label": "{\"en_US\":\"\",\"pureEn_US\":\"\",\"type\":\"i18n\",\"zh_CN\":\"\"}",
            },
            {
                "componentName": "TextField",
                "key": "textField_x",
                "label": "{\"en_US\":\"姓名\",\"pureEn_US\":\"姓名\",\"type\":\"i18n\",\"zh_CN\":\"姓名\"}",
                "parentId": "formContainer_x",
            },
        ],
    }

    old_pipeline_run_dws = jsx_pipeline._run_dws
    old_inspector_run_dws = form_inspector._run_dws
    try:
        jsx_pipeline._run_dws = lambda args: (payload, None)
        fields, err = jsx_pipeline.fetch_form_fields("APP_X", "FORM_X")
        normalized_fields = [jsx_pipeline._normalize_field(item) for item in (fields or [])]
        normalized = next((item for item in normalized_fields if item.get("fieldId") == "textField_x"), {})
        if err or normalized.get("fieldId") != "textField_x" or normalized.get("label") != "姓名":
            raise AssertionError(f"pipeline did not parse result payload: fields={fields} err={err}")
        if any(item.get("fieldId") == "formContainer_x" for item in normalized_fields):
            raise AssertionError(f"pipeline treated formContainer as a field: {normalized_fields}")

        form_inspector._run_dws = lambda args: payload
        comps = form_inspector._components("APP_X", "FORM_X")
        normalized_comps = [form_inspector._normalize_field(item) for item in (comps or [])]
        normalized_comp = next((item for item in normalized_comps if item.get("fieldId") == "textField_x"), {})
        if normalized_comp.get("fieldId") != "textField_x" or normalized_comp.get("label") != "姓名":
            raise AssertionError(f"inspector did not parse result payload: comps={comps}")
        if any(item.get("fieldId") == "formContainer_x" for item in normalized_comps):
            raise AssertionError(f"inspector treated formContainer as a field: {normalized_comps}")
    finally:
        jsx_pipeline._run_dws = old_pipeline_run_dws
        form_inspector._run_dws = old_inspector_run_dws


def check_render_side_effect_guard() -> None:
    bad_source = """export function renderJsx() {
  var self = this;
  if (self.didMountCalled !== true) {
    self.didMountCalled = true;
    self.loadLeaveRecords();
  }
  self.loadLeaveRecords = function() {
    Yida.api.form.searchFormDatasV2({
      appType: 'APP_X',
      formUuid: 'FORM-X',
      pageSize: 20
    }).then(function(res) {}).catch(function(err) {});
  };
  return <div>bad</div>;
}
"""
    lint = lint_check(bad_source, filename="bad-render.jsx")
    rules = {item.get("rule") for item in lint.get("errors", [])}
    expected = {"method-defined-in-render", "lifecycle-emulated-in-render", "api-call-in-render"}
    missing = expected - rules
    if missing:
        raise AssertionError(f"render side-effect guards did not fire: missing={sorted(missing)} lint={lint}")


def check_custom_page_update_guards() -> None:
    cases = [
        ({"formType": "display"}, "display"),
        ({"content": {"formType": "DISPLAY"}}, "display"),
        ({"data": {"type": "receipt"}}, "receipt"),
        ({"content": {"data": {"pageType": "process"}}}, "process"),
        ({}, ""),
    ]
    for payload, expected in cases:
        got = custom_update._extract_form_type(payload)
        if got != expected:
            raise AssertionError(f"_extract_form_type({payload!r})={got!r}, want {expected!r}")

    resolved = custom_update._resolve_safe_path("/private/tmp/dws-yida-page-test.jsx")
    if str(resolved) != "/private/tmp/dws-yida-page-test.jsx":
        raise AssertionError(f"unexpected /private/tmp resolution: {resolved}")


def check_packaged_doc_script_links() -> None:
    docs = [
        _SCRIPT_DIR.parent / "references" / "products" / "yida.md",
        _SCRIPT_DIR.parent / "references" / "products" / "yida-custom-page-codegen.md",
    ]
    missing = []
    link_re = re.compile(r"\]\((\.\./\.\./scripts/[^)]+\.py)\)")
    for doc in docs:
        if not doc.exists():
            missing.append(str(doc))
            continue
        for rel in link_re.findall(doc.read_text(encoding="utf-8")):
            target = (doc.parent / rel).resolve()
            if not target.exists():
                missing.append(f"{doc.name} -> {rel}")
    if missing:
        raise AssertionError("packaged doc script links missing: " + ", ".join(missing))


def run_all() -> None:
    check_page_generator()
    check_missing_custom_state_guard()
    check_compiler_injects_custom_state()
    check_modern_hooks_authoring()
    check_modern_helper_functions_are_bound()
    check_modern_functional_state_updater()
    check_modern_render_derived_vars_stay_in_render()
    check_jsx_arrow_expression_body_is_transformed()
    check_lint_allows_jsx_array_expression()
    check_components_result_payload()
    check_render_side_effect_guard()
    check_custom_page_update_guards()
    check_packaged_doc_script_links()


def main() -> int:
    try:
        run_all()
    except AssertionError as exc:
        print(f"[FAIL] {exc}", file=sys.stderr)
        return 1
    print("[OK] yida_page_self_check passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
