#!/usr/bin/env python3
"""
考勤报表导出 — 月度汇总粒度

[AI Agent 强制门禁] 调用本脚本前必须先阅读：
   dingtalk-workspace/references/products/attendance-report.md

  本脚本仅是"考勤报表导出工作流"的执行末端，工作流完整定义在 attendance-report.md，
  包含但不限于：
    - 阶段 0：报表类型判断（默认月度汇总）
    - 阶段 1：人员列表获取（aisearch person / contact dept list-members）
    - 阶段 2：列选择（是否传 --column-keywords）
    - 阶段 3：调用本脚本
    - 阶段 4：结果回传给用户的标准格式
    - 错误处理（403 权限、HSF_ILLEGALPARAMS、空数据等）

  [严禁] 仅凭本脚本 docstring 或 --help 输出就直接拼命令执行，会导致：
     - 报表数据不全 / 列错位 / 人员遗漏
     - 错误处理缺失，把环境错误当业务错误反馈给用户
     - 输出格式不规范，用户体验差

按人按字段汇总，每人一行（如：迟到 5 次、加班 32 小时、出勤 21 天）。

聚合策略：
  - 数值字段（看起来是 int/float）→ 求和
  - 时长字段（字段名含"时长"且值为数字）→ 求和（保留单位语义）
  - 字符串/枚举字段（如出勤状态）→ 计数（distinct value → count）
  - 日期字段 → 计数（去重日期 → 出勤天数）
  - 复杂字段（dict/list）→ 拼接（最多 5 条）

用法:
  python attendance_report_monthly.py \
    --users userId1,userId2,...  \
    --start "2026-03-01 00:00:00" \
    --end   "2026-03-31 23:59:59" \
    [--columns 1001,1002]
    [--column-keywords "迟到次数,加班时长"]
    [--out attendance_report_2026-03-01_2026-03-31_monthly.xlsx]
    [--inspect]
"""

from __future__ import annotations

import argparse
import sys
from collections import defaultdict
from datetime import datetime, timedelta
from typing import Any

import attendance_report_common as cmn

# 默认关注字段 — 与 SKILL.md「月度汇总预定义列集合」严格对齐（共 20 个）
# 字段名必须和 `dws attendance report columns` 返回的 name 精确匹配
DEFAULT_KEYWORDS = [
    "出勤天数",
    "休息天数",
    "工作时长",
    "迟到次数",
    "迟到时长",
    "严重迟到次数",
    "严重迟到时长",
    "旷工迟到次数",
    "早退次数",
    "早退时长",
    "上班缺卡次数",
    "下班缺卡次数",
    "旷工天数",
    "出差时长",
    "外出时长",
    "请假",
    "加班-审批单统计",
    "考勤结果",
]

# 每日维度字段 — 这些字段在月度汇总中不做聚合，而是按天展开成多列
DAILY_EXPAND_FIELDS = {"考勤结果"}

# 日历表指标 — sheet2"日历表"展示的 3 行指标
# 这 3 个字段会被 resolve_columns 强制追加到查询字段集中（即使用户的 --column-keywords 没包含），
# 否则日历表会是空的。
# 注意：这 3 个字段名必须和 dws attendance report columns 返回的 name 严格一致。
CALENDAR_METRICS: tuple[str, ...] = ("班次名称", "考勤结果", "工作时长")

# 请假字段 — 触发"按假期类型展开"的字段名
# 不参与 query-data 查询，单独走 query-leave 接口，按 4 类假期展开为多列
# 注意：钉钉接口实际返回的字段名可能是 "请假"、"请假分类"、"请假时长" 等，
# 凡以 "请假" 开头的都视为请假字段，统一替换为 4 列假期类型展开。
LEAVE_FIELD_NAME = "请假"
LEAVE_TYPES: tuple[str, ...] = ("事假", "调休", "病假", "年假")


def _is_leave_field(name: str) -> bool:
    """判断一个字段名是否属于"请假"系列（如 请假 / 请假分类 / 请假时长）。"""
    return isinstance(name, str) and name.startswith(LEAVE_FIELD_NAME)

# 工作日期字段的候选 key（按优先级试探）
DATE_KEY_CANDIDATES = (
    "workDate", "work_date", "userCheckDate", "checkDate",
    "date", "day", "工作日期",
)


# ─────────────────────────────────────────────────────────────────────────────
# 参数解析
# ─────────────────────────────────────────────────────────────────────────────

def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description=(
            "导出考勤报表 — 月度汇总粒度。"
            "[强制] AI Agent 必须先读 references/products/attendance-report.md 再调用本脚本，"
            "禁止凭 --help 或脚本路径自行拼命令。"
        ),
    )
    p.add_argument("--users", required=True,
                   help="userId 列表，逗号分隔（必填）")
    p.add_argument("--start", required=True,
                   help='开始时间，YYYY-MM-DD 或 "YYYY-MM-DD HH:mm:ss"（必填）')
    p.add_argument("--end", required=True,
                   help='结束时间，YYYY-MM-DD 或 "YYYY-MM-DD HH:mm:ss"（必填）')
    p.add_argument("--columns", default="",
                   help="字段 ID 列表，逗号分隔；与 --column-keywords 二选一")
    p.add_argument("--column-keywords", default="",
                   help="字段名关键词，逗号分隔；不传则走默认字段集")
    p.add_argument("--out", default="",
                   help="输出 xlsx 文件名；不传则按规范自动生成")
    p.add_argument("--inspect", action="store_true",
                   help="首次跑时打印首条记录原始结构（用于核对真实字段）")
    return p.parse_args()


# ─────────────────────────────────────────────────────────────────────────────
# 字段解析（与 detail 一致）
# ─────────────────────────────────────────────────────────────────────────────

def _ensure_calendar_metrics(
    matched: list[dict],
    all_cols: list[dict],
) -> list[dict]:
    """
    确保 CALENDAR_METRICS 中的 3 个指标字段（班次名称/考勤结果/工作时长）
    出现在最终查询字段集中（即使用户传入的 --column-keywords 没匹配到）。

    日历表 sheet2 强依赖这 3 个字段，缺一不可。
    """
    existing_names = {c["_column_name"] for c in matched}
    name_to_col: dict[str, dict] = {}
    for col in all_cols:
        cid = cmn._first_nonempty(col, ("id", "columnId", "code", "key"))
        name = cmn._first_nonempty(col, ("name", "columnName", "title", "label"))
        if cid is not None and name:
            name_to_col[str(name)] = {
                "_column_id": str(cid),
                "_column_name": str(name),
            }

    appended: list[str] = []
    for metric_name in CALENDAR_METRICS:
        if metric_name in existing_names:
            continue
        col = name_to_col.get(metric_name)
        if col is None:
            cmn.log(
                f"[calendar] 警告：月历指标字段「{metric_name}」在"
                f" report columns 中未找到，月历对应行可能为空"
            )
            continue
        matched.append(col)
        appended.append(metric_name)
    if appended:
        cmn.log(f"[calendar] 已强制追加月历指标字段：{appended}")
    return matched


def resolve_columns(args: argparse.Namespace) -> list[dict]:
    all_cols_payload = cmn.run_dws(["attendance", "report", "columns"])
    all_cols = cmn.extract_records(all_cols_payload)

    if args.columns.strip():
        cids = [c.strip() for c in args.columns.split(",") if c.strip()]
        id_to_name: dict[str, str] = {}
        for col in all_cols:
            cid = cmn._first_nonempty(col, ("id", "columnId", "code", "key"))
            name = cmn._first_nonempty(col, ("name", "columnName", "title", "label"))
            if cid is not None:
                id_to_name[str(cid)] = str(name) if name else str(cid)
        matched = [{"_column_id": cid, "_column_name": id_to_name.get(cid, cid)}
                   for cid in cids]
        return _ensure_calendar_metrics(matched, all_cols)

    keywords = (
        [k.strip() for k in args.column_keywords.split(",") if k.strip()]
        if args.column_keywords.strip()
        else DEFAULT_KEYWORDS
    )
    cmn.log(f"[columns] 使用关键词匹配字段：{keywords}")
    cmn.log(f"[columns] dws 返回 {len(all_cols)} 个字段")
    matched = cmn.match_columns_by_keywords(all_cols, keywords)
    if not matched:
        raise RuntimeError(
            f"未匹配到任何字段。可用字段示例："
            f"{[cmn._first_nonempty(c, ('name','columnName','title','label')) for c in all_cols[:10]]}"
        )
    cmn.log(f"[columns] 匹配到 {len(matched)} 个字段：{[c['_column_name'] for c in matched]}")
    return _ensure_calendar_metrics(matched, all_cols)


# ─────────────────────────────────────────────────────────────────────────────
# 接口调用（与 detail 一致）
# ─────────────────────────────────────────────────────────────────────────────

def query_one_batch(
    user_batch: list[str],
    column_ids: list[str],
    date_slice: cmn.DateSlice,
    stats: cmn.CallStats,
    *,
    column_id_to_name: dict[str, str] | None = None,
    inspect: bool = False,
    inspected_flag: list[bool] = None,
) -> list[dict]:
    cmn.log(
        f"[query] users={len(user_batch)} cols={len(column_ids)} "
        f"slice={date_slice.label}"
    )
    try:
        payload = cmn.run_dws([
            "attendance", "report", "query-data",
            "--users", ",".join(user_batch),
            "--columns", ",".join(column_ids),
            "--start", date_slice.start_str,
            "--end", date_slice.end_str,
        ])
        stats.total_dws_calls += 1
    except cmn.DwsCallError as e:
        stats.total_dws_calls += 1
        stats.failed_calls += 1
        if e.is_permission_error:
            cmn.error(
                "权限错误：当前账号无管理员权限，无法导出考勤报表。"
                "请联系考勤管理员或换号重试。"
            )
            raise SystemExit(2) from e
        stats.add_warning(f"[query failed] {date_slice.label}: {e}")
        return []

    records = cmn.extract_records(payload)
    # 展平 report query-data 返回的嵌套 values 结构
    records = cmn.flatten_query_data_records(records, column_id_to_name)
    if inspect and records and inspected_flag is not None and not inspected_flag[0]:
        cmn.dump_first_record_for_inspection(records, "query-data (flattened)")
        inspected_flag[0] = True
    return records


# ─────────────────────────────────────────────────────────────────────────────
# 日期提取（复用 daily 脚本的逻辑）
# ─────────────────────────────────────────────────────────────────────────────

def _normalize_date(raw: Any) -> str | None:
    """把任意形态的日期值归一化为 YYYY-MM-DD 字符串。"""
    if raw is None:
        return None
    if isinstance(raw, (int, float)) and 1_000_000_000_000 <= raw <= 9_999_999_999_999:
        try:
            return datetime.fromtimestamp(raw / 1000).strftime(cmn.DATE_FMT)
        except (OSError, ValueError, OverflowError):
            return None
    if isinstance(raw, (int, float)) and 1_000_000_000 <= raw <= 9_999_999_999:
        try:
            return datetime.fromtimestamp(raw).strftime(cmn.DATE_FMT)
        except (OSError, ValueError, OverflowError):
            return None
    s = str(raw).strip()
    if not s:
        return None
    if len(s) >= 10 and s[4] == "-" and s[7] == "-":
        head = s[:10]
        try:
            datetime.strptime(head, cmn.DATE_FMT)
            return head
        except ValueError:
            return None
    return None


def _extract_work_date(record: dict, columns: list[dict]) -> str | None:
    """从一条记录里提取工作日期（YYYY-MM-DD 格式）。"""
    candidates: list[Any] = []
    for key in DATE_KEY_CANDIDATES:
        if key in record and record[key] not in (None, ""):
            candidates.append(record[key])
    for col in columns:
        if "日期" in col["_column_name"] or "date" in col["_column_name"].lower():
            v = _value_for_column(record, col)
            if v not in (None, ""):
                candidates.append(v)
    for raw in candidates:
        date_str = _normalize_date(raw)
        if date_str:
            return date_str
    return None


def _generate_date_columns(start: datetime, end: datetime) -> list[str]:
    """根据日期范围生成按天展开的列标签列表，格式为日号（如 '1', '2', ...）。"""
    dates: list[str] = []
    current = start.replace(hour=0, minute=0, second=0, microsecond=0)
    end_date = end.replace(hour=0, minute=0, second=0, microsecond=0)
    while current <= end_date:
        dates.append(current.strftime(cmn.DATE_FMT))
        current += timedelta(days=1)
    return dates


# ─────────────────────────────────────────────────────────────────────────────
# 月度聚合
# ─────────────────────────────────────────────────────────────────────────────

def _value_for_column(record: dict, col: dict) -> Any:
    """从一条原始记录里取某个字段的值（命名顺位试探）。"""
    cname, cid = col["_column_name"], col["_column_id"]
    for key in (cname, cid, f"col_{cid}", f"column_{cid}"):
        if key in record:
            return record[key]
    return None


def _try_number(value: Any) -> float | None:
    """尝试把 value 解析为数字；不能则返回 None。"""
    if value is None or value == "":
        return None
    if isinstance(value, bool):
        return None
    if isinstance(value, (int, float)):
        return float(value)
    if isinstance(value, str):
        s = value.strip()
        try:
            return float(s)
        except ValueError:
            return None
    return None


def _user_id_of(record: dict) -> str | None:
    uid = cmn._first_nonempty(record, ("userId", "userid", "user_id", "targetUserId"))
    return str(uid) if uid is not None else None


def aggregate_monthly(
    all_records: list[dict],
    columns: list[dict],
    user_ids: list[str],
    user_name_map: dict[str, str],
) -> tuple[list[dict[str, Any]], dict[str, dict[str, dict[str, str]]]]:
    """
    按 userId 分组聚合：
      - 普通字段（数值/非数值）：按原聚合策略处理
      - DAILY_EXPAND_FIELDS 中的字段（如"考勤结果"）：按 (userId, date) 存储，不聚合

    返回:
      - rows: 每人一行的聚合结果（不含按天展开字段）
      - daily_data: {field_name: {userId: {date_str: value}}}
    """
    # 识别哪些列需要按天展开
    expand_col_names = {col["_column_name"] for col in columns
                        if col["_column_name"] in DAILY_EXPAND_FIELDS}
    agg_columns = [col for col in columns if col["_column_name"] not in expand_col_names]

    # 聚合累加器（仅普通字段）
    agg: dict[str, dict[str, dict]] = defaultdict(
        lambda: {col["_column_name"]: {"sum": 0.0, "count": 0, "non_numeric": set()}
                 for col in agg_columns}
    )

    # 按天展开数据：field_name → userId → date_str → value
    daily_data: dict[str, dict[str, dict[str, str]]] = {
        fname: defaultdict(dict) for fname in expand_col_names
    }

    for record in all_records:
        uid = _user_id_of(record)
        if uid is None:
            continue
        work_date = _extract_work_date(record, columns)

        # 按天展开字段
        for fname in expand_col_names:
            matching_col = next((c for c in columns if c["_column_name"] == fname), None)
            if matching_col and work_date:
                raw = _value_for_column(record, matching_col)
                if raw not in (None, ""):
                    daily_data[fname][uid][work_date] = str(raw)

        # 普通字段聚合
        for col in agg_columns:
            cname = col["_column_name"]
            raw = _value_for_column(record, col)
            num = _try_number(raw)
            if num is not None:
                agg[uid][cname]["sum"] += num
                agg[uid][cname]["count"] += 1
            elif raw not in (None, ""):
                agg[uid][cname]["non_numeric"].add(str(raw))

    rows: list[dict[str, Any]] = []
    for uid in user_ids:
        row: dict[str, Any] = {
            "userId": uid,
            "userName": user_name_map.get(uid, uid),
        }
        bucket = agg.get(uid, {})
        for col in agg_columns:
            cname = col["_column_name"]
            cell = bucket.get(cname)
            if not cell or (cell["count"] == 0 and not cell["non_numeric"]):
                row[cname] = ""
            elif cell["count"] > 0 and not cell["non_numeric"]:
                total = cell["sum"]
                row[cname] = int(total) if total == int(total) else round(total, 2)
            elif cell["count"] == 0 and cell["non_numeric"]:
                vals = sorted(cell["non_numeric"])
                preview = "/".join(vals[:5]) + ("…" if len(vals) > 5 else "")
                row[cname] = f"{len(vals)} 种：{preview}"
            else:
                total = cell["sum"]
                num_part = int(total) if total == int(total) else round(total, 2)
                vals = sorted(cell["non_numeric"])
                preview = "/".join(vals[:3])
                row[cname] = f"{num_part}（另含非数值：{preview}）"
        rows.append(row)
    return rows, daily_data


# ─────────────────────────────────────────────────────────────────────────────
# 日历表（sheet2）构建
# ─────────────────────────────────────────────────────────────────────────────

def _build_calendar_value_map(
    all_records: list[dict],
    columns: list[dict],
    user_ids: list[str],
) -> dict[str, dict[str, dict[str, str]]]:
    """
    从 all_records 中按 (uid, date, metric_name) 提取 CALENDAR_METRICS 的值。

    返回: {uid: {date_str: {metric_name: value_str}}}

    注：同一 (uid, date, metric) 若有多条记录，取最后一条非空值（query-data 同日同字段
    通常只返回一条）。
    """
    valid_user_ids = set(user_ids)
    metric_cols: dict[str, dict] = {}
    for col in columns:
        if col["_column_name"] in CALENDAR_METRICS:
            metric_cols[col["_column_name"]] = col

    result: dict[str, dict[str, dict[str, str]]] = {}
    for record in all_records:
        uid = _user_id_of(record)
        if uid is None or uid not in valid_user_ids:
            continue
        work_date = _extract_work_date(record, columns)
        if not work_date:
            continue
        for metric_name, col in metric_cols.items():
            raw = _value_for_column(record, col)
            if raw in (None, ""):
                continue
            uid_bucket = result.setdefault(uid, {})
            date_bucket = uid_bucket.setdefault(work_date, {})
            date_bucket[metric_name] = str(raw)
    return result


def build_calendar_sheet(
    all_records: list[dict],
    columns: list[dict],
    user_ids: list[str],
    user_info_map: dict[str, "cmn.UserInfo"],
    group_name_map: dict[str, str],
    start: datetime,
    end: datetime,
) -> dict:
    """
    构建日历表 sheet2 的描述 dict（供 write_excel_multi_sheets 使用）。

    布局（参考钉钉考勤月历）：
      列：姓名 | 考勤组 | 部门 | 指标 | 1日 | 2日 | ... | N日
      每个用户占 3 行（班次名称 / 考勤结果 / 工作时长）
      基础列（前 3 列）做纵向 3 行合并

    返回的 sheet dict 包含 merge_groups 配置，让 write_excel_multi_sheets
    自动完成基础列合并。
    """
    all_dates = _generate_date_columns(start, end)

    # 表头：基础列 + 指标列 + 日期列
    headers = ["姓名", "考勤组", "部门", "指标"] + [
        f"{datetime.strptime(d, cmn.DATE_FMT).day}日" for d in all_dates
    ]

    # 抽取每个 (uid, date, metric) 的值
    value_map = _build_calendar_value_map(all_records, columns, user_ids)

    rows: list[list[Any]] = []
    merge_groups: list[tuple[int, int, int]] = []
    attend_result_row_offsets: set[int] = set()
    n_metrics = len(CALENDAR_METRICS)

    for uid in user_ids:
        info = user_info_map.get(uid, cmn.UserInfo(name=uid))
        group_name = group_name_map.get(uid, "")
        base_cells = [info.name or uid, group_name, info.dept_name]
        block_start = len(rows)  # 当前用户首行的 row_offset

        for metric_name in CALENDAR_METRICS:
            row_cells: list[Any] = list(base_cells) + [metric_name]
            for date_str in all_dates:
                val = value_map.get(uid, {}).get(date_str, {}).get(metric_name, "")
                row_cells.append(val)
            if metric_name == "考勤结果":
                attend_result_row_offsets.add(len(rows))
            rows.append(row_cells)

        block_end = len(rows) - 1  # 当前用户末行的 row_offset
        if block_end > block_start:
            # 基础列 = 前 3 列（姓名/考勤组/部门），需纵向合并
            merge_groups.append((block_start, block_end, 3))

    title = (
        f"日历表  统计日期：{start.strftime(cmn.DATE_FMT)} "
        f"至 {end.strftime(cmn.DATE_FMT)}"
    )
    subtitle = f"报表生成时间：{datetime.now().strftime('%Y-%m-%d %H:%M')}"

    return {
        "name": "日历表",
        "headers": headers,
        "rows": rows,
        "title": title,
        "subtitle": subtitle,
        "merge_groups": merge_groups,
        "attend_result_rows": attend_result_row_offsets or None,
    }


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
    except ValueError as e:
        cmn.error(str(e))
        return 2

    if end < start:
        cmn.error(f"--end ({end}) 早于 --start ({start})")
        return 2

    try:
        columns = resolve_columns(args)
    except cmn.DwsCallError as e:
        if e.is_permission_error:
            cmn.error("权限错误：当前账号无管理员权限，无法获取考勤字段列表。")
            return 2
        cmn.error(f"获取字段列表失败：{e}")
        return 1
    except RuntimeError as e:
        cmn.error(str(e))
        return 1
    column_ids = [c["_column_id"] for c in columns]
    column_names = [c["_column_name"] for c in columns]
    column_id_to_name = {c["_column_id"]: c["_column_name"] for c in columns}

    cmn.log(f"[users] 获取 {len(user_ids)} 个用户基础信息")
    user_info_map = cmn.resolve_user_info(user_ids)
    user_name_map = {uid: info.name or uid for uid, info in user_info_map.items()}

    user_batches = cmn.chunk_users(user_ids)
    date_slices = cmn.slice_date_range(start, end)
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
    for bi, batch in enumerate(user_batches, start=1):
        for si, dslice in enumerate(date_slices, start=1):
            cmn.log(f"[batch {bi}/{len(user_batches)}] [slice {si}/{len(date_slices)}]")
            records = query_one_batch(
                batch, column_ids, dslice, stats,
                column_id_to_name=column_id_to_name,
                inspect=args.inspect,
                inspected_flag=inspected_flag,
            )
            all_records.extend(records)

    if not all_records:
        stats.add_warning("查询完成，但未得到任何记录")

    # 从原始记录中提取每个用户的考勤组名称
    group_name_map = cmn.extract_group_names_from_records(all_records, user_ids)

    # 月度聚合（普通字段聚合 + 每日维度字段按天存储）
    rows_dict, daily_data = aggregate_monthly(all_records, columns, user_ids, user_name_map)

    # 请假数据特殊处理：通过 query-leave 单独查询，按 4 类假期月度求和
    # 凡是 "请假" 开头的字段（请假 / 请假分类 / 请假时长 等）都视为请假列
    leave_in_columns = any(_is_leave_field(name) for name in column_names)
    leave_data: dict[str, dict[str, dict[str, float]]] = {}
    if leave_in_columns:
        try:
            leave_data = cmn.query_leave_data(
                user_ids, start, end,
                leave_names=LEAVE_TYPES,
                stats=stats,
            )
        except cmn.DwsCallError as e:
            stats.add_warning(f"[leave] 查询请假数据失败：{e}")

    # 生成日期范围内所有日期列表
    all_dates = _generate_date_columns(start, end)

    # 构建表头：基础列 + 普通聚合字段（剔除"请假*"系列和按天展开字段）+ 请假展开列 + 按天展开字段
    base_headers = ["姓名", "考勤组", "部门"]
    agg_column_names = [
        name for name in column_names
        if name not in DAILY_EXPAND_FIELDS and not _is_leave_field(name)
    ]

    # 请假按假期类型展开（如 "请假-事假", "请假-调休", ...）
    leave_headers: list[str] = []
    if leave_in_columns:
        leave_headers = [f"{LEAVE_FIELD_NAME}-{lt}" for lt in LEAVE_TYPES]

    # 按天展开的表头：字段名-日号（如 "考勤结果-1日", "考勤结果-2日", ...）
    expand_headers: list[str] = []
    expand_date_map: list[tuple[str, str]] = []  # [(field_name, date_str), ...]
    for fname in column_names:
        if fname in DAILY_EXPAND_FIELDS:
            for date_str in all_dates:
                day_num = datetime.strptime(date_str, cmn.DATE_FMT).day
                header_label = f"{fname}-{day_num}日"
                expand_headers.append(header_label)
                expand_date_map.append((fname, date_str))

    headers = base_headers + agg_column_names + leave_headers + expand_headers

    # 计算考勤结果列的 0-based 列索引集合（供 Excel 条件配色使用）
    _expand_col_start = len(base_headers) + len(agg_column_names) + len(leave_headers)
    attend_result_col_indices: set[int] = set()
    for i, (fname, _date) in enumerate(expand_date_map):
        if fname == "考勤结果":
            attend_result_col_indices.add(_expand_col_start + i)

    rows_2d = []
    for row in rows_dict:
        uid = row.get("userId", "")
        info = user_info_map.get(uid, cmn.UserInfo(name=uid))
        group_name = group_name_map.get(uid, "")
        base = [info.name or uid, group_name, info.dept_name]
        agg_data = [row.get(h, "") for h in agg_column_names]
        # 请假按假期类型聚合（月度求和）
        leave_row: list[Any] = []
        if leave_in_columns:
            user_leave = leave_data.get(uid, {})
            for lt in LEAVE_TYPES:
                total = 0.0
                for day_bucket in user_leave.values():
                    total += day_bucket.get(lt, 0.0)
                if total == 0.0:
                    leave_row.append("")
                elif total == int(total):
                    leave_row.append(int(total))
                else:
                    leave_row.append(round(total, 2))
        # 按天展开字段的数据
        expand_data = []
        for fname, date_str in expand_date_map:
            value = daily_data.get(fname, {}).get(uid, {}).get(date_str, "")
            expand_data.append(value)
        rows_2d.append(base + agg_data + leave_row + expand_data)

    out_name = args.out or cmn.build_output_filename(start, end, suffix="monthly")
    title = (
        f"月度汇总展示  统计日期：{start.strftime(cmn.DATE_FMT)} "
        f"至 {end.strftime(cmn.DATE_FMT)}"
    )
    subtitle = f"报表生成时间：{datetime.now().strftime('%Y-%m-%d %H:%M')}"

    # sheet1：月度汇总（每人一行）
    summary_sheet = {
        "name": "月度汇总",
        "headers": headers,
        "rows": rows_2d,
        "title": title,
        "subtitle": subtitle,
        "attend_result_columns": attend_result_col_indices or None,
    }

    # sheet2：日历表（每人 3 行：班次名称 / 考勤结果 / 工作时长，按日期展开）
    calendar_sheet = build_calendar_sheet(
        all_records, columns, user_ids,
        user_info_map, group_name_map,
        start, end,
    )

    try:
        cmn.write_excel_multi_sheets(out_name, [summary_sheet, calendar_sheet])
    except (RuntimeError, ValueError) as e:
        cmn.error(str(e))
        return 1

    cmn.print_summary(
        granularity_label="月度汇总",
        out_path=out_name,
        user_count=len(user_ids),
        column_names=column_names,
        start=start,
        end=end,
        rows_count=len(rows_2d),
        stats=stats,
        extra_tail=(
            "[提示] 数值字段已求和；"
            "「考勤结果」按天展开为多列（每天一列显示当天考勤状态）。\n"
            "[提示] 已附加第二个 sheet「日历表」：每人 3 行（班次名称/考勤结果/工作时长），"
            "按日期横向展开，基础列（姓名/考勤组/部门）已纵向合并。"
        ),
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
