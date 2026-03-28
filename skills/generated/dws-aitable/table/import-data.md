---
name: dws-aitable-table-import-data
description: "钉钉 AI 表格: 将已通过 prepare_import_upload 上传完成的文件导入 AI 表格，每个 Sheet 会新建为独立的数据表（不支持追加到已有表格）。
工具内部会等待导入完成，大多数情况下一次调用即可拿到最终结果。若…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable table import_data --help"
---

# aitable table import_data

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

将已通过 prepare_import_upload 上传完成的文件导入 AI 表格，每个 Sheet 会新建为独立的数据表（不支持追加到已有表格）。
工具内部会等待导入完成，大多数情况下一次调用即可拿到最终结果。若在 timeout 内未完成，再次传入相同 importId 继续等待，无需重新提交任务，也不要重新上传同一文件。

## Usage

```bash
dws aitable table import_data --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--importId` | ✓ | — | prepare_import_upload 返回的 importId |
| `--timeout` | — | — | 可选，本次调用的最长等待时间（秒），默认且推荐使用最大值 30。最小 5，最大 30。超时后若任务仍未完成，再次传入相同 importId 继续等待 |

## Required Fields

- `importId`

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
