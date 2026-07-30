#!/usr/bin/env python3
"""
查询与某人的单聊聊天记录

用法:
    python3 scripts/chat_history_with_user.py --name "张三" --time "2026-03-10 00:00:00"
    python3 scripts/chat_history_with_user.py --user <userId> --time "2026-03-10 00:00:00" --limit 50
    python3 scripts/chat_history_with_user.py --name "张三" --time "2026-03-01 00:00:00" --output history.json

工作流:
  1. 通过 --name 搜索通讯录，获取 userId（或直接传 --user）
  2. 调用 chat message list-direct --user <userId> 拉取单聊消息
  3. 输出到终端或导出为 JSON 文件
"""

import sys
import json
import datetime
import subprocess
import argparse
from typing import List, Any, Optional


class ScriptError(RuntimeError):
    """可预期的脚本执行错误。"""

    exit_code = 1


class AmbiguousTargetError(ScriptError):
    """搜索结果不唯一，需要调用方消歧。"""

    exit_code = 2


def normalize_boundary_time(value: Any) -> str:
    """将消息时间边界规范为 CLI 接受的 yyyy-MM-dd HH:mm:ss 或原字符串。"""
    if value is None or isinstance(value, bool):
        return ''
    if isinstance(value, (int, float)):
        timestamp = float(value)
    elif isinstance(value, str):
        text = value.strip()
        if not text:
            return ''
        try:
            timestamp = float(text)
        except ValueError:
            return text
    else:
        return str(value).strip()
    if timestamp > 10_000_000_000:
        timestamp /= 1000
    try:
        dt = datetime.datetime.fromtimestamp(
            timestamp, tz=datetime.timezone(datetime.timedelta(hours=8))
        )
    except (OSError, OverflowError, ValueError) as exc:
        raise ScriptError(f'无效的消息时间边界：{value}') from exc
    return dt.strftime("%Y-%m-%d %H:%M:%S")


def run_dws(
    args: List[str], dry_run: bool = False,
) -> Optional[Any]:
    """执行 dws 命令并解析 JSON 输出"""
    cmd = ['dws'] + args
    if dry_run:
        print(f"[dry-run] {' '.join(cmd)}")
        return None
    try:
        result = subprocess.run(
            cmd, capture_output=True, text=True, timeout=120
        )
    except (subprocess.TimeoutExpired, FileNotFoundError) as exc:
        raise ScriptError(f'执行 dws 失败：{exc}') from exc
    if result.returncode != 0:
        detail = result.stderr.strip() or f'退出码 {result.returncode}'
        raise ScriptError(f'dws 命令失败：{detail}')
    try:
        data = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        raise ScriptError(f'dws 返回的不是合法 JSON：{exc}') from exc
    if isinstance(data, dict) and data.get('success') is False:
        detail = data.get('errorMsg') or data.get('message') or '未知错误'
        raise ScriptError(f'dws 业务调用失败：{detail}')
    return data


def search_user(
    name: str, dry_run: bool = False,
) -> Optional[str]:
    """按关键词搜索用户，返回第一个匹配的 userId"""
    data = run_dws([
        'contact', 'user', 'search',
        '--query', name, '--format', 'json',
    ], dry_run=dry_run)
    if dry_run:
        return '<USER_ID>'
    users = data
    if isinstance(data, dict):
        inner = data.get('result', data)
        if isinstance(inner, dict):
            users = (
                inner.get('value')
                or inner.get('users')
                or inner.get('list')
                or inner.get('items')
                or []
            )
        elif isinstance(inner, list):
            users = inner
        else:
            users = []
    if not users or not isinstance(users, list):
        raise ScriptError(f'未找到用户：{name}')
    exact = [
        item for item in users
        if isinstance(item, dict)
        and str(item.get('name') or item.get('nick') or '').strip().casefold()
        == name.strip().casefold()
    ]
    candidates = exact if exact else users
    if len(candidates) != 1:
        rendered = []
        for item in candidates:
            if not isinstance(item, dict):
                continue
            user_name = item.get('name') or item.get('nick') or '未知'
            user_id = item.get('userId') or item.get('userid') or '无ID'
            rendered.append(f'{user_name} ({user_id})')
        detail = '；'.join(rendered) or f'{len(candidates)} 个候选'
        raise AmbiguousTargetError(
            f'姓名“{name}”匹配到多个候选，请指定 --user：{detail}'
        )
    u = candidates[0]
    user_name = u.get('name') or u.get('nick', '未知')
    user_id = u.get('userId') or u.get('userid', '')
    if not user_id:
        raise ScriptError(f'用户“{user_name}”缺少 userId')
    print(f"  找到用户: {user_name} ({user_id})")
    return user_id


def parse_message_page(data: Any) -> tuple[List[Any], bool]:
    """提取消息页和 hasMore。"""
    if isinstance(data, list):
        return data, False
    if not isinstance(data, dict):
        return [], False
    inner = data.get('result', data)
    if isinstance(inner, dict):
        messages = inner.get('messages', [])
        return messages if isinstance(messages, list) else [], bool(
            inner.get('hasMore', False)
        )
    if isinstance(inner, list):
        return inner, False
    return [], False


def message_identity(message: Any) -> Optional[str]:
    if not isinstance(message, dict):
        return None
    value = (
        message.get('openMessageId')
        or message.get('openMsgId')
        or message.get('msgId')
    )
    return str(value) if value else None


def run(argv: Optional[List[str]] = None) -> int:
    parser = argparse.ArgumentParser(
        description='查询与某人的单聊聊天记录'
    )
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument('--name', help='按姓名搜索用户')
    group.add_argument('--user', help='直接指定 userId')
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
    parser.add_argument('--output', default='', help='导出到 JSON 文件')
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args(argv)

    try:
        user_id = args.user
        if not user_id:
            print(f'🔍 搜索用户: {args.name}')
            user_id = search_user(args.name, args.dry_run)

        print(f'📥 拉取与 {user_id} 的聊天记录 (起始: {args.time})...')
        all_messages: List[Any] = []
        seen_ids = set()
        current_time = args.time
        direction = 'older' if args.no_forward else 'newer'
        page = 0
        max_pages = 50
        remaining = args.limit if args.limit > 0 else float('inf')
        has_more = False

        while page < max_pages and remaining > 0:
            cmd_args = [
                'chat', 'message', 'list-direct',
                '--user', user_id or '<USER_ID>',
                '--time', current_time,
                '--direction', direction,
                '--format', 'json',
            ]
            page_limit = min(int(remaining), 200) if args.limit > 0 else 0
            if page_limit > 0:
                cmd_args.extend(['--limit', str(page_limit)])
            data = run_dws(cmd_args, dry_run=args.dry_run)

            if args.dry_run:
                print('[dry-run] 翻页循环: hasMore → 使用末条消息 createTime')
                return 0

            page_msgs, has_more = parse_message_page(data)
            if not page_msgs:
                if has_more:
                    raise ScriptError('服务端返回 hasMore=true，但本页没有消息')
                break

            for message in page_msgs:
                if not isinstance(message, dict):
                    raise ScriptError('消息列表包含非对象条目，无法安全导出')
                identity = message_identity(message)
                if identity and identity in seen_ids:
                    continue
                if identity:
                    seen_ids.add(identity)
                all_messages.append(message)
                remaining -= 1
                if remaining <= 0:
                    break
            page += 1

            if not has_more or remaining <= 0:
                break

            last_msg = page_msgs[-1]
            if not isinstance(last_msg, dict):
                raise ScriptError('末条消息不是对象，无法取得翻页时间边界')
            boundary_time = normalize_boundary_time(
                last_msg.get('createTime')
                or last_msg.get('createAt')
                or last_msg.get('time')
            )
            if not boundary_time:
                raise ScriptError('hasMore=true，但末条消息缺少 createTime')
            if boundary_time == current_time:
                raise ScriptError('分页边界没有推进，已停止以避免重复循环')
            current_time = boundary_time
            print(f"  翻页 {page}: 已累计 {len(all_messages)} 条, 继续...")

        if has_more and page >= max_pages and remaining > 0:
            raise ScriptError(f'达到最大分页数 {max_pages}，结果不完整')

        if not all_messages:
            print('未拉取到消息')
            return 0

        if args.output:
            with open(args.output, 'w', encoding='utf-8') as file:
                json.dump(all_messages, file, ensure_ascii=False, indent=2)
            print(f"  ✓ 已导出 {len(all_messages)} 条消息到 {args.output}")
        else:
            for message in all_messages:
                sender = (
                    message.get('sender')
                    or message.get('senderNick')
                    or '未知'
                )
                text = message.get('content') or message.get('text', '')
                time_str = (
                    message.get('createTime')
                    or message.get('createAt')
                    or message.get('time', '')
                )
                print(f"  [{time_str}] {sender}: {text[:80]}")
            print(f"\n合计: {len(all_messages)} 条消息 ({page} 页)")
        return 0
    except ScriptError as exc:
        print(f'错误：{exc}', file=sys.stderr)
        return exc.exit_code
    except OSError as exc:
        print(f'错误：无法写入输出文件：{exc}', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(run())
