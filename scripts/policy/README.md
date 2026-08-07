# scripts/policy —— 统一输出契约 CI 扫描原型（Phase I，B161~B167）

本 README 只覆盖 Phase I「CI 扫描原型」四个脚本与配套 fixture：

- `check-stdout-json.sh`（B161）—— json 模式 stdout 纯 JSON 检查（AC-11）
- `check-string-bool.sh`（B162）—— 字符串布尔违规扫描（AC-02）
- `check-envelope-keys.sh`（B163）—— 非标准信封键名扫描（信封顶层键集合固化，G1）
- `output-contract-lib.sh` —— 三脚本共享库（样本表、隔离 HOME、重试纪律、self-test 框架）
- `testdata/` —— self-test fixture（合法信封 + 各类违规样例）

## 定位：原型，非强制门禁（B165）

这三个脚本是 **Phase I 扫描原型**：

- **不接入 `make policy`**，不是 CI 强制门禁；接入方式见文末「接入 make policy 的挂点设计草案」（B167，仅设计稿）。
- 脚本自带 `--self-test` 模式，用 `testdata/` fixture 验证扫描逻辑本身（合法样例必须 pass、违规样例必须 fail），这是原型阶段的主要回归手段。
- 契约锚点：AC-02（ok/success 类布尔恒为 JSON 布尔）、AC-11（json 模式 stdout 零日志字节、primary result 恰为一个可解析 JSON 文档）、信封顶层键集合固化（`ok/outcome/identity/dry_run/data/meta/error/_notice` snake_case；`contract_version/errcode/error_code/errorCode/success` 等非标准顶层形态违规，camelCase wire 键形态违规）。

## 用法

```bash
make build            # 先构建 ./dws（脚本消费真实二进制）
./scripts/policy/check-stdout-json.sh            # 默认 --scope dev
./scripts/policy/check-string-bool.sh --scope all
./scripts/policy/check-envelope-keys.sh --self-test   # fixture 自检
```

退出码：`0` = 通过；`1` = 发现违规（逐条打印 `[样本] 原因`）；`2` = 用法/环境错误（缺二进制、缺 jq、非法参数）。

## 样本与 --scope（B166）

脚本对**代表性命令的真实 stdout** 做扫描，样本表定义在 `output-contract-lib.sh`
的 `output_contract_samples()`，按 scope 分两档：

| scope | 样本 | 说明 |
|---|---|---|
| `dev`（默认） | `dev connect list`、`dev connect status`、`dev connect stop --dry-run` | 全部是**离线确定性** v2 terminal 样本：隔离新建 HOME + `DWS_DISABLE_KEYCHAIN=1`，无需登录、无网络、无副作用。`dev connect` 根同时承载 stream 与 daemon start，当前整体 legacy，不纳入 v2 信封样本。 |
| `all` | dev 档 + `dev app list` + `devapp +list` + `schema list` + `auth status` + `version` | 两个 devapp 入口需要登录态（继承真实 HOME），并覆盖原子命令与 shortcut adapter；后三个是**未迁移的非信封 json 输出**（legacy class），只做可解析性与字符串布尔检查，**不做信封形状检查**（那是迁移前已知形态，不是回归） |

样本分两类（`output_contract_samples()` 首列）：

- `envelope` —— 已迁移统一信封的输出，三个脚本的全部规则适用；
- `legacy` —— 迁移前的非信封 json（如 `auth status` 顶层 `success` 键），
  `check-envelope-keys.sh` 对其**豁免信封形状检查**，只保留可解析性与
  字符串布尔检查；`dev` scope 下不含 legacy 样本，因此默认档对信封形状零豁免。

扫描纪律（硬纪律 3 落点）：

- 每个样本**失败自动重试一次**再下结论（并行在途改动可能造成偶发异常）。
  非零退出但 stdout 非空时仍扫描该文档（v2 `failure` / `partial_failure` 本来就
  使用非零退出码）；仅非零退出且 stdout 为空时视为环境/鉴权不可用，记
  `[skip]` 并打印 stderr 尾部。
- **至少一个样本成功验证**，否则脚本拒绝通过（防止空跑假绿）。
- `DWS_SCAN_HOME` 环境变量可覆盖样本 HOME（运维/CI 定制环境用）。

## B164 误报核查记录

核查方式：`make build` 后对真实二进制输出跑三个脚本，两档 scope 分别执行：

1. **`--scope dev`（默认档）**：当前扫描 3 个离线 v2 terminal 样本。样本真实输出形态核对：`dev connect list` 输出
   `{ok,outcome,data:[],meta.count:0}` 信封；`status`/`stop` 对一次性
   probe id 输出 `{ok:true,outcome:success,...}`；stop 使用 dry-run 预览，保证策略扫描不发进程信号。
2. **`--scope all`**：覆盖 9 个样本，包含 `dev app list` 原子入口与
   `devapp +list` shortcut 入口。三个脚本均对非零 v2 结果继续扫描 stdout。
   legacy 样本（`schema list`/`auth status`/`version`）顶层含 `success` 等
   非信封键，被 legacy class 正确豁免信封形状检查；`dev app list` 信封
   （含 `meta.pagination.endpoint_exhausted:true`）全规则通过。
3. **检出能力反向验证**：用一个故意违规的 stub 二进制（`DWS_BIN` 覆盖）
   验证三脚本确实能抓到违规——stdout 混入 `[INFO]` 行且不可解析、
   `"ok":"true"` 字符串布尔、`errorCode` 顶层历史键、`meta.timedOut`
   camelCase 形态均被对应脚本以非零退出码报出；`--self-test` 模式下
   全部违规 fixture（`string_bool_violation.json`、`envelope_legacy_keys.json`、
   `envelope_camel_keys.json`、`legacy_envelope_keys.json`、`stdout_*` 系列）
   判 fail、全部合法 fixture 判 pass。

核查结论：**三脚本对现有合法输出零误报，对契约违规形态可检出**。
已知边界（原型立场，见下「设计取舍」）：`data` 载荷内的 camelCase /
legacy 命名业务字段（如服务端返回的 `hasMore`/`success` 业务字段）不视为
信封违规——信封契约只约束信封结构层（顶层键 + meta/error 子树）。

## 设计取舍（供 DESIGN 评审）

1. **扫描面是运行时 stdout 而非源码静态 grep**：三个脚本都消费真实二进制的
   json 模式输出（B161/B162/B163 的批次描述均以「命令输出样例」为扫描对象）。
   源码静态扫描（如 Go 文件里的 json tag 审计）是另一个维度，与 B158~B160
   的 Go 侧 fixture 扫描互补，不在本原型范围。
2. **legacy class 豁免**：`--scope all` 纳入的 `schema list`/`auth status`/
   `version` 是未迁移输出，豁免信封形状检查而非删除样本——保留其可解析性与
   字符串布尔检查，等 Phase F/E 迁移完成后这些样本升级为 envelope class，
   豁免自然消失。
3. **失败重试一次 + 有限 skip 机制**：对齐硬纪律 3；非零且有 stdout 的 v2
   结果必须扫描，只有无 stdout 的环境/鉴权失败可 skip。skip 计数显式打印在
   结果行，全 skip 时拒绝通过。

## 接入 make policy 的挂点设计草案（B167）

**本草案仅为设计稿，不修改 Makefile。**

### 挂点位置

建议挂在 `make policy` 链的**尾部**（`check-schema-binary.sh` 与
`test-schema-agent-examples` 之后），理由：

- 三脚本需要 `make build` 产物 `./dws`；policy 链前段的 schema 门禁已隐含
  构建，挂尾部可复用产物、避免重复构建。
- 扫描对象是「最终交付二进制的运行时输出」，语义上位于所有装配门禁之后。

### 接入形态（两档渐进）

**第一档（原型转正初期，建议）——独立 target，不进 policy 链：**

```make
output-contract-prototype:
	@$(POLICY_ENV) ./scripts/policy/check-stdout-json.sh
	@$(POLICY_ENV) ./scripts/policy/check-string-bool.sh
	@$(POLICY_ENV) ./scripts/policy/check-envelope-keys.sh
```

手动/按需触发，允许 skip（登录态、网络等环境差异）不阻塞主链。

**第二档（dev 域全域迁移完成后）——进 policy 链，scope=dev 硬门禁：**

```make
policy: ...
	@$(POLICY_ENV) ./scripts/policy/check-stdout-json.sh --scope dev
	@$(POLICY_ENV) ./scripts/policy/check-string-bool.sh --scope dev
	@$(POLICY_ENV) ./scripts/policy/check-envelope-keys.sh --scope dev
```

升级前提（进入第二档的准入条件）：

1. dev 域全部叶子完成统一信封迁移（Phase F 收口，B138 核销清单测试通过）；
2. `--scope dev` 样本表扩充到 dev 域全部读叶子（当前 4 个代表样本），
   且连续 N 次（建议 N=5）本地全绿；
3. CI 环境能提供登录态或确认全部样本离线可跑（当前 dev 档已满足离线要求，
   此条主要针对第二档扩样后是否引入登录依赖）。

`--scope all` 保持**永不进硬门禁**（legacy 样本的豁免语义 + 登录态依赖），
作为 advisory 扫描保留在独立 target。

### 与现有门禁的边界

- 与 `check-schema-catalog.sh` 无重叠：后者校验 Schema Catalog 装配契约，
  本原型校验命令**运行时输出**契约。
- 与 `check-cli-smoke.sh` 无重叠：smoke 校验 help 渲染与命令树存在性，
  本原型校验 json stdout 的机器消费契约。
- self-test（`--self-test`）建议同时接入第一档 target，作为 fixture 回归：
  `@for s in check-stdout-json check-string-bool check-envelope-keys; do ./scripts/policy/$$s.sh --self-test; done`

### shellcheck 备注

本批次开发机上 `shellcheck` 不可用（未安装），四脚本以 `sh -n` 语法校验 +
真实运行（dev/all 两档 + stub 反向验证）替代；接入 CI 前建议补一轮
`shellcheck -s sh`。脚本按 POSIX sh 编写（`#!/bin/sh` + `set -eu`，与目录
既有脚本一致），bash 3.2 兼容（未使用 bash 扩展语法）。
