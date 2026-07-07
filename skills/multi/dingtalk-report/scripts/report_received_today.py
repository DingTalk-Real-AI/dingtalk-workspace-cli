#!/usr/bin/env python3
"""
查看今天收到的日志列表及详情

用法:
    python report_received_today.py
    python report_received_today.py --days 3    # 最近 3 天
    python report_received_today.py --dry-run
"""

import sys
import json
import subprocess
import argparse
from datetime import datetime, timedelta
from typing import List, Any, Optional
from urllib.parse import parse_qs, urlparse


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


def to_iso(dt: datetime) -> str:
    return dt.strftime('%Y-%m-%dT%H:%M:%S+08:00')


def first_value(data: dict, *keys: str, default: str = '') -> str:
    for key in keys:
        value = data.get(key)
        if value not in (None, ''):
            return str(value)
    return default


def report_id_from_link(link: str) -> str:
    if not link:
        return ''
    query = parse_qs(urlparse(link).query)
    for key in ('reportId', 'report_id', 'id'):
        values = query.get(key)
        if values:
            return values[0]
    return ''


def main():
    parser = argparse.ArgumentParser(
        description='查看收到的日志'
    )
    parser.add_argument(
        '--days', type=int, default=1, help='查询天数 (默认 1)'
    )
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()

    now = datetime.now()
    start = now - timedelta(days=args.days)
    start = start.replace(hour=0, minute=0, second=0)

    label = '今天' if args.days == 1 else f'最近 {args.days} 天'
    print(f'📓 查看{label}收到的日志...\n')

    data = run_dws([
        'report', 'list',
        '--start', to_iso(start),
        '--end', to_iso(now),
        '--cursor', '0',
        '--size', '20',
        '--format', 'json',
    ], dry_run=args.dry_run)

    if args.dry_run:
        return
    if not data:
        print('未查到日志')
        return

    if isinstance(data, list):
        reports = data
    elif isinstance(data, dict):
        inner = data.get('result', data)
        if isinstance(inner, dict):
            reports = inner.get('report_list',
                                inner.get('reports',
                                          inner.get('items', [])))
        elif isinstance(inner, list):
            reports = inner
        else:
            reports = []
    else:
        reports = []
    if not reports:
        print('  ✅ 暂无收到的日志')
        return

    print(f"📓 {label}日志 ({len(reports)} 条)")
    print('=' * 50)

    for r in reports:
        if not isinstance(r, dict):
            print(f"\n  📝 {r}")
            continue
        link = first_value(r, '钉钉链接', 'dingTalkUrl', 'url')
        rid = first_value(r, 'reportId', 'id') or report_id_from_link(link)
        creator = first_value(r, 'creatorName', 'creator', '发送人', default='未知')
        template = first_value(r, 'templateName', 'template', '标题')
        create_time = r.get('createTime') or r.get('日期') or ''
        if isinstance(create_time, (int, float)):
            create_time = datetime.fromtimestamp(
                create_time / 1000
            ).strftime('%Y-%m-%d %H:%M')

        print(f"\n  📝 {template or '日志'} - {creator}")
        print(f"     时间: {create_time}")
        print(f"     ID: {rid}")

        if rid:
            detail = run_dws([
                'report', 'detail',
                '--report-id', rid, '--format', 'json',
            ])
            if detail and isinstance(detail, dict):
                contents = detail.get('contents', [])
                for c in contents[:3]:
                    key = c.get('key') or c.get('title', '')
                    val = c.get('value') or c.get('content', '')
                    if key and val:
                        print(f"     {key}: {str(val)[:60]}")


if __name__ == '__main__':
    main()
