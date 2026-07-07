#!/usr/bin/env python3
"""
扫描已过截止时间但未完成的待办，输出逾期清单

用法:
    python todo_overdue_check.py
    python todo_overdue_check.py --dry-run
"""

import sys
import json
import subprocess
from datetime import datetime
from typing import List, Dict, Any, Optional

PAGE_SIZE = 50
MAX_PAGES = 10
PRIORITY_MAP = {10: '低', 20: '普通', 30: '较高', 40: '紧急'}


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
    except subprocess.TimeoutExpired:
        print('错误：命令执行超时', file=sys.stderr)
        return None
    except json.JSONDecodeError:
        # 输出非 JSON 多为底层错误 (如 TOKEN 失效), 透出原文
        print(f"错误：dws 返回非 JSON 输出: "
              f"{result.stdout.strip()[:300]}", file=sys.stderr)
        return None
    except FileNotFoundError as e:
        print(f"错误：{e}", file=sys.stderr)
        return None


def extract_todo_cards(data: Any) -> Optional[List[Dict[str, Any]]]:
    """兼容两种结构: 顶层 todoCards / {result: {todoCards: [...]}}。
    返回 None 表示结构无法识别或调用失败。"""
    if isinstance(data, list):
        return data
    if not isinstance(data, dict):
        return None
    if data.get('success') is False:
        print(f"错误：todo 查询失败: "
              f"{data.get('errorMsg') or data.get('errorCode') or data}",
              file=sys.stderr)
        return None
    items = data.get('todoCards')
    if items is None:
        inner = data.get('result')
        if isinstance(inner, dict):
            items = inner.get('todoCards')
        elif isinstance(inner, list):
            items = inner
    return items if isinstance(items, list) else None


def fetch_all_undone(
    dry_run: bool = False,
) -> Optional[List[Dict[str, Any]]]:
    """返回 None 表示查询失败 (与"确实没有待办"区分开)"""
    all_todos: List[Dict[str, Any]] = []
    for page in range(1, MAX_PAGES + 1):
        data = run_dws([
            'todo', 'task', 'list',
            '--page', str(page), '--size', str(PAGE_SIZE),
            '--status', 'false', '--format', 'json',
        ], dry_run=dry_run)
        if dry_run:
            return []
        if data is None:
            return None if page == 1 else all_todos
        items = extract_todo_cards(data)
        if items is None:
            return None if page == 1 else all_todos
        if not items:
            break
        all_todos.extend(items)
        if len(items) < PAGE_SIZE:
            break
    return all_todos


def find_overdue(todos: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    now_ms = int(datetime.now().timestamp() * 1000)
    overdue = []
    for t in todos:
        due = t.get('dueTime') or t.get('due')
        if not due:
            continue
        try:
            if int(due) < now_ms:
                overdue.append(t)
        except (ValueError, TypeError):
            continue
    return overdue


def days_overdue(due_ms) -> int:
    now = datetime.now()
    try:
        due_dt = datetime.fromtimestamp(int(due_ms) / 1000)
        return max(0, (now - due_dt).days)
    except (ValueError, TypeError, OSError):
        return 0


def main():
    if '--help' in sys.argv or '-h' in sys.argv:
        print(__doc__)
        return
    dry_run = '--dry-run' in sys.argv
    todos = fetch_all_undone(dry_run=dry_run)
    if dry_run:
        return
    if todos is None:
        print('错误：待办查询失败，无法给出逾期结论', file=sys.stderr)
        sys.exit(2)

    overdue = find_overdue(todos)
    overdue.sort(
        key=lambda t: int(t.get('dueTime') or t.get('due', 0))
    )

    print(f"\n⏰ 逾期待办检查 ({datetime.now().strftime('%Y-%m-%d %H:%M')})")
    print('=' * 50)

    if not overdue:
        print('  ✅ 没有逾期待办，继续保持！')
        return

    for t in overdue:
        title = t.get('subject') or t.get('title', '无标题')
        due = t.get('dueTime') or t.get('due')
        days = days_overdue(due)
        pri = PRIORITY_MAP.get(
            int(t.get('priority', 20)), '普通'
        )
        due_str = datetime.fromtimestamp(
            int(due) / 1000
        ).strftime('%Y-%m-%d')
        print(f"  🔴 [{pri}] {title}")
        print(f"     截止: {due_str}  逾期: {days} 天")

    print(f"\n合计: {len(overdue)} 条逾期待办")
    sys.exit(1 if overdue else 0)


if __name__ == '__main__':
    main()
