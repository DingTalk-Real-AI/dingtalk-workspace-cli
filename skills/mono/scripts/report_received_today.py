#!/usr/bin/env python3
"""有界分页列出今天或最近几天收到的日志摘要。

脚本只读取列表投影，不再为每条日志调用 ``entry get``。需要正文时，调用方应
从结果中选择明确的 reportId，再单独读取那一条，避免列表任务退化成 N+1。
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
import time
from datetime import datetime, timedelta
from typing import Any, NamedTuple
from zoneinfo import ZoneInfo


SHANGHAI = ZoneInfo("Asia/Shanghai")
PAGE_SIZE = 20
DEFAULT_MAX_PAGES = 10
HARD_MAX_PAGES = 10
MAX_REPORTS = PAGE_SIZE * HARD_MAX_PAGES
DEFAULT_DISPLAY_LIMIT = 20
PER_COMMAND_TIMEOUT_SECONDS = 60
TOTAL_TIMEOUT_SECONDS = 120
MAX_ERROR_DETAIL_CHARS = 4096


class ReportCommandError(RuntimeError):
    """DWS 执行或响应契约失败，不能降级成合法空结果。"""


class InboxScanResult(NamedTuple):
    """完整扫描证据与受展示上限约束的摘要。"""

    total_count: int
    visible_items: list[dict[str, Any]]


def query_window(days: int, now: datetime | None = None) -> tuple[datetime, datetime]:
    """冻结查询时间窗，并保证午夜调度也得到严格递增的范围。"""
    current = now or datetime.now(SHANGHAI)
    start = (current - timedelta(days=days - 1)).replace(
        hour=0, minute=0, second=0, microsecond=0
    )
    end = current.replace(microsecond=0)
    if end <= start:
        end = start + timedelta(seconds=1)
    return start, end


def format_create_time(value: Any) -> str:
    """把服务端 epoch 毫秒转换为带时区的可读时间，未知形态如实保留。"""
    if isinstance(value, (int, float)) and not isinstance(value, bool):
        return datetime.fromtimestamp(value / 1000, SHANGHAI).strftime(
            "%Y-%m-%d %H:%M:%S %z"
        )
    return str(value or "")


def clip_detail(value: Any, limit: int = MAX_ERROR_DETAIL_CHARS) -> str:
    """把诊断压到固定上限，避免响应正文进入错误日志或模型上下文。"""
    if isinstance(value, (dict, list)):
        text = json.dumps(value, ensure_ascii=False, separators=(",", ":"))
    else:
        text = str(value or "")
    text = text.strip()
    if len(text) <= limit:
        return text
    return text[: limit - 14] + "…[已截断]"


def process_error_detail(result: subprocess.CompletedProcess[str]) -> str:
    """优先保留结构化错误与 stderr；原始 stdout 仅做有界兜底。"""
    parts: list[str] = []
    try:
        payload = json.loads(result.stdout)
    except (json.JSONDecodeError, TypeError):
        payload = None
    if isinstance(payload, dict):
        structured = payload.get("error") or payload.get("message")
        if structured:
            parts.append("error=" + clip_detail(structured, 2048))
    stderr = clip_detail(result.stderr, 2048)
    if stderr:
        parts.append("stderr=" + stderr)
    if not parts:
        stdout = clip_detail(result.stdout, 2048)
        if stdout:
            parts.append("stdout=" + stdout)
    return clip_detail("; ".join(parts)) or "无错误详情"


def run_dws(
    args: list[str], *, dry_run: bool = False, timeout_seconds: float = 60
) -> Any | None:
    cmd = ["dws", *args]
    if dry_run:
        print("[dry-run] " + " ".join(cmd))
        return None
    try:
        result = subprocess.run(
            cmd, capture_output=True, text=True, timeout=timeout_seconds
        )
    except (subprocess.TimeoutExpired, FileNotFoundError) as exc:
        raise ReportCommandError(f"DWS 执行失败: {exc}") from exc
    if result.returncode != 0:
        raise ReportCommandError(
            f"DWS 返回非零状态 exit={result.returncode}: "
            f"{process_error_detail(result)}"
        )
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        raise ReportCommandError(f"DWS 返回的不是合法 JSON: {exc}") from exc


def parse_inbox_page(
    payload: Any, current_cursor: int
) -> tuple[list[dict[str, Any]], int | None]:
    if not isinstance(payload, dict):
        raise ReportCommandError(
            f"收件箱响应应为对象，实际为 {type(payload).__name__}"
        )
    if payload.get("ok") is not True or payload.get("outcome") != "success":
        error = payload.get("error")
        raise ReportCommandError(
            "收件箱调用未成功: " + clip_detail(error or payload)
        )

    data = payload.get("data")
    if not isinstance(data, dict):
        raise ReportCommandError("收件箱成功响应缺少 data 对象")
    reports = data.get("reports")
    if not isinstance(reports, list) or any(
        not isinstance(item, dict) for item in reports
    ):
        raise ReportCommandError("收件箱 data.reports 必须是对象数组")
    count = data.get("count")
    if (
        not isinstance(count, int)
        or isinstance(count, bool)
        or count != len(reports)
    ):
        raise ReportCommandError("收件箱 count 与 reports 数量不一致")
    complete = data.get("complete")
    if not isinstance(complete, bool):
        raise ReportCommandError("收件箱响应缺少布尔 complete")

    meta = payload.get("meta")
    pagination = meta.get("pagination") if isinstance(meta, dict) else None
    if not isinstance(pagination, dict):
        raise ReportCommandError("收件箱响应缺少 meta.pagination")
    exhausted = pagination.get("endpoint_exhausted")
    if not isinstance(exhausted, bool) or exhausted != complete:
        raise ReportCommandError("收件箱 data.complete 与分页终止证据冲突")
    if exhausted:
        return reports, None

    raw_next = pagination.get("next_token")
    try:
        next_cursor = int(raw_next)
    except (TypeError, ValueError) as exc:
        raise ReportCommandError("收件箱续页缺少整数 next_token") from exc
    if next_cursor <= current_cursor:
        raise ReportCommandError("收件箱 continuation cursor 没有严格前进")
    return reports, next_cursor


def scan_inbox(
    start: datetime,
    end: datetime,
    max_pages: int,
    *,
    display_limit: int = DEFAULT_DISPLAY_LIMIT,
    total_timeout_seconds: float = TOTAL_TIMEOUT_SECONDS,
) -> InboxScanResult:
    if not 1 <= display_limit <= MAX_REPORTS:
        raise ReportCommandError(
            f"展示上限必须在 1..{MAX_REPORTS} 之间"
        )
    cursor = 0
    total_count = 0
    visible_items: list[dict[str, Any]] = []
    seen: dict[str, Any] = {}
    deadline = time.monotonic() + total_timeout_seconds
    for _ in range(max_pages):
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise ReportCommandError(
                f"收件箱分页超过总时限 {total_timeout_seconds:g} 秒"
            )
        payload = run_dws([
            "report", "+inbox-list",
            "--start", start.isoformat(timespec="seconds"),
            "--end", end.isoformat(timespec="seconds"),
            "--cursor", str(cursor),
            "--size", str(PAGE_SIZE),
            "--format", "json",
        ], timeout_seconds=max(
            0.1, min(PER_COMMAND_TIMEOUT_SECONDS, remaining)
        ))
        page, next_cursor = parse_inbox_page(payload, cursor)
        for item in page:
            report_id = item.get("reportId")
            if not isinstance(report_id, str) or not report_id.strip():
                raise ReportCommandError("收件箱条目缺少稳定 reportId")
            created = item.get("createTime")
            if report_id in seen:
                if seen[report_id] != created:
                    raise ReportCommandError(
                        f"收件箱重复 reportId 的 createTime 冲突: {report_id}"
                    )
                continue
            if total_count >= MAX_REPORTS:
                raise ReportCommandError(
                    f"收件箱结果超过有界条数上限 {MAX_REPORTS}"
                )
            seen[report_id] = created
            total_count += 1
            if len(visible_items) < display_limit:
                visible_items.append(item)
        if next_cursor is None:
            return InboxScanResult(total_count, visible_items)
        cursor = next_cursor
    raise ReportCommandError(
        f"达到 --max-pages={max_pages} 时收件箱仍有后续页；"
        "拒绝把部分结果伪装成完整列表"
    )


def main() -> int:
    parser = argparse.ArgumentParser(description="查看收到的日志摘要")
    parser.add_argument(
        "--days", type=int, default=1, help="查询天数（默认 1）"
    )
    parser.add_argument(
        "--max-pages",
        type=int,
        default=DEFAULT_MAX_PAGES,
        help=f"最大分页数（默认 {DEFAULT_MAX_PAGES}，范围 1..{HARD_MAX_PAGES}）",
    )
    parser.add_argument(
        "--display-limit",
        type=int,
        default=DEFAULT_DISPLAY_LIMIT,
        help=f"最多展开的摘要数（默认 {DEFAULT_DISPLAY_LIMIT}，范围 1..{MAX_REPORTS}）",
    )
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()
    if args.days < 1:
        parser.error("--days must be >= 1")
    if not 1 <= args.max_pages <= HARD_MAX_PAGES:
        parser.error(
            f"--max-pages must be between 1 and {HARD_MAX_PAGES}"
        )
    if not 1 <= args.display_limit <= MAX_REPORTS:
        parser.error(
            f"--display-limit must be between 1 and {MAX_REPORTS}"
        )

    # 以调用开始时刻冻结查询窗，避免分页过程中把未来新增条目插进结果集。
    start, end = query_window(args.days)
    label = "今天" if args.days == 1 else f"最近 {args.days} 天"

    if args.dry_run:
        run_dws([
            "report", "+inbox-list",
            "--start", start.isoformat(timespec="seconds"),
            "--end", end.isoformat(timespec="seconds"),
            "--cursor", "0",
            "--size", str(PAGE_SIZE),
            "--format", "json",
        ], dry_run=True)
        return 0

    scan = scan_inbox(
        start, end, args.max_pages, display_limit=args.display_limit
    )
    if scan.total_count == 0:
        print(f"{label}暂无收到的日志")
        return 0

    print(f"{label}收到的日志（{scan.total_count} 条，已完成分页）")
    for item in scan.visible_items:
        creator = (
            item.get("creatorName")
            or item.get("creatorUserId")
            or "未知创建人"
        )
        template = item.get("templateName") or "日志"
        print(
            f"- {template} | {creator} | "
            f"{format_create_time(item.get('createTime'))} | {item['reportId']}"
        )
    if len(scan.visible_items) < scan.total_count:
        print(
            f"另有 {scan.total_count - len(scan.visible_items)} 条未展开；"
            f"需要时用 --display-limit {min(scan.total_count, MAX_REPORTS)} 显示。"
        )
    print(
        "需要正文时，请选择上面的明确 reportId "
        "再执行 dws report entry get。"
    )
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except ReportCommandError as exc:
        print(f"错误：{exc}", file=sys.stderr)
        raise SystemExit(2) from exc
