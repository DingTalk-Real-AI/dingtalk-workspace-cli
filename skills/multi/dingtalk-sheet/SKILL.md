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

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[sheet.md](references/sheet.md)；URL 识别与类型探测：[url-patterns.md](references/url-patterns.md)。

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
