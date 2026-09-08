# record query — 查询记录

## 命令格式

```
Usage:
  dws aitable record query [flags]
Example:
  dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID>
  dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> --record-ids rec1,rec2
  dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> --query "关键词" --limit 50
Flags:
      --base-id string      Base ID (必填)
      --cursor string       分页游标，首次不传
      --field-ids string    返回字段 ID 列表，逗号分隔，单次最多 100 个
      --filters string      结构化过滤条件 JSON
      --query string        全文关键词搜索
      --limit int           单次最大记录数，默认 100，最大 100
      --record-ids string   指定记录 ID 列表，逗号分隔，单次最多 100 个
      --sort string         排序条件 JSON 数组
      --table-id string     Table ID (必填)
      --all                 启用自动翻页，循环获取并合并所有记录后统一输出
      --page-limit int      自动翻页最大页数（仅 --all 时生效）。默认 50，设为 0 表示无限制
```

两种模式: 按 ID 取（传 record-ids，忽略 filters/sort）或条件查（filters+sort+cursor 分页）。

## 自动翻页（--all + --page-limit）

- 用户要求“全部/完整/汇总/统计/导出/逐条处理”时，必须传 `--all`，由 CLI 循环获取并合并记录；不要让 Agent 手写分页循环。
- 要求完整结果时使用 `--page-limit 0`；默认值 50 只适用于允许最多读取约 5000 条的有界场景。
- CLI 首次请求不传 cursor，后续原样使用上一页 `data.nextCursor`，并检测 cursor 循环；页间间隔 200ms。
- 同一分页会话的 `base-id/table-id/filters/sort/query/field-ids/limit` 必须保持不变，禁止重发第一页、复用旧会话 cursor、修改排序或自行构造 cursor。
- 只有自动分页输出 `complete=true`，或手动分页时 `data.nextCursor` 为空，才表示完整结束。普通扫描某页恰好返回 `limit` 条时，服务端可能返回 `nextCursor`；用它续页后若调用成功、`records=[]` 且 `nextCursor` 为空，这是正常的末页探测，应正常完成，不能报错、重试或判定漏查。
- 成功页的 `records=[]` 本身既不是错误也不是结束条件：`nextCursor` 非空就继续，`nextCursor` 为空就正常结束。
- 达到 page-limit、网络错误、缺少 `records` 的空响应、非法响应或 cursor 循环时，命令返回非零结构化错误；错误详情保留已取记录和续传 cursor。必须标记结果不完整，禁止把 partial 当 success 或据此给出全量结论。

```bash
# 完整拉取：自动翻页直到 nextCursor 为空
dws aitable record query --base-id X --table-id Y --all --page-limit 0 --format json

# 从结构化错误 details.incomplete_result.cursor 断点续传；其余查询条件必须与原请求完全一致
dws aitable record query --base-id X --table-id Y --filters '<原 filters>' --sort '<原 sort>' --all --page-limit 0 --cursor '<原样 cursor>' --format json
```

## 手动分页状态机

1. 第一页不传 `--cursor`，读取 `data.records` 和 `data.nextCursor`。
2. `nextCursor` 非空时，将其原样作为下一次 `--cursor`；不得使用当前请求 cursor，也不得自行拼接或复用更早页 cursor。
3. 每一页保持全部查询条件不变并累计记录；如需防御性校验，以 `recordId` 检测重复，但重复 cursor 必须按异常停止，不能静默继续。
4. `nextCursor` 为空才结束。成功返回的空页是合法结果：若 `records=[]` 且 `nextCursor` 为空，则正常完成；若 `records=[]` 但 `nextCursor` 非空，则继续查询，不能把空页当作异常或自行重试当前 cursor。

## 排序参数规范

`--sort` 需要传 JSON 数组，排序方向字段必须是 `direction`（`asc` 或 `desc`），不要使用 `order`。

正确示例：
```bash
--sort '[{"fieldId":"wm8ns9bw2vmucb45xj3ix","direction":"desc"}]'
```

## filters 结构

详细语法见 [aitable-filter-sort.md](./aitable-filter-sort.md)。

> **查询指定数据前先读表头**：先用 `dws aitable field get --base-id <BASE_ID> --table-id <TABLE_ID> --format json`（不加 `--field-ids`）完整读一遍表头，拿到所有字段的 `fieldId`/`name`/`type`/`config`；从表头中确定用户条件对应哪个字段，再按该字段类型解析值并传入 filters/sort，禁止跳过读表头凭猜测选字段。

快速模板：
```json
{"operator":"and","operands":[{"operator":"eq","operands":["<fieldId>","<value>"]}]}
```

> **singleSelect/multipleSelect 过滤**：先通过 `field get` 或 `field search-options` 将用户输入唯一解析到现有选项，filter 优先传稳定 option ID；当条件涉及 multipleSelect 或其他数组型字段时，第二个 operand 必须是 option ID/稳定 ID 数组（如 `"operands":["fldMulti",["optA"]]`），不能传裸字符串；`record create/update` 仍传 option name。不要把模糊名称或未确认的原始文本直接透传。
>
> **人员/部门/群组过滤**：禁止把姓名、部门名或群名直接放入 filters。必须先调用 `dws aisearch person --keyword "<姓名>" --dimension name`、`dws contact +resolve-dept --name "<部门名>"`、`dws chat +chat-search --query "<群名>" --page-all`，唯一取得 `userId`、`deptId`、`openConversationId`，再分别传 `[{"userId":"..."}]`、`[{"departmentId":"..."}]`、`[{"cid":"..."}]`。零命中或多命中必须停止并消歧。

## 减少响应体积

字段较多时，用 `--field-ids` 仅返回需要的字段，可显著减少返回数据量。

## 常见错误

- 不先读表头就组装条件 → 必须先 `field get` 完整读一遍表头，确定用户条件对应的字段后再传值，禁止凭字段名猜 fieldId 或猜类型
- 直接把字段中文名、人员姓名、部门名、群名或关联记录标题放进 filters → 必须先解析为 fieldId 和对应稳定 ID；人员/部门/群组还必须传结构化 ID 数组，不能传裸字符串、标量 ID 或单个对象
- `--filters` 根节点直接用 `"operator":"eq"` → API 静默忽略，返回全表
- `--sort` 用 `"order":"desc"` → 必须用 `"direction":"desc"`
- 不加 `--field-ids` 拉全字段 → 大表响应体积过大
- 只读取第一页或以 `records=[]` 判定结束 → 必须检查 `nextCursor`，完整任务优先 `--all --page-limit 0`
- 全量拉取后在 context 里手动统计 → 应优先用 `--filters` 服务端过滤

## record query-empty — 找空行

`record query-empty` 是与 `record query` 平行的独立子命令，专门按表内顺序扫描出"完全没填用户字段"的空行。

```bash
dws aitable record query-empty --base-id BASE_ID --table-id TABLE_ID
```

| flag | 说明 |
|------|------|
| `--base-id` / `--base` | 必填 |
| `--table-id` | 必填 |
| `--limit` | 单次**扫描预算**（不是返回数）；范围 [1, 100]，默认 100 |
| `--cursor` | 分页游标。响应中 `nextCursor` 非空 → 用它翻页继续扫；nextCursor 为空（或不存在）→ 已扫完整表 |

返回结构：

```jsonc
{ "data": { "records": [...], "nextCursor": "..." } }
```

### 关键语义

1. **`--limit` 是扫描预算不是返回数**：可能扫了 100 条但全部非空，本页 `records: []`。
2. **本页空 records ≠ 全表无空行**：必须看 `nextCursor`，nextCursor 还在就要继续翻。
3. **空行定义**：除系统字段（recordId / 创建人 / 创建时间 / 修改人 / 修改时间）外，所有 cell 都是 null、空字符串、空集合或空 Map。一般是用户在 UI 上"插入空行"产生的。

### 典型用法

```bash
# 扫一页，看本页有没有空行
dws aitable record query-empty --base-id BASE --table-id TBL

# 翻页
dws aitable record query-empty --base-id BASE --table-id TBL --cursor <上次的nextCursor>

# 把整表扫完（手动循环 cursor）
NC=""
while : ; do
  R=$(dws aitable record query-empty --base-id BASE --table-id TBL ${NC:+--cursor "$NC"} --format json)
  echo "$R" | jq '.data.records[] | .recordId'
  NC=$(echo "$R" | jq -r '.data.nextCursor // empty')
  [ -z "$NC" ] && break
done
```
