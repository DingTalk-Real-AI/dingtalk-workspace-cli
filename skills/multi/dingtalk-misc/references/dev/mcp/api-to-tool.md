# 从 API 材料生成 MCP 工具

适用于用户提供 OpenAPI/Swagger、Postman Collection、curl 或接口文档，并要求“做成 MCP”“让 Agent 调用”或“把接口变成工具”。本文只覆盖 HTTP 工具；HSF 工具先执行 `dws dev mcp hsf method-list`，再使用 `tool create-hsf`。

## 开始前对齐

缺少以下任一项时先询问，不猜字段或鉴权：

1. API 材料：OpenAPI、Postman、curl 或完整接口文档至少一种。
2. 业务目标：Agent 要完成什么动作，用于决定工具拆分和描述。
3. 鉴权方式：无鉴权、Basic、静态 API key、动态 token 或自定义签名。
4. 调试资源：写接口必须由用户指定可安全操作的测试对象。

密钥不能进入工具定义。静态 API key、动态 token 和签名均使用服务级鉴权与凭证，见 [auth-and-credentials.md](auth-and-credentials.md)。

## 材料提取

| 材料 | 提取规则 |
|---|---|
| OpenAPI / Swagger | `paths.{path}.{method}` 形成候选动作；`operationId` 作为工具名素材；`parameters` 按 query/path/header 分组；`requestBody` 进入 body；成功响应 Schema 进入 `apiOutputs.body`；`servers[].url + path` 形成 URL |
| Postman Collection | `item[].request` 提取 method、URL、headers 和 body；item 名称形成标题；example/response 形成调试输入和 `apiOutputs` 素材 |
| curl | `-X` 提取 method；URL query 拆入 `apiInputs.query`；`-H` 拆入 headers；`-d`、`--data`、`--form` 拆入 body |
| 文档文本 | 逐接口提取；字段类型、必填性或响应层级不明确时询问用户 |

只读接口可先直接取一次样本，用真实响应反推 `apiOutputs`，但不得把响应中的 token、cookie、签名 URL 或个人数据写入文档。

## 三段式定义

HTTP `tool create/update` 使用三组相互对应的事实：

| 层 | 作用 |
|---|---|
| `apiInputs` / `apiOutputs` | 下游 HTTP 接口的真实字段树 |
| `toolInputs` / `toolOutputs` | Agent 可见的稳定字段树，可裁剪、改名并补充防呆描述 |
| `inputMappings` / `outputMappings` | 工具字段、系统上下文与 HTTP 字段之间的映射 |

字段节点使用 `{key,title,type,required,description,children}`。支持 `string/number/integer/boolean/object/array`；array 的 `children` 必须且只能有一个 `key=items` 节点。

`--api-inputs` 的每个位置是字段数组，不是字段对象：

```json
{
  "query": [{"key":"name","type":"string","description":"城市名"}],
  "body": [],
  "path": [],
  "headers": []
}
```

不要写成 `{"query":{"name":{...}}}`，CLI 会以“必须是字段数组”拒绝。

## Agent 侧加工

- 裁剪分页游标、固定控制位和内部参数；固定值使用 `fixed` 映射。
- 把不友好的 API 字段名改成稳定语义名，再用 `reference` 映射连接。
- 平台字段模型不承载 enum/default/example 时，把枚举、默认值和正反例写进 `description`。
- `corpId/userId` 等身份字段使用 `$.system_node.*`，不要暴露给 Agent 输入。
- 一个语义动作一个工具，不要机械地让一个 endpoint 永远等于一个工具。
- 有前后依赖时，在工具描述中说明前置工具以及后续参数的来源。

## 设计评审表

执行写命令前，先向用户展示每个工具的完整设计：

| 项 | 要求 |
|---|---|
| `name` | snake_case，包含明确动作词，最多 32 字符 |
| `title` | 简短中文标题 |
| `description` | 功能、参数来源、输出、适用场景和副作用 |
| `httpInfo` | method、URL、内联 NO_AUTH/BASIC 或服务级鉴权关系 |
| `apiInputs` / `toolInputs` | 真实字段树与 Agent 投影 |
| `inputMappings` | 每个工具输入、固定值和系统变量的去向 |
| `apiOutputs` / `toolOutputs` | 真实响应字段树与 Agent 投影；不精修时 `toolOutputs=[]` |
| `outputMappings` | 整体透传或字段级精修，二选一 |
| 调试输入 | 来自材料或用户提供的安全样本，不能用空 `{}` 走过场 |

映射写法见 [mapping-rules.md](mapping-rules.md)。

## 创建与验证闭环

1. `service create --dry-run`，展示预览并取得用户明确确认后执行同命令 `--yes`。
2. 先创建最简单的一个工具，随后 `tool get` 回读字段树和映射。
3. 首个工具结构正确后再创建其余工具，每个工具都回读。
4. 草稿调试必须使用 `tool get` 返回的 `versionId`；无鉴权传 `--no-credential`，有鉴权传 `--credential-id`。
5. `tool debug` 必须返回符合业务预期的真实数据，不能只看“未报错”。
6. HTTP `tool update` 是全量提交，必须先 `tool get`，在完整定义上修改后整包提交。
7. 所有工具调试通过后，列出待发布清单并取得发布确认，再逐个发布。
8. 发布后执行 `dws mcp published tools <mcpId>`，确认实时工具名和 `inputSchema`。
9. 使用 `published invoke ... --dry-run` 展示调用，用户明确确认后再真实调用。

任何写操作的目标或参数发生变化，都必须重新 dry-run 和确认。
