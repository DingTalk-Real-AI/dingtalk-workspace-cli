# 开放平台文档 (devdoc) 命令参考

搜索钉钉**开放平台**开发文档，用于回答开发者关于 OpenAPI、字段、错误码、接入指南、配额等技术问题。

## 命令总览

### 搜索开发文档
```
Usage:
  dws devdoc article search [flags]
Example:
  dws devdoc article search "MCP"
  dws devdoc article search --query "OAuth2 接入"
  dws devdoc article search --keyword "机器人" --size 10
  dws devdoc article search --query "消息卡片" --page 2 --size 5
Flags:
      --query string     搜索关键词 (必填)
      --keyword string   搜索关键词 (--query 的别名)
      --page int         分页页码 (从 1 开始，默认 1)
      --size int         分页大小 (默认 10)
```

### 错误排查
```
Usage:
  dws devdoc error diagnose [flags]
  dws devdoc error troubleshoot [flags]
Example:
  dws devdoc error diagnose --request-id 15r6h45w0muec
  dws devdoc error diagnose --trace-id 15r6h45w0muec --api "创建日程"
  dws devdoc error diagnose --error-code 33012 --error-message "missing scope"
  dws devdoc error diagnose --query "机器人回调失败" --context "HTTP 403"
Flags:
      --query string           原始排查问题
      --request-id string      开放平台 requestId
      --trace-id string        requestId 的兼容别名
      --error-code string      错误码
      --error-message string   错误描述，会合并进原始问题
      --api string             API 名称，会合并进原始问题作为补充检索词
      --context string         额外排查上下文，会合并进原始问题
      --page int               分页页码 (从 1 开始，默认 1)
      --size int               分页大小 (默认 10)
```

## 意图判断

用户问开放平台 API / 字段 / 错误码 / SDK / 鉴权 / 回调 / 配额相关的技术细节:
- 走 `devdoc article search`，把用户问的关键短语作为位置参数或 `--query`

用户已经提供 requestId / traceId / 错误码 / 错误描述 / 失败上下文:
- 走 `devdoc error diagnose`，优先传 `--request-id`，没有 requestId 时传 `--error-code`、`--error-message`、`--query` 或 `--context`

关键区分:
- devdoc(钉钉**开放平台**开发者文档，面向研发) vs doc(钉钉在线文档，面向普通用户内容)
- devdoc 只做搜索，不做读取；命中条目返回标题、摘要、文档链接，由 Agent 引用链接或进一步浏览
- `devdoc error diagnose` 只返回诊断事实、参考资料和链接，不生成 AI 分析结论
- `--api`、`--error-message`、`--context` 是 CLI 侧易用参数，调用 MCP 时会合并到 `query`；MCP 入参只发送 `query`、`requestId`、`errorCode`、`page`、`size`

## 核心工作流

```bash
# 开发者问"OAuth2 怎么接"
dws devdoc article search --query "OAuth2 接入" --format json

# 简短关键词可直接作为位置参数
dws devdoc article search "MCP" --format json

# 命中结果多时翻页
dws devdoc article search --query "消息卡片" --page 2 --size 5 --format json

# 查错误码 / 字段含义
dws devdoc article search --query "errcode 40078" --format json

# 已经有 requestId 时排查
dws devdoc error diagnose --request-id 15r6h45w0muec --format json

# 只有 traceId 时按 requestId 兼容处理
dws devdoc error diagnose --trace-id 15r6h45w0muec --api "创建日程" --format json

# 只有错误码和错误描述时排查
dws devdoc error diagnose --error-code 33012 --error-message "missing scope" --format json
```

## 注意事项

- 关键词必填；可用位置参数、`--query` 或兼容别名 `--keyword`。建议传用户原话里的关键名词（API 名、错误码、能力名），不要过度改写
- 错误排查至少提供 `--query`、`--request-id`、`--error-code`、`--error-message`、`--context` 之一；单独 `--api` 只作为补充上下文，不足以发起排查
- 返回按相关性排序，默认 `--size 10`；要拿更多结果时先翻页，再考虑换关键词
- 命中结果里的链接是钉钉开放平台公开文档，可直接给用户做参考
- 不要把 devdoc 用来查业务数据（那是 aitable / doc / report 的事）；devdoc 只查**官方开发者文档**
