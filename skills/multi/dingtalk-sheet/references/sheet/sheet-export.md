# 导出 (export)

## 使用场景

### 导出

用户说"导出/下载xlsx/存为Excel/存成表格文件/把表格变成xlsx/导出表格/下载表格/导出为 excel":
- 导出表格 → `export`（单命令一站式，内部自动完成提交、轮询、可选下载）
- 仅需传 `--node`，可选 `--output` 指定本地文件/目录（不传则返回 downloadUrl）
- 需要落盘到本地 → `dws sheet export --node <NODE_ID> --output <path>`，命令自动下载 xlsx
- 禁止用 `range read` 全量读取后自行拼接 xlsx 来模拟导出，必须使用 `export` 命令（服务端原子导出，保留格式/合并/公式等属性）
- 禁止在 AI Agent 侧实现轮询或重试，CLI 内部已按渐进式退避策略完成（最多 30 次约 5 分钟）

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
      --node string     表格文档 ID 或 URL (必填)
      --output string   本地保存路径（可选，支持文件路径或目录）
```

将钉钉在线电子表格导出为 Office xlsx 格式。**单命令一站式**：命令内部自动完成「提交任务 → 渐进式退避轮询 → （可选）下载文件」全流程，AI Agent 无需自行拆分步骤或实现轮询。

**内部流程**：
1. 调 `submit_export_job` 获取 `jobId`
2. 按渐进式退避策略轮询 `query_export_job` 直至任务终态或超时
3. 任务成功后取得 `downloadUrl`；若指定了 `--output`，自动 HTTP GET 下载 xlsx 到本地文件

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

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `export` | `downloadUrl`（未指定 --output）/ `导出完成: <path>`（指定 --output） | 直接下发给用户或告知文件已保存到本地。命令内部已完成轮询，不要再调用其他 export 相关命令 |

## 注意事项

- ★ `export` 仅支持钉钉在线电子表格（alxs）→ xlsx；传入钉钉文字文档会报 `invalidRequest.document.typeIllegal`
- ★ `export` 为单命令一站式，CLI 内部已自动完成「提交 → 渐进式退避轮询 → 可选下载」，**Agent 不得在外部实现轮询或重试**；命令返回成功后不再调用其他 export 相关命令
- `export` 内置轮询策略：1~5 次间隔 2s、6~10 次间隔 5s、11~20 次间隔 10s、21~30 次间隔 15s，硬上限 30 次（约 5 分钟）；超时后命令返回错误，告知用户稍后再试即可
- ★ `export` 命令返回失败或超时时，**禁止自动重调 `dws sheet export`**；直接告知用户导出失败并建议稍后再试
- `export` 未指定 `--output` 时，返回的 `downloadUrl` 具有时效性，获取后请尽快下载；若用户需要本地文件，优先直接传 `--output` 让 CLI 代为下载
- `export` 的 `--output` 可为文件路径或已存在目录；为目录时自动从 `downloadUrl` 推断文件名，为文件路径时直接按该路径保存
- 用户要求"导出表格/下载 xlsx"时，必须使用 `export` 单命令，禁止用 `range read` 读全量数据后自行拼 xlsx 模拟导出（服务端导出会保留格式/合并/公式等完整属性）
