# workflow — 自动化工作流管理

启停 / 查看 / 列出 Base 下的自动化工作流（在 AI 表格 Web 端配置的 "当 X 时自动 Y" 流程）。
适用场景：用户问 "停掉这个流程"、"看下都有哪些自动化流程"、"流程 X 的配置是什么"。

## 命令一览

| 命令 | 用途 |
|------|------|
| `workflow list` | 列出 Base 下所有工作流（含状态/创建人/最后修改时间），支持分页 |
| `workflow get` | 获取单个工作流详情（含 flowSchema 完整节点定义） |
| `workflow enable` | 启用指定工作流（按配置的触发条件自动执行） |
| `workflow disable` | 禁用指定工作流（高危，建议 `--yes` 二次确认） |

> 所有子命令的 `--base-id` 必填（可用隐藏别名 `--base`）。
> 当前**不支持通过 CLI 新建工作流**，请在 AI 表格 Web 端配置好后用 `workflow list` 拿到 ID 再启停。

## 命令详情

### workflow list — 列出工作流

```bash
dws aitable workflow list --base-id BASE_ID --format json
dws aitable workflow list --base-id BASE_ID --limit 50 --offset 100
```

| flag | 说明 |
|------|------|
| `--base-id` | 必填 |
| `--limit` | 可选，分页大小 `[1, 100]`，不传走服务端默认 20 |
| `--offset` | 可选，分页偏移量 `>= 0`，不传走服务端默认 0 |

返回结构：

```json
{
  "data": {
    "list": [
      {
        "flowId": "G-FLOW-XXXXXX",            // ★ 注意字段名是 flowId
        "name": "流程1",
        "description": "当创建记录时，就更新记录",
        "status": "RUNNING",                  // RUNNING / STOP
        "creatorStaffId": "281493",
        "lastModifier": { "name": "李普阳", "staffId": "281493" },
        "gmtModified": 1780318540000,
        "versionId": "G-FLOW-VER-XXXXXX",
        "icons": ["..."],                     // 触发器+动作的图标
        "isSubFlow": false,
        "opPermissions": { "canEdit": true }
      }
    ],
    "recordCount": 1,                         // Base 下总数
    "runningCount": 1                         // RUNNING 状态的数量
  }
}
```

**注意**：
- 标识字段服务端在 `list` 里叫 **`flowId`**，但在 `enable` / `disable` 出参里叫 **`workflowId`**。CLI `--workflow-id` 传任一即可（同值）。
- `status` 是字符串枚举：`RUNNING`（启用中）/ `STOP`（已禁用），**不是** boolean。
- `runningCount` 是当前 Base 下 status=RUNNING 的工作流数，方便快速判断「有几个流程在跑」。

### workflow get — 获取单个工作流详情

```bash
dws aitable workflow get --base-id BASE_ID --workflow-id WORKFLOW_ID --format json
```

| flag | 说明 |
|------|------|
| `--base-id` | 必填 |
| `--workflow-id` | 必填，对应 list 出参里的 `flowId` |

返回完整工作流配置：

```json
{
  "data": {
    "name": "流程1",
    "namespace": "...",
    "status": "RUNNING",
    "versionId": "G-FLOW-VER-XXXXXX",
    "versionNo": 14,
    "versionStatus": "...",
    "accessor": {...},                  // 访问者信息
    "corpId": "...",
    "flowAttribute": {...},             // 流程顶层属性
    "flowSchema": {...},                // ★ 流程节点定义（触发器/动作/分支等）
    "gmtCreate": 1780317804000,
    "gmtModified": 1780318540000
  }
}
```

`flowSchema` 是完整的节点 DAG，结构因流程而异（条件触发器 vs 定时触发器、单分支 vs 多分支等）。agent 应按需读取关心字段，不要试图建静态 schema。

### workflow enable — 启用工作流

```bash
dws aitable workflow enable --base-id BASE_ID --workflow-id WORKFLOW_ID --format json
```

返回 `{workflowId, enabled: true}` —— **`enabled: true` 是动作确认，不是当前状态查询**。要确认真启用了，必须再 `workflow list` 看 `status` 是否变成 `"RUNNING"` 或 `runningCount` 是否加 1。

### workflow disable — 禁用工作流（高危）

```bash
dws aitable workflow disable --base-id BASE_ID --workflow-id WORKFLOW_ID --yes --format json
```

返回 `{workflowId, disabled: true}` —— 同样是动作确认。禁用后该工作流不再自动触发。

**风险**：直接影响业务自动化（如停掉「记录创建后自动发通知」会让通知断流）。建议：
- 操作前先 `workflow get` 留底当前配置
- 脚本场景显式传 `--yes`；交互场景让用户在 prompt 中再次确认

## 能力边界

| 能力 | 状态 |
|------|------|
| 列出工作流 | ✅ |
| 看工作流详情（含 flowSchema） | ✅ |
| 启用/禁用 | ✅ |
| 新建工作流 | ❌ 当前不支持，请去 AI 表格 Web 端 → 数据表 → 自动化 创建 |
| 修改工作流配置 | ❌ 同上，需 Web UI 编辑 |
| 删除工作流 | ❌ 同上 |
| 查看运行历史/执行日志 | ❌ 暂未开放 |
| 手动触发/单次运行 | ❌ 暂未开放 |

## 错误码速查

| 场景 | code | type | 备注 |
|------|------|------|------|
| `workflow-id` 不存在调 get | `GET_WORKFLOW_ERROR` | `SYSTEM_ERROR` | message 可能为 null，先 `workflow list` 核对 ID |
| `workflow-id` 不存在调 enable | `ENABLE_WORKFLOW_ERROR` | `SYSTEM_ERROR` | message 含 "场域中不存在该 namespace" |
| `workflow-id` 不存在调 disable | `DISABLE_WORKFLOW_ERROR` | `SYSTEM_ERROR` | 同上 |
| `--limit` < 1 或 > 100 | （CLI 层拦截） | — | `--limit 必须在 [1, 100] 范围内，got N` |
| `--offset` < 0 | （CLI 层拦截） | — | `--offset 必须 >= 0，got N` |

> 拿到 `*_WORKFLOW_ERROR / SYSTEM_ERROR` 时，先 `workflow list` 自查目标 ID 是否还存在、是否在当前 Base 下。

## 典型工作流

### 看看 Base 里有哪些自动化在跑

```bash
dws aitable workflow list --base-id BASE_ID --format json | jq '.data | {total: .recordCount, running: .runningCount, items: .list | map({name, status, flowId})}'
```

### 临时停掉某个流程做调试

```bash
# 1. 留底当前状态
dws aitable workflow get --base-id BASE_ID --workflow-id WORKFLOW_ID --format json > /tmp/wf-backup.json

# 2. 禁用
dws aitable workflow disable --base-id BASE_ID --workflow-id WORKFLOW_ID --yes --format json

# 3. 调试做完后重启
dws aitable workflow enable --base-id BASE_ID --workflow-id WORKFLOW_ID --format json

# 4. 确认 status=RUNNING
dws aitable workflow list --base-id BASE_ID --format json | jq '.data.list[] | select(.flowId == "WORKFLOW_ID") | .status'
```

### 批量关掉某个 Base 下所有 workflow（调试 / 迁移前清场）

```bash
for WF in $(dws aitable workflow list --base-id BASE_ID --limit 100 --format json | jq -r '.data.list[] | select(.status == "RUNNING") | .flowId'); do
  dws aitable workflow disable --base-id BASE_ID --workflow-id "$WF" --yes --format json | jq .status
done
```

## 注意事项

- `--workflow-id` 接受的就是 `list` 返回里的 `flowId`（同值，CLI 屏蔽了服务端字段名差异）。
- enable / disable 出参里的 `enabled` / `disabled` 是 **动作确认 flag**，不是当前状态字段。要确认真生效请走 `workflow list` 查 `status`。
- `workflow get` 的 `flowSchema` 结构随触发器/动作类型变化，不要假设固定字段。
- 新建/修改/删除工作流目前必须在 AI 表格 Web 端（数据表页面 → 自动化）完成。
