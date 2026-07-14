---
name: dingtalk-wiki
description: 钉钉知识库（Wiki 空间）。Use when 用户说 知识库/wiki/创建知识库/搜索知识库空间/我的文档/知识库归档。Distinct from dingtalk-doc(单文档编辑)、dingtalk-drive(钉盘文件)。命令前缀：dws wiki。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉知识库 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[wiki.md](references/wiki.md)。

## 意图表

`dws wiki` 三个命令族：`space`（知识库容器）、`node`（库内节点：文档/文件夹/表格等）、`member`（容器级成员）。

| 用户说 | 命令 |
|--------|------|
| "创建知识库" | `dws wiki space create --name "<名称>" [--desc "<描述>"]` |
| "查看知识库详情" | `dws wiki space get --workspace <workspaceId>` |
| "搜索知识库空间" | `dws wiki space search --query "<关键词>" [--limit <1-20>]` |
| "我的文档 / 个人知识库" | `dws wiki space list --type myWikiSpace` |
| "列出组织知识库" | `dws wiki space list [--type orgWikiSpace] [--limit <1-50>]` |
| "删除知识库" | `dws wiki space delete --workspace <workspaceId>`（不可逆，先确认）|
| "知识库里有哪些文档/浏览内容" | `dws wiki node list --workspace <workspaceId> [--folder <nodeId>]` |
| "在知识库里创建文档/文件夹" | `dws wiki node create --workspace <workspaceId> --name "<名>" [--type folder\|axls\|...]` |
| "空间内搜文档" | `dws wiki node search --workspace <workspaceId> --query "<关键词>"` |
| "复制 / 移动库内节点" | `dws wiki node copy` / `dws wiki node move --workspace <workspaceId> --node <nodeId> [--folder <targetId>]` |
| "删除库内节点" | `dws wiki node delete --workspace <workspaceId> --node <nodeId>`（不可逆，先确认）|
| "把知识库分享给某人 / 加成员" | `dws wiki member add --workspace <WS_ID> --users <UID> --role EDITOR` |
| "改成员角色" | `dws wiki member update --workspace <WS_ID> --users <UID> --role <ROLE>` |
| "移除知识库成员" | `dws wiki member remove --workspace <WS_ID> --users <UID>` |
| "查看知识库成员" | `dws wiki member list --workspace <WS_ID>` |

## 评测高频硬约束

- `space search` / `node search` 关键词 flag 是 `--query`（`--keyword` 是遗留别名，仍可跑通，但优先用 `--query`）；`space create --desc`、`space list --cursor` 同理（`--description` / `--page-token` 为旧别名）。
- 按类型列出空间优先走 `space list --type myWikiSpace/orgWikiSpace`；用户说"我的文档/个人空间"时用 `dws wiki space list --type myWikiSpace --format json`。
- 用户给空关键词时，不要构造空 `--query ""`；若语义是我的文档则走 `space list --type myWikiSpace`，否则请用户补关键词。
- `node create --type` 服务端支持 `adoc / axls / able / appt / adraw / amind / folder`；**`asheet` 不支持**，创建在线表格用 `axls`。
- `member` 的 `--users` 是复数、逗号分隔；`--role` 大小写不敏感（`editor` 与 `EDITOR` 都行）。`member list` 只返回 `name/role/type`，**不含 userId**，拿不到 userId 去串联 update/remove，需用 `dws contact user search --query "<姓名>"` 反查。
- 知识库内节点的**内容读写**（读取/编辑文档正文）切到 `dingtalk-doc`（先 `node list`/`node search` 拿 nodeId，再 `dws doc read --node <nodeId>`）；不要在 `wiki` 下编造 doc 子命令。
- 所有 `dws wiki` 命令加 `--format json`。

## 跨产品协作

- 知识库内文档内容读写 → 切到 `dingtalk-doc`
- 文件存储 / 全局搜索 → 切到 `dingtalk-drive`
