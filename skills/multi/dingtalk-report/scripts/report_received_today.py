#!/usr/bin/env python3
"""
查看今天收到的日志列表及详情

基于 `dws report inbox list` + `dws report entry get`（旧的 report list / report detail
已废弃）。列表返回的 result[] 使用中文展示键；reportId 位于
_internalDetailCommands[].command，并与 result[] 按页同序对应。

用法:
    python report_received_today.py
    python report_received_today.py --days 3     # 最近 3 天
    python report_received_today.py --detail     # 额外拉取每条正文
    python report_received_today.py --dry-run
"""

import sys
import json
import subprocess
import argparse
from datetime import datetime, timedelta
from typing import List, Any, Optional, Tuple


def run_dws(
    args: List[str], dry_run: bool = False,
) -> Optional[Any]:
    cmd = ['dws'] + args
    if dry_run:
        print(f"[dry-run] {' '.join(cmd)}")
        return None
    try:
        result = subprocess.run(
            cmd, capture_output=True, text=True, timeout=60
        )
        if result.returncode != 0:
            print(f"错误：{result.stderr.strip()}", file=sys.stderr)
            return None
        return json.loads(result.stdout)
    except (subprocess.TimeoutExpired, json.JSONDecodeError,
            FileNotFoundError) as e:
        print(f"错误：{e}", file=sys.stderr)
        return None


def iso_start(dt: datetime) -> str:
    return dt.strftime('%Y-%m-%dT00:00:00+08:00')


def iso_end(dt: datetime) -> str:
    return dt.strftime('%Y-%m-%dT23:59:59+08:00')


def report_id_from_command(command: str) -> str:
    parts = command.split()
    if '--report-id' in parts:
        index = parts.index('--report-id')
        if index + 1 < len(parts):
            return parts[index + 1]
    return ''


def fetch_inbox(
    start: str, end: str, dry_run: bool,
) -> List[Tuple[dict, str]]:
    """按 cursor 拉取全部日志，返回 (展示项, reportId)。"""
    pairs: List[Tuple[dict, str]] = []
    cursor = 0
    while True:
        data = run_dws([
            'report', 'inbox', 'list',
            '--start', start,
            '--end', end,
            '--cursor', str(cursor),
            '--size', '20',
            '--format', 'json',
        ], dry_run=dry_run)
        if dry_run or not isinstance(data, dict):
            return pairs
        items = data.get('result') or []
        commands = data.get('_internalDetailCommands') or []
        for index, item in enumerate(items):
            if not isinstance(item, dict):
                continue
            report_id = ''
            if index < len(commands) and isinstance(commands[index], dict):
                report_id = report_id_from_command(
                    commands[index].get('command', '')
                )
            pairs.append((item, report_id))
        if data.get('hasMore') and data.get('nextCursor') is not None:
            cursor = data['nextCursor']
        else:
            break
    return pairs


def print_detail(report_id: str) -> None:
    detail = run_dws([
        'report', 'entry', 'get',
        '--report-id', report_id, '--format', 'json',
    ])
    if not isinstance(detail, dict):
        return
    result = detail.get('result')
    if not isinstance(result, dict):
        return
    contents = (result.get('report_content') or result.get('contents') or [])
    for content in contents[:3]:
        if not isinstance(content, dict):
            continue
        key = content.get('key') or content.get('title') or ''
        value = (content.get('value') or content.get('content')
                 or content.get('text') or '')
        if key and value:
            print(f"     {key}: {str(value).strip()[:60]}")


def main():
    parser = argparse.ArgumentParser(description='查看收到的日志')
    parser.add_argument(
        '--days', type=int, default=1, help='查询天数 (默认 1，即今天)'
    )
    parser.add_argument(
        '--detail', action='store_true', help='额外拉取每条正文'
    )
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()
    if args.days < 1:
        parser.error('--days must be >= 1')

    now = datetime.now()
    start_dt = now - timedelta(days=args.days - 1)
    start = iso_start(start_dt)
    end = iso_end(now)

    label = '今天' if args.days == 1 else f'最近 {args.days} 天'
    print(f'查看{label}收到的日志...\n')

    pairs = fetch_inbox(start, end, args.dry_run)
    if args.dry_run:
        return
    if not pairs:
        print('  暂无收到的日志')
        return

    print(f"{label}日志 ({len(pairs)} 条)")
    print('=' * 50)

    for item, report_id in pairs:
        title = item.get('标题') or '日志'
        sender = item.get('发送人') or '未知'
        date = item.get('日期') or ''
        status = item.get('状态') or ''
        link = item.get('钉钉链接') or ''

        print(f"\n  {title} - {sender}")
        print(f"     时间: {date}")
        if status:
            print(f"     状态: {status}")
        if link:
            print(f"     链接: {link}")
        if args.detail and report_id:
            print_detail(report_id)


if __name__ == '__main__':
    main()
