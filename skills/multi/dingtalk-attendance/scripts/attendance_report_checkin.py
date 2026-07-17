#!/usr/bin/env python3
"""
考勤报表导出 — 签到记录粒度

[AI Agent 强制门禁] 调用本脚本前必须先阅读：
   references/attendance-report.md

  本脚本是"签到报表导出工作流"的执行末端，工作流完整定义在 attendance-report.md。

  [严禁] 仅凭本脚本 docstring 或 --help 输出就直接拼命令执行。

导出签到报表：每条签到记录一行，包含签到详情（地点、经纬度、拜访客户、图片等）。

前置依赖：
  pip install openpyxl

用法:
  python attendance_report_checkin.py \
    --users userId1,userId2,...  \
    --start "2026-04-01 00:00:00" \
    --end   "2026-04-07 23:59:59" \
    [--out 签到报表_研发部_20260401_20260407.xlsx]
    [--inspect]
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from datetime import datetime, timedelta
from typing import Any

# ── 前置依赖检查（在任何 dws 调用之前就检测，避免查完数据才报错）───────
_missing_deps: list[str] = []
try:
    import openpyxl as _openpyxl_check  # noqa: F401
except ImportError:
    _missing_deps.append("openpyxl")
try:
    import requests as _requests_check  # noqa: F401
except ImportError:
    _missing_deps.append("requests")
try:
    from PIL import Image as _pil_check  # noqa: F401
except ImportError:
    _missing_deps.append("Pillow")

if _missing_deps:
    print(
        f"[ERROR] 缺少以下依赖：{', '.join(_missing_deps)}\n"
        f"  请先安装：pip install {' '.join(_missing_deps)}\n"
        "安装后重新执行本脚本。\n"
        "（签到报表需要 openpyxl 生成 Excel、requests + Pillow 下载并嵌入签到图片）",
        file=sys.stderr,
    )
    sys.exit(2)

import attendance_report_common as cmn

# ─────────────────────────────────────────────────────────────────────────────
# 自动获取当前认证的 operator 信息
# ─────────────────────────────────────────────────────────────────────────────

def _get_operator_context() -> tuple[str, str]:
    """
    从 `dws auth status --format json` 自动获取当前认证的 corp_id 和 user_id。

    签到接口 (checkin records) 必须传 --operator-corp-id 和 --operator-staff-id，
    这两个值来自 dws 的认证上下文（即 `dws auth status` 返回的 corp_id / user_id），
    而非 `dws contact user get-self` 返回的长格式 userId。

    Returns:
        (operator_corp_id, operator_staff_id) 元组

    Raises:
        SystemExit: 未登录或无法获取认证信息时直接退出
    """
    try:
        result = subprocess.run(
            ["dws", "auth", "status", "--format", "json"],
            capture_output=True,
            text=True,
            timeout=30,
        )
    except FileNotFoundError:
        cmn.error("未找到 dws 命令，请确认 dws CLI 已安装并在 PATH 中")
        sys.exit(2)
    except subprocess.TimeoutExpired:
        cmn.error("dws auth status 超时，请检查网络或重新登录（dws auth login）")
        sys.exit(2)

    if result.returncode != 0:
        cmn.error(
            "获取认证信息失败，请确保已执行 dws auth login 完成登录。\n"
            f"  错误详情：{(result.stderr or result.stdout or '').strip()}"
        )
        sys.exit(2)

    try:
        auth_data = json.loads(result.stdout)
    except json.JSONDecodeError:
        cmn.error(f"dws auth status 返回非 JSON：{result.stdout[:200]!r}")
        sys.exit(2)

    corp_id = auth_data.get("corp_id") or auth_data.get("corpId") or ""
    user_id = auth_data.get("user_id") or auth_data.get("userId") or ""

    if not corp_id or not user_id:
        cmn.error(
            "无法从认证信息中提取 corp_id / user_id，请重新登录：\n"
            "  dws auth login\n"
            f"  当前返回：{json.dumps(auth_data, ensure_ascii=False)[:300]}"
        )
        sys.exit(2)

    cmn.log(f"[auth] 已获取 operator 信息：corp_id={corp_id}, user_id={user_id}")
    return str(corp_id), str(user_id)


# 签到接口限制：开始到结束最多 7 天
MAX_DAYS_PER_CHECKIN_SLICE = 7

# 签到接口限制：每次最多查 100 人（与 check record 一致）
MAX_USERS_PER_CHECKIN_BATCH = 50

# 最多支持 9 张图片列
MAX_IMAGE_COLUMNS = 9

# 报表表头（与用户要求严格对齐）
REPORT_HEADERS = [
    "姓名", "部门", "完整部门",
    "日期", "时间",
    "经度", "纬度", "地点", "详细地址",
    "拜访客户", "客户部门名称", "工作内容",
    "手机标识",
] + [f"图片{i}" for i in range(1, MAX_IMAGE_COLUMNS + 1)]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "导出签到报表 — 签到记录粒度。"
            "[强制] AI Agent 必须先读 references/attendance-report.md 再调用本脚本。"
        ),
    )
    parser.add_argument("--users", required=True,
                        help="userId 列表，逗号分隔（必填）")
    parser.add_argument("--start", required=True,
                        help='开始时间，YYYY-MM-DD 或 "YYYY-MM-DD HH:mm:ss"（必填）')
    parser.add_argument("--end", required=True,
                        help='结束时间，YYYY-MM-DD 或 "YYYY-MM-DD HH:mm:ss"（必填）')
    parser.add_argument("--out", default="",
                        help="输出 xlsx 文件名；不传则按规范自动生成")
    parser.add_argument("--inspect", action="store_true",
                        help="首次跑时打印首条记录原始结构（用于核对真实字段）")
    return parser.parse_args()


# ─────────────────────────────────────────────────────────────────────────────
# 签到接口时间切片（7 天一段）
# ─────────────────────────────────────────────────────────────────────────────

def slice_checkin_date_range(
    start: datetime, end: datetime,
) -> list[cmn.DateSlice]:
    """将日期范围按 7 天一段切片（签到接口限制开始到结束最多 7 天）。"""
    slices: list[cmn.DateSlice] = []
    current = start
    while current <= end:
        slice_end = min(current + timedelta(days=MAX_DAYS_PER_CHECKIN_SLICE - 1), end)
        # 确保 slice_end 的时间部分是当天最后一秒
        slice_end = slice_end.replace(hour=23, minute=59, second=59)
        if slice_end > end:
            slice_end = end
        slices.append(cmn.DateSlice(
            start=current,
            end=slice_end,
        ))
        current = slice_end.replace(hour=0, minute=0, second=0) + timedelta(days=1)
    return slices


def chunk_checkin_users(user_ids: list[str]) -> list[list[str]]:
    """将用户列表按 MAX_USERS_PER_CHECKIN_BATCH 分批。"""
    batches: list[list[str]] = []
    for i in range(0, len(user_ids), MAX_USERS_PER_CHECKIN_BATCH):
        batches.append(user_ids[i:i + MAX_USERS_PER_CHECKIN_BATCH])
    return batches


# ─────────────────────────────────────────────────────────────────────────────
# 签到数据查询
# ─────────────────────────────────────────────────────────────────────────────

def query_checkin_batch(
    user_batch: list[str],
    date_slice: cmn.DateSlice,
    operator_corp_id: str,
    operator_staff_id: str,
    stats: cmn.CallStats,
    *,
    inspect: bool = False,
    inspected_flag: list[bool] | None = None,
) -> list[dict]:
    """查询一批用户在一个时间片内的签到记录。"""
    cmn.log(
        f"[checkin] users={len(user_batch)} "
        f"slice={date_slice.label}"
    )
    try:
        payload = cmn.run_dws([
            "attendance", "checkin", "records",
            "--operator-corp-id", operator_corp_id,
            "--operator-staff-id", operator_staff_id,
            "--staff-ids", ",".join(user_batch),
            "--start", date_slice.start_str,
            "--end", date_slice.end_str,
        ])
        stats.total_dws_calls += 1
    except cmn.DwsCallError as exc:
        stats.total_dws_calls += 1
        stats.failed_calls += 1
        if exc.is_permission_error:
            cmn.error(
                "权限错误：当前账号无管理员权限，无法导出签到报表。\n"
                "请联系考勤管理员或换号重试。"
            )
            raise SystemExit(2) from exc
        err_msg = str(exc)
        if "missing required flag" in err_msg.lower():
            cmn.error(
                "签到接口调用失败：缺少必需参数。\n"
                "请确保已执行 dws auth login 完成登录，以便自动获取 operator 参数。\n"
                f"当前 operator: corp_id={operator_corp_id}, staff_id={operator_staff_id}\n"
                f"原始错误：{err_msg}"
            )
            raise SystemExit(2) from exc
        stats.add_warning(f"[checkin failed] {date_slice.label}: {exc}")
        return []

    records = cmn.extract_records(payload)
    if inspect and records and inspected_flag is not None and not inspected_flag[0]:
        cmn.dump_first_record_for_inspection(records, "checkin-records")
        inspected_flag[0] = True
    return records


# ─────────────────────────────────────────────────────────────────────────────
# 签到记录 → 报表行转换
# ─────────────────────────────────────────────────────────────────────────────

def _format_timestamp(timestamp_value: Any) -> tuple[str, str]:
    """
    将签到时间戳转换为 (日期字符串, 时间字符串)。

    签到接口的 timestamp 为毫秒时间戳。
    """
    if timestamp_value is None:
        return "", ""
    try:
        ts = float(timestamp_value)
        # 判断是毫秒还是秒级时间戳
        if ts > 1_000_000_000_000:
            ts = ts / 1000
        dt = datetime.fromtimestamp(ts)
        return dt.strftime("%Y-%m-%d"), dt.strftime("%H:%M:%S")
    except (ValueError, TypeError, OSError, OverflowError):
        return str(timestamp_value), ""


def transform_records_to_rows(
    records: list[dict],
    user_info_map: dict[str, cmn.UserInfo],
) -> list[list[Any]]:
    """将签到原始记录转换为报表行（与 REPORT_HEADERS 对齐）。"""
    rows: list[list[Any]] = []
    for record in records:
        uid = cmn._first_nonempty(record, ("userId", "userid", "user_id"))
        uid_str = str(uid) if uid is not None else ""
        info = user_info_map.get(uid_str, cmn.UserInfo(name=uid_str))

        # 姓名：优先用 resolve_user_info 的结果，回退到接口返回的 name
        name = info.name or record.get("name", uid_str)
        dept_name = info.dept_name
        # 完整部门：暂用 dept_name（如需更完整的路径可后续扩展）
        full_dept = dept_name

        # 日期与时间
        date_str, time_str = _format_timestamp(record.get("timestamp"))

        # 经纬度
        longitude = record.get("longitude", "")
        latitude = record.get("latitude", "")

        # 地点
        place = record.get("place", "")
        detail_place = record.get("detailPlace", "")

        # 拜访客户 & 客户部门名称
        customers = record.get("customers", "")
        # 签到接口暂无客户部门名称字段，预留空值
        customer_dept = ""

        # 工作内容（备注）
        remark = record.get("remark", "")

        # 手机标识
        mobile_id = record.get("mobileId", "")

        # 图片列（最多 9 张）
        image_list = record.get("imageList") or []
        if isinstance(image_list, str):
            # 兼容接口可能返回逗号分隔的字符串
            image_list = [img.strip() for img in image_list.split(",") if img.strip()]
        image_cells = []
        for i in range(MAX_IMAGE_COLUMNS):
            if i < len(image_list):
                image_cells.append(image_list[i])
            else:
                image_cells.append("")

        row = [
            name, dept_name, full_dept,
            date_str, time_str,
            longitude, latitude, place, detail_place,
            customers, customer_dept, remark,
            mobile_id,
        ] + image_cells
        rows.append(row)

    return rows


# ─────────────────────────────────────────────────────────────────────────────
# main
# ─────────────────────────────────────────────────────────────────────────────

def main() -> int:
    args = parse_args()

    raw_ids = [u.strip() for u in args.users.split(",") if u.strip()]
    if not raw_ids:
        cmn.error("--users 不能为空")
        return 2

    # 自动识别部门ID并展开为员工userId
    user_ids = cmn.resolve_users_from_input(raw_ids)
    if not user_ids:
        cmn.error("未能解析出任何有效的员工userId")
        return 2
    cmn.log(f"[users] 最终用户列表：{len(user_ids)} 人")

    try:
        start = cmn.parse_datetime_arg(args.start, end_of_day=False)
        end = cmn.parse_datetime_arg(args.end, end_of_day=True)
    except ValueError as exc:
        cmn.error(str(exc))
        return 2

    if end < start:
        cmn.error(f"--end ({end}) 早于 --start ({start})")
        return 2

    # 获取当前认证的 operator 信息（签到接口必需）
    operator_corp_id, operator_staff_id = _get_operator_context()

    # 获取用户基础信息（姓名、部门）
    cmn.log(f"[users] 获取 {len(user_ids)} 个用户基础信息")
    user_info_map = cmn.resolve_user_info(user_ids)

    # 分批分段查询签到记录
    user_batches = chunk_checkin_users(user_ids)
    date_slices = slice_checkin_date_range(start, end)
    stats = cmn.CallStats(
        user_batches=len(user_batches),
        date_slices=len(date_slices),
    )
    cmn.log(
        f"[plan] 共 {len(user_batches)} 批 × {len(date_slices)} 个时间片 "
        f"= {len(user_batches) * len(date_slices)} 次接口调用"
    )

    inspected_flag = [False]
    all_records: list[dict] = []
    for batch_idx, batch in enumerate(user_batches, start=1):
        for slice_idx, date_slice in enumerate(date_slices, start=1):
            cmn.log(
                f"[batch {batch_idx}/{len(user_batches)}] "
                f"[slice {slice_idx}/{len(date_slices)}]"
            )
            records = query_checkin_batch(
                batch, date_slice,
                operator_corp_id, operator_staff_id,
                stats,
                inspect=args.inspect,
                inspected_flag=inspected_flag,
            )
            all_records.extend(records)

    if not all_records:
        stats.add_warning("查询完成，但未得到任何签到记录")

    # 转换为报表行
    rows = transform_records_to_rows(all_records, user_info_map)

    # 按日期时间排序（日期列索引=3，时间列索引=4）
    rows.sort(key=lambda r: (r[3] or "", r[4] or ""))

    # 生成 Excel
    out_name = args.out or cmn.build_output_filename(start, end, suffix="checkin")
    title = (
        f"签到报表  统计日期：{start.strftime(cmn.DATE_FMT)} "
        f"至 {end.strftime(cmn.DATE_FMT)}"
    )
    subtitle = f"报表生成时间：{datetime.now().strftime('%Y-%m-%d %H:%M')}"

    # 图片列名列表（图片1~图片9），让 write_excel_multi_sheets 自动将 URL 嵌入为缩略图
    image_column_names = [f"图片{i}" for i in range(1, MAX_IMAGE_COLUMNS + 1)]

    checkin_sheet = {
        "name": "签到记录",
        "headers": REPORT_HEADERS,
        "rows": rows,
        "title": title,
        "subtitle": subtitle,
        "image_columns": image_column_names,
        "image_size": (60, 60),
    }

    try:
        cmn.write_excel_multi_sheets(out_name, [checkin_sheet])
    except (RuntimeError, ValueError) as exc:
        cmn.error(str(exc))
        return 1

    cmn.print_summary(
        granularity_label="签到报表",
        out_path=out_name,
        user_count=len(user_ids),
        column_names=[h for h in REPORT_HEADERS],
        start=start,
        end=end,
        rows_count=len(rows),
        stats=stats,
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
