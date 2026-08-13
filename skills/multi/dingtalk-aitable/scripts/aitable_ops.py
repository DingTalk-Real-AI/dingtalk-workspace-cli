#!/usr/bin/env python3
"""Stable black-box entry point for bundled AI Table workflows."""
from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path


SCRIPT_DIR = Path(__file__).resolve().parent
LEDGER_SCHEMA_VERSION = "dws-skill-script-ledger/v1"


def command_for(args: argparse.Namespace) -> list[str]:
    if args.operation == "dashboard":
        command = [
            "create_dashboard_chart.py", args.base_id, args.dashboard_name,
            *(["--chart-specs", args.chart_specs] if args.chart_specs else []),
        ]
    elif args.operation == "import-new":
        command = ["aitable_import_via_task.py", args.base_id, args.file]
    elif args.operation == "import-records":
        command = [
            "import_records.py", args.base_id, args.table_id, args.file,
            str(args.batch_size),
        ]
    elif args.operation == "export":
        command = [
            "aitable_export_via_task.py", args.base_id, "--scope", args.scope,
            *(["--table-id", args.table_id] if args.table_id else []),
            *(["--view-id", args.view_id] if args.view_id else []),
            *(["--output", args.output] if args.output else []),
            *(["--export-format", args.export_format] if args.export_format else []),
            *(["--overwrite"] if args.overwrite else []),
        ]
    elif args.operation == "add-fields":
        command = [
            "bulk_add_fields.py", args.base_id, args.table_id, args.fields_file,
        ]
    else:
        command = ["upload_attachment.py", args.base_id, args.file]
    return [sys.executable, str(SCRIPT_DIR / command[0]), *command[1:]]


def parser() -> argparse.ArgumentParser:
    root = argparse.ArgumentParser(
        description=(
            "Run a bundled AI Table workflow without reading implementation source. "
            "Each operation preserves the underlying script ledger and exit status."
        )
    )
    sub = root.add_subparsers(dest="operation", required=True)

    dashboard = sub.add_parser("dashboard", help="create and read back a dashboard/chart")
    dashboard.add_argument("base_id")
    dashboard.add_argument("dashboard_name")
    dashboard.add_argument("--chart-specs", help="workspace JSON array of chart specs")

    import_new = sub.add_parser("import-new", help="import CSV/XLS/XLSX as a new table")
    import_new.add_argument("base_id")
    import_new.add_argument("file")

    import_records = sub.add_parser("import-records", help="append JSON/CSV records to an existing table")
    import_records.add_argument("base_id")
    import_records.add_argument("table_id")
    import_records.add_argument("file")
    import_records.add_argument("--batch-size", type=int, default=100)

    export = sub.add_parser("export", help="export a Base, table, or view")
    export.add_argument("base_id")
    export.add_argument("--scope", choices=("all", "table", "view"), required=True)
    export.add_argument("--table-id")
    export.add_argument("--view-id")
    export.add_argument("--output")
    export.add_argument(
        "--export-format",
        choices=("attachment", "excel", "excel_and_attachment", "excel_with_inline_images"),
    )
    export.add_argument("--overwrite", action="store_true")

    fields = sub.add_parser("add-fields", help="create up to 15 fields from JSON")
    fields.add_argument("base_id")
    fields.add_argument("table_id")
    fields.add_argument("fields_file")

    attachment = sub.add_parser("upload-attachment", help="upload a file and return fileToken")
    attachment.add_argument("base_id")
    attachment.add_argument("file")
    return root


def normalize_output(raw: str, args: argparse.Namespace | None = None) -> str:
    """Make a delegated trusted ledger attributable to this stable entry point."""
    try:
        payload = json.loads(raw)
    except json.JSONDecodeError:
        return raw
    if (
        isinstance(payload, dict)
        and payload.get("schema_version") == LEDGER_SCHEMA_VERSION
        and isinstance(payload.get("script"), str)
    ):
        payload["implementation_script"] = payload["script"]
        payload["script"] = Path(__file__).name
        return json.dumps(payload, ensure_ascii=False)
    if args and args.operation == "export" and isinstance(payload, dict):
        status = str(payload.get("status") or "error")
        saved_path = str(payload.get("savedPath") or "")
        file_size = 0
        if saved_path:
            try:
                file_size = Path(saved_path).stat().st_size
            except OSError:
                file_size = 0
        task_id = str(payload.get("taskId") or "")
        ledger_status = "success" if status == "success" and task_id and (
            bool(payload.get("downloadUrl")) and (not saved_path or file_size > 0)
        ) else status
        ledger = {
            "schema_version": LEDGER_SCHEMA_VERSION,
            "script": Path(__file__).name,
            "implementation_script": "aitable_export_via_task.py",
            "status": ledger_status,
            "result": payload,
            "ledger": [{
                "cli_path": "aitable export data",
                "status": ledger_status,
                "params": {
                    "base-id": args.base_id,
                    "scope": args.scope,
                    **({"table-id": args.table_id} if args.table_id else {}),
                    **({"view-id": args.view_id} if args.view_id else {}),
                },
                "output_ids": {
                    "taskId": task_id,
                    "polledTimes": int(payload.get("polledTimes") or 0),
                    "savedPath": saved_path,
                    "fileSize": file_size,
                },
                "error": "" if ledger_status == "success" else str(payload.get("summary") or "export incomplete"),
            }],
        }
        return json.dumps(ledger, ensure_ascii=False)
    return raw


def main() -> int:
    args = parser().parse_args()
    if args.operation == "export":
        if args.scope in {"table", "view"} and not args.table_id:
            parser().error("export --scope table|view requires --table-id")
        if args.scope == "view" and not args.view_id:
            parser().error("export --scope view requires --view-id")
    result = subprocess.run(command_for(args), check=False, capture_output=True, text=True)
    if result.stdout:
        print(normalize_output(result.stdout, args), end="" if result.stdout.endswith("\n") else "\n")
    if result.stderr:
        print(result.stderr, file=sys.stderr, end="" if result.stderr.endswith("\n") else "\n")
    return result.returncode


if __name__ == "__main__":
    raise SystemExit(main())
