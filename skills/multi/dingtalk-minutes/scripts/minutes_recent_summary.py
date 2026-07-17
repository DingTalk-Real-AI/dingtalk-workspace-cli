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
        '--limit', str(args.max),
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
        summary_text = ''
        if summary_data:
            if isinstance(summary_data, str):
                summary_text = summary_data
            elif isinstance(summary_data, dict):
                inner = summary_data.get('result')
                container = (inner if isinstance(inner, dict)
                             else summary_data)
                summary_text = (container.get('fullSummary')
                                or container.get('summary')
                                or container.get('content')
                                or (inner if isinstance(inner, str) else '')
                                or json.dumps(container,
                                              ensure_ascii=False))

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
    main()
