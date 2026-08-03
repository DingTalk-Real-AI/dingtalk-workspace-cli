#!/usr/bin/env python3
"""
导出群聊消息到 JSON 文件（从指定时间点拉取）

用法:
    python3 scripts/chat_export_messages.py \
        --group <openconversation_id> \
        --time "2026-03-10 00:00:00" \
        --output messages.json

    python3 scripts/chat_export_messages.py \
        --query "项目冲刺" \
        --time "2026-03-10 00:00:00" \
        --no-forward --limit 100
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


def search_group(
    query: str, dry_run: bool = False,
) -> Optional[str]:
    data = run_dws([
        'chat', 'search',
        '--query', query, '--format', 'json',
    ], dry_run=dry_run)
    if dry_run:
        return '<CONV_ID>'
    if isinstance(data, list):
        groups = data
    elif isinstance(data, dict):
        inner = data.get('result', data)
        if isinstance(inner, dict):
            groups = (
                inner.get('value')
                or inner.get('groups')
                or inner.get('items')
                or []
            )
        elif isinstance(inner, list):
            groups = inner
        else:
            groups = []
    else:
        groups = []
    if not groups:
        raise ScriptError(f'未找到群聊：{query}')
    exact = [
        item for item in groups
        if isinstance(item, dict)
        and str(item.get('title') or item.get('name') or '').strip().casefold()
        == query.strip().casefold()
    ]
    candidates = exact if exact else groups
    if len(candidates) != 1:
        rendered = []
        for item in candidates:
            if not isinstance(item, dict):
                continue
            name = item.get('title') or item.get('name') or '未知'
            conv_id = item.get('openConversationId') or item.get('id') or '无ID'
            rendered.append(f'{name} ({conv_id})')
        detail = '；'.join(rendered) or f'{len(candidates)} 个候选'
        raise AmbiguousTargetError(
            f'群名“{query}”匹配到多个候选，请指定 --group：{detail}'
        )
    g = candidates[0]
    name = g.get('title') or g.get('name', '未知')
    conv_id = g.get('openConversationId') or g.get('id')
    if not conv_id:
        raise ScriptError(f'群聊“{name}”缺少 openConversationId')
    print(f"  找到群聊: {name} ({conv_id})")
    return conv_id


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
    args = parser.parse_args(argv)

    try:
        conv_id = args.group
        if not conv_id:
            if not args.query:
                raise ScriptError('需要 --group 或 --query 参数')
            print(f'🔍 搜索群聊: {args.query}')
            conv_id = search_group(args.query, args.dry_run)

        print(f'📥 拉取消息 (起始: {args.time})...')
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
                'chat', 'message', 'list',
                '--group', conv_id or '<CONV_ID>',
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
