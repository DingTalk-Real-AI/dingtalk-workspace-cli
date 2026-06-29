---
name: dingtalk-drive
description: 钉盘文件存储。Use when 用户说 钉盘/上传文件/下载文件/文件夹/查文件/创建文件夹。Distinct from dingtalk-doc(钉钉文档内容编辑)、dingtalk-wiki(知识库空间)。命令前缀：dws drive。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉盘 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。20 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性可能因企业服务发现配置而异**。本文档列出的命令基于 dws envelope schema 与本仓库 v1.0.30 实测，但部分命令的 cobra 子命令暴露与否还取决于你的企业 MCP gateway 是否注册了对应 tool。如果跑某条命令报 `unknown command` 或 fall back 到父级 help，说明当前账号企业未开通该能力。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[drive.md](references/drive.md)。

## 开放平台文档 RAG / 错误码排查

- 任何产品执行中，只要用户问开放平台 API、接口参数、字段含义、权限点、回调、SDK、配额、错误码，或命令返回上游 OpenAPI/SDK 错误，必须先用 `dws devdoc article search --query "<关键词>" --format json` 做官方文档 RAG。
- 查询词优先保留原始 API 名、能力名、权限点、完整错误码和 message；首轮形如 `errcode <code> <message>`，无结果再换 `<产品/场景> <错误码>`、`<接口名> 参数`。
- 本地 CLI 错误（如 `unknown command` / `unknown flag` / 认证 / recovery）仍按 root `dws` / `dws-shared` 的错误处理执行；`devdoc` 用于开放平台业务错误码和接口语义排查。
- `devdoc` 只查钉钉开放平台开发者文档，不查业务数据；排查结论必须基于命中条目的标题、摘要或链接，不能编造错误原因或不存在的命令。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "看钉盘空间 / 团队文件 / 有哪些 space" | `dws drive list-spaces` |
| "看钉盘文件 / 文件夹列表" | `dws drive list --space-id <spaceId> [--parent-id <fileId>]` |
| "钉盘目录树" | `python scripts/drive_tree_list.py --depth 2` |
| "查文件元数据" | `dws drive info --space-id <spaceId> --file-id <fileId>` |
| "下载文件" | `dws drive download --space-id <spaceId> --file-id <fileId> --output <path>` |
| "上传本地文件（首选一键）" | `dws drive upload --file ./report.pdf [--folder <fileId>]` |
| "上传文件（手动三步）" | `dws drive upload-info --space-id <spaceId> --file-name <名> --file-size <bytes> [--parent-id <fileId>]` → 客户端 HTTP PUT → `dws drive commit --space-id <spaceId> --upload-id <uploadId> --file-name <名> --file-size <bytes> [--parent-id <fileId>]` |
| "建文件夹" | `dws drive mkdir --space-id <spaceId> --name "<名称>" [--parent-id <fileId>]` |
| "删除文件 / 移到回收站（需确认）" | `dws drive delete --file-id <dentryUuid> --yes` |

## 评测高频硬约束

- 查找文件不要只看根目录后放弃；根目录没命中时，进入最相关的评测/目标文件夹继续 `drive list --space-id <spaceId> --parent-id <fileId>`，必要时用目录树脚本递归到合理深度。
- `drive list` 默认 `--max 20`，评测里保守使用 `--max 50` 以内并处理 `nextToken` 翻页；不要因为参数边界报错反复重试。
- `dws drive` 当前没有 search 子命令，按目录递归 `drive list`；命中后必须 `drive info --space-id <spaceId> --file-id <fileId> --format json` 回读元数据。
- `drive download` 需要 `--output` 指定本地保存路径或目录；不要省略必填输出位置。
- 删除、覆盖、移动等破坏性操作必须确认；上传（upload-info + commit 两步）、创建文件夹、下载后要读回或列目录验证。
- 所有 `dws drive` 命令加 `--format json`。

## 跨产品协作

- 文件内容编辑（钉钉文档）→ 切到 `dingtalk-doc`
- 知识库空间 → 切到 `dingtalk-wiki`
