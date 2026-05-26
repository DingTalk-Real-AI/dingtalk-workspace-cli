#!/usr/bin/env python3
"""
用机器人向多个群批量发送相同消息（如日报提醒）

用法:
    python bot_broadcast.py \
        --robot-code <ROBOT_CODE> \
        --chats "conv_id1,conv_id2,conv_id3" \
        --title "日报提醒" \
        --text "请大家今天下班前提交日报"

    python bot_broadcast.py \
        --robot-code <ROBOT_CODE> \
        --chats-file groups.txt \
        --title "周会通知" \
        --text "明天下午3点周会"

    python bot_broadcast.py --dry-run ...
"""

import sys
import json
import subprocess
import argparse
from pathlib import Path
from typing import List, Any, Optional


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


def main():
    parser = argparse.ArgumentParser(
        description='向多个群批量发送机器人消息'
    )
    parser.add_argument(
        '--robot-code', required=True, help='机器人 Code'
    )
    parser.add_argument('--chats', default='', help='会话 ID 列表')
    parser.add_argument(
        '--chats-file', default='',
        help='会话 ID 文件 (每行一个)',
    )
    parser.add_argument('--title', required=True, help='消息标题')
    parser.add_argument(
        '--text', required=True, help='消息内容 Markdown'
    )
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()

    chat_ids: List[str] = []
    if args.chats:
        chat_ids = [c.strip() for c in args.chats.split(',')
                    if c.strip()]
    elif args.chats_file:
        p = Path(args.chats_file)
        if not p.exists():
            print(f"错误：文件不存在: {p}")
            sys.exit(1)
        chat_ids = [line.strip() for line in
                    p.read_text(encoding='utf-8').splitlines()
                    if line.strip() and not line.startswith('#')]
    if not chat_ids:
        print('错误：需要 --chats 或 --chats-file')
        sys.exit(1)

    print(f"📢 批量发送消息到 {len(chat_ids)} 个群")
    print(f"   标题: {args.title}")
    print(f"   机器人: {args.robot_code}")
    print('=' * 50)

    success, fail = 0, 0
    for i, chat_id in enumerate(chat_ids, 1):
        result = run_dws([
            'chat', 'message', 'send-by-bot',
            '--robot-code', args.robot_code,
            '--group', chat_id,
            '--title', args.title,
            '--text', args.text,
            '--format', 'json',
        ], dry_run=args.dry_run)
        if result:
            print(f"  ✓ [{i}/{len(chat_ids)}] {chat_id}")
            success += 1
        else:
            print(f"  ✗ [{i}/{len(chat_ids)}] {chat_id}")
            fail += 1

    print(f"\n完成: 成功 {success}, 失败 {fail}")
    sys.exit(0 if fail == 0 else 1)


if __name__ == '__main__':
    main()
