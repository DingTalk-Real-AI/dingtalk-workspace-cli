## Ralph 验收补充

本轮按技术方案优先收敛了多组织登录能力，并把在线资料、PRD、技术方案、验收评审都落到了 `docs/ralph/dws-multi-profile-login/`。

### 关键结论

- 第二/第三个组织不新增特殊命令，继续执行 `dws auth login --format json`，在 OAuth 页选择目标组织。
- 新 `corpId` 会新增 profile；已存在 `corpId` 只刷新 token 和元数据。
- 查看组织：`dws profile list --format json`。
- 持久切换：`dws profile switch <name|corpId|-> --format json`；目标可以是主组织，选中主组织即可切回。
- 交互切换：`dws profile switch` 无参数时展示组织选择 TUI，列表包含主组织、当前组织和已登录附属组织。
- 单次命令指定组织：`dws --profile <name|corpId> <product> <command> --format json`。
- 登出：`dws auth logout` 默认清理所有组织登录态；`dws auth logout --profile <name|corpId>` 只清指定组织；`--all` 已移除。
- 产品稿里的 `auth list/--associated/--组织corp ID/auth switch` 不进入 P0，分别由 `profile list`、重复 `auth login`、全局 `--profile` 和 `profile switch` 替代；`auth` 命令组不暴露 switch。

### 验证

```bash
go test ./internal/auth ./internal/app -run 'Test(MultiProfile|RuntimeProfile|DeleteProfile|UpsertProfile|LoadProfiles|LegacyKeychain|WriteProfile|ProfileList|ProfileUse|ProfileSwitch|AuthCommandDoesNotExposeSwitch|AuthStatus|AuthLogout|AuthLogin|ResolveAuthLogin|EnrichAuthLogin|RootHelp|RootShortHelp|RootCommand)'
go test ./internal/auth ./internal/app
```

结果：`internal/auth` 与 `internal/app` 均通过。

另已验证本地打包安装，本机 `dws` 已指向本 PR 最新构建。
