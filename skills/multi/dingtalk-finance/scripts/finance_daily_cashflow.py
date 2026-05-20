#!/usr/bin/env python3
"""
查看指定日期现金日报

用法:
    python finance_daily_cashflow.py                    # 今天
    python finance_daily_cashflow.py --date 2026-03-10
    python finance_daily_cashflow.py --dry-run
"""

import sys
import json
import subprocess
import argparse
from datetime import datetime
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


def main():
    parser = argparse.ArgumentParser(
        description='查看现金日报'
    )
    parser.add_argument(
        '--date', default='', help='日期 YYYY-MM-DD (默认今天)'
    )
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()

    date_str = args.date or datetime.now().strftime('%Y-%m-%d')

    print(f'💰 现金日报 ({date_str})\n')
    data = run_dws([
        'finance', 'journal', 'daily',
        '--date', date_str,
        '--format', 'json',
    ], dry_run=args.dry_run)

    if args.dry_run:
        return
    if not data:
        print('未查到现金日报')
        return

    print('=' * 50)
    print(json.dumps(data, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
