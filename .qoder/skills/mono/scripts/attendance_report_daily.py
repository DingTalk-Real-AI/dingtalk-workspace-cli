#!/usr/bin/env python3
"""
考勤报表导出 — 每日统计粒度

⛔ 【AI Agent 强制门禁】调用本脚本前必须先阅读：
   dingtalk-workspace/references/products/attendance-report.md

  本脚本仅是"考勤报表导出工作流"的执行末端，工作流完整定义在 attendance-report.md，
  包含但不限于：
    - 阶段 0：报表类型判断（默认月度汇总）
    - 阶段 1：人员列表获取（aisearch person / contact dept list-members）
    - 阶段 2：列选择（是否传 --column-keywords）
    - 阶段 3：调用本脚本
    - 阶段 4：结果回传给用户的标准格式
    - 错误处理（403 权限、HSF_ILLEGALPARAMS、空数据等）

  ❌ 严禁仅凭本脚本 docstring 或 --help 输出就直接拼命令执行，会导致：
     - 报表数据不全 / 列错位 / 人员遗漏
     - 错误处理缺失，把环境错误当业务错误反馈给用户
     - 输出格式不规范，用户体验差

按 (userId, workDate) 分组，每人每天一行。

聚合策略：
  - 通过启发式识别每条记录的"工作日期"：依次尝试字段名
    workDate / work_date / date / userCheckTime / day / 工作日期
  - 同一 (userId, workDate) 下的多条记录按字段聚合：
    * 数值字段 → sum
    * 非数值字段 → 取首个非空值（因为同一天同一字段通常只有一个值）
  - 缺少 workDate 的记录会归入 "_no_date"，并 warn

用法:
  python attendance_report_daily.py \
    --users userId1,userId2,...  \
    --start "2026-03-01 00:00:00" \
    --end   "2026-03-31 23:59:59" \
    [--columns 1001,1002]
    [--column-keywords "工作日期,出勤状态,迟到时长"]
    [--out attendance_report_2026-03-01_2026-03-31_daily.xlsx]
    [--inspect]
"""

from __future__ import annotations

import argparse
import sys
from collections import defaultdict
from datetime import datetime
from typing import Any

import attendance_report_common as cmn

# 默认关注字段 — 与 SKILL.md「每日统计预定义列集合」严格对齐（共 33 个）
# 字段名必须和 `dws attendance report columns` 返回的 name 精确匹配
DEFAULT_KEYWORDS = [
    "班次",
    "上班1打卡时间",
    "上班1打卡结果",
    "下班1打卡时间",
    "下班1打卡结果",
    "上班2打卡时间",
    "上班2打卡结果",
    "下班2打卡时间",
    "下班2打卡结果",
    "上班3打卡时间",
    "上班3打卡结果",
    "下班3打卡时间",
    "下班3打卡结果",
    "关联的审批单",
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
]

# 工作日期字段的候选 key（按优先级试探）
DATE_KEY_CANDIDATES = (
    "workDate", "work_date", "userCheckDate", "checkDate",
    "date", "day", "工作日期",
)

# 请假字段 — 触发"按假期类型展开"的字段名
# 不参与 query-data 查询，单独走 query-leave 接口，按 4 类假期展开为多列
# 注意：钉钉接口实际返回的字段名可能是 "请假"、"请假分类"、"请假时长" 等，
# 凡以 "请假" 开头的都视为请假字段，统一替换为 4 列假期类型展开。
LEAVE_FIELD_NAME = "请假"
LEAVE_TYPES: tuple[str, ...] = ("事假", "调休", "病假", "年假")


def _is_leave_field(name: str) -> bool:
    """判断一个字段名是否属于"请假"系列（如 请假 / 请假分类 / 请假时长）。"""
    return isinstance(name, str) and name.startswith(LEAVE_FIELD_NAME)


# ─────────────────────────────────────────────────────────────────────────────
# 参数解析
# ─────────────────────────────────────────────────────────────────────────────

def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description=(
            "导出考勤报表 — 每日统计粒度。"
            "⛔ AI Agent 必须先读 references/products/attendance-report.md 再调用本脚本，"
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
# 字段解析（与 detail / monthly 一致）
# ─────────────────────────────────────────────────────────────────────────────

def resolve_columns(args: argparse.Namespace) -> list[dict]:
    if args.columns.strip():
        cids = [c.strip() for c in args.columns.split(",") if c.strip()]
        all_cols_payload = cmn.run_dws(["attendance", "report", "columns"])
        all_cols = cmn.extract_records(all_cols_payload)
        id_to_name: dict[str, str] = {}
        for col in all_cols:
            cid = cmn._first_nonempty(col, ("id", "columnId", "code", "key"))
            name = cmn._first_nonempty(col, ("name", "columnName", "title", "label"))
            if cid is not None:
                id_to_name[str(cid)] = str(name) if name else str(cid)
        return [{"_column_id": cid, "_column_name": id_to_name.get(cid, cid)}
                for cid in cids]

    keywords = (
        [k.strip() for k in args.column_keywords.split(",") if k.strip()]
        if args.column_keywords.strip()
        else DEFAULT_KEYWORDS
    )
    cmn.log(f"[columns] 使用关键词匹配字段：{keywords}")
    all_cols_payload = cmn.run_dws(["attendance", "report", "columns"])
    all_cols = cmn.extract_records(all_cols_payload)
    cmn.log(f"[columns] dws 返回 {len(all_cols)} 个字段")
    matched = cmn.match_columns_by_keywords(all_cols, keywords)
    if not matched:
        raise RuntimeError(
            f"未匹配到任何字段。可用字段示例："
            f"{[cmn._first_nonempty(c, ('name','columnName','title','label')) for c in all_cols[:10]]}"
        )
    cmn.log(f"[columns] 匹配到 {len(matched)} 个字段：{[c['_column_name'] for c in matched]}")
    return matched


# ─────────────────────────────────────────────────────────────────────────────
# 接口调用（与 detail / monthly 一致）
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
# 每日聚合
# ─────────────────────────────────────────────────────────────────────────────

def _value_for_column(record: dict, col: dict) -> Any:
    cname, cid = col["_column_name"], col["_column_id"]
    for key in (cname, cid, f"col_{cid}", f"column_{cid}"):
        if key in record:
            return record[key]
    return None


def _try_number(value: Any) -> float | None:
    if value is None or value == "":
        return None
    if isinstance(value, bool):
        return None
    if isinstance(value, (int, float)):
        return float(value)
    if isinstance(value, str):
        try:
            return float(value.strip())
        except ValueError:
            return None
    return None


def _user_id_of(record: dict) -> str | None:
    uid = cmn._first_nonempty(record, ("userId", "userid", "user_id", "targetUserId"))
    return str(uid) if uid is not None else None


def _extract_work_date(record: dict, columns: list[dict]) -> str | None:
    """
    从一条记录里提取"工作日期"（YYYY-MM-DD 格式）。

    试探顺序：
      1. record 里的 DATE_KEY_CANDIDATES
      2. columns 里 _column_name 含"日期"的字段
      3. 13 位毫秒时间戳 → 转 YYYY-MM-DD
      4. ISO 字符串 → 截前 10 位
    都没找到返回 None。
    """
    candidates: list[Any] = []

    # 1) 直接 key
    for key in DATE_KEY_CANDIDATES:
        if key in record and record[key] not in (None, ""):
            candidates.append(record[key])

    # 2) 字段名含"日期"
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


def _normalize_date(raw: Any) -> str | None:
    """把任意形态的日期值归一化为 YYYY-MM-DD 字符串。"""
    if raw is None:
        return None
    # 毫秒时间戳
    if isinstance(raw, (int, float)) and 1_000_000_000_000 <= raw <= 9_999_999_999_999:
        try:
            return datetime.fromtimestamp(raw / 1000).strftime(cmn.DATE_FMT)
        except (OSError, ValueError, OverflowError):
            return None
    # 秒级时间戳
    if isinstance(raw, (int, float)) and 1_000_000_000 <= raw <= 9_999_999_999:
        try:
            return datetime.fromtimestamp(raw).strftime(cmn.DATE_FMT)
        except (OSError, ValueError, OverflowError):
            return None
    s = str(raw).strip()
    if not s:
        return None
    # 已经是 YYYY-MM-DD
    if len(s) >= 10 and s[4] == "-" and s[7] == "-":
        head = s[:10]
        try:
            datetime.strptime(head, cmn.DATE_FMT)
            return head
        except ValueError:
            return None
    return None


def aggregate_daily(
    all_records: list[dict],
    columns: list[dict],
    user_ids: list[str],
    user_name_map: dict[str, str],
    stats: cmn.CallStats,
) -> list[dict[str, Any]]:
    """
    按 (userId, workDate) 聚合：
      - 数值字段：sum
      - 非数值字段：取首个非空值（同一天同字段通常只有一个值）
    返回每人每天一行的 dict 列表，按 userId、workDate 排序。
    """
    # bucket: (userId, date) → column_name → {sum: float, count_num: int, first_nonnum: Any}
    buckets: dict[tuple[str, str], dict[str, dict]] = defaultdict(
        lambda: {col["_column_name"]: {"sum": 0.0, "count_num": 0, "first_nonnum": None}
                 for col in columns}
    )
    no_date_count = 0

    for record in all_records:
        uid = _user_id_of(record)
        if uid is None:
            continue
        date_str = _extract_work_date(record, columns)
        if date_str is None:
            no_date_count += 1
            date_str = "_no_date"

        for col in columns:
            cname = col["_column_name"]
            raw = _value_for_column(record, col)
            num = _try_number(raw)
            cell = buckets[(uid, date_str)][cname]
            if num is not None:
                cell["sum"] += num
                cell["count_num"] += 1
            elif raw not in (None, "") and cell["first_nonnum"] is None:
                cell["first_nonnum"] = raw

    if no_date_count > 0:
        stats.add_warning(
            f"{no_date_count} 条记录无法识别工作日期，已归入 '_no_date'。"
            "请用 --inspect 查看真实字段名"
        )

    # 输出：按 (uid, date) 排序
    rows: list[dict[str, Any]] = []
    for (uid, date_str) in sorted(buckets.keys(), key=lambda x: (x[0], x[1])):
        row: dict[str, Any] = {
            "userId": uid,
            "userName": user_name_map.get(uid, uid),
            "workDate": date_str,
        }
        bucket = buckets[(uid, date_str)]
        for col in columns:
            cname = col["_column_name"]
            cell = bucket[cname]
            if cell["count_num"] > 0:
                total = cell["sum"]
                row[cname] = int(total) if total == int(total) else round(total, 2)
            elif cell["first_nonnum"] is not None:
                row[cname] = cell["first_nonnum"]
            else:
                row[cname] = ""
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

    rows_dict = aggregate_daily(all_records, columns, user_ids, user_name_map, stats)

    # 请假数据特殊处理：通过 query-leave 单独查询，按 4 类假期按天展开
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

    # 表头对齐 SKILL.md 每日统计预定义列集合：姓名 | 考勤组 | 部门 | 日期 | 考勤字段...
    # 请假按假期类型展开为多列（如 "请假-事假", "请假-调休", ...），其余字段保持顺序
    # 多个 "请假*" 字段（如 "请假分类" + "请假时长"）只展开 1 次，避免重复
    base_headers = ["姓名", "考勤组", "部门", "日期"]
    data_headers: list[str] = []
    leave_expanded = False
    for cname in column_names:
        if cname == "工作日期":
            continue
        if _is_leave_field(cname):
            if not leave_expanded:
                data_headers.extend(f"{LEAVE_FIELD_NAME}-{lt}" for lt in LEAVE_TYPES)
                leave_expanded = True
            continue
        data_headers.append(cname)
    headers = base_headers + data_headers

    rows_2d = []
    for row in rows_dict:
        uid = row.get("userId", "")
        info = user_info_map.get(uid, cmn.UserInfo(name=uid))
        group_name = group_name_map.get(uid, "")
        work_date = row.get("workDate", "")
        base = [info.name or uid, group_name, info.dept_name, work_date]
        # 当天该用户的请假数据
        day_leave = leave_data.get(uid, {}).get(work_date, {}) if leave_in_columns else {}
        data: list[Any] = []
        leave_filled = False
        for cname in column_names:
            if cname == "工作日期":
                continue
            if _is_leave_field(cname):
                if not leave_filled:
                    for lt in LEAVE_TYPES:
                        val = day_leave.get(lt, 0.0)
                        if val == 0.0:
                            data.append("")
                        elif val == int(val):
                            data.append(int(val))
                        else:
                            data.append(round(val, 2))
                    leave_filled = True
                continue
            data.append(row.get(cname, ""))
        rows_2d.append(base + data)

    out_name = args.out or cmn.build_output_filename(start, end, suffix="daily")
    title = (
        f"每日统计展示  统计日期：{start.strftime(cmn.DATE_FMT)} "
        f"至 {end.strftime(cmn.DATE_FMT)}"
    )
    subtitle = f"报表生成时间：{datetime.now().strftime('%Y-%m-%d %H:%M')}"
    try:
        cmn.write_excel(
            out_name, headers, rows_2d,
            sheet_name="每日统计",
            title=title,
            subtitle=subtitle,
        )
    except RuntimeError as e:
        cmn.error(str(e))
        return 1

    cmn.print_summary(
        granularity_label="每日统计",
        out_path=out_name,
        user_count=len(user_ids),
        column_names=column_names,
        start=start,
        end=end,
        rows_count=len(rows_2d),
        stats=stats,
        extra_tail="ℹ️ 同一 (用户, 日期) 下数值字段已求和、非数值字段取首个值。",
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
