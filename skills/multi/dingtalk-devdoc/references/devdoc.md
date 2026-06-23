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
  dws devdoc article search --query "消息卡片" --cursor "<NEXT_CURSOR>" --size 5
Flags:
      --query string     搜索关键词 (必填)
      --keyword string   搜索关键词 (--query 的别名)
      --page int         分页页码 (从 1 开始，默认 1)
      --cursor string    分页游标，翻页传上次返回的 nextCursor；传入后不再使用 --page
      --size int         分页大小 (默认 10)
```

### 排查调用错误
```
Usage:
  dws devdoc error diagnose [flags]
Example:
  dws devdoc error diagnose --error-code "40014" --query "获取用户信息 access_token"
  dws devdoc error diagnose --request-id "<REQUEST_ID>" --query "接口调用失败"
  dws devdoc error troubleshoot --error-message "token is illegal" --api "获取用户信息"
Flags:
      --query string           原始排查问题
      --error-code string      错误码
      --error-message string   错误描述，会合并进原始问题
      --api string             API 名称，会合并进原始问题
      --context string         额外排查上下文
      --request-id string      开放平台 requestId
      --page int               分页页码 (从 1 开始，默认 1)
      --cursor string          分页游标，翻页传上次返回的 nextCursor；传入后不再使用 --page
      --size int               分页大小 (默认 10)
```

## 意图判断

用户问开放平台 API / 字段 / 错误码 / SDK / 鉴权 / 回调 / 配额相关的技术细节:
- 走 `devdoc article search`，把用户问的关键短语作为位置参数或 `--query`
- 用户已经给出错误码、错误描述或 requestId:
- 先走 `devdoc error diagnose`；若后端返回 `PARAM_ERROR - 未找到指定工具` / `unknown tool`，降级 `devdoc article search`，同时把该结果标记为 `needs_gateway_tool_registration`

关键区分:
- devdoc(钉钉**开放平台**开发者文档，面向研发) vs doc(钉钉在线文档，面向普通用户内容)
- devdoc 只做搜索/诊断，不做读取；命中条目返回标题、摘要、文档链接，由 Agent 引用链接或进一步浏览

## 核心工作流

```bash
# 开发者问"OAuth2 怎么接"
dws devdoc article search --query "OAuth2 接入" --format json

# 简短关键词可直接作为位置参数
dws devdoc article search "MCP" --format json

# 命中结果多时翻页
dws devdoc article search --query "消息卡片" --page 2 --size 5 --format json

# 推荐动态翻页：优先使用上一页返回的 nextCursor
dws devdoc article search --query "消息卡片" --cursor "<NEXT_CURSOR>" --size 5 --format json

# 查错误码 / 字段含义
dws devdoc article search --query "errcode 40078" --format json

# 有明确错误码时优先走错误诊断
dws devdoc error diagnose --error-code "40014" --query "获取用户信息 access_token" --format json

# 有 requestId 时尝试链路排查；不是所有接口都会返回 requestId
dws devdoc error diagnose --request-id "<REQUEST_ID>" --query "接口调用失败" --format json

# 错误排查结果继续翻页
dws devdoc error diagnose --error-code "40014" --query "access_token" --cursor "<NEXT_CURSOR>" --format json
```

## 上线/注入验收

```bash
dws devdoc --help
dws devdoc article search --query "OAuth2 接入" --dry-run --format json
dws devdoc error diagnose --error-code "40014" --query "access_token" --dry-run --format json
dws schema devdoc.search_open_platform_docs_rag --format json || true
dws schema devdoc.search_open_error_code_rag --format json || true
```

- `--help` / `--dry-run` 只能证明 CLI 命令面和映射存在，不能证明后端工具已注册
- 真实调用若返回 `PARAM_ERROR - 未找到指定工具`，不是用户参数问题；记录为网关/工具注册待闭环，并降级到 article search 或本地已挂载 Markdown

## 注意事项

- 关键词必填；可用位置参数、`--query` 或兼容别名 `--keyword`。建议传用户原话里的关键名词（API 名、错误码、能力名），不要过度改写
- 返回按相关性排序，默认 `--size 10`；响应有 `hasMore=true` 时，用 `nextCursor` 传给 `--cursor` 继续翻页，不要手写猜测下一页
- 命中结果里的链接是钉钉开放平台公开文档，可直接给用户做参考
- 不要把 devdoc 用来查业务数据（那是 aitable / doc / report 的事）；devdoc 只查**官方开发者文档**
