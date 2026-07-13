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

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。22 个 dingtalk-* skill 全部通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[drive.md](references/drive.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "看钉盘文件 / 文件夹列表" | `dws drive list --limit 20 [--folder <fileId>]` |
| "按名字搜文件（不知道在哪）" | `dws drive search --query "<关键词>"` |
| "看钉盘空间 / 团队文件 / 有哪些 space" | `dws wiki space list --type orgSpace`（`drive list-spaces` 已 deprecated） |
| "最近访问 / 最近编辑的文档" | `dws drive recent` |
| "钉盘目录树" | `python scripts/drive_tree_list.py --depth 2` |
| "查文件元数据" | `dws drive info --node <fileId>` |
| "查阅读/编辑/评论/下载等节点统计" | `dws drive stats --node <fileId>` |
| "创建文件快捷方式" | `dws drive shortcut --node <fileId> [--folder <targetFolderId>] [--workspace <workspaceId>]` |
| "下载文件" | `dws drive download --node <fileId> --output <path>` |
| "上传本地文件" | `dws drive upload --file ./report.pdf [--folder <fileId>]` |
| "建文件夹" | `dws drive mkdir --name "<名称>" [--folder <fileId>]` |
| "复制 / 移动 / 重命名" | `dws drive copy` / `move` / `rename --node <fileId> --name "<主名>"` |
| "删除文件 / 移到回收站（需确认）" | `dws drive delete --node <fileId> --yes` |
| "回收站 / 还原" | `dws drive recycle list` / `recycle restore --id <recycleItemId>` |
| "公开 / 取消公开 / 查公开状态" | `dws drive publish set` / `unset` / `get --node <fileId>` |

## 评测高频硬约束

- 找文件优先用 `drive search --query "<关键词>"`（不知道位置时全局搜）；只有需要逐层浏览时才用 `drive list`。命中后必须 `drive info --node <fileId> --format json` 回读元数据。
- **ID 字段选择**：`drive list` 返回同时有 `dentryId`（纯数字）和 `fileId`（UUID 格式）。所有 `--node` 和 `--folder` 参数**必须用 `fileId`**，纯数字 `dentryId` 会被拒绝。
- `drive list` 默认 `--limit 20`，最大 50；要更多用 `--cursor` 翻页，不要因参数边界报错反复重试。
- `rename` 的 `--name` **只传主名，不带扩展名**；服务端会按原扩展名自动补后缀，带了扩展名会变成双扩展名（如 `报告.txt` → `报告.txt.txt`）。
- `drive download` 需要 `--output` 指定本地保存路径或目录；不要省略必填输出位置。
- 删除、覆盖、移动、公开（publish set/unset）等破坏性操作必须先确认；上传、创建文件夹、下载后要读回或列目录验证。
- `shortcut` 会创建新节点，执行后必须用 `drive list` 回读目标位置；`stats` 是只读操作。
- 所有 `dws drive` 命令加 `--format json`。

## 跨产品协作

- 文件内容编辑（钉钉文档）→ 切到 `dingtalk-doc`
- 知识库空间 → 切到 `dingtalk-wiki`
