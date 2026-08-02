#!/usr/bin/env python3
"""
宜搭自定义页面 schema 生成/修改（编排：get-schema → 编译 + 构建 → update-schema）

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
【调用前必读】references/yida-custom-page-codegen.md
    （JSX 入口签名 / Hooks 限制 / 行内样式 / 跨表单联动 5 种模式 / SEARCH/REPLACE
    增量改写 / 常见坑速查表）。本脚本 --help 仅给出基本用法，**不要**只看
    --help 就直接拼 JSX，几乎必踩坑。
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
用法:
    python yida_custom_page_update.py --app APP_X --form FORM-XXX --code-file page.jsx --yes
    python yida_custom_page_update.py --app APP_X --form FORM-XXX --code 'import ...' --yes
    python yida_custom_page_update.py --app APP_X --form FORM-XXX --show-current

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
--show-current 模式：
    只拉取现有 schema 并输出当前代码，不做修改。

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
写入模式：
    全量替换代码。脚本调用纯 Python 编译管线（JSX→createElement 转换 +
    Hooks 兼容层 _customState/didMount），构建标准 Jsx 组件 schema，并保留
    page_id 和已有 dataSource。**零第三方依赖**（仅需 Python 3.7+ 标准库，
    无需 pip install、无需 Node.js）。空页面和已有代码的页面均可使用。

    跨表单联动场景：JSX 内可通过 Yida.api.form.* 直接读写同应用内任意表单，
    搭配 `yida_form_inspector.py --action fields-snippet` 取目标表的字段常量片段。
    详见 references/yida-custom-page-codegen.md §9。

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

from yida_page_compiler import compile_jsx_to_schema  # noqa: E402
from yida_page_schema import extract_source_code  # noqa: E402
from yida_jsx_pipeline import field_check, lint_check  # noqa: E402

MAX_CODE_FILE_SIZE = 1 * 1024 * 1024
MAX_INLINE_CODE = 200 * 1024


def _gather_allowed_roots() -> list[Path]:
    """收集所有允许的路径根目录。任一命中即放行，详见 _resolve_safe_path。

    优先级（靠前的优先，仅影响报错提示顺序）：
      1. OPENYIDA_ALLOWED_ROOTS：显式多根，以 os.pathsep / ':' / ',' 分隔。
      2. OPENCLAW_WORKSPACE：老环境变量，向后兼容。
      3. 当前工作目录 cwd：兼容原行为。
      4. 临时目录 tempdir：供脚本中转使用。
    """
    roots: list[Path] = []
    extra = os.environ.get("OPENYIDA_ALLOWED_ROOTS", "")
    if extra:
        seps = [os.pathsep, ":", ","]
        parts: list[str] = [extra]
        for sep in seps:
            parts = [seg for chunk in parts for seg in chunk.split(sep)]
        for part in parts:
            part = part.strip()
            if part:
                roots.append(Path(part).expanduser().resolve())
    legacy = os.environ.get("OPENCLAW_WORKSPACE")
    if legacy:
        roots.append(Path(legacy).expanduser().resolve())
    roots.append(Path.cwd().resolve())
    import tempfile as _tempfile
    roots.append(Path(_tempfile.gettempdir()).resolve())
    roots.append(Path("/tmp").resolve())
    roots.append(Path("/private/tmp").resolve())
    # 去重保序
    seen: set[str] = set()
    uniq: list[Path] = []
    for r in roots:
        s = str(r)
        if s not in seen:
            seen.add(s)
            uniq.append(r)
    return uniq


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
    listing = "\n  - ".join(str(r) for r in roots)
    raise ValueError(
        f"路径超出允许范围：{path_str}\n"
        f"已尝试的允许根目录：\n  - {listing}\n"
        f"提示：设置 OPENYIDA_ALLOWED_ROOTS（允许多根，冒号/逗号分隔）或 OPENCLAW_WORKSPACE 扩展允许范围。"
    )


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
    except json.JSONDecodeError as e:
        print(f"  [FAIL] 非 JSON: {e}\n  输出: {result.stdout[:300]}", file=sys.stderr)
        return None


def _unwrap_content(data: Any) -> Any:
    """兼容 dws JSON 输出的常见包裹层。"""
    current = data
    for _ in range(4):
        if not isinstance(current, dict):
            return current
        if "content" in current:
            current = current["content"]
            continue
        if "data" in current and isinstance(current["data"], dict):
            current = current["data"]
            continue
        return current
    return current


def _extract_form_type(info: Any) -> str:
    """从 get-info 的不同返回形态中提取 formType/type。"""
    candidates: list[Any] = []
    current = info
    for _ in range(4):
        if not isinstance(current, dict):
            break
        candidates.append(current)
        next_obj = None
        for key in ("content", "data", "result"):
            value = current.get(key)
            if isinstance(value, dict):
                next_obj = value
                break
        if next_obj is None:
            break
        current = next_obj

    for item in candidates:
        value = item.get("formType") or item.get("type") or item.get("pageType")
        if isinstance(value, str) and value.strip():
            return value.strip().lower()
    return ""


def _check_display_target(app: str, form: str, force: bool = False) -> bool:
    """发布前确认目标是自定义展示页，避免覆盖普通表单/流程表单。"""
    print("Step 0: 校验发布目标")
    info = _run_dws(["yida", "design", "form", "get-info", "--app", app,
                     "--form", form, "--format", "json"])
    form_type = _extract_form_type(info)
    if form_type == "display":
        print("  [OK] 目标类型 display")
        return True

    if force:
        reason = form_type or "unknown"
        print(f"  [WARN] 目标类型为 {reason}，已按 --force 跳过保护")
        return True

    if not info:
        print("  [FAIL] 无法获取目标页面类型，已拒绝写入", file=sys.stderr)
    elif form_type:
        print(f"  [FAIL] 目标 formType={form_type}，不是 display 自定义页面，已拒绝写入",
              file=sys.stderr)
    else:
        print("  [FAIL] get-info 返回中未找到 formType，已拒绝写入", file=sys.stderr)
    print("  [HINT] 请确认 --form 是 display 页面；确认无误时可加 --force 显式绕过",
          file=sys.stderr)
    return False


def _load_code(args: argparse.Namespace) -> str:
    if args.code_file:
        safe = _resolve_safe_path(args.code_file)
        if not safe.exists():
            raise ValueError(f"文件不存在: {safe}")
        if safe.stat().st_size > MAX_CODE_FILE_SIZE:
            raise ValueError(f"文件过大 (限制 {MAX_CODE_FILE_SIZE:,} 字节)")
        return safe.read_text(encoding="utf-8")
    elif args.code:
        if len(args.code.encode("utf-8")) > MAX_INLINE_CODE:
            raise ValueError(f"--code 过长 (限制 {MAX_INLINE_CODE:,} 字节)")
        return args.code
    else:
        raise ValueError("必须提供 --code-file 或 --code")


def _extract_existing_data_source(schema: dict) -> dict | None:
    """从已有 schema 中提取 Page 组件的 dataSource，用于 merge 保留用户自定义数据源。"""
    try:
        return schema["pages"][0]["componentsTree"][0].get("dataSource")
    except (KeyError, IndexError, TypeError):
        return None


def main() -> int:
    ap = argparse.ArgumentParser(
        description=(
            "宜搭自定义页面 schema 生成/修改 "
            "【调用前必读】references/yida-custom-page-codegen.md"
            "（JSX 写法 / Hooks 限制 / 跨表单联动 / 常见坑），不要只看 --help 就拼 JSX"
        ),
        formatter_class=argparse.RawDescriptionHelpFormatter, epilog=__doc__)
    ap.add_argument("--app", required=True, help="应用编码 appType")
    ap.add_argument("--form", required=True, help="页面 formUuid")
    ap.add_argument("--code-file", help="新代码文件路径")
    ap.add_argument("--code", help="新代码内联字符串")
    ap.add_argument("--show-current", action="store_true", help="只输出当前代码不修改")
    ap.add_argument("--yes", action="store_true", help="确认写入")
    ap.add_argument("--dry-run", action="store_true", help="只编译不写入")
    ap.add_argument("--skip-field-check", action="store_true",
                    help="跳过字段 ID 对账预检（不推荐）")
    ap.add_argument("--skip-lint", action="store_true",
                    help="跳过 JSX 静态检查（30 条宜搭专属陷阱，不推荐）")
    ap.add_argument("--force", action="store_true",
                    help="跳过发布目标 formType=display 保护（仅确认目标无误时使用）")
    args = ap.parse_args()

    if not args.show_current and not args.code_file and not args.code:
        print("错误: 必须提供 --code-file / --code 或 --show-current", file=sys.stderr)
        return 1

    if not args.show_current and not args.dry_run:
        if not _check_display_target(args.app, args.form, force=args.force):
            return 1

    # Step 1: 拉取现有 schema
    print("Step 1: 获取现有 schema")
    resp = _run_dws(["yida", "design", "form", "get-schema", "--app", args.app,
                     "--form", args.form, "--format", "json"], dry_run=args.dry_run)
    if args.dry_run and not args.show_current:
        try:
            new_code = _load_code(args)
        except ValueError as e:
            print(f"错误: {e}", file=sys.stderr)
            return 1
        result = compile_jsx_to_schema(new_code, form_uuid=args.form)
        if not result.get("ok"):
            errors = result.get("errors", [])
            err_msgs = "; ".join(e.get("message", "") for e in errors)
            print(f"  [FAIL] 编译失败: {err_msgs}", file=sys.stderr)
            lint = result.get("lint", {})
            if lint.get("warnings"):
                for w in lint["warnings"]:
                    print(f"  [WARN] {w.get('message', w)}", file=sys.stderr)
            return 1
        schema_json = result["schema"]
        print(json.dumps({"ok": True, "dry_run": True, "formUuid": args.form,
                          "codeSize": len(new_code),
                          "schemaSize": len(schema_json)}, ensure_ascii=False, indent=2))
        return 0

    if not resp:
        return 1
    schema = resp
    print("  [OK] 拿到 schema")

    # --show-current 模式
    if args.show_current:
        try:
            current_code = extract_source_code(schema)
        except (ValueError, TypeError):
            current_code = None
        if current_code is None:
            print("  [WARN] schema 中没有可提取的自定义页面代码")
            print(json.dumps({"ok": False, "error": "not_a_custom_page"}, ensure_ascii=False))
            return 1
        print(json.dumps({"ok": True, "formUuid": args.form,
                          "codeSize": len(current_code),
                          "currentCode": current_code}, ensure_ascii=False, indent=2))
        return 0

    # 获取 page_id
    page_id = args.form
    pages = schema.get("pages", [])
    if pages:
        page_id = pages[0].get("id", args.form) or args.form

    # 提取已有 dataSource（用于 merge）
    existing_ds = _extract_existing_data_source(schema)

    # Step 2: 加载新代码、编译并构建 schema
    try:
        new_code = _load_code(args)
    except ValueError as e:
        print(f"错误: {e}", file=sys.stderr)
        return 1

    if not new_code.strip():
        print("错误: 代码不能为空", file=sys.stderr)
        return 1

    try:
        current_code = extract_source_code(schema)
    except (ValueError, TypeError):
        current_code = None
    previous_size = len(current_code) if current_code else 0

    print(f"Step 2: 编译 + 构建 schema (新代码 {len(new_code):,} 字节)")

    result = compile_jsx_to_schema(new_code, form_uuid=page_id, existing_data_source=existing_ds)
    if not result.get("ok"):
        errors = result.get("errors", [])
        err_msgs = "; ".join(e.get("message", "") for e in errors)
        print(f"  [FAIL] 编译失败: {err_msgs}", file=sys.stderr)
        lint = result.get("lint", {})
        if lint.get("warnings"):
            for w in lint["warnings"]:
                print(f"  [WARN] {w.get('message', w)}", file=sys.stderr)
        return 1

    schema_json = result["schema"]
    lint = result.get("lint", {})
    if lint.get("warnings"):
        for w in lint["warnings"]:
            print(f"  [WARN] lint: {w.get('message', w)}")

    print(f"  [OK] 编译成功, schema 大小: {len(schema_json):,} 字节")

    # Step 2.5: 字段 ID 对账预检（避免发布后运行时才报 fieldId 不存在）
    if not args.skip_field_check:
        print("Step 2.5: 字段 ID 对账")
        chk = field_check(new_code, args.app)
        for w in chk.get("warnings", []):
            print(f"  [WARN] {w.get('message', w)}")
        if not chk.get("ok"):
            print("  [FAIL] 字段对账未通过，为避免发布后页面报错，拒绝写入：", file=sys.stderr)
            for e in chk.get("errors", []):
                print(f"    - {e.get('message', e)}", file=sys.stderr)
            print("  [HINT] 修复后重试；确认需要忽略可加 --skip-field-check（不推荐）", file=sys.stderr)
            return 1
        info = chk.get("info", {})
        if info.get("skipped"):
            print(f"  [OK] 跳过（{info['skipped']}）")
        else:
            print(f"  [OK] 已校验 {info.get('referencedFieldCount', 0)} 个字段引用，"
                  f"覆盖 {len(info.get('checkedForms', []))} 张表单")

    # Step 2.7: JSX 静态检查（避免发布后运行时才报错）
    if not args.skip_lint:
        print("Step 2.7: JSX 静态检查")
        lr = lint_check(new_code, filename=args.code_file or "page.jsx")
        for w in lr.get("warnings", []):
            print(f"  [WARN] L{w['line']} [{w['rule']}] {w['message']}")
        if not lr.get("ok"):
            print("  [FAIL] JSX 静态检查未通过，为避免发布后页面报错，拒绝写入：", file=sys.stderr)
            for e in lr.get("errors", []):
                print(f"    L{e['line']} [{e['rule']}] {e['message']}", file=sys.stderr)
            print("  [HINT] 修复后重试；确认需要忽略可加 --skip-lint（不推荐）", file=sys.stderr)
            print("  [HINT] 或在 JSX 中加 // dws-lint-disable-line [rule] 关闭单行检查", file=sys.stderr)
            return 1
        info = lr.get("info", {})
        print(f"  [OK] 检查通过（错误 {info.get('errorCount', 0)} / 警告 {info.get('warningCount', 0)}）")

    # Step 3: 写回
    print("Step 3: 写入 schema")
    resp = _run_dws(["yida", "design", "form", "update-schema", "--app", args.app,
                     "--form", args.form, "--form-type", "display",
                     "--content", schema_json, "--yes", "--format", "json"])
    if not resp:
        return 1
    print("  [OK] 写入成功")
    print(json.dumps({"ok": True, "formUuid": args.form, "codeSize": len(new_code),
                      "previousCodeSize": previous_size}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main())
