---
name: dws-aitable-table-prepare-import-upload
description: "钉钉 AI 表格: 为导入任务申请 OSS 直传地址。返回 uploadUrl 和 importId。
客户端应通过 HTTP PUT 将原始文件字节流上传至 uploadUrl；除非 uploadUrl 对应的存储服务明确要求，否则不…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable table prepare_import_upload --help"
---

# aitable table prepare_import_upload

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

为导入任务申请 OSS 直传地址。返回 uploadUrl 和 importId。
客户端应通过 HTTP PUT 将原始文件字节流上传至 uploadUrl；除非 uploadUrl 对应的存储服务明确要求，否则不要额外附带 Content-Type 等自定义请求头。上传完成后将 importId 传入 import_data 即可触发导入，无需再传其他参数。

## Usage

```bash
dws aitable table prepare_import_upload --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--baseId` | ✓ | — | Base ID，可通过 list_bases 或 search_bases 获取 |
| `--fileName` | ✓ | — | 文件名，须带扩展名，例如 data.xlsx。扩展名将作为导入格式依据 |
| `--fileSize` | ✓ | — | 文件大小（字节数） |

## Required Fields

- `baseId`
- `fileName`
- `fileSize`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
