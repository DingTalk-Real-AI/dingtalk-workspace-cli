# Minutes 复杂流程

> 返回入口：[DingTalk Minutes Skill](../SKILL.md) · [Reference 与脚本索引](minutes.md)

只在根 Skill 已确定属于 ASR、上传恢复、异步生成或批量权限 workflow 时读取本文件。普通搜索、详情、逐字稿、标题或摘要修改直接按根 Skill Golden Route 执行。

## 1. ASR 热词

| 用户意图 | 推荐入口 | 是否确认 | 关键语义 |
|---|---|---|---|
| “录音前加上这些专有词”“补充热词” | `dws minutes +prepare-asr --words "DWS,听记"` | 需要 | 只新增缺失热词，不删除现有项；读回验证 |
| “让热词最终只保留这组”“精确同步热词” | `dws minutes +sync-asr --words "DWS,听记"` | 需要，且先说明会删除目标集合外热词 | 新增缺失项并删除多余项；属于 destructive/high |
| 只查看当前 ASR 热词 / “识别词配置” | `dws minutes hot-word list --format json` | 不需要 | 原子只读；返回当前账号的识别词配置，不是某个音频的转写结果 |
| 删除一个或多个已知热词 | `dws minutes hot-word delete --words "<热词1,热词2>"` | 原子入口历史 `not_required` | 仅作兼容底层入口；推荐需要确认且能读回验证的 `+sync-asr`，不模糊删除 |

`+prepare-asr --sync` 作为已发布参数保持公开可见，但只返回迁移提示且在任何 MCP 调用前停止。需要删除时必须显式改用 `+sync-asr`，不能把“准备热词”解释成“覆盖整个词表”。两个 Shortcut 的 `--dry-run` 都只输出本地计划，不读取或写入远端；要比较真实差异时先读取词表，再单独执行目标入口。

用户说“先核对识别词/词表”时，默认指当前 ASR 热词配置，必须实际执行 `hot-word list`，不能只展示命令。如果用户明确要核对某个音频最终识别出的文字，则必须真实上传并等待转写；upload dry-run 做不到这一点。用户同时要求“不实际创建听记”时，以不写入为最高边界，如实说明两项要求不能同时满足，不能通过真实 create 后 cancel 来伪造预览。

## 2. 上传、通知与恢复

### 2.1 直接上传

| 目标 | 推荐入口 | 结果边界 |
|---|---|---|
| 上传并创建听记，不发送额外消息 | `dws minutes +upload --file <相对路径> [--title <标题>]` | 真实执行需要确认；完成 create、文件 PUT、complete 和详情读回，失败时取消可取消的 session |
| 上传并额外发送闪记卡片 | `dws minutes +upload-and-notify --file <相对路径> [--title <标题>]` | 推荐新入口；旧 `+upload --enable-message-card` 仍可执行并遵循 `+upload` 的确认门禁 |
| 上传并等待摘要/逐字稿等分析产物 | `dws minutes +upload-and-analyze --file <相对路径> --artifacts summary,transcript` | 真实执行需要确认；有界等待，可加 `--mindmap` / `--speaker-insights`，不要把 pending/timeout 说成完成 |

用户要求预览上传，并明确要求核对热词配置或比较听记列表是否变化时，执行以下可验证流程；没有这些额外要求时不必增加读操作：

```text
dws minutes hot-word list --format json
dws minutes +list-mine --page-all --format json
dws minutes +upload --file <相对路径> --title "<标题>" --input-language zh --template-id <templateId> --dry-run --format json
dws minutes +list-mine --page-all --format json
```

- 热词查询、上传计划和前后列表是三份不同证据；命令示例不能代替真实查询结果。
- dry-run 只证明请求计划与 `executed=false`，不会创建 session、听记或 ASR 结果。前后列表按 `taskUuid` 集合比较；不能只比较数量或第一页。
- 没有真实文件、文件字节数或 sessionId 时，停止在相应前置门禁；不得调用 create/complete。用户要求确认“没有生成新听记”时，仍需用真实列表证据回答，不能仅由“我没有调用上传”推断列表事实。

如果已有 `taskUuid`，并且只需要读取当前已经生成的摘要与逐字稿，直接使用只读入口：

```text
dws minutes +detail --id <taskUuid> --artifacts basic,summary,keywords --format json
dws minutes +transcript --id <taskUuid> --format json
```

不要仅因资源最初来自上传就再次进入上传 workflow。只有产物尚未就绪、确实需要有界轮询时，才使用：

```text
dws minutes +upload-and-analyze --resume-id <taskUuid> --artifacts summary,transcript
```

`--resume-id` 分支不重复上传或再次通知，但 `+upload-and-analyze` 是同时包含新上传分支的混合入口，因此仍按该命令的 Runtime confirmation 执行。旧 `+upload-and-analyze --enable-message-card` 继续执行原有通知语义并遵循同一确认门禁；新调用需要通知时优先使用 `+upload-and-notify`，再按需读取或恢复分析。

### 2.2 原子 upload session

只在 Shortcut 返回了可恢复 session、需要诊断某一阶段，或调用方自己负责文件 PUT 时使用原子命令：

| 阶段 | 原子命令 | 关键句柄 |
|---|---|---|
| 创建普通 session | `dws minutes upload create ...` | 保存 session/upload URL 等真实返回；旧 `--enable-message-card` 只返回迁移提示 |
| 创建并通知 | `dws minutes upload create-and-notify ...` | 需要确认；不要用普通 create 模拟通知 |
| 完成 session | `dws minutes upload complete ...` | 只对已知 session 执行，保留最终 taskUuid |
| 取消 session | `dws minutes upload cancel ...` | 取消失败或状态未知时停止，不能谎报已清理 |

上传状态未知时先根据真实 session/taskUuid 读回；不能重新 create 来“试一次”。预签名 URL 属于敏感临时数据，不写入日志、报告或长期 manifest。

## 3. 异步生成与录音收尾

### 3.1 思维导图

```text
dws minutes +mindmap --id <taskUuid>
```

- 首次执行负责 create + 有界轮询。
- 真实执行遵循 `user_required`；`--resume` 沿用同一命令级门禁。
- 返回 pending/timeout 时保留 taskUuid；继续检查用 `--resume`，不重复 create。
- 只有明确终态成功才声称已生成；失败和无法解析的状态返回非零。

### 3.2 发言人洞察

```text
dws minutes +speaker-insights --id <taskUuid>
```

- 首次执行保存 create 返回的异步 `taskId`。
- 真实执行遵循 `user_required`；`--resume` 沿用同一命令级门禁。
- 超时后使用 `--resume [--task-id <taskId>]` 继续轮询。
- `taskId` 缺失、状态未知或结果不可解析时保留恢复信息并停止，不再次创建任务。

### 3.3 结束录音并等待产物

```text
dws minutes +record-wrap-up --id <taskUuid> --artifacts summary,transcript
```

该入口先停止指定录音，再有界等待产物。它只接受已绑定的真实 `taskUuid`；如果 `+record-start` 返回 `controlReady=false`，不能通过 `+latest` 或列表第一项猜录音目标。停止已成功但等待超时时，保留 taskUuid 和未完成产物，后续只恢复读取，不再次 stop。

## 4. 权限 workflow

先区分三种身份语义：

| 用户意图 | 推荐入口 | 目标身份 |
|---|---|---|
| “我打不开，帮我申请查看/下载/编辑” | `+apply-permission --id <taskUuid> --permission view|download|edit` | 当前登录用户 |
| “把这条听记分享给张三” | `+share --id <taskUuid> --member-uids <UID> --permission view|download|edit` | 所有者给指定成员授权 |
| “撤销张三对这条听记的权限” | `+unshare --id <taskUuid> --member-uids <UID>` | 所有者移除指定成员权限 |

### 4.1 成员解析

1. 用户已给稳定成员 UID：直接复用，但必须保持同一 profile/组织。
2. 只有姓名、手机号或部门线索：切 `dingtalk-contact`/`dingtalk-aisearch` 解析；零或多候选时停止。
3. 不把姓名、手机号、userId、openId 或跨组织 UID 互相猜测转换。

### 4.2 批量执行

`+share` 的精确业务参数是 `--id|--ids`、`--member-uids`、必填的 `--permission view|download|edit`，以及可选的 `--cover`、`--sub-resources OrigContent|Summary|Analysis|Note`、`--failure-policy stop|continue`：

```text
dws minutes +share --id <taskUuid> --member-uids <uid1,uid2> --permission view --failure-policy stop --format json
dws minutes +share --ids <uuid1,uuid2> --member-uids <uid> --permission edit --cover --sub-resources OrigContent,Summary --failure-policy continue --format json
```

`+unshare` 的精确业务参数是 `--id|--ids`、`--member-uids` 和可选的 `--failure-policy stop|continue`；它没有 `--permission`、`--cover` 或 `--sub-resources`：

```text
dws minutes +unshare --id <taskUuid> --member-uids <uid1,uid2> --failure-policy stop --format json
dws minutes +unshare --ids <uuid1,uuid2> --member-uids <uid> --failure-policy continue --format json
```

- `--id` 与 `--ids` 必须且只能选一个；听记 taskUuid 和成员 UID 去重后各为 `1..50` 个。
- `+share --permission` 必填、没有默认值：`edit=policy 2`、`download=policy 3`、`view=policy 4`。管理员 `0`、所有者 `1` 只能走 `minutes permission add --policy 0|1`。
- `--failure-policy` 默认 `stop`，首个成员失败后停止；显式 `continue` 才继续其他成员。任何失败都必须作为 partial/非零交付，并保留失败与未执行成员。
- `+unshare` 是 write/medium、`user_required`；执行前明确听记、成员和撤权影响。`+share`、`+apply-permission` 同样执行 Runtime 的 `user_required` confirmation。对应原子 permission 命令只保留历史兼容 Contract，不作为绕过确认的推荐路径。
- `+apply-permission` 只接受单个 `--id` 和必填的 `--permission view|download|edit`，目标固定为当前登录用户，不接受 `--member-uids`。

### 4.3 验证边界

当前没有公开的 `minutes permission list/get/inspect` 命令。真实执行后，`+share/+unshare` 可输出逐成员成功、失败和未执行 ledger，但成功项只表示写接口已确认接收；即使 `+unshare` 执行前读取了听记基本信息，也没有读到成员的最终权限。

因此权限结果必须按 `verification.mode=write_ack_only`、`verified=false` 交付。不要把 ledger 中的 `complete=true`、听记基本信息、dry-run 计划或退出码解释为“已读回验证权限生效”；也不要为了验证而重放授权或撤权请求。

### 4.4 dry-run

这些权限 Shortcut 的 dry-run 只展示目标组合与将执行的动作，不调用远端，也不证明听记、成员或当前权限状态。真实执行后也只能声明“写调用已确认接收”，不能声明“已读回最终权限”。
