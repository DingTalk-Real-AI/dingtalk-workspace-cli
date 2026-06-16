---
name: dingtalk-dev
description: 钉钉开放平台企业内部应用全生命周期管理。应用是容器：凭证/权限点/成员/安全配置/能力扩展(网页应用/机器人)/版本，需审批的变更要发版本才生效。Use when 用户说 开发者后台应用/开放平台应用/企业内部应用/查应用/创建应用/修改应用/删除应用/停用应用/启用应用/应用成员/安全配置/IP白名单/登录重定向/端内免登/agentId/clientId/appKey/appSecret/customKey/应用权限/权限点/scopeValue/创建机器人/智能体机器人/机器人配置/机器人回调/应用版本/版本发布/发布审核/选审批人/本地调试机器人/把机器人接到本地agent/建联/connect。
cli_version: "1.0.15+"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉开放平台应用管理 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — dev 为预览能力，仅在 multi 模式提供，接口与命名后续可能调整。问题请提 issue 反馈。

> **PREREQUISITE:** Read the root `dws` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> `dws dev app ...` 是内置 helper 命令，不依赖 MCP 服务发现。首次使用前（或命令报错时）用 `dws dev app --help` 验证可用，验证过就不用每次重复；若报 `unknown command`，说明当前 dws 构建不含 dev 命令组（预览能力，`dws upgrade` 装不到），按仓库 `docs/dev-agent-install-guide.md` 用 install-dev.sh 从 `feat/dws-dev` 分支安装。

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

映射示例：「想做个钉钉里打开的网页」= 创建应用 → webapp 配置 → 发版本；「做个答疑机器人」= robot submit/result 异步建号（或现有应用 robot config）→ 发版本 →（本地调试用 dev connect）。

### ID 体系

| 标识 | 是什么 | 用在哪 |
|------|--------|--------|
| `unifiedAppId` | 统一应用 ID，新模型主键 | **唯一全树定位标识**，所有单应用命令都用 `--unified-app-id` |
| `appKey` = `clientId` | 应用身份标识，同一个东西的新旧两个名字，非密钥 | OpenAPI 调用、建联；也可作 `--app-key` 列表过滤（不能定位单应用） |
| `appSecret` = `clientSecret` | 应用密钥，敏感 | 同上，按敏感凭证处理 |
| `agentId` | 旧版微应用 ID，历史遗留 | 仅出现在返回数据里；**不能用于定位**，拿到它先查出 unifiedAppId |
| `robotCode` | 机器人编号 | 加群、机器人发消息、建联 |

应用定位统一只用 `--unified-app-id`（--app-key/--name 仅作 list 过滤，不能定位单应用）。旧的 `--agent-id`/`--app-id`/`--custom-key` 定位 flag 已移除——agentId 只是返回字段，不再是入参定位手段。appKey 与 clientId 是同一标识的新旧两名，无需追问区别。

### 生效模型（最重要）

**改配置 ≠ 线上生效。** 需审批的变更（如 `requiredApproval=true` 的权限点）先累积在开发态，必须走版本通道才上线：

```
配置变更（permission add / robot config / webapp config ...）
  → version create → check-approval（预检审批要求+候选审批人）
  → publish（需审批时由用户选审批人）→ versionStatus=RELEASE 才生效
```

- 机器人等能力需版本发布后才能被搜索、加群、路由消息。
- 用户问「为什么没生效 / 机器人搜不到 / 权限加了还报错」→ 先查 `version status`。
- 两套状态别混：应用 appStatus（0 停用 / 1 激活 / 2 待激活 / 3 过期）是应用开关；版本 versionStatus（INIT / AUDIT / RELEASE / GRAY）是变更走到哪了。

### 边界与角色

- 本 skill 只管**企业内部应用**。接口文档 → `dingtalk-devdoc`；钉钉云文档 → `dingtalk-doc`；工作台入口的「应用」→ `workbench app`；群里发消息用的机器人 → `dingtalk-chat`；审批流 → `dingtalk-oa`。
- 角色：开发者（member DEVELOPER）改配置；管理员管启停；审批人批版本发布——审批人必须用户拍板，agent 不代选、不默认取第一个。

## 核心规则

1. `应用`、`机器人` 是泛词：用户只说这两个词、无开放平台上下文时，先追问确认是不是开发者后台的企业内部应用，不要猜——很可能是工作台应用或群消息机器人（转出口见上方「边界与角色」）。
2. 所有命令加 `--format json`。
3. 写操作先 `--dry-run`，确认后才加 `--yes`。例外：`dev connect` 的 dry-run 是建联预检（渠道识别、凭证来源、本地 agent CLI 是否安装），正式 connect 是前台长驻进程——在对话里跑必须后台运行并告诉用户如何停止，或引导用户自己开终端跑。
4. 写操作返回成功 ≠ 状态已变：启停、机器人配置、版本发布等关键写操作执行后，回读对应查询命令（`get` / `robot get` / `version status`）确认状态，以回读结果为准；接口只返回操作成功但未带状态时，向用户说明以回读为准。
5. 应用名/appKey 命中多条时展示候选，不取第一条。
6. 权限申请/取消只接受 `scopeValue`，不传 API 名或分组名——权限点才是授权单元，API 名与权限点是多对一。
7. 主动读取密钥走 `credentials get`；任何返回里的 `clientSecret/appSecret` 都按敏感凭证处理，不写进回答文本。例外：建联流程内部把 secret 作为参数传给 `dev connect` 是必要用途。

### 通用出参约定（跨所有命令）

- **dry-run**：出参里看 `invocation.params`（将下发的参数）确认无误，再把 `--dry-run` 换成 `--yes`；dry-run 时 `response` 只是回显。
- **游标分页**（list / permission list / version list / event list / doc search）：首次不传 `--cursor`，出参带 `nextCursor`（空=到底）原样回传续翻；`hasMore == nextCursor 非空`。cursor 是上游不透明令牌，不要自己解析或构造，也不要跨命令复用。
- **批量聚合**：`permission remove` 传多个权限点时出参是 `{results:[...], ok, total, failedCount}`，逐条判断成败（`ok=false` 即有失败）。
- **pretty**：`--format pretty` 会在应用/版本状态字段旁附 `*Text` 可读标签（如 `appStatusText`）；JSON 格式不附，以原始字段为准。
- **失败**：`ServiceResult.success=false` 原样透传 `errorCode/errorMsg`，不编造解释，解读走下方文档 RAG。

## 开放平台文档 RAG / 错误码排查

- dev 命令执行中，只要用户问开放平台 API、接口参数、字段含义、权限点、回调、SDK、配额、错误码，或命令返回上游 OpenAPI/SDK 错误，必须先用 `dws dev doc search --query "<关键词>" --format json` 做官方文档 RAG。
- 业务错误（`ServiceResult.success=false`）原样透传 `errorCode/errorMsg`，不要编造解释；需要解读错误含义时走 devdoc RAG。
- 查询词优先保留原始 API 名、能力名、权限点、完整错误码和 message；首轮形如 `errcode <code> <message>`，无结果再换 `<产品/场景> <错误码>`、`<接口名> 参数`。
- 本地 CLI 错误（如 `unknown command` / `unknown flag` / 认证 / recovery）仍按 root `dws` / `dws-shared` 的错误处理执行；`devdoc` 用于开放平台业务错误码和接口语义排查。
- `devdoc` 只查钉钉开放平台开发者文档，不查业务数据；排查结论必须基于命中条目的标题、摘要或链接，不能编造错误原因或不存在的命令。

## 典型任务

端到端任务都是「定位应用 → 改容器某节点 →（按审批需要）走版本生效 → 回读验证」。常见链路（每步先 `--dry-run` 确认再 `--yes`，细节进对应产品文件）：

- **建钉钉里打开的网页应用**：`app create` → `webapp config` → `version create` → `check-approval` → `publish` →（回读 `version status` 到 `RELEASE`）。
- **权限从申请到生效**：`permission list`（选 `scopeValue`）→ `permission add` →（`requiredApproval` 的权限）`version create` → `check-approval`（看是否需审批+候选审批人）→ `publish --approver <用户选的>` → `version status`。
- **做答疑机器人**：`robot submit` → `robot result`（`SUCCESS` 拿 clientId/secret），或现有应用 `robot config` →（发版本）→ 本地调试 `dev connect`。
- **查"为什么没生效 / 机器人搜不到 / 权限加了还报错"**：先 `version status`——改配置 ≠ 生效，未发到 `RELEASE` 就不生效。

> 审批人必须用户拍板，agent 不代选、不默认取第一个。

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
| 事件订阅 | [event.md](references/event.md) | event list / subscribe / unsubscribe |

## Gotchas

跨命令组的真实坑，随使用积累追加（单命令组的坑记在对应 reference 的错误处理表里）：

- 新应用 `version list` 返回空 ≠ 无可发布内容：先 `version create`，用返回的 `versionId` 继续 check-approval/publish。
- `robot info is not exist` 是「应用还没配过机器人」，走 `robot config` 首次创建，不是 `enable`。
