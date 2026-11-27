#!/usr/bin/env python3
"""汇总本人拥有的钉钉、邮箱日历，先发现日历本，再逐本完整翻页。

用法:
    python calendar_today_agenda.py [today|yesterday|tomorrow|week]
    python calendar_today_agenda.py --start 2026-11-24T00:00:00+08:00 --end 2026-11-25T00:00:00+08:00
    python calendar_today_agenda.py yesterday --profile CORP_ID:USER_ID --format json
    python calendar_today_agenda.py today --calendar-id CALENDAR_ID
    python calendar_today_agenda.py --dry-run

默认文本输出；JSON 的 complete 只表示所列日历与时间窗完整，不代表所有可访问日历。
退出码：0 完整，1 查询不完整/失败，2 参数错误。dry-run 不执行任何 dws 请求。
"""

import argparse
import json
import shlex
import subprocess
import sys
from datetime import datetime, timedelta, timezone

TZ = timezone(timedelta(hours=8), name="Asia/Shanghai")
MAX_PAGES = 20
CANCELLED = {"cancelled", "canceled", "deleted"}
ACTIVE = {"confirmed", "normal", "active"}


class QueryError(RuntimeError):
    pass


def text(value):
    return value.strip() if isinstance(value, str) else ""


def parse_iso(value):
    if not isinstance(value, str) or "T" not in value:
        raise ValueError("时间必须是带时区的 ISO-8601 日期时间")
    dt = datetime.fromisoformat(value.replace("Z", "+00:00"))
    if dt.tzinfo is None or dt.utcoffset() is None:
        raise ValueError("时间必须带时区")
    return dt


def get_range(scope, now=None):
    current = (now or datetime.now(TZ)).astimezone(TZ)
    today = current.replace(hour=0, minute=0, second=0, microsecond=0)
    if scope == "week":
        start = today - timedelta(days=today.weekday())
        return start, start + timedelta(days=7)
    start = today + timedelta(days={"today": 0, "yesterday": -1, "tomorrow": 1}[scope])
    return start, start + timedelta(days=1)


def command(args, profile):
    return ["dws"] + (["--profile", profile] if profile else []) + args + ["--format", "json"]


def run_dws(args, profile=None):
    try:
        proc = subprocess.run(command(args, profile), capture_output=True, text=True, timeout=60)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise QueryError(str(exc)) from exc
    if proc.returncode != 0:
        raise QueryError(proc.stderr.strip() or "dws exited " + str(proc.returncode))
    try:
        payload = json.loads(proc.stdout)
    except json.JSONDecodeError as exc:
        raise QueryError("dws 返回非 JSON 内容") from exc
    # These atomic Calendar commands currently deliver the legacy success/result
    # envelope. Unknown shapes fail closed instead of looking like empty results.
    if not isinstance(payload, dict) or payload.get("success") is not True:
        raise QueryError("日历请求未返回 success=true")
    if "result" not in payload:
        raise QueryError("日历请求缺少 result")
    return payload["result"]


def fail(report, stage, message, calendar_id=None):
    entry = {"stage": stage, "message": message}
    if calendar_id is not None:
        entry["calendarId"] = calendar_id
    report["failures"].append(entry)


def select_calendars(rows, requested, report):
    if not isinstance(rows, list):
        raise QueryError("日历本 result 不是数组")
    books, conflicts = {}, set()
    for row in rows:
        if not isinstance(row, dict) or not text(row.get("calendarId")):
            fail(report, "discovery", "日历本条目缺少合法 calendarId")
            continue
        cid = text(row["calendarId"])
        book = {key: text(row.get(key)) for key in ("calendarId", "summary", "type", "privilege")}
        previous = books.get(cid)
        if previous and (previous["type"], previous["privilege"]) != (book["type"], book["privilege"]):
            conflicts.add(cid)
            fail(report, "discovery", "同一日历本的类型或归属信息冲突", cid)
        books.setdefault(cid, book)

    selected = []
    for cid in dict.fromkeys(requested or books):
        if cid in conflicts:
            continue
        book = books.get(cid)
        if book is None:
            fail(report, "selection", "指定日历本不在本次发现结果中", cid)
            continue
        kind, role = book["type"], book["privilege"]
        if requested:
            if role not in ("owner", "writer", "reader"):
                fail(report, "selection", "不能确认指定日历本可读取", cid)
                continue
        else:
            if kind in ("subscribed", "team", "public"):
                continue
            if role in ("reader", "writer", "freebusy", "none"):
                continue
            # Owned mailbox calendars can be shared; also check ownership.
            if kind not in ("primary", "shared") or role != "owner":
                fail(report, "selection", "不能确认日历本属于本人日历，未按名称或 ID 猜测", cid)
                continue
        selected.append(dict(book, complete=False, pages=0))
    if not selected:
        fail(report, "selection", "未发现可确认的目标日历本，不能判断是否有日程")
    if not requested and len(selected) == 1:
        report["notes"].append("本次仅纳入 1 本可确认的本人日历；不能据此确认钉钉和邮箱日历均已覆盖，结果仅覆盖所列日历。")
    report["notes"].append("查询范围以本次日历本列表为准；未返回的日历不代表不存在，未查询的日历不计入汇总。")
    return selected


def event_time(event, key):
    value = event.get(key)
    if isinstance(value, dict):
        if text(value.get("dateTime")):
            value = value["dateTime"]
        elif text(value.get("date")):
            return datetime.strptime(value["date"], "%Y-%m-%d").replace(tzinfo=TZ), True
        else:
            raise ValueError(key + " 缺少 dateTime/date")
    if value is None:
        value = event.get(key + "Time") or event.get(key + "DateTime")
    return parse_iso(value).astimezone(TZ), False


def normalize_event(raw, book):
    if not isinstance(raw, dict):
        raise ValueError("日程条目不是对象")
    eid = text(raw.get("id")) or text(raw.get("eventId"))
    if not eid:
        raise ValueError("日程缺少 id/eventId")
    start, start_date = event_time(raw, "start")
    end, end_date = event_time(raw, "end")
    if end <= start or start_date != end_date:
        raise ValueError("日程起止时间无效")
    status = text(raw.get("status") or raw.get("eventStatus") or raw.get("event_status"))
    state = "cancelled" if status.lower() in CANCELLED else "active" if status.lower() in ACTIVE else "unknown"
    source = {
        "calendarId": book["calendarId"], "calendarName": book["summary"],
        "eventId": eid, "status": status, "state": state,
        "responseStatus": raw.get("responseStatus"),
    }
    event = dict(raw)
    event.update({
        "eventId": eid, "title": text(raw.get("summary")) or text(raw.get("title")) or "无标题",
        "start": start.isoformat(), "end": end.isoformat(),
        "isAllDay": start_date or raw.get("isAllDay") is True,
        "status": status, "state": state, "sources": [source],
    })
    return event


def page_events(result):
    if not isinstance(result, dict) or not isinstance(result.get("events"), list):
        raise QueryError("日程 result 缺少 events 数组")
    return result["events"]


def next_cursor(result):
    more = result.get("hasMore")
    cursor = result.get("nextCursor")
    if not isinstance(more, bool):
        raise QueryError("分页缺少布尔 hasMore 终态")
    if cursor is not None and not isinstance(cursor, str):
        raise QueryError("nextCursor 不是字符串")
    cursor = text(cursor)
    if more != bool(cursor):
        raise QueryError("hasMore 与 nextCursor 矛盾，不能证明列表完整")
    return cursor


def event_args(cid, start, end, cursor=""):
    args = ["calendar", "event", "list", "--calendar-id", cid,
            "--start", start.isoformat(), "--end", end.isoformat(), "--limit", "100"]
    return args + (["--cursor", cursor] if cursor else [])


def collect_events(book, start, end, profile, report):
    cid = book["calendarId"]
    cursor, seen = "", set()
    failures_before = len(report["failures"])
    for _ in range(MAX_PAGES):
        try:
            result = run_dws(event_args(cid, start, end, cursor), profile)
            rows = page_events(result)
            book["pages"] += 1
            for index, row in enumerate(rows):
                try:
                    event = normalize_event(row, book)
                    # Keep overlapping events, but exclude an event that begins
                    # exactly at the next day's boundary if the API includes it.
                    if parse_iso(event["start"]) < end and parse_iso(event["end"]) > start:
                        report["events"].append(event)
                except (ValueError, TypeError, OverflowError) as exc:
                    fail(report, "event", "第 {} 页第 {} 条：{}".format(book["pages"], index + 1, exc), cid)
            following = next_cursor(result)
            if not following:
                book["complete"] = len(report["failures"]) == failures_before
                return
            if following in seen:
                raise QueryError("分页游标重复，停止查询")
            seen.add(following)
            cursor = following
        except QueryError as exc:
            fail(report, "events", str(exc), cid)
            return
    fail(report, "events", "达到每本日历 20 页上限，尚未取完", cid)


def merge_events(events):
    merged, local, shared = [], {}, {}
    for event in events:
        source = event["sources"][0]
        cid = source["calendarId"]
        times = (event["start"], event["end"], event["isAllDay"])
        local_key = (cid, event["eventId"]) + times
        uid = text(event.get("iCalUid"))
        shared_key = (uid,) + times
        target = local.get(local_key)
        if target is None and uid:
            target = next((item for item in shared.get(shared_key, [])
                           if all(s["calendarId"] != cid for s in item["sources"])), None)
        if target is None:
            target = event
            merged.append(target)
            if uid:
                shared.setdefault(shared_key, []).append(target)
        elif source not in target["sources"]:
            target["sources"].append(source)
        local[local_key] = target
        states = {item["state"] for item in target["sources"]}
        target["state"] = next(iter(states)) if len(states) == 1 else "unknown"
        statuses = {item["status"] for item in target["sources"]}
        target["status"] = next(iter(statuses)) if len(statuses) == 1 else "conflicting"
    return sorted(merged, key=lambda event: (event["start"], event["end"], event["title"], event["eventId"]))


def collect(start, end, profile=None, requested=None):
    report = {
        "complete": False, "scope": "specified_calendars" if requested else "discovered_own_primary_calendars",
        "range": {"start": start.isoformat(), "end": end.isoformat(), "timezone": "Asia/Shanghai"},
        "calendars": [], "events": [], "failures": [], "notes": [],
    }
    try:
        rows = run_dws(["calendar", "book", "list"], profile)
        report["calendars"] = select_calendars(rows, requested, report)
    except QueryError as exc:
        fail(report, "discovery", str(exc))
    for book in report["calendars"]:
        collect_events(book, start, end, profile, report)
    report["events"] = merge_events(report["events"])
    report["complete"] = bool(report["calendars"]) and not report["failures"]
    report["counts"] = {state: sum(e["state"] == state for e in report["events"])
                        for state in ("active", "cancelled", "unknown")}
    report["counts"]["total"] = len(report["events"])
    return report


def render_text(report):
    print("日程范围：{} ~ {}（北京时间，结束边界不含）".format(
        report["range"]["start"], report["range"]["end"]))
    print("查询{}；仅覆盖以下日历：".format("完整" if report["complete"] else "不完整"))
    for book in report["calendars"]:
        print("- {} [{}]：{}，{} 页".format(book["summary"] or book["calendarId"],
              book["calendarId"], "完整" if book["complete"] else "不完整", book["pages"]))
    for note in report["notes"]:
        print("说明：" + note)
    for event in report["events"]:
        status = {"active": "有效", "cancelled": "已取消", "unknown": "状态未确认"}[event["state"]]
        names = list(dict.fromkeys(s["calendarName"] or s["calendarId"] for s in event["sources"]))
        location = event.get("location")
        if isinstance(location, dict):
            location = location.get("displayName", "")
        suffix = "；地点：" + location if isinstance(location, str) and location else ""
        print("{} ~ {}{}  {} [{}；{}]{}".format(
            event["start"], event["end"], "（全天）" if event["isAllDay"] else "",
            event["title"], status, " / ".join(names), suffix))
    if not report["events"]:
        print("所列日历在该时间窗内暂无日程。" if report["complete"]
              else "尚未取得日程，查询不完整，不能判断是否有日程。")
    counts = report["counts"]
    print("{}：{} 项有效、{} 项已取消、{} 项状态未确认（去重后 {} 项）。".format(
        "所列日历合计" if report["complete"] else "已取得的部分结果",
        counts["active"], counts["cancelled"], counts["unknown"], counts["total"]))
    for error in report["failures"]:
        print("未完成 [{}]：{}".format(error.get("calendarId", error["stage"]), error["message"]))


def run(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("scope", nargs="?", choices=("today", "yesterday", "tomorrow", "week"))
    parser.add_argument("--start")
    parser.add_argument("--end")
    parser.add_argument("--profile", help="单个组织/账号，原样透传至每条 dws 命令")
    parser.add_argument("--calendar-id", action="append", help="仅查询明确指定且发现列表中存在的日历本，可重复")
    parser.add_argument("--format", choices=("text", "json"), default="text")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)
    if args.profile is not None and (not args.profile.strip() or "," in args.profile):
        parser.error("--profile 必须指定单个账号，不支持多账号 CSV")
    if args.calendar_id and any(not cid.strip() for cid in args.calendar_id):
        parser.error("--calendar-id 不得为空")
    has_start, has_end = args.start is not None, args.end is not None
    if has_start != has_end or (has_start and args.scope):
        parser.error("--start/--end 必须成对，且不能与日期预设同时使用")
    try:
        if has_start:
            start, end = parse_iso(args.start).astimezone(TZ), parse_iso(args.end).astimezone(TZ)
        else:
            start, end = get_range(args.scope or "today")
        if end <= start:
            raise ValueError("end 必须晚于 start")
    except ValueError as exc:
        parser.error(str(exc))
    if args.dry_run:
        preview = {
            "dryRun": True,
            "discoveryCommand": command(["calendar", "book", "list"], args.profile),
            "eventCommandTemplate": command(event_args("<calendarId-from-book-list>", start, end), args.profile),
            "requestedCalendarIds": args.calendar_id or [],
            "instructions": "先发现一次日历本；默认选 privilege=owner 且 type 为 primary 或 shared；逐本翻页，后续页追加 --cursor <nextCursor>，每本最多 20 页。占位符必须替换为真实返回值。",
        }
        if args.format == "json":
            print(json.dumps(preview, ensure_ascii=False, indent=2))
        else:
            print("[dry-run] " + shlex.join(preview["discoveryCommand"]))
            print("[dry-run template] " + shlex.join(preview["eventCommandTemplate"]))
            print(preview["instructions"])
        return 0
    report = collect(start, end, args.profile,
                     [cid.strip() for cid in args.calendar_id] if args.calendar_id else None)
    if args.format == "json":
        print(json.dumps(report, ensure_ascii=False, indent=2))
    else:
        render_text(report)
    return 0 if report["complete"] else 1


if __name__ == "__main__":
    sys.exit(run())
