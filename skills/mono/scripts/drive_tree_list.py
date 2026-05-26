#!/usr/bin/env python3
"""
递归列出钉盘目录树结构（可指定深度）

用法:
    python drive_tree_list.py                   # 列出根目录
    python drive_tree_list.py --depth 2         # 递归 2 层
    python drive_tree_list.py --parent-id <id>  # 指定目录
    python drive_tree_list.py --dry-run
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


def list_dir(
    parent_id: str = '', dry_run: bool = False,
) -> list:
    cmd_args = [
        'drive', 'list', '--max', '50', '--format', 'json',
    ]
    if parent_id:
        cmd_args.extend(['--parent-id', parent_id])
    data = run_dws(cmd_args, dry_run=dry_run)
    if not data:
        return []
    if isinstance(data, list):
        return data
    if isinstance(data, dict):
        inner = data.get('result', data)
        if isinstance(inner, dict):
            return inner.get('items', inner.get('dentryList', []))
        if isinstance(inner, list):
            return inner
    return []


def print_tree(
    items: list, depth: int, max_depth: int,
    prefix: str = '', dry_run: bool = False,
):
    for i, item in enumerate(items):
        is_last = (i == len(items) - 1)
        connector = '└── ' if is_last else '├── '
        name = item.get('name') or item.get('fileName', '?')
        item_type = item.get('type') or item.get('dentryType', '')
        is_dir = str(item_type).lower() in (
            'folder', 'directory', '1', 'FOLDER'
        )
        icon = '📁' if is_dir else '📄'
        size_str = ''
        size = item.get('size') or item.get('fileSize')
        if size and not is_dir:
            size = int(size)
            if size > 1024 * 1024:
                size_str = f" ({size / 1024 / 1024:.1f}MB)"
            elif size > 1024:
                size_str = f" ({size / 1024:.1f}KB)"
            else:
                size_str = f" ({size}B)"

        print(f"{prefix}{connector}{icon} {name}{size_str}")

        if is_dir and depth < max_depth:
            child_prefix = prefix + ('    ' if is_last else '│   ')
            dentry_id = (item.get('dentryUuid')
                         or item.get('id', ''))
            if dentry_id:
                children = list_dir(dentry_id, dry_run=dry_run)
                print_tree(
                    children, depth + 1, max_depth,
                    child_prefix, dry_run,
                )


def main():
    parser = argparse.ArgumentParser(
        description='递归列出钉盘目录树'
    )
    parser.add_argument(
        '--parent-id', default='', help='起始目录 ID'
    )
    parser.add_argument(
        '--depth', type=int, default=1,
        help='递归深度 (默认 1, 最大 5)',
    )
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()
    args.depth = min(args.depth, 5)

    root_name = args.parent_id or '我的文件'
    print(f"📁 {root_name}")

    items = list_dir(args.parent_id, dry_run=args.dry_run)
    if args.dry_run:
        return
    if not items:
        print('  (空目录)')
        return

    print_tree(items, 0, args.depth, '', args.dry_run)
    print(f"\n共 {len(items)} 个项目 (根目录)")


if __name__ == '__main__':
    main()
