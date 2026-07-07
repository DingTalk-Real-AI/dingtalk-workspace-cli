#!/usr/bin/env python3
"""
查询今天/明天/本周未完成的待办并汇总输出

用法:
    python todo_daily_summary.py              # 默认查今天
    python todo_daily_summary.py today        # 今天的待办
    python todo_daily_summary.py tomorrow     # 明天的待办
    python todo_daily_summary.py week         # 本周的待办
    python todo_daily_summary.py --dry-run    # 仅显示将执行的命令
"""

import sys
import json
import subprocess
from datetime import datetime, timedelta
from typing import List, Dict, Any, Optional

PRIORITY_MAP = {10: '低', 20: '普通', 30: '较高', 40: '紧急'}
PAGE_SIZE = 50
MAX_PAGES = 10


def run_dws(args: List[str], dry_run: bool = False) -> Optional[Any]:
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


def get_date_range(scope: str):
    now = datetime.now()
    today_start = now.replace(hour=0, minute=0, second=0, microsecond=0)
    if scope == 'today':
        return today_start, today_start + timedelta(days=1)
    elif scope == 'tomorrow':
        tmr = today_start + timedelta(days=1)
        return tmr, tmr + timedelta(days=1)
    elif scope == 'week':
        week_start = today_start - timedelta(days=today_start.weekday())
        return week_start, week_start + timedelta(days=7)
    return today_start, today_start + timedelta(days=1)


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


def fetch_all_todos(
    dry_run: bool = False,
) -> Optional[List[Dict[str, Any]]]:
    """返回 None 表示查询失败 (与"确实没有待办"区分开)"""
    all_todos: List[Dict[str, Any]] = []
    for page in range(1, MAX_PAGES + 1):
        data = run_dws([
            'todo', 'task', 'list',
            '--page', str(page),
            '--size', str(PAGE_SIZE),
            '--status', 'false',
            '--format', 'json',
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


def format_priority(p) -> str:
    try:
        return PRIORITY_MAP.get(int(p), str(p))
    except (ValueError, TypeError):
        return '普通'


def format_due(due_ms) -> str:
    if not due_ms:
        return '无截止时间'
    try:
        dt = datetime.fromtimestamp(int(due_ms) / 1000)
        return dt.strftime('%Y-%m-%d %H:%M')
    except (ValueError, TypeError, OSError):
        return str(due_ms)


def filter_by_due(
    todos: List[Dict[str, Any]], start: datetime, end: datetime,
) -> List[Dict[str, Any]]:
    start_ms = int(start.timestamp() * 1000)
    end_ms = int(end.timestamp() * 1000)
    result = []
    for t in todos:
        due = t.get('dueTime') or t.get('due')
        if not due:
            result.append(t)
            continue
        try:
            due_val = int(due)
            if start_ms <= due_val < end_ms:
                result.append(t)
        except (ValueError, TypeError):
            result.append(t)
    return result


def print_summary(
    todos: List[Dict[str, Any]], scope: str,
    start: datetime, end: datetime,
):
    scope_label = {
        'today': '今天', 'tomorrow': '明天', 'week': '本周',
    }.get(scope, scope)
    print(f"\n📋 {scope_label}未完成待办 "
          f"({start.strftime('%m-%d')} ~ {end.strftime('%m-%d')})")
    print('=' * 50)
    if not todos:
        print('  ✅ 暂无待办，轻松一下！')
        return
    urgent = [t for t in todos if format_priority(
        t.get('priority')) == '紧急']
    if urgent:
        print(f"\n🔴 紧急 ({len(urgent)} 条)")
        for t in urgent:
            title = t.get('subject') or t.get('title', '无标题')
            print(f"  • {title}  ⏰ {format_due(t.get('dueTime'))}")
    normal = [t for t in todos if t not in urgent]
    if normal:
        print(f"\n📌 其他 ({len(normal)} 条)")
        for t in normal:
            title = t.get('subject') or t.get('title', '无标题')
            pri = format_priority(t.get('priority'))
            print(f"  • [{pri}] {title}  ⏰ {format_due(t.get('dueTime'))}")
    print(f"\n合计: {len(todos)} 条待办")


def main():
    dry_run = '--dry-run' in sys.argv
    args = [a for a in sys.argv[1:] if a != '--dry-run']
    scope = args[0] if args else 'today'
    if scope not in ('today', 'tomorrow', 'week'):
        print(__doc__)
        sys.exit(1)
    start, end = get_date_range(scope)
    todos = fetch_all_todos(dry_run=dry_run)
    if dry_run:
        return
    if todos is None:
        print('错误：待办查询失败，无法给出汇总结论', file=sys.stderr)
        sys.exit(2)
    filtered = filter_by_due(todos, start, end)
    print_summary(filtered, scope, start, end)


if __name__ == '__main__':
    main()
