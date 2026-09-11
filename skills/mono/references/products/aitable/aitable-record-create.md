# record create — 新增记录

## 命令格式

```
Usage:
  dws aitable record create [flags]
Example:
  dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
    --records '[{"cells":{"fldTextId":"文本内容","fldNumId":123}}]'
Flags:
      --base-id string        Base ID (必填)
      --records string        记录列表 JSON 数组，单次最多 100 条 (必填，与 --records-file 二选一)
      --records-file string   从文件读取 records JSON（替代 --records，适合超长数据或 Windows 环境）
      --table-id string       Table ID (必填)
      --parent-record-id string  父记录 ID；传入后新记录作为它的子记录创建（子记录模式）
      --view-id string        子记录模式可选：从该视图读取层级配置；缺省自动找第一个配置了层级结构的表格视图
```

## Windows / 超长 JSON 推荐

将 records JSON 写入文件，用 `--records-file ./records.json` 传入，避免命令行截断和引号转义问题。

## 子记录模式（层级记录）

在某条父记录下创建子记录：同一个 `record create` 命令，加 `--parent-record-id`：

```bash
dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --parent-record-id <PARENT_RECORD_ID> \
  --records '[{"cells":{"fldABC123":"子记录内容"}}]' --format json
```

- cells 无需手写层级字段，服务端自动注入指向父记录的关联；写入格式与普通记录一致。
- 表尚未配置层级字段时，服务端会新增 **1 个自关联字段**（默认名 `父记录`，`type=unidirectionalLink`，`config.linkedTableId` 指向本表，`multiple=true`）并更新视图配置（首次调用会改变表结构）；表上已有层级字段时**直接复用**，不再加字段（真实表实测零 schema 变更）。
- 子记录模式单次最多 100 条。
- `--view-id` 仅子记录模式有效；不带 `--parent-record-id` 使用会被 CLI 拒绝。
- 返回 `data.newRecordIds[]`（子记录 ID，与普通模式同名），额外附带 `data.hierarchyFieldId`（服务端注入关联所用的层级字段）和 `data.parentRecordId`。表上已有承载 `hierarchyConfig` 的视图时还会回显 `data.viewId`（即该视图）——**只有首次自动创建层级字段的那一次不回显**，同一张表后续调用都会带上（自动创建时已写好视图配置），因此不要把它当必填字段断言。

## 查询某条父记录的子记录

查询要先知道**层级字段 ID**（下文 `<HIERARCHY_FIELD_ID>`）。三种取法，实测结果一致：

```bash
# 取法 1（纯只读，最语义化）：读视图上的 hierarchyConfig，承载它的字段就是层级字段
dws aitable view get --base-id <BASE_ID> --table-id <TABLE_ID> \
  --jq '[.data.views[] | select(.custom.hierarchyConfig.fieldId != null) | {viewId, viewName, hierarchyFieldId: .custom.hierarchyConfig.fieldId}]' -f json

# 取法 2（纯只读，按类型启发）：找指向本表的单向关联字段
dws aitable field list --base-id <BASE_ID> --table-id <TABLE_ID> \
  --jq '[.data.fields[] | select(.type=="unidirectionalLink" and .config.linkedTableId=="<TABLE_ID>") | {fieldId, fieldName}]' -f json

# 取法 3：刚用 --parent-record-id 建过子记录，响应里的 data.hierarchyFieldId 就是它，不用再查
```

- 取法 1 优先：`hierarchyConfig` 是层级视图真正在用的字段。表上如果自建了多个指向本表的关联字段，取法 2 会一并命中，这时必须用取法 1 消歧。
- 三种取法都只依赖表本身，不要求这条父记录已经有子记录（新建表首次调用 `--parent-record-id` 后，服务端已把层级字段写进视图配置）。

层级关系存为子记录侧的关联值，用 `any_of` 匹配父记录 ID（`record list --filters`）：

```bash
dws aitable record list --base-id <BASE_ID> --table-id <TABLE_ID> \
  --filters '{"operator":"and","operands":[{"operator":"any_of","operands":["<HIERARCHY_FIELD_ID>",["<PARENT_RECORD_ID>"]]}]}' \
  --format json
```

- ⚠️ 关联字段用 `contain` 或 `eq` 匹配父记录 ID 会**静默返回 0 条**（不报错），必须用 `any_of`；值传父记录 ID 数组（实测单个 ID 传裸字符串也能命中，服务端会归一化，但数组是多父记录并集的显式写法，推荐一律传数组）。
- `any_of` 只返回**直接子记录**，不含孙记录；需要某父的整棵子树得逐层递归。
- 想一次取多个父记录的子记录，把父 ID 放进同一个数组即可（结果为并集，无重复）。
- 只想判断「某条记录是不是子记录」用 `{"operator":"exist","operands":["<HIERARCHY_FIELD_ID>"]}`。
- 这是**单向关联**：父记录的 `cells` 里不会出现子记录 ID，反向查询只能按上面的 filter 表达。
- 更新子记录的其他字段不会丢层级关联；但**删除父记录不会级联清理**，子记录会留下指向已删除记录的悬挂关联。

## 常见错误（严格避免）

| 错误 | 说明 |
|------|------|
| 参数名用 `--data` | ❌ 参数名是 `--records`，不是 `--data` |
| cells key 用字段名 | ❌ cells key 必须是 fieldId（如 `fldXXX`），不是字段名称（如 `"课程名称"`） |
| 不先获取 fieldId | ❌ 必须先 `field get` 获取 fieldId，再写入记录 |
| 单次超 100 条 | ❌ 单次最多 100 条，超过需分批 |
| 附件/图片字段直传 URL | ❌ 严禁 `{"url":"https://..."}` — 会触发 TIMEOUT_ERROR。必须先 `attachment upload` 获取 `fileToken`，再用 `{"fileToken":"ft_xxx"}` 写入。详见 [aitable-attachment.md](./aitable-attachment.md) |
| 子记录模式手写层级字段 | ❌ 传 `--parent-record-id` 即可，服务端自动注入关联，不要手填 |

## 正确流程

```bash
# 先获取 fieldId
dws aitable field get --base-id <BASE_ID> --table-id <TABLE_ID> --format json
# 从返回中提取 fieldId（如 fldABC123）

# 再用 fieldId 写入记录
dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"cells":{"fldABC123":"Python入门"}}]' --format json

# 从创建响应的 data.newRecordIds[] 提取新 ID，并回读确认真实写入值
dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> \
  --record-ids <NEW_RECORD_ID> --format json
```

创建成功以 `data.newRecordIds[]` 为 ID 来源；不要把整个 `data` 当作单个 recordId，也不要只以退出码作为写入成功证据。

## cells 写入格式

各字段类型的写入格式见 [aitable-cell-value.md](./aitable-cell-value.md)。
