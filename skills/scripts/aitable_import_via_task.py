#!/usr/bin/env python3
"""
通过 MCP 文件导入任务（prepare_import_upload -> PUT -> import_data）导入 AI 表格。

与 import_records.py 的区别：
- 本脚本：走“文件导入任务”链路，通常会新建导入数据表。
- import_records.py：走 create_records，写入已有 table。

用法:
    python scripts/aitable_import_via_task.py <baseId> <filePath>
    python scripts/aitable_import_via_task.py <baseId> <filePath> --timeout 30
    python scripts/aitable_import_via_task.py <baseId> <filePath> --dws /tmp/dws
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path
from typing import Any, Dict, Optional, Tuple
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

RESOURCE_ID_PATTERN = re.compile(r"^[A-Za-z0-9_-]{8,128}$")
ALLOWED_EXTENSIONS = {".csv", ".xlsx", ".xls"}


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


def put_file(upload_url: str, file_path: Path) -> Tuple[bool, str]:
    payload = file_path.read_bytes()
    req = Request(upload_url, data=payload, method="PUT")
    # 关键：清空 Content-Type，避免 SignatureDoesNotMatch。
    req.add_header("Content-Type", "")
    try:
        with urlopen(req, timeout=180) as resp:
            if resp.status == 200:
                return True, ""
            return False, f"unexpected HTTP status: {resp.status}"
    except HTTPError as e:
        body = e.read().decode("utf-8", "ignore")
        return False, f"HTTP {e.code}: {body[:300]}"
    except URLError as e:
        return False, f"URL error: {e.reason}"


def fail(msg: str, exit_code: int = 1) -> None:
    print(f"错误：{msg}", file=sys.stderr)
    sys.exit(exit_code)


def main() -> None:
    parser = argparse.ArgumentParser(description="通过文件导入任务导入 AI 表格")
    parser.add_argument("base_id", help="目标 AI 表格 baseId")
    parser.add_argument("file_path", help="待导入文件路径（.csv/.xlsx/.xls）")
    parser.add_argument("--timeout", type=int, default=30, help="import_data 等待秒数，默认 30")
    parser.add_argument("--dws", default="dws", help="dws 可执行文件路径，默认 dws")
    args = parser.parse_args()

    base_id = args.base_id.strip()
    file_path = Path(args.file_path).expanduser().resolve()

    if not validate_resource_id(base_id):
        fail("无效的 baseId 格式")
    if not file_path.exists() or not file_path.is_file():
        fail(f"文件不存在或不可读: {file_path}")
    if file_path.suffix.lower() not in ALLOWED_EXTENSIONS:
        fail(f"仅支持 {sorted(ALLOWED_EXTENSIONS)}，当前文件: {file_path.name}")

    file_size = file_path.stat().st_size
    if file_size <= 0:
        fail("文件为空")

    print(f"[1/3] prepare import upload: {file_path.name} ({file_size} bytes)", file=sys.stderr)
    rc, out, err = run_dws(
        args.dws,
        [
            "aitable",
            "import",
            "upload",
            "--base-id",
            base_id,
            "--file-name",
            file_path.name,
            "--file-size",
            str(file_size),
            "--format",
            "json",
        ],
    )
    if rc != 0:
        fail(f"prepare_import_upload 失败: {err or out}", rc)
    prepare_obj = parse_json_output(out)
    if not prepare_obj:
        fail(f"prepare_import_upload 返回非 JSON: {out[:300]}")
    if prepare_obj.get("status") != "success":
        fail(f"prepare_import_upload 返回失败: {json.dumps(prepare_obj, ensure_ascii=False)}")

    pdata = prepare_obj.get("data") or {}
    upload_url = pdata.get("uploadUrl")
    import_id = pdata.get("importId")
    if not upload_url or not import_id:
        fail(f"prepare_import_upload 缺少 uploadUrl/importId: {json.dumps(pdata, ensure_ascii=False)}")

    print("[2/3] upload file bytes via PUT", file=sys.stderr)
    ok, put_err = put_file(upload_url, file_path)
    if not ok:
        fail(f"PUT 上传失败: {put_err}")

    print("[3/3] trigger import_data", file=sys.stderr)
    rc2, out2, err2 = run_dws(
        args.dws,
        [
            "aitable",
            "import",
            "data",
            "--import-id",
            import_id,
            "--timeout",
            str(args.timeout),
            "--format",
            "json",
        ],
        timeout_sec=max(120, args.timeout + 30),
    )
    if rc2 != 0:
        fail(f"import_data 调用失败: {err2 or out2}", rc2)
    import_obj = parse_json_output(out2)
    if not import_obj:
        fail(f"import_data 返回非 JSON: {out2[:300]}")

    result = {
        "baseId": base_id,
        "fileName": file_path.name,
        "fileSize": file_size,
        "importId": import_id,
        "status": import_obj.get("status"),
        "summary": import_obj.get("summary"),
        "data": import_obj.get("data", {}),
        "error": import_obj.get("error", {}),
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))

    if import_obj.get("status") != "success":
        sys.exit(2)


if __name__ == "__main__":
    main()
