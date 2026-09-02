# AI 表格最佳实践

## 1. 记录单元格可写性分类

| 字段类型 | 可写 | 正确方式 |
|----------|------|----------|
| 文本/数字/日期/单选/多选/复选框/URL | ✅ | record create/update |
| 附件 | ⚠️ | 必须先走 [attachment upload 流程](./aitable-attachment.md) |
| 创建人/修改人/创建时间/修改时间 | ❌ | 系统字段，只读 |
| 公式/查找引用 | ❌ | 单元格只读，由系统计算；字段定义能否创建/更新以当前 leaf Schema 为准 |
| AI 字段 | ❌ | 只读，由 AI 自动计算 |

## 2. 查询执行契约

1. **不要拉全量后在 context 里手动统计** — 标量聚合用 `record stats`，分组/去重用 `record group-stats`
2. **has_more=true 时不能做全局结论** — 数据可能不完整
3. **优先用 `--filters` 在服务端过滤** — 不要拉全量后在本地 jq/grep
4. **字段名必须来自 `table get` 真实返回** — 不要猜测 fieldId
5. **减少响应体积** — 用 `--field-ids` 仅返回需要的字段

## 3. 任务选路

| 用户诉求 | 优先方案 | 不要误走 |
|---------|----------|----------|
| 查看几条数据 | `record query` | 不要用 `--all` |
| 全量拉取明细 | `record query --all` | 不要手动循环 cursor |
| 标量统计 | `record stats` | 不要先拉全量再本地计算 |
| 分组/去重统计 | `record group-stats` | 不要先拉全量再本地 groupby |
| 全量导出为文件 | `export data` | 不要 `--all` 拉全量再写文件 |
| 批量写入 | `record create`（分批 100 条） | 不要一次传超过 100 条 |
| 附件上传 | `attachment upload` + `record update` | 不要在 cells 里伪造附件值 |
| 文件级导入 | `import upload` + `import data` | 不要手动解析 xlsx 再逐条写入 |

## 4. 创建/修改后回读确认

执行写操作后，必须使用独立读取立即回读确认结果；不能用 mutation 回执、HTTP 200 或退出码 0 代替。

| 写操作 | 必须回读命令 | 确认内容 |
|--------|-------------|----------|
| `table create` | `table get --table-ids <新tableId>` | 表名、字段列表是否符合预期 |
| `field create` | `table get --table-ids <tableId>` | 新字段是否出现在字段列表中 |
| `record create/update` | `record query --record-ids <新recordId>` | 写入值是否正确 |

## 5. AI 字段注意事项

- AI 字段的 prompt **必须至少包含一个 `fieldRef` 引用**，纯文本 prompt 会被后端拒绝
- 先创建/确认被引用字段的 fieldId，再在 prompt 中引用
- `outputType` 必须与字段类型一致（如 `outputType=text` 配 `--type text`）

## 6. DWS 与 MCP 交互契约

1. **Schema 优先** — 先读精确 leaf Runtime Schema，任务内按 canonical leaf path 缓存。只有 Schema 不足或与实际结果冲突时才读 leaf Help；版本、profile 变化或出现契约错误时废弃缓存。
2. **写后缓存失效** — mutation 返回真实 ID/名称后立即更新对象索引，清除受影响对象、Table、Dashboard 及相关查询缓存。写后验证不得复用写前数据。
3. **独立回读** — 创建/更新返回可能带旧名称或旧配置。使用真实 ID、recordId 或唯一键再读取，比较业务值和 JSON 类型；删除操作必须证明对象已不存在。
4. **未知写入不重放** — 写调用超时、断连或回执不完整时，副作用可能已发生。先用最小只读范围查真实状态，不原样自动重放创建、更新、删除或导入。
5. **并发有边界** — 同一 Base 的写入默认串行；无依赖只读默认最多 4 路并发。只有不同 Base、无数据/ID/顺序依赖且失败可独立回读时，才允许受控并发写。
6. **信封需归一化** — 按“合法 `ok/outcome` → 已知旧 `status` → JSON boolean `success`”判断，不假设固定 `data` 层。`partial_failure` 可在 stdout 并使用退出码 7；先解析 stdout/stderr 中的结构化信封，再处理退出码。
7. **上传凭据必须脱敏** — `uploadUrl`、token、authorization、signature、secret 不能输出或记录；错误对象、嵌套 `data/result` 和字符串内 URL 都要递归脱敏。
