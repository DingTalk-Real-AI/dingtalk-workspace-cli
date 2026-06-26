# 多组织登录验收评审

生成时间：2026-06-26

## 评审结论

无阻塞问题。按技术方案口径，本轮多组织登录 P0 能力可以验收：多槽 token、profile 元数据、顶层 profile 命令、全局 `--profile`、legacy 兼容与主组织保护均已有代码和测试覆盖。

## 需求对齐

- PRD 已落地：`prd.json` 与 `docs/ralph/dws-multi-profile-login/prd.json`
- 顶层命令已落地：`dws profile list/switch`
- 第二/第三组织登录路径已落地：重复 `dws auth login`，默认进入授权流程以新增/刷新组织 profile
- 单次组织指定已落地：全局 `--profile`
- 组织名展示已落地：profile JSON 包含 `corpName`，表格包含 `ORG_NAME`
- 技术方案拒绝项已裁决：不实现 `auth list/--associated/--组织corp ID/auth switch`；切换组织统一使用 `dws profile switch`，无参数展示 TUI

## 代码证据

- `internal/auth/profiles.go`：维护 `primaryProfile`、`currentProfile`、`previousProfile`，按 `corpId` upsert profile。
- `internal/auth/token.go`：token 写入 `auth-token:<corpId>`，并同步 legacy `auth-token`。
- `internal/app/profile_command.go`：实现 `profile list`、`profile switch [name|corpId|-]`，无参数展示 TUI，输出组织名和 corpId；`profile use` 保留为兼容别名。
- `internal/app/auth_command.go`：保留登录、登出、状态、导入导出等认证动作，不暴露 `auth switch`。
- `internal/app/root.go`：注册顶层 `profile` 命令，并在运行时预解析/注入全局 `--profile`。
- `internal/app/auth_command.go`：auth status/logout/reset 对 profile 语义做了补齐；`auth logout` 默认清理所有组织，`--profile` 只清指定组织。

## 测试证据

已通过：

```bash
go test ./internal/auth ./internal/app -run 'Test(MultiProfile|RuntimeProfile|DeleteProfile|UpsertProfile|LoadProfiles|LegacyKeychain|WriteProfile|ProfileList|ProfileUse|ProfileSwitch|AuthCommandDoesNotExposeSwitch|AuthStatus|AuthLogout|AuthLogin|ResolveAuthLogin|EnrichAuthLogin|RootHelp|RootShortHelp|RootCommand)'
```

结果：

- `ok github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth`
- `ok github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app`

本机 dws 验证：

- `dws version`：本机 `dws` 已按 PR 最新构建安装，版本为 `v1.0.41-SNAPSHOT`
- `dws profile list --format json`：当前本机存在两个 profile，一个 primary，一个 current，说明第二组织登录已进入多槽 profile 体系。

## 未阻塞但需记录的风险

- 全量 `go test ./internal/auth ./internal/app` 被 `internal/app` 中 upgrade 相关用例 `TestValidateNewBinary_RecoversFromUnsignedDarwin` 阻塞，错误为测试二进制执行被 macOS kill。该路径与多组织登录无直接耦合，应单独排查本机签名/隔离属性/测试环境。
- real/embedded hook 后端仍不支持显式 `--profile`，这是技术方案明确的 P0 非目标。
- P0 不自动发现所有所属组织；第三组织必须由用户再次完成 OAuth 授权后才会出现在 `profile list`。
- `auth logout` 与飞书 CLI 的登出心智对齐为“清当前认证上下文下的用户登录态”；在 dws 多组织模型里，不带 `--profile` 表示清所有已登录组织。
- 跨组织聚合由 agent 编排，不由 CLI 内置 `--all-orgs`。

## 验收判断

可以验收。对产品经理侧，当前可交付用户路径是：

1. 首次 `dws auth login` 登录主组织。
2. 继续 `dws auth login` 登录第二/第三组织。
3. 用 `dws profile list` 看组织列表。
4. 用 `dws profile switch` 切默认组织；无参数时弹 TUI，也可选择主组织切回。
5. 用 `dws --profile <corpId>` 做单次跨组织调度。

这条路径覆盖了“多组织登录、切换、终端可调度、组织名可见”的核心需求。
