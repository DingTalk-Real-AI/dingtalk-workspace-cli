#!/usr/bin/env python3
"""批量追加 CSV / JSON 记录到已有钉钉 AI 表格数据表。

用法:
    python3 import_records.py <baseId> <tableId> data.csv [batch_size]
    python3 import_records.py <baseId> <tableId> data.json [batch_size]

CSV 表头必须是 fieldId。CSV 值保持字符串；需要布尔、数组、对象等精确类型时使用
JSON。JSON 支持 [{"cells": {...}}] 或 [{"fld...": value}] 两种格式。
脚本逐批检查业务状态、提取 newRecordIds 并回读验证；部分成功会保留 ledger，
但整体以非零状态结束。
"""

from __future__ import annotations

import argparse
import csv
import json
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple, Union

JsonData = Union[List[Any], Dict[str, Any]]
MAX_FILE_SIZE = 50 * 1024 * 1024
RESOURCE_ID_PATTERN = re.compile(r"^[A-Za-z0-9_-]{8,128}$")
MAX_RECORDS_PER_BATCH = 100
DEFAULT_BATCH_SIZE = 50


def resolve_safe_path(path: str, allowed_root: Optional[str] = None) -> Path:
    root = Path(allowed_root or os.environ.get("OPENCLAW_WORKSPACE", os.getcwd())).resolve()
    candidate = Path(path)
    target = candidate.resolve() if candidate.is_absolute() else (Path.cwd() / candidate).resolve()
    try:
        target.relative_to(root)
    except ValueError as exc:
        raise ValueError(f"路径超出允许范围：{path}（允许根目录：{root}）") from exc
    return target


def validate_resource_id(resource_id: str) -> bool:
    return bool(resource_id and RESOURCE_ID_PATTERN.fullmatch(resource_id.strip()))


def safe_csv_load(file_path: Path) -> List[Dict[str, str]]:
    if file_path.stat().st_size > MAX_FILE_SIZE:
        raise ValueError(f"文件过大（限制 {MAX_FILE_SIZE:,} 字节）")
    with file_path.open("r", encoding="utf-8-sig", newline="") as stream:
        reader = csv.DictReader(stream)
        if not reader.fieldnames or any(not str(name).strip() for name in reader.fieldnames):
            raise ValueError("CSV 必须包含非空 fieldId 表头")
        return list(reader)


def safe_json_load(file_path: Path) -> JsonData:
    if file_path.stat().st_size > MAX_FILE_SIZE:
        raise ValueError(f"文件过大（限制 {MAX_FILE_SIZE:,} 字节）")
    with file_path.open("r", encoding="utf-8") as stream:
        return json.load(stream)


def normalize_record(record: Dict[str, Any]) -> Dict[str, Any]:
    cells = record.get("cells") if isinstance(record.get("cells"), dict) else record
    normalized: Dict[str, Any] = {}
    for key, value in cells.items():
        if not isinstance(key, str) or not key.strip():
            continue
        if value is None:
            continue
        if isinstance(value, str):
            value = value.strip()
            if not value:
                continue
        normalized[key.strip()] = value
    return {"cells": normalized}


def validate_record(record: Any) -> Tuple[bool, str]:
    if not isinstance(record, dict):
        return False, "记录必须是对象"
    cells = normalize_record(record).get("cells")
    if not isinstance(cells, dict) or not cells:
        return False, "记录必须包含非空 cells 对象"
    return True, ""


def run_dws(dws_bin: str, args: List[str], timeout_sec: int = 120) -> Tuple[Optional[Dict[str, Any]], str]:
    try:
        result = subprocess.run(
            [dws_bin] + args,
            capture_output=True,
            text=True,
            timeout=timeout_sec,
        )
    except subprocess.TimeoutExpired:
        return None, f"dws 命令超时（{timeout_sec} 秒）"
    except FileNotFoundError:
        return None, f"未找到 dws 命令：{dws_bin}"
    if result.returncode != 0:
        return None, (result.stderr or result.stdout).strip() or f"dws 退出码 {result.returncode}"
    try:
        payload = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        return None, f"dws 返回非 JSON：{exc}"
    if not isinstance(payload, dict):
        return None, "dws 返回的 JSON 不是对象"
    if payload.get("status") != "success":
        detail = payload.get("summary") or payload.get("error") or payload
        return None, f"业务失败：{detail}"
    return payload, ""


def extract_query_record_ids(payload: Dict[str, Any]) -> List[str]:
    data = payload.get("data") if isinstance(payload.get("data"), dict) else {}
    records = data.get("records") or data.get("items") or []
    if not isinstance(records, list):
        return []
    return [
        str(item.get("recordId"))
        for item in records
        if isinstance(item, dict) and item.get("recordId")
    ]


def import_records(
    base_id: str,
    table_id: str,
    records: List[Dict[str, Any]],
    batch_size: int,
    dws_bin: str = "dws",
) -> Dict[str, Any]:
    if batch_size <= 0 or batch_size > MAX_RECORDS_PER_BATCH:
        raise ValueError(f"batch_size 必须在 1..{MAX_RECORDS_PER_BATCH} 之间")

    ledger: List[Dict[str, Any]] = []
    verified_ids: List[str] = []
    total_batches = (len(records) + batch_size - 1) // batch_size
    for start in range(0, len(records), batch_size):
        batch = records[start : start + batch_size]
        batch_number = start // batch_size + 1
        print(f"[{batch_number}/{total_batches}] 创建 {len(batch)} 条记录", file=sys.stderr)
        created, error = run_dws(
            dws_bin,
            [
                "aitable", "record", "create",
                "--base-id", base_id,
                "--table-id", table_id,
                "--records", json.dumps(batch, ensure_ascii=False),
                "--format", "json",
            ],
        )
        entry: Dict[str, Any] = {
            "batch": batch_number,
            "inputCount": len(batch),
            "status": "failed",
            "recordIds": [],
        }
        if created is None:
            entry["error"] = error
            ledger.append(entry)
            continue

        data = created.get("data") if isinstance(created.get("data"), dict) else {}
        record_ids = data.get("newRecordIds")
        if not isinstance(record_ids, list) or len(record_ids) != len(batch) or not all(record_ids):
            entry["error"] = "创建响应缺少与输入数量一致的 data.newRecordIds[]"
            ledger.append(entry)
            continue
        record_ids = [str(value) for value in record_ids]
        entry["recordIds"] = record_ids

        queried, query_error = run_dws(
            dws_bin,
            [
                "aitable", "record", "query",
                "--base-id", base_id,
                "--table-id", table_id,
                "--record-ids", ",".join(record_ids),
                "--format", "json",
            ],
        )
        if queried is None:
            entry["status"] = "verify_failed"
            entry["error"] = query_error
            ledger.append(entry)
            continue
        found = set(extract_query_record_ids(queried))
        missing = [record_id for record_id in record_ids if record_id not in found]
        if missing:
            entry["status"] = "verify_failed"
            entry["missingRecordIds"] = missing
            entry["error"] = "回读未返回全部新记录"
            ledger.append(entry)
            continue
        entry["status"] = "success"
        verified_ids.extend(record_ids)
        ledger.append(entry)

    complete = all(item["status"] == "success" for item in ledger)
    return {
        "status": "success" if complete else "partial",
        "complete": complete,
        "requestedCount": len(records),
        "verifiedCount": len(verified_ids),
        "recordIds": verified_ids,
        "ledger": ledger,
    }


def load_records(input_file: str) -> List[Dict[str, Any]]:
    path = resolve_safe_path(input_file)
    if not path.exists() or not path.is_file():
        raise ValueError(f"文件不存在或不可读：{path}")
    suffix = path.suffix.lower()
    if suffix == ".csv":
        raw: Any = safe_csv_load(path)
    elif suffix == ".json":
        raw = safe_json_load(path)
    else:
        raise ValueError("仅支持 .csv 或 .json 文件")
    if not isinstance(raw, list) or not raw:
        raise ValueError("输入文件必须包含至少一条记录")
    normalized: List[Dict[str, Any]] = []
    for index, record in enumerate(raw, start=1):
        valid, error = validate_record(record)
        if not valid:
            raise ValueError(f"记录 #{index} 格式无效：{error}")
        normalized.append(normalize_record(record))
    return normalized


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("base_id")
    parser.add_argument("table_id")
    parser.add_argument("input_file")
    parser.add_argument("batch_size", nargs="?", type=int, default=DEFAULT_BATCH_SIZE)
    parser.add_argument("--dws", default="dws", help="dws 可执行文件路径")
    args = parser.parse_args()
    if not validate_resource_id(args.base_id):
        parser.error("无效的 baseId 格式")
    if not validate_resource_id(args.table_id):
        parser.error("无效的 tableId 格式")
    try:
        records = load_records(args.input_file)
        result = import_records(
            args.base_id,
            args.table_id,
            records,
            args.batch_size,
            args.dws,
        )
    except (ValueError, OSError, csv.Error, json.JSONDecodeError) as exc:
        print(f"错误：{exc}", file=sys.stderr)
        sys.exit(1)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    sys.exit(0 if result["complete"] else 2)


if __name__ == "__main__":
    main()
