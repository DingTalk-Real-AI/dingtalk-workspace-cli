# dws 多组织登录完整测试用例

更新时间：2026-06-29

## 1. 测试目标

验证 dws 多组织登录新增能力在本地隔离环境下完整可用：

- 多个组织 profile 可独立保存、刷新、切换、删除和迁移。
- 当前组织、主组织、上一个组织指针行为符合 PRD F1-F8。
- 业务命令可通过全局 `--profile` 进行单次组织覆盖，也可通过 `--profile corpA,corpB` 或 `--profile corpA, corpB` 一次性读取多个组织，且不修改持久 current profile。
- 所有涉及 TUI / 交互选择的关键命令，都存在可由 Agent 或脚本执行的机器指令路径。
- `profiles.json` 只存非敏感元数据，不落 access token、refresh token、persistent code、client secret。

## 2. 自动化入口

主测试脚本：

```bash
bash scripts/dev/test-multi-profile-e2e.sh
```

调试模式：

```bash
bash scripts/dev/test-multi-profile-e2e.sh --skip-go-tests --verbose
bash scripts/dev/test-multi-profile-e2e.sh --keep-workdir
```

脚本行为：

- 使用临时 `DWS_CONFIG_DIR`、`DWS_KEYCHAIN_DIR`、`DWS_CACHE_DIR`。
- 设置 `DWS_DISABLE_KEYCHAIN=1`，避免写入真实系统 Keychain。
- 构建临时 `dws` 二进制。
- 通过生产 auth 存储 API seed 登录后的 token 结果，不依赖真实扫码。
- 使用真实 CLI 命令验证 profile/auth 命令面和状态变化。

## 2.1 多 profile 参数设计规范

核心规范参照 `lark-cli`：profile 仍是一个全局 string flag，多值由同一个 flag 值承载 CSV 列表。

- 推荐写法：`dws --profile corpA,corpB contact user get-self --format json`。
- 容错写法：`dws --profile corpA, corpB contact user get-self --format json`，CLI 在 Cobra 解析前规整为 `corpA,corpB`。
- 兼容目标：保持 `lark-cli` 一类 CSV 多值参数的简单心智模型，同时吸收钉钉历史 CLI 对逗号列表的支持方式。
- 非目标：不把 `--profile corpA --profile corpB` 作为主推荐 API；重复 flag 若后续支持，只能作为兼容增强，不改变 CSV 为主的规范。

## 3. 覆盖矩阵

| PRD 功能 | 覆盖方式 | 用例 |
|---|---|---|
| F1 多组织登录写入 profile | seed token + CLI list/status/token assert | TC-03, TC-04, TC-05 |
| F2 profile 元数据列表 | `dws profile list --format json/table` | TC-02, TC-03, TC-04 |
| F3 切换当前组织 | `profile switch/use <corpId>`、`profile switch -` | TC-07, TC-08 |
| F4 单次命令临时指定组织 | `dws --profile ... auth status`、`auth status --profile ...` | TC-10 |
| F5 按 profile 查看认证状态 | `auth status --format json` | TC-03, TC-10 |
| F6 退出与重置 | `auth reset`；`auth logout` 由 Go 回归覆盖 | TC-12, GT-05 |
| F7 legacy 单槽迁移 | seed legacy `auth-token` 后触发 `profile list` | TC-13 |
| F8 agent 跨组织聚合原语 | `--profile corpA,corpB` / `--profile corpA, corpB` 聚合读取 + `profile list` 枚举 | TC-11, MTC-01 |
| TUI 机器替代路径 | help surface + 非交互失败断言 | MTC-01 至 MTC-07 |

## 4. 测试数据

自动化脚本使用以下虚拟组织：

| 组织 | corpId | corpName | userId | access token |
|---|---|---|---|---|
| Alpha | `corp_alpha` | `Alpha Org` | `user_alpha` | `access-alpha-v1` |
| Beta | `corp_beta` | `Beta Org` | `user_beta` | `access-beta-v1/v2` |
| Gamma | `corp_gamma` | `Beta Org` | `user_gamma` | `access-gamma-v1` |
| Legacy | `corp_legacy` | `Legacy Org` | `user_legacy` | `access-legacy-v1` |

Gamma 故意与 Beta 使用相同 `corpName`，用于验证重复组织名的稳定 fallback name。

## 5. 自动化黑盒用例

### TC-01 命令面与机器指令入口

目的：确认关键交互能力都有非 TUI / Agent 可执行入口。

步骤：

```bash
dws --help
dws profile --help
dws auth login --help
dws skill setup --help
dws upgrade --help
dws dev connect --help
dws doc delete --help
dws aitable base delete --help
dws auth --help
```

断言：

- 根命令展示 `--profile`、`--yes`、`--dry-run`。
- `profile` 展示 `list`、`switch`、`use`、全局 `--profile`。
- `auth login` 展示 `--device`、`--token`、`--recommend`、`--yes`。
- `skill setup` 展示 `--mode`、`--target`、`--yes`、`--skill`、`--exclude`。
- `upgrade` 展示 `--dry-run`、`--yes`。
- `dev connect` 展示 `--robot-client-id`、`--robot-client-secret`、`--unified-app-id`、`--agent-cmd`、`--daemon`。
- 删除类命令展示 `--yes`。
- `auth` 命令组不暴露 `switch`。

### TC-02 空 profile 列表

步骤：

```bash
dws profile list --format json
```

断言：

- `success=true`。
- `profiles=[]`。
- `primaryProfile/currentProfile/previousProfile` 为空。

### TC-03 首次组织登录后创建 primary/current profile

前置：

- 通过 helper seed `corp_alpha` token。

步骤：

```bash
dws profile list --format json
dws auth status --format json
```

断言：

- `profiles` 数量为 1。
- `primaryProfile=corp_alpha`。
- `currentProfile=corp_alpha`。
- `previousProfile` 为空。
- `auth status` 返回 Alpha 的 `corpId/corpName/userId`。
- 默认 token mirror 指向 `corp_alpha`。
- corp-scoped token `auth-token:corp_alpha` 存在。
- `profiles.json` 不包含敏感字段。

### TC-04 第二组织登录不覆盖第一组织

前置：

- 已存在 `corp_alpha`。
- seed `corp_beta` token。

步骤：

```bash
dws profile list --format json
```

断言：

- `profiles` 数量为 2。
- `primaryProfile=corp_alpha`。
- `currentProfile=corp_beta`。
- `previousProfile=corp_alpha`。
- Alpha token 仍为 `access-alpha-v1`。
- Beta token 为 `access-beta-v1`。
- legacy mirror 指向 `corp_beta`。

### TC-05 同组织重复登录只刷新 token，不新增 profile

前置：

- 已存在 `corp_beta`。
- 再次 seed `corp_beta`，access token 改为 `access-beta-v2`。

步骤：

```bash
dws profile list --format json
```

断言：

- `profiles` 数量仍为 2。
- `currentProfile=corp_beta`。
- `previousProfile=corp_alpha`。
- `auth-token:corp_beta` 的 access token 更新为 `access-beta-v2`。

### TC-06 重复组织名生成稳定 fallback name

前置：

- 已存在 `corp_beta`，`corpName=Beta Org`。
- seed `corp_gamma`，`corpName=Beta Org`。

步骤：

```bash
dws profile list --format json
```

断言：

- `profiles` 数量为 3。
- `currentProfile=corp_gamma`。
- `previousProfile=corp_beta`。
- `corp_gamma` 的 `corpName=Beta Org`。
- `corp_gamma` 的本地 profile name 不等于裸 `Beta Org`，而是带 corpId 后缀的稳定 fallback。
- JSON 输出不暴露本地 `name` 字段。

### TC-07 按 corpId 切换组织并同步 legacy mirror

步骤：

```bash
dws profile switch corp_alpha --format json
dws profile switch corp_beta --format table
```

断言：

- 第一次切换后 `currentProfile=corp_alpha`。
- 第一次切换后 `previousProfile=corp_gamma`。
- JSON 输出包含 Alpha 的 `corpId/corpName`，且 `isCurrent=true`。
- legacy mirror 指向 `corp_alpha`。
- 第二次切换后 `currentProfile=corp_beta`。
- 第二次切换后 `previousProfile=corp_alpha`。
- 表格输出包含 `Beta Org` 和 `corp_beta`。
- legacy mirror 指向 `corp_beta`。

### TC-08 使用 previousProfile 快速切回

步骤：

```bash
dws profile switch - --format json
```

断言：

- 当前组织从 `corp_beta` 切回 `corp_alpha`。
- `previousProfile=corp_beta`。
- JSON 输出包含 Alpha，并标记为 current。

### TC-09 `profile use` 兼容 switch 语义

步骤：

```bash
dws profile use corp_gamma --format json
```

断言：

- `currentProfile=corp_gamma`。
- `previousProfile=corp_alpha`。
- JSON 输出包含 Gamma。
- legacy mirror 指向 `corp_gamma`。

### TC-10 单次 profile override 不修改 currentProfile

步骤：

```bash
dws --profile corp_alpha auth status --format json
dws auth status --profile corp_beta --format json
dws auth status --format json
```

断言：

- 第一条返回 Alpha。
- 第二条返回 Beta。
- 两条 override 后 `currentProfile` 仍为 `corp_gamma`。
- 第三条默认返回 Gamma。
- `previousProfile` 不因 override 改变。

### TC-11 多 profile 一次性读取信息

步骤：

```bash
dws --mock --profile corp_alpha, corp_beta contact user get-self --format json
dws --mock contact user get-self --profile corp_alpha, corp_beta --format json
```

断言：

- 输出为聚合对象，`multiProfile=true`。
- `success=true`。
- `summary.total=2`、`summary.succeeded=2`、`summary.failed=0`。
- `profiles[0].corpId=corp_alpha`，`profiles[1].corpId=corp_beta`。
- 每个 `profiles[i].ok=true`，且每个组织都有独立 `result`。
- 执行后 `currentProfile` 仍为 `corp_gamma`，`previousProfile` 仍为 `corp_alpha`。
- `--profile` 放在根命令后或 leaf 命令后都可解析。
- `--profile corp_alpha,corp_alpha,corp_beta` 按 resolved `corpId` 去重后只执行 Alpha/Beta 两个组织。
- 若存在历史 profile name 本身为 `alpha,beta`，优先按单 profile 精确匹配，不触发聚合，保持向前兼容。

### TC-12 `auth reset` 清除所有本地认证态

前置：

- 保存测试 app config。

步骤：

```bash
dws auth reset
```

断言：

- 输出包含 `[OK]`。
- `profiles.json` 清空或不存在。
- 所有 profile-scoped token 删除。
- legacy `auth-token` 删除。
- `token.json` marker 删除。
- app config 删除。

### TC-13 legacy 单槽自动迁移

前置：

- 清空 profiles。
- 只写入 legacy `auth-token`，token 中包含 `corp_legacy`。

步骤：

```bash
dws profile list --format json
```

断言：

- 自动生成 profile。
- `primaryProfile=corp_legacy`。
- `currentProfile=corp_legacy`。
- `previousProfile` 为空。
- corp-scoped `auth-token:corp_legacy` 存在。
- legacy mirror 仍可读取。

## 6. Go 回归用例组

脚本默认先执行：

```bash
go test -timeout 180s -count=1 ./internal/auth ./internal/app ./test/cli -run 'Test(MultiProfile|RuntimeProfile|ProfileFlagArgs|PreparseProfileFlag|NormalizeProcessProfileArgs|CommaSeparated|CommaNamed|DeleteProfile|UpsertProfile|LoadProfiles|LegacyKeychain|WriteProfile|ProfileList|ProfileUse|ProfileSwitch|AuthCommandDoesNotExposeSwitch|AuthStatus|AuthLogout|AuthLogin|ResolveAuthLogin|EnrichAuthLogin|RootHelp|RootShortHelp|RootCommand|ProductCommandsAcceptGlobalProfileFlag)'
```

### GT-01 auth/profile 存储单元回归

覆盖：

- `SaveTokenData` 写入 corp-scoped slot。
- `UpsertProfileFromToken` 新增/刷新 profile。
- `ResolveProfile` 按 corpId/name/corpName 查找。
- `SetCurrentProfile` 和 `UsePreviousProfile` 指针更新。
- `DeleteTokenDataForProfile` 单 profile 删除。
- legacy token migration。

### GT-02 profile 命令回归

覆盖：

- `profile list --format json` 包含 `corpName`。
- `profile switch/use` 输出组织名和 corpId。
- `profile switch -` toggle。
- 无参数交互路径调用 selector。
- 非交互无参数返回 validation error。
- 冲突 selector 返回 validation error。

### GT-03 auth 命令回归

覆盖：

- `auth status` 默认使用 current profile。
- `auth status --profile` 不修改 current profile。
- `auth logout` 默认删除全部 profile 且保留 app config。
- `auth logout --profile` 只删除指定 profile。
- `auth reset` 删除 token、profiles、marker、app config。
- `auth login` 默认强制进入授权流程。
- `auth login --profile` 可解析目标 corpId。
- 登录后可从 contact profile 补充 corpName/userName。

### GT-04 全局 `--profile` 业务命令注入

覆盖：

- 每个产品命令接受全局 `--profile`。
- `--profile` 不泄漏为业务参数。
- runtime profile 在调用 runner 前已设置。
- `--profile corpA,corpB` 进入多组织聚合读取。
- `--profile corpA, corpB` 在 Cobra 解析前规整为同一个 profile selector，避免 `corpB` 被误判为 command/arg。
- 聚合读取按 resolved `corpId` 去重，并在执行后恢复原始 runtime profile。
- 含逗号的历史 profile name 仍按单 profile 解析。

### GT-05 命令可见性和兼容性

覆盖：

- root help 展示 profile。
- root help 展示全局 `--profile`。
- `auth switch` 不暴露。
- 旧命令和 docs compatibility 不退化。

## 7. TUI / 交互入口机器替代审计用例

### MTC-01 profile TUI

交互入口：

```bash
dws profile switch
dws profile use
```

机器指令：

```bash
dws profile switch <name|corpId>
dws profile switch --corpId <corpId>
dws profile switch --name <corpName>
dws profile switch -
dws --profile <corpId> <product> <command>
dws --profile <corpIdA>,<corpIdB> <product> <command>
dws --profile <corpIdA>, <corpIdB> <product> <command>
```

预期：

- 交互终端可展示 TUI。
- 非交互环境无 selector 时失败并提示传入 selector。
- 机器指令可完成同等切换、单次覆盖或多组织聚合读取能力。

### MTC-02 auth login 交互授权

交互入口：

```bash
dws auth login
dws auth login --recommend
```

机器指令：

```bash
dws auth login --device --format json
dws auth login --token <accessToken> --format json
dws auth login --recommend --yes --format json
dws auth login --profile <corpId> --format json
```

预期：

- 无头环境可走 device flow。
- Agent 可用 `--recommend --yes` 跳过登录后推荐授权 TUI。
- 目标组织可由全局 `--profile` 指定。
- OAuth 浏览器扫码本身属于授权链路，不视为 CLI TUI 强依赖。

### MTC-03 skill setup 模式选择和确认

交互入口：

```bash
dws skill setup
```

机器指令：

```bash
dws skill setup --mode mono --yes
dws skill setup --mode multi --target claude --yes
dws skill setup --mode multi --skill aitable --skill calendar --yes
dws skill setup --mode multi --exclude live --yes
```

预期：

- 非交互未指定 mode 时默认 mono。
- 指定 `--mode` + `--yes` 可完全绕过 TUI。
- multi 子 skill 可通过 `--skill/--exclude` 明确选择。

### MTC-04 upgrade 确认

交互入口：

```bash
dws upgrade
dws upgrade --rollback
```

机器指令：

```bash
dws upgrade --dry-run
dws upgrade --yes
dws upgrade --rollback --yes
dws upgrade --check --format json
dws upgrade --list --format json
```

预期：

- Agent 可先 `--dry-run` 获取计划。
- 用户确认后追加 `--yes` 执行。
- 查询类命令可用 JSON 输出。

### MTC-05 dev connect 建联引导

交互入口：

```bash
dws dev connect
```

机器指令：

```bash
dws dev connect --channel <channel> --robot-client-id <id> --robot-client-secret <secret>
dws dev connect --channel <channel> --unified-app-id <appId>
dws dev connect --channel custom --agent-cmd "<cmd>" --robot-client-id <id> --robot-client-secret <secret>
dws dev connect --daemon --channel <channel> --robot-client-id <id> --robot-client-secret <secret>
```

预期：

- 非交互缺凭证时 fail-fast，不阻塞等待输入。
- 现成凭证和 unified app id 均可绕过建联引导。
- 自研 agent 可用 `--agent-cmd`。

### MTC-06 删除/敏感操作确认

交互入口：

```bash
dws doc delete ...
dws drive delete ...
dws aitable base delete ...
dws todo task delete ...
```

机器指令：

```bash
dws <delete command> --dry-run
dws <delete command> --yes
```

预期：

- 默认需要确认。
- `--dry-run` 可预览。
- 用户确认后 `--yes` 可由 Agent 执行。

### MTC-07 PAT 批量授权

交互入口：

```bash
dws pat chmod --products ...
dws pat chmod --recommend ...
```

机器指令：

```bash
dws pat chmod ... --dry-run --format json
dws pat chmod ... --yes --format json
```

预期：

- 批量授权未加 `--yes` 时阻断。
- Agent 先展示 dry-run plan，用户明确确认后再追加 `--yes`。

## 8. 已知残余风险

| 风险 | 说明 | 建议 |
|---|---|---|
| 部分 legacy compat delete path 仍会读 stdin 确认 | `internal/compat/registry.go` 中 `_blocked` 后仍存在 `Confirm? (yes/no)` 交互路径 | 后续可改成非交互环境直接 validation，并提示 `--yes` |
| 真实 OAuth 登录未在黑盒脚本中扫码验证 | 自动脚本 seed 登录后 token，避免人工和网络依赖 | 发布前可追加一次手工 UAT：真实 `auth login` 登录 A/B 两个组织 |
| 远端 revoke/logout 不在黑盒脚本中直连验证 | 网络不稳定且可能影响真实环境 | 已由 Go 回归用 mock/隔离环境覆盖本地删除语义；真实环境只做冒烟 |
| TUI 视觉细节不在脚本中截图校验 | 脚本关注机器链路和非交互替代路径 | TUI 视觉可用人工验收或独立截图测试 |

## 9. 手工 UAT 建议

在真实账号拥有两个钉钉组织的环境中执行：

```bash
dws auth login --format json
dws profile list --format json
dws auth login --format json
dws profile list --format json
dws profile switch <primaryCorpId>
dws auth status
dws --profile <secondaryCorpId> contact user get-self --format json
dws --profile <primaryCorpId>,<secondaryCorpId> contact user get-self --format json
dws profile switch -
dws auth logout --profile <secondaryCorpId>
dws profile list --format json
dws auth logout
```

验收重点：

- 第二次 `auth login` 能选择另一个组织，不因当前 token 有效而跳过授权。
- `profile list` 表格和 JSON 均展示组织名。
- `--profile` 取数返回对应组织身份。
- `--profile A,B` 返回聚合结果，且不改变 current profile。
- 单 profile logout 不影响另一个组织。
- 默认 logout 清空全部组织登录态。

## 10. 通过标准

必须同时满足：

- `bash scripts/dev/test-multi-profile-e2e.sh` 通过。
- `profiles.json` 无敏感字段。
- 所有 P0 功能 F1-F7 至少有一个自动化用例覆盖。
- 关键 TUI/交互入口均有机器指令替代路径。
- 手工 UAT 未发现真实 OAuth 组织选择和权限链路阻断。
