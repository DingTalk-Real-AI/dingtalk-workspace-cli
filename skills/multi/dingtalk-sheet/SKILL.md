---
name: dingtalk-sheet
description: 钉钉电子表格。Use when 用户说 电子表格/工作表/单元格读写/单元格追加/查找/公式/超链接/插入图片/浮动图片/sheet。Distinct from dingtalk-aitable(AI表格/多维表/字段类型)、dingtalk-doc(普通文档)。命令前缀：dws sheet。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉电子表格 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。20 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性可能因企业服务发现配置而异**。本文档列出的命令基于 dws envelope schema 与本仓库 v1.0.30 实测，但部分命令的 cobra 子命令暴露与否还取决于你的企业 MCP gateway 是否注册了对应 tool。如果跑某条命令报 `unknown command` 或 fall back 到父级 help，说明当前账号企业未开通该能力。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[sheet.md](references/sheet.md)；URL 识别与类型探测：[url-patterns.md](references/url-patterns.md)。

## 开放平台文档 RAG / 错误码排查

- 任何产品执行中，只要用户问开放平台 API、接口参数、字段含义、权限点、回调、SDK、配额、错误码，或命令返回上游 OpenAPI/SDK 错误，必须先用 `dws devdoc article search --query "<关键词>" --format json` 做官方文档 RAG。
- 查询词优先保留原始 API 名、能力名、权限点、完整错误码和 message；首轮形如 `errcode <code> <message>`，无结果再换 `<产品/场景> <错误码>`、`<接口名> 参数`。
- 本地 CLI 错误（如 `unknown command` / `unknown flag` / 认证 / recovery）仍按 root `dws` / `dws-shared` 的错误处理执行；`devdoc` 用于开放平台业务错误码和接口语义排查。
- `devdoc` 只查钉钉开放平台开发者文档，不查业务数据；排查结论必须基于命中条目的标题、摘要或链接，不能编造错误原因或不存在的命令。

## URL 预检（含 alidocs URL 时必读）

输入含 `alidocs.dingtalk.com` URL 时，该域名下存在多种路径格式：`/i/p/...`（分享短链）、`/i/nodes/...`（节点链接，类型需探测）、`/spreadsheetv2/...`（电子表格直链，直接路由到 `sheet`）、`/document/edit|preview?dentryKey=...`（文档链接，路由到 `dingtalk-doc`）等。**必须先读 [url-patterns.md](references/url-patterns.md) 中的「alidocs URL 分流决策」**，按规则识别 URL 类型；仅当确认是在线电子表格（`/spreadsheetv2/...` 或 `i/nodes/` 且 probe 出 `contentType=ALIDOC` + `extension=axls`）时，才继续走本 skill 的命令。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "创建电子表格" | `dws sheet create --name "<标题>"` |
| "新建工作表" | `dws sheet new --node <nodeId或URL> --name "<sheet名>"` |
| "读取单元格" | `dws sheet range read --node <nodeId或URL> --sheet-id <sheetId> --range A1:B2` |
| "写入单元格" | `dws sheet range update --node <nodeId或URL> --sheet-id <sheetId> --range A1:B2 --values '[[..]]'` |
| "追加一行" | `dws sheet append --node <nodeId或URL> --sheet-id <sheetId> --values '[[..]]'` |
| "查找 / 替换" | `dws sheet find --node <nodeId或URL> --sheet-id <sheetId> --find "<关键词>"` / `dws sheet replace --node <nodeId或URL> --sheet-id <sheetId> --find "<旧值>" --replacement "<新值>"` |
| "插入图片到单元格" | `dws sheet write-image --node <nodeId或URL> --sheet-id <sheetId> --range A1 --resource-id <attachmentId>`（或 `--resource-url <url>`） |

## URL 与 ID 前置

- 用户直接粘贴 `alidocs.dingtalk.com` URL 时，先执行 `dws doc info --node "<URL>" --format json` 探测；只有 `contentType=ALIDOC` 且 `extension=axls` 才继续走 `sheet`。
- `spreadsheetv2` 链接不要截取短 path segment，必须把完整 URL 原样传给 `--node`。
- 写入类命令必须先用 `dws sheet list --node <nodeId或URL> --format json` 取得真实 `sheetId`；不要猜 `Sheet1`、`sheet1`、`0`。
- 所有 sheet 子命令使用 `--node` 参数；不要写 `--node-id`、`--file-id` 或把 JSON 字段名 `id` 当成节点值传入。

## 评测高频硬约束

- `sheet create` 后必须从返回结果提取真实 `nodeId` 或文档 URL，后续 `range update/read` 原样传给 `--node`；如果返回里同时有 `nodeId` 和 `url`，优先用 `nodeId`。
- 写入区域前先 `sheet list --node <nodeId> --format json` 获取真实 `sheetId`；写入后立即 `range read` 同一区域验证。
- `range update` 的 `--values` 必须是二维 JSON，行列数量与 `--range` 完全一致；批量样式用 `range batch-set-style --batch <json文件>`。
- 同一参数错误最多纠正 2 次；若持续 `nodeId 格式不合法`，回到 `doc info --node <URL>` 或 `sheet create` 返回重新取 ID，不要无意义重试。

## 跨产品协作

- 多维表 / 字段类型 → 切到 `dingtalk-aitable`
- 把表数据写进文档 → 切到 `dingtalk-doc`
