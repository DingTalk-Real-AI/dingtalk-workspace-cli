#!/usr/bin/env python3
"""
按部门名称搜索并列出所有成员（自动 deptId 解析）

用法:
    python contact_dept_members.py --query "技术部"
    python contact_dept_members.py --query "产品" --dry-run
"""

import sys
import re
import json
import subprocess
import argparse
from typing import List, Any, Optional


def strip_highlight(text: str) -> str:
    """去除 dept search 返回名称中的 <red>…</red> 高亮标签。"""
    if not isinstance(text, str):
        return text
    return re.sub(r'</?red>', '', text)


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
        description='按部门名称搜索并列出所有成员'
    )
    parser.add_argument(
        '--query', required=True, help='部门名称关键词'
    )
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()

    print(f'🔍 搜索部门: {args.query}')
    dept_data = run_dws([
        'contact', 'dept', 'search',
        '--query', args.query, '--format', 'json',
    ], dry_run=args.dry_run)

    if args.dry_run:
        run_dws([
            'contact', 'dept', 'list-members',
            '--ids', '<DEPT_ID>', '--format', 'json',
        ], dry_run=True)
        return

    if not dept_data:
        print('未找到匹配部门')
        sys.exit(1)

    if isinstance(dept_data, list):
        depts = dept_data
    elif isinstance(dept_data, dict):
        inner = dept_data.get('result', dept_data)
        if isinstance(inner, dict):
            # dept search 返回顶层 deptList；兼容历史 items/depts 键。
            depts = (inner.get('deptList')
                     or inner.get('items')
                     or inner.get('depts')
                     or [])
        elif isinstance(inner, list):
            depts = inner
        else:
            depts = []
    else:
        depts = []
    if not depts:
        print('未找到匹配部门')
        sys.exit(1)

    for dept in depts:
        dept_id = dept.get('id') or dept.get('deptId')
        dept_name = strip_highlight(
            dept.get('name') or dept.get('deptName', '未知')
        )
        if not dept_id:
            continue

        print(f"\n📂 {dept_name} (ID: {dept_id})")
        print('-' * 40)

        members_data = run_dws([
            'contact', 'dept', 'list-members',
            '--ids', str(dept_id), '--format', 'json',
        ])
        if not members_data:
            print('  无法获取成员列表')
            continue

        if isinstance(members_data, list):
            members = members_data
        elif isinstance(members_data, dict):
            inner = members_data.get('result', members_data)
            if isinstance(inner, dict):
                # list-members 返回 deptUserList；兼容历史 userlist/list 键。
                members = (inner.get('deptUserList')
                           or inner.get('userlist')
                           or inner.get('list')
                           or [])
            elif isinstance(inner, list):
                members = inner
            else:
                members = []
        else:
            members = []
        if not members:
            print('  (暂无成员)')
            continue

        for m in members:
            # list-members 每项形如 {"userInfo": {"name":..., "userId":...}}，
            # 成员字段嵌在 userInfo 下；兼容历史扁平结构。
            info = m.get('userInfo', m)
            name = info.get('name') or info.get('userName', '未知')
            title = info.get('title') or info.get('position', '')
            uid = info.get('userId') or info.get('userid', '')
            line = f"  👤 {name}"
            if title:
                line += f" ({title})"
            if uid:
                line += f"  [ID: {uid}]"
            print(line)

        print(f"  共 {len(members)} 人")


if __name__ == '__main__':
    main()
