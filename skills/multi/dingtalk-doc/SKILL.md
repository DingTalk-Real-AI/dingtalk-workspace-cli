---
name: dingtalk-doc
description: 钉钉文档（云文档）。Use when 用户说 写文档/读文档/创建文档/编辑文档/搜文档/文档块/分块编辑/Markdown 写入/上传文件到文档。Distinct from dingtalk-drive(钉盘文件存储)、dingtalk-aitable(数据表格)、dingtalk-wiki(知识库空间)。命令前缀：dws doc。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 钉钉文档 Skill

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性可能因企业服务发现配置而异**。本文档列出的命令基于 dws envelope schema 与本仓库 v1.0.30 实测，但部分命令的 cobra 子命令暴露与否还取决于你的企业 MCP gateway 是否注册了对应 tool。如果跑某条命令报 `unknown command` 或 fall back 到父级 help，说明当前账号企业未开通该能力。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[doc.md](references/doc.md)；剧本：[04-document.md](references/04-document.md)。

## 参数硬约束

- 创建文档只用 `--name`，不要写 `--title`。
- 目标文件夹只用 `--folder <文档文件夹nodeId或URL>`，不要写 `--parent` / `--parent-node` / `--parent-id`。
- 目标知识库只用 `--workspace <workspaceId或URL>`，不要写 `--space-id` / `--spaceId`。
- 文档内容：`create` 只接 `--markdown`，不要写 `--content` / `--content-file`；`update` 只接 `--content` / `--content-file`，不要写 `--markdown`。
- 复杂内容（换行、表格、代码块、长 Markdown）先写临时 `.md`，再用 `--content-file`，不要把大段 Markdown 塞进命令行。
- 每次 `create` / `update` / `block insert` / `media insert` 后必须 `dws doc read` 或 `dws doc block list` 回读关键内容。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "创建文档（短内容）" | `dws doc create --name "<标题>" --markdown "<内容>"` |
| "创建+写入（长内容自动分块）" | `python scripts/doc_create_and_write.py --name "<标题>" --content "<内容>" [--mode append\|overwrite]` |
| "搜文档" | `dws doc search --query "<关键词>"` |
| "读文档内容" | `dws doc read --node <nodeId>` |
| "更新文档内容 / 分块追加" | `dws doc update --node <nodeId> --content "<分块>" --mode append` |
| "删除块" | `dws doc block delete`（需用户确认） |

## 评测/多步文档短路径

- 知识库「评测记录」下按日期文件夹执行：`dws wiki space search --keyword "评测记录" --format json` → `dws doc list --workspace <WS_ID> --format json` → 找 `评测-doc-YYYYMMDD`；不存在则 `dws doc folder create --name "评测-doc-YYYYMMDD" --workspace <WS_ID> --format json`。
- 在目标文件夹创建文字文档：`dws doc create --name "<标题>" --folder <FOLDER_NODE_ID> --markdown "$(cat <tmp.md>)" --format json`。拿到 `nodeId` 后立即回读。
- 块级编辑固定顺序：`doc block list --node <nodeId>` → 选 `blockId` → `doc block insert/update/delete` → `doc block list` 验证。删除块必须已有用户明确删除意图或二次确认。
- 插入引用块、代码块、表格、分栏、附件、图片时，优先读 [doc.md](references/doc.md) 对应小节，不要只停在"准备查看 help"。说出"我将插入..."后必须立即执行对应 terminal 调用。
- 用户要求多个子文档/附件/块操作时，按 checklist 串行完成；最后一条 assistant 消息不能停在"接下来我要..."，必须有实际工具调用或明确失败原因。
- 用户说"读取并下载/导出"时，统一用 `doc download --node ... --output <path>`（开源 v1.0.30 无 `doc export` 命令）。
- 所有 dws 命令带 `--format json`；仅参数不确定时查 `--help`，不要把完整 help 当成最终结果。

## 危险操作

`block delete` 不可逆，必须确认再加 `--yes`。

## 跨产品协作

- 文件存储 / 上传下载 → 切到 `dingtalk-drive`
- 知识库空间管理 → 切到 `dingtalk-wiki`
- 数据表 → 切到 `dingtalk-aitable`
- 长篇报告生成（多源采集 + 写文档）→ 此 skill 提供 `doc_create_and_write.py` 脚本
