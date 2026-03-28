---
name: dws-aitable-attachment-upload
description: "钉钉 AI 表格: 为单个 attachment 字段文件申请带容量校验的 OSS 直传地址。
该工具仅适用于“需要先上传本地文件，再将其写入 attachment 字段”的场景，不是通用文件上传入口，也不适用于后续导入类任务上传。…"
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws aitable attachment upload --help"
---

# aitable attachment upload

> **PREREQUISITE:** Read `../../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

为单个 attachment 字段文件申请带容量校验的 OSS 直传地址。
该工具仅适用于“需要先上传本地文件，再将其写入 attachment 字段”的场景，不是通用文件上传入口，也不适用于后续导入类任务上传。
如果已经有可直接下载的在线文件 URL，不要先下载文件再调用本工具；可直接在 create_records / update_records 的 attachment 字段中传入 [{"url":"https://..."}]，由服务端自动代拉外链并转存为内部附件。
该工具只负责准备上传，不直接接收文件二进制内容；实际文件字节流应由客户端在 MCP 外上传到返回的 uploadUrl。
上传文件时，向 uploadUrl 发起的 PUT 请求必须携带 Content-Type header，且其值必须是该文件的具体 MIME type。
上传成功后，请在 create_records / update_records 的 attachment 字段中写入 [{"fileToken":"..."}]。

## Usage

```bash
dws aitable attachment upload --json '{...}'
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--base-id` | ✓ | — | Base ID，可通过 list_bases 或 search_bases 获取 |
| `--file-name` | ✓ | — | 待写入 attachment 字段的文件名，必须包含扩展名（如 report.xlsx、photo.png）。服务端会基于扩展名和 mimeType 推断资源类型。 |
| `--mime-type` | — | — | 可选，文件 MIME type（如 application/pdf、image/png）。不传时服务端会根据 fileName 扩展名推断。若传入该值，则上传文件到 uploadUrl 时，PUT 请求必须携带 Content-Type header，且其值必须与这里完全一致。该字段只影响附件资源识别，不会把该工具升级为通用上传接口。 |
| `--size` | ✓ | — | 文件大小（字节），必须大于 0。prepare 阶段会用它向下游申请带容量校验的 attachment 上传地址。 |

## Required Fields

- `baseId`
- `fileName`
- `size`

> [!CAUTION]
> This is a **write** command — confirm with the user before executing.

## See Also

- [dws-shared](../../dws-shared/SKILL.md) — Global rules and auth
- [dws-aitable](../SKILL.md) — Product skill
