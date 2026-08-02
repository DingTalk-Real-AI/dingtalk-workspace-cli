#!/usr/bin/env python3
"""Yida process draft save/publish helper.

Flow for creating a new process definition:

  1. create draft from the initial/published process id
  2. generate process schema with yida_process_flow.build_flow_schema
  3. save draft with `dws yida design process update`
  4. optionally publish with `dws yida design process publish`

The flow schema is the same shape used by integration automation:
`build_automation_flow` output from references/yida-process-node.md can
be passed directly via --flow-file / --flow-json.
"""
from __future__ import annotations

import argparse
import copy
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any

_SCRIPT_DIR = Path(__file__).resolve().parent
if str(_SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(_SCRIPT_DIR))

from yida_process_flow import (  # noqa: E402
    FlowSchemaError,
    build_flow_schema,
    build_process_view_schema,
    merge_process_view_schema,
)


MAX_FLOW_FILE_SIZE = 1024 * 1024
MAX_INLINE_JSON = 256 * 1024


def _gather_allowed_roots() -> list[Path]:
    roots: list[Path] = []
    extra = os.environ.get("OPENYIDA_ALLOWED_ROOTS", "")
    if extra:
        parts: list[str] = [extra]
        for sep in (os.pathsep, ":", ","):
            parts = [seg for chunk in parts for seg in chunk.split(sep)]
        roots.extend(Path(part).expanduser().resolve() for part in parts if part.strip())
    legacy = os.environ.get("OPENCLAW_WORKSPACE")
    if legacy:
        roots.append(Path(legacy).expanduser().resolve())
    roots.append(Path.cwd().resolve())
    roots.append(Path(tempfile.gettempdir()).resolve())
    roots.append(Path("/tmp").resolve())
    roots.append(Path("/private/tmp").resolve())

    seen: set[str] = set()
    result: list[Path] = []
    for root in roots:
        key = str(root)
        if key not in seen:
            seen.add(key)
            result.append(root)
    return result


def _resolve_safe_path(path_str: str) -> Path:
    target = Path(path_str).expanduser()
    target = target.resolve() if target.is_absolute() else (Path.cwd() / target).resolve()
    roots = _gather_allowed_roots()
    for root in roots:
        try:
            target.relative_to(root)
            return target
        except ValueError:
            continue
    listing = "\n  - ".join(str(root) for root in roots)
    raise ValueError(f"路径超出允许范围：{path_str}\n已尝试的允许根目录：\n  - {listing}")


def _load_json(file_path: str | None, inline_json: str | None, label: str) -> Any:
    if file_path:
        safe = _resolve_safe_path(file_path)
        if not safe.exists():
            raise ValueError(f"{label} 文件不存在: {safe}")
        if safe.stat().st_size > MAX_FLOW_FILE_SIZE:
            raise ValueError(f"{label} 文件过大 (限制 {MAX_FLOW_FILE_SIZE:,} 字节)")
        return json.loads(safe.read_text(encoding="utf-8"))
    if inline_json:
        if len(inline_json.encode("utf-8")) > MAX_INLINE_JSON:
            raise ValueError(f"{label} 内联 JSON 过长 (限制 {MAX_INLINE_JSON:,} 字节)")
        return json.loads(inline_json)
    raise ValueError(f"必须提供 --{label}-file 或 --{label}-json")


def _run_dws(args: list[str], dry_run: bool = False) -> Any | None:
    cmd = ["dws"] + args
    if dry_run:
        print(f"  [dry-run] {' '.join(cmd)}")
        return {"dry_run": True}
    try:
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
    except FileNotFoundError:
        print("  [FAIL] 找不到 'dws' 命令", file=sys.stderr)
        return None
    except subprocess.TimeoutExpired:
        print("  [FAIL] dws 超时", file=sys.stderr)
        return None
    if result.returncode != 0:
        err = result.stderr.strip() or result.stdout.strip()
        print(f"  [FAIL] dws 失败 (exit {result.returncode}): {err}", file=sys.stderr)
        return None
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        print(f"  [FAIL] 非 JSON: {exc}\n  输出: {result.stdout[:300]}", file=sys.stderr)
        return None


def _iter_dicts(value: Any):
    if isinstance(value, dict):
        yield value
        for child in value.values():
            yield from _iter_dicts(child)
    elif isinstance(value, list):
        for child in value:
            yield from _iter_dicts(child)


def _iter_flow_nodes(nodes: list[dict[str, Any]]):
    for node in nodes:
        yield node
        child_nodes = node.get("childNodes") or []
        if isinstance(child_nodes, list):
            yield from _iter_flow_nodes(child_nodes)


def _extract_payload(data: Any) -> Any:
    if isinstance(data, dict):
        for key in ("result", "content", "data"):
            value = data.get(key)
            if value is not None:
                return value
    return data


def _loads_maybe(value: Any) -> Any:
    if isinstance(value, str):
        try:
            return json.loads(value)
        except json.JSONDecodeError:
            return value
    return value


def _i18n_text(value: Any) -> str:
    value = _loads_maybe(value)
    if isinstance(value, dict):
        return str(value.get("zh_CN") or value.get("en_US") or value.get("pureEn_US") or value.get("value") or "")
    return str(value or "")


def _extract_records(data: Any) -> list[Any]:
    payload = _extract_payload(data)
    payload = _loads_maybe(payload)
    if isinstance(payload, dict):
        for key in ("data", "result", "records", "list"):
            value = payload.get(key)
            if isinstance(value, list):
                return value
    if isinstance(payload, list):
        return payload
    return []


def _load_form_titles(app_type: str, dry_run: bool = False) -> dict[str, str]:
    resp = _run_dws([
        "yida", "app", "list-forms",
        "--app", app_type,
        "--format", "json",
    ], dry_run=dry_run)
    titles: dict[str, str] = {}
    if not resp or dry_run:
        return titles
    for item in _extract_records(resp):
        item = _loads_maybe(item)
        if not isinstance(item, dict):
            continue
        form_uuid = item.get("formUuid")
        if isinstance(form_uuid, str) and form_uuid:
            titles[form_uuid] = _i18n_text(item.get("title")) or form_uuid
    return titles


def _field_from_component(item: Any) -> dict[str, Any] | None:
    item = _loads_maybe(item)
    if not isinstance(item, dict):
        return None
    field_id = item.get("key") or item.get("fieldId") or item.get("componentId") or item.get("id")
    component_name = item.get("componentName")
    if not isinstance(field_id, str) or not field_id:
        return None
    if component_name in {"Page", "FormContainer"}:
        return None
    label = _i18n_text(item.get("label") or item.get("varName")) or field_id
    return {
        "fieldId": field_id,
        "name": field_id,
        "value": field_id,
        "label": label,
        "text": label,
        "componentName": component_name,
        "componentOption": "[]",
        "required": False,
        "componentProps": {
            "defaultDataSource": {},
            "relateAppType": "",
            "relateOrderEnable": False,
            "relateOrderConfig": [],
        },
    }


def _load_form_schema(app_type: str, form_uuid: str, dry_run: bool = False) -> dict[str, Any] | None:
    resp = _run_dws([
        "yida", "design", "form", "get-schema",
        "--app", app_type,
        "--form", form_uuid,
        "--format", "json",
    ], dry_run=dry_run)
    if not resp or dry_run:
        return None
    payload = _extract_payload(resp)
    payload = _loads_maybe(payload)
    return payload if isinstance(payload, dict) else None


def _iter_schema_components(value: Any, parent: dict[str, Any] | None = None):
    value = _loads_maybe(value)
    if isinstance(value, list):
        for item in value:
            yield from _iter_schema_components(item, parent)
        return
    if not isinstance(value, dict):
        return

    current_parent = parent
    if isinstance(value.get("componentName"), str):
        yield value, parent
        current_parent = value

    for key in ("pages", "componentsTree", "children"):
        child = value.get(key)
        if isinstance(child, (list, dict)):
            yield from _iter_schema_components(child, current_parent)


def _field_from_schema_node(item: Any, parent: dict[str, Any] | None = None) -> dict[str, Any] | None:
    item = _loads_maybe(item)
    if not isinstance(item, dict):
        return None
    component_name = item.get("componentName")
    props = item.get("props") if isinstance(item.get("props"), dict) else {}
    field_id = props.get("fieldId") or item.get("key") or item.get("fieldId") or item.get("componentId") or item.get("id")
    if not isinstance(field_id, str) or not field_id:
        return None
    if component_name in {"Page", "RootHeader", "RootContent", "RootFooter", "FormContainer"}:
        return None

    label = _i18n_text(props.get("label") or item.get("label") or item.get("varName")) or field_id
    field_props = copy.deepcopy(props)
    if parent and isinstance(parent, dict):
        parent_component = parent.get("componentName")
        parent_props = parent.get("props") if isinstance(parent.get("props"), dict) else {}
        parent_id = parent_props.get("fieldId") or parent.get("fieldId") or parent.get("id")
        if parent_component:
            field_props.setdefault("parentComponentName", parent_component)
        if parent_id:
            field_props.setdefault("parentId", parent_id)

    return {
        "fieldId": field_id,
        "name": field_id,
        "value": field_id,
        "label": label,
        "text": label,
        "componentName": component_name,
        "componentOption": "[]",
        "required": bool(props.get("required", False)),
        "props": field_props,
        "componentProps": {
            "defaultDataSource": copy.deepcopy(props.get("defaultDataSource") or {}),
            "relateAppType": props.get("relateAppType", ""),
            "relateOrderEnable": bool(props.get("relateOrderEnable", False)),
            "relateOrderConfig": copy.deepcopy(props.get("relateOrderConfig") or []),
        },
    }


def _load_form_fields_from_schema(app_type: str, form_uuid: str, dry_run: bool = False) -> list[dict[str, Any]]:
    schema = _load_form_schema(app_type, form_uuid, dry_run=dry_run)
    if not schema:
        return []
    fields: list[dict[str, Any]] = []
    seen: set[str] = set()
    for item, parent in _iter_schema_components(schema):
        field = _field_from_schema_node(item, parent)
        if not field:
            continue
        field_id = str(field.get("fieldId") or "")
        if field_id in seen:
            continue
        seen.add(field_id)
        fields.append(field)
    return fields


def _load_form_fields(app_type: str, form_uuid: str, dry_run: bool = False) -> list[dict[str, Any]]:
    fields = _load_form_fields_from_schema(app_type, form_uuid, dry_run=dry_run)
    if fields:
        return fields

    resp = _run_dws([
        "yida", "form", "components",
        "--app", app_type,
        "--form", form_uuid,
        "--format", "json",
    ], dry_run=dry_run)
    if not resp or dry_run:
        return []
    fields = []
    for item in _extract_records(resp):
        field = _field_from_component(item)
        if field:
            fields.append(field)
    return fields


def _collect_data_create_targets(flow: dict[str, Any], default_app: str) -> set[tuple[str, str]]:
    targets: set[tuple[str, str]] = set()
    for node in _iter_flow_nodes(flow.get("nodes") or []):
        props = node.get("props") or {}
        node_type = str(node.get("type") or "")
        if node_type == "dataCreate":
            form_uuid = props.get("formUuid")
        elif node_type == "dataRetrieve":
            form_uuid = props.get("sourceId")
        else:
            continue
        if not isinstance(form_uuid, str) or not form_uuid:
            continue
        if node_type == "dataRetrieve" and not form_uuid.startswith("FORM-"):
            continue
        app_type = props.get("appType") if isinstance(props.get("appType"), str) else ""
        targets.add((app_type or default_app, form_uuid))
    return targets


def _build_data_create_metadata(flow: dict[str, Any], app_type: str, dry_run: bool = False) -> dict[tuple[str, str], dict[str, Any]]:
    targets = _collect_data_create_targets(flow, app_type)
    if not targets:
        return {}

    titles_by_app: dict[str, dict[str, str]] = {}
    metadata: dict[tuple[str, str], dict[str, Any]] = {}
    for target_app, form_uuid in sorted(targets):
        if target_app not in titles_by_app:
            titles_by_app[target_app] = _load_form_titles(target_app, dry_run=dry_run)
        metadata[(target_app, form_uuid)] = {
            "title": titles_by_app[target_app].get(form_uuid, form_uuid),
            "fields": _load_form_fields(target_app, form_uuid, dry_run=dry_run),
        }
    return metadata


def _load_source_view(app_type: str, process_code: str, process_id: str, dry_run: bool = False) -> dict[str, Any] | None:
    if not app_type or not process_code or not process_id or dry_run:
        return None
    resp = _run_dws([
        "yida", "design", "process", "get",
        "--app", app_type,
        "--process-code", process_code,
        "--process-id", process_id,
        "--format", "json",
    ], dry_run=dry_run)
    if not resp:
        return None
    for item in _iter_dicts(resp):
        view_json = item.get("viewJson")
        if isinstance(view_json, str) and view_json:
            parsed = _loads_maybe(view_json)
            if isinstance(parsed, dict):
                return parsed
    return None


def _extract_process_id(data: Any) -> str:
    for item in _iter_dicts(data):
        for key in ("processId", "id", "processVersionId", "result", "content"):
            value = item.get(key)
            if isinstance(value, (str, int)) and str(value):
                return str(value)
    return ""


def _load_optional_text(file_path: str | None, inline_value: str | None, label: str) -> str:
    if file_path:
        safe = _resolve_safe_path(file_path)
        if not safe.exists():
            raise ValueError(f"{label} 文件不存在: {safe}")
        if safe.stat().st_size > MAX_FLOW_FILE_SIZE:
            raise ValueError(f"{label} 文件过大 (限制 {MAX_FLOW_FILE_SIZE:,} 字节)")
        return safe.read_text(encoding="utf-8")
    return inline_value or ""


def _summarize_flow(flow: dict[str, Any]) -> list[str]:
    nodes = list(_iter_flow_nodes(flow.get("nodes") or []))
    counts: dict[str, int] = {}
    for node in nodes:
        node_type = str(node.get("type") or "unknown")
        counts[node_type] = counts.get(node_type, 0) + 1

    lines = [
        f"节点数: {len(nodes)}",
        "节点类型: " + ", ".join(f"{key}={counts[key]}" for key in sorted(counts)),
    ]

    data_targets = []
    card_rules = []
    for node in nodes:
        props = node.get("props") if isinstance(node.get("props"), dict) else {}
        node_name = _i18n_text(node.get("name")) or str(node.get("nodeId") or "")
        node_type = str(node.get("type") or "")
        if node_type == "dataCreate":
            data_targets.append(f"{node_name}->{props.get('appType') or '<当前应用>'}/{props.get('formUuid')}")
        if node_type == "sendCard":
            biz_id = props.get("bizId") if isinstance(props.get("bizId"), dict) else {}
            card_rules.append(
                f"{node_name}: page={props.get('cardPageCode')}, sendType={props.get('sendType')}, "
                f"bizId={biz_id.get('value')}"
            )

    if data_targets:
        lines.append("新增数据目标: " + "; ".join(data_targets))
    if card_rules:
        lines.append("卡片规则: " + "; ".join(card_rules))
    return lines


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(
        description="宜搭流程定义保存/发布：新建草稿 -> 生成流程 schema -> 保存流程 -> 可选发布",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    ap.add_argument("--app", help="应用编码 appType；--schema-only 时可不传")
    ap.add_argument("--form", help="流程表单 formUuid；创建草稿时必填")
    ap.add_argument("--process-code", required=True, help="流程 code，如 TPROC-XXX")
    ap.add_argument("--source-process-id", help="源流程版本 id，用于 create-draft")
    ap.add_argument("--draft-process-id", help="已有草稿 processId；传入后跳过 create-draft")
    ap.add_argument("--flow-file", help="流程 schema / build_automation_flow 输入 JSON 文件")
    ap.add_argument("--flow-json", help="流程 schema / build_automation_flow 输入内联 JSON")
    ap.add_argument("--view-file", help="可选 viewJson 文件")
    ap.add_argument("--view-json", help="可选内联 viewJson")
    ap.add_argument("--publish", action="store_true", help="保存后发布流程")
    ap.add_argument("--schema-only", action="store_true", help="只输出生成后的流程 schema，不调用 dws")
    ap.add_argument("--yes", action="store_true", help="确认执行保存/发布")
    ap.add_argument("--dry-run", action="store_true", help="打印将执行的命令，不调用 dws")
    args = ap.parse_args(argv)

    try:
        flow_spec = _load_json(args.flow_file, args.flow_json, "flow")
        flow = build_flow_schema(flow_spec, process_code=args.process_code)
        if args.form:
            flow.setdefault("props", {})["bindingForm"] = args.form
        view_content = _load_optional_text(args.view_file, args.view_json, "view") if (args.view_file or args.view_json) else ""
    except (ValueError, json.JSONDecodeError, FlowSchemaError) as exc:
        print(f"错误: {exc}", file=sys.stderr)
        return 1

    flow_json = json.dumps(flow, ensure_ascii=False, separators=(",", ":"))

    if args.schema_only:
        print(json.dumps(flow, ensure_ascii=False, indent=2))
        return 0

    if not args.app:
        print("错误: 保存/发布流程必须提供 --app", file=sys.stderr)
        return 1

    if not args.yes and not args.dry_run:
        print("错误: 保存/发布流程是高影响操作，必须显式传 --yes", file=sys.stderr)
        return 1

    if not args.draft_process_id and (not args.form or not args.source_process_id):
        print("错误: 未传 --draft-process-id 时，必须提供 --form 和 --source-process-id 创建草稿", file=sys.stderr)
        return 1

    if not view_content:
        print("Step 0/4: 生成流程设计器 viewJson")
        print("  [检查] 流程摘要")
        for line in _summarize_flow(flow):
            print(f"    - {line}")
        metadata = _build_data_create_metadata(flow, args.app, dry_run=args.dry_run)
        source_view_id = args.source_process_id or args.draft_process_id or ""
        source_view = _load_source_view(args.app, args.process_code, source_view_id, dry_run=args.dry_run)
        if source_view:
            view = merge_process_view_schema(source_view, flow, data_create_metadata=metadata)
        else:
            view = build_process_view_schema(flow, data_create_metadata=metadata)
        view_content = json.dumps(view, ensure_ascii=False, separators=(",", ":"))
        print(f"  [OK] viewJson {len(view_content):,} 字节")

    # Step 1: create or reuse draft
    draft_process_id = args.draft_process_id or ""
    if draft_process_id:
        print(f"Step 1/4: 复用草稿 processId={draft_process_id}")
    else:
        print("Step 1/4: 新建流程草稿")
        resp = _run_dws([
            "yida", "design", "process", "create-draft",
            "--app", args.app,
            "--form", args.form,
            "--process-id", args.source_process_id,
            "--format", "json",
        ], dry_run=args.dry_run)
        if args.dry_run:
            draft_process_id = "<draft-process-id>"
        else:
            if not resp:
                return 1
            draft_process_id = _extract_process_id(resp)
            if not draft_process_id:
                print("  [FAIL] create-draft 返回中未找到 processId", file=sys.stderr)
                return 1
            print(f"  [OK] 新草稿 processId={draft_process_id}")

    print(f"Step 2/4: 生成流程 schema ({len(flow_json):,} 字节)")

    # Step 3: save draft
    print("Step 3/4: 保存流程草稿")
    update_args = [
        "yida", "design", "process", "update",
        "--app", args.app,
        "--process-code", args.process_code,
        "--process-id", draft_process_id,
        "--content", flow_json,
        "--yes",
        "--format", "json",
    ]
    if view_content:
        update_args.extend(["--view-content", view_content])
    resp = _run_dws(update_args, dry_run=args.dry_run)
    if not args.dry_run and not resp:
        return 1

    # Step 4: optional publish
    if args.publish:
        print("Step 4/4: 发布流程")
        resp = _run_dws([
            "yida", "design", "process", "publish",
            "--app", args.app,
            "--process-code", args.process_code,
            "--process-id", draft_process_id,
            "--yes",
            "--format", "json",
        ], dry_run=args.dry_run)
        if not args.dry_run and not resp:
            return 1
    else:
        print("Step 4/4: 跳过发布（未传 --publish）")

    print(json.dumps({
        "ok": True,
        "app": args.app,
        "processCode": args.process_code,
        "draftProcessId": draft_process_id,
        "schemaSize": len(flow_json),
        "published": bool(args.publish),
        "dryRun": bool(args.dry_run),
    }, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
