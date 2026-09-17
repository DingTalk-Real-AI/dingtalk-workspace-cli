#!/usr/bin/env python3
"""
获取最近 N 条听记的 AI 摘要并合并输出

用法:
    python minutes_recent_summary.py          # 最近 5 条
    python minutes_recent_summary.py --max 10 # 最近 10 条
    python minutes_recent_summary.py --output summary.md
    python minutes_recent_summary.py --dry-run
"""

import sys
import json
import subprocess
import argparse
from typing import List, Any, Optional, Tuple


class DWSCommandError(RuntimeError):
    """DWS 没有返回可用 JSON；调用方不得把它解释成空业务结果。"""


def _unwrap_rows(payload: Any) -> List[Any]:
    if isinstance(payload, list):
        return payload
    if not isinstance(payload, dict):
        return []
    for key in ('result', 'data', 'list'):
        value = payload.get(key)
        if isinstance(value, list):
            return value
        if isinstance(value, dict):
            for inner_key in (
                'itemList', 'items', 'list', 'records', 'minutes',
            ):
                inner = value.get(inner_key)
                if isinstance(inner, list):
                    return inner
    return []


def uuid_title_pairs_from_payload(
    payload: Any,
) -> List[Tuple[str, str]]:
    """列表项可为对象、JSON 字符串、或纯 taskUuid 字符串。"""
    out: List[Tuple[str, str]] = []
    for item in _unwrap_rows(payload):
        if isinstance(item, dict):
            uuid = (
                item.get('taskUuid')
                or item.get('id')
                or item.get('task_uuid')
            )
            if not uuid:
                continue
            title = item.get('title') or item.get('name') or '无标题'
            if not isinstance(uuid, (str, int, float, bool)):
                continue
            if not isinstance(title, (str, int, float, bool)):
                title = str(title) if isinstance(title, dict) else '无标题'
            out.append((str(uuid), str(title)))
        elif isinstance(item, str):
            text = item.strip()
            if not text:
                continue
            if text.startswith('{'):
                try:
                    parsed = json.loads(text)
                except json.JSONDecodeError:
                    continue
                if not isinstance(parsed, dict):
                    continue
                uuid = (
                    parsed.get('taskUuid')
                    or parsed.get('id')
                    or parsed.get('task_uuid')
                )
                if not uuid:
                    continue
                title = parsed.get('title') or parsed.get('name') or '无标题'
                out.append((str(uuid), str(title)))
            else:
                out.append((text, text))
    return out


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
    except (subprocess.TimeoutExpired, FileNotFoundError) as exc:
        raise DWSCommandError(str(exc)) from exc
    if result.returncode != 0:
        detail = result.stderr.strip() or f"退出码 {result.returncode}"
        raise DWSCommandError(detail)
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        raise DWSCommandError(f"DWS 返回的不是合法 JSON：{exc}") from exc


def summary_text_from_payload(payload: Any) -> str:
    """兼容当前 Runtime 的 result.fullSummary 与历史直接字段。"""
    if isinstance(payload, str):
        return payload
    if not isinstance(payload, dict):
        return ''
    inner = payload.get('result', payload)
    if isinstance(inner, str):
        return inner
    if not isinstance(inner, dict):
        return ''
    value = (inner.get('fullSummary') or inner.get('summary')
             or inner.get('content'))
    if isinstance(value, str):
        return value
    if value is not None:
        return json.dumps(value, ensure_ascii=False)
    return json.dumps(inner, ensure_ascii=False)


def main():
    parser = argparse.ArgumentParser(
        description='获取最近听记的 AI 摘要'
    )
    parser.add_argument(
        '--max', type=int, default=5, help='获取条数 (默认 5)'
    )
    parser.add_argument(
        '--output', default='', help='输出到 Markdown 文件'
    )
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()

    print('🎙️ 获取听记列表...')
    list_data = run_dws([
        'minutes', 'list', 'mine',
        '--max', str(args.max),
        '--format', 'json',
    ], dry_run=args.dry_run)

    if args.dry_run:
        run_dws([
            'minutes', 'get', 'summary',
            '--id', '<TASK_UUID>', '--format', 'json',
        ], dry_run=True)
        return

    if not list_data:
        print('未找到听记')
        return

    pairs = uuid_title_pairs_from_payload(list_data)
    if not pairs:
        print('暂无听记')
        return

    output_lines = [f"# 最近 {len(pairs)} 条听记摘要\n"]
    for i, (uuid, title) in enumerate(pairs, 1):
        print(f"  [{i}/{len(pairs)}] 获取摘要: {title}")

        summary_data = run_dws([
            'minutes', 'get', 'summary',
            '--id', uuid, '--format', 'json',
        ])
        summary_text = summary_text_from_payload(summary_data)

        output_lines.append(f"## {i}. {title}\n")
        if summary_text:
            output_lines.append(f"{summary_text}\n")
        else:
            output_lines.append("(暂无摘要)\n")

    full_output = '\n'.join(output_lines)

    if args.output:
        with open(args.output, 'w', encoding='utf-8') as f:
            f.write(full_output)
        print(f"\n✓ 已输出到 {args.output}")
    else:
        print('\n' + full_output)


if __name__ == '__main__':
    try:
        main()
    except DWSCommandError as exc:
        print(f"错误：{exc}", file=sys.stderr)
        sys.exit(1)
