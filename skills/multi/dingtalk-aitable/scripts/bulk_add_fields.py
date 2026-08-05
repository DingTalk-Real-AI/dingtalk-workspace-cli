#!/usr/bin/env python3
"""批量添加字段到钉钉 AI 表格数据表。

用法:
    python3 bulk_add_fields.py <baseId> <tableId> fields.json

脚本检查业务状态和逐项结果，并回读成功字段。部分成功会输出 ledger，但整体以非零
状态结束。fields.json 单次最多 15 个字段；name 会映射为 fieldName，phone 会映射为
telephone。
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple, Union

JsonData = Union[List[Any], Dict[str, Any]]
MAX_FILE_SIZE = 10 * 1024 * 1024
RESOURCE_ID_PATTERN = re.compile(r"^[A-Za-z0-9_-]{8,128}$")
ALLOWED_FIELD_TYPES = {
    "text", "number", "singleSelect", "multipleSelect", "date", "currency",
    "user", "department", "group", "progress", "rating", "checkbox",
    "attachment", "url", "richText", "telephone", "email", "idCard",
    "barcode", "geolocation", "address", "primaryDoc", "formula",
    "unidirectionalLink", "bidirectionalLink", "lookup", "filterUp",
    "creator", "lastModifier", "createdTime", "lastModifiedTime",
}


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


def safe_json_load(file_path: Path) -> JsonData:
    if file_path.stat().st_size > MAX_FILE_SIZE:
        raise ValueError(f"文件过大（限制 {MAX_FILE_SIZE:,} 字节）")
    with file_path.open("r", encoding="utf-8") as stream:
        return json.load(stream)


def normalize_field_config(field: Dict[str, Any]) -> Dict[str, Any]:
    normalized = dict(field)
    if "fieldName" not in normalized and "name" in normalized:
        normalized["fieldName"] = normalized.pop("name")
    if normalized.get("type") == "phone":
        normalized["type"] = "telephone"
    return normalized


def validate_field_config(field: Any) -> Tuple[bool, str]:
    if not isinstance(field, dict):
        return False, "字段配置必须是对象"
    normalized = normalize_field_config(field)
    name = normalized.get("fieldName")
    if not isinstance(name, str) or not name.strip():
        return False, "fieldName 必须是非空字符串"
    field_type = normalized.get("type", "text")
    if field_type not in ALLOWED_FIELD_TYPES:
        return False, f"不支持的字段类型：{field_type}"
    config = normalized.get("config")
    if config is not None and not isinstance(config, dict):
        return False, "config 必须是对象"
    ai_config = normalized.get("aiConfig")
    if ai_config is not None and not isinstance(ai_config, dict):
        return False, "aiConfig 必须是对象"
    if field_type in {"singleSelect", "multipleSelect"}:
        options = (config or {}).get("options")
        if not isinstance(options, list) or not options:
            return False, "singleSelect / multipleSelect 必须提供 config.options 数组"
    if field_type in {"unidirectionalLink", "bidirectionalLink"}:
        linked_table_id = (config or {}).get("linkedTableId")
        if not validate_resource_id(str(linked_table_id or "")):
            return False, "关联字段必须提供合法的 config.linkedTableId"
    if field_type == "lookup":
        required = ("associateField", "valuesField", "aggregator")
        missing = [name for name in required if not (config or {}).get(name)]
        if missing:
            return False, f"lookup 缺少 config.{missing[0]}"
    if field_type == "filterUp":
        required = ("targetSheet", "filters", "valuesField", "aggregator")
        missing = [name for name in required if not (config or {}).get(name)]
        if missing:
            return False, f"filterUp 缺少 config.{missing[0]}"
        if not isinstance((config or {}).get("filters"), list):
            return False, "filterUp config.filters 必须是数组"
    return True, ""


def build_fields_payload(fields: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    payload: List[Dict[str, Any]] = []
    for field in fields:
        normalized = normalize_field_config(field)
        item: Dict[str, Any] = {
            "fieldName": normalized["fieldName"].strip(),
            "type": normalized.get("type", "text"),
        }
        for key in ("config", "aiConfig"):
            if normalized.get(key) is not None:
                item[key] = normalized[key]
        payload.append(item)
    return payload


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


def normalize_result_item(index: int, item: Any) -> Dict[str, Any]:
    if not isinstance(item, dict):
        return {"index": index, "status": "failed", "error": "逐项结果不是对象"}
    field_id = item.get("fieldId") or (item.get("data") or {}).get("fieldId")
    succeeded = item.get("success") is True or item.get("status") == "success"
    if "success" not in item and "status" not in item:
        succeeded = bool(field_id)
    result: Dict[str, Any] = {
        "index": index,
        "status": "success" if succeeded and field_id else "failed",
    }
    if field_id:
        result["fieldId"] = str(field_id)
    if result["status"] != "success":
        result["error"] = item.get("reason") or item.get("error") or "字段创建未返回 fieldId"
    return result


def bulk_add_fields(
    base_id: str,
    table_id: str,
    fields: List[Dict[str, Any]],
    dws_bin: str = "dws",
) -> Dict[str, Any]:
    created, error = run_dws(
        dws_bin,
        [
            "aitable", "field", "create",
            "--base-id", base_id,
            "--table-id", table_id,
            "--fields", json.dumps(build_fields_payload(fields), ensure_ascii=False),
            "--format", "json",
        ],
    )
    if created is None:
        return {
            "status": "failed",
            "complete": False,
            "requestedCount": len(fields),
            "verifiedCount": 0,
            "ledger": [{"status": "failed", "error": error}],
        }

    data = created.get("data") if isinstance(created.get("data"), dict) else {}
    raw_results = data.get("results")
    if not isinstance(raw_results, list) or len(raw_results) != len(fields):
        return {
            "status": "failed",
            "complete": False,
            "requestedCount": len(fields),
            "verifiedCount": 0,
            "ledger": [{"status": "failed", "error": "响应缺少与输入数量一致的 data.results[]"}],
        }
    ledger = [normalize_result_item(index, item) for index, item in enumerate(raw_results)]
    field_ids = [item["fieldId"] for item in ledger if item["status"] == "success"]
    verified_ids: List[str] = []
    if field_ids:
        queried, query_error = run_dws(
            dws_bin,
            [
                "aitable", "field", "get",
                "--base-id", base_id,
                "--table-id", table_id,
                "--field-ids", ",".join(field_ids),
                "--format", "json",
            ],
        )
        if queried is None:
            for item in ledger:
                if item["status"] == "success":
                    item["status"] = "verify_failed"
                    item["error"] = query_error
        else:
            query_data = queried.get("data") if isinstance(queried.get("data"), dict) else {}
            raw_fields = query_data.get("fields") or query_data.get("items") or []
            found = {
                str(item.get("fieldId"))
                for item in raw_fields
                if isinstance(item, dict) and item.get("fieldId")
            }
            verified_ids = [field_id for field_id in field_ids if field_id in found]
            for item in ledger:
                if item.get("fieldId") in field_ids and item.get("fieldId") not in found:
                    item["status"] = "verify_failed"
                    item["error"] = "回读未返回该字段"
    complete = len(verified_ids) == len(fields) and all(item["status"] == "success" for item in ledger)
    return {
        "status": "success" if complete else "partial",
        "complete": complete,
        "requestedCount": len(fields),
        "verifiedCount": len(verified_ids),
        "fieldIds": verified_ids,
        "ledger": ledger,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("base_id")
    parser.add_argument("table_id")
    parser.add_argument("fields_file")
    parser.add_argument("--dws", default="dws", help="dws 可执行文件路径")
    args = parser.parse_args()
    if not validate_resource_id(args.base_id):
        parser.error("无效的 baseId 格式")
    if not validate_resource_id(args.table_id):
        parser.error("无效的 tableId 格式")
    try:
        path = resolve_safe_path(args.fields_file)
        if path.suffix.lower() != ".json" or not path.exists() or not path.is_file():
            raise ValueError("fields_file 必须是工作区内存在的 .json 文件")
        fields = safe_json_load(path)
        if not isinstance(fields, list) or not fields:
            raise ValueError("fields.json 必须是非空 JSON 数组")
        if len(fields) > 15:
            raise ValueError("单次最多创建 15 个字段，请拆分后重试")
        for index, field in enumerate(fields, start=1):
            valid, error = validate_field_config(field)
            if not valid:
                raise ValueError(f"字段 #{index} 配置无效：{error}")
        result = bulk_add_fields(args.base_id, args.table_id, fields, args.dws)
    except (ValueError, OSError, json.JSONDecodeError) as exc:
        print(f"错误：{exc}", file=sys.stderr)
        sys.exit(1)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    sys.exit(0 if result["complete"] else 2)


if __name__ == "__main__":
    main()
