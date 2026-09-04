# MCP 字段与映射规则

本页是 `--input-mappings` 和 `--output-mappings` 的格式指南。HTTP 与 HSF 的路径不同，不能混用。

## 映射结构

```json
{"type":"reference","source":"$.node_start.city_name","target":"$.Query.name"}
```

| 类型 | 字段 | 用途 |
|---|---|---|
| `reference` | `source` + `target` | 引用工具输入、接口输出或系统变量 |
| `fixed` | `source` + `target` | 把固定常量写入接口参数，不暴露给 Agent |
| `express` | `expression` + `target`，可选 `displayText` | 对输入进行计算或格式转换 |

`express` 的表达式必须写在 `expression`，不能写在 `source`。CLI 会拒绝缺少 `expression` 的 express 映射，防止平台静默存成无效规则。

```json
{
  "type":"express",
  "expression":"GET('operateUserId',${@(\"system_node/$\")})",
  "displayText":"读取当前调用用户",
  "target":"$.Query.user_id"
}
```

复杂表达式才读取 [expression-functions.md](expression-functions.md)。普通 API 转换优先使用 reference、fixed 和系统变量。

## HTTP 入参路径

`apiInputs` 的分组键是小写 `query/body/path/headers`，但映射 target 的位置名必须是 Pascal：

| API 位置 | target 前缀 |
|---|---|
| body | `$.Body.` |
| query | `$.Query.` |
| headers | `$.Head.` |
| path | `$.Path.` |

`$.query.name` 和 `$.QUERY.name` 都会在平台侧静默失效；必须使用 `$.Query.name`。

```json
[
  {"type":"reference","source":"$.node_start.city_name","target":"$.Query.name"},
  {"type":"fixed","source":"zh","target":"$.Query.language"},
  {"type":"reference","source":"$.system_node.ddDataCorpId","target":"$.Head.X-Corp-Id"}
]
```

`reference` 的 Agent 输入 source 使用 `$.node_start.<toolInput key>`。固定值的 source 直接写值，不加 `$.`。

## HTTP 出参路径

整体透传：

```json
[{"type":"reference","source":"$.node_service_activator.Body","target":"$"}]
```

整体透传仍必须在同批 `--api-outputs` 中如实声明 Body 字段。映射只会读取声明 Schema 内的值；漏声明会导致字段被裁剪或管理台显示“变量已失效”。

字段级精修：

```json
[
  {
    "type":"reference",
    "source":"$.node_service_activator.Body.data.staff_id",
    "target":"$.user.userId"
  }
]
```

字段级 target 必须在同批 `toolOutputs` 字段树中存在。未映射字段会被裁剪。

## 数组

数组逐元素映射时 source 和 target 都使用 `[*]`，并确保两侧 array 字段都有唯一 `key=items` 子节点：

```json
{
  "type":"reference",
  "source":"$.node_service_activator.Body.items[*].staff_id",
  "target":"$.members[*].userId"
}
```

平台配置页可能要求数组整体和元素级映射成对出现。创建后必须用 `tool get` 回读并通过 `tool debug` 验证实际形状。

## 系统上下文

常用安全系统变量：

| source | 含义 |
|---|---|
| `$.system_node.operateUserId` | 当前调用用户 userId |
| `$.system_node.ddDataCorpId` | 当前调用组织 corpId |
| `$.system_node.deapAgentCode` | Agent code |
| `$.system_node.deapAgentName` | Agent name |
| `$.system_node.deapRunId` | 当前运行 ID，debug 中可能为空 |
| `$.system_node.deapClientSessionId` | 当前会话 ID，debug 中可能为空 |
| `$.system_node.deapScenarioCode` | 场景 code |
| `$.system_node.deapParentAbilityCallSessionId` | 父能力调用会话 ID |

不要让 Agent 手工传 `corpId/userId` 冒充系统身份。服务配置鉴权后可能还提供 `$.system_node.AppKey/AppSecret`，这些值同样不得输出或写入仓库。

## HSF 差异

| 项 | HTTP | HSF |
|---|---|---|
| 入参 target | `$.Body/Query/Head/Path.<字段>` | `$.<DTO简名>.<字段>` |
| 出参 source | `$.node_service_activator.Body...` | `$.node_service_activator.result...`，具体根以方法 Schema 和读回结果为准 |
| 字段权威 | `apiInputs/apiOutputs` | `hsf method-list` 返回的 input/output Schema |
| 身份字段 | 按接口需要注入 | DTO 需要 corpId/userId 时必须显式注入 |

HSF 身份注入示例：

```json
[
  {"type":"reference","source":"$.system_node.ddDataCorpId","target":"$.SearchRequest.corpId"},
  {"type":"reference","source":"$.system_node.operateUserId","target":"$.SearchRequest.userId"}
]
```

HSF `update-hsf` 是部分更新，未传字段保持原值；HTTP `tool update` 是全量更新。`update-hsf --output-mappings '[]'` 会清空映射，不表示“不修改”。

## 发布前检查

1. 每条 reference/fixed 有非空 `source`，每条 express 有非空 `expression`。
2. HTTP target 使用 Pascal 位置名，并在 `apiInputs` 中可解析。
3. output source 在 `apiOutputs` 中声明到最深引用字段。
4. 字段级 output target 在 `toolOutputs` 中可解析。
5. `tool get` 回读的规则数量、类型和路径与设计一致。
6. `tool debug` 的 `toolOutput` 形状和业务值均符合预期。

映射问题常常不会在服务端报错，不能跳过 debug。
