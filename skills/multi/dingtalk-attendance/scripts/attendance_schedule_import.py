#!/usr/bin/env python3
"""
考勤排班导入脚本

[AI Agent 强制门禁] 本脚本执行前必须先阅读：
   references/attendance-schedule.md

   排班工作流、参数校验、班次校验、回显确认等约束全部在
   attendance-schedule.md，禁止凭本脚本源码或 --help 自行组装命令。

职责：
  1. 二次校验考勤组类型（必须为 TURN 排班制）
  2. 二次校验班次 ID 在可用班次列表中
  3. 回显排班内容表格，等待用户确认
  4. 调用 dws attendance schedule import 执行排班
  5. 输出执行结果摘要

用法:
    python attendance_schedule_import.py \
        --group-id 123456 \
        --schedules '[{"userId":"u001","workDate":"2026-05-19","classId":789,"isRest":"N"}]' \
        --confirm
"""

from __future__ import annotations

import argparse
import json
import sys
from datetime import datetime
from typing import Any

# 复用公共模块
from attendance_report_common import (
    run_dws,
    DwsCallError,
    extract_records,
    resolve_user_names,
    log,
    warn,
    error,
)

DATE_FMT = "%Y-%m-%d"
DATETIME_FMT = "%Y-%m-%d %H:%M:%S"


# ─────────────────────────────────────────────────────────────────────────────
# 考勤组校验
# ─────────────────────────────────────────────────────────────────────────────

def _unwrap_group_vo(result: dict) -> dict:
    """从 group get 返回结构中提取 groupVO（type/name/classIds 等字段所在层）。

    group get 返回结构：{groupVO: {type, name, classIds, ...}, ...}
    filtered-get 返回结构可能直接是扁平的 {type, name, memberUsers, ...}
    """
    if not isinstance(result, dict):
        return result
    group_vo = result.get("groupVO")
    if isinstance(group_vo, dict) and group_vo.get("type"):
        return group_vo
    # 如果顶层已经有 type 字段，说明是扁平结构，直接返回
    if result.get("type"):
        return result
    # 兜底：尝试从所有 dict 类型的值中找包含 type 字段的
    for value in result.values():
        if isinstance(value, dict) and value.get("type"):
            return value
    return result


def validate_group_is_turn(group_id: int) -> dict:
    """校验考勤组存在且类型为 TURN（排班制），返回考勤组信息（groupVO 层级）。"""
    log(f"🔍 校验考勤组 {group_id} ...")

    # 优先用 group get 获取完整信息（含绑定班次列表）
    try:
        result = run_dws([
            "attendance", "group", "get",
            "--group-id", str(group_id),
        ])
    except DwsCallError:
        # 降级使用 filtered-get
        try:
            result = run_dws([
                "attendance", "group", "filtered-get",
                "--group-id", str(group_id),
            ])
        except DwsCallError as exc:
            error(f"查询考勤组失败: {exc}")
            raise SystemExit(1) from exc

    if not result or not isinstance(result, dict):
        error(f"考勤组 {group_id} 不存在或返回数据异常")
        raise SystemExit(1)

    # 关键：从 groupVO 中提取 type/name 等字段
    group_vo = _unwrap_group_vo(result)
    group_type = group_vo.get("type", "")
    group_name = group_vo.get("name", f"ID:{group_id}")

    if not group_type:
        # 调试输出，帮助排查结构
        log(f"[debug] group get 返回顶层 keys: {list(result.keys())}")
        error(f"未能从考勤组 {group_id} 返回数据中识别出类型字段")
        raise SystemExit(1)

    if group_type != "TURN":
        type_label = {"FIXED": "固定班制", "NONE": "自由工时"}.get(group_type, group_type)
        error(f"考勤组「{group_name}」类型为 {type_label}，不是排班制（TURN），无法执行排班操作")
        raise SystemExit(1)

    log(f"✅ 考勤组「{group_name}」确认为排班制")
    return group_vo


# ─────────────────────────────────────────────────────────────────────────────
# 班次校验
# ─────────────────────────────────────────────────────────────────────────────

def extract_group_bound_classes(group_info: dict) -> set[int]:
    """从考勤组详情中提取绑定的班次 ID 集合。

    兼容多种字段结构：
      - classIds: [int]                    — 班次 ID 数组
      - classes / selectedClass: [dict]    — 班次对象数组 (含 id/classId)
      - shiftVOList: [dict]                — 排班制特有，含 shiftSetting.shiftId
      - classNameIdMap: {name: id}         — 名称到 ID 映射
    """

    def _extract_from_obj(obj: dict) -> set[int]:
        """从单个 dict 层级中提取班次 ID。"""
        ids: set[int] = set()

        # 方式1: classIds / shiftIds 数组（最常见）
        for key in ("classIds", "shiftIds", "classIdList"):
            ids_list = obj.get(key)
            if isinstance(ids_list, list):
                for item in ids_list:
                    try:
                        ids.add(int(item))
                    except (ValueError, TypeError):
                        pass

        # 方式2: classes / selectedClass 对象数组
        for key in ("classes", "selectedClass"):
            classes = obj.get(key)
            if isinstance(classes, list):
                for item in classes:
                    if isinstance(item, dict):
                        class_id = item.get("id") or item.get("classId")
                        if class_id is not None:
                            ids.add(int(class_id))
                    elif isinstance(item, (int, str)):
                        try:
                            ids.add(int(item))
                        except (ValueError, TypeError):
                            pass

        # 方式3: shiftVOList — 排班制考勤组特有字段
        shift_vo_list = obj.get("shiftVOList")
        if isinstance(shift_vo_list, list):
            for shift_vo in shift_vo_list:
                if not isinstance(shift_vo, dict):
                    continue
                # shiftSetting.shiftId
                shift_setting = shift_vo.get("shiftSetting")
                if isinstance(shift_setting, dict):
                    shift_id = shift_setting.get("shiftId") or shift_setting.get("classId")
                    if shift_id is not None:
                        ids.add(int(shift_id))
                # 直接在 shiftVO 层级的 id/shiftId/classId
                for id_key in ("id", "shiftId", "classId"):
                    val = shift_vo.get(id_key)
                    if val is not None:
                        try:
                            ids.add(int(val))
                        except (ValueError, TypeError):
                            pass

        # 方式4: classNameIdMap {name: id}
        class_map = obj.get("classNameIdMap")
        if isinstance(class_map, dict):
            for _, class_id in class_map.items():
                try:
                    ids.add(int(class_id))
                except (ValueError, TypeError):
                    pass

        return ids

    # 优先从 groupVO 提取（group get 返回结构），兼容顶层扁平结构
    bound_ids: set[int] = set()

    group_vo = group_info.get("groupVO")
    if isinstance(group_vo, dict):
        bound_ids.update(_extract_from_obj(group_vo))

    # 同时从顶层提取（兼容 filtered-get 或已解包的结构）
    bound_ids.update(_extract_from_obj(group_info))

    return bound_ids


def fetch_all_classes() -> dict[int, str]:
    """获取全局所有班次，返回 {classId: className}，用于 ID→名称映射。"""
    log("🔍 获取班次名称映射 ...")
    all_classes: dict[int, str] = {}
    page_index = 1
    page_size = 200

    while True:
        try:
            result = run_dws([
                "attendance", "class", "search",
                "--page", str(page_index),
                "--limit", str(page_size),
            ])
        except DwsCallError as exc:
            error(f"查询班次列表失败: {exc}")
            raise SystemExit(1) from exc

        records = extract_records(result) if result else []
        if not records:
            break

        for record in records:
            class_id = record.get("id") or record.get("classId")
            class_name = record.get("name") or record.get("className") or str(class_id)
            if class_id is not None:
                all_classes[int(class_id)] = class_name

        if len(records) < page_size:
            break
        page_index += 1

    log(f"✅ 获取到 {len(all_classes)} 个班次名称")
    return all_classes


def validate_class_ids(
    schedules: list[dict],
    group_bound_class_ids: set[int],
    all_classes: dict[int, str],
    group_name: str,
) -> None:
    """校验排班记录中的 classId 都在该考勤组绑定的班次中。

    如果考勤组未提取到绑定班次列表（可能是接口字段差异），
    则降级为全局班次校验并输出警告。
    """
    # 如果两个来源都无法获取到班次信息，跳过校验（排班导入接口本身有服务端校验）
    no_bound = len(group_bound_class_ids) == 0
    no_global = len(all_classes) == 0

    if no_bound and no_global:
        warn(f"无法获取考勤组绑定班次和全局班次列表，跳过班次校验（将依赖服务端校验）")
        return

    use_global_fallback = no_bound
    if use_global_fallback:
        warn(f"未能从考勤组「{group_name}」详情中提取绑定班次列表，降级为全局班次校验")
        check_set = set(all_classes.keys())
    else:
        check_set = group_bound_class_ids

    invalid_class_ids: set[int] = set()

    for schedule in schedules:
        is_rest = str(schedule.get("isRest", "N")).upper()
        if is_rest == "Y":
            continue
        class_id = int(schedule.get("classId", 0))
        if class_id != 0 and class_id not in check_set:
            invalid_class_ids.add(class_id)

    if invalid_class_ids:
        invalid_names = [all_classes.get(cid, f"ID:{cid}") for cid in sorted(invalid_class_ids)]
        if use_global_fallback:
            error(f"以下班次不在可用班次列表中: {', '.join(invalid_names)}")
        else:
            error(f"以下班次不属于考勤组「{group_name}」: {', '.join(invalid_names)}")
        log(f"「{group_name}」可用班次:")
        available_ids = check_set if not use_global_fallback else set(all_classes.keys())
        for cid in sorted(available_ids):
            cname = all_classes.get(cid, f"ID:{cid}")
            log(f"  - {cname} (ID: {cid})")
        raise SystemExit(1)


# ─────────────────────────────────────────────────────────────────────────────
# 日期格式标准化
# ─────────────────────────────────────────────────────────────────────────────

def normalize_work_date(work_date: Any) -> str:
    """将 workDate 统一转换为 yyyy-MM-dd HH:mm:ss 格式。"""
    if isinstance(work_date, (int, float)):
        timestamp = work_date / 1000 if work_date > 1e12 else work_date
        return datetime.fromtimestamp(timestamp).strftime(DATETIME_FMT)

    date_str = str(work_date).strip()

    for fmt in (DATETIME_FMT, DATE_FMT):
        try:
            parsed = datetime.strptime(date_str, fmt)
            return parsed.strftime(DATETIME_FMT)
        except ValueError:
            continue

    raise ValueError(f"无法解析日期格式: {work_date!r}，请使用 YYYY-MM-DD 格式")


# ─────────────────────────────────────────────────────────────────────────────
# 回显排班内容
# ─────────────────────────────────────────────────────────────────────────────

def print_schedule_preview(
    group_name: str,
    group_id: int,
    schedules: list[dict],
    available_classes: dict[int, str],
    user_names: dict[str, str],
) -> None:
    """向 stdout 打印排班预览表格供用户确认。"""
    print("\n📋 排班确认")
    print(f"\n考勤组: {group_name} (ID: {group_id})")

    dates = sorted({s.get("workDate", "")[:10] for s in schedules})
    if dates:
        print(f"排班日期: {dates[0]} ~ {dates[-1]}")

    print(f"\n{'员工姓名':<12} {'日期':<14} {'班次':<16} {'是否排休':<8}")
    print("-" * 54)

    for schedule in sorted(schedules, key=lambda s: (s.get("userId", ""), s.get("workDate", ""))):
        user_id = schedule.get("userId", "")
        user_name = user_names.get(user_id, user_id)
        work_date = str(schedule.get("workDate", ""))[:10]
        class_id = int(schedule.get("classId", 0))
        is_rest = str(schedule.get("isRest", "N")).upper()

        if is_rest == "Y":
            class_display = "休息"
            rest_display = "是"
        else:
            class_display = available_classes.get(class_id, f"未知班次(ID:{class_id})")
            rest_display = "否"

        print(f"{user_name:<12} {work_date:<14} {class_display:<16} {rest_display:<8}")

    print(f"\n共 {len(schedules)} 条排班记录")


# ─────────────────────────────────────────────────────────────────────────────
# 执行排班
# ─────────────────────────────────────────────────────────────────────────────

def execute_schedule_import(group_id: int, schedules: list[dict]) -> None:
    """调用 dws attendance schedule import 执行排班。"""
    log(f"🚀 正在执行排班导入 ({len(schedules)} 条记录) ...")

    schedules_json = json.dumps(schedules, ensure_ascii=False)

    try:
        result = run_dws([
            "attendance", "schedule", "import",
            "--groupId", str(group_id),
            "--scheduleVOS", schedules_json,
            "--user-say-yes",
        ])
    except DwsCallError as exc:
        error(f"排班导入失败: {exc}")
        if exc.is_permission_error:
            error("提示: 当前账号可能不是考勤管理员，请确认权限")
        raise SystemExit(1) from exc

    log("✅ 排班导入完成")
    return result


# ─────────────────────────────────────────────────────────────────────────────
# 主流程
# ─────────────────────────────────────────────────────────────────────────────

def main() -> None:
    parser = argparse.ArgumentParser(
        description="考勤排班导入（含校验、回显、执行）",
        epilog="执行前必须阅读 attendance-schedule.md",
    )
    parser.add_argument(
        "--group-id", required=True, type=int,
        help="考勤组 ID（必填，必须为排班制考勤组）",
    )
    parser.add_argument(
        "--schedules", required=True,
        help="排班记录 JSON 数组（必填），每条记录包含 userId/workDate/classId/isRest",
    )
    parser.add_argument(
        "--confirm", action="store_true",
        help="用户已确认排班内容（必填，表示用户已在 Agent 回显中确认）",
    )
    parser.add_argument(
        "--dry-run", action="store_true",
        help="仅校验和回显，不实际执行排班",
    )
    args = parser.parse_args()

    # ── 解析排班记录 JSON ──
    try:
        schedules: list[dict] = json.loads(args.schedules)
    except json.JSONDecodeError as exc:
        error(f"--schedules JSON 格式错误: {exc}")
        raise SystemExit(1) from exc

    if not isinstance(schedules, list) or len(schedules) == 0:
        error("--schedules 必须是非空 JSON 数组")
        raise SystemExit(1)

    # ── 校验必填字段 ──
    required_fields = ("userId", "workDate", "classId", "isRest")
    for idx, schedule in enumerate(schedules):
        for field_name in required_fields:
            if field_name not in schedule:
                error(f"schedule[{idx}] 缺少必填字段: {field_name}")
                raise SystemExit(1)

    # ── 标准化日期格式 ──
    for idx, schedule in enumerate(schedules):
        try:
            schedule["workDate"] = normalize_work_date(schedule["workDate"])
        except ValueError as exc:
            error(f"schedule[{idx}] 日期格式错误: {exc}")
            raise SystemExit(1) from exc

    # ── 阶段 1: 校验考勤组（必须为 TURN 排班制） ──
    group_info = validate_group_is_turn(args.group_id)
    group_name = group_info.get("name", f"ID:{args.group_id}")

    # ── 阶段 2: 解析员工姓名 ──
    user_ids = list({s["userId"] for s in schedules})
    user_names = resolve_user_names(user_ids)

    # ── 阶段 3: 校验班次（必须属于该考勤组） ──
    group_bound_class_ids = extract_group_bound_classes(group_info)
    all_classes = fetch_all_classes()
    if group_bound_class_ids:
        log(f"📋 考勤组「{group_name}」绑定了 {len(group_bound_class_ids)} 个班次:")
        for cid in sorted(group_bound_class_ids):
            cname = all_classes.get(cid, f"ID:{cid}")
            log(f"   - {cname} (ID: {cid})")
    validate_class_ids(schedules, group_bound_class_ids, all_classes, group_name)
    log("✅ 班次校验通过")

    # ── 阶段 4: 回显排班内容 ──
    print_schedule_preview(group_name, args.group_id, schedules, all_classes, user_names)

    if args.dry_run:
        print("\n[dry-run] 仅校验和回显，未实际执行排班")
        return

    if not args.confirm:
        print("\n⚠️ 未传入 --confirm 参数，排班未执行")
        print("请在 Agent 回显确认后，添加 --confirm 参数重新执行")
        return

    # ── 阶段 5: 执行排班 ──
    execute_schedule_import(args.group_id, schedules)

    # ── 阶段 6: 输出摘要 ──
    print(f"\n✅ 排班导入成功！")
    print(f"   考勤组: {group_name}")
    print(f"   排班人数: {len(user_ids)}")
    print(f"   排班记录: {len(schedules)} 条")
    dates = sorted({s.get('workDate', '')[:10] for s in schedules})
    if dates:
        print(f"   日期范围: {dates[0]} ~ {dates[-1]}")

    # 展示所有排班明细
    print(f"\n{'员工姓名':<12} {'日期':<14} {'班次':<16} {'是否排休':<8}")
    print("-" * 54)
    for schedule in sorted(schedules, key=lambda s: (s.get("userId", ""), s.get("workDate", ""))):
        uid = schedule.get("userId", "")
        uname = user_names.get(uid, uid)
        wdate = str(schedule.get("workDate", ""))[:10]
        cid = int(schedule.get("classId", 0))
        is_rest = str(schedule.get("isRest", "N")).upper()
        if is_rest == "Y":
            class_display = "休息"
            rest_display = "是"
        else:
            class_display = all_classes.get(cid, f"未知班次(ID:{cid})")
            rest_display = "否"
        print(f"{uname:<12} {wdate:<14} {class_display:<16} {rest_display:<8}")


if __name__ == "__main__":
    main()
