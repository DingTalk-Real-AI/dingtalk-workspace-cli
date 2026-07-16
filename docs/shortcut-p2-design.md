# DWS Shortcut P2 详细设计 — 高频场景自动沉淀为自定义 Shortcut

> 前置：P1 已交付静态声明式 shortcut 框架（`internal/shortcut/`），见 `docs/shortcut-plan.md`。
> P2 目标：**观察用户高频使用 → 主动建议 → 一键沉淀为可复用的自定义 shortcut**，
> 让 CLI 越用越顺手。这是 dws 相对 larksuite/cli 的差异化能力。

## 0. 体验闭环（一句话）

```
用户反复敲 dws chat send_message --json '{"open_conversation_id":"cid_x","text":"..."}'
        │  （每次执行被静默记录到 ~/.dws/usage.jsonl）
        ▼
第 N 次后，dws 主动提示：
  💡 你已 12 次向「项目群」发消息，是否沉淀为 `dws chat +notify-team`？[y/N]
        │  y
        ▼
写入 ~/.dws/shortcuts/chat.notify-team.yaml
        ▼
之后：dws chat +notify-team --text "发布完成"   ← 参数从一堆 JSON 收敛成一个 flag
```

## 1. 总体架构

复用 P1 的 `Shortcut` 模型作为「编译目标」，新增四个部件：

| 部件 | 位置（建议） | 职责 |
|------|-------------|------|
| Usage 埋点 | `internal/shortcut/usage/recorder.go` | 每次 MCP 调用后追加一条 usage 记录 |
| 模式挖掘 | `internal/shortcut/usage/miner.go` | 聚合 usage → 高频候选 + 打分 |
| 主动提示 | `internal/shortcut/usage/nudge.go` | 命中候选时在命令收尾处提示 |
| YAML 加载 | `internal/shortcut/userdef/loader.go` | 扫描 `~/.dws/shortcuts/*.yaml` → 编译成 `Shortcut` → 注册 |
| 管理命令 | `internal/shortcut/usage`（cobra） | `dws shortcut list/suggest/add/rm/stats` |

数据流：

```
CallMCP ──► recorder.Append(usage)                 [写侧，热路径，必须极轻]
                                    ┌─► miner.TopCandidates()  [读侧，suggest 时才算]
~/.dws/usage.jsonl ─────────────────┤
                                    └─► nudge (命令收尾抽样触发)
~/.dws/shortcuts/*.yaml ──► loader.Compile() ──► shortcut.Register()  [启动时]
```

## 2. Usage 埋点

### 2.1 采集点（choke point）

**首选**：装饰 `executor.Runner` / `edition.ToolCaller`。`internal/app/tool_caller_adapter.go`
的 `CallTool(ctx, productID, toolName, args)` 是**所有 MCP 调用的唯一必经点**，天然拿到
`(product, tool, args)` 三元组。用装饰器包一层即可，零侵入命令层：

```go
type recordingCaller struct{ inner edition.ToolCaller }
func (r recordingCaller) CallTool(ctx, product, tool string, args map[string]any) (*edition.ToolResult, error) {
    res, err := r.inner.CallTool(ctx, product, tool, args)
    usage.Append(product, tool, args, err == nil)   // 异步/带 recover，绝不影响主流程
    return res, err
}
```

> 注意：P1 的 shortcut 也走这条 `CallMCP → helpers → deps.Caller`，所以内建 shortcut 的
> 使用同样会被记录，可用于「哪些内建 shortcut 最受欢迎」的洞察。

### 2.2 记录内容（**隐私优先：记形状不记值**）

`~/.dws/usage.jsonl`，每行一条：

```json
{
  "ts": "2026-07-08T10:12:33+08:00",
  "product": "chat",
  "tool": "send_message",
  "arg_keys": ["open_conversation_id", "text"],
  "const_args": {"open_conversation_id": "cid_x"},
  "ok": true
}
```

- `arg_keys`：参数键集合（排序），用于识别「同一种调用形状」。
- `const_args`：**仅收敛出的「疑似固定值」**（见 §3 挖掘时判定），写入时不保证脱敏，
  因此需要一层白名单/黑名单：`text/content/body/message` 等自由文本字段**永不入库**，
  只保留看起来像 ID/枚举的短值（长度阈值 + 无空格 + 非多行）。
- 绝不记录：token、手机号、邮箱、文件内容、消息正文。用 `internal/logging/redact.go`
  已有的脱敏能力复核。

### 2.3 热路径约束

- 追加写用 `O_APPEND`，单行 < 1KB；失败静默（`recover` + debug 日志），**绝不阻断命令**。
- 文件滚动：超过 N 行（如 5000）或 M 天，截断/归档，避免无限增长。
- 开关：默认关闭（opt-in），环境变量 `DWS_USAGE_TRACKING=1` 开启（本地遥测即便只记形状也不应未经用户同意默认开启）。
  首次启用时在 `dws` 首跑给一次性告知（尊重知情）。

## 3. 模式挖掘

`dws shortcut suggest` 触发（也被 nudge 复用）。算法：

1. 读 usage.jsonl，按 `(product, tool, arg_keys)` 分桶。
2. 对每桶：
   - `count` = 出现次数；低于阈值（默认 5）直接丢弃。
   - 对每个 arg_key，统计其值的分布：某值占比 ≥ 80% → 判定为**固定值**（进 `const_args`）；
     否则判定为**可变参数**（沉淀后成为 flag）。
   - `recency` = 最近一次使用距今；越近权重越高。
3. 打分 `score = count * log(distinct_days+1) * recencyDecay`，取 TopN。
4. 生成候选 `Candidate{product, tool, fixed{...}, varFlags[...], score, samples}`。

输出示例（`dws shortcut suggest --format table`）：

```
候选  | 命令建议            | 依据                     | 固定参数           | 可变flag
#1    | chat +notify-team  | 12 次 / 近 3 天          | open_conv=cid_x    | text
#2    | doc +new-agenda    | 7 次 / 近 5 天           | template=agenda    | title
```

## 4. 主动提示（nudge）

- **时机**：命令成功收尾时（root `PersistentPostRunE`），**抽样**触发（如每 N 次调用或每次
  命中新达标候选时），避免打扰。仅在 TTY 交互态提示；非交互（Agent/管道/`--yes`）**不提示**。
- **频控**：同一候选提示过一次被拒后，冷却期内不再提示（记 `~/.dws/shortcuts/.declined`）。
- **交互**：
  ```
  💡 检测到高频操作：你已 12 次向同一会话发消息。
     沉淀为快捷指令  dws chat +notify-team --text "..."  ？
     [y] 沉淀  [n] 以后再说  [d] 不再提示此项
  ```
- y → 走 §5 生成 YAML；命名默认 `+<tool 去下划线的动宾>`，允许用户改名。

## 5. 自定义 Shortcut：YAML 格式与运行时加载

### 5.1 YAML schema（与 P1 `Shortcut` 一一对应）

`~/.dws/shortcuts/chat.notify-team.yaml`：

```yaml
version: 1
service: chat
command: "+notify-team"
product: chat
description: "发消息到 项目群（自动沉淀于 2026-07-08）"
risk: write                    # 默认 read；send 类判定为 write
source: auto                   # auto=沉淀 / manual=手写
flags:
  - name: text
    type: string
    required: true
    desc: 消息内容
execute:
  tool: send_message
  bind:                        # 参数绑定：常量 + ${flag} 模板
    open_conversation_id: "cid_x"
    text: "${text}"
```

### 5.2 编译与注册

`userdef.Compile(yaml)` → `shortcut.Shortcut`，其 `Execute` 由 `bind` 生成：
遍历 `bind`，`${flag}` 用 `rt.Str(flag)` 填充，常量原样，组装 params 后 `rt.CallMCP(tool, params)`。
完全复用 P1 的 runner，不新增执行路径。

加载时机：`legacy.go` 装配点，在 `builtin.Commands()` 之后追加 `userdef.Commands()`，
一起 merge。复用 `internal/plugin/loader.go` 已验证的「扫 `~/.dws/` 目录 + 挂 cobra」模式。

### 5.3 冲突与优先级

- 自定义 shortcut 命令名若与内建 shortcut / helper 冲突：**内建优先**，自定义重命名或跳过并告警。
- `+` 前缀天然与 helper leaf 区分，冲突面小。

## 6. 管理命令面

```
dws shortcut list                 # 列出内建 + 自定义 shortcut
dws shortcut suggest [--min N]    # 展示高频候选（不写入）
dws shortcut add <candidate|--from-last>   # 交互式/从最近一次调用沉淀
dws shortcut rm <service> <+cmd>  # 删除自定义 shortcut
dws shortcut stats                # usage 统计概览
```

`dws shortcut` 本身作为一个新的顶层 utility 命令注册（对齐 `dws plugin`）。

## 7. 安全与隐私边界（红线）

1. **值不入库**：自由文本/正文/凭证一律不记；`const_args` 仅短 ID/枚举，且过 redact 复核。
2. **可关可清**：`DWS_USAGE_TRACKING=0` 关闭；`dws shortcut stats --purge` 清空 usage。
3. **执行白名单**：自定义 shortcut 的 `execute.tool` 必须解析到合法 MCP server（过
   `internal/security` endpoint 白名单），禁止指向任意 endpoint。
4. **不自动执行**：沉淀只生成命令定义，**绝不**自动发起写操作；写类 shortcut 仍受 P1 的
   risk 确认约束。
5. **知情**：首次开启埋点一次性告知；提示可永久关闭。

## 8. 实现顺序（P2 分步，便于 loop 推进）

- P2-1 usage 埋点：`recordingCaller` 装饰器 + `usage.Append` + jsonl 写 + 开关 + 脱敏白名单。
- P2-2 `dws shortcut stats` / `list`：先让数据可见，验证埋点质量。
- P2-3 miner + `dws shortcut suggest`：离线挖掘与打分。
- P2-4 userdef YAML 加载 + `Compile` + 注册 + 冲突处理（打通「手写 YAML 也能用」）。
- P2-5 `dws shortcut add`（从候选/最近调用沉淀）。
- P2-6 nudge 主动提示（最后做，最谨慎，默认保守频控）。

## 9. 待决策点（需产品确认）

1. 埋点默认开还是默认关？→ **修订后：默认关（opt-in）+ 开启后首跑一次性告知**（原设计默认开，反思后改为 opt-in：自主 agent 不应单方面默认开本地遥测）
   （`DWS_USAGE_TRACKING=0` / 配置项关闭；`dws shortcut stats --purge` 清空）。实现时以此为准。
2. `const_args` 允许记录的字段白名单粒度？（保守起步：只记形如 `*_id/*Id/type/status` 的短值）
3. nudge 触发频率与渠道？（建议：仅 TTY、命中新候选时、每候选一生仅一次）
4. 自定义 shortcut 是否需要跨设备同步？（v1 先本地 `~/.dws/`，同步留待后续）
