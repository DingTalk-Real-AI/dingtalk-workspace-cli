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

GitHub Actions 入口：

```text
.github/workflows/multi-profile-e2e.yml
```

触发方式：

- PR 自动触发。
- 任意分支 push 自动触发。
- Actions 页面手动执行 `workflow_dispatch`。

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

## 3.1 `--profile` 命令级覆盖矩阵

本次 `--profile` 变更影响的是 root persistent flag、runtime runner、auth/profile 管理命令和所有会读取登录态的命令。测试按以下语义分层：

| 语义 | 说明 | 断言重点 |
|---|---|---|
| P-SINGLE | 单 profile 临时覆盖 | 使用指定组织 token / runtime profile；不修改 `currentProfile`、`previousProfile` |
| P-MULTI | CSV 多 profile 聚合 | 输出 `multiProfile=true`，按输入顺序返回每个组织结果；不修改持久 profile 指针 |
| P-LOCAL | 命令自己的 `--profile` | local flag 优先于 root persistent flag；只影响该命令定义的局部行为 |
| P-IGNORED | 接受 root `--profile` 但命令本身不读组织态 | 命令成功；输出与未传 profile 一致；不修改 profile 指针 |
| P-UNSUPPORTED | 多 profile 对该命令无业务语义，应明确失败或不扩大影响 | 不静默误删、不默认切换、不把第二个 profile 当业务参数 |

### 3.1.1 Auth / Profile 管理命令

| 指令 | 必测 `--profile` 场景 | 预期 |
|---|---|---|
| `dws auth login` | `dws --profile corp_alpha auth login --device --format json`、`dws auth login --profile corp_alpha` help 示例 | 指定本次授权目标组织；不持久切换 current；缺失 profile 返回 validation |
| `dws auth status` | `dws --profile corp_alpha auth status --format json`、`dws auth status --profile corp_beta --format json`、root + local 同时存在 | local `--profile` 优先；root `--profile` 可选中 token；均不修改 current |
| `dws auth logout` | `dws auth logout --profile corp_alpha`、`dws --profile corp_alpha auth logout`、`dws auth logout` | local `--profile` 只退出单组织；root `--profile` 不应被误认为单组织 logout；默认仍退出全部 |
| `dws auth reset` | `dws --profile corp_alpha auth reset`、`dws auth reset` | reset 是全局清理；root `--profile` 不改变清理范围 |
| `dws auth export` | `dws --profile corp_alpha auth export --base64` | 导出当前 runtime profile 对应 token/配置，或明确说明导出全量；不修改 current |
| `dws auth import` | `dws --profile corp_alpha auth import --input <bundle>` | import 的写入语义不被 root `--profile` 误导；导入后 profile 指针符合 bundle/实现契约 |
| `dws auth exchange` | `dws --profile corp_alpha auth exchange --code <code>` | 仅在真实 OAuth/UAT 中验证；目标 profile 解析不应泄漏到业务参数 |
| `dws profile list` | `dws --profile corp_alpha profile list --format json` | list 永远列出全部 profile；root `--profile` 不过滤、不切换 |
| `dws profile switch` | `dws --profile corp_alpha profile switch corp_beta --format json`、`dws profile switch --corpId corp_beta`、`dws profile switch --name "Beta Org"`、`dws profile switch -` | 显式 selector 决定持久切换；root `--profile` 不覆盖 switch 目标 |
| `dws profile use` | `dws --profile corp_alpha profile use corp_gamma --format json`、`dws profile use -` | 与 switch 等价；root `--profile` 不改变 use 目标 |

### 3.1.2 Runtime / 产品命令

所有 runtime 产品命令均必须覆盖 `P-SINGLE` 和 `P-MULTI`，但不能把 `--mock` 当作唯一测试方式。测试分三层：

- L1 自动化回归：允许使用 `--mock`，只验证 CLI 参数解析、runner 注入、聚合结构、无副作用和 `profile` 不泄漏为业务参数。
- L2 真实只读冒烟：不加 `--mock`，对每个产品 root 选择一个当前可见的只读 leaf，验证真实 token 读取、endpoint 解析、远端调用链路。业务可以因权限/数据为空返回结构化错误，但不能是 CLI 解析错误、profile 解析错误或错误组织 token。
- L3 高风险写操作：不加 `--mock` 时必须使用 `--dry-run`、测试租户或显式 `--yes` 确认；测试重点是 `--profile` 只选择组织，不替代确认、不扩大操作范围。

| 产品根命令 | L1 自动化回归（可 mock） | L2 真实只读冒烟（不可 mock） | 断言 |
|---|---|---|---|
| `aisearch` | `dws --mock --profile corp_alpha,corp_beta aisearch person --keyword Alice --format json` | `dws --profile corp_alpha aisearch person --keyword Alice --format json` | L1 聚合；L2 使用 Alpha token 调真实链路 |
| `aitable` | `dws --mock --profile corp_alpha,corp_beta aitable base list --format json` | `dws --profile corp_alpha aitable base list --format json` | 不泄漏 `profile` 业务参数；真实链路不因 profile 解析失败 |
| `attendance` | `dws --mock --profile corp_alpha,corp_beta attendance group list --format json` | `dws --profile corp_alpha attendance group list --format json` | 输出按组织聚合；真实链路使用 selected profile |
| `calendar` | `dws --mock --profile corp_alpha,corp_beta calendar event list --format json` | `dws --profile corp_alpha calendar event list --format json` | 单 profile 不改 current；真实只读日程链路可达 |
| `chat` | `dws --mock chat search --query test --profile corp_alpha, corp_beta --format json` | `dws --profile corp_alpha chat search --query test --format json` | leaf 后 `--profile` 解析；真实链路不误用 current |
| `conference` | `dws --mock --profile corp_alpha,corp_beta conference list --format json` | `dws --profile corp_alpha conference list --format json` | L1 不触网；L2 真实会议只读链路 |
| `contact` | `dws --mock --profile corp_alpha, corp_beta contact user get-self --format json` | `dws --profile corp_alpha contact user get-self --format json` | 覆盖不加引号 CSV continuation；真实返回当前用户身份 |
| `devdoc` | `dws --mock --profile corp_alpha,corp_beta devdoc article search --query auth --format json` | `dws --profile corp_alpha devdoc article search --query auth --format json` | 文档类命令也接受 global profile；真实搜索链路可达 |
| `ding` | `dws --mock --profile corp_alpha,corp_beta ding list --format json` | `dws --profile corp_alpha ding list --format json` | 无业务参数污染；真实只读链路可达或结构化权限错误 |
| `doc` | `dws --mock --profile corp_alpha,corp_beta doc search --query test --format json` | `dws --profile corp_alpha doc search --query test --format json` | doc helper/runtime 均覆盖；真实文档搜索链路 |
| `doc-comment` | `dws --mock --profile corp_alpha,corp_beta doc-comment list --node doc_x --format json` | `dws --profile corp_alpha doc-comment list --node <testNodeId> --format json` | serverOverride 子 server 覆盖；真实用测试文档节点 |
| `drive` | `dws --mock --profile corp_alpha,corp_beta drive file list --format json` | `dws --profile corp_alpha drive file list --format json` | 真实云盘只读链路；不修改 current |
| `hrmregister` | `dws --mock --profile corp_alpha,corp_beta hrmregister field list --format json` | `dws --profile corp_alpha hrmregister field list --format json` | 子 server 覆盖；真实权限错误也要结构化 |
| `live` | `dws --mock --profile corp_alpha,corp_beta live list --format json` | `dws --profile corp_alpha live list --format json` | 聚合结构一致；真实只读直播列表 |
| `mail` | `dws --mock --profile corp_alpha,corp_beta mail message list --format json` | `dws --profile corp_alpha mail message list --format json` | 不泄漏 profile 参数；真实邮箱权限链路 |
| `minutes` | `dws --mock --profile corp_alpha,corp_beta minutes list mine --format json` | `dws --profile corp_alpha minutes list mine --format json` | 多 profile 每组织独立 result；真实听记列表 |
| `oa` | `dws --mock --profile corp_alpha,corp_beta oa list-pending --format json` | `dws --profile corp_alpha oa list-pending --format json` | 真实审批只读列表；不修改 current |
| `pat` | `dws --mock --profile corp_alpha,corp_beta pat status --format json` | `dws --profile corp_alpha pat status --format json` | PAT runtime 命令和 `pat chmod` utility 分开测 |
| `report` | `dws --mock --profile corp_alpha,corp_beta report template list --format json` | `dws --profile corp_alpha report template list --format json` | 日志产品真实只读链路 |
| `sheet` | `dws --mock --profile corp_alpha,corp_beta sheet read --sheet-id sh_x --format json` | `dws --profile corp_alpha sheet read --sheet-id <testSheetId> --format json` | 参数存在时 profile 不混入 params；真实用测试表格 |
| `todo` | `dws --mock --profile corp_alpha,corp_beta todo task list --format json` | `dws --profile corp_alpha todo task list --format json` | 已有 helper merge 路径覆盖；真实待办只读链路 |
| `wiki` | `dws --mock --profile corp_alpha,corp_beta wiki space list --format json` | `dws --profile corp_alpha wiki space list --format json` | 真实知识库只读链路 |

说明：若某个示例 leaf 在当前 discovery 快照中不存在，自动化生成器必须用该产品当前可见的第一个只读 leaf 替换，并在测试报告记录替换后的真实路径。覆盖目标是“每个产品 root 至少一个 leaf 的 L1 + L2”，不是绑定上表的文案路径。

### 3.1.3 Utility / 非 runtime 命令

这些命令必须测试 root `--profile` 是否被正确接受、正确忽略或正确用于 token 读取。

| 指令 | 必测命令 | 预期 |
|---|---|---|
| `dws api` | `dws --profile corp_alpha api GET /v1.0/contact/users/me --dry-run --format json` | raw API token 从 selected profile 解析；dry-run 不触网；不修改 current |
| `dws doctor` | `dws --profile corp_alpha doctor --json` | auth check 使用 selected profile；网络/cache/version 检查不被 profile 污染 |
| `dws cache status` | `dws --profile corp_alpha cache status --json` | profile 被接受但不影响 cache status |
| `dws cache refresh` | `dws --profile corp_alpha cache refresh --product contact` | refresh 请求使用 selected profile token；不修改 current |
| `dws schema` | `dws --profile corp_alpha schema contact.user.get-self --format json` | schema/discovery 读取 selected token；无 profile 参数泄漏 |
| `dws recovery plan` | `dws --profile corp_alpha recovery plan --last -f json` | 恢复分析中的 runtime 调用使用 selected profile；无快照时按原错误返回 |
| `dws recovery execute` | `dws --profile corp_alpha recovery execute --last -f json` | 同 plan；不修改 current |
| `dws recovery finalize` | `dws --profile corp_alpha recovery finalize --event-id evt --outcome recovered` | finalize 为本地状态写入，profile 应被接受但不改变语义 |
| `dws skill setup` | `dws --profile corp_alpha skill setup --mode mono --yes` | profile 被接受但 skill 安装布局不被影响 |
| `dws skill install` | `dws --profile corp_alpha skill install <skillId> claude` | 若需联网，profile 不应成为业务参数；认证失败仍按原错误 |
| `dws skill get/search/find/add` | `dws --profile corp_alpha skill search --query doc` 等 | profile 被接受；输出与未传 profile 一致 |
| `dws plugin list/info/install/remove/enable/disable/validate/create/dev/build/config` | 每个子命令加 root `--profile corp_alpha` 跑 help 或 dry-run/validation | 插件管理不应读取/修改组织 profile |
| `dws config list` | `dws --profile corp_alpha config list --json` | profile 被接受；配置输出不被过滤 |
| `dws completion` | `dws --profile corp_alpha completion zsh` | profile 被接受；补全文本包含 profile flag |
| `dws version` | `dws --profile corp_alpha version --format json` | 输出版本信息；无 profile 副作用 |
| `dws upgrade` | `dws --profile corp_alpha upgrade --check --format json`、`dws --profile corp_alpha upgrade --dry-run` | upgrade 与组织无关；profile 被接受但不改变升级计划 |
| `dws pat chmod` | `dws --profile corp_alpha pat chmod --products calendar --dry-run --format json` | 批量授权读取 selected profile / agent context；未加 `--yes` 不执行授权 |

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

L1 自动化回归步骤：

```bash
dws --mock --profile corp_alpha, corp_beta contact user get-self --format json
dws --mock contact user get-self --profile corp_alpha, corp_beta --format json
```

L2 真实只读冒烟步骤：

```bash
dws --profile corp_alpha contact user get-self --format json
dws --profile corp_beta contact user get-self --format json
dws --profile corp_alpha,corp_beta contact user get-self --format json
dws contact user get-self --profile corp_alpha, corp_beta --format json
```

断言：

- L1 输出为聚合对象，`multiProfile=true`。
- L1 `success=true`。
- L1 `summary.total=2`、`summary.succeeded=2`、`summary.failed=0`。
- L1 `profiles[0].corpId=corp_alpha`，`profiles[1].corpId=corp_beta`。
- L1 每个 `profiles[i].ok=true`，且每个组织都有独立 `result`。
- L2 不允许使用 `--mock`；真实返回应能证明使用了对应组织 token，若远端权限不足，也必须是结构化权限/业务错误而不是 CLI profile 解析错误。
- L2 多 profile 输出仍为聚合对象；允许单个组织因权限/数据状态失败，但 `summary` 和每个 `profiles[i].error` 必须结构化。
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

### TC-14 `--profile` 参数解析全形态

目的：确保 root persistent `--profile` 和 leaf 后置 `--profile` 都符合 `lark-cli` 风格 CSV 规范，并对未加引号空格写法做容错。

L1 parser/runner 自动化步骤，可使用 `--mock` 隔离远端依赖：

```bash
dws --mock --profile corp_alpha contact user get-self --format json
dws --mock --profile corp_alpha,corp_beta contact user get-self --format json
dws --mock --profile corp_alpha, corp_beta contact user get-self --format json
dws --mock --profile=corp_alpha, corp_beta contact user get-self --format json
dws --mock contact user get-self --profile corp_alpha --format json
dws --mock contact user get-self --profile corp_alpha, corp_beta --format json
dws --mock --profile corp_alpha,corp_alpha,corp_beta contact user get-self --format json
```

负向步骤：

```bash
dws --mock --profile corp_alpha, contact user get-self --format json
dws --mock --profile corp_alpha,,corp_beta contact user get-self --format json
dws --mock --profile missing_org,corp_beta contact user get-self --format json
```

L2 真实链路步骤，不使用 `--mock`：

```bash
dws --profile corp_alpha contact user get-self --format json
dws --profile corp_alpha,corp_beta contact user get-self --format json
dws contact user get-self --profile corp_alpha, corp_beta --format json
```

断言：

- L1 单 profile 输出不是聚合对象，且 current profile 不变。
- L1 CSV 多 profile 输出 `multiProfile=true`。
- L1 `corp_alpha, corp_beta` 在 Cobra 解析前被规整，不会把 `corp_beta` 识别为子命令或位置参数。
- `--profile=corp_alpha, corp_beta` 与 `--profile corp_alpha, corp_beta` 等价。
- leaf 后置 `--profile` 与 root 前置 `--profile` 等价。
- 重复 profile 按 resolved `corpId` 去重，返回 Alpha/Beta 两项。
- 尾部逗号、连续逗号、缺失 profile 均返回 validation error，且不执行任何 runtime 调用。
- 若存在本地 profile name 为 `alpha,beta`，`--profile alpha,beta` 先按单 profile 精确匹配，不触发聚合。
- L2 真实链路必须实际读取对应组织 token；失败只接受认证、权限或业务层结构化错误，不接受 mock payload。

### TC-15 Auth 命令 `--profile` 覆盖

目的：覆盖每个 auth 子命令对 root/local `--profile` 的支持、忽略或拒绝语义，防止误删、误切换、误用 token。

前置：

- 已存在 `corp_alpha`、`corp_beta`、`corp_gamma`。
- 当前组织为 `corp_gamma`，previous 为 `corp_alpha`。

步骤与断言：

| 用例 | 命令 | 断言 |
|---|---|---|
| AUTH-P01 root profile status | `dws --profile corp_alpha auth status --format json` | 返回 Alpha；`currentProfile` 仍为 Gamma |
| AUTH-P02 local profile status | `dws auth status --profile corp_beta --format json` | 返回 Beta；`currentProfile` 仍为 Gamma |
| AUTH-P03 local wins | `dws --profile corp_alpha auth status --profile corp_beta --format json` | 返回 Beta；root profile 不覆盖 local profile |
| AUTH-P04 missing status profile | `dws auth status --profile missing_org --format json` | 返回未登录或 validation，不修改 current |
| AUTH-P05 multi status unsupported | `dws auth status --profile corp_alpha,corp_beta --format json` | 不聚合；返回 validation/未登录，避免静默读取错误 profile |
| AUTH-P06 scoped logout | `dws auth logout --profile corp_alpha` | 只删除 Alpha token/profile；Beta/Gamma 保留；current/primary 指针重算正确 |
| AUTH-P07 root profile must not scope logout | `dws --profile corp_alpha auth logout` | 按默认 logout 全部清理，或未来若改为拒绝则必须明确报错；绝不能“看似成功但只删一部分” |
| AUTH-P08 logout local wins | `dws --profile corp_alpha auth logout --profile corp_beta` | 只退出 Beta；Alpha/Gamma 保留 |
| AUTH-P09 reset ignores profile | `dws --profile corp_alpha auth reset` | 清空所有 token、profiles、marker、app config |
| AUTH-P10 login target | `dws --profile corp_alpha auth login --device --format json` | 授权目标解析为 Alpha corpId；不持久切换 current |
| AUTH-P11 login missing profile | `dws --profile missing_org auth login --device --format json` | 授权前 validation fail |
| AUTH-P12 export profile | `dws --profile corp_alpha auth export --base64` | 导出与 Alpha 相关的认证资料，输出不包含明文 secret |
| AUTH-P13 import with profile | `dws --profile corp_alpha auth import --input bundle.json` | import 语义不被 root profile 误导；导入后 token/profile 指针符合 bundle 内容 |
| AUTH-P14 exchange with profile | `dws --profile corp_alpha auth exchange --code <code>` | 真实 UAT 验证；profile 不泄漏为 exchange 业务参数 |

### TC-16 Profile 命令 `--profile` 覆盖

目的：确认 profile 管理命令的显式 selector 优先，root `--profile` 只作为全局 flag 被接受，不会偷偷切换或过滤。

步骤：

```bash
dws --profile corp_alpha profile list --format json
dws --profile corp_alpha profile switch corp_beta --format json
dws --profile corp_alpha profile switch --corpId corp_gamma --format json
dws --profile corp_beta profile switch --name "Alpha Org" --format json
dws --profile corp_beta profile switch - --format json
dws --profile corp_alpha profile use corp_gamma --format json
dws --profile corp_alpha profile use - --format json
```

负向步骤：

```bash
dws --profile corp_alpha profile switch
dws --profile corp_alpha profile switch corp_beta --corpId corp_gamma
dws --profile corp_alpha profile switch missing_org
```

断言：

- `profile list` 仍返回全部组织，不被 root `--profile` 过滤。
- `profile switch/use` 的位置参数、`--corpId`、`--name`、`-` 决定持久切换目标。
- root `--profile` 不覆盖显式 selector。
- 切换后 legacy mirror 同步到新的 current。
- 非交互无 selector 仍 validation fail，不因 root `--profile` 自动选择。
- 冲突 selector 返回 validation fail，不修改 current/previous。

### TC-17 Utility 命令 `--profile` 覆盖

目的：覆盖所有 utility 命令是否正确接受、忽略或使用 root `--profile`。

步骤与断言：

| 用例 | 命令 | 断言 |
|---|---|---|
| UTIL-P01 raw API dry-run | `dws --profile corp_alpha api GET /v1.0/contact/users/me --dry-run --format json` | 使用 Alpha token 构造请求；不触网；current 不变 |
| UTIL-P02 doctor auth | `dws --profile corp_alpha doctor --json` | auth check 针对 Alpha；network/cache/version check 不被污染 |
| UTIL-P03 cache status | `dws --profile corp_alpha cache status --json` | 输出 cache 状态；profile 无副作用 |
| UTIL-P04 cache refresh | `dws --profile corp_alpha cache refresh --product contact` | refresh 读取 Alpha token；不修改 current |
| UTIL-P05 schema | `dws --profile corp_alpha schema contact.user.get-self --format json` | schema/discovery 使用 Alpha 上下文；输出不含 profile 业务参数 |
| UTIL-P06 recovery plan | `dws --profile corp_alpha recovery plan --last -f json` | 有快照时 runtime 分析用 Alpha；无快照时错误语义不变 |
| UTIL-P07 recovery execute | `dws --profile corp_alpha recovery execute --last -f json` | 同 plan |
| UTIL-P08 recovery finalize | `dws --profile corp_alpha recovery finalize --event-id evt_test --outcome recovered` | 本地 finalize 不受 profile 影响 |
| UTIL-P09 skill setup | `dws --profile corp_alpha skill setup --mode mono --yes` | skill 布局与未传 profile 一致 |
| UTIL-P10 skill query | `dws --profile corp_alpha skill search --query doc` | 搜索/查询类输出与未传 profile 一致 |
| UTIL-P11 plugin list | `dws --profile corp_alpha plugin list --format json` | 插件列表不读组织态 |
| UTIL-P12 plugin validate | `dws --profile corp_alpha plugin validate <dir>` | 插件校验不读组织态 |
| UTIL-P13 config list | `dws --profile corp_alpha config list --json` | 配置列表不被 profile 过滤 |
| UTIL-P14 completion | `dws --profile corp_alpha completion zsh` | 生成补全脚本，包含 `--profile` flag |
| UTIL-P15 version | `dws --profile corp_alpha version --format json` | 版本输出不变 |
| UTIL-P16 upgrade check | `dws --profile corp_alpha upgrade --check --format json` | 升级检查与组织无关 |
| UTIL-P17 upgrade dry-run | `dws --profile corp_alpha upgrade --dry-run` | 计划输出与未传 profile 一致 |
| UTIL-P18 pat chmod dry-run | `dws --profile corp_alpha pat chmod --products calendar --dry-run --format json` | 只生成授权 plan；未加 `--yes` 不执行授权；使用 Alpha 上下文 |

断言通用要求：

- 所有命令执行前后 `currentProfile/previousProfile` 不变，除非命令本身是 `profile switch/use` 或 auth 清理命令。
- 对 P-IGNORED 命令，输出不得因 `--profile` 改变业务范围。
- 对 P-SINGLE 命令，token 解析必须落到 selected profile。
- 对 P-UNSUPPORTED 命令，多 profile 必须明确失败或保持原语义，不能将第二个 profile 当位置参数继续执行。

### TC-18 Runtime 产品命令全量 `--profile` 覆盖

目的：每个 runtime 产品 root 至少选一个只读 leaf，覆盖 mock 自动化回归和非 mock 真实冒烟。若 discovery 中 leaf 名称变化，脚本动态选择该产品第一个只读 leaf，并在测试报告记录真实命令。

L1 自动化回归步骤：

```bash
for product in aisearch aitable attendance calendar chat conference contact devdoc ding doc doc-comment drive hrmregister live mail minutes oa pat report sheet todo wiki; do
  dws --mock --profile corp_alpha "$product" <readonly-leaf> --format json
  dws --mock --profile corp_alpha,corp_beta "$product" <readonly-leaf> --format json
  dws --mock "$product" <readonly-leaf> --profile corp_alpha, corp_beta --format json
done
```

L2 真实只读冒烟步骤：

```bash
for product in aisearch aitable attendance calendar chat conference contact devdoc ding doc doc-comment drive hrmregister live mail minutes oa pat report sheet todo wiki; do
  dws --profile corp_alpha "$product" <readonly-leaf> --format json
  dws --profile corp_alpha,corp_beta "$product" <readonly-leaf> --format json
done
```

每个产品的断言：

- L1 单 profile：runner 执行时 `RuntimeProfile=corp_alpha`；输出不是聚合对象。
- L1 多 profile：输出 `multiProfile=true`。
- L1 `summary.total` 等于去重后组织数。
- L1 `profiles[*].corpId` 按输入顺序返回。
- 每个 entry 都有 `ok`；成功时有 `result`；失败时有结构化 `error`。
- 任意产品命令的 invocation params 中不能出现 root `profile` 业务参数。
- leaf 后置 `--profile corp_alpha, corp_beta` 也能被 parser 规整。
- L2 不允许使用 `--mock`；必须真实经过 token resolver、endpoint resolver、transport 调用或真实 stdio client。
- L2 允许远端返回权限/业务错误，但必须能证明请求落在 selected profile；不能是 current profile 泄漏、profile 参数泄漏或 CLI 解析错误。
- 命令执行后 current/previous 不变。

### TC-19 多 profile 聚合负向与局部失败

目的：验证多组织聚合不是“全有或全无”的脆弱实现，局部失败可被结构化表达。

步骤：

```bash
dws --mock --profile corp_alpha,missing_org contact user get-self --format json
dws --profile corp_alpha,missing_org contact user get-self --format json
dws --profile corp_alpha,corp_expired contact user get-self --format json
dws --profile corp_alpha,corp_no_token contact user get-self --format json
dws --profile corp_alpha,,corp_beta contact user get-self --format json
dws --profile corp_alpha, contact user get-self --format json
```

断言：

- selector 解析阶段失败（如 `missing_org`、空 selector）应整体 validation fail，不执行任何组织调用。
- 已解析 profile 中单组织 token 过期/缺失时，聚合输出 `success=false`、`summary.failed>0`，成功组织仍返回 result。
- 错误 entry 包含 `message`，若是 typed error 还包含 `category/reason/operation/exitCode`。
- 局部失败不修改 current/previous。
- 日志和 stdout 不输出 access token、refresh token、persistent code。

### TC-20 高风险指令与 `--profile` 防误用覆盖

目的：覆盖删除、授权、升级、导入导出等高风险指令在 `--profile` 下不会被误解为用户确认或范围扩大。

步骤与断言：

| 风险点 | 命令 | 断言 |
|---|---|---|
| 删除类命令 | `dws --profile corp_alpha doc delete <id> --dry-run`、`dws --profile corp_alpha aitable base delete <id> --dry-run` | `--profile` 只选组织，不等同 `--yes`；未确认不执行 |
| 删除类确认 | `dws --profile corp_alpha doc delete <id> --yes` | 只在 Alpha 组织上下文执行；输出记录目标组织/endpoint |
| PAT 批量授权 | `dws --profile corp_alpha pat chmod --products calendar --grant-type once --dry-run --format json` | dry-run 只出 plan；无 `--yes` 不授权 |
| PAT 执行确认 | `dws --profile corp_alpha pat chmod --products calendar --grant-type once --yes --format json` | 用户已确认时才执行；agentCode/sessionId/profile 不混淆 |
| auth logout | `dws --profile corp_alpha auth logout` | 按 TC-15 明确验证：root profile 不能被误认为 local scoped logout |
| auth reset | `dws --profile corp_alpha auth reset` | 仍是全局清理或明确拒绝；不能只清 Alpha 后声称 reset 成功 |
| upgrade rollback | `dws --profile corp_alpha upgrade --rollback --yes` | profile 与 rollback 无关；回滚确认仍由 `--yes` 控制 |
| import/export | `dws --profile corp_alpha auth export/import ...` | 不输出明文 secret；不把 profile 写入 bundle 的业务字段 |

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
| 真实 OAuth 登录未在黑盒脚本中扫码验证 | 自动脚本 seed 登录后 token，避免人工和网络依赖 | 发布前必须追加一次手工 UAT：真实 `auth login` 登录 A/B 两个组织 |
| Runtime 自动化大量使用 `--mock` | `--mock` 只覆盖 CLI 解析和 runner 聚合，不能证明真实 MCP 链路、权限和租户隔离 | 每个产品 root 还必须执行 L2 非 mock 只读冒烟；权限失败也要是结构化远端错误 |
| 远端 revoke/logout 不在黑盒脚本中直连验证 | 网络不稳定且可能影响真实环境 | 已由 Go 回归用隔离环境覆盖本地删除语义；真实环境只做单组织 logout 冒烟 |
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
- L2 非 mock 真实只读冒烟通过，或对权限不足场景产出结构化错误并确认使用了 selected profile。
- L3 高风险写操作只在 `--dry-run`、测试租户或显式用户确认后执行。
- `profiles.json` 无敏感字段。
- 所有 P0 功能 F1-F7 至少有一个自动化用例覆盖。
- 关键 TUI/交互入口均有机器指令替代路径。
- 手工 UAT 未发现真实 OAuth 组织选择和权限链路阻断。
