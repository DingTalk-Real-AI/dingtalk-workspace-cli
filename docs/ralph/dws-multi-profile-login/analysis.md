# dws 多组织登录综合分析

生成时间：2026-06-26

## 本地资料

- 源文档索引：`docs/ralph/dws-multi-profile-login/source/source-manifest.json`
- 技术方案原文导出：`docs/ralph/dws-multi-profile-login/source/mweZ92PV6O36dZbnsMLBrOLGJxEKBD6p-tech-solution.md`
- 方案补充原文导出：`docs/ralph/dws-multi-profile-login/source/MyQA2dXW7oOA63YacZgBYPbmWzlwrZgb-supplement.md`
- 产品方案原文导出：`docs/ralph/dws-multi-profile-login/source/MyQA2dXW7oOA63YacZ4vrQ1RWzlwrZgb-product-plan.md`
- PRD：`docs/ralph/dws-multi-profile-login/prd.json`
- 收敛技术方案：`docs/ralph/dws-multi-profile-login/technical-solution.md`
- 验收评审：`docs/ralph/dws-multi-profile-login/review/acceptance-review.md`
- PR 评论稿：`docs/ralph/dws-multi-profile-login/comments/pr-comment.md`

导出说明：源文档通过 `dws doc info/read` 拉取，本地 Markdown 中的阿里文档 OSS 签名图片 URL 已替换为 `redacted://alidocs-signed-image`，避免把临时签名链接固化进仓库。

## 决策结论

本轮以技术方案为主线。产品稿中提到的 `auth list`、`auth switch`、`auth login --associated`、`--组织corp ID` 属于被技术方案替换的命名或交互，不作为当前实现验收口径。当前 P0 采用：

- `dws auth login --force`：继续登录第二个、第三个组织。
- `dws profile list`：查看已登录组织。
- `dws profile use <name|corpId|->`：持久切换当前组织。
- `dws --profile <name|corpId> <product> <command>`：单次命令临时指定组织。

这个拆分符合“auth 管凭证、profile 管组织上下文”的边界，也让终端调度 dws 时能在顶层直接命中 profile 管理能力。

## 第二、第三个组织怎么处理

### 登录第二个组织

已有第一个组织后，继续执行：

```bash
dws auth login --force --format json
```

浏览器授权页里选择第二个目标组织并授权。登录成功后，CLI 根据返回 token 中的 `corpId` 写入独立 keychain 槽 `auth-token:<corpId>`，并在 `profiles.json` 中新增 profile。新登录的组织会成为 `currentProfile`，原来的 current 会进入 `previousProfile`。

如果是 SSH/headless 环境，使用设备码模式：

```bash
dws auth login --device --force --format json
```

登录后检查：

```bash
dws profile list --format json
```

当前本机验收时已经有两个 profile：主组织 `ding8196cd9a2b2405da24f2f5cc6abecb85`，以及当前组织 `ding32fff839a3e0105d`。这说明第二组织已经按新 profile 槽位进入本机 dws。

### 登录第三个组织

第三个组织不需要新命令，重复第二组织流程：

```bash
dws auth login --force --format json
dws profile list --format json
```

选择第三个组织授权后，如果返回的是新的 `corpId`，它会新增为第三个 profile；如果返回的是已经存在的 `corpId`，只刷新该 profile 的 token 和组织/用户元数据，不产生重复项。

### 切换与临时使用

持久切换默认组织：

```bash
dws profile use <name-or-corpId> --format json
```

切回上一个组织：

```bash
dws profile use - --format json
```

单次命令指定组织，不改变默认 current：

```bash
dws --profile <name-or-corpId> <product> <command> --format json
```

跨组织聚合由 agent 编排：先 `dws profile list --format json` 拿到 profile，再对每个 profile 带 `--profile <corpId>` 分别调用业务命令，最后由 agent 合并结果并标注来源组织。当前 CLI 不提供内置 `--all-orgs`。

## 实现对应关系

- 多槽 profile 元数据与 current/primary/previous 指针：`internal/auth/profiles.go`
- token 按组织独立存储，legacy `auth-token` 镜像兼容旧逻辑：`internal/auth/token.go`
- 顶层 `dws profile list/use`：`internal/app/profile_command.go`
- 全局 `--profile` 预解析与运行时注入：`internal/app/root.go`
- PRD 与验收口径：`prd.json`

## 关键边界

- P0 不自动发现用户属于的所有组织，只列出用户主动登录过的 profile。
- P0 不扩展 real/embedded hook 协议；hook 后端显式 profile 选择仍会返回“不支持”。
- 主 profile 不通过普通 logout 误删；需要全量清理时走 `dws auth reset`。
- profile 元数据不保存 access token、refresh token、persistent code 或 client secret。

## 验收状态

聚焦多组织能力的单元测试已通过：

```bash
go test ./internal/auth ./internal/app -run 'Test(MultiProfile|RuntimeProfile|DeleteProfile|UpsertProfile|LoadProfiles|LegacyKeychain|WriteProfile|ProfileList|ProfileUse|AuthStatus|AuthLogout|AuthLogin|ResolveAuthLogin|EnrichAuthLogin|RootHelp|RootShortHelp|RootCommand)'
```

全量 `go test ./internal/auth ./internal/app` 中 `internal/auth` 通过，但 `internal/app` 被升级模块用例 `TestValidateNewBinary_RecoversFromUnsignedDarwin` 阻塞，错误是测试二进制执行时被 macOS kill。该失败发生在 upgrade 验签/回滚路径，不在多组织登录代码改动面内，需作为独立 CI/本机签名环境问题跟进。
