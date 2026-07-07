#!/usr/bin/env python3
"""
导出群聊消息到 JSON 文件（从指定时间点拉取）

用法:
    python chat_export_messages.py \
        --group <openconversation_id> \
        --time "2026-03-10 00:00:00" \
        --output messages.json

    python chat_export_messages.py \
        --query "项目冲刺" \
        --time "2026-03-10 00:00:00" \
        --no-forward --limit 100
"""

import sys
import json
import subprocess
import argparse
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
            cmd, capture_output=True, text=True, timeout=120
        )
        if result.returncode != 0:
            print(f"错误：{result.stderr.strip()}", file=sys.stderr)
            return None
        return json.loads(result.stdout)
    except (subprocess.TimeoutExpired, json.JSONDecodeError,
            FileNotFoundError) as e:
        print(f"错误：{e}", file=sys.stderr)
        return None


def search_group(
    query: str, dry_run: bool = False,
) -> Optional[str]:
    data = run_dws([
        'chat', 'search',
        '--query', query, '--format', 'json',
    ], dry_run=dry_run)
    if dry_run:
        return '<CONV_ID>'
    if not data:
        return None
    if isinstance(data, list):
        groups = data
    elif isinstance(data, dict):
        inner = data.get('result', data)
        if isinstance(inner, dict):
            groups = inner.get('items', inner.get('groups', []))
        elif isinstance(inner, list):
            groups = inner
        else:
            groups = []
    else:
        groups = []
    if not groups:
        print(f"未找到群聊: {query}")
        return None
    g = groups[0]
    name = g.get('title') or g.get('name', '未知')
    conv_id = g.get('openConversationId') or g.get('id')
    print(f"  找到群聊: {name} ({conv_id})")
    return conv_id


def main():
    parser = argparse.ArgumentParser(
        description='导出群聊消息到 JSON'
    )
    parser.add_argument('--group', help='群聊 openconversation_id')
    parser.add_argument('--query', help='按群名搜索')
    parser.add_argument(
        '--time', required=True,
        help='起始时间 yyyy-MM-dd HH:mm:ss',
    )
    parser.add_argument(
        '--no-forward', action='store_true',
        help='拉给定时间之前的消息 (默认拉给定时间之后)',
    )
    parser.add_argument(
        '--limit', type=int, default=0,
        help='返回条数 (不传则不限制)',
    )
    parser.add_argument('--output', default='', help='输出文件')
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()

    conv_id = args.group
    if not conv_id:
        if not args.query:
            print('错误：需要 --group 或 --query 参数')
            sys.exit(1)
        print(f'🔍 搜索群聊: {args.query}')
        conv_id = search_group(args.query, args.dry_run)
        if not conv_id and not args.dry_run:
            sys.exit(1)

    print(f'📥 拉取消息 (起始: {args.time})...')
    all_messages: List[Any] = []
    current_time = args.time
    page = 0
    max_pages = 50
    remaining = args.limit if args.limit > 0 else float('inf')

    while page < max_pages and remaining > 0:
        cmd_args = [
            'chat', 'message', 'list',
            '--group', conv_id or '<CONV_ID>',
            '--time', current_time,
            '--format', 'json',
        ]
        if args.no_forward:
            cmd_args.append('--forward=false')
        page_limit = min(int(remaining), 200) if args.limit > 0 else 0
        if page_limit > 0:
            cmd_args.extend(['--limit', str(page_limit)])
        data = run_dws(cmd_args, dry_run=args.dry_run)

        if args.dry_run:
            print('[dry-run] 翻页循环: hasMore → 继续用边界 createTime 作为 --time')
            return

        if not data:
            break

        # 兼容两种结构: 顶层 messages / {result: {messages, hasMore}}
        if isinstance(data, list):
            page_msgs = data
            has_more = False
        else:
            container = data
            inner = data.get('result')
            if isinstance(inner, dict):
                container = inner
            page_msgs = container.get('messages')
            if page_msgs is None and isinstance(inner, list):
                page_msgs = inner
            if not isinstance(page_msgs, list):
                page_msgs = []
            has_more = bool(container.get('hasMore', False))

        if not page_msgs:
            break

        all_messages.extend(page_msgs)
        remaining -= len(page_msgs)
        page += 1

        if not has_more:
            break

        last_msg = page_msgs[-1]
        boundary_time = (last_msg.get('createTime')
                         or last_msg.get('createAt')
                         or last_msg.get('time', ''))
        if not boundary_time or boundary_time == current_time:
            break
        current_time = boundary_time
        print(f"  翻页 {page}: 已累计 {len(all_messages)} 条, 继续...")

    if not all_messages:
        print('未拉取到消息')
        return

    if args.output:
        with open(args.output, 'w', encoding='utf-8') as f:
            json.dump(all_messages, f, ensure_ascii=False, indent=2)
        print(f"  ✓ 已导出 {len(all_messages)} 条消息到 {args.output}")
    else:
        for m in all_messages:
            # 实际字段: sender/createTime/content (兼容旧字段名)
            sender = (m.get('sender') or m.get('senderNick')
                      or m.get('senderOpenDingTalkId', '未知'))
            text = m.get('content') or m.get('text', '')
            time_str = (m.get('createTime') or m.get('createAt')
                        or m.get('time', ''))
            print(f"  [{time_str}] {sender}: {text[:80]}")
        print(f"\n合计: {len(all_messages)} 条消息 ({page} 页)")


if __name__ == '__main__':
    main()
