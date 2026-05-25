#!/usr/bin/env python3
"""
从听记中提取所有待办事项并汇总

用法:
    python minutes_extract_todos.py              # 最近 5 条听记
    python minutes_extract_todos.py --max 10     # 最近 10 条
    python minutes_extract_todos.py --id <uuid>  # 指定听记
    python minutes_extract_todos.py --dry-run
"""

import sys
import json
import subprocess
import argparse
from pathlib import Path
from typing import List, Any, Optional

_scripts_dir = Path(__file__).resolve().parent
if str(_scripts_dir) not in sys.path:
    sys.path.insert(0, str(_scripts_dir))

from minutes_list_parse import uuid_title_pairs_from_payload


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
        description='从听记中提取待办事项'
    )
    parser.add_argument('--max', type=int, default=5)
    parser.add_argument('--id', default='', help='指定听记 UUID')
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()

    uuids_with_titles = []
    if args.id:
        uuids_with_titles = [(args.id, args.id)]
    else:
        print('🎙️ 获取听记列表...')
        data = run_dws([
            'minutes', 'list', 'mine',
            '--max', str(args.max),
            '--format', 'json',
        ], dry_run=args.dry_run)
        if args.dry_run:
            run_dws([
                'minutes', 'get', 'todos',
                '--id', '<TASK_UUID>', '--format', 'json',
            ], dry_run=True)
            return
        if not data:
            return
        uuids_with_titles = uuid_title_pairs_from_payload(data)

    all_todos = []
    for uuid, title in uuids_with_titles:
        print(f"  提取待办: {title}")
        todos_data = run_dws([
            'minutes', 'get', 'todos',
            '--id', uuid, '--format', 'json',
        ])
        if not todos_data:
            continue
        if isinstance(todos_data, list):
            items = todos_data
        elif isinstance(todos_data, dict):
            inner = todos_data.get('result', todos_data)
            if isinstance(inner, dict):
                items = inner.get('todos', [])
            elif isinstance(inner, list):
                items = inner
            else:
                items = []
        else:
            items = []
        for t in items:
            if isinstance(t, dict):
                t['_source'] = title
        all_todos.extend(items)

    print(f"\n📋 听记待办汇总")
    print('=' * 50)

    if not all_todos:
        print('  ✅ 暂无待办事项')
        return

    for t in all_todos:
        if not isinstance(t, dict):
            print(f"  • {t!r}")
            continue
        content = (t.get('content') or t.get('text')
                   or t.get('title', ''))
        source = t.get('_source', '')
        print(f"  • {content}")
        if source:
            print(f"    来自: {source}")

    print(f"\n合计: {len(all_todos)} 条待办")


if __name__ == '__main__':
    main()
