#!/usr/bin/env python3
"""
完整报销流程：搜索供应商 → 搜索类别 → 创建付款单

用法:
    python finance_expense_flow.py \
        --amount 5000 \
        --supplier "华为" \
        --category "差旅" \
        --category-type expense

    python finance_expense_flow.py --dry-run --amount 1000
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


def main():
    parser = argparse.ArgumentParser(
        description='完整报销流程'
    )
    parser.add_argument(
        '--amount', required=True, help='报销金额'
    )
    parser.add_argument(
        '--supplier', default='', help='供应商名称关键词'
    )
    parser.add_argument(
        '--category', default='', help='费用类别关键词'
    )
    parser.add_argument(
        '--category-type', default='expense',
        choices=['income', 'expense'],
    )
    parser.add_argument('--tax', default='', help='税额')
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()

    supplier_code = ''
    category_code = ''

    if args.supplier:
        print(f'🔍 搜索供应商: {args.supplier}')
        data = run_dws([
            'finance', 'supplier', 'search',
            '--query', args.supplier,
            '--format', 'json',
        ], dry_run=args.dry_run)
        if not args.dry_run and data:
            if isinstance(data, list):
                items = data
            elif isinstance(data, dict):
                inner = data.get('result', data)
                items = inner if isinstance(inner, list) else []
            else:
                items = []
            if items:
                supplier_code = (items[0].get('code')
                                 or items[0].get('supplierCode', ''))
                name = items[0].get('name', '')
                print(f"  ✓ 找到: {name} ({supplier_code})")
            else:
                print(f"  ⚠ 未找到供应商: {args.supplier}")

    if args.category:
        print(f'🔍 搜索费用类别: {args.category}')
        data = run_dws([
            'finance', 'category', 'search',
            '--type', args.category_type,
            '--query', args.category,
            '--format', 'json',
        ], dry_run=args.dry_run)
        if not args.dry_run and data:
            if isinstance(data, list):
                items = data
            elif isinstance(data, dict):
                inner = data.get('result', data)
                items = inner if isinstance(inner, list) else []
            else:
                items = []
            if items:
                category_code = (items[0].get('code')
                                 or items[0].get('categoryCode', ''))
                name = items[0].get('name', '')
                print(f"  ✓ 找到: {name} ({category_code})")
            else:
                print(f"  ⚠ 未找到类别: {args.category}")

    print(f'\n💰 创建付款单 (金额: {args.amount})')
    cmd_args = [
        'finance', 'receipt', 'create',
        '--amount', args.amount,
        '--format', 'json',
    ]
    if supplier_code:
        cmd_args.extend(['--supplier-code', supplier_code])
    if category_code:
        cmd_args.extend(['--category-code', category_code])
    if args.tax:
        cmd_args.extend(['--tax', args.tax])

    result = run_dws(cmd_args, dry_run=args.dry_run)
    if result:
        print(f"  ✓ 付款单已创建")
    else:
        print(f"  ✗ 创建失败")
        sys.exit(1)

    print('\n✅ 报销流程完成!')


if __name__ == '__main__':
    main()
