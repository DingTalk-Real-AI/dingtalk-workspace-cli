#!/usr/bin/env python3
"""
在指定目录创建文档并写入 Markdown 内容（一键完成）

用法:
    python doc_create_and_write.py \
        --name "项目周报" \
        --content "# 本周总结\n\n## 完成事项\n- 任务A"

    python doc_create_and_write.py \
        --name "会议纪要" \
        --content-file notes.md

    python doc_create_and_write.py \
        --name "知识库文档" --content "# 内容" --folder FOLDER_ID

    python doc_create_and_write.py --name "test" --content "hello" --dry-run
"""

import sys
import json
import time
import subprocess
import argparse
from pathlib import Path
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
            cmd, capture_output=True, text=True, timeout=60
        )
        if result.returncode != 0:
            print(f"  ✗ 错误：{result.stderr.strip()}")
            return None
        return json.loads(result.stdout)
    except (subprocess.TimeoutExpired, json.JSONDecodeError,
            FileNotFoundError) as e:
        print(f"  ✗ 错误：{e}")
        return None


def run_dws_with_retry(
    args: List[str],
    dry_run: bool = False,
    max_retries: int = 3,
    retry_delay: float = 1.0,
) -> Optional[Any]:
    """带重试机制的 dws 命令执行"""
    last_error = None
    for attempt in range(1, max_retries + 1):
        result = run_dws(args, dry_run=dry_run)
        if result is not None:
            return result
        if attempt < max_retries:
            print(f"  ⚠️  第 {attempt} 次尝试失败，{retry_delay}秒后重试...")
            time.sleep(retry_delay)
            retry_delay *= 1.5  # 指数退避
    return None


def main():
    parser = argparse.ArgumentParser(
        description='创建文档并写入内容'
    )
    parser.add_argument('--name', required=True, help='文档名称')
    parser.add_argument('--content', default='', help='Markdown 内容')
    parser.add_argument('--content-file', default='', help='内容文件')
    parser.add_argument('--folder', default='', help='目标文件夹 ID 或 URL')
    parser.add_argument('--workspace', default='', help='目标知识库 ID')
    parser.add_argument(
        '--mode', default='append', choices=['overwrite', 'append'],
        help='写入模式: overwrite=覆盖, append=追加 (默认 append)',
    )
    parser.add_argument(
        '--max-retries', type=int, default=3,
        help='每块写入失败时的最大重试次数 (默认 3)',
    )
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()

    content = args.content
    if args.content_file:
        p = Path(args.content_file)
        if not p.exists():
            print(f"错误：文件不存在: {p}")
            sys.exit(1)
        content = p.read_text(encoding='utf-8')
    if not content:
        print('错误：需要 --content 或 --content-file')
        sys.exit(1)
    chunk_size = 30000

    create_args = ['doc', 'create', '--name', args.name, '--format', 'json']
    if args.folder:
        create_args.extend(['--folder', args.folder])
    if args.workspace:
        create_args.extend(['--workspace', args.workspace])

    print(f'\n📝 创建文档: {args.name}')
    create_data = run_dws(create_args, dry_run=args.dry_run)

    node_id = None
    if not args.dry_run:
        if not create_data:
            sys.exit(1)
        node_id = (create_data.get('nodeId')
                   or create_data.get('dentryUuid')
                   or create_data.get('id', ''))
        print(f"  ✓ 文档已创建 (ID: {node_id})")

    if len(content) <= chunk_size:
        mode_label = '追加' if args.mode == 'append' else '覆盖'
        print(f'\n✍️  写入内容 (模式: {mode_label}, {len(content)} 字符)...')
        write_data = run_dws([
            'doc', 'update',
            '--node', node_id or '<NODE_ID>',
            '--content', content,
            '--mode', args.mode,
            '--format', 'json',
        ], dry_run=args.dry_run)
        if write_data:
            print(f"  ✓ 内容已写入 ({len(content)} 字符)")
    else:
        chunks = []
        pos = 0
        while pos < len(content):
            end = min(pos + chunk_size, len(content))
            if end < len(content):
                newline_pos = content.rfind('\n', pos, end)
                if newline_pos > pos:
                    end = newline_pos + 1
            chunks.append(content[pos:end])
            pos = end

        total_chunks = len(chunks)
        print(f'\n✍️  内容较长 ({len(content)} 字符), 分 {total_chunks} 块写入...')

        success_chunks = 0
        for idx, chunk in enumerate(chunks):
            chunk_mode = args.mode if idx == 0 else 'append'
            write_data = run_dws_with_retry(
                [
                    'doc', 'update',
                    '--node', node_id or '<NODE_ID>',
                    '--content', chunk,
                    '--mode', chunk_mode,
                    '--format', 'json',
                ],
                dry_run=args.dry_run,
                max_retries=args.max_retries,
            )
            if write_data:
                print(f"  ✓ 块 {idx + 1}/{total_chunks} 已写入 ({len(chunk)} 字符)")
                success_chunks += 1
            elif not args.dry_run:
                # 写入失败，报告部分写入状态
                print(f"\n❌ 块 {idx + 1}/{total_chunks} 写入失败（已重试 {args.max_retries} 次）")
                print(f"\n⚠️  文档处于部分写入状态:")
                print(f"   - 文档 ID: {node_id}")
                print(f"   - 已写入: {success_chunks}/{total_chunks} 块")
                print(f"   - 失败位置: 第 {idx + 1} 块")
                if args.mode == 'overwrite':
                    print(f"   - 模式: 覆盖模式，文档可能包含不完整内容")
                    print(f"   - 建议: 手动检查文档内容，或删除后重新创建")
                else:
                    print(f"   - 模式: 追加模式，已写入内容已保存")
                    print(f"   - 建议: 可手动补充剩余内容，或重新运行脚本")
                sys.exit(1)
    print('\n✅ 完成!')


if __name__ == '__main__':
    main()
