#!/usr/bin/env python3
"""
通过 MCP 导出任务（export_data）导出 AI 表格，并可自动下载文件。

与普通命令的区别：
- 自动处理 taskId 轮询（直到拿到 downloadUrl 或达到轮询上限）。
- 自动保存导出文件到本地（可选 --output）。

用法:
    python scripts/aitable_export_via_task.py <baseId> --scope all
    python scripts/aitable_export_via_task.py <baseId> --scope table --table-id <tableId>
    python scripts/aitable_export_via_task.py <baseId> --scope view --table-id <tableId> --view-id <viewId>
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
import time
from pathlib import Path
from typing import Any, Dict, Optional, Tuple
from urllib.error import HTTPError, URLError
from urllib.parse import urlparse
from urllib.request import Request, urlopen

RESOURCE_ID_PATTERN = re.compile(r"^[A-Za-z0-9_-]{8,128}$")
ALLOWED_FORMATS = {"excel", "attachment", "excel_and_attachment", "excel_with_inline_images"}


def validate_resource_id(resource_id: str) -> bool:
    return bool(resource_id and RESOURCE_ID_PATTERN.match(resource_id.strip()))


def run_dws(dws_bin: str, args: list[str], timeout_sec: int = 120) -> Tuple[int, str, str]:
    cmd = [dws_bin] + args
    try:
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=timeout_sec)
        return result.returncode, result.stdout.strip(), result.stderr.strip()
    except subprocess.TimeoutExpired:
        return 124, "", f"dws command timeout after {timeout_sec}s"
    except FileNotFoundError:
        return 127, "", f"dws binary not found: {dws_bin}"


def parse_json_output(raw: str) -> Optional[Dict[str, Any]]:
    try:
        obj = json.loads(raw)
        return obj if isinstance(obj, dict) else None
    except json.JSONDecodeError:
        return None


def normalize_download_url(url: str) -> str:
    if url.startswith("http://") or url.startswith("https://"):
        return url
    return f"https://{url}"


def download_file(url: str, output_path: Path) -> Tuple[bool, str]:
    req = Request(url, method="GET")
    try:
        with urlopen(req, timeout=180) as resp:
            if resp.status != 200:
                return False, f"download http status: {resp.status}"
            output_path.write_bytes(resp.read())
            return True, ""
    except HTTPError as e:
        body = e.read().decode("utf-8", "ignore")
        return False, f"HTTP {e.code}: {body[:300]}"
    except URLError as e:
        return False, f"URL error: {e.reason}"


def fail(msg: str, code: int = 1) -> None:
    print(f"错误：{msg}", file=sys.stderr)
    sys.exit(code)


def build_start_args(args: argparse.Namespace) -> list[str]:
    cmd = [
        "aitable",
        "export",
        "data",
        "--base-id",
        args.base_id,
        "--scope",
        args.scope,
        "--format",
        args.export_format,
        "--timeout-ms",
        str(args.timeout_ms),
    ]
    if args.table_id:
        cmd.extend(["--table-id", args.table_id])
    if args.view_id:
        cmd.extend(["--view-id", args.view_id])
    return cmd


def main() -> None:
    parser = argparse.ArgumentParser(description="通过 MCP 导出任务导出 AI 表格")
    parser.add_argument("base_id", help="目标 AI 表格 baseId")
    parser.add_argument("--scope", choices=["all", "table", "view"], required=True, help="导出范围")
    parser.add_argument("--table-id", help="scope=table/view 时必填")
    parser.add_argument("--view-id", help="scope=view 时必填")
    parser.add_argument("--export-format", default="excel", choices=sorted(ALLOWED_FORMATS), help="导出格式")
    parser.add_argument("--timeout-ms", type=int, default=1000, help="单次等待毫秒数，默认 1000")
    parser.add_argument("--poll-timeout-ms", type=int, default=3000, help="轮询等待毫秒数，默认 3000")
    parser.add_argument("--max-polls", type=int, default=10, help="最大轮询次数，默认 10")
    parser.add_argument("--output", help="本地保存路径（不传则按 fileName 保存到当前目录）")
    parser.add_argument("--dws", default="dws", help="dws 可执行文件路径，默认 dws")
    parser.add_argument("--no-download", action="store_true", help="仅返回 downloadUrl，不下载文件")
    args = parser.parse_args()

    if not validate_resource_id(args.base_id):
        fail("无效的 baseId 格式")
    if args.scope in ("table", "view") and not args.table_id:
        fail("scope=table/view 时必须传 --table-id")
    if args.scope == "view" and not args.view_id:
        fail("scope=view 时必须传 --view-id")

    print("[1/2] start export task", file=sys.stderr)
    rc, out, err = run_dws(args.dws, build_start_args(args), timeout_sec=120)
    if rc != 0:
        fail(f"export_data 启动失败: {err or out}", rc)
    obj = parse_json_output(out)
    if not obj:
        fail(f"export_data 返回非 JSON: {out[:300]}")

    data = obj.get("data", {}) or {}
    status = obj.get("status")
    if status == "error":
        fail(f"export_data 返回失败: {json.dumps(obj, ensure_ascii=False)}")

    download_url = data.get("downloadUrl")
    task_id = data.get("taskId")
    file_name = data.get("fileName") or "export_result.bin"

    polls = 0
    while not download_url and task_id and polls < args.max_polls:
        polls += 1
        print(f"[2/2] polling task ({polls}/{args.max_polls})", file=sys.stderr)
        rc2, out2, err2 = run_dws(
            args.dws,
            [
                "aitable",
                "export",
                "data",
                "--base-id",
                args.base_id,
                "--task-id",
                task_id,
                "--timeout-ms",
                str(args.poll_timeout_ms),
            ],
            timeout_sec=max(120, int(args.poll_timeout_ms / 1000) + 60),
        )
        if rc2 != 0:
            fail(f"export_data 轮询失败: {err2 or out2}", rc2)
        obj2 = parse_json_output(out2)
        if not obj2:
            fail(f"export_data 轮询返回非 JSON: {out2[:300]}")
        if obj2.get("status") == "error":
            fail(f"export_data 轮询返回失败: {json.dumps(obj2, ensure_ascii=False)}")
        d2 = obj2.get("data", {}) or {}
        download_url = d2.get("downloadUrl") or download_url
        file_name = d2.get("fileName") or file_name
        task_id = d2.get("taskId") or task_id
        if not download_url:
            time.sleep(0.2)

    result: Dict[str, Any] = {
        "baseId": args.base_id,
        "scope": args.scope,
        "exportFormat": args.export_format,
        "taskId": task_id,
        "fileName": file_name,
        "downloadUrl": download_url,
        "polledTimes": polls,
    }

    if not download_url:
        result["status"] = "pending"
        result["summary"] = "导出任务仍在处理中，请继续用 taskId 轮询。"
        print(json.dumps(result, ensure_ascii=False, indent=2))
        sys.exit(3)

    if args.no_download:
        result["status"] = "success"
        result["summary"] = "导出完成（未下载文件）。"
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return

    norm_url = normalize_download_url(download_url)
    output_path = Path(args.output).expanduser().resolve() if args.output else Path.cwd() / file_name
    ok, dl_err = download_file(norm_url, output_path)
    if not ok:
        fail(f"downloadUrl 下载失败: {dl_err}")

    result["status"] = "success"
    result["summary"] = "导出完成并已下载。"
    result["savedPath"] = str(output_path)
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
