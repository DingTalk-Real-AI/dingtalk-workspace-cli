#!/usr/bin/env python3
"""兼容旧入口，将文件导入参数转交给 DWS +import-file。"""

from __future__ import annotations

import argparse
import os
import sys


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="兼容入口：使用 DWS +import-file 导入 CSV/XLSX/XLS 文件"
    )
    parser.add_argument("base_id", help="目标 AI 表格 baseId；续等时仅为旧接口占位")
    parser.add_argument(
        "file_path",
        nargs="?",
        help="待导入文件路径；使用 --resume-import-id 时省略",
    )
    parser.add_argument("--table-id")
    parser.add_argument("--header-row", type=int)
    parser.add_argument("--src-sheet-name")
    parser.add_argument("--field-mapping")
    parser.add_argument("--timeout", type=int, default=30)
    parser.add_argument("--dws", default="dws", help="DWS 可执行文件路径")
    parser.add_argument("--resume-import-id")
    parser.add_argument(
        "--confirmed",
        action="store_true",
        help="确认已向用户展示迁移范围、异常处理和回退方案",
    )
    return parser


def build_dws_command(args: argparse.Namespace) -> list[str]:
    if not args.confirmed:
        raise ValueError("执行迁移前必须获得专门确认，并传入 --confirmed")
    if not args.resume_import_id and not args.file_path:
        raise ValueError("首次导入必须提供文件路径")

    command = [args.dws, "aitable", "+import-file"]
    if args.resume_import_id:
        command.extend(["--resume-import-id", args.resume_import_id])
        if args.file_path:
            command.extend(["--file", args.file_path])
    else:
        command.extend(["--base-id", args.base_id, "--file", args.file_path])

    for flag, value in (
        ("--table-id", args.table_id),
        ("--header-row", args.header_row),
        ("--src-sheet-name", args.src_sheet_name),
        ("--field-mapping", args.field_mapping),
        ("--timeout", args.timeout),
    ):
        if value is not None:
            command.extend([flag, str(value)])
    command.extend(["--format", "json", "--yes"])
    return command


def main() -> None:
    parser = build_parser()
    args = parser.parse_args()
    try:
        command = build_dws_command(args)
    except ValueError as error:
        parser.error(str(error))
    try:
        os.execvp(command[0], command)
    except FileNotFoundError:
        print(f"错误：找不到 DWS 可执行文件：{command[0]}", file=sys.stderr)
        raise SystemExit(127)


if __name__ == "__main__":
    main()
