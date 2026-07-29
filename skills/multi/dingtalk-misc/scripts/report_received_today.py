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


def main():
    parser = argparse.ArgumentParser(
        description='查看收到的日志'
    )
    parser.add_argument(
        '--days', type=int, default=1, help='查询天数 (默认 1)'
    )
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()
    if args.days < 1:
        parser.error('--days must be >= 1')

    now = datetime.now()
    start = now - timedelta(days=args.days - 1)
    start = start.replace(hour=0, minute=0, second=0, microsecond=0)
    end = now.replace(hour=23, minute=59, second=59, microsecond=0)

    label = '今天' if args.days == 1 else f'最近 {args.days} 天'
    print(f'📓 查看{label}收到的日志...\n')

    data = run_dws([
        'report', 'inbox', 'list',
        '--start', to_iso(start),
        '--end', to_iso(end),
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
                                inner.get('reports', []))
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
        rid = r.get('reportId') or r.get('id', '')
        creator = r.get('creatorName') or r.get('creator', '未知')
        template = r.get('templateName') or r.get('template', '')
        create_time = r.get('createTime', '')
        if isinstance(create_time, (int, float)):
            create_time = datetime.fromtimestamp(
                create_time / 1000
            ).strftime('%Y-%m-%d %H:%M')

        print(f"\n  📝 {template or '日志'} - {creator}")
        print(f"     时间: {create_time}")
        print(f"     ID: {rid}")

        if rid:
            detail = run_dws([
                'report', 'entry', 'get',
                '--report-id', rid, '--format', 'json',
            ])
            if detail and isinstance(detail, dict):
                result = detail.get('result', detail)
                if isinstance(result, dict):
                    contents = (
                        result.get('contents')
                        or result.get('report_content')
                        or result.get('reportContent')
                        or result.get('reportContents')
                        or []
                    )
                else:
                    contents = []
                for c in contents[:3]:
                    if not isinstance(c, dict):
                        continue
                    key = c.get('key') or c.get('title', '')
                    val = (
                        c.get('value')
                        or c.get('content')
                        or c.get('text')
                        or ''
                    )
                    if key and val:
                        print(f"     {key}: {str(val)[:60]}")


if __name__ == '__main__':
    main()
