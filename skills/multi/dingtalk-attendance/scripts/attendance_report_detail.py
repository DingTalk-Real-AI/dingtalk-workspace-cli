#!/usr/bin/env python3
"""
考勤报表导出 — 明细粒度（打卡记录）

通过 `dws attendance check result` + `dws attendance check record`
查询打卡数据，每条打卡记录输出一行，不做聚合。



[AI Agent 强制门禁] 调用本脚本前必须先阅读：
   dingtalk-workspace/references/products/attendance-report.md

  本脚本仅是"考勤报表导出工作流"的执行末端，工作流完整定义在 attendance-report.md，
  包含但不限于：
    - 阶段 0：报表类型判断（默认月度汇总，明细需用户明确说"明细/原始记录/每条打卡"）
    - 阶段 1：人员列表获取（aisearch person / contact dept list-members）
    - 阶段 2：列选择（明细报表列固定，不支持 --column-keywords）
    - 阶段 3：调用本脚本
    - 阶段 4：结果回传给用户的标准格式
    - 错误处理（403 权限、HSF_ILLEGALPARAMS、空数据等）

  [严禁] 仅凭本脚本 docstring 或 --help 输出就直接拼命令执行，会导致：
     - 用户本来要"汇总"被给成"明细"（粒度错误）
     - 报表数据不全 / 人员遗漏
     - 错误处理缺失，把环境错误当业务错误反馈给用户

与月度汇总/每日统计不同，明细报表：
  - 不使用 report columns / report query-data
  - 列固定（基础信息 + 打卡字段），不支持自定义列选择
  - 分批限制：≤100 人/次（check result），时间跨度 ≤1 个月

用法:
  python attendance_report_detail.py \
    --users userId1,userId2,...  \
    --start "2026-03-01" \
    --end   "2026-03-31" \
    [--out attendance_report_2026-03-01_2026-03-31_detail.xlsx]
    [--inspect]                # 首次跑时打印首条记录原始结构

约束:
  - 仅管理员可用，否则 dws 接口返回 403
  - --users 超过 100 人 → 自动按每批 100 人分批
  - --start 到 --end 超过 31 天 → 自动按月切片
"""

from __future__ import annotations

import argparse
import json
import sys
from datetime import datetime
from typing import Any

import attendance_report_common as cmn

# ─────────────────────────────────────────────────────────────────────────────
# 接口限制（check result / check record）
# ─────────────────────────────────────────────────────────────────────────────

CHECK_MAX_USERS_PER_BATCH = 100     # check result: --users 最多 100 人
CHECK_MAX_DAYS_PER_SLICE = 31       # check result/record: 跨度 ≤ 1 个月
CHECK_RESULT_PAGE_SIZE = 1000       # check result: --limit 最大值

# ─────────────────────────────────────────────────────────────────────────────
# 固定表头（与 SKILL.md 明细预定义列集合对齐）
# ─────────────────────────────────────────────────────────────────────────────

# 基础信息列
BASE_HEADERS = ["姓名", "考勤组", "部门"]

# 打卡字段列（以打卡流水为主，关联 check result 的考勤时间和打卡结果）
# 对应 Diamond 配置中 termId 8-20 的列定义
CHECK_HEADERS = [
    "考勤日期", "考勤时间", "打卡时间", "打卡结果",
    "打卡地址", "打卡备注", "异常打卡原因",
    "打卡图片1", "打卡图片2", "打卡设备", "管理员修改备注",
    "管理员修改备注图片1", "管理员修改备注图片2", "管理员修改备注图片3",
]

ALL_HEADERS = BASE_HEADERS + CHECK_HEADERS


# ─────────────────────────────────────────────────────────────────────────────
# 参数解析
# ─────────────────────────────────────────────────────────────────────────────

def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description=(
            "导出考勤报表 — 明细粒度（打卡记录）。"
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
    p.add_argument("--out", default="",
                   help="输出 xlsx 文件名；不传则按规范自动生成")
    p.add_argument("--inspect", action="store_true",
                   help="首次跑时打印首条记录原始结构（用于核对真实字段）")
    p.add_argument("--no-images", action="store_true",
                   help="不在 Excel 中嵌入打卡图片（默认会下载 URL 并嵌入为缩略图，"
                        "图片多时较慢；加此参数仅保留 URL 文本）")
    p.add_argument("--image-size", default="80x120",
                   help="嵌入图片像素尺寸 WxH，默认 80x120")
    return p.parse_args()


# 含图片 URL 的列名（与 CHECK_HEADERS 中的中文名严格一致）
IMAGE_COLUMN_NAMES = [
    "打卡图片1", "打卡图片2",
    "管理员修改备注图片1", "管理员修改备注图片2", "管理员修改备注图片3",
]


def _parse_image_size(spec: str) -> tuple[int, int]:
    """解析 --image-size 参数，格式 WxH。失败时回退到默认 (80, 120)。"""
    try:
        parts = spec.lower().replace(" ", "").split("x")
        w, h = int(parts[0]), int(parts[1])
        if w > 0 and h > 0:
            return (w, h)
    except (ValueError, IndexError):
        pass
    cmn.warn(f"--image-size 格式无效: {spec!r}，使用默认 80x120")
    return (80, 120)


# ─────────────────────────────────────────────────────────────────────────────
# check result 查询（打卡结果，含分页）
# ─────────────────────────────────────────────────────────────────────────────

def query_check_results(
    user_batch: list[str],
    date_slice: cmn.DateSlice,
    stats: cmn.CallStats,
    *,
    inspect: bool = False,
    inspected_flag: list[bool] | None = None,
) -> list[dict]:
    """
    对一批 users × 一个时间片调用 `dws attendance check result`。

    自动分页：每次最多 1000 条，返回满 1000 条时递增 offset 继续拉取。
    """
    from_date = date_slice.start.strftime(cmn.DATE_FMT)
    to_date = date_slice.end.strftime(cmn.DATE_FMT)
    all_records: list[dict] = []
    offset = 0

    while True:
        cmn.log(
            f"[check-result] users={len(user_batch)} "
            f"slice={date_slice.label} offset={offset}"
        )
        try:
            payload = cmn.run_dws([
                "attendance", "check", "result",
                "--users", ",".join(user_batch),
                "--from", from_date,
                "--to", to_date,
                "--offset", str(offset),
                "--limit", str(CHECK_RESULT_PAGE_SIZE),
            ])
            stats.total_dws_calls += 1
        except cmn.DwsCallError as exc:
            stats.total_dws_calls += 1
            stats.failed_calls += 1
            if exc.is_permission_error:
                cmn.error(
                    "权限错误：当前账号无管理员权限，无法查询打卡结果。"
                    "请联系考勤管理员或换号重试。"
                )
                raise SystemExit(2) from exc
            stats.add_warning(f"[check-result failed] {date_slice.label} offset={offset}: {exc}")
            break

        records = cmn.extract_records(payload)

        if inspect and records and inspected_flag is not None and not inspected_flag[0]:
            cmn.dump_first_record_for_inspection(records, "check-result")
            inspected_flag[0] = True

        all_records.extend(records)

        # 未满一页 → 无需翻页
        if len(records) < CHECK_RESULT_PAGE_SIZE:
            break
        offset += CHECK_RESULT_PAGE_SIZE

    return all_records


# ─────────────────────────────────────────────────────────────────────────────
# check record 查询（打卡流水）
# ─────────────────────────────────────────────────────────────────────────────

def query_check_records(
    user_batch: list[str],
    date_slice: cmn.DateSlice,
    stats: cmn.CallStats,
    *,
    inspect: bool = False,
    inspected_flag: list[bool] | None = None,
) -> list[dict]:
    """对一批 users × 一个时间片调用 `dws attendance check record`。"""
    from_date = date_slice.start.strftime(cmn.DATE_FMT)
    to_date = date_slice.end.strftime(cmn.DATE_FMT)

    cmn.log(
        f"[check-record] users={len(user_batch)} slice={date_slice.label}"
    )
    try:
        payload = cmn.run_dws([
            "attendance", "check", "record",
            "--users", ",".join(user_batch),
            "--from", from_date,
            "--to", to_date,
        ])
        stats.total_dws_calls += 1
    except cmn.DwsCallError as exc:
        stats.total_dws_calls += 1
        stats.failed_calls += 1
        if exc.is_permission_error:
            cmn.error(
                "权限错误：当前账号无管理员权限，无法查询打卡流水。"
                "请联系考勤管理员或换号重试。"
            )
            raise SystemExit(2) from exc
        stats.add_warning(f"[check-record failed] {date_slice.label}: {exc}")
        return []

    records = cmn.extract_records(payload)

    if inspect and records and inspected_flag is not None and not inspected_flag[0]:
        cmn.dump_first_record_for_inspection(records, "check-record")
        inspected_flag[0] = True

    return records


# ─────────────────────────────────────────────────────────────────────────────
# 值提取工具
# ─────────────────────────────────────────────────────────────────────────────

def _humanize_timestamp(value: Any) -> str:
    """把毫秒/秒级时间戳转成可读字符串；非时间戳原样返回。"""
    if value is None:
        return ""
    if isinstance(value, (int, float)):
        # 13 位毫秒时间戳
        if 1_000_000_000_000 <= value <= 9_999_999_999_999:
            try:
                return datetime.fromtimestamp(value / 1000).strftime(cmn.DATETIME_FMT)
            except (OSError, ValueError, OverflowError):
                return str(value)
        # 10 位秒级时间戳
        if 1_000_000_000 <= value <= 9_999_999_999:
            try:
                return datetime.fromtimestamp(value).strftime(cmn.DATETIME_FMT)
            except (OSError, ValueError, OverflowError):
                return str(value)
    return str(value) if value != "" else ""


def _extract_field(record: dict, candidate_keys: tuple[str, ...]) -> Any:
    """从 record 中按候选 key 顺序取第一个非空值。"""
    return cmn._first_nonempty(record, candidate_keys)


def _extract_date_str(record: dict) -> str:
    """从 check result 记录中提取考勤日期（YYYY-MM-DD）。"""
    raw = _extract_field(record, (
        "workDate", "work_date", "checkDate", "userCheckDate", "date", "day",
    ))
    if raw is None:
        return ""
    # 毫秒时间戳
    if isinstance(raw, (int, float)) and raw > 1_000_000_000_000:
        try:
            return datetime.fromtimestamp(raw / 1000).strftime(cmn.DATE_FMT)
        except (OSError, ValueError, OverflowError):
            return str(raw)
    s = str(raw).strip()
    # 已经是 YYYY-MM-DD 或 YYYY-MM-DD HH:mm:ss → 取前 10 位
    if len(s) >= 10 and s[4] == "-" and s[7] == "-":
        return s[:10]
    return s


def _extract_time_str(record: dict, candidate_keys: tuple[str, ...]) -> str:
    """从记录中提取时间字段，毫秒时间戳自动转 HH:mm:ss。"""
    raw = _extract_field(record, candidate_keys)
    if raw is None:
        return ""
    if isinstance(raw, (int, float)) and raw > 1_000_000_000_000:
        try:
            return datetime.fromtimestamp(raw / 1000).strftime("%H:%M:%S")
        except (OSError, ValueError, OverflowError):
            return str(raw)
    if isinstance(raw, (int, float)) and raw > 1_000_000_000:
        try:
            return datetime.fromtimestamp(raw).strftime("%H:%M:%S")
        except (OSError, ValueError, OverflowError):
            return str(raw)
    return str(raw)


# ─────────────────────────────────────────────────────────────────────────────
# 字段翻译 / 提取工具函数（与 Java DataProvider 实现对齐）
# ─────────────────────────────────────────────────────────────────────────────

# 打卡结果映射（对应 CheckResultUtil.java 的 getCheckResultStr 逻辑）
_CHECK_RESULT_MAP: dict[str, str] = {
    "Normal":      "正常",
    "Late":        "迟到",
    "Early":       "早退",
    "NotSigned":   "未打卡",
    "SeriousLate": "严重迟到",
    "Absenteeism": "旷工迟到",
    "LeaveEarly":  "早退",
}

# 打卡设备 / 来源类型映射（对应 SourceType 枚举 + UserDeviceOriginData.java）
_SOURCE_TYPE_MAP: dict[str, str] = {
    "ATM":          "考勤机",
    "BEACON":       "蓝牙",
    "DING_ATM":     "钉钉考勤机",
    "USER":         "手机打卡",
    "BOSS":         "管理员",
    "SYSTEM":       "系统",
    "CARD":         "门禁",
    "SELF_SERVICE": "自助补卡",
}

# 异常打卡原因中文描述（对应 SecurityConfigureUtil DEFAULT_CHEAT_LIST）
_CHEAT_REASON_MAP: dict[str, str] = {
    "LocationNotMatch":  "定位异常",
    "WifiNotMatch":      "WIFI异常",
    "MockLocation":      "模拟定位",
    "FaceNotMatch":      "人脸比对失败",
    "DeviceNotMatch":    "设备异常",
    "OutsideRange":      "不在打卡范围",
    "NoBluetooth":       "蓝牙未开启",
    "BluetoothNotMatch": "蓝牙不匹配",
}


def _translate_check_result(raw_result: str) -> str:
    """
    把接口返回的英文打卡结果翻译成中文，与 CheckResultUtil.getCheckResultStr 对齐。
    未命中翻译表时原样返回。
    """
    if not raw_result:
        return ""
    return _CHECK_RESULT_MAP.get(raw_result, raw_result)


def _translate_source_type(raw_source: str) -> str:
    """
    把接口返回的 sourceType 枚举值翻译成中文，与 UserDeviceOriginData 对齐。
    未命中翻译表时原样返回。
    """
    if not raw_source:
        return ""
    return _SOURCE_TYPE_MAP.get(raw_source, raw_source)


def _extract_location(record: dict) -> str:
    """
    拼接打卡地址：地点名称 + 详细地址，与 UserLocationOriginData 对齐。

    Java 逻辑：
      locationResult.getSpaceName()  → 地点名称
      locationResult.getDetailAddr() → 详细地址（含省市区+街道）
    两者均有时拼接，只有一个时单独返回。
    """
    space_name = str(_extract_field(record, (
        "spaceName", "space_name", "locationName", "location_name",
    )) or "").strip()
    detail_addr = str(_extract_field(record, (
        "detailAddr", "detail_addr", "detailAddress", "address", "userAddress",
    )) or "").strip()

    if space_name and detail_addr:
        return f"{space_name} {detail_addr}"
    return space_name or detail_addr


def _extract_exception_reason(record: dict) -> str:
    """
    提取并翻译异常打卡原因，与 CheckExceptionReasonOriginData 对齐。

    Java 逻辑：
      取 features.getInvalidRecordMsg()（逗号分隔的错误码列表）
      逐个从 DEFAULT_CHEAT_LIST 查中文描述后再拼接返回。
    """
    raw = str(_extract_field(record, (
        "invalidRecordMsg", "invalid_record_msg",
        "outsideRemark", "outside_remark",
        "exceptionReason",
    )) or "").strip()

    if not raw:
        return ""

    # 逗号分隔的多个错误码，逐个翻译后重新拼接
    codes = [c.strip() for c in raw.split(",") if c.strip()]
    translated = [_CHEAT_REASON_MAP.get(code, code) for code in codes]
    return ",".join(translated)


def _extract_photo_url(record: dict, candidate_keys: tuple[str, ...]) -> str:
    """从 record 或其 features 嵌套结构中提取图片 URL。"""
    raw = _extract_field(record, candidate_keys)
    if raw is None:
        return ""
    return str(raw).strip()


def _extract_remark_photo(record: dict) -> str:
    """
    打卡图片1（备注/外勤打卡照片）。

    dws check record 的真实返回字段（实测验证）：
      - 顶层 photoUrl：外勤/拍照打卡的主图片 URL
      - 顶层 outsideAttachment：外勤打卡的附件（可能含多张图片）
      - 顶层 remarkPhotos：备注图片数组（旧字段，部分版本）
    Java 侧 RemarkPhotoOriginData 对应 features.getRemarkPhotos()，
    但 dws CLI 实际把图片字段提到了顶层，需直接读顶层字段。
    """
    # 1) 兼容数组形式的 remarkPhotos（早期版本）
    remark_photos = record.get("remarkPhotos") or record.get("remark_photos")
    if isinstance(remark_photos, list) and remark_photos:
        return str(remark_photos[0]).strip()
    if isinstance(remark_photos, str) and remark_photos.strip():
        parts = [p.strip() for p in remark_photos.split(",") if p.strip()]
        return parts[0] if parts else ""

    # 2) dws CLI 当前实际返回的字段（顶层）
    #    photoUrl 优先，其次 outsideAttachment，再次旧候选名
    photo = _extract_photo_url(record, (
        "photoUrl", "photo_url",
        "outsideAttachment", "outside_attachment",
        "remarkPhoto", "remark_photo",
        "userImage", "user_image", "imageUrl", "image_url",
    ))
    if photo:
        # outsideAttachment 可能是逗号分隔多张，取第一张
        if "," in photo:
            first = photo.split(",")[0].strip()
            if first:
                return first
        return photo

    # 3) 兜底：从 features 嵌套 JSON 里翻
    return _extract_photo_from_features(record, (
        "photoUrl", "remarkPhoto", "remarkPhotos",
        "outsideAttachment", "userImage", "imageUrl",
    ))


def _extract_face_check_photo(record: dict) -> str:
    """
    打卡图片2（人脸识别照片）。

    Java 侧 FaceCheckPhotoOriginData 对应 features.getFacePhoto()。
    dws CLI 中人脸图未稳定暴露在顶层，优先读 features 嵌套字段。
    """
    # 1) 顶层候选
    face = _extract_photo_url(record, (
        "facePhoto", "face_photo",
        "faceCheckPhoto", "face_check_photo",
        "faceImage", "face_image",
        "faceUrl", "face_url",
    ))
    if face:
        return face
    # 2) features 嵌套兜底
    return _extract_photo_from_features(record, (
        "facePhoto", "faceCheckPhoto", "faceImage", "faceUrl",
    ))


def _extract_photo_from_features(
    record: dict,
    candidate_keys: tuple[str, ...],
) -> str:
    """
    从 record['features']（JSON 字符串或 dict）中提取图片 URL。
    候选 key 命中 features 中第一个非空值则返回。
    """
    feat = record.get("features")
    if isinstance(feat, str):
        feat_str = feat.strip()
        if not feat_str or feat_str[0] not in "{[":
            return ""
        try:
            feat = json.loads(feat_str)
        except (ValueError, TypeError):
            return ""
    if not isinstance(feat, dict):
        return ""
    for key in candidate_keys:
        val = feat.get(key)
        if val in (None, "", [], {}):
            continue
        if isinstance(val, list) and val:
            return str(val[0]).strip()
        s = str(val).strip()
        if "," in s:
            return s.split(",")[0].strip()
        return s
    return ""


def _extract_boss_remark(record: dict) -> str:
    """
    管理员修改备注，与 BossCheckRemarkOriginData 对齐。
    Java 逻辑：features.getBossRemark()。
    """
    return str(_extract_field(record, (
        "bossRemark", "boss_remark",
        "approveRemark", "approve_remark",
        "adminModifyRemark", "admin_modify_remark",
    )) or "").strip()


def _extract_boss_photo(record: dict, photo_index: int) -> str:
    """
    管理员修改备注图片（1/2/3），与 BossCheckPhoto1/2/3OriginData 对齐。
    Java 逻辑：features.getBossPhotos()，按 index 取对应张。
    photo_index: 0-based 索引（0=图片1, 1=图片2, 2=图片3）
    """
    boss_photos = record.get("bossPhotos") or record.get("boss_photos")
    if isinstance(boss_photos, list):
        if photo_index < len(boss_photos):
            return str(boss_photos[photo_index]).strip()
        return ""
    if isinstance(boss_photos, str) and boss_photos.strip():
        parts = [p.strip() for p in boss_photos.split(",") if p.strip()]
        return parts[photo_index] if photo_index < len(parts) else ""

    # 降级：尝试独立字段
    val = _extract_field(record, (
        f"bossPhoto{photo_index + 1}", f"boss_photo_{photo_index + 1}",
    ))
    return str(val).strip() if val else ""


# ─────────────────────────────────────────────────────────────────────────────
# 关联合并 check result + check record → 明细行
# ─────────────────────────────────────────────────────────────────────────────

def _build_result_index(
    check_results: list[dict],
) -> dict[tuple[str, str], list[dict]]:
    """
    把 check result 按 (userId, 打卡时间 YYYY-MM-DD HH:mm:ss) 建索引，
    用于关联打卡流水获取考勤时间和打卡结果。
    """
    index: dict[tuple[str, str], list[dict]] = {}
    for rec in check_results:
        uid = str(_extract_field(rec, ("userId", "userid", "user_id")) or "")
        raw_time = _extract_field(rec, (
            "userCheckTime", "user_check_time", "checkTime", "baseCheckTime",
        ))
        time_key = _humanize_timestamp(raw_time) if raw_time else "_unknown"
        key = (uid, time_key)
        index.setdefault(key, []).append(rec)
    return index


def build_record_rows(
    check_records: list[dict],
    check_results: list[dict],
    user_info_map: dict[str, cmn.UserInfo],
    group_name_map: dict[str, str],
) -> list[dict[str, str]]:
    """
    以 check record（打卡流水）为主表构建明细行。

    每条打卡流水记录输出一行，只展示有实际打卡的记录。
    通过打卡时间关联 check result 获取"考勤时间"和"打卡结果"。

    列顺序与 Diamond 配置 termId 8-20 对齐，各字段逻辑与 Java DataProvider 一致。
    返回每行一个 dict，key 与 ALL_HEADERS 对齐。
    """
    result_index = _build_result_index(check_results)
    rows: list[dict[str, str]] = []

    for record in check_records:
        uid = str(_extract_field(record, ("userId", "userid", "user_id")) or "")
        info = user_info_map.get(uid, cmn.UserInfo(name=uid))

        # ── 打卡时间（实际打卡时间，OriginUserCheckTimePlug）────────────────
        actual_time_raw = _extract_field(record, (
            "userCheckTime", "user_check_time", "checkTime",
        ))
        actual_time = _humanize_timestamp(actual_time_raw)

        # ── 关联 check result 获取"考勤时间"和"打卡结果" ──────────────────
        time_key = actual_time if actual_time else "_unknown"
        matched_results = result_index.get((uid, time_key), [])
        result_rec = matched_results[0] if matched_results else {}

        # 考勤时间 = 班次规定的应打卡时间（OriginPlanCheckTimePlug）
        plan_time_raw = _extract_field(result_rec, (
            "planCheckTime", "plan_check_time", "baseCheckTime",
        )) if result_rec else None
        plan_time = _humanize_timestamp(plan_time_raw) if plan_time_raw else ""

        # 打卡结果（OriginUserCheckResultPlug）：英文枚举 → 中文
        raw_check_result = str(_extract_field(result_rec, (
            "checkResult", "check_result", "timeResult", "result",
        )) or "") if result_rec else ""
        check_result_str = _translate_check_result(raw_check_result)

        # ── 打卡设备（OriginUserDevicePlug）：sourceType 枚举 → 中文 ────────
        raw_source_type = str(_extract_field(record, (
            "sourceType", "source_type", "deviceType", "device_type",
        )) or "")
        device_str = _translate_source_type(raw_source_type)

        row: dict[str, str] = {
            # 基础信息
            "姓名":   info.name or uid,
            "考勤组": group_name_map.get(uid, ""),
            "部门":   info.dept_name,

            # termId=8  考勤时间（OriginPlanCheckTimePlug）
            "考勤日期": _extract_date_str(record),
            "考勤时间": plan_time,

            # termId=9  打卡时间（OriginUserCheckTimePlug）
            "打卡时间": actual_time,

            # termId=10 打卡结果（OriginUserCheckResultPlug）
            "打卡结果": check_result_str,

            # termId=11 打卡地址（OriginUserLocationPlug）
            # Java 逻辑：spaceName + detailAddr 拼接
            "打卡地址": _extract_location(record),

            # termId=12 打卡备注（OriginUserRemarkPlug）
            # Java 逻辑：features.getRemark()
            "打卡备注": str(_extract_field(record, (
                "remark", "userRemark", "user_remark",
            )) or "").strip(),

            # termId=13 异常打卡原因（OriginCheckExceptionReasonPlug）
            # Java 逻辑：features.getInvalidRecordMsg() → 翻译错误码
            "异常打卡原因": _extract_exception_reason(record),

            # termId=14 打卡图片1（OriginRemarkPhotoPlug）
            # Java 逻辑：features.getRemarkPhotos()[0]
            "打卡图片1": _extract_remark_photo(record),

            # termId=15 打卡图片2（OriginFaceCheckPhotoPlug）
            # Java 逻辑：features.getFacePhoto()
            "打卡图片2": _extract_face_check_photo(record),

            # termId=16 打卡设备（OriginUserDevicePlug）
            # Java 逻辑：SourceType 枚举 → 中文
            "打卡设备": device_str,

            # termId=17 管理员修改备注（OriginBossCheckRemarkPlug）
            # Java 逻辑：features.getBossRemark()
            "管理员修改备注": _extract_boss_remark(record),

            # termId=18/19/20 管理员修改备注图片1/2/3（OriginBossCheckPhoto1/2/3Plug）
            # Java 逻辑：features.getBossPhotos()[0/1/2]
            "管理员修改备注图片1": _extract_boss_photo(record, 0),
            "管理员修改备注图片2": _extract_boss_photo(record, 1),
            "管理员修改备注图片3": _extract_boss_photo(record, 2),
        }
        rows.append(row)

    return rows


# ─────────────────────────────────────────────────────────────────────────────
# main
# ─────────────────────────────────────────────────────────────────────────────

def main() -> int:
    args = parse_args()

    # 1. 解析参数
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

    # 2. 解析 userId → 用户信息（使用 resolve_user_info，已适配 labels 职位提取）
    cmn.log(f"[users] 获取 {len(user_ids)} 个用户基础信息")
    user_info_map = cmn.resolve_user_info(user_ids)

    # 3. 切批 + 切片（明细用 100 人/批、31 天/片）
    user_batches = cmn.chunk_users(user_ids, size=CHECK_MAX_USERS_PER_BATCH)
    date_slices = cmn.slice_date_range(start, end, max_days=CHECK_MAX_DAYS_PER_SLICE)
    stats = cmn.CallStats(
        user_batches=len(user_batches),
        date_slices=len(date_slices),
    )
    cmn.log(f"[plan] 共 {len(user_batches)} 批 × {len(date_slices)} 个时间片")

    # 4. 拉数据：check record（打卡流水）+ check result（用于关联考勤时间和打卡结果）
    inspected_result_flag = [False]
    inspected_record_flag = [False]
    all_check_results: list[dict] = []
    all_check_records: list[dict] = []

    for batch_idx, batch in enumerate(user_batches, start=1):
        for slice_idx, date_slice in enumerate(date_slices, start=1):
            cmn.log(f"[batch {batch_idx}/{len(user_batches)}] "
                    f"[slice {slice_idx}/{len(date_slices)}]")

            results = query_check_results(
                batch, date_slice, stats,
                inspect=args.inspect, inspected_flag=inspected_result_flag,
            )
            all_check_results.extend(results)

            records = query_check_records(
                batch, date_slice, stats,
                inspect=args.inspect, inspected_flag=inspected_record_flag,
            )
            all_check_records.extend(records)

    cmn.log(f"[data] check result: {len(all_check_results)} 条, "
            f"check record: {len(all_check_records)} 条")

    if not all_check_records:
        stats.add_warning("查询完成，但未得到任何打卡流水记录")

    # 5. 获取考勤组信息（通过 group API 反向映射 userId → 考勤组名称）
    group_name_map = cmn.extract_group_names_from_records(all_check_records, user_ids)

    # 6. 构建明细行（以 check record 为主表，关联 check result 获取考勤时间和打卡结果）
    detail_rows = build_record_rows(
        all_check_records, all_check_results, user_info_map, group_name_map,
    )

    # 7. 写 Excel
    rows_2d = [[row.get(h, "") for h in ALL_HEADERS] for row in detail_rows]
    out_name = args.out or cmn.build_output_filename(start, end, suffix="detail")

    title = (
        f"考勤明细展示  统计日期：{start.strftime(cmn.DATE_FMT)} "
        f"至 {end.strftime(cmn.DATE_FMT)}"
    )
    subtitle = f"报表生成时间：{datetime.now().strftime('%Y-%m-%d %H:%M')}"

    # 图片嵌入参数：默认开启，--no-images 关闭
    image_columns = None if args.no_images else IMAGE_COLUMN_NAMES
    image_size = _parse_image_size(args.image_size)

    try:
        cmn.write_excel(
            out_name, ALL_HEADERS, rows_2d,
            sheet_name="考勤明细",
            title=title,
            subtitle=subtitle,
            image_columns=image_columns,
            image_size=image_size,
        )
    except RuntimeError as exc:
        cmn.error(str(exc))
        return 1

    # 8. 摘要
    cmn.print_summary(
        granularity_label="明细（打卡流水）",
        out_path=out_name,
        user_count=len(user_ids),
        column_names=CHECK_HEADERS,
        start=start,
        end=end,
        rows_count=len(rows_2d),
        stats=stats,
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())