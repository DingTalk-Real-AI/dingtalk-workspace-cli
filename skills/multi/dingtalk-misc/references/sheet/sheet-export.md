# 导出 (export)

## 使用场景

### 导出

用户说"导出/下载xlsx/存为Excel/存成表格文件/把表格变成xlsx/导出表格/下载表格/导出为 excel":
- 导出表格 → `export`（单命令一站式，内部自动完成提交、轮询、可选下载）
- 仅需传 `--node`，可选 `--output` 指定本地文件/目录（不传则返回 downloadUrl）
- 需要落盘到本地 → `dws sheet export --node <NODE_ID> --output <path>`，命令自动下载 xlsx
- 禁止用 `range read` 全量读取后自行拼接 xlsx 来模拟导出，必须使用 `export` 命令（服务端原子导出，保留格式/合并/公式等属性）
- 禁止在 AI Agent 侧实现轮询或重试，CLI 内部已按渐进式退避策略完成（最多 30 次约 5 分钟）

用户说"导出 CSV/存成 csv/导出这个工作表为 csv":
- 导出单个工作表为纯 CSV → `export --export-format csv`（**同步**，不走异步任务）
- 用 `--sheet-id` 指定工作表（不传取第一个）、`--range` 限定范围、`--value-render-option` 选取值模式
- 不传 `--output` 时 CSV 正文打印到 stdout，可直接管道处理

## 命令详细参考

### 导出表格为 xlsx（异步任务一站式）
```
Usage:
  dws sheet export [flags]    # 一站式：提交 → 轮询 → 可选下载
Example:
  # 仅导出，返回 downloadUrl（链接有时效性，请尽快下载）
  dws sheet export --node <NODE_ID>
  dws sheet export --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>"

  # 导出并自动下载为本地文件
  dws sheet export --node <NODE_ID> --output ./report.xlsx

  # --output 为目录时，自动按下载链接中的文件名保存
  dws sheet export --node <NODE_ID> --output ./

Flags:
      --node string                  表格文档 ID 或 URL (必填)
      --output string                本地保存路径（可选，支持文件路径或目录）
      --export-format string         导出格式: xlsx(默认,异步任务) / csv(单个工作表,同步) (default "xlsx")
      --sheet-id string              工作表 ID 或名称（仅 --export-format csv，不传则第一个）
      --range string                 导出范围，A1 表示法（仅 --export-format csv，不传则整表；大表可用此分块导出）
      --value-render-option string   取值模式（仅 --export-format csv）: formatted_value(默认) / raw_value / formula
      --allow-truncated              允许 CSV 被截断时仍然导出（仅 --export-format csv）。默认截断即报错且不写文件
```

将钉钉在线电子表格导出为 Office xlsx 格式。**单命令一站式**：命令内部自动完成「提交任务 → 渐进式退避轮询 → （可选）下载文件」全流程，AI Agent 无需自行拆分步骤或实现轮询。

**内部流程**：
1. 调 `submit_export_job` 获取 `jobId`
2. 按渐进式退避策略轮询 `query_export_job` 直至任务终态或超时
3. 任务成功后取得 `downloadUrl`；若指定了 `--output`，自动 HTTP GET 下载 xlsx 到本地文件

**`--export-format csv`（同步路径）**：不走异步任务，直接读取单个工作表并输出 RFC4180 CSV。仅 `--sheet-id` / `--range` / `--value-render-option` / `--output` / `--allow-truncated` 生效。不传 `--output` 时 CSV 正文打印到 stdout。

**csv 专属参数漏写 `--export-format csv` 会直接报错**：`--sheet-id` / `--range` / `--value-render-option` / `--allow-truncated` 只在 csv 分支生效，传给 xlsx 分支时命令报错而不是静默忽略——否则本想按 `--range` 导一小块，实际会拿到整篇工作簿且报成功。

**`--output` 落盘是原子替换**：CSV 先写同目录临时文件、成功后再替换目标，写入失败时已有文件保持原样（父目录不存在仍按错误处理，不会自动创建）。

**超大表默认 fail-closed**：数据超出单次读取上限（服务端返回 `hasMore`）时，命令**直接报错并以非 0 退出，既不打印 CSV 也不写文件**（已存在的目标文件不会被截断数据覆盖）。处理方式：
- 用 `--range` 分块导出（如 `--range A1:Z1000`、`A1001:Z2000` …）
- 改用默认的 `--export-format xlsx` 导出完整表格
- 确认可以接受不完整数据时，显式加 `--allow-truncated`；此时才会照常输出/落盘，并在 stderr 给出「已被截断」警告，成功提示也会写明"数据已截断，不是完整表格"

```bash
# 落盘到本地
dws sheet export --node <NODE_ID> --export-format csv --sheet-id <SHEET_ID> --output ./data.csv

# 输出到 stdout 便于管道处理
dws sheet export --node <NODE_ID> --export-format csv --sheet-id <SHEET_ID>

# 大表分块导出（避免截断；不分块时默认会因截断而报错）
dws sheet export --node <NODE_ID> --export-format csv --sheet-id <SHEET_ID> --range "A1:Z1000" --output ./part1.csv

# 明确接受不完整数据（否则截断即失败）
dws sheet export --node <NODE_ID> --export-format csv --sheet-id <SHEET_ID> --allow-truncated --output ./partial.csv
```

注意：CSV 只写纯值，不保留样式/合并/公式；需要完整属性请用默认的 xlsx。

**内置轮询策略（CLI 内实现，无需关心）**：
- 第 1~5 次：每次间隔 2 秒
- 第 6~10 次：每次间隔 5 秒
- 第 11~20 次：每次间隔 10 秒
- 第 21~30 次：每次间隔 15 秒
- **硬上限：最多轮询 30 次（约 5 分钟）**，超时后命令返回错误

**命令返回**：
- `--output` 未指定：进度日志 + 末尾输出 `jobId` 和 `downloadUrl`（链接有时效性，请尽快下载）
- `--output` 指定为文件路径：下载到该路径并输出 `导出完成: <path>`
- `--output` 指定为已存在目录：自动从 `downloadUrl` 推断文件名并保存到该目录下

**失败处理（命令内部已处理，Agent 仅需转述）**：
- 导出任务返回 `FAILED`：命令立即返回错误并附带失败原因，**禁止自动重试 `dws sheet export`**，告知用户稍后再试
- 轮询 30 次仍 `PROCESSING`：命令返回超时错误，告知用户稍后再试

**限制**：仅支持钉钉在线电子表格（alxs）→ xlsx。导出钉钉文字文档请使用 `doc` 产品对应的导出工具。

## 核心工作流

```bash
# ── 工作流 12: 导出表格为 xlsx（单命令一站式）──

# 场景 A：仅获取下载链接（命令内部自动完成提交+轮询，最终返回 downloadUrl）
dws sheet export --node <NODE_ID> --format json
# 传入 URL 也可：
# dws sheet export --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --format json

# 场景 B：导出并自动下载为本地文件
dws sheet export --node <NODE_ID> --output ./report.xlsx

# 场景 C：下载到目录，自动按链接推断文件名
dws sheet export --node <NODE_ID> --output ./

# 禁止在 Agent 侧实现任何轮询或重试，CLI 内部已按 2s/5s/10s/15s 渐进式退避自动完成（最多 30 次）。
# 若命令返回失败或超时，直接告知用户稍后再试，不要自动重调 dws sheet export。
```

```bash
# ── 工作流 13: 导出超时后查询任务状态（手动兜底）──

# 1. 执行导出（一体化命令，内部自动轮询约 5 分钟）
dws sheet export --node <NODE_ID> --format json
# 若超时，命令返回错误；等待一段时间后重新执行导出即可
```

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `export` | `downloadUrl`（未指定 --output）/ `导出完成: <path>`（指定 --output） | 直接下发给用户或告知文件已保存到本地。命令内部已完成轮询，不要再调用其他 export 相关命令 |
| `export` 超时中断 | 错误信息 | 告知用户稍后重试 |

## 注意事项

- ★ `export` 仅支持钉钉在线电子表格（alxs）→ xlsx；传入钉钉文字文档会报 `invalidRequest.document.typeIllegal`
- ★ `export` 为单命令一站式，CLI 内部已自动完成「提交 → 渐进式退避轮询 → 可选下载」，**Agent 不得在外部实现轮询或重试**；命令返回成功后不再调用其他 export 相关命令
- `export` 内置轮询策略：1~5 次间隔 2s、6~10 次间隔 5s、11~20 次间隔 10s、21~30 次间隔 15s，硬上限 30 次（约 5 分钟）；超时后命令返回错误，告知用户稍后再试即可
- ★ `export` 命令返回失败或超时时，**禁止自动重调 `dws sheet export`**；直接告知用户导出失败并建议稍后再试
- `export` 未指定 `--output` 时，返回的 `downloadUrl` 具有时效性，获取后请尽快下载；若用户需要本地文件，优先直接传 `--output` 让 CLI 代为下载
- `export` 的 `--output` 可为文件路径或已存在目录；为目录时自动从 `downloadUrl` 推断文件名，为文件路径时直接按该路径保存
- 用户要求"导出表格/下载 xlsx"时，必须使用 `export` 单命令，禁止用 `range read` 读全量数据后自行拼 xlsx 模拟导出（服务端导出会保留格式/合并/公式等完整属性）
