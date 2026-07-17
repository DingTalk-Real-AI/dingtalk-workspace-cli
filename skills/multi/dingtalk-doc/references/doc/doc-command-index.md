# 命令索引表

## 命令索引表

> 命令名 → 单文件，按需加载子文档。复杂任务请优先看下方 §场景索引。

### 阅读 / 元信息

| 命令 | 用途 | 必填参数 | 详见 |
|------|------|----------|------|
| `doc info` | 文档元信息（含 contentType / extension） | `--node` | [`doc/doc-info.md`](./doc-info.md) |
| `doc read` | 读取正文（markdown 或 jsonml） | `--node` | [`doc/doc-read.md`](./doc-read.md) |

### 创建 / 写入 / 块编辑

| 命令 | 用途 | 必填参数 | 详见 |
|------|------|----------|------|
| `doc create` | 创建文字文档（adoc） | `--name` | [`doc/doc-create.md`](./doc-create.md) |
| `doc update` | 整篇 / 段落级更新（markdown / jsonml） | `--node` `--mode` | [`doc/doc-update.md`](./doc-update.md) |
| `doc block list/insert/update/delete` | 块级精细编辑（含 JSONML 节点操作） | `--node` (+ `--block-id`) | [`doc/doc-block.md`](./doc-block.md) |

### 附件 / 评论 / 导出

| 命令 | 用途 | 必填参数 | 详见 |
|------|------|----------|------|
| `doc media insert/download` | 附件 / 图片插入与下载 | `--node` `--file` 或 `--resource-id` | [`doc/doc-media.md`](./doc-media.md) |
| `doc comment list/create/reply/create-inline` | 文档评论与划词评论 | `--node` (+ ...) | [`doc/doc-comment.md`](./doc-comment.md) |
| `doc export` / `doc export get` | 在线文档导出 docx/markdown/pdf | `--node` `--output` | [`doc/doc-export.md`](./doc-export.md) |

### 导入

| 命令 | 用途 | 必填参数 | 详见 |
|------|------|----------|------|
| `doc import` / `doc import get` | 本地文件导入为在线文档（docx→文档 / xlsx→表格 / xmind→脑图 / md→文档） | `--file` | [`doc/doc-import.md`](./doc-import.md) |

### 版本管理

| 命令 | 用途 | 必填参数 | 详见 |
|------|------|----------|------|
| `doc version save` | 手动保存文档版本快照 | `--node` | 本节 |
| `doc version list` | 查看文档历史版本列表 | `--node` | 本节 |
| `doc version revert` | 回滚文档到指定版本 | `--node` `--version` `--yes` | 本节 |

### 模板管理

| 命令 | 用途 | 必填参数 | 详见 |
|------|------|----------|------|
| `doc template list` | 获取文档模板列表 | 无 | 本节 |
| `doc template search` | 搜索文档模板 | `--query` | 本节 |
| `doc template apply` | 应用文档模板创建新文档 | `--template-id` | 本节 |

#### 手动保存文档版本

```
Usage:
  dws doc version save [flags]
Example:
  dws doc version save --node <nodeId>
Flags:
      --node string    文档 ID 或 URL (必填)
```

> 仅支持 adoc 类型文档。保存后会创建一个 USER_SAVE 类型的版本记录。

#### 查看文档历史版本列表

```
Usage:
  dws doc version list [flags]
Example:
  dws doc version list --node <nodeId>
  dws doc version list --node <nodeId> --limit 10
Flags:
      --node string      文档 ID 或 URL (必填)
      --limit int        返回版本数量上限 (可选)
      --cursor string    分页游标 (可选)
```

#### 回滚文档到指定版本

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws doc version revert [flags]
Example:
  dws doc version revert --node <nodeId> --version 3 --yes
Flags:
      --node string      文档 ID 或 URL (必填)
      --version int      目标版本号 (必填，从 version list 获取)
      --yes              跳过确认提示 (非交互终端必须传)
```

#### 获取文档模板列表

```
Usage:
  dws doc template list [flags]
Example:
  dws doc template list
  dws doc template list --source MY
  dws doc template list --source PUBLIC
  dws doc template list --limit 20
Flags:
      --source string    模板来源: MY(我的模版)/PUBLIC(公开模版)，不传默认 MY
      --limit int        返回数量上限 (可选)
      --cursor string    分页游标 (可选)
```

#### 搜索文档模板

```
Usage:
  dws doc template search [flags]
Example:
  dws doc template search --query "周报"
  dws doc template search --query "会议纪要" --limit 10
  dws doc template search --query "项目" --source PUBLIC
Flags:
      --query string     搜索关键词 (必填)
      --source string    模板来源: MY(我的模版)/PUBLIC(公开模版)，不传默认 MY
      --limit int        返回数量上限 (可选)
      --cursor string    分页游标 (可选)
```

#### 应用文档模板

```
Usage:
  dws doc template apply [flags]
Example:
  dws doc template apply --template-id TPL_ID --name "我的周报"
  dws doc template apply --template-id TPL_ID --name "项目方案" --folder FOLDER_ID
Flags:
      --template-id string  模板 ID (必填，从 template list/search 获取)
      --name string         新文档名称 (可选)
      --folder string       目标文件夹 ID (可选)
      --workspace string    知识库 ID (可选)
```

### 文件操作（已迁移，以下命令 deprecated）

> **迁移提示**：文件管理命令已按产品领域架构重新归属，旧命令在过渡期内仍可使用，运行时会输出 deprecated 警告。

| 旧命令 | 推荐命令 | 详见 |
|--------|----------|------|
| `doc upload` | `dws drive upload` | [`drive.md`](../../../dingtalk-drive/references/drive.md) |
| `doc download` | `dws drive download` | [`drive.md`](../../../dingtalk-drive/references/drive.md) |
| `doc copy` | `dws drive copy` | [`drive.md`](../../../dingtalk-drive/references/drive.md) |
| `doc move` | `dws drive move` | [`drive.md`](../../../dingtalk-drive/references/drive.md) |
| `doc rename` | `dws drive rename` | [`drive.md`](../../../dingtalk-drive/references/drive.md) |
| `doc delete` | `dws drive delete` | [`drive.md`](../../../dingtalk-drive/references/drive.md) |
| `doc folder create` | `dws drive folder create` | [`drive.md`](../../../dingtalk-drive/references/drive.md) |
| `doc file create` | `dws wiki node create --type <type>` | [`wiki.md`](../../../dingtalk-wiki/references/wiki.md) |
| `doc permission *` | `dws drive permission *` | [`drive.md`](../../../dingtalk-drive/references/drive.md) |
| `doc list` | `dws drive list --workspace` / `dws wiki node list` | [`drive.md`](../../../dingtalk-drive/references/drive.md) / [`wiki.md`](../../../dingtalk-wiki/references/wiki.md) |
| `doc search` | `dws drive search` / `dws wiki node search` | [`drive.md`](../../../dingtalk-drive/references/drive.md) / [`wiki.md`](../../../dingtalk-wiki/references/wiki.md) |

### 排版规范 / JSONML 参考

| 资源 | 用途 | 详见 |
|------|------|------|
| 创建工作流 | 标题、位置、骨架、回读校验 | [`doc/style/doc-create-workflow.md`](./style/doc-create-workflow.md) |
| 改写工作流 | 编辑形态优先级、分片 append、回读验收 | [`doc/style/doc-update-workflow.md`](./style/doc-update-workflow.md) |
| 排版规范 | 文档类型 / 骨架 / 元素边界 / 颜色语义 | [`doc/style/doc-style-guideline.md`](./style/doc-style-guideline.md) |
| JSONML 范例 | 所有节点类型的可复制命令 | [`doc/format/doc-jsonml-cookbook.md`](./format/doc-jsonml-cookbook.md) |
| JSONML 节点结构 | 字段定义 + JSON Schema | [`doc/format/doc-jsonml-schema.md`](./format/doc-jsonml-schema.md) |
