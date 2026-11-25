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

## 导入文件（DWS 闭环）

当用户要求将 Excel（`.xlsx`）或 CSV 文件完整导入 AI 表格时，**不需要自己解析文件内容**，直接使用文件级导入。

> **无需手动解析 CSV/Excel 再逐条 record create**，效率极低且容易出错。

先展示来源、目标、字段映射、异常处理和回退方式，获得这次迁移的专门确认。使用 `+import-file`，由 DWS 在进程内完成申请、PUT 和触发，不向调用方暴露签名 URL：

```bash
dws aitable +import-file --base-id <BASE_ID> --file ./data.xlsx

# 追加到已有表并指定映射
dws aitable +import-file --base-id <BASE_ID> --file ./data.xlsx \
  --table-id <TABLE_ID> --field-mapping '{"目标字段名":"源列名"}'

# 终态未知时，先检查目标数据；需要继续等待时只能复用同一 importId
dws aitable +import-file --resume-import-id <IMPORT_ID> --timeout 30
```

追加到已有表时，先用 `dws aitable field list --base-id <BASE_ID> --table-id <TABLE_ID>` 读目标表字段。`--field-mapping` 的键必须逐字等于读回的字段名，大小写和空格都一致（用户写 amount、表里是 Amount，就写 Amount）；值必须逐字等于源文件表头。`--field-mapping` 只包含用户指定的列，以及名称、含义和单位都一致的同名列；其余源列不导入，目标字段留空。同名列不需要用户点名，默认一并映射，也不为此反问，用户明确说只导入某几列时才排除。原价与实付价、单价与总额、含税与不含税都算不一致，不能只因名称相近就映射。用户的说法与表头措辞不完全相同时（如「结算金额」与表头「实收结算金额」），只有唯一对应才采用，有多个可能对应时才向用户确认；用户已给出映射就直接执行，不再反问。源表头不确定时，用`python3 <本 Skill 目录>/scripts/read_import_headers.py <文件>`（按本 Skill 的实际路径替换）（多 Sheet 时用 `--sheet <工作表>` 指定，并在导入时用 `--src-sheet-name` 传入同一名称）；只读取源文件表头行，不读取数据行；该脚本只支持 CSV/XLSX 的首行表头；其他格式、表头不在首行或无法仅取表头时停止并请用户提供源表头，但读到的内容只用于确定映射，不用来写入、改写或补全导入数据，也不要自行解析文件后逐行 `record create`。

`+import-file` 只接受 CSV/XLS/XLSX，自动校验本地普通文件并隐藏上传地址和凭据。导入触发后超时会保留 `importId` 并返回 `unknown`；必须先查目标表真实状态，不得重新申请上传地址、重复 PUT 或更换 importId。续等模式只允许 `--resume-import-id` 和可选 `--timeout`，不能改变目标表、表头、源 Sheet 或字段映射。下方手动命令仅用于诊断三段协议。

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
| 触发导入 | `import data --import-id <ID> [--table-id <TABLE_ID>]` | 同步等待；超时先查真实状态，仅在当前契约允许时用相同 importId 续等，不重新提交 |

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
> 1. 核对源文件列名与已有目标表字段，按上述映射规则为 `+import-file` 显式提供 `--field-mapping '{"目标字段名":"Excel列名"}'`，不新建或修改目标表来迁就源列名。
> 2. 明确写前拒绝时修正参数；结果未知时只读核对目标数据，不重复导入，也不改用 `record create` 重放。

### 适用场景

- **新建表导入**：首次导入 Excel/CSV，让系统自动建表建字段
- **追加到已有表**：使用 `+import-file`，传 `--table-id` 和按上述规则确定的 `--field-mapping`（推荐 xlsx）
- **只导入部分列**：仍使用 `+import-file`，通过显式 `--field-mapping` 选择源列，不自行解析后逐行写入
- **需要指定表头行**：源文件前几行非数据（如注释行）→ `--header-row`（必须用 xlsx）
- **多 Sheet 文件**：只导入特定 Sheet → `--src-sheet-name`（必须用 xlsx）
- **需要数据转换**：属于单独的数据处理需求，先与用户确认转换方案，不作为追加导入失败后的自动兜底

> **导入数据无法整体撤销**：文件一旦导入成功，数据即写入表中，没有"撤销导入"操作。如需清理导入的测试数据，只能手动通过 `record delete` 逐条或批量删除记录；如果是新建表模式导入的，可以直接 `table delete` 删除整张表。因此：
> - 测试/验证场景建议导入到**独立的测试表或测试 Base**，用完后整体删除
> - 如果用户明确表示不想导入测试数据或要求先预览内容再决定，应先解析文件内容展示给用户确认，而非直接导入
