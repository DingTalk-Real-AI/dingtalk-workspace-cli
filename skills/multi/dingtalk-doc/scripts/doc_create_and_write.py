#!/usr/bin/env python3
"""用原生 dws 写入管道创建文档，并回读验证。"""

from __future__ import annotations

import argparse
import json
import shlex
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any, Optional, Sequence


class ScriptError(RuntimeError):
    """可预期的脚本执行错误。"""


def decode_json_output(output: str) -> Any:
    """解析 JSON；兼容长内容写入前置的进度行。"""
    text = output.strip()
    if not text:
        raise ScriptError("dws 未返回 JSON")
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        decoder = json.JSONDecoder()
        for offset, character in enumerate(text):
            if character not in "[{":
                continue
            try:
                value, end = decoder.raw_decode(text, offset)
            except json.JSONDecodeError:
                continue
            if not text[end:].strip():
                return value
    raise ScriptError("dws 返回的不是合法 JSON")


def run_dws(args: Sequence[str], dry_run: bool = False) -> Any:
    """执行一条 dws 命令，并把命令/业务失败统一转成 ScriptError。"""
    command = ["dws", *args]
    if dry_run:
        print(f"[dry-run] {shlex.join(command)}")
        return {"dry_run": True}
    try:
        result = subprocess.run(
            command,
            capture_output=True,
            text=True,
            timeout=120,
            check=False,
        )
    except (subprocess.TimeoutExpired, FileNotFoundError) as exc:
        raise ScriptError(f"执行 dws 失败：{exc}") from exc
    if result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip()
        raise ScriptError(
            f"dws 命令失败：{detail or f'退出码 {result.returncode}'}"
        )
    data = decode_json_output(result.stdout)
    if isinstance(data, dict) and data.get("success") is False:
        detail = data.get("errorMsg") or data.get("message") or "未知错误"
        raise ScriptError(f"dws 业务调用失败：{detail}")
    return data


def first_value(payload: Any, keys: Sequence[str]) -> str:
    """从嵌套响应中提取第一个非空稳定字段。"""
    if isinstance(payload, dict):
        for key in keys:
            value = payload.get(key)
            if value is not None and str(value).strip():
                return str(value).strip()
        for value in payload.values():
            found = first_value(value, keys)
            if found:
                return found
    elif isinstance(payload, list):
        for value in payload:
            found = first_value(value, keys)
            if found:
                return found
    return ""


def run(argv: Optional[Sequence[str]] = None) -> int:
    parser = argparse.ArgumentParser(
        description="使用 dws doc create 创建文档并回读验证"
    )
    parser.add_argument("--name", required=True, help="文档名称")
    content_group = parser.add_mutually_exclusive_group(required=True)
    content_group.add_argument("--content", help="Markdown 内容")
    content_group.add_argument("--content-file", help="UTF-8 Markdown 文件")
    location_group = parser.add_mutually_exclusive_group()
    location_group.add_argument(
        "--folder", default="", help="目标文档文件夹 ID 或 URL"
    )
    location_group.add_argument(
        "--workspace", default="", help="目标知识库 ID 或 URL"
    )
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)

    supplied_path: Optional[Path] = None
    temporary_path: Optional[Path] = None
    if args.content_file:
        supplied_path = Path(args.content_file)
        if not supplied_path.is_file():
            raise ScriptError(f"内容文件不存在：{supplied_path}")
    elif not args.content or not args.content.strip():
        raise ScriptError("--content 不能为空")

    try:
        if supplied_path is None and not args.dry_run:
            with tempfile.NamedTemporaryFile(
                mode="w", encoding="utf-8", suffix=".md", delete=False
            ) as handle:
                handle.write(args.content)
                temporary_path = Path(handle.name)
            supplied_path = temporary_path

        content_path = str(supplied_path) if supplied_path else "<TEMP_CONTENT.md>"
        create_args = [
            "doc", "create",
            "--name", args.name,
            "--content-file", content_path,
            "--content-format", "markdown",
            "--format", "json",
        ]
        if args.folder:
            create_args.extend(["--folder", args.folder])
        if args.workspace:
            create_args.extend(["--workspace", args.workspace])

        created = run_dws(create_args, dry_run=args.dry_run)
        node_id = "<NODE_ID>" if args.dry_run else first_value(
            created, ("nodeId", "dentryUuid")
        )
        if not node_id:
            raise ScriptError("文档创建响应缺少 nodeId，无法验证")

        info = run_dws(
            ["doc", "info", "--node", node_id, "--format", "json"],
            dry_run=args.dry_run,
        )
        run_dws(
            ["doc", "read", "--node", node_id, "--format", "json"],
            dry_run=args.dry_run,
        )
        if args.dry_run:
            return 0

        summary = {
            "success": True,
            "nodeId": node_id,
            "docUrl": first_value(info, ("docUrl", "documentUrl", "url"))
            or first_value(created, ("docUrl", "documentUrl", "url")),
            "chunksWritten": first_value(created, ("chunksWritten",)),
            "verified": True,
        }
        print(json.dumps(summary, ensure_ascii=False))
        return 0
    finally:
        if temporary_path is not None:
            temporary_path.unlink(missing_ok=True)


def main() -> None:
    try:
        raise SystemExit(run())
    except ScriptError as exc:
        print(f"错误：{exc}", file=sys.stderr)
        raise SystemExit(1) from exc


if __name__ == "__main__":
    main()
