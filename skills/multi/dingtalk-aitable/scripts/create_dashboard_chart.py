#!/usr/bin/env python3
"""Create an AI Table dashboard and optional common charts deterministically.

Examples:
  python3 create_dashboard_chart.py BASE_ID "状态分析仪表盘"
  python3 create_dashboard_chart.py BASE_ID "状态分析仪表盘" --chart-specs charts.json

charts.json is a JSON array. Each item accepts:
  name, chart_type, table_id, measure_type, measure_field_id,
  dimension_field_id, aggregation, view_id.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Any, Optional

LEDGER_SCHEMA_VERSION = "dws-skill-script-ledger/v1"
SCRIPT_NAME = "create_dashboard_chart.py"
RESOURCE_ID_PATTERN = re.compile(r"^[A-Za-z0-9_-]{3,128}$")
SUPPORTED_CHART_TYPES = {"AREA", "BAR", "HISTOGRAM", "LINE", "PIE", "STATISTICS"}
SUPPORTED_AGGREGATIONS = {"sum", "count", "count_distinct", "average", "avg", "min", "max"}
MAX_CHARTS = 6


def run_dws(dws_bin: str, args: list[str]) -> tuple[Optional[dict[str, Any]], str]:
    try:
        completed = subprocess.run(
            [dws_bin, *args], capture_output=True, text=True, timeout=120
        )
    except subprocess.TimeoutExpired:
        return None, "dws command timeout after 120 seconds"
    except FileNotFoundError:
        return None, f"dws binary not found: {dws_bin}"
    if completed.returncode != 0:
        return None, (completed.stderr or completed.stdout).strip()[:800]
    try:
        payload = json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        return None, f"dws returned non-JSON output: {exc}"
    if not isinstance(payload, dict) or payload.get("status") != "success":
        return None, f"dws business failure: {payload}"
    return payload, ""


def safe_json_file(value: str) -> Any:
    root = Path(os.environ.get("OPENCLAW_WORKSPACE", os.getcwd())).resolve()
    source = Path(value).expanduser()
    source = source.resolve() if source.is_absolute() else (Path.cwd() / source).resolve()
    try:
        source.relative_to(root)
    except ValueError as exc:
        raise ValueError(f"chart specs must be inside the workspace: {source}") from exc
    if not source.is_file() or source.stat().st_size > 1024 * 1024:
        raise ValueError("chart specs must be a readable JSON file no larger than 1 MiB")
    with source.open("r", encoding="utf-8") as stream:
        return json.load(stream)


def validate_specs(value: Any) -> list[dict[str, Any]]:
    if not isinstance(value, list) or not value or len(value) > MAX_CHARTS:
        raise ValueError(f"chart specs must contain 1-{MAX_CHARTS} items")
    specs: list[dict[str, Any]] = []
    for index, item in enumerate(value, start=1):
        if not isinstance(item, dict):
            raise ValueError(f"chart spec #{index} must be an object")
        name = str(item.get("name") or "").strip()
        chart_type = str(item.get("chart_type") or "").strip().upper()
        table_id = str(item.get("table_id") or "").strip()
        measure_type = str(item.get("measure_type") or "record-count").strip()
        measure_field_id = str(item.get("measure_field_id") or "").strip()
        dimension_field_id = str(item.get("dimension_field_id") or "").strip()
        aggregation = str(item.get("aggregation") or "sum").strip().lower()
        view_id = str(item.get("view_id") or "").strip()
        if not name or len(name) > 80:
            raise ValueError(f"chart spec #{index} needs a 1-80 character name")
        if chart_type not in SUPPORTED_CHART_TYPES:
            raise ValueError(f"chart spec #{index} has unsupported chart_type: {chart_type}")
        if not RESOURCE_ID_PATTERN.fullmatch(table_id):
            raise ValueError(f"chart spec #{index} has invalid table_id")
        if measure_type not in {"record-count", "field"}:
            raise ValueError(f"chart spec #{index} has invalid measure_type")
        if measure_type == "field" and not RESOURCE_ID_PATTERN.fullmatch(measure_field_id):
            raise ValueError(f"chart spec #{index} needs measure_field_id")
        if aggregation not in SUPPORTED_AGGREGATIONS:
            raise ValueError(f"chart spec #{index} has unsupported aggregation")
        for label, resource_id in (
            ("dimension_field_id", dimension_field_id), ("view_id", view_id)
        ):
            if resource_id and not RESOURCE_ID_PATTERN.fullmatch(resource_id):
                raise ValueError(f"chart spec #{index} has invalid {label}")
        specs.append(
            {
                "name": name,
                "chart_type": chart_type,
                "table_id": table_id,
                "measure_type": measure_type,
                "measure_field_id": measure_field_id,
                "dimension_field_id": dimension_field_id,
                "aggregation": "average" if aggregation == "avg" else aggregation,
                "view_id": view_id,
            }
        )
    return specs


def chart_config(spec: dict[str, Any]) -> dict[str, Any]:
    config: dict[str, Any] = {
        "chartType": spec["chart_type"],
        "name": spec["name"],
        "sheet": spec["table_id"],
        "view": spec["view_id"] or None,
        "measureType": spec["measure_type"],
        "measure": [],
        "filter": [],
    }
    if spec["measure_type"] == "field":
        config["measure"] = [
            {
                "value": spec["measure_field_id"],
                "externalValue": [{"type": "formula", "value": spec["aggregation"]}],
            }
        ]
    if spec["dimension_field_id"]:
        config["dimension"] = [
            {"value": spec["dimension_field_id"], "externalValue": []}
        ]
    if spec["chart_type"] in {"AREA", "BAR", "HISTOGRAM", "LINE", "PIE"}:
        config.update({"colors": "COLOR_PALETTE_1", "legend": "top", "label": True})
    if spec["chart_type"] in {"AREA", "BAR", "HISTOGRAM", "LINE"}:
        config.update({"xAxisShow": True, "yAxisShow": True})
    if spec["chart_type"] == "PIE":
        config.update({"innerRadius": 0, "outerRadius": 60})
    return config


def layout(index: int, total: int) -> dict[str, int]:
    width = 12 if total == 1 else 6 if total in {2, 3, 4} else 4
    per_row = 12 // width
    return {"x": (index % per_row) * width, "y": (index // per_row) * 5, "w": width, "h": 5}


def extract_id(payload: dict[str, Any], key: str) -> str:
    data = payload.get("data") if isinstance(payload.get("data"), dict) else {}
    value = data.get(key)
    return str(value or "").strip()


def dashboard_chart_ids(payload: dict[str, Any]) -> set[str]:
    data = payload.get("data") if isinstance(payload.get("data"), dict) else {}
    charts = data.get("charts") if isinstance(data.get("charts"), list) else []
    return {
        str(item.get("chartId"))
        for item in charts
        if isinstance(item, dict) and item.get("chartId")
    }


def ledger_step(
    cli_path: str, status: str, params: dict[str, Any], output_ids: Optional[dict[str, str]] = None,
    error: str = "",
) -> dict[str, Any]:
    return {
        "cli_path": cli_path,
        "status": status,
        "params": params,
        "output_ids": output_ids or {},
        "error": error,
    }


def emit(status: str, ledger: list[dict[str, Any]], **result: Any) -> None:
    print(
        json.dumps(
            {
                "schema_version": LEDGER_SCHEMA_VERSION,
                "script": SCRIPT_NAME,
                "status": status,
                "result": result,
                "ledger": ledger,
            },
            ensure_ascii=False,
        )
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("base_id", help="Target AI Table baseId")
    parser.add_argument("dashboard_name", help="Dashboard name")
    parser.add_argument("--chart-specs", help="Workspace-local JSON chart spec file")
    parser.add_argument("--dws", default="dws", help="dws executable")
    args = parser.parse_args()

    base_id = args.base_id.strip()
    dashboard_name = args.dashboard_name.strip()
    if not RESOURCE_ID_PATTERN.fullmatch(base_id) or not dashboard_name:
        parser.error("base_id and dashboard_name are required and must be valid")
    try:
        specs = validate_specs(safe_json_file(args.chart_specs)) if args.chart_specs else []
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        parser.error(str(exc))

    ledger: list[dict[str, Any]] = []
    dashboard_params = {"base-id": base_id, "name": dashboard_name, "format": "json"}
    dashboard, error = run_dws(
        args.dws,
        ["aitable", "dashboard", "create", "--base-id", base_id, "--name", dashboard_name, "--format", "json"],
    )
    if not dashboard:
        ledger.append(ledger_step("aitable dashboard create", "failed", dashboard_params, error=error))
        emit("failed", ledger, error=error)
        return 1
    dashboard_id = extract_id(dashboard, "dashboardId")
    if not dashboard_id:
        error = "dashboard create returned no dashboardId"
        ledger.append(ledger_step("aitable dashboard create", "failed", dashboard_params, error=error))
        emit("failed", ledger, error=error)
        return 1
    ledger.append(
        ledger_step(
            "aitable dashboard create", "success", dashboard_params,
            {"dashboardId": dashboard_id},
        )
    )

    chart_ids: list[str] = []
    for index, spec in enumerate(specs):
        config = chart_config(spec)
        chart_layout = layout(index, len(specs))
        params = {
            "base-id": base_id,
            "dashboard-id": dashboard_id,
            "config": config,
            "layout": chart_layout,
            "format": "json",
        }
        chart, error = run_dws(
            args.dws,
            [
                "aitable", "chart", "create", "--base-id", base_id,
                "--dashboard-id", dashboard_id,
                "--config", json.dumps(config, ensure_ascii=False, separators=(",", ":")),
                "--layout", json.dumps(chart_layout, separators=(",", ":")),
                "--format", "json",
            ],
        )
        if not chart:
            ledger.append(ledger_step("aitable chart create", "failed", params, error=error))
            emit("failed", ledger, dashboardId=dashboard_id, chartIds=chart_ids, error=error)
            return 1
        chart_id = extract_id(chart, "chartId")
        if not chart_id:
            error = "chart create returned no chartId"
            ledger.append(ledger_step("aitable chart create", "failed", params, error=error))
            emit("failed", ledger, dashboardId=dashboard_id, chartIds=chart_ids, error=error)
            return 1
        chart_ids.append(chart_id)
        ledger.append(
            ledger_step("aitable chart create", "success", params, {"chartId": chart_id})
        )

    get_params = {"base-id": base_id, "dashboard-id": dashboard_id, "format": "json"}
    verified, error = run_dws(
        args.dws,
        ["aitable", "dashboard", "get", "--base-id", base_id, "--dashboard-id", dashboard_id, "--format", "json"],
    )
    if not verified:
        ledger.append(ledger_step("aitable dashboard get", "failed", get_params, error=error))
        emit("failed", ledger, dashboardId=dashboard_id, chartIds=chart_ids, error=error)
        return 1
    missing_chart_ids = set(chart_ids) - dashboard_chart_ids(verified)
    if missing_chart_ids:
        error = "dashboard verification missing chartIds: " + ",".join(sorted(missing_chart_ids))
        ledger.append(ledger_step("aitable dashboard get", "failed", get_params, error=error))
        emit("failed", ledger, dashboardId=dashboard_id, chartIds=chart_ids, error=error)
        return 1
    ledger.append(
        ledger_step("aitable dashboard get", "success", get_params, {"dashboardId": dashboard_id})
    )
    emit("success", ledger, dashboardId=dashboard_id, chartIds=chart_ids)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
