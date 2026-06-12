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

### RAG 检索能力

`dws devdoc article search` 调用开放平台文档 RAG 工具 `search_open_platform_docs_rag`，适合查官方开发文档、OpenAPI 入参/出参、字段含义、鉴权流程、回调配置、配额限制和 SDK 接入说明。

使用原则:
- 保留用户原话中的精确标识：API 名、OpenAPI 路径、字段名、errcode、scope、事件名、回调名
- 用户问题很宽泛时，先用场景 + 能力名检索；命中太多再加 API 名、错误码或字段名缩小范围
- 返回结果是 RAG 命中的标题、摘要、材料和链接；优先引用高相关结果，不要把摘要改写成未验证的官方结论
- 如果第一页结果弱相关，先换更精确关键词；需要更多候选时再调整 `--page` / `--size`
- devdoc 只查开放平台开发者资料，不查企业内文档、业务数据或用户自己的钉钉云文档

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

错误排查输入优先级:

| 已知信息 | 推荐命令 | 说明 |
|----------|----------|------|
| requestId / traceId | `dws devdoc error diagnose --request-id <ID>` | 最优先；`--trace-id` 是兼容别名，最终按 `requestId` 传给 MCP |
| 错误码 + 错误描述 | `dws devdoc error diagnose --error-code <CODE> --error-message "<MSG>"` | 错误码进 `errorCode`，错误描述合并进 `query` |
| API 名 + 失败现象 | `dws devdoc error diagnose --query "<现象>" --api "<API名>" --context "<上下文>"` | `--api` 只补充检索词，不能单独发起排查 |
| 只有宽泛描述 | `dws devdoc error diagnose --query "<用户原话>"` | 保留原始报错文本，必要时让用户补 requestId 或错误码 |

关键区分:
- devdoc(钉钉**开放平台**开发者文档，面向研发) vs doc(钉钉在线文档，面向普通用户内容)
- devdoc 只做搜索，不做读取；命中条目返回标题、摘要、文档链接，由 Agent 引用链接或进一步浏览
- `devdoc article search` 底层工具是 `search_open_platform_docs_rag`；`devdoc error diagnose` 底层工具是 `search_open_error_code_rag`
- `devdoc error diagnose` 只返回诊断事实、参考资料和链接；CLI 本身不生成 AI 分析结论
- `devdoc error troubleshoot` 是 `devdoc error diagnose` 的别名，行为一致
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

# 查 RAG 能力材料，保留精确字段名
dws devdoc article search --query "openConversationId 群消息回调" --format json

# 已经有 requestId 时排查
dws devdoc error diagnose --request-id 15r6h45w0muec --format json

# 只有 traceId 时按 requestId 兼容处理
dws devdoc error diagnose --trace-id 15r6h45w0muec --api "创建日程" --format json

# 只有错误码和错误描述时排查
dws devdoc error diagnose --error-code 33012 --error-message "missing scope" --format json

# 现象 + API + 运行上下文一起排查
dws devdoc error troubleshoot --query "机器人回调失败" --api "消息回调" --context "HTTP 403" --format json
```

## 返回结果处理

- 面向用户回答时，先说结论置信度，再列出命中的官方参考链接；如果结果只提供参考材料，应明确这是基于 RAG 命中材料的判断
- 遇到 requestId 查询无结果，不要编造服务端原因；请用户补充错误码、接口名、调用时间、请求参数摘要，或改用错误码/现象继续查
- 遇到权限、scope、回调、签名、IP 白名单、频控类问题，优先把官方材料里的检查项整理成可执行排查清单
- 不要因为一次无命中就反复调用同一查询；最多换 1-2 个更精确关键词，仍无结果则如实说明未找到可靠材料

## 注意事项

- 关键词必填；可用位置参数、`--query` 或兼容别名 `--keyword`。建议传用户原话里的关键名词（API 名、错误码、能力名），不要过度改写
- 错误排查至少提供 `--query`、`--request-id`、`--error-code`、`--error-message`、`--context` 之一；单独 `--api` 只作为补充上下文，不足以发起排查
- 返回按相关性排序，默认 `--size 10`；要拿更多结果时先翻页，再考虑换关键词
- 命中结果里的链接是钉钉开放平台公开文档，可直接给用户做参考
- 不要把 devdoc 用来查业务数据（那是 aitable / doc / report 的事）；devdoc 只查**官方开发者文档**
