# Dev 跨域任务闭环

仅当同一请求明确跨两个以上 Dev 域时读取本文件。先建立交付清单并复用一个 `unifiedAppId`；本页就是常见跨域任务的首轮执行契约，禁止先读 `app/robot/event/version/...`。只有真实响应出现本页未覆盖的特殊状态或字段时，才补读一份对应专题 Reference。

## 通用顺序

```text
定位/创建应用
  → 读取用户要求的初始状态或名单
  → 配置成员/权限/安全/网页/机器人/事件
  → 仅当用户明确要求或成功写结果要求发布时创建版本
  → 检查审批并发布（若需要）
  → 回读版本与用户要求的各项结果
  → 删除/停用等清理（若用户要求）
```

句子里“办完后删除”即使出现在配置或发布之前，也按依赖关系把删除移到最后；这不是冲突，不要因此停止反问。只有两个最终状态确实无法同时成立且不能用“操作前快照 + 最终清理”满足时才澄清。

## 常用原子路径（首轮直接执行）

| 域 | 已知 leaf 与关键定位参数 |
|---|---|
| 应用 | `app list --name <name>`；详情/更新/删除复用 `--unified-app-id` |
| 成员 | `app member list/add/remove --unified-app-id <id>` |
| 权限 | `app permission list/add/remove --unified-app-id <id>` |
| 安全/网页 | `app security get/config`；`app webapp get/config --unified-app-id <id>` |
| 机器人/事件 | `app robot get/config/enable`；`app event list/subscribe/unsubscribe --unified-app-id <id>` |
| 版本 | `app version create/check-approval/publish/status --unified-app-id <id>`；后续命令复用 create 返回的 `versionId` |
| 本机连接器 | `dev connect list/status/restart`；list/status 另带 `--json`，不扫描系统进程 |

所有命令补 `dws dev` 前缀和 `--format json`；写操作先 `--dry-run`，正式执行按 leaf 要求使用 `--yes`。已知路径不要用 help 探路。

## 常见闭环

- **网页应用**：`app create/list` → `webapp config` → 写结果要求发布时才 `version create` → `check-approval` → `publish` → `status` → `webapp get`。
- **权限治理**：按关键词 `permission list` → `add/remove` → 成功结果要求发布时才走版本闭环 → 回读目标权限。未指定“多余权限”时不拉全量猜目标，只暂停 remove，其它安全步骤继续完成。
- **机器人**：`app create/list` → `robot config/enable` → 版本闭环；用户另要本地调试时再 `connect`。建联和线上发布是两项独立状态。
- **事件订阅**：按关键词一次 `event list` → subscribe/unsubscribe 各至多一次正式执行 → 只有写成功且要求发布才走版本闭环 → 一次目标回读。
- **安全 + 网页**：先 `app get` 保存初始状态 → `security config` → `webapp config` → 一次版本闭环 → 分别回读。
- **成员临时变更**：解析唯一 userId → add → `member list` 保存名单 → remove → 回读；若还要删除应用，删除最后做。

## 轮次与失败预算

- 每个写步骤：一次 dry-run；用户在请求中已经明确授权该动作时，参数正确就正式执行，不再二次询问。一次必要回读即可。
- 相同业务错误不做无状态变化的重试；只在新的查询结果实际改变参数/状态后重试一次。发布错误按 [`version.md`](./version.md) 止损。
- 某个破坏性选择缺失时，暂停该动作但继续所有不依赖它的步骤；最终分开写“已完成 / 等待选择 / 失败阻塞”，不要只留下一个反问。
- 大列表只保留总数、用户相关项和分页完整性；空列表写“暂无”。最终逐项核对交付清单，避免结果被截断。
- 清理对象只能使用本任务创建并返回的 ID；删除失败不得声称已清理。
