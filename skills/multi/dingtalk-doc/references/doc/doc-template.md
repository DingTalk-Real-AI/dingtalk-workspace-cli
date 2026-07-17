# 模板管理

| 命令 | 用途 | 必填参数 |
|---|---|---|
| `doc template list` | 获取文档模板列表 | 无 |
| `doc template search` | 搜索文档模板 | `--query` |
| `doc template apply` | 应用文档模板创建新文档 | `--template-id` |

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
