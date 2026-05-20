# 易混淆操作与字段规则

## 易混淆操作 (高风险场景必读)

| 用户说的 | 正确命令 | 不是这个 |
|---------|----------|---------|
| "创建一个新表格 (Base)" | `base create` | 不是 `table create` |
| "在表格里加一个数据表" | `table create` | 不是 `base create` |
| "看看表格里有哪些表" | `base get` | 不是 `field get` |
| "看看表里有哪些列" | `field get` / `table get` | 不是 `base get` |
| "搜索表格" (找 Base) | `base search` | 不是 `record query` |
| "搜索记录" (查表内数据) | `record query` | 不是 `base search` |
| "删掉这个数据表" | `table delete` | 不是 `record delete` |
| "删掉这条数据" | `record delete` | 不是 `table delete` |
| "删掉这个列" | `field delete` | 不是 `record delete` |
| "改字段类型" | 先 `field delete` 再 `field create` | `field update` **不能改类型** |
| "移动字段/调整字段顺序" | `view update --config '{"visibleFieldIds":[...]}'`（视图层重排，首列主字段必须保留在第一位） | 没有 `field reorder`/`field move` 命令；不能改字段在元数据里的"原始定义顺序" |

## field 子命令总览

> ⚠️ field 有且仅有以下 **4 个** 子命令，没有 `list`、`reorder`、`move`：

| 子命令 | 用途 |
|-------|------|
| `field get` | 获取字段详情（含完整 config/options）。**不是 `field list`** |
| `field create` | **创建字段（支持通过 config.options 设置选项）** |
| `field update` | 更新字段名称或配置（**不能改类型**，**不能改顺序**） |
| `field delete` | 删除字段（不可逆） |

> **想"调整字段顺序"？请使用 `view update`**（视图层操作，不属于 `field` 子命令）：
> - 通过 `--config '{"visibleFieldIds":["fld1","fld2",...]}'` 传入 fieldId 数组，数组顺序即视图中的字段显示顺序
> - 仅影响**该视图**的列排列，同一 table 的其他视图与字段元数据原始顺序不变
> - **首列字段（主字段）必须保留在数组第一位**，不能移动到非首位
> - **漏传的字段不会被隐藏**，而是会被 API 自动追加到列表末尾
> - **读写命名不一致**：写入键名 `visibleFieldIds`，但读出（`view get`）时该字段在 view 里叫 `columns`——校验顺序时请看 `views[].columns`

## field create 高频错误 flag（LLM 极易踩坑）

> 以下是 LLM 生成命令时**最常犯的 flag 错误**，每个都会返回 `unknown flag` 或被静默忽略。
> **CLI 只接受 `--name` / `--type` / `--config`（单字段模式）或 `--fields`（批量模式），没有其他 flag 表达字段名 / 类型 / 选项。**

| 错误写法（LLM 直觉） | 正确写法 | 说明 |
|---|---|---|
| `--field-name "状态"` | `--name "状态"` | 没有 `--field-name`，flag 就是 `--name` |
| `--field-type singleSelect` | `--type singleSelect` | 没有 `--field-type` |
| `--options "进行中,已完成,已归档"` | `--config '{"options":[{"name":"进行中"},{"name":"已完成"},{"name":"已归档"}]}'` | **没有 `--options` flag**，选项必须嵌套在 `--config` 的 JSON 里 |
| `--property '{"options":...}'` | `--config '{"options":...}'` | 没有 `--property`，对应 flag 是 `--config` |
| `--select-options "高,中,低"` | `--config '{"options":[{"name":"高"},{"name":"中"},{"name":"低"}]}'` | 同上，没有任何逗号分隔的简写语法 |
| `--field-config '{...}'` | `--config '{...}'` | 没有 `--field-config` |

> **记忆口诀**：`field create` 的参数只有 4 个单词—— **name / type / config / fields**，其他任何变体拼写都是错的。

## 字段创建时设置 config（重要）

创建 singleSelect/multipleSelect 字段时，**必须设置选项 (options)**：

```bash
# 创建带选项的单选字段 (推荐新语法: --name/--type/--config)
dws aitable field create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --name "优先级" --type "singleSelect" \
  --config '{"options":[{"name":"高"},{"name":"中"},{"name":"低"}]}' \
  --format json

# 建表时也可以直接通过 --fields 批量带选项字段
dws aitable table create --base-id <BASE_ID> --name "任务表" \
  --fields '[{"fieldName":"任务","type":"text"},{"fieldName":"状态","type":"singleSelect","config":{"options":[{"name":"待办"},{"name":"进行中"},{"name":"已完成"}]}}]' \
  --format json
```

> ⚠️ **不要混淆**：
> - **字段创建**（`field create` 的 `--config` 或 `table create` 的 `--fields`）：创建时就指定 `options`
> - **记录写入**（`record create` / `record update` 的 `--records`）：只能写入已存在的选项名称

## 主字段约束（table create 必读）

> ⚠️ `table create` 的 `--fields` 中，**第一个字段自动成为主字段**。
> 主字段只能是 **text** 类型，不能是 attachment、checkbox、formula 等。

**实际影响**：当用户要求创建的字段不适合做主字段时（如附件、复选框），必须：
1. 先放一个 text 字段作为第一个字段（主字段）
2. 再放用户要求的字段
3. **告知用户**为何多了一个字段

```bash
# 例: 用户要求只创建附件字段 → 附件不能做主字段，必须先加 text 主字段
dws aitable table create --base-id <BASE_ID> --name "产品图片" \
  --fields '[{"fieldName":"名称","type":"text"},{"fieldName":"产品图片","type":"attachment"}]' \
  --format json
```

## 只读字段 (不可写入)

以下类型的字段不可写入, 执行 `field get` / `table get` 后识别并跳过:
- 创建时间 / 修改时间 (系统自动)
- 创建人 / 修改人 (系统自动)
- 自动编号
- 公式字段
- 引用字段

## 记录写入格式（record create / record update）

> 以下是 **记录写入** 时 cells 的格式，不是字段创建的格式。

| 类型 | 写入格式 | 读取返回格式 |
|------|----------|-------------|
| text | `"fldXXX":"文本值"` | `"fldXXX":"文本值"` |
| number | `"fldXXX":123` | `"fldXXX":123` |
| singleSelect | `"fldXXX":"选项名"` (必须是已存在的选项) | `"fldXXX":{"id":"x","name":"选项名"}` |
| multipleSelect | `"fldXXX":["选项1","选项2"]` (必须是已存在的选项) | `"fldXXX":[{"id":"x","name":"选项1"}]` |
| date | `"fldXXX":"2026-03-04"` 或 Unix ms | `"fldXXX":1709510400000` (ms) |
| user | `"fldXXX":[{"userId":"123"}]` | `"fldXXX":{"uid":"123"}` |
| attachment | `[{"fileToken":"<token>"}]` — 需先通过 `attachment upload` 获取，见下方 ⬇️ | `[{"url":"...","filename":"...","size":N}]` |

## ⚠️ 附件上传完整流程（必读！）

> **不要**使用钉盘 (drive) 上传来替代此流程！钉盘 fileId **无法**写入 attachment 字段。

附件字段写入使用 `upload_attachment.py` 脚本，**2 步**完成：

```bash
# 步骤 1: 一键上传文件（脚本内部自动完成 prepare + PUT to OSS）
python3 scripts/upload_attachment.py <BASE_ID> /path/to/photo.png
# 输出: { "fileToken": "ft_xxx", "fileName": "photo.png", "size": 1024 }

# 步骤 2: 在 record create/update 中使用 fileToken
dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"cells":{"fldAttachId":[{"fileToken":"ft_xxx"}]}}]' --format json
```
