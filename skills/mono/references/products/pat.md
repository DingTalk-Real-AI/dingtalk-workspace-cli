# 行为授权 (pat) 命令参考

PAT（Permission/Action Token，行为授权）管理 AI agent 调用 dws 各产品能力时所需的 **操作权限（scope）**。当某条业务命令因缺少授权被服务端拦住（撞墙）时，由本产品按 scope 授予权限后才能继续。这是 agent 必须理解的权限链路。

## 命令总览

### 授予权限 (chmod)
```
Usage:
  dws pat chmod <scope>... [flags]
Example:
  dws pat chmod aitable.record:read --grant-type session --session-id session-xxx
  dws pat chmod chat.message:list --grant-type once
  dws pat chmod aitable.record:read aitable.record:write --grant-type permanent --yes
  dws pat chmod --product calendar --product aitable --grant-type once --dry-run --format json
  dws pat chmod --products calendar,aitable --grant-type session --session-id session-xxx --yes
  dws pat chmod --domain calendar --domain chat --grant-type once --yes
  dws pat chmod --recommend --grant-type session --session-id session-xxx --yes
Flags:
      --grant-type string     授权策略: once|session|permanent (默认 session)
      --session-id string     会话标识 (session 模式下必填)
      --products strings      产品编码列表, 逗号分隔; 按产品 scope 模板批量授权; 执行需 --yes
      --product stringArray   产品编码, 可重复; 与 --products 等价; 执行需 --yes
      --domains strings       产品域/产品编码列表, 逗号分隔; 按域 scope 模板批量授权; 执行需 --yes
      --domain stringArray    产品域/产品编码, 可重复; 执行需 --yes
      --recommend             使用服务端推荐 scope 集合批量授权; 执行需 --yes
      --agentCode string      Agent 唯一标识 (可选; 也可由 env DINGTALK_DWS_AGENTCODE 注入, flag 优先; 不传由服务端兜底)
```

**scope 格式**: `<product>.<entity>:<permission>`
例: `aitable.record:read`、`chat.group:write`、`calendar.event:read`、`chat.message:list`

**grant-type（授权策略）**:
| 值 | 含义 |
|------|------|
| `once` | 一次性，执行一次后自动失效 |
| `session` | 当前会话有效（**默认**），需配 `--session-id` |
| `permanent` | 永久有效 |

**批量授权三种入口**（与直接传多个 scope 互补）:
- `--products` / `--product`：按产品编码展开该产品的 scope 模板
- `--domains` / `--domain`：按产品域展开 scope 模板
- `--recommend`：使用服务端推荐 scope 集合

批量模式下 CLI 先生成 **batch plan**（返回 `selected` / `skipped` / `pending` 明细），再对 `selected` 执行 batch grant。`--dry-run` 只预览计划、不写入。真正执行批量授权 **必须显式加 `--yes`**；未加 `--yes` 时 CLI 会阻断并提示先确认。

### 浏览器策略 (browser-policy)
```
Usage:
  dws pat browser-policy [flags]
Flags:
      --enabled            PAT 撞墙时是否允许本地打开浏览器
      --agentCode string   Agent 唯一标识 (不填则写入全局默认策略, 此命令不从 env 回退)
```

配置 PAT 撞墙（缺权限）时是否在本地拉起浏览器完成授权。`--enabled` 写入「允许打开」策略。是否打开浏览器由本地 PAT 策略单独决定，与输出是否为 json **独立**。

策略读取顺序：策略生效时优先按 `DINGTALK_DWS_AGENTCODE` 读 agent 策略，再回退默认策略。写 agent 策略需显式传 `--agentCode`；不传写入全局默认策略。

## 意图判断

- 用户/agent 说"授权/给权限/开权限/scope/缺权限/撞墙" → `pat chmod`
- 用户说"按整个产品/整个域开权限""一次把表格相关权限都开了" → `pat chmod --products/--domains/--recommend`
- 用户说"先看看会开哪些权限/不要真授" → 加 `--dry-run`
- 用户说"配置是否弹浏览器/不要弹浏览器/允许弹浏览器授权" → `pat browser-policy`

## 核心工作流

```bash
# 单条 scope, 当前会话有效
dws pat chmod aitable.record:read --grant-type session --session-id session-xxx --format json

# 一次性授权(执行后失效)
dws pat chmod chat.message:list --grant-type once --format json

# 多条 scope 永久授权(写入需 --yes)
dws pat chmod aitable.record:read aitable.record:write --grant-type permanent --yes --format json

# 按产品批量, 先预览计划不写入
dws pat chmod --product calendar --product aitable --grant-type once --dry-run --format json

# 确认计划后批量授予(必须 --yes)
dws pat chmod --products calendar,aitable --grant-type session --session-id session-xxx --yes --format json

# 用推荐集合批量授权
dws pat chmod --recommend --grant-type session --session-id session-xxx --yes --format json

# 关闭撞墙弹浏览器(写全局默认策略)
dws pat browser-policy --format json   # 不带 --enabled = 不允许
dws pat browser-policy --enabled --format json   # 允许打开

# 给指定 agent 写浏览器策略
dws pat browser-policy --enabled --agentCode <AGENT_CODE> --format json
```

## 注意事项

- **批量授权必须 `--yes`**：`--products` / `--domains` / `--recommend` 真正写入前未加 `--yes` 时 CLI 会阻断；先用 `--dry-run` 看 `selected/skipped/pending`，确认后再加 `--yes`。
- `session` 模式（默认）必须配 `--session-id`，否则无法定位会话。
- `chmod` 默认输出轻量授权摘要；要逐 scope 明细的完整服务端 JSON，加 `--format json` 或 `--verbose`。
- **agentCode**：可由 `--agentCode` 或 env `DINGTALK_DWS_AGENTCODE` 注入，flag 优先；不传由服务端默认兜底。
- **Host-owned PAT 开关**：当 env `DINGTALK_DWS_AGENTCODE` 非空时，CLI 命中 PAT 固定以 stderr JSON + exit=4 的 host-owned 形式返回，交由宿主处理全部 UI / 交互 / 重试，CLI 侧不再拉起本地浏览器或轮询。
- 浏览器策略与 json 输出相互独立：是否弹浏览器只由 `browser-policy` 决定。
