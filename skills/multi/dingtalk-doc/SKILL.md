---
name: dingtalk-doc
description: 钉钉在线文字文档（adoc）：创建、读取、追加、覆盖、块级编辑、评论、附件、导入导出、版本、模板、封面/背景，以及 Markdown/JSONML 保真写入。Use when 用户要写文档、读文档、改正文、处理富文本块或文档评论。原生 .md 文件走 dingtalk-markdown；普通文件存储与上传下载走 dingtalk-drive；知识库空间/节点管理走 dingtalk-wiki；电子表格走 dingtalk-sheet，AI 表格走 dingtalk-aitable。命令前缀：dws doc。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉文档 Skill

## Preconditions

> **CRITICAL — Before any `dws` operation, MUST fully read [`dws-shared`](../dws-shared/SKILL.md).** It defines the global execution contract, safety floor, and on-demand shared-reference routing. Do not preload all references.

> Atomic command router: [doc.md](references/doc.md); document workflows: [04-document.md](references/04-document.md).

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcut 发现（按需）

`doc` 当前有 17 条公开 Shortcut，已全部进入 Runtime Schema。完整清单保留在 Runtime Shortcut Catalog，根 Skill 不重复展开；单条参数与安全契约按需查询 leaf Schema。已知意图直接使用下方的优先路由、意图表或任务 reference；命令已选中时直接执行，只在参数/安全语义不确定时读取 leaf Schema，在当前 Cobra flags 不确定时读取 leaf Help。

仅当现有路由和 reference 都无法定位低频能力时，才执行 `dws shortcut list --service doc --compact --format json` 做最后回退；不要为已知高频意图加载完整 Shortcut Catalog 或产品级 Schema。
<!-- VISIBLE_SHORTCUTS_END -->

## Atomic 回退与执行契约

没有 Shortcut 时，才按需读取 [doc.md](references/doc.md) 指向的一个 atomic branch reference。路由优先级为 `已评审直接骨架/精确 recipe > 匹配的公开 Shortcut > atomic fallback`。`doc_create_and_write.py` 只在用户明确需要可复用本地包装器且 `python3` 可用时使用；普通创建直接调用 `dws doc create`。

命令确定且参数清楚时直接执行，不重复发现。只使用真实 `cli_path`，不猜参数。`confirmation=user_required` 时先确认，再添加 `--yes`；来源冲突时采用更安全解释并报告契约漂移。

## 核心对象、位置与格式

| 对象 | 核心标识与边界 |
|---|---|
| 文档 | 使用真实 `nodeId` / `dentryUuid` 或完整 alidocs URL；纯数字 `dentryId`、单独 `dentryKey` 不能替代 |
| 目标位置 | `--folder` 只接文档文件夹 nodeId/URL；`--workspace` 只接知识库 ID/URL；不要猜 `--parent*` |
| 块 | `blockId` / JSONML `uuid` 必须来自 `block list`，更新节点的 uuid 必须与目标块一致 |
| 评论 | `commentKey` 来自评论 list/create；划词评论还需同一块的真实 `start/end` |
| 异步任务 | 导出 `jobId` 与导入 `taskId` 只查询对应任务，不能替代 nodeId |
| 内容格式 | Markdown 适合线性正文；已有富结构优先 JSONML/块级编辑，禁止用 Markdown overwrite 误称保真 |
| 普通文件 | adoc 才用 `doc read/export`；`.md`、axls、able 和普通文件按真实 `extension` 切对应 Skill |

## 核心意图与执行骨架

所有结构化命令加 `--format json`，下游 ID 只取真实输出。创建、更新、块、附件和样式写入后回读；返回成功但未回读，不能宣称内容完整。

| 用户意图 | 精确骨架 | 必须保留的执行边界 |
|---|---|---|
| 按名称找文档 | `+find-doc --query <关键词>`；需最近访问/扩展名/创建者等过滤用 `+search` | 候选不唯一先消歧；随后 `drive info --node <nodeId>` 判 `extension` |
| 读取 adoc | `drive info --node <nodeId>` → `doc read --node <nodeId>` | 用户已给 nodeId/URL 时不再搜索；非 adoc 不调用 `doc read` |
| 创建文档 | `doc create --name <标题> --content-file <tmp.md> [--folder <folder> | --workspace <ws>]` | 原生写入管道自动分片；取 `nodeId` 后 `doc read`，缺链接再 `doc info` |
| 末尾补短文本 | `+doc-append --doc <nodeId> --text <内容>` | 该 Shortcut 为 write/user_required；确认后执行并 `doc read` 核对 |
| 改写正文 | `doc read --content-format jsonml` → `block update` 或 `doc update --content-format jsonml --mode overwrite` | 单块优先块级编辑；整篇 overwrite 先预览/确认，Markdown overwrite 不保富结构 |
| 评论与回复 | `+comment-list --node <nodeId>` → `+comment-create` / `+comment-reply` | `commentKey` 来自真实结果；写 Shortcut 先确认；划词评论走 atomic `comment create-inline` |
| 导入 / 导出 | `doc import --file <path> ...` / `doc export --node <nodeId> --export-format <fmt> --output <path>` | 一体化命令优先；仅超时/中断后用 `import get` / `export get` |
| 版本操作 | `+version-list --node <nodeId>` → `+version-save` / `+version-revert --version <N>` | save/revert 先确认；revert 版本号必须来自 list，完成后回读 |
| 模板创建 | `+template-list` / `+template-search --query <词>` → atomic `template apply --template-id <id>` | templateId 来自真实列表；要复刻已有文档形态时用 drive copy + 副本块级更新 |
| 分享链接给某人 | `+share-doc --to <姓名> --url <docUrl> [--note <附言>]` | 会真实发消息，确认后执行；同名人员必须消歧，不改变文档权限 |

## 写入与验证边界

- 创建只用 `--name`；内容只用 `--content` / `--content-file`。长、多行、表格或特殊字符必须用临时 UTF-8 文件和 `--content-file`。
- 原生 Markdown 写入管道在内容超过 10,000 个 Unicode 字符时自动按结构分片；不要在 Skill 或脚本中预先复制分片循环。仅在 `CONTENT_TRUNCATED`、中断或回读缺失时按 [04-document.md](references/04-document.md) 恢复。
- `doc update --mode append` 不清空原文；`--mode overwrite` 会清空后重写，先 `--dry-run`，得到确认后才加 `--yes`。
- 已有 callout、分栏、样式、@人、图片或附件时，先读 JSONML；局部改动优先 `block update`，不要用 Markdown 整篇重写。
- `block insert` 默认追加；只有明确相对位置时才传真实 `--ref-block` / `--parent-block`。`block delete` 和评论删除必须确认。
- 写后按对象验证：正文用 `doc read`，块/附件用 `doc block list`，元信息/链接用 `doc info`，版本用 `version list`。

## 低频 atomic 路由

```text
dws doc
├── info / read
├── create / update
├── block     # list, insert, update, delete
├── comment   # list, create, reply, update, delete, create-inline
├── media     # insert, download
├── import / export
├── template / version / style
├── +shortcut
└── deprecated file operations → drive/wiki
```

按需读取，不要预加载：定位与 URL 边界读 [doc-info.md](references/doc/doc-info.md)；创建/改写读 [doc.md](references/doc.md) 的场景索引所列文件组；块、评论、附件、导入、导出分别读对应 branch reference。JSONML schema/cookbook 仅在实际选择 JSONML 写入后加载。

## 错误恢复

- 路径或参数错误：按既定顺序查 leaf Schema、再查 leaf Help，校正一次；不要连续尝试近似参数。
- 始终从实际输出重新提取 `nodeId`、`blockId`、`commentKey`、`jobId` 或 `taskId`。
- 部分写入或回读缺失：保留已创建的 nodeId，报告已完成范围与缺失位置；先读回，再只补缺失内容，禁止无条件重新创建副本。
- 权限不足、候选未消歧、目标类型不符、没有可推进的任务 ID 或 Schema/Help 冲突时停止并报告。

## 跨产品协作

- 原生 `.md` 文件内容 → `dingtalk-markdown`。
- 普通文件存储、搜索、复制、移动、重命名、删除、上传下载与权限 → `dingtalk-drive`。
- 知识库空间、节点和成员管理 → `dingtalk-wiki`；`doc create --workspace` 可直接在已知知识库根创建带内容的 adoc。
- 在线电子表格 / AI 表格 → `dingtalk-sheet` / `dingtalk-aitable`。
- 人名消歧 → `dingtalk-aisearch` 或 `dingtalk-contact`；文档评论 mention 使用真实 userId。
- 局部意图边界见 [intent-guide.md](references/intent-guide.md)，固定短流程见 [lite-recipes.md](references/lite-recipes.md)。
