# 指令裁剪策略（Command Restriction Policy）

企业把 dws 嵌入自研 Agent 时，可以把一组规则编译进专属二进制，限制 Agent 能执行的命令。规则通过 `pkg/edition.Hooks.Restrict` 提供。

## 定位与边界

- **它是给 Agent 的护栏。** 拦住误操作和越权尝试：规则在二进制里，Agent 和使用者在这个二进制内无法修改或绕过。
- **它不防二进制被替换。** 使用者换成标准版 dws 即可绕过。防人需要凭证不落本地（`TokenProvider`、`SaveToken`/`LoadToken` 等钩子）加服务端权限控制。
- **它管不到命令内部。** 通用调用类命令（见「通道入口」）的真实操作由参数决定，必须单独点名才能开放。
- **受限版不加载使用者控制的扩展。** 策略启用后，运行时插件、插件 Hook、用户自定义 shortcut 都不加载。企业自己的扩展通过 `Hooks.RegisterExtraCommands` 编译进二进制，正常参与裁剪。

策略同时管控**执行**和**发现**：被拒绝的命令无法执行，也不出现在 `dws schema`、帮助和 Shell 补全里（见「发现面」）。内置 skill 文档不随策略裁剪，仍可能提到被拒绝的命令，调用时得到 `command_denied`。

## 接入

```go
edition.Override(&edition.Hooks{
    Name: "acme",
    Restrict: func() []edition.CommandRule {
        return []edition.CommandRule{
            {Name: "calendar-readonly", Allow: []string{"calendar/**"}, MaxEffect: edition.EffectRead},
            {Name: "auth-basic", Allow: []string{"auth/login", "auth/logout", "auth/status"}, AllowUnannotated: true},
        }
    },
})
```

`Restrict` 每棵可执行命令树调用一次，必须返回静态规则，不要在里面做网络请求或其他阻塞操作。

用 `pkg/editiontest.RunContractTests` 在构建阶段校验规则；它会对「放行全部」和「开放了通道入口」的规则输出提示。

## 策略状态

| `Restrict` | 状态 | 行为 |
|---|---|---|
| 为 nil | `disabled` | 与标准版完全一致 |
| 返回空列表 | `active` | 业务命令全部拒绝 |
| 规则非法，或调用时 panic | `invalid` | 只保留 `config policy show`、`help`、`version` |

规则出错不会退化成「未启用」。

## 规则字段

| 字段 | 含义 |
|---|---|
| `Name` | 必填，规则集内唯一。出现在诊断输出和拒绝错误里 |
| `Description` | 仅展示 |
| `DenyMessage` | 可选。本条规则拒绝命令时附加到拒绝错误的指引，见[自定义拒绝指引](#自定义拒绝指引) |
| `Allow` | glob 列表。为空表示不限制路径 |
| `Deny` | glob 列表。仅在本条规则内优先于 `Allow` |
| `MaxEffect` | effect 上限（含）：`read` < `write` < `destructive`。为空不设上限 |
| `MaxRisk` | risk 上限（含）：`low` < `medium` < `high`。为空不设上限 |
| `AllowUnannotated` | 放行缺少 effect 或 risk 声明的命令。不放行非法值 |
| `AllowChannels` | 精确路径列表，点名开放通道入口 |

### 路径

- 用 `/` 分隔，不含 `dws` 根名，全小写：`dws calendar event list` 写作 `calendar/event/list`。
- `Allow`、`Deny` 是 doublestar glob：`*` 匹配一段，`**` 跨多段。例：`calendar/**`、`**/delete`。
- 不允许空段、首尾 `/`、空白字符。`AllowChannels` 必须是精确路径，不能用通配。

### 组合语义

1. **单条规则内是 AND。** `Deny` 未命中、`Allow` 命中（非空时）、声明完整（或开了 `AllowUnannotated`）、不超过两个上限，全部满足这条规则才算通过。
2. **多条规则之间是 OR。** 任一条通过即放行。
3. **`Deny` 只在本条规则内有效**，不能否决另一条规则的 `Allow`。要全局禁止一条命令，必须让每条规则都拒绝它或都不放行它。

第 3 点容易出错。下面的规则集里 `calendar event delete` 是**放行**的，因为规则 `all` 没有拒绝它：

```go
{Name: "no-delete", Allow: []string{"calendar/**"}, Deny: []string{"calendar/event/delete"}},
{Name: "all",       Allow: []string{"calendar/**"}},
```

`dws config policy show` 会把这种情况列为 `deny_overridden` 提示。

### effect 与 risk

命令的 effect 和 risk 取自编译进二进制的安全声明（`ContractFinal.Safety`），两个维度各自独立判断，互不推导。

- 缺少任一维度的命令视为未标注，默认拒绝；规则开了 `AllowUnannotated` 才放行，且缺失的维度跳过上限检查，不当作最低等级。
- 声明了非法值（如拼写错误）的命令一律拒绝，`AllowUnannotated` 也不能放行。
- 用户自定义 shortcut 自己填写的风险等级不是可信声明，永不参与判断。

### 别名与兼容命令

同一个操作的多个入口共用一个判决。入口包括：

- 独立挂载的兼容命令，例如 `report create` 是 `report entry submit` 的兼容入口；
- 任意层级的原生命令别名，例如 `report` 分组的别名 `log` 使 `log entry submit` 也是同一个入口。

匹配规则：

- `Allow` 按主路径（`report/entry/submit`）匹配。规则请以主路径书写；放行主路径后，各别名写法都可用。
- `Deny` 命中任一入口写法（如 `log/**`、`report/create`），该规则对所有入口都不通过。

### 混合分组

少数命令既能自己执行业务、又挂有子命令（如 `dingtalk-tag connect`）。它自身的操作和子命令分别判断：放行子命令不等于放行它自己，拒绝它自己也不影响被放行的子命令。

## 默认可用的命令

策略为 `active` 时，以下只读发现与诊断命令不需要规则放行：`schema`、`help`、`version`、`config policy show`。

其余命令都要规则放行，包括 `auth`。

### auth

`auth` 下多数命令没有安全声明，需要一条开了 `AllowUnannotated` 的规则精确放行：

```go
{Name: "auth-basic", Allow: []string{"auth/login", "auth/logout", "auth/status"}, AllowUnannotated: true}
```

- 不要向 Agent 开放 `auth/export`、`auth/import`、`auth/reset`。
- 凭证由企业托管时不要放行 `auth/login`。`Hooks.HideAuthLogin` 只是隐藏，不阻止执行。

## 通道入口

通道入口是「真实操作由参数决定」或「会改变 CLI 运行的代码」的命令。它们自身的安全声明描述的是命令本身，不是它打开的通道，所以：

- 没有被点名时，`Allow` 的 glob（包括 `**`）、上限、`AllowUnannotated` 都不能开放它们；
- 必须在某条规则的 `AllowChannels` 里按精确路径点名；
- 点名是这条规则的**附加条件**，不替代其他条件：被点名的通道入口仍要通过同一条规则的 `Deny`、`Allow`、声明完整性和上限检查。

例如下面的规则**不会**开放 `api`：规则限定在 `calendar/**`，`api` 不在其中，而且 `api` 没有安全声明、规则也没开 `AllowUnannotated`。

```go
{Name: "calendar", Allow: []string{"calendar/**"}, MaxEffect: edition.EffectRead, AllowChannels: []string{"api"}}
```

要开放 `api`，需要一条专门的规则：

```go
{Name: "raw-api", Allow: []string{"api"}, AllowChannels: []string{"api"}, AllowUnannotated: true}
```

注意 `api` 没有 effect 声明，`MaxEffect` 对它不起作用：开放后它可以发起任意方法的 OpenAPI 调用。

| 路径 | 说明 |
|---|---|
| `api` | 调用任意 OpenAPI |
| `mcp/published/invoke` | 调用参数指定的远端 MCP 工具 |
| `mcp/url/get` | 返回可在 CLI 之外使用的 MCP 服务地址 |
| `upgrade` | 替换当前二进制，可能丢掉内置策略 |
| `plugin/*`、`plugin/config/*` | 插件管理（清单见 `internal/cmdpolicy/reviewed_paths.go`） |

开放一个通道入口意味着规则无法再约束经由它发出的操作，请只在明确需要时使用。

## 发现面

执行、诊断、Schema、帮助读取同一份策略快照。

**Schema（`dws schema`）**

- 单路径查询（canonical 路径、主 CLI 路径、别名、`--cli-path`）命中被拒绝的命令时返回 `command_denied`，不返回「未找到」。
- `schema`（概览）、产品、分组、`--all` 只列出放行的工具；`tool_count` 和 `count` 按放行结果重算，没有放行工具的产品和分组不出现。查询一个没有任何放行工具的产品或分组返回 `command_denied`（`all_children_denied`）。
- 策略只改变展示，不改变 Schema 的组装和磁盘缓存；`catalog_hash`、`surface_hash`、`agent_metadata` 仍描述基础目录。
- 策略为 `active` 时，带 `catalog_hash` 的输出额外带 `policy_view_hash`，标识当前规则集下的可见面。规则变化时 `catalog_hash` 不变而 `policy_view_hash` 变化，缓存 Schema 的消费方应同时比较两者。策略未启用时不输出该字段，输出字节与标准版一致。

**帮助与补全**

- `dws help <路径>` 对被拒绝的命令、以及没有任何放行命令的分组返回 `command_denied`，不输出用法和参数。
- `dws <分组> --help` 在该分组没有任何放行命令时只输出拒绝说明。
- 帮助列表、Shell 补全、拼写纠错建议都不包含被拒绝的命令；补全也不提供被拒绝命令的参数名。
- 混合分组自身被拒绝但仍有放行的子命令时，分组保持可见，以便发现这些子命令；它自身业务操作的参数不出现在帮助和补全里。

## 规则模板

**只读 Agent，限定产品**

```go
[]edition.CommandRule{
    {Name: "readonly", Allow: []string{"calendar/**", "contact/**"}, MaxEffect: edition.EffectRead},
    {Name: "auth-basic", Allow: []string{"auth/login", "auth/logout", "auth/status"}, AllowUnannotated: true},
}
```

**指定产品可读写，禁止破坏性操作**

```go
[]edition.CommandRule{
    {Name: "todo-rw", Allow: []string{"todo/**"}, MaxEffect: edition.EffectWrite, MaxRisk: edition.RiskMedium},
    {Name: "auth-basic", Allow: []string{"auth/login", "auth/logout", "auth/status"}, AllowUnannotated: true},
}
```

## 诊断

```bash
dws config policy show --format json
```

返回 `state`、`source`、`invalid_reason`、`rules`、`denied_count`、`denied_paths`、`channels`、`extensions_disabled`、`hints`。返回结构作为 Result 契约发布在 Schema 中（`dws schema config.policy_show --compact`）。

该命令只读本机状态，在 `active` 和 `invalid` 状态下都可用，并且不经过 `Hooks.AfterPersistentPreRun`：企业在该钩子里做的登录校验不会挡住策略诊断。

提示（`hints`）不阻断执行：

| kind | 含义 |
|---|---|
| `deny_overridden` | 某命令被一条规则的 `Deny` 命中，却被另一条规则放行 |
| `rule_allows_all` | 某条规则既无 `Allow` 也无上限，放行全部已声明的非通道命令 |

## 拒绝错误

被拒绝的命令返回 validation 类错误，退出码 3，`reason` 为 `command_denied`，`details.reason_code` 给出具体原因：

| reason_code | 含义 |
|---|---|
| `domain_not_allowed` | 路径不在规则的 `Allow` 里 |
| `command_denylisted` | 命中规则的 `Deny` |
| `risk_too_high` | effect 或 risk 超过上限 |
| `risk_not_annotated` | 缺少 effect 或 risk 声明，且规则未开 `AllowUnannotated` |
| `risk_invalid` | 声明了非法的 effect 或 risk 值 |
| `channel_not_allowed` | 通道入口未被任何规则点名 |
| `no_matching_rule` | 多条规则都不放行，或规则集为空 |
| `identity_conflict` | 命令归属无法可靠判断 |
| `all_children_denied` | 分组（或 Schema 的产品、分组）下没有任何放行的命令 |
| `policy_invalid` | 策略非法 |

被拒绝的命令不会触发鉴权、不发起请求。Agent 不应重试。

### 自定义拒绝指引

规则的 `DenyMessage` 用来告诉使用者下一步怎么办，例如找谁开通：

```go
{Name: "deny-calendar-agenda", Deny: []string{"calendar/+agenda"}, DenyMessage: "日程查询未开放，请联系 IT 开通"}
```

- 它追加在拒绝错误 `hint` 的末尾，同时写入 `details.deny_message`。`message`、`reason`、`details.reason_code` 和退出码不变，Agent 仍应按这些稳定字段判断。
- 只有一条规则拒绝该命令时，用这条规则的 `DenyMessage`。
- 多条规则都拒绝时（`no_matching_rule`），只用 `Deny` 命中该命令的规则的 `DenyMessage`，按规则顺序拼接并去重。仅仅是 `Allow` 没覆盖或上限不满足的规则不参与，所以 `auth-basic` 这类模板规则不会给无关命令附加文案。
- 通道入口未被点名（`channel_not_allowed`）、策略非法（`policy_invalid`）等与具体规则无关的拒绝不带自定义指引。
- 内容须为单行纯文本，不超过 500 个字符，不含换行等控制字符；不满足时规则非法，策略进入 `invalid`。

## 与 lark-cli 的差异

规则语义（AND/OR、局部 `Deny`、空 `Allow`、默认拒绝未标注）与 lark-cli 的 `platform.Rule` 一致。差异如下：

| 差异 | dws | 原因 |
|---|---|---|
| 风险维度 | `MaxEffect` 和 `MaxRisk` 两个上限 | dws 的安全声明是两个字段 |
| 身份维度 | 无 | dws 命令没有 user/bot 标注 |
| 规则来源 | 只有编译进去的 hook，没有 `policy.yml` | 使用者可改的文件不能作企业管控 |
| hook 返回空列表 | 全部拒绝 | 避免取规则出错时管控静默消失 |
| 默认可用的命令 | `schema`、`help`、`version`、`config policy show` | 这几条没有安全声明，不豁免则无法发现和诊断 |
| 通道入口 | 单独分类，须精确点名 | 命令的风险等级不等于通道权限 |
| 运行时插件 | 受限版不加载 | dws 的插件由使用者安装 |
| 别名、混合分组 | 身份归并、分组自身单独判断 | dws 命令树特有的结构 |

## 维护

- 新增通用调用类、插件管理类、自更新类命令时，必须在同一改动里登记到 `internal/cmdpolicy/reviewed_paths.go` 的通道清单。
- `internal/cmdpolicy` 不得 import `internal/cli`。
- 策略快照按命令树保存（以 root 为键），不使用进程全局状态；声明树（Schema source root）不携带策略。
