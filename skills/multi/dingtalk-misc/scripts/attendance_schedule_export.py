#!/usr/bin/env python3
"""
考勤排班查询导出脚本

[AI Agent 强制门禁] 本脚本执行前必须先阅读：
   references/attendance-schedule.md

职责：
  1. 分批查询排班记录（支持大量用户自动分批）
  2. 将 classId 转为班次名称
  3. 将 userId 转为员工姓名
  4. 输出日历表格式的排班表 Excel（行=员工，列=日期，单元格=班次名称）

用法:
    python attendance_schedule_export.py \
        --users userId1,userId2,userId3 \
        --start 2026-05-19 --end 2026-05-23

    python attendance_schedule_export.py \
        --users userId1,userId2 \
        --start 2026-05-01 --end 2026-05-31 \
        --output my_schedule.xlsx
"""

from __future__ import annotations

import argparse
import os
import sys
from datetime import datetime, timedelta
from typing import Any

from attendance_report_common import (
    DATE_FMT,
    DATETIME_FMT,
    DwsCallError,
    chunk_users,
    error,
    extract_records,
    log,
    parse_datetime_arg,
    resolve_user_names,
    run_dws,
    warn,
    write_excel,
)

# schedule get 接口每批最多用户数（保守值，避免超时）
SCHEDULE_BATCH_SIZE = 20

WEEKDAY_NAMES = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"]


# ─────────────────────────────────────────────────────────────────────────────
# 排班数据查询（分批）
# ─────────────────────────────────────────────────────────────────────────────

def fetch_schedule_batch(
    user_ids: list[str],
    start_date: str,
    end_date: str,
) -> list[dict]:
    """调用 dws attendance schedule get 查询一批用户的排班记录。"""
    users_str = ",".join(user_ids)
    try:
        result = run_dws([
            "attendance", "schedule", "get",
            "--users", users_str,
            "--start", start_date,
            "--end", end_date,
        ])
    except DwsCallError as exc:
        error(f"查询排班失败 (users={len(user_ids)}, {start_date}~{end_date}): {exc}")
        return []
    return extract_records(result) if result else []


def fetch_all_schedules(
    user_ids: list[str],
    start_date: str,
    end_date: str,
) -> list[dict]:
    """分批查询所有用户的排班记录，自动处理用户数超限。"""
    all_records: list[dict] = []
    batches = chunk_users(user_ids, SCHEDULE_BATCH_SIZE)
    total = len(batches)

    log(f"📋 共 {len(user_ids)} 人，分 {total} 批查询排班 ({start_date} ~ {end_date})")

    for idx, batch in enumerate(batches, start=1):
        if total > 1:
            log(f"   批次 {idx}/{total}: {len(batch)} 人")
        records = fetch_schedule_batch(batch, start_date, end_date)
        all_records.extend(records)

    log(f"✅ 查询完成，共 {len(all_records)} 条排班记录")
    return all_records


# ─────────────────────────────────────────────────────────────────────────────
# 班次名称映射
# ─────────────────────────────────────────────────────────────────────────────

def build_class_name_map(records: list[dict]) -> dict[int, str]:
    """从排班记录中提取 classId → className 映射。

    优先使用记录自带的 className；缺失时回退 class search 补全。
    """
    class_map: dict[int, str] = {}
    missing_ids: set[int] = set()

    for record in records:
        raw_id = record.get("classId") or record.get("class_id")
        raw_name = record.get("className") or record.get("class_name")
        if raw_id is None:
            continue
        cid = int(raw_id)
        if raw_name and str(raw_name).strip():
            class_map[cid] = str(raw_name).strip()
        elif cid != 0 and cid not in class_map:
            missing_ids.add(cid)

    if missing_ids:
        log(f"🔍 {len(missing_ids)} 个班次缺名称，从 class search 补全 ...")
        try:
            result = run_dws(["attendance", "class", "search", "--limit", "200"])
            for cls in (extract_records(result) if result else []):
                cid_raw = cls.get("id") or cls.get("classId")
                cname = cls.get("name") or cls.get("className")
                if cid_raw is not None and cname:
                    class_map[int(cid_raw)] = str(cname).strip()
        except DwsCallError as exc:
            warn(f"class search 失败，部分班次将显示为 ID: {exc}")

    return class_map


# ─────────────────────────────────────────────────────────────────────────────
# 日期工具
# ─────────────────────────────────────────────────────────────────────────────

def normalize_work_date(raw: Any) -> str:
    """将排班记录中的 workDate 标准化为 YYYY-MM-DD。"""
    if raw is None:
        return ""
    if isinstance(raw, (int, float)):
        ts = raw / 1000 if raw > 1e12 else raw
        try:
            return datetime.fromtimestamp(ts).strftime(DATE_FMT)
        except (OSError, ValueError, OverflowError):
            return ""
    s = str(raw).strip()
    if len(s) >= 10 and s[4] == "-" and s[7] == "-":
        return s[:10]
    return s


def generate_date_range(start: datetime, end: datetime) -> list[str]:
    """生成 start 到 end 之间的所有日期字符串列表。"""
    dates: list[str] = []
    current = start
    while current <= end:
        dates.append(current.strftime(DATE_FMT))
        current += timedelta(days=1)
    return dates


# ─────────────────────────────────────────────────────────────────────────────
# 构建排班表（日历表格式）
# ─────────────────────────────────────────────────────────────────────────────

def build_schedule_table(
    records: list[dict],
    user_ids: list[str],
    user_names: dict[str, str],
    class_map: dict[int, str],
    date_range: list[str],
) -> tuple[list[str], list[list[str]]]:
    """构建日历表格式的排班表。

    Returns:
        (headers, rows)
        headers = ["员工姓名", "05-19\n周一", "05-20\n周二", ...]
        rows    = [["张三", "早班", "早班", "休息", ...], ...]
    """
    # 构建 (userId, date) → 班次显示文本
    schedule_lookup: dict[tuple[str, str], str] = {}
    for record in records:
        uid = str(record.get("userId") or record.get("userid") or "")
        work_date = normalize_work_date(record.get("workDate") or record.get("work_date"))
        if not uid or not work_date:
            continue

        is_rest = str(record.get("isRest") or record.get("is_rest") or "N").upper()
        raw_cid = record.get("classId") or record.get("class_id") or 0
        raw_cname = record.get("className") or record.get("class_name") or ""

        if is_rest == "Y":
            display = "休息"
        elif raw_cname and str(raw_cname).strip():
            display = str(raw_cname).strip()
        else:
            cid = int(raw_cid) if raw_cid else 0
            if cid in class_map:
                display = class_map[cid]
            elif cid == 0:
                display = "休息"
            else:
                display = f"班次{cid}"

        schedule_lookup[(uid, work_date)] = display

    # 表头
    headers = ["员工姓名"]
    for date_str in date_range:
        dt = datetime.strptime(date_str, DATE_FMT)
        weekday = WEEKDAY_NAMES[dt.weekday()]
        headers.append(f"{date_str[5:]}\n{weekday}")

    # 数据行
    rows: list[list[str]] = []
    for uid in user_ids:
        name = user_names.get(uid, uid)
        row = [name]
        for date_str in date_range:
            row.append(schedule_lookup.get((uid, date_str), ""))
        rows.append(row)

    return headers, rows


# ─────────────────────────────────────────────────────────────────────────────
# 摘要输出
# ─────────────────────────────────────────────────────────────────────────────

def print_summary(
    rows: list[list[str]],
    date_range: list[str],
    out_path: str,
    record_count: int,
) -> None:
    """输出排班查询摘要到 stdout。"""
    out_abs = os.path.abspath(out_path)
    print(f"\n✅ 排班表导出成功！")
    print(f"   文件: {out_abs}")
    print(f"   人数: {len(rows)}")
    print(f"   日期: {date_range[0]} ~ {date_range[-1]} ({len(date_range)} 天)")
    print(f"   记录: {record_count} 条")

    # 预览前 10 人 × 前 7 天
    preview_rows = min(len(rows), 10)
    preview_cols = min(len(date_range), 7)
    if preview_rows > 0:
        print(f"\n排班预览（前 {preview_rows} 人 × 前 {preview_cols} 天）:")
        header_line = f"{'姓名':<10}" + "".join(
            f"{d[5:]:<8}" for d in date_range[:preview_cols]
        )
        print(header_line)
        print("-" * len(header_line))
        for row in rows[:preview_rows]:
            line = f"{row[0]:<10}" + "".join(
                f"{cell:<8}" for cell in row[1:preview_cols + 1]
            )
            print(line)
        if len(date_range) > preview_cols:
            print(f"   ... 共 {len(date_range)} 天，完整数据见 Excel")
        if len(rows) > preview_rows:
            print(f"   ... 共 {len(rows)} 人，完整数据见 Excel")


def main() -> None:
    parser = argparse.ArgumentParser(
        description="考勤排班查询导出（排班表格式）",
        epilog="执行前必须阅读 attendance-schedule.md",
    )
    parser.add_argument("--users", required=True, help="userId 列表，逗号分隔（必填）")
    parser.add_argument("--start", required=True, help="开始日期 YYYY-MM-DD（必填）")
    parser.add_argument("--end", required=True, help="结束日期 YYYY-MM-DD（必填）")
    parser.add_argument("--output", default="", help="输出文件路径（可选）")
    args = parser.parse_args()

    # ── 解析参数 ──
    user_ids = [uid.strip() for uid in args.users.split(",") if uid.strip()]
    if not user_ids:
        error("--users 不能为空")
        raise SystemExit(1)

    try:
        start_dt = parse_datetime_arg(args.start)
        end_dt = parse_datetime_arg(args.end, end_of_day=True)
    except ValueError as exc:
        error(str(exc))
        raise SystemExit(1) from exc

    start_date = start_dt.strftime(DATE_FMT)
    end_date = end_dt.strftime(DATE_FMT)

    if end_dt < start_dt:
        error(f"结束日期 {end_date} 早于开始日期 {start_date}")
        raise SystemExit(1)

    output_path = args.output or f"attendance_schedule_{start_date}_{end_date}.xlsx"

    log(f"🗓️ 排班查询: {len(user_ids)} 人, {start_date} ~ {end_date}")

    # ── 阶段 1: 查询排班记录（分批） ──
    records = fetch_all_schedules(user_ids, start_date, end_date)
    if not records:
        print(f"⚠️ 未查询到排班记录 ({start_date} ~ {end_date})")
        return

    # ── 阶段 2: 构建班次名称映射 ──
    class_map = build_class_name_map(records)

    # ── 阶段 3: 解析员工姓名 ──
    user_names = resolve_user_names(user_ids)

    # ── 阶段 4: 生成日期范围 & 构建排班表 ──
    date_range = generate_date_range(start_dt, end_dt)
    headers, rows = build_schedule_table(
        records, user_ids, user_names, class_map, date_range,
    )

    # ── 阶段 5: 输出 Excel ──
    title = f"排班表  {start_date} 至 {end_date}"
    subtitle = f"生成时间：{datetime.now().strftime(DATETIME_FMT)}  共 {len(rows)} 人"

    write_excel(
        output_path,
        headers,
        rows,
        sheet_name="排班表",
        title=title,
        subtitle=subtitle,
    )

    log(f"📄 Excel 已保存: {os.path.abspath(output_path)}")

    # ── 阶段 6: 输出摘要 ──
    print_summary(rows, date_range, output_path, len(records))


if __name__ == "__main__":
    main()
