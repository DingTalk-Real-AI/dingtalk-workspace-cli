#!/usr/bin/env python3
"""
创建 AI 应用并自动轮询等待完成

用法:
    python aiapp_create_and_poll.py \
        --prompt "创建一个仓库管理应用"

    python aiapp_create_and_poll.py \
        --prompt "生成客户管理 CRM" \
        --skills skill1,skill2 \
        --interval 30 \
        --timeout 600

    python aiapp_create_and_poll.py --dry-run --prompt "test"
"""

import sys
import json
import subprocess
import argparse
import time
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
            cmd, capture_output=True, text=True, timeout=120
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
        description='创建 AI 应用并轮询等待完成'
    )
    parser.add_argument(
        '--prompt', required=True, help='应用描述'
    )
    parser.add_argument(
        '--skills', default='', help='技能 ID 列表'
    )
    parser.add_argument(
        '--interval', type=int, default=30,
        help='轮询间隔秒 (默认 30)',
    )
    parser.add_argument(
        '--timeout', type=int, default=600,
        help='最大等待秒 (默认 600)',
    )
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()

    print(f'🚀 创建 AI 应用...')
    print(f'   Prompt: {args.prompt}')
    cmd_args = [
        'aiapp', 'create',
        '--prompt', args.prompt,
        '--format', 'json',
    ]
    if args.skills:
        cmd_args.extend(['--skills', args.skills])

    create_data = run_dws(cmd_args, dry_run=args.dry_run)
    if args.dry_run:
        run_dws([
            'aiapp', 'query',
            '--task-id', '<TASK_ID>',
            '--format', 'json',
        ], dry_run=True)
        return

    if not create_data:
        sys.exit(1)

    task_id = create_data.get('taskId') or create_data.get('id', '')
    thread_id = create_data.get('threadId', '')
    print(f"  ✓ 任务已创建")
    print(f"    taskId: {task_id}")
    print(f"    threadId: {thread_id}")

    print(f'\n⏳ 轮询等待 (间隔 {args.interval}s, '
          f'超时 {args.timeout}s)...')
    elapsed = 0
    while elapsed < args.timeout:
        time.sleep(args.interval)
        elapsed += args.interval

        query_data = run_dws([
            'aiapp', 'query',
            '--task-id', task_id,
            '--format', 'json',
        ])
        if not query_data:
            print(f"  [{elapsed}s] ⚠ 查询失败，继续等待...")
            continue

        status = (query_data.get('status')
                  or query_data.get('state', 'unknown'))
        progress = query_data.get('progress', {})
        step = ''
        if isinstance(progress, dict):
            step = progress.get('currentStep', '')

        if status == 'succeeded':
            print(f"  [{elapsed}s] ✅ 应用创建成功!")
            if thread_id:
                print(f"  threadId: {thread_id}")
            return
        elif status == 'failed':
            print(f"  [{elapsed}s] ❌ 创建失败")
            sys.exit(1)
        else:
            info = f"  [{elapsed}s] ⏳ {status}"
            if step:
                info += f" ({step})"
            print(info)

    print(f"\n⏰ 超时 ({args.timeout}s)，任务可能仍在运行")
    print(f"  可手动查询: dws aiapp query --task-id {task_id}")


if __name__ == '__main__':
    main()
