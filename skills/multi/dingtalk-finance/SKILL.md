---
name: dingtalk-finance
description: 钉钉智能财务。Use when 用户说 财务/付款单/收款单/发票/开票/查发票/银行交易明细/会计凭证/会计分录/客户管理/供应商/收支类别/审批单开票/现金日报/数电发票。Distinct from dingtalk-oa(普通审批流程，非财务专项)。命令前缀：dws finance。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 钉钉智能财务 Skill

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[finance.md](references/finance.md)（覆盖审批单 / 发票申请 / 添加发票到审批单等部分子命令）。
> 完整子命令列表（receipt / invoice / bank / voucher / customer / account / journal / supplier / category / company / digital-invoice）请用 `dws finance --help` 查询；本文档随产品演进逐步补全。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "查审批单详情 / 按审批编号查表单" | `dws finance process form-data --business-id <BIZ_ID>` |
| "按模版查审批单列表" | `dws finance process list --form-name "<模版名>" --start-time <date> --end-time <date>` |
| "查开票申请列表" | `dws finance invoice list-application [--start-time <date>] [--end-time <date>]` |
| "添加发票到审批单 / 审批单开票" | `dws finance invoice add-record --business-id <BIZ_ID> --invoice-pdf-url <URL>` |
| "上传发票 / 开具发票" | `dws finance invoice upload` / `dws finance invoice issue` |
| "AI 推荐发票收支类别" | `dws finance invoice recommend-category --items '[...]'` |
| "创建付款单 / 收款单" | `dws finance receipt create` / `dws finance receipt create-collection` |
| "录入 / 查银行交易明细" | `dws finance bank create` / `dws finance bank query` |
| "生成会计凭证" | `dws finance voucher entries / generate` |
| "查客户 / 查账户" | `dws finance customer list / get` / `dws finance account list` |
| "现金日报" | `python scripts/finance_daily_cashflow.py --date <YYYY-MM-DD>` |
| "录支出（按供应商+类别）" | `python scripts/finance_expense_flow.py --supplier "<名>" --category "<类>" --amount <num>` |
| "供应商 / 收支类别 / 主体搜索" | `dws finance supplier search` / `dws finance category search` / `dws finance company search` |
| "数电发票登录 / 开票 / 校脸" | `dws finance digital-invoice do-login` / `do-login-status` / `face-qr` / `face-status` 等 |

## 跨产品协作

- 审批单本身的状态流转（同意 / 拒绝 / 撤销）→ 切到 `dingtalk-oa`
- 把发票文件落到钉盘 → 切到 `dingtalk-drive`
- 把开票汇总写文档 → 切到 `dingtalk-doc`
