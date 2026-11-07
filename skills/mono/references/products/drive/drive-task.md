# drive 异步任务查询（task get）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../drive.md`](../drive.md) — 命令路由 + 场景索引 + 意图判断 + 工作流

---

## task get（统一任务查询入口）

```
Usage:
  dws drive task get --type <TYPE> --id <TASK_ID>
Example:
  dws drive task get --type export --id <taskId>
  dws drive task get --type import --id <taskId>
  dws drive task get --type copy --id <taskId>
  dws drive task get --type move --id <taskId>
Flags:
      --type string   任务类型: export|import|copy|move (必填)
      --id string     任务 ID (必填)
```

drive 域异步任务的统一查询入口，返回归一化 TaskResult。服务端原始状态值（QUEUED/RUNNING/DONE 等）统一归一化为以下六种枚举：

| 状态 | 含义 |
|------|------|
| `PENDING` | 排队中 |
| `PROCESSING` | 处理中（含空/未知状态的保守归一） |
| `SUCCESS` | 任务成功；导出任务返回 `resultUrl` 下载链接 |
| `PARTIAL_FAILED` | 部分失败（终态，常见于批量复制/移动），返回部分失败说明 |
| `FAILED` | 任务失败，返回错误信息 |
| `TIMEOUT` | 任务超时 |

注意：

- `resultUrl` 是临时下载链接，**有时效性，成功后应尽快下载**。
- `PENDING` / `PROCESSING` 是非终态，可间隔后再次查询；四种终态（`SUCCESS` / `PARTIAL_FAILED` / `FAILED` / `TIMEOUT`）出现后任务不再变化。

## copy/move 轮询中断后的兜底查询

copy/move 提交后会自动轮询直至终态；轮询超时或 Ctrl-C 中断时 CLI 会输出 `taskId`，服务端任务不会中止，之后用本命令手动查询：

```bash
# 复制任务超时/中断后查询状态
dws drive task get --type copy --id <taskId> --format json

# 移动任务返回 PARTIAL_FAILED 时查明细
dws drive task get --type move --id <taskId> --format json
```

`PARTIAL_FAILED` 时同样可用该命令查明部分失败明细。

## 三种查询入口的区分

| 入口 | 适用场景 |
|------|---------|
| `dws drive task get` | 全部四种任务类型（export/import/copy/move），返回归一化 TaskResult，一般场景统一用它 |
| `dws drive export get` | 仅查导出任务（`drive export --async` 提交后的配套查询） |
| `dws doc export get` | doc 产品级导出入口，透传原始响应 |

## 参考

- [`../drive.md` §意图判断](../drive.md#意图判断)（如何路由到本命令）
