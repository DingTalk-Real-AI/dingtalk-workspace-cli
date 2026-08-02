#!/usr/bin/env python3
"""
宜搭表单 schema 生成/修改（编排：get-schema → apply changes → update-schema）

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
用法:
    python yida_form_update.py --app APP_X --form FORM-XXX --changes-file fields.json --yes
    python yida_form_update.py --app APP_X --form FORM-XXX --changes-json '[...]' --yes

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
changes.json 格式（非空数组，≤ 30 条）：

    [
      {"action": "add", "field": {"type": "TextField", "label": "备注"}, "after": "请假事由"},
      {"action": "update", "label": "天数", "changes": {"required": true, "suffix": "天"}},
      {"action": "delete", "label": "废弃字段"}
    ]

action 支持: add / update / delete
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
新建场景：CLI 先 `dws yida design form create` 拿到 formUuid，
再调本脚本传全 add 的 changes → 自动走 build_form_schema 全量构建。

更新场景：传含 add/update/delete 的 changes → 增量修改既有 schema。
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
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

from yida_form_builder import apply_changes_to_schema, build_form_schema, is_empty_skeleton  # noqa: E402

MAX_CHANGES = 30
MAX_CHANGES_FILE_SIZE = 512 * 1024
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


def _extract_schema(resp: Any) -> dict[str, Any] | None:
    if isinstance(resp, dict) and isinstance(resp.get("content"), dict):
        return resp["content"]
    if isinstance(resp, dict):
        return resp
    return None


def _load_changes(args: argparse.Namespace) -> list[dict]:
    if args.changes_file:
        safe = _resolve_safe_path(args.changes_file)
        if not safe.exists():
            raise ValueError(f"文件不存在: {safe}")
        if safe.stat().st_size > MAX_CHANGES_FILE_SIZE:
            raise ValueError(f"文件过大 (限制 {MAX_CHANGES_FILE_SIZE:,} 字节)")
        with safe.open("r", encoding="utf-8") as f:
            changes = json.load(f)
    elif args.changes_json:
        if len(args.changes_json.encode("utf-8")) > MAX_INLINE_JSON:
            raise ValueError(f"--changes-json 过长 (限制 {MAX_INLINE_JSON:,} 字节)")
        changes = json.loads(args.changes_json)
    else:
        raise ValueError("必须提供 --changes-file 或 --changes-json")

    if not isinstance(changes, list) or not changes:
        raise ValueError("changes 必须是非空数组")
    if len(changes) > MAX_CHANGES:
        raise ValueError(f"changes 过多 ({len(changes)} > {MAX_CHANGES})")
    for i, c in enumerate(changes):
        if not isinstance(c, dict):
            raise ValueError(f"change #{i+1} 必须是对象")
        if c.get("action") not in ("add", "update", "delete"):
            raise ValueError(f"change #{i+1} action 无效: {c.get('action')}")
    return changes


def main() -> int:
    ap = argparse.ArgumentParser(description="宜搭表单 schema 生成/修改",
                                 formatter_class=argparse.RawDescriptionHelpFormatter, epilog=__doc__)
    ap.add_argument("--app", required=True, help="应用编码 appType")
    ap.add_argument("--form", required=True, help="表单 formUuid")
    ap.add_argument("--changes-file", help="变更定义 JSON 文件路径")
    ap.add_argument("--changes-json", help="变更定义 JSON 内联")
    ap.add_argument("--corp-id", default="", help="企业 ID")
    ap.add_argument("--yes", action="store_true", help="确认写入")
    ap.add_argument("--dry-run", action="store_true", help="只生成不写入")
    args = ap.parse_args()

    try:
        changes = _load_changes(args)
    except ValueError as e:
        print(f"错误: {e}", file=sys.stderr)
        return 1

    # Step 1: 拉取现有 schema
    print("Step 1/3: 获取现有 schema")
    resp = _run_dws(["yida", "design", "form", "get-schema", "--app", args.app,
                     "--form", args.form, "--format", "json"], dry_run=args.dry_run)
    if args.dry_run:
        print("  [dry-run] 跳过 get-schema")
        print(json.dumps({"ok": True, "dry_run": True, "changeCount": len(changes)}, ensure_ascii=False))
        return 0
    if not resp:
        return 1
    schema = _extract_schema(resp)
    if not schema:
        print("错误: get-schema 返回结构异常，未找到 schema", file=sys.stderr)
        return 1
    print("  ✓ 拿到 schema")

    # Step 2: 空骨架自愈
    all_add = all(c.get("action") == "add" for c in changes)
    if is_empty_skeleton(schema):
        if not all_add:
            print("错误: 表单是空骨架，但 changes 含 update/delete；空表单只能用全 add", file=sys.stderr)
            return 1
        print("  ⚠ 空骨架 + 全 add → 全量构建")
        info = _run_dws(["yida", "design", "form", "get-info", "--app", args.app,
                         "--form", args.form, "--format", "json"])
        title = (info.get("title", "") or "未命名") if info else "未命名"
        fields = [c["field"] for c in changes]
        schema = build_form_schema(form_title=title, fields=fields, form_uuid=args.form,
                                   corp_id=args.corp_id, app_type=args.app)
    else:
        print(f"Step 2/3: 应用 {len(changes)} 条变更")
        schema = apply_changes_to_schema(schema, changes, corp_id=args.corp_id,
                                         app_type=args.app, form_uuid=args.form)
    print("  ✓ 本地变更完成")

    # Step 3: 写回
    schema_json = json.dumps(schema, ensure_ascii=False, separators=(",", ":"))
    print(f"Step 3/3: 写入 schema ({len(schema_json):,} 字节)")
    resp = _run_dws(["yida", "design", "form", "update-schema", "--app", args.app,
                     "--form", args.form, "--content", schema_json, "--yes", "--format", "json"])
    if not resp:
        return 1
    print("  ✓ 写入成功")
    print(json.dumps({"ok": True, "formUuid": args.form, "changeCount": len(changes)}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main())
