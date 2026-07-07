# export & import — 导入导出

## 导出数据（两阶段轮询）

`export data` 为异步任务：首次调用可能只返回 `taskId`，需要继续轮询。

> ⚠️ **`--format` 冲突警告**：`export data` 的 `--format` 是**导出格式**（excel/attachment 等），不是全局输出格式。**此命令禁止追加全局 `--format json`**，否则会覆盖导出格式导致 `INVALID_EXPORT_FORMAT` 错误。输出默认就是 JSON，无需额外指定。

```bash
# 第一步：创建任务（按 scope 传必要参数）——注意：不要加 --format json！
dws aitable export data --base-id <BASE_ID> --scope table --table-id <TABLE_ID> --format excel --timeout-ms 1000

# 第二步：拿 taskId 继续轮询，直到返回 downloadUrl
dws aitable export data --base-id <BASE_ID> --task-id <TASK_ID> --timeout-ms 3000
```

### 参数约束

| scope | 必传参数 |
|-------|----------|
| `all` | 只需 `--base-id` |
| `table` | 必须 `--table-id` |
| `view` | 必须 `--table-id` + `--view-id` |

## 导入文件（三步流程）

当用户要求将 Excel（`.xlsx`）或 CSV 文件完整导入 AI 表格时，**不需要自己解析文件内容**，直接使用文件级导入。

> **无需手动解析 CSV/Excel 再逐条 record create**，效率极低且容易出错。

```bash
# 第 1 步：申请上传凭证
dws aitable import upload --base-id <BASE_ID> \
  --file-name data.xlsx --file-size <字节数> --format json
# → 返回 uploadUrl 和 importId

# 第 2 步：上传文件到 OSS（注意：Content-Type 必须设为空）
curl -X PUT "<uploadUrl>" -H "Content-Type:" --data-binary @data.xlsx

# 第 3 步：触发导入（新建表模式）
dws aitable import data --import-id <importId> --format json
# → 返回 status: success 和新建的 tableIds

# 第 3 步（替代）：追加到已有表
dws aitable import data --import-id <importId> --table-id <TABLE_ID> --format json
# → 数据作为新行追加到指定表中
```

### 步骤说明

| 步骤 | 命令 | 说明 |
|------|------|------|
| 申请上传凭证 | `import upload --base-id <ID> --file-name <名称> --file-size <字节>` | `--file-size` 必须与实际文件大小一致 |
| 上传文件 | HTTP PUT（curl 等） | **必须** 带 `-H "Content-Type:"` 将 Content-Type 设为空，否则 OSS 返回 403 |
| 触发导入 | `import data --import-id <ID> [--table-id <TABLE_ID>]` | 同步等待，大多一次调用即返回结果；超时可用相同 importId 重试 |

### import data 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--import-id` | ✅ | `import upload` 返回的 importId |
| `--table-id` | ❌ | 传入时数据追加到该已有表；不传则每个 Sheet 新建独立的数据表 |
| `--timeout` | ❌ | 最长等待秒数，默认且推荐 30 |
| `--header-row` | ❌ | 表头所在行号（从 1 开始），数据从下一行读取。不传则自动识别 |
| `--src-sheet-name` | ❌ | 源文件中的 Sheet 名称，多 Sheet 文件时指定。不传则用第一个 Sheet |
| `--field-mapping` | ❌ | 字段映射 JSON（`{"目标字段名":"源列名"}`）。不传则按列名自动匹配 |

### 两种导入模式

| 模式 | 触发条件 | 效果 |
|------|----------|------|
| **新建表导入** | 不传 `--table-id` | 每个 Sheet 自动新建为独立数据表 |
| **追加导入** | 传入 `--table-id` | 数据作为新行追加到指定已有表，按列名自动匹配字段 |

### 支持的文件格式：xlsx vs csv

| 特性 | xlsx | csv |
|------|------|-----|
| 新建表导入 | ✅ | ✅ |
| 追加导入（`--table-id`） | ✅ | ✅ |
| `--header-row` | ✅ | ❌ 不支持 |
| `--src-sheet-name` | ✅（多 Sheet 支持） | ❌ 无 Sheet 概念 |
| `--field-mapping` | ✅ | ✅ |

> **CSV 限制**：CSV 没有 Sheet 概念，且表头固定为第一行，因此 `--header-row` 和 `--src-sheet-name` 对 CSV 均不可用。
>
> **建议**：需要指定表头行或多 Sheet 选择时，**必须使用 xlsx 格式**。CSV 仅适用于表头在第一行的简单导入场景。

### 追加导入的字段匹配规则

追加导入时，系统按以下规则将 Excel 列映射到目标表字段：

1. **不传 `--field-mapping`（自动匹配）**：按字段名**精确匹配** Excel 列名和目标表字段名。如果没有任何一列匹配上，导入会失败。
2. **传 `--field-mapping`（显式映射）**：按映射关系指定对应关系，key 为目标表字段名，value 为 Excel 列名。

> **追加导入失败常见原因**：Excel 列名与目标表字段名不一致（如 Excel 是"销售姓名"但表字段是"姓名"），导致自动匹配 0 个字段，报错 `"Failed to build import sheet infos from preview data"`。
>
> **解决方案**：
> 1. **首选**：创建目标表时，字段名与 Excel 表头列名**保持完全一致**
> 2. **备选**：传 `--field-mapping '{"目标字段名":"Excel列名"}'` 手动指定映射
> 3. **兜底**：如果 import data 多次失败，改用 `record create` 逐条写入

### 适用场景

- **新建表导入**：首次导入 Excel/CSV，让系统自动建表建字段
- **追加到已有表**：已有数据表结构，需要把 Excel 数据批量写入 → 传 `--table-id`（推荐 xlsx）
- **需要指定表头行**：源文件前几行非数据（如注释行）→ `--header-row`（必须用 xlsx）
- **多 Sheet 文件**：只导入特定 Sheet → `--src-sheet-name`（必须用 xlsx）
- **不适用**：需要复杂字段级控制（如只导入部分列、数据转换）→ 解析后用 `record create`

> **导入数据无法整体撤销**：文件一旦导入成功，数据即写入表中，没有"撤销导入"操作。如需清理导入的测试数据，只能手动通过 `record delete` 逐条或批量删除记录；如果是新建表模式导入的，可以直接 `table delete` 删除整张表。因此：
> - 测试/验证场景建议导入到**独立的测试表或测试 Base**，用完后整体删除
> - 如果用户明确表示不想导入测试数据或要求先预览内容再决定，应先解析文件内容展示给用户确认，而非直接导入

---

## 导入数据：两条链路怎么选

用户给文件让导入 AI 表格时，路径选择决定成败：

| 用户原话 | 链路 | 命令 / 脚本 | 行为 |
|---------|------|------------|------|
| "把这个 Excel 导入到 AI 表格"（无指定目标表） | **文件导入任务** | `python scripts/aitable_import_via_task.py <baseId> <file>` 或 `dws aitable import upload --base-id B --file ./x.xlsx` + `dws aitable import data --import-id <ID>` | 服务端解析文件，**新建数据表**，自动识别表头 |
| "把这个 Excel 导入新表 / 自动建表" | 同上 | 同上 | 同上 |
| "把这批记录追加到已有的『成员表』里" | **记录批量写入** | `python scripts/import_records.py <baseId> <tableId> <file>` | 走 `record create`，**写入已有 tableId**，需要字段名匹配 |
| "Excel 列名和表字段对不上但要追加" | 文件导入 + 追加 + 字段映射 | `dws aitable import upload --base-id B --file ./x.xlsx` → `dws aitable import data --import-id <ID> --table-id <TBL> --field-mapping '{"目标":"源"}'` | 服务端按映射追加 |

## 大表 / 长任务超时续等

默认整体轮询超时 5 分钟。大表导出/导入超时后命令会返回 `taskId` / `importId`，用同命令带 ID 续等：

```bash
# 续等导出
dws aitable export data --base-id <B> --task-id <ID> --output ./out.xlsx

# 续等导入
dws aitable import data --import-id <ID>
```

或一次性把 timeout 提到 15 分钟：`--timeout-sec 900`。
