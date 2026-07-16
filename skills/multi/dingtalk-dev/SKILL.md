---
name: dingtalk-dev
description: 钉钉开放平台企业内部应用管理：应用查询/创建/修改/删除/停启、凭证、权限点、成员、安全配置、网页应用、机器人、版本发布。Use when 用户说 开发者后台应用/开放平台应用/企业内部应用/agentId/clientId/appKey/appSecret/customKey/应用权限/权限点/scopeValue/创建机器人/智能体机器人/机器人配置/机器人回调/IP白名单/登录重定向/端内免登/应用版本/版本发布/发布审核/选审批人/把机器人接到本地调试/connect。
cli_version: "1.0.37+"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉开放平台应用管理 Skill
## MUST DO

每次执行 dev 命令前，先查清楚再敲，别凭记忆或猜：

1. `--help` 看命令树（一个组下有哪些子命令、flag），例 `dws dev app --help`
2. `--help` 看叶子命令参数（flag、默认值、示例），按当前二进制输出构造；不要再依赖已下线的动态 schema
3. 全部命令带 `--format json`
4. 写操作：`--dry-run` 看 `invocation.params` 确认无误，再换 `--yes`（`dev connect` 例外见 [connect.md](references/connect.md)）
5. 写完回读确认（`get` / `robot get` / `version status`）
6. `clientSecret/appSecret` 不写进回答（脱敏）
7. `robot result` 只要出现 `completionState=BLOCKED_BY_VERSION_PUBLISH` 或 `mustContinue=true`，必须继续执行 blocking `nextSteps`，不得停在 `dev connect`
8. `dev connect` 的 `completionState=LOCAL_DEBUG_ONLY` / `doesNotPublish=true` 只代表本地调试，不能作为最终完成态
9. `version check-approval` 若返回 `completionState=WAITING_FOR_APPROVER_SELECTION`，选择题原样展示 `approvalPromptText`（或 `approvalOptions[].label`），不得把姓名丢成泛化“候选审批人”
10. `robot result` 若缺 `unifiedAppId` 或返回 `completionState=BLOCKED_BY_MISSING_UNIFIED_APP_ID`，必须停下要求明确的 `unifiedAppId`；禁止用 `clientId/appKey` 自动反查后继续执行版本写操作

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 来自独立于 Runtime Schema 的公开 catalog。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 `dws shortcut list --service devapp --format json` 读取参数、约束、风险和示例，并以 `dws devapp <shortcut> --help` 核对当前 Cobra flags；不要对 `+` 路径调用 `dws schema`。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws devapp +create` | write | 创建开放平台企业内部应用 |
| `dws devapp +delete` | high-risk-write | 删除开放平台企业内部应用（不可逆） |
| `dws devapp +disable` | high-risk-write | 停用开放平台企业内部应用 |
| `dws devapp +enable` | write | 启用开放平台企业内部应用 |
| `dws devapp +event-list` | read | 查询应用已订阅的事件列表 |
| `dws devapp +get` | read | 查询开放平台企业内部应用详情 |
| `dws devapp +list` | read | 查询开放平台企业内部应用列表 |
| `dws devapp +member-add` | write | 添加开放平台应用成员 |
| `dws devapp +member-list` | read | 查询开放平台应用成员 |
| `dws devapp +member-remove` | high-risk-write | 移除开放平台应用成员 |
| `dws devapp +permission-list` | read | 查询开放平台应用权限列表 |
| `dws devapp +robot-get` | read | 查询现有应用的机器人配置 |
| `dws devapp +update` | write | 修改开放平台企业内部应用基础信息 |
| `dws devapp +version-check-approval` | read | 预检版本发布是否需要审批（不实际发布） |
| `dws devapp +version-get` | read | 查询指定版本详情 |
| `dws devapp +version-list` | read | 分页查询应用版本列表 |
| `dws devapp +version-status` | read | 查询版本发布/审批状态 |
| `dws devapp +webapp-config` | write | 配置网页应用能力 |
| `dws devapp +webapp-get` | read | 查询网页应用配置 |
<!-- VISIBLE_SHORTCUTS_END -->

## 概念地图
先建立领域模型，再看命令——所有命令都是对这张图上某个节点的操作，用户的模糊意图先映射到节点再选命令。

### 应用是什么
钉钉开放平台的「企业内部应用」是企业自建的扩展程序。一个应用是一个容器：

```
企业内部应用（主键 unifiedAppId）
├── 凭证        appKey/appSecret —— 应用调 OpenAPI 的身份（credentials）
├── 权限        权限点 scopeValue，每个权限点授权一组 OpenAPI（permission）
├── 成员        DEVELOPER 等角色，决定谁能改这个应用（member）
├── 安全配置    IP 白名单 / 登录重定向 / 端内免登 URL（security）
├── 能力扩展    应用对用户「长什么样」，可同时挂多种：
│   ├── 网页应用  钉钉内打开的 H5，配移动端/PC 首页地址（webapp）
│   └── 机器人    群聊/单聊收发消息，走回调 URL 或接本地 agent（robot）
└── 版本        配置改动的生效通道（version）
```
映射示例：「想做个钉钉里打开的网页」就是 创建应用，再配 webapp，再发版本；「做个答疑机器人」就是先创建应用拿 `unifiedAppId`，再 `robot config/enable` 配机器人，发版本后本地调试用 `dev connect`。无绑定的 `robot submit/result` 只有在结果返回明确 `unifiedAppId` 时才能续到版本发布。

### ID 体系
| 标识 | 是什么 | 用在哪 |
|------|--------|--------|
| `unifiedAppId` | 统一应用 ID，全树主键 | 唯一全树定位标识，所有单应用命令都用 `--unified-app-id` |
| `appKey` = `clientId` | 应用身份标识，同一个标识的两个名字，非密钥 | OpenAPI 调用、建联；也可作 `--app-key` 列表过滤（不能定位单应用） |
| `appSecret` = `clientSecret` | 应用密钥，敏感 | 同上，按敏感凭证处理 |
| `agentId` | 应用 ID，仅出现在返回数据里 | 不能用于写操作定位 |
| `robotCode` | 机器人编号 | 加群、机器人发消息、建联 |

应用定位统一只用 `--unified-app-id`（--app-key/--name 仅作 list 过滤，不能定位单应用）。agentId 只是返回字段，不能用于写操作定位。appKey 与 clientId 是同一标识的两个名字，无需追问区别。

### 生效模型
- 改配置不等于线上生效，需审批的变更（如 `requiredApproval=true` 的权限点）先累积在开发态，必须走版本通道才上线：
```
配置变更（permission add / robot config / webapp config ...）
  → version create → check-approval（预检审批要求+候选审批人）
  → publish（需审批时由用户选审批人）→ versionStatus=RELEASE 才生效
```
- 机器人等能力需版本发布后才能被搜索、加群、路由消息。
- `robot result` 返回 `APPROVAL_REQUIRED` 时不要重复建号：这表示已建号但线上使用需走版本发布审核；若已返回 clientId/clientSecret，可先用于本地 `dev connect` 调试。
- `robot result` 顶层 `completionState=BLOCKED_BY_VERSION_PUBLISH` / `mustContinue=true` 是硬门禁：继续执行 blocking `nextSteps`，直到版本 `RELEASE`、`AUDIT/UNDER_REVIEW`，或停在 `SELECT_APPROVER` 等用户选审批人。
- `robot result` 顶层 `completionState=BLOCKED_BY_MISSING_UNIFIED_APP_ID` / `actionRequired=provide_unified_app_id` 表示缺少可安全写版本的应用主键：只能让用户提供明确 `unifiedAppId`，不能根据 `clientId/appKey` 的列表结果自动选择应用。
- `dev connect` 成功只代表本地 Stream 调试可用。只要 `robot result.lifecycle.overallComplete=false`，或版本未进入 `RELEASE` / `AUDIT` / `UNDER_REVIEW`，不要总结“全部完成”“机器人已创建并成功连接”“可以在钉钉中 @机器人使用”。
- 用户问「为什么没生效 / 机器人搜不到 / 权限加了还报错」时，先查 `version status`。
- 两套状态别混：应用 appStatus（字符串，取值如 normal / published）是应用开关；版本 versionStatus（INIT / AUDIT / RELEASE / GRAY）是变更走到哪了。app list 不回 appStatus（恒 null），看应用状态以 app get 为准。

### 边界与角色
- 本 skill 只管企业内部应用。接口文档用 `dingtalk-devdoc`；钉钉云文档用 `dingtalk-doc`；工作台入口的「应用」用 `workbench app`；群里发消息用的机器人用 `dingtalk-chat`；审批流用 `dingtalk-oa`。
- 角色：开发者（member DEVELOPER）改配置；管理员管启停；审批人批版本发布。

## 核心规则
1. `应用`、`机器人` 是泛词：用户只说这两个词、无开放平台上下文时，先追问确认是不是开发者后台的企业内部应用，不要猜——很可能是工作台应用或群消息机器人（转出口见上方「边界与角色」）。
2. 应用名/appKey 只可用于只读列表过滤或人工排查；任何写操作必须由用户或上游结果提供明确 `unifiedAppId`，不能把单条列表命中当自动确认。
3. 权限申请/取消只接受 `scopeValue`，不传 API 名或分组名——权限点才是授权单元，API 名与权限点是多对一。
4. 主动读取密钥走 `credentials get`（secret 的脱敏要求见 MUST DO）；例外：connect 流程内部把 secret 作为参数传给 `dev connect` 是必要用途。
5. 审批人必须用户拍板，agent 不代选、不默认取第一个。
6. 选审批人时优先原样展示 `approvalPromptText`（成品文案）；需结构化时读 `approvalOptions[].label`；只有都缺时才用原始 `approvalCandidates` 的 `name（userId: xxx）` 自己拼标签。

### 通用出参约定（跨所有命令）
- 游标分页（list / permission list / version list / event list / `devdoc article search`）：首次不传 `--cursor`，出参带 `nextCursor`（空=到底）原样回传续翻；`hasMore == nextCursor 非空`。cursor 是上游不透明令牌，不要自己解析或构造，也不要跨命令复用。
- 批量聚合：`permission remove` 出参是 `{removed, removedScopeValues, rejectedScopeValues, success, message}`，逐条看 `removedScopeValues`/`rejectedScopeValues` 判断每个权限点成败。
- pretty：`--format pretty` 会在应用/版本状态字段旁附 `*Text` 可读标签（如 `appStatusText`）；JSON 格式不附，以原始字段为准。
- 失败：`ServiceResult.success=false` 原样透传 `errorCode/errorMsg`，不编造解释，解读走下方文档 RAG。

## 开放平台文档 RAG / 错误码排查
- dev 命令执行中，只要用户问开放平台 API、接口参数、字段含义、权限点、回调、SDK、配额、错误码，或命令返回上游 OpenAPI/SDK 错误，必须先用 `dws devdoc article search --query "<关键词>" --format json` 做官方文档 RAG（`dev doc search` 当前网关未注册该工具键，会报「未找到指定工具」，一律走 `devdoc article search`；flag 是 `--query` 不是 `--keyword`）。
- 业务错误（`ServiceResult.success=false`）原样透传 `errorCode/errorMsg`，不要编造解释；需要解读错误含义时走 devdoc RAG。
- 查询词优先保留原始 API 名、能力名、权限点、完整错误码和 message；首轮形如 `errcode <code> <message>`，无结果再换 `<产品/场景> <错误码>`、`<接口名> 参数`。
- 本地 CLI 错误（如 `unknown command` / `unknown flag` / 认证 / recovery）仍按 root `dws` / `dws-shared` 的错误处理执行；`devdoc` 用于开放平台业务错误码和接口语义排查。
- `devdoc` 只查钉钉开放平台开发者文档，不查业务数据；排查结论必须基于命中条目的标题、摘要或链接，不能编造错误原因或不存在的命令。

## 典型任务
端到端任务都是「定位应用，改容器某节点，按审批需要走版本生效，最后回读验证」。完整链路（建网页应用 / 权限到生效 / 建机器人接本地调试 / 排查没生效）见 [recipes.md](references/recipes.md)。

## 产品索引

按命令组直达（一命令组一文件）：

| 命令组 | 参考文档 | 覆盖命令 |
|--------|---------|---------|
| 应用 | [app.md](references/app.md) | list / get / create / update / delete / disable / enable |
| 凭证 | [credentials.md](references/credentials.md) | credentials get |
| 网页应用 | [webapp.md](references/webapp.md) | webapp get / config |
| 权限 | [permission.md](references/permission.md) | permission list / add / remove |
| 成员 | [member.md](references/member.md) | member list / add / remove |
| 安全配置 | [security.md](references/security.md) | security config |
| 机器人 | [robot.md](references/robot.md) | robot submit / result / get / config / enable / disable |
| 本地建联 | [connect.md](references/connect.md) | dev connect（渠道预检 / agent 模型工作目录 / 会话记忆 / AI 卡片） |
| 版本发布 | [version.md](references/version.md) | version create / list / get / check-approval / publish / status |
| 事件订阅 | [event.md](references/event.md) | event list / subscribe / unsubscribe（事件定位走搜索优先） |

## Gotchas
- 新应用 `version list` 返回空不等于无可发布内容：先 `version create`，用返回的 `versionId` 继续 check-approval/publish。
- `robotStatus=UNCONFIGURED` 是「应用还没配过机器人」，走 `robot config` 首次创建，不是 `enable`。
