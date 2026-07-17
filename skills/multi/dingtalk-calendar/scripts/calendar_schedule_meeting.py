#!/usr/bin/env python3
"""
一键创建日程（可选：带参与者 + 预定空闲会议室）

流程：
  1. 若需预定会议室 (--book-room)，先搜索空闲会议室；无可用则提前报错退出
  2. 使用 event create 一次性完成日程创建 + 添加参与者 + 预定会议室

用法:
    python calendar_schedule_meeting.py \
        --title "Q1 复盘会" \
        --start "2026-03-15T14:00" \
        --end "2026-03-15T15:00" \
        --users userId1,userId2 \
        --book-room

    python calendar_schedule_meeting.py --dry-run \
        --title "测试" --start "2026-03-15T14:00" --end "2026-03-15T15:00"
"""

import sys
import json
import subprocess
import argparse
from datetime import datetime, timedelta, timezone
from typing import List, Any, Optional, Tuple

TZ = timezone(timedelta(hours=8))


def run_dws(
    args: List[str], dry_run: bool = False,
) -> Optional[Any]:
    cmd = ['dws'] + args
    if dry_run:
        print(f"[dry-run] {' '.join(cmd)}")
        return {'dry_run': True}
    try:
        result = subprocess.run(
            cmd, capture_output=True, text=True, timeout=60
        )
        if result.returncode != 0:
            print(f"  ✗ 错误：{result.stderr.strip()}")
            return None
        return json.loads(result.stdout)
    except (subprocess.TimeoutExpired, json.JSONDecodeError,
            FileNotFoundError) as e:
        print(f"  ✗ 错误：{e}")
        return None


def normalize_time(time_str: str) -> str:
    for fmt in ('%Y-%m-%dT%H:%M', '%Y-%m-%d %H:%M',
                '%Y-%m-%dT%H:%M:%S'):
        try:
            dt = datetime.strptime(time_str, fmt)
            dt = dt.replace(tzinfo=TZ)
            return dt.strftime('%Y-%m-%dT%H:%M:%S+08:00')
        except ValueError:
            continue
    if '+' in time_str or time_str.endswith('Z'):
        return time_str
    raise ValueError(f"无法解析时间：{time_str}")


def parse_group_ids(raw: str) -> List[str]:
    if raw is None:
        return []
    return [part.strip() for part in raw.split(',') if part.strip()]


def extract_room_candidates(payload: Any) -> Tuple[List[dict], str]:
    candidates: Any = []
    if isinstance(payload, list):
        candidates = payload
    elif isinstance(payload, dict):
        if isinstance(payload.get('rooms'), list):
            candidates = payload.get('rooms', [])
        elif isinstance(payload.get('result'), dict):
            nested = payload.get('result', {})
            if isinstance(nested.get('rooms'), list):
                candidates = nested.get('rooms', [])
            elif isinstance(nested.get('result'), list):
                candidates = nested.get('result', [])
        elif isinstance(payload.get('result'), list):
            candidates = payload.get('result', [])

    if not isinstance(candidates, list):
        return [], '返回结构中未找到会议室列表'

    valid_rooms: List[dict] = []
    placeholder_count = 0
    for item in candidates:
        if not isinstance(item, dict):
            continue
        if item.get('roomId') or item.get('id'):
            valid_rooms.append(item)
            continue
        if item.get('labels') is None and len(item) == 1:
            placeholder_count += 1

    if valid_rooms:
        return valid_rooms, f'返回 {len(valid_rooms)} 个有效会议室'
    if placeholder_count:
        return [], '仅返回占位结果（如 labels:null），无有效 roomId'
    if candidates:
        return [], '返回了对象列表，但均不含有效 roomId'
    return [], '未返回任何会议室'


def main():
    parser = argparse.ArgumentParser(
        description='一键创建日程（可选：带参与者 + 预定会议室）'
    )
    parser.add_argument('--title', required=True, help='日程标题')
    parser.add_argument('--start', required=True, help='开始时间')
    parser.add_argument('--end', required=True, help='结束时间')
    parser.add_argument('--desc', default='', help='日程描述')
    parser.add_argument('--users', default='', help='参与者 userId，逗号分隔')
    parser.add_argument(
        '--book-room', action='store_true', help='自动搜索并预定空闲会议室'
    )
    parser.add_argument(
        '--room-group-id', default='',
        help='允许搜索的 groupId；同一地点请只传最相关 group，多个仅用于用户明确允许的多个地点'
    )
    parser.add_argument(
        '--dry-run', action='store_true', help='仅显示命令'
    )
    args = parser.parse_args()

    try:
        start_iso = normalize_time(args.start)
        end_iso = normalize_time(args.end)
    except ValueError as e:
        print(f"错误：{e}")
        sys.exit(1)

    # ── Step 1: 若需预定会议室，先搜索空闲会议室 ──────────────────
    room_id: Optional[str] = None
    room_name: Optional[str] = None

    if args.book_room:
        print('🏢 搜索空闲会议室...')
        group_ids = parse_group_ids(args.room_group_id)
        search_scopes = group_ids or [None]
        selected_room = None
        failure_reasons: List[str] = []

        for group_id in search_scopes:
            scope_label = f'group {group_id}' if group_id else '根目录'
            print(f'  - 查询范围: {scope_label}')
            search_args = [
                'calendar', 'room', 'search',
                '--start', start_iso,
                '--end', end_iso,
                '--format', 'json',
            ]
            if group_id:
                search_args.extend(['--group-id', group_id])
            rooms_data = run_dws(search_args, dry_run=args.dry_run)

            if args.dry_run:
                continue
            if not rooms_data:
                failure_reasons.append(f'{scope_label}: room search 执行失败')
                continue

            rooms, detail = extract_room_candidates(rooms_data)
            if rooms:
                selected_room = rooms[0]
                break
            failure_reasons.append(f'{scope_label}: {detail}')

        if not args.dry_run:
            if selected_room:
                room_id = selected_room.get('roomId') or selected_room.get('id')
                room_name = selected_room.get('roomName') or selected_room.get('name')
                print(f'  ✓ 找到空闲会议室: {room_name} ({room_id})')
            else:
                print(f'  ✗ {start_iso} ~ {end_iso} 时段内无可用会议室')
                for reason in failure_reasons:
                    print(f'    - {reason}')
                print('  请向用户汇报失败，或询问是否放宽范围/改时间。')
                sys.exit(2)

    # ── Step 2: 一次性创建日程（含参与者 + 会议室） ────────────────
    print('\n📅 创建日程...')
    create_args = [
        'calendar', 'event', 'create',
        '--title', args.title,
        '--start', start_iso,
        '--end', end_iso,
        '--format', 'json',
    ]
    if args.desc:
        create_args.extend(['--desc', args.desc])
    if args.users:
        create_args.extend(['--attendees', args.users])
    if room_id:
        create_args.extend(['--rooms', str(room_id)])

    result = run_dws(create_args, dry_run=args.dry_run)
    if not result:
        sys.exit(1)

    # 解析响应
    event_id = None
    if not args.dry_run and isinstance(result, dict):
        # MCP 响应通常嵌套在 result 字段内: {"result": {"id": "..."}}
        inner = result.get('result', result)
        if isinstance(inner, dict):
            event_id = inner.get('eventId') or inner.get('id')
        else:
            event_id = result.get('eventId') or result.get('id')

    # 输出结果摘要
    parts = []
    if event_id:
        parts.append(f'eventId: {event_id}')
    if args.users:
        parts.append(f'参与者: {args.users}')
    if room_name:
        parts.append(f'会议室: {room_name}')
    detail_str = f" ({', '.join(parts)})" if parts else ''
    print(f'  ✓ 日程已创建{detail_str}')

    print('\n✅ 完成!')


if __name__ == '__main__':
    main()
