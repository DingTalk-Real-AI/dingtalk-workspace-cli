# Drive 文件夹比较与同步

仅用于本地目录与钉盘普通文件夹之间的递归文件级比较或镜像。单个文件使用根 Skill 的 `+download/+upload`。

## 选择入口

| 目标 | 唯一入口 |
|---|---|
| 只比较，不写任何一侧 | `dws drive status` |
| 钉盘 → 本地 | `dws drive pull` |
| 本地 → 钉盘 | `dws drive push` |
| 两侧互相补齐 | `dws drive sync` |

四个入口都要求 `--local-folder <绝对路径> --remote-folder <dentryUuid>`；`--space-id` 可选。动作已经明确时直接进入对应命令，不先重复执行 status。

## 安全流程

`status` 只读，可直接执行。`pull/push/sync` 会写本地或钉盘，先使用相同参数 `--dry-run` 查看计划；确认目标、方向和冲突策略后，再按 Runtime confirmation 执行。

```bash
dws drive status --local-folder /abs/local --remote-folder <folderId> --format json
dws drive pull --local-folder /abs/local --remote-folder <folderId> --if-exists skip --dry-run --format json
dws drive push --local-folder /abs/local --remote-folder <folderId> --if-exists skip --dry-run --format json
dws drive sync --local-folder /abs/local --remote-folder <folderId> --on-conflict skip --dry-run --format json
```

## 策略

- pull/push `--if-exists`: `skip` 是不覆盖的安全默认；`smart` 按修改时间做增量；`overwrite` 以来源侧覆盖目标侧。用户未明确选择覆盖策略时，Agent 必须保留 `skip`，不得代选 `smart/overwrite`；只有回显 dry-run 的覆盖项并得到明确授权后，才使用后两者。
- sync `--on-conflict`: `skip` 默认保留两侧；`remote-wins` 覆盖本地；`local-wins` 覆盖远端；`keep-both` 先改名本地再拉远端；`ask` 需要交互。
- 默认 exact 通过 MD5 比较；远端缺少可靠 MD5 时进入 `unknown` 并跳过。只有用户接受时间戳近似时才加 `--quick`。
- 这些命令只处理普通 `type=file` 和目录；在线文档、快捷方式不会按普通二进制同步。
- 文件级镜像只新增或覆盖，不删除任一侧多余文件。

## 结果与恢复

- status 检查 `new_local/new_remote/modified/unchanged/unknown`。
- pull/push/sync 检查 summary 与逐条 items；任何 failed/unknown 必须保留，不把部分结果称为成功。
- 下载先写临时文件再原子替换；失败时保留原目标。上传在提交前明确失败时直接返回。`push/sync` Runtime 遇到 `commit_upload` 超时、连接中断、空响应或畸形响应会直接返回错误，不会自动恢复，也不能据此断言未提交；Agent 必须把该项保留为 `unknown`，再用只读 list/inspect 按远端路径、名称和大小有限核对，无法证明时报告未知，不能直接重放可能已提交项。服务未同时提供源端与结果的可比哈希时，不虚构 checksum 验证。
- dry-run 与正式执行必须使用同一 local-folder、remote-folder、space-id 和策略；不要在确认后静默改变方向或冲突策略。
