---
name: dingtalk-drive
description: 钉盘文件存储。Use when 用户说 钉盘/上传文件/下载文件/文件夹/查文件/创建文件夹。Distinct from dingtalk-doc(钉钉文档内容编辑)、dingtalk-wiki(知识库空间)。命令前缀：dws drive。
cli_version: ">=0.2.14"
metadata:
  category: product
  requires:
    bins:
      - dws
---

# 钉盘 Skill

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[drive.md](references/drive.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "看钉盘文件 / 文件夹列表" | `dws drive list --space-id <spaceId> [--parent-id <fileId>]` |
| "钉盘目录树" | `python scripts/drive_tree_list.py --depth 2` |
| "查文件元数据" | `dws drive info --space-id <spaceId> --file-id <fileId>` |
| "下载文件" | `dws drive download --space-id <spaceId> --file-id <fileId>` |
| "上传文件（两步）" | `dws drive upload-info --space-id <spaceId> --file-name <名> --file-size <bytes> [--parent-id <fileId>]` → `dws drive commit --space-id <spaceId> --upload-id <uploadId> --file-name <名> --file-size <bytes> [--parent-id <fileId>]` |
| "建文件夹" | `dws drive mkdir --space-id <spaceId> --name "<名称>" [--parent-id <fileId>]` |

## 评测高频硬约束

- 查找文件不要只看根目录后放弃；根目录没命中时，进入最相关的评测/目标文件夹继续 `drive list --space-id <spaceId> --parent-id <fileId>`，必要时用目录树脚本递归到合理深度。
- `drive list` 默认 `--max 20`，评测里保守使用 `--max 50` 以内并处理 `nextToken` 翻页；不要因为参数边界报错反复重试。
- `dws drive` 当前没有 search 子命令，按目录递归 `drive list`；命中后必须 `drive info --space-id <spaceId> --file-id <fileId> --format json` 回读元数据。
- `drive download` 没有 `--output` flag，文件落地路径由 CLI 决定，必要时再用 shell 移动；不要拼写不存在的 flag。
- 删除、覆盖、移动等破坏性操作必须确认；上传（upload-info + commit 两步）、创建文件夹、下载后要读回或列目录验证。
- 所有 `dws drive` 命令加 `--format json`。

## 跨产品协作

- 文件内容编辑（钉钉文档）→ 切到 `dingtalk-doc`
- 知识库空间 → 切到 `dingtalk-wiki`
