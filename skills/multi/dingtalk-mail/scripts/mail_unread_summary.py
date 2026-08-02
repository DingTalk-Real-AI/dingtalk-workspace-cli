#!/usr/bin/env python3
"""
查询今天未读邮件并汇总（自动获取邮箱地址）

用法:
    python mail_unread_summary.py
    python mail_unread_summary.py --size 30
    python mail_unread_summary.py --dry-run
"""

import sys
import json
import subprocess
import argparse
from datetime import datetime, timezone, timedelta
from typing import List, Any, Optional

TZ = timezone(timedelta(hours=8))


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


def get_my_email(dry_run: bool = False) -> Optional[str]:
    data = run_dws([
        'mail', 'mailbox', 'list', '--format', 'json',
    ], dry_run=dry_run)
    if dry_run:
        return '<MY_EMAIL>'
    if not data:
        return None
    if isinstance(data, list) and data:
        item = data[0]
        return (item.get('email') or item.get('address')
                if isinstance(item, dict) else str(item))
    if isinstance(data, dict):
        return data.get('email') or data.get('address')
    return None


def main():
    parser = argparse.ArgumentParser(
        description='查询今天未读邮件'
    )
    parser.add_argument(
        '--size', type=int, default=20, help='返回数量'
    )
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()

    print('📬 获取邮箱地址...')
    email = get_my_email(dry_run=args.dry_run)
    if not email and not args.dry_run:
        print('错误：无法获取邮箱地址')
        sys.exit(1)

    today = datetime.now(TZ).strftime('%Y-%m-%dT00:00:00Z')
    kql = f'isRead:false AND date>{today}'

    print(f'🔍 搜索未读邮件...\n')
    data = run_dws([
        'mail', 'message', 'search',
        '--email', email or '<MY_EMAIL>',
        '--query', kql,
        '--size', str(args.size),
        '--format', 'json',
    ], dry_run=args.dry_run)

    if args.dry_run:
        return
    if not data:
        print('未查到邮件')
        return

    messages = (data if isinstance(data, list)
                else data.get('items', data.get('messages', [])))

    print(f"📧 今日未读邮件")
    print('=' * 50)
    if not messages:
        print('  ✅ 收件箱清空，没有未读邮件！')
        return

    for m in messages:
        subj = m.get('subject', '(无主题)')
        sender = m.get('from', {})
        sender_name = (sender.get('name') or sender.get('email', '未知')
                       if isinstance(sender, dict) else str(sender))
        print(f"  📩 {subj}")
        print(f"     发件人: {sender_name}")

    print(f"\n合计: {len(messages)} 封未读邮件")


if __name__ == '__main__':
    main()
