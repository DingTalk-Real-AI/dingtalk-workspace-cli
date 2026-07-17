# 模板管理

## 模板管理

当用户说“用模板创建表格”、“搜索表格模板”、“有哪些表格模板”时使用。

### 获取表格模板列表

```
Usage:
  dws sheet template list [flags]
Example:
  dws sheet template list
  dws sheet template list --source MY
  dws sheet template list --source PUBLIC
  dws sheet template list --limit 20
Flags:
      --source string    模板来源: MY(我的模版)/PUBLIC(公开模版)，不传默认 MY
      --limit int        返回数量上限 (可选)
      --cursor string    分页游标 (可选)
```

### 搜索表格模板

```
Usage:
  dws sheet template search [flags]
Example:
  dws sheet template search --query "预算"
  dws sheet template search --query "排班表" --limit 10
  dws sheet template search --query "财务" --source PUBLIC
Flags:
      --query string     搜索关键词 (必填)
      --source string    模板来源: MY(我的模版)/PUBLIC(公开模版)，不传默认 MY
      --limit int        返回数量上限 (可选)
      --cursor string    分页游标 (可选)
```

### 应用表格模板

```
Usage:
  dws sheet template apply [flags]
Example:
  dws sheet template apply --template-id TPL_ID --name "月度预算表"
  dws sheet template apply --template-id TPL_ID --name "排班表" --folder FOLDER_ID
Flags:
      --template-id string  模板 ID (必填，从 template list/search 获取)
      --name string         新表格文档名称 (可选)
      --folder string       目标文件夹 ID (可选)
      --workspace string    知识库 ID (可选)
```
