# 导入 (import)

## 使用场景

### 导入

用户说"导入/上传本地表格/把 xlsx 变成在线表格/把 Excel 传成在线电子表格/供应商发来的报价单传成在线表格":
- 导入本地表格文件 → `import`（单命令一站式，内部自动完成创建会话、上传、确认、轮询）
- 需传 `--file` 与目标位置（`--folder-token` 或 `--workspace` 至少一个），`--name` 自定义表格名
- 固定产出**在线电子表格**；只**新建**文档，不写入/覆盖已有表格
- 支持格式：`xlsx` / `xls`（按扩展名识别）；文件大小上限 20MB
- 禁止用 `drive upload` 代替：`drive upload` 只上传文件，不转换为可编辑的在线电子表格，语义不同且不在表格任务闭环内
- 禁止在 AI Agent 侧实现轮询或重试，CLI 内部已按渐进式退避策略完成（最多 30 次约 5 分钟）
- 导入后如需 @同事，由 Agent 拿到 `documentUrl` 后再走 `chat`/`doc` 相关命令编排，本命令不负责通知

## 命令详细参考

### 导入本地表格文件为在线电子表格（异步任务一站式）
```
Usage:
  dws sheet import [flags]        # 一站式：创建会话 → 上传 → 确认 → 轮询
  dws sheet import get [flags]    # 兜底：按 taskId 手动查询导入结果
Example:
  # 导入 xlsx 为在线电子表格（默认表格名取文件名）
  dws sheet import --file ./quote.xlsx

  # 指定目标文件夹与导入后表格名称
  dws sheet import --file ./report.xls --folder-token <FOLDER_TOKEN> --name "月度报表"

  # 导入到指定知识库
  dws sheet import --file ./data.xls --workspace <WORKSPACE_ID>

Flags:
      --file string           本地表格文件路径 (必填，支持 xlsx/xls)
      --folder-token string   目标文件夹 ID 或 URL (与 --workspace 至少传一个)
      --workspace string      目标知识库 ID 或 URL (与 --folder-token 至少传一个)
  -n, --name string           导入后表格名称 (可选，默认取文件名去扩展名)
```

将本地表格文件（xlsx/xls）导入为一个新的钉钉在线电子表格，与 `dws sheet export`（导出）对称，构成表格"进出闭环"。**单命令一站式**：命令内部自动完成「创建导入会话 → 上传文件到 OSS → 确认导入 → 渐进式退避轮询」全流程，AI Agent 无需自行拆分步骤或实现轮询。

**内部流程**：
1. 调 `create_import_session` 获取 `sessionId` 与 OSS `uploadUrl`（入参 `fileName`/`suffix`/`fileSize`，可选 `targetFolderId`/`workspaceId`）
2. HTTP PUT 上传本地文件到 `uploadUrl`
3. 调 `confirm_import` 触发格式转换，获取 `taskId`
4. 按渐进式退避策略轮询 `query_import_task` 直至任务终态或超时

**内置轮询策略（CLI 内实现，无需关心）**：
- 第 1~5 次：每次间隔 2 秒
- 第 6~10 次：每次间隔 5 秒
- 第 11~20 次：每次间隔 10 秒
- 第 21~30 次：每次间隔 15 秒
- **硬上限：最多轮询 30 次（约 5 分钟）**，超时后命令输出结构化 `timed_out:true` + `next_command` 并以成功退出（exit 0）

**命令返回**：
- 成功：进度日志（`[1/4]`~`[4/4]`）+ 末尾 JSON，含 `success` / `taskId` / `documentUrl` / `documentName` / `documentType` / `nodeId`
- 超时：输出结构化 JSON，含 `success:false` / `timed_out:true` / `taskId` / `status:"processing"` / `next_command`（可直接复制执行的续查命令 `dws sheet import get --task-id <taskId>`）；命令以成功退出（exit 0），业务态由 `success:false` 表达，便于 Agent 编排续查

**失败处理（命令内部已处理，Agent 仅需转述）**：
- 转换返回失败状态：命令立即返回错误并附带原因，**禁止自动重试**，告知用户检查文件内容后再试
- 轮询 30 次仍处理中：命令输出 `timed_out:true` + `next_command` 结构化字段（exit 0），Agent 检测到后按 `next_command` 用 `import get --task-id` 续查

**限制**：仅支持 xlsx/xls → 在线电子表格。导入为钉钉在线文档（md/docx 等 → 文字文档）请使用 `doc` 产品的 `import` 命令。

### 查询导入任务结果（兜底）
```
Usage:
  dws sheet import get [flags]
Example:
  dws sheet import get --task-id <TASK_ID>

Flags:
      --task-id string   导入任务 ID (必填)
```

按 `taskId` 查询导入任务状态。通常不需要手动调用，`import` 会自动轮询到终态；仅在导入命令超时或中断后用于兜底查询。任务状态：`processing`（转换中）/ `completed`（成功，返回 documentUrl）/ `failed`（失败）。

## 核心工作流

```bash
# ── 工作流 13: 导入本地表格文件为在线电子表格（单命令一站式）──

# 场景 A：导入 xlsx，默认表格名取文件名（命令内部自动完成上传+转换+轮询）
dws sheet import --file ./quote.xlsx --format json

# 场景 B：导入 xls 到指定文件夹并自定义表格名
dws sheet import --file ./report.xls --folder-token <FOLDER_TOKEN> --name "月度报表"

# 场景 C：导入到指定知识库
dws sheet import --file ./data.xls --workspace <WORKSPACE_ID>

# 场景 D：导入超时后兜底查询（用命令输出的 taskId）
dws sheet import get --task-id <TASK_ID>

# 禁止在 Agent 侧实现任何轮询或重试，CLI 内部已按 2s/5s/10s/15s 渐进式退避自动完成（最多 30 次）。
# 若命令返回失败或超时，按提示用 import get 查询，不要自动重调 import。
```

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `import` | `documentUrl` / `nodeId`（导入成功后返回） | 下发给用户，或作为后续 `sheet` 操作的 `--node`；如需 @同事再走 `chat` 命令编排 |
| `import`（超时） | `timed_out` / `taskId` / `next_command`（结构化输出） | 检测到 `timed_out:true` 后执行 `next_command`（即 `import get --task-id`）续查 |
| `import get` | `status` / `documentUrl` | 判断任务是否完成并取回文档链接 |

## 注意事项

- ★ `import` 仅支持 xlsx/xls → 在线电子表格；导入为文字文档（md/docx 等）请用 `doc import`
- ★ `import` 只**新建**在线电子表格，不写入或覆盖已有表格；要往已有表格追加数据请用 `range update` / `range append` / `csv put`
- ★ `import` 为单命令一站式，CLI 内部已自动完成「创建会话 → 上传 → 确认 → 渐进式退避轮询」，**Agent 不得在外部实现轮询或重试**
- ★ 命令失败或超时时，**禁止自动重调 `import`**；超时会输出 `timed_out:true`，按 `next_command` 用 `import get --task-id` 续查，失败则告知用户检查文件后再试
- 文件大小上限 20MB；空文件、目录、非白名单格式均会在上传前被 CLI 拦截报错
- `--name` 未指定时默认取文件名去掉扩展名；`--folder-token` 与 `--workspace` 用于指定目标位置，至少提供一个，均不传会被 CLI 层拦截报错
- 用户要求"导入表格/上传 xlsx 变在线表格"时，必须使用 `import`，禁止用 `drive upload`（只上传不转换，产出不可编辑，且不在表格闭环内）
