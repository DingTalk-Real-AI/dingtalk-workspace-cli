# dws 多组织登录技术方案

生成时间：2026-06-26

## 目标

让同一个本机用户可以在 dws 中登录多个钉钉组织，并在后续命令执行时明确选择组织上下文。能力必须位于 dws 顶层，便于终端调度直接命中。

## 命令设计

### 登录

首次登录：

```bash
dws auth login --format json
```

继续登录第二个或第三个组织：

```bash
dws auth login --force --format json
```

设备码模式：

```bash
dws auth login --device --force --format json
```

说明：不新增 `--associated`。OAuth 授权结果中的 `corpId` 是 profile 的唯一组织键。

### 查看 profile

```bash
dws profile list --format json
dws profile ls
```

JSON 输出包含 `primaryProfile`、`currentProfile`、`previousProfile` 和 `profiles[]`；profile 项包含 `name`、`corpId`、`corpName`、`userId`、`userName`、`status`、过期时间与 current/primary 标记。表格输出必须展示组织名，避免只展示 corpId。

### 切换 profile

```bash
dws profile use [name-or-corpId] --format json
dws auth switch [name-or-corpId] --format json
dws profile use - --format json
dws auth switch - --format json
```

`profile use <name-or-corpId>` 持久切换默认 current；`auth switch <name-or-corpId>` 是产品兼容入口，语义等价。`profile use -` 和 `auth switch -` 在 current 和 previous 间 toggle。无参数执行 `dws auth switch` 或 `dws profile use` 时，在交互终端展示组织选择 TUI；非交互环境要求显式传入 profile 名或 corpId。切换成功后同步 legacy `auth-token` 镜像，并清理进程内 token/runtime cache。

### 单次命令覆盖

```bash
dws --profile <name-or-corpId> <product> <command> --format json
```

`--profile` 只影响本次运行，不修改 `currentProfile`。适合 agent 在一个任务中轮询多个组织。

## 数据模型

### profiles.json

`profiles.json` 只保存非敏感元数据：

- `primaryProfile`：首次成功登录的组织，用于默认 fallback 和标记；`auth logout` 默认全登出时会一并清除。
- `currentProfile`：默认命令上下文。
- `previousProfile`：上一个 current，用于 `profile use -` 或 `auth switch -`。
- `profiles[]`：按 `corpId` 维护 profile 元数据。

不得在 `profiles.json` 中保存 access token、refresh token、persistent code 或 client secret。

### keychain token 槽

- 新槽：`auth-token:<corpId>`
- legacy 镜像：`auth-token`

每个组织 token 独立存储。当前 profile 的 token 会同步到 legacy 槽，兼容旧二进制、旧宿主或只检查 token marker 的逻辑。

## 运行时解析

profile 解析优先级：

1. 全局 `--profile`
2. `currentProfile`
3. `primaryProfile`
4. legacy `auth-token`

命令初始化时先预解析 `--profile`，再构造运行时 loader，确保插件命令获取 token 前已经有正确的组织上下文。

## 第二/第三组织登录语义

第二个组织和第三个组织都是“重复登录同一个自然人但选择不同组织”的场景。CLI 不需要知道这是第几个组织，只看 OAuth 返回的 `corpId`：

- 新 `corpId`：新增 profile，写入 `auth-token:<corpId>`，设为 current。
- 已存在 `corpId`：刷新该 profile token 和元数据，不新增重复项。
- 首个 profile：同时成为 primary 和 current。
- 后续 profile：成为 current，原 current 进入 previous。

## 被拒绝或延期的产品逻辑

以下产品稿能力不进入 P0 技术实现：

- `dws auth list`：替换为 `dws profile list`。
- `dws auth switch`：保留为 `dws profile use` 的兼容入口；无参数时必须展示 TUI。
- `dws auth login --associated`：替换为重复执行 `dws auth login --force`。
- `--组织corp ID`：替换为全局 `--profile <name|corpId>`。
- 自动发现所有所属组织：P1；P0 只展示主动登录过的 profile。
- 内置跨组织聚合 `--all-orgs`：不做；由 agent 编排多次 `--profile` 调用。

## 验收标准

- 首次登录创建 primary/current profile。
- 第二/第三组织登录不会覆盖已有组织 token。
- 同组织重复登录只刷新，不重复新增。
- `dws profile list` 顶层可见，JSON 和表格都展示组织名。
- `dws profile use` 与 `dws auth switch` 可按 name/corpId 切换，可用 `-` 切回 previous，无参数时展示 TUI。
- `--profile` 可一次性指定组织，且不改变 current。
- `auth logout` 默认清理所有组织登录态；`auth logout --profile <name|corpId>` 只清指定组织；`auth reset` 额外清 app config 等本机认证配置。
- legacy 单槽可迁移，current token 可镜像到 legacy 槽。

## 验证命令

```bash
go test ./internal/auth ./internal/app -run 'Test(MultiProfile|RuntimeProfile|DeleteProfile|UpsertProfile|LoadProfiles|LegacyKeychain|WriteProfile|ProfileList|ProfileUse|ProfileSwitch|AuthSwitch|AuthStatus|AuthLogout|AuthLogin|ResolveAuthLogin|EnrichAuthLogin|RootHelp|RootShortHelp|RootCommand)'
dws version
dws profile list --format json
```
