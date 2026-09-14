# 独立 `meta.pagination` Schema 方案

## 1. 目标结构

业务结果与分页控制信息分层：

```json
{
  "ok": true,
  "outcome": "success",
  "data": {
    "items": [{"id": "a"}]
  },
  "meta": {
    "pagination": {
      "endpoint_exhausted": false,
      "next_token": "cursor-2"
    }
  }
}
```

对应 compact/full leaf Schema：

```json
{
  "result": {
    "outcomes": ["success", "failure"],
    "data_schema": {
      "type": "object",
      "properties": {
        "items": {
          "type": "array",
          "description": "当前页业务记录",
          "items": {"type": "object"}
        }
      }
    }
  },
  "pagination": {
    "kind": "cursor",
    "cursor_parameter": "cursor",
    "meta_path": "meta.pagination",
    "endpoint_exhausted_path": "meta.pagination.endpoint_exhausted",
    "next_token_path": "meta.pagination.next_token"
  }
}
```

`result` 只描述 `data`；`pagination` 是与 `result` 同级的命令能力声明。

## 2. 分页状态

| 状态 | `endpoint_exhausted` | `next_token` | Agent 行为 |
|---|---:|---|---|
| 可续跑 | `false` | 必须非空 | 将 token 传给 `--<cursor_parameter>` |
| 已耗尽 | `true` | 必须省略 | 停止翻页 |

`endpoint_exhausted:true` 只表示观察到 Endpoint 分页耗尽，不表示搜索索引
健康、数据全量覆盖或业务对象不存在。

## 3. 映射规则

产品 mapper 可以读取服务端原始 `hasMore/nextCursor`、`has_more/page_token`
等字段，但统一 CLI 输出只公布 `meta.pagination`：

- 服务端表示还有下一页且 cursor 非空 → `endpoint_exhausted:false` + token。
- 服务端表示没有下一页 → `endpoint_exhausted:true`，不带 token。
- 表示还有下一页但 cursor 缺失、类型错误或证据冲突 → typed
  `pagination_inconsistent`，禁止伪装终页。
- mapper 使用同一份上游响应构造 `data` 与 `meta`，不得重新请求。

原始分页控制字段不进入新的 `result.data_schema`。未迁移命令保持 legacy；
已迁移命令按命令独立切换和回滚，不通过 Agent 参数选择协议。

## 4. Schema 规则

- `kind` 当前只允许 `cursor`。
- `cursor_parameter` 是真实 canonical CLI flag 名，不带 `--`，并必须存在于
  同一 leaf 的 `parameters`。
- 三个 meta path 由框架固定生成，产品不能覆盖。
- compact/full leaf 同时包含相同的 `result` 和 `pagination`。
- product/group 导航摘要不复制分页对象；Agent 需要时查询具体 compact leaf。
- 没有 `pagination` 表示该命令尚未发布经评审的分页能力，Agent 不得猜测。

## 5. 渐进接入

1. **legacy_only**：保持原输出，不公布分页声明。
2. **dual_validate**：业务执行一次；影子构造并校验 `meta.pagination`，外部
   legacy 字节不变。
3. **unified_active**：输出独立 `meta.pagination`，Schema 公布同级
   `pagination` 声明。
4. **unified_stable**：Skill、示例和 Agent 审计均只读取 meta 分页。

不增加 `contract_version`、`--output-contract` 或分页协议别名。

## 6. 验收

每个分页命令至少验证：

1. 有下一页时 `endpoint_exhausted:false` 且 token 非空。
2. 终页和空终页为 `endpoint_exhausted:true` 且无 token。
3. 分页矛盾产生 typed failure，不 panic、不静默停止。
4. `cursor_parameter` 在 Help/Schema 中真实存在。
5. compact/full 的 `result`、`pagination` 分别 JSON 等价。
6. `data_schema` 不包含分页控制字段。
7. 运行时 `data` 不包含迁移后的分页控制字段。
8. dual validate 与 active 都只消费一次上游响应。
9. Agent 逐命令扫描结果进入评测台账；不提交生成 Schema JSON fixture。

### DevApp 首批落地

以下 8 个终结命令已发布独立 `pagination` Schema；运行时统一输出只在
`meta.pagination` 返回分页控制信息：

- `dev app list`
- `dev app permission list`
- `dev app event list`
- `dev app version list`
- `devapp +list`
- `devapp +permission-list`
- `devapp +event-list`
- `devapp +version-list`

两套既有命令前缀继续保留。原子命令的业务记录字段为 `data.items`；Shortcut
保留既有业务投影（例如 `data.apps`、`data.permissions`、`data.events`、
`data.versions` 以及 `data.count`），但两套入口都不再在业务数据中公布
`hasMore/nextCursor`。

## 7. 对齐依据

GWS 用请求参数和 response schema 描述分页事实；Lark 在统一输出层维护分页
元数据。DWS 采用更明确的分层：业务 `data` 保真承载记录，框架 `meta` 承载
续跑状态，Schema 用独立能力把 token 与下一次 CLI 参数连接起来。
