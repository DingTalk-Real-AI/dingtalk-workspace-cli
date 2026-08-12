#!/usr/bin/env python3
"""
宜搭报表 schema 生成/修改（编排：get-schema → apply chart changes → update-schema）

用法:
    python yida_report_update.py --app APP_X --form REPORT-XXX \\
        --changes-file changes.json --corp-id <corpId> --yes

changes.json 格式（非空数组，≤ 20 条）：

    [
      {"action": "add", "chart": {"type": "bar", "title": "新图", ...}, "after": "现有图标题"},
      {"action": "remove", "title": "废弃图表"},
      {"action": "replace", "title": "旧图", "chart": {"type": "line", ...}},
      {"action": "update-props", "title": "总览", "props": {"isHeightAuto": true}}
    ]

action: add / remove / replace / update-props

chart 对象格式、字段对象格式、数据绑定模型、布局系统、常见陷阱等完整规范，
请参考同目录下的参考文档：references/yida-report-builder.md

构建 chart 前必须先获取源表字段：
    dws yida form components --app <appType> --form <源表formUuid>

新建场景：CLI 先 `dws yida design form create --form-type report` 拿到 formUuid，
再调本脚本传全 add 的 changes → 自动走全量构建；即使空报表 schema 暂无
RootContent，也会自动生成完整报表骨架。

更新场景：传含 add/remove/replace/update-props 的 changes → 增量修改既有 schema。
"""
from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from pathlib import Path
from typing import Any

_SCRIPT_DIR = Path(__file__).resolve().parent
if str(_SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(_SCRIPT_DIR))

from yida_report_builder import (  # noqa: E402
    apply_chart_changes_to_schema,
    build_report_schema_with_filters,
    count_report_children,
)

MAX_CHANGES = 20
MAX_FILE_SIZE = 512 * 1024
MAX_INLINE_JSON = 64 * 1024


def _resolve_safe_path(path_str: str) -> Path:
    allowed_root = os.environ.get("OPENCLAW_WORKSPACE", os.getcwd())
    allowed_root_p = Path(allowed_root).resolve()
    target = Path(path_str).resolve() if Path(path_str).is_absolute() else (Path.cwd() / path_str).resolve()
    try:
        target.relative_to(allowed_root_p)
    except ValueError:
        raise ValueError(f"路径超出允许范围：{path_str}\n允许根目录：{allowed_root_p}")
    return target


def _run_dws(args: list[str], dry_run: bool = False) -> Any | None:
    cmd = ["dws"] + args
    if dry_run:
        print(f"  [dry-run] {' '.join(cmd)}")
        return {"dry_run": True}
    try:
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
    except FileNotFoundError:
        print("  ✗ 找不到 'dws' 命令", file=sys.stderr)
        return None
    except subprocess.TimeoutExpired:
        print("  ✗ dws 超时", file=sys.stderr)
        return None
    if result.returncode != 0:
        err = result.stderr.strip() or result.stdout.strip()
        print(f"  ✗ dws 失败 (exit {result.returncode}): {err}", file=sys.stderr)
        return None
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as e:
        print(f"  ✗ 非 JSON: {e}\n  输出: {result.stdout[:300]}", file=sys.stderr)
        return None


def _extract_payload(data: Any) -> Any:
    """Unwrap common dws JSON envelopes and parse JSON-string payloads."""
    current = data
    for _ in range(3):
        if isinstance(current, dict):
            for key in ("content", "result", "data"):
                if key in current and current[key] not in (None, ""):
                    current = current[key]
                    break
            else:
                break
            continue
        break

    if isinstance(current, str):
        stripped = current.strip()
        if stripped.startswith(("{", "[")):
            try:
                return json.loads(stripped)
            except json.JSONDecodeError:
                return current
    return current


def _extract_title(info: Any) -> str:
    payload = _extract_payload(info)
    if isinstance(payload, dict):
        title = payload.get("title") or payload.get("name") or ""
        if isinstance(title, dict):
            return str(title.get("zh_CN") or title.get("en_US") or "报表")
        if title:
            return str(title)
    return "报表"


def _load_changes(args: argparse.Namespace) -> list[dict]:
    if args.changes_file:
        safe = _resolve_safe_path(args.changes_file)
        if not safe.exists():
            raise ValueError(f"文件不存在: {safe}")
        if safe.stat().st_size > MAX_FILE_SIZE:
            raise ValueError(f"文件过大 (限制 {MAX_FILE_SIZE:,} 字节)")
        with safe.open("r", encoding="utf-8") as f:
            changes = json.load(f)
    elif args.changes_json:
        if len(args.changes_json.encode("utf-8")) > MAX_INLINE_JSON:
            raise ValueError("--changes-json 过长")
        changes = json.loads(args.changes_json)
    else:
        raise ValueError("必须提供 --changes-file 或 --changes-json")

    if not isinstance(changes, list) or not changes:
        raise ValueError("changes 必须是非空数组")
    if len(changes) > MAX_CHANGES:
        raise ValueError(f"changes 过多 ({len(changes)} > {MAX_CHANGES})")
    valid_actions = {"add", "remove", "replace", "update-props"}
    for i, c in enumerate(changes):
        if c.get("action") not in valid_actions:
            raise ValueError(f"change #{i+1} action 无效: {c.get('action')}")
    return changes


def main() -> int:
    ap = argparse.ArgumentParser(description="宜搭报表 schema 生成/修改",
                                 formatter_class=argparse.RawDescriptionHelpFormatter, epilog=__doc__)
    ap.add_argument("--app", required=True, help="应用编码 appType")
    ap.add_argument("--form", required=True, help="报表 formUuid")
    ap.add_argument("--changes-file", help="变更定义 JSON 文件路径")
    ap.add_argument("--changes-json", help="变更定义 JSON 内联")
    ap.add_argument("--corp-id", default="", help="企业 ID")
    ap.add_argument("--yes", action="store_true", help="确认写入")
    ap.add_argument("--dry-run", action="store_true", help="只生成不写入")
    ap.add_argument("--force-rebuild", action="store_true", help="强制全量重建（丢弃已有图表）")
    args = ap.parse_args()

    try:
        changes = _load_changes(args)
    except ValueError as e:
        print(f"错误: {e}", file=sys.stderr)
        return 1

    # Step 1
    print("Step 1/3: 获取现有 schema")
    resp = _run_dws(["yida", "design", "form", "get-schema", "--app", args.app,
                     "--form", args.form, "--format", "json"], dry_run=args.dry_run)
    if args.dry_run:
        print(json.dumps({"ok": True, "dry_run": True, "changeCount": len(changes)}, ensure_ascii=False))
        return 0
    if not resp:
        return 1
    schema = _extract_payload(resp)
    if not isinstance(schema, dict):
        print("错误: get-schema 返回内容不是对象，无法生成报表 schema", file=sys.stderr)
        return 1
    print("  ✓ 拿到 schema")

    # Step 2
    all_add = all(c.get("action") == "add" for c in changes)
    child_count = count_report_children(schema)

    do_full_build = False
    if child_count < 0:
        if all_add:
            if args.force_rebuild:
                print("  ⚠ schema 结构异常（找不到 RootContent），--force-rebuild 强制全量重建")
            else:
                print("  ⚠ schema 结构异常（找不到 RootContent）+ 全 add → 自动全量构建")
            do_full_build = True
        else:
            print("错误: schema 结构异常，找不到 RootContent，无法安全操作", file=sys.stderr)
            print("  提示: 若这是新建空报表，请传全 add；若是已有报表，请先确认 schema 是否完整", file=sys.stderr)
            return 1
    elif child_count == 0 and all_add:
        print("  ⚠ 空骨架 + 全 add → 全量构建")
        do_full_build = True
    elif args.force_rebuild and all_add:
        print(f"  ⚠ --force-rebuild: 强制全量重建（丢弃已有 {child_count} 个组件）")
        do_full_build = True

    try:
        if do_full_build:
            if not all_add:
                print("错误: 全量构建只支持全 add 操作", file=sys.stderr)
                return 1
            info = _run_dws(["yida", "design", "form", "get-info", "--app", args.app,
                             "--form", args.form, "--format", "json"])
            title = _extract_title(info)
            charts = [c["chart"] for c in changes]
            schema = build_report_schema_with_filters(report_title=title, charts=charts,
                                                      report_id=args.form, corp_id=args.corp_id)
        else:
            print(f"Step 2/3: 应用 {len(changes)} 条变更（已有 {child_count} 个组件）")
            schema = apply_chart_changes_to_schema(schema, changes, cube_tenant_id=args.corp_id)
    except ValueError as e:
        print(f"错误: {e}", file=sys.stderr)
        return 1
    print("  ✓ 变更完成")

    # Step 3
    schema_json = json.dumps(schema, ensure_ascii=False, separators=(",", ":"))
    print(f"Step 3/3: 写入 schema ({len(schema_json):,} 字节)")
    resp = _run_dws(["yida", "design", "form", "update-schema", "--app", args.app,
                     "--form", args.form, "--form-type", "report",
                     "--content", schema_json, "--yes", "--format", "json"])
    if not resp:
        return 1
    print("  ✓ 写入成功")
    print(json.dumps({"ok": True, "formUuid": args.form, "changeCount": len(changes)}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main())
