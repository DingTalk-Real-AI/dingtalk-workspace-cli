---
name: dingtalk-contract
description: 钉钉智能合同。Use when 用户说 智能合同/合同管理/合同台账/合同详情/合同状态/查合同/合同分类/合同审批模板/合同起草/听记起草合同/合同审查/合同权益/批量导入合同/钉盘批量导入合同/合同 AI 审查/合同任务结果。Distinct from dingtalk-oa(普通审批，非合同专项)。命令前缀：dws dingtalk contract（兼容 `dws contract`）。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 钉钉智能合同 Skill

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[contract.md](references/contract.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "查合同台账 / 合同列表" | `dws dingtalk contract record list [--status approving,signing] [--start <ISO>] [--end <ISO>]` |
| "看某个合同详情" | `dws dingtalk contract record get --contract-id <id>` |
| "按状态/类型统计合同数量" | `dws dingtalk contract record quantity-by-type` |
| "新建合同" | `dws dingtalk contract record create --file <path|-> ` |
| "钉盘批量导入合同（异步）" | `dws dingtalk contract import batch --file-id <id> --space-id <id>` → `dws dingtalk contract import batch-result --task-id <taskId>` |
| "看审批模板 / 合同起草模板" | `dws dingtalk contract process-templates` |
| "查台账分类 / 合同分类树" | `dws dingtalk contract file-directories`（别名 `directories`） |
| "用听记起草合同" | `dws dingtalk contract draft` |
| "查 AI 合同审查权益" | `dws dingtalk contract review benefit` |
| "创建合同 AI 审查任务（异步）" | `dws dingtalk contract review create --file ./review.json` → `dws dingtalk contract review result --task-id <id> --review-type AI_REVIEW` |
| "解析合同（同步）" | `dws dingtalk contract review analysis --file <path|->` |

## 关键约束

- **时间格式**：`record list` 的 `--start` / `--end` 用 **ISO-8601 字符串**（不是毫秒时间戳）；表示合同**创建时间**
- **状态枚举**：`--status` 用英文，逗号分隔（如 `approving,signing`）
- **`--type` 是查询维度枚举**：默认 `all`，**与台账分类名称无关**（不要把 file-directories 返回的分类名当 type 用）
- **JSON 文件 / stdin**：`record create` / `review create` / `review analysis` 的 `--file` 支持 `-` 读 stdin
- **命名空间**：优先 `dws dingtalk contract ...`；`dws contract ...` 为兼容隐藏入口

## 异步任务通用模式

```
dws dingtalk contract <verb> create [...]   →  返回 taskId
                              ↓
dws dingtalk contract <verb> result --task-id <taskId> [--review-type ...]   →  轮询直到 success
```

适用于：`import batch` / `review create`。

## 跨产品协作

- 听记起草合同 → 先用 `dingtalk-minutes` 拿 taskUuid，再调 `dws dingtalk contract draft`
- 合同附件存钉盘 → 用 `dingtalk-drive` 上传，拿 `fileId` / `spaceId` 给 `import batch`
- 合同走 OA 审批 → 切到 `dingtalk-oa`（合同的审批模板已由 `process-templates` 列出）
- 合同摘要写文档 → 切到 `dingtalk-doc`
