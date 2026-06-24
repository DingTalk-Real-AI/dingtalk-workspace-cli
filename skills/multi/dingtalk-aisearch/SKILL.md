---
name: dingtalk-aisearch
description: AI 搜问 - 搜人首选入口（按姓名/部门/职位/职责/上下级/手机号/工号维度）。Use when 用户说 找同事/找人/谁负责XX/XX的负责人是谁/查上级/查下级/团队成员/XX工号是谁/XX手机号。Distinct from dingtalk-contact(精确按 userId 查详情)。命令前缀：dws aisearch。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉 AI 搜问（搜人）Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。20 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性可能因企业服务发现配置而异**。本文档列出的命令基于 dws envelope schema 与本仓库 v1.0.30 实测，但部分命令的 cobra 子命令暴露与否还取决于你的企业 MCP gateway 是否注册了对应 tool。如果跑某条命令报 `unknown command` 或 fall back 到父级 help，说明当前账号企业未开通该能力。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[aisearch.md](references/aisearch.md)。

## 开放平台文档 RAG / 错误码排查

- 任何产品执行中，只要用户问开放平台 API、接口参数、字段含义、权限点、回调、SDK、配额、错误码，或命令返回上游 OpenAPI/SDK 错误，必须先用 `dws devdoc article search --query "<关键词>" --format json` 做官方文档 RAG。
- 查询词优先保留原始 API 名、能力名、权限点、完整错误码和 message；首轮形如 `errcode <code> <message>`，无结果再换 `<产品/场景> <错误码>`、`<接口名> 参数`。
- 本地 CLI 错误（如 `unknown command` / `unknown flag` / 认证 / recovery）仍按 root `dws` / `dws-shared` 的错误处理执行；`devdoc` 用于开放平台业务错误码和接口语义排查。
- `devdoc` 只查钉钉开放平台开发者文档，不查业务数据；排查结论必须基于命中条目的标题、摘要或链接，不能编造错误原因或不存在的命令。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "找张三 / 张三是谁" | `dws aisearch person --keyword "张三" --dimension name` |
| "谁负责 XX / XX 负责人是谁" | `dws aisearch person --keyword "<XX>" --dimension duty` |
| "张三的上级 / 下级" | `dws aisearch person --keyword "张三" --dimension supervisor`（或 `subordinate`） |
| "X 部门有哪些人" | `dws aisearch person --keyword "<部门>" --dimension department` |
| "工号 12345 是谁 / 138xxxx 手机号是谁" | `dws aisearch person --keyword "<工号>" --dimension jobNumber` / `dws aisearch person --keyword "<手机号>" --dimension phone` |

## 评测高频硬约束

- 搜索目标必须完整保真：姓名、工号、手机号、部门名按用户原文完整传入 `--keyword`，严禁自行截断、拆字、改昵称或扩展同音字。
- 首次未命中时最多换维度重试一次（如 name → department/jobNumber/phone），仍必须保留完整目标值；不要用半截姓名扩大搜索。
- 找到候选后，如用户要邮箱、部门、职位、主管等详情，必须切到 `dingtalk-contact` 执行 `contact user get --ids <userId> --format json` 补全。
- 多候选且无法唯一判断时输出候选并询问；不要默认取第一个，也不要编造未返回的人员信息。
- 所有 `dws aisearch` 命令加 `--format json`。

## 跨产品协作

- 拿到 userId 后查详情 / 部门 → 切到 `dingtalk-contact`
- 拿到 userId 发消息 → 切到 `dingtalk-chat`
- 拿到 userId 发 DING → 切到 `dingtalk-ding`
