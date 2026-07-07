# 悟空 → 开源仓库同步专项 · 交接文档

> 最后更新：2026-07-07  
> 分支：`feat/remove-discovery`（开源 repo dingtalk-workspace-cli）

---

## 一、专项背景

悟空仓库 (`dws-wukong`) 发版或合入主干时，需要运行 `make sync-oss` 自动同步端点/产品/路由到开源仓库 (`dingtalk-workspace-cli`)。

**目标：** 修雨点一下 approve PR 即可完成同步，不再手改开源文件。

**当前核心约束：**
- 悟空业务产品代码必须来自 `dws-wukong` 的 Go module / 编译期同步产物，开源仓不手改 fork。
- 开源最终指令集只能做加法：`悟空同步产物 + 主干旧开源指令 + 开源定制包装`，不能因为服务发现下线丢老命令。
- skills 也按同一模型处理：悟空 baseline 同步，开源旧 skill 与定制说明做 overlay，并用最终命令集校验。
- `--help` 也按同一模型处理：悟空 Cobra baseline + 开源旧命令/定制包装；最终以二进制命令树为事实源。
- 真下线能力先保留 AI-friendly 提示或标记 unavailable，不能静默删除命令。

---

## 二、当前分支状态

### feat/remove-discovery 已完成的工作

1. **移除服务发现** — 切到静态端点模式，`dws schema` 返回 `"note":"static endpoint mode"`  
2. **移除 conference 后端产品** — 从 endpoints/routing/synclist 去除真实 MCP 注册；旧 CLI 路径保留 unavailable 兼容提示
3. **sync-oss 基线对齐** — `register_products.go` / `dws-data/syncdata/endpoints.go` / `dws-data/syncdata/routing.go` 都已用 sync-oss 从悟空同步  
4. **skills 同步** — mono + multi skills 从 wukong 同步到开源 `skills/`，`.qoder/skills/` 仅作为 Agent 安装目标
5. **发版** — 已打包给勤泽/重鱼/郑御白测试  
6. **`--ai-tag` flag** — `chat message send` 和 `reply` 命令新增 flag，默认 `true`；实际 `clawType` 走 `edition.ClawType()`，开源默认 `openClaw`，悟空版可保持 `wukong`
7. **静态端点缺口 guardrail** — 新增单测扫描已注册 open 产品 helper 中显式 server 调用，防止 `im` / `bot` / `attendance-wukong` 这类 endpoint 漏注册再次进入

### 未提交变更

```
M  internal/helpers/chat.go              --ai-tag flag + edition.ClawType()
M  internal/app/runner.go                endpoint_not_resolved AI-friendly 提示
M  internal/cli/loader.go                degraded hint 去服务发现化
?? test/unit/static_endpoint_coverage_test.go
?? dws-bin / dws-cli                     构建产物（不应提交）
```

### 待提交

上述代码改动需要 commit + push + 重新打 tag + 通知测试者。`dws-bin` / `dws-cli` 构建产物不要提交。

---

## 三、当前实现架构

**同步范围：**

| 同步内容 | 源（wukong） | 目标（open-source） |
|----------|-------------|---------------------|
| 端点列表 | `wukong.StaticServers()` | `dws-data/syncdata/endpoints.go` |
| 产品注册 | `wukong/products/register.go` → `RegisterProducts()` | `internal/helpers/register_products.go` → `init()` |
| MCP 路由表 | `products.CmdToProduct()` | `dws-data/syncdata/routing.go` |

**开源运行时：**
- `pkg/edition/default.go` 通过 Go module 引入 `github.com/DingTalk-Real-AI/dws-data/syncdata`。
- `openStaticServers()` 只做类型转换，不维护手写 endpoint。
- `internal/helpers/register_products.go` 是生成文件，注册开源可见产品。
- 主干旧开源指令（尤其 `dev connect`）属于开源 overlay，不能被悟空同步覆盖或删除。

**同步入口：** `/Users/huyz/Documents/data/dingding/dws/dws-wukong/cmd/sync-oss/main.go`

该入口直接调用悟空运行时代码：
- `wukong.StaticServers()` 生成 endpoint。
- `products.CmdToProduct()` 生成 routing。
- `products.RegisteredProductNames()` 生成注册表。
- 扫描同步产品源码里的 `callMCPToolOnServer("...")`，自动补依赖子 server。

**重要：** 这里已经不是旧文档里的 `scripts/sync-oss` / AST 抽取方案；后续执行以 `cmd/sync-oss` 为准。

## 四、服务发现下线后的兼容策略

老主干架构里，`EnvironmentLoader.Load()` 负责从 market/portal discovery 拉 envelope、缓存 catalog，并支撑 `dws schema`。`feat/remove-discovery` 切到静态端点后，这些职责必须由静态生成物和 overlay 承接：

1. **endpoint/catalog**：由 `dws-data/syncdata.StaticServers()` 提供，缺失时会触发 `endpoint_not_resolved`。
2. **routing**：由 `dws-data/syncdata.CmdToProduct()` 提供，不能漏主干旧命令的 product 映射。
3. **schema/help**：服务发现和动态 schema 下线后，不能再靠远程 schema 推断命令；`--help` 与 skill 必须以最终 open CLI 命令树为准生成或校验。
4. **不可用能力**：不优先删命令，先给 AI-friendly 提示，说明能力已下线/未注册，并给替代命令或处理建议。

勤泽发现的 `im` / `bot` / `attendance-wukong` 漏注册，本质就是静态端点目录没有完全承接服务发现时代的隐式 endpoint 能力；不是用户参数问题。

---

## 五、特殊指令实现（开源独有 vs 悟空）

### 开源独有模块

**`dws dev` 开发者工具链**（~30 个文件，悟空完全不存在）：
- `dev.go` — `dws dev` 总入口
- `devapp.go` / `devapp_connect.go` / `devapp_cursor.go` / `devapp_pretty.go` — app 生命周期管理
- `connect_daemon.go` / `connect_stream.go` / `connect_health.go` — daemon 长连接/流/健康检查
- `connect_card.go` / `connect_approval_card.go` / `connect_approval.go` — 卡片/审批
- `connect_opencode.go` / `connect_qoder_stream.go` / `connect_codex_appserver.go` — 多协议适配
- `connect_media.go` / `connect_knowledge.go` / `connect_at_poll.go` — 媒体/知识库/@轮询
- `connect_onboarding.go` / `connect_role.go` / `connect_lock.go` 等辅助文件

**基础架构**：
- `interfaces.go` — `Handler`/`RegisterPublic` 可插拔组件模型
- `common.go` — `resolveStringFlag` / `commandDryRun` / `writeCommandPayload` / `preferLegacyLeaf`
- `atomicwrite.go` — 原子文件写入
- `skill_name.go` — skill 名称规范化
- `register_products.go` — sync-oss 自动生成的产品注册表

### 共享文件中开源有差异的

| 文件 | 开源特殊实现 |
|------|-------------|
| `helpers.go` | `InitDeps()` 依赖注入、`CmdToProduct()` 导出、`isTakenOverByDynamic()` 永远返回 false；conference 不进入真实 MCP routing |
| `chat.go` | `--ai-tag` flag（默认 true）、clawType 走 `edition.ClawType()` |

### 悟空独有（开源不需要）

- `cmdutil_bridge.go` — 悟空内部命令工具桥接
- `register.go` — 悟空注册+驱逐逻辑（服务发现相关）
- 大量 `_test.go` — 悟空内部测试

---

## 六、关键文件路径

| 用途 | 路径 |
|------|------|
| 同步计划 | `/Users/huyz/.qoder/plans/deep-dale-swallow.md` |
| 开源 repo | `/Users/huyz/Documents/data/dingding/dws/dingtalk-workspace-cli` |
| 悟空 repo | `/Users/huyz/Documents/data/dingding/dws/dws-wukong` |
| dws-data 模块 | `/Users/huyz/Documents/data/dingding/dws/dws-data/` |
| 开源端点数据 | `dws-data/syncdata/endpoints.go` |
| 开源端点引入 | `dingtalk-workspace-cli/pkg/edition/default.go` |
| 开源注册 | `dingtalk-workspace-cli/internal/helpers/register_products.go` |
| 开源路由数据 | `dws-data/syncdata/routing.go` |
| 开源 chat | `dingtalk-workspace-cli/internal/helpers/chat.go` |
| 悟空端点 | `dws-wukong/wukong/endpoints.go` |
| 悟空注册 | `dws-wukong/wukong/products/register.go` |
| 悟空路由 | `dws-wukong/wukong/products/helpers.go` |
| sync-oss 入口 | `dws-wukong/cmd/sync-oss/main.go` |
| sync-oss 配置 | `dws-wukong/scripts/sync-oss/synclist.json` |
| skills 源 | `dws-wukong/target/open-source-cli/skills/` |
| 开源 skills 源码 | `dingtalk-workspace-cli/skills/` |
| Agent 安装目标 | `~/.qoder/skills/`、`~/.codex/skills/` 等，由 `dws skill setup` 写入 |

---

## 七、剩余待办

1. **提交当前 P0 改动** — chat `clawType`、endpoint hint、静态端点覆盖测试
2. **重新打 tag + release** — 通知勤泽/重鱼/郑御白更新
3. **主干旧指令兼容清单** — 对比 `origin/main` 与 `feat/remove-discovery`，确认 `dev connect` 等旧开源命令不丢
4. **help/skill 校验** — 以最终 open CLI 命令树校验 `--help` 与 skill.md，修复 50+ 文档不符
5. **首次手动全量同步** — 跑 `make sync-oss` 建立 baseline
6. **真下线能力标记** — 对不可恢复能力补 AI-friendly unavailable 提示和替代命令
7. ~~conference 后端产品移除~~ ✅（旧 CLI 路径保留 unavailable 兼容提示）
8. ~~skills 同步~~ ✅
9. ~~发版测试~~ ✅

---

## 八、注意事项

- `feat/remove-discovery` 分支的服务发现代码/兼容层已完全移除，不要再加回来
- 开源不接受悟空专属产品（如 `law`/`tb`/`yida`/`finance`/`credit`），这些是悟空扩展
- sync-oss 范围在 `synclist.json` 里控制；同步产物只来自悟空 module，不手写补业务产品代码
- `--ai-tag` 默认 true，但 `clawType` 取 edition：开源默认 `openClaw`，不要硬编码 `wukong`
- 构建命令：`go build -o dws-cli ./cmd/`（不要 `-o dws` 会跟目录冲突）
