## Ralph 验收补充

本轮按技术方案优先收敛了多组织登录能力，并把在线资料、PRD、技术方案、验收评审都落到了 `docs/ralph/dws-multi-profile-login/`。

### 关键结论

- 第二/第三个组织不新增特殊命令，继续执行 `dws auth login --force --format json`，在 OAuth 页选择目标组织。
- 新 `corpId` 会新增 profile；已存在 `corpId` 只刷新 token 和元数据。
- 查看组织：`dws profile list --format json`。
- 持久切换：`dws profile use <name|corpId|-> --format json` 或 `dws auth switch <name|corpId|-> --format json`。
- 交互切换：`dws auth switch` 或 `dws profile use` 无参数时展示组织选择 TUI。
- 单次命令指定组织：`dws --profile <name|corpId> <product> <command> --format json`。
- 登出：`dws auth logout` 默认清理所有组织登录态；`dws auth logout --profile <name|corpId>` 只清指定组织；`--all` 已移除。
- 产品稿里的 `auth list/--associated/--组织corp ID` 不进入 P0，分别由 `profile list`、重复 `auth login --force` 和全局 `--profile` 替代；`auth switch` 作为 `profile use` 的兼容入口保留。

### 验证

```bash
go test ./internal/auth ./internal/app -run 'Test(MultiProfile|RuntimeProfile|DeleteProfile|UpsertProfile|LoadProfiles|LegacyKeychain|WriteProfile|ProfileList|ProfileUse|ProfileSwitch|AuthSwitch|AuthStatus|AuthLogout|AuthLogin|ResolveAuthLogin|EnrichAuthLogin|RootHelp|RootShortHelp|RootCommand)'
```

结果：`internal/auth` 与多组织相关 `internal/app` 用例通过。

另已验证本地打包安装：

- `dws version`：`v1.0.41-SNAPSHOT`，commit `756c7d1`
- `dws profile list --format json`：本机已有两个 profile，一个 primary，一个 current

### 残余说明

全量 `go test ./internal/auth ./internal/app` 中 `internal/app` 被 upgrade 模块用例 `TestValidateNewBinary_RecoversFromUnsignedDarwin` 阻塞，错误是测试二进制执行被 macOS kill。该失败不在本次多组织登录改动面内，建议作为独立本机签名/隔离环境问题跟进。
