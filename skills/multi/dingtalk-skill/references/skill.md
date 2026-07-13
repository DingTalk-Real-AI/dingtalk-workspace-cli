# 单命令产品合集

以下产品命令较少，合并参考。

---

## devdoc — 开放平台文档

### 搜索开放平台文档
```
Usage:
  dws devdoc article search [flags]
Example:
  dws devdoc article search --query "OAuth2 接入" --page 1 --size 10 --format json
Flags:
      --query string     搜索关键词 (必填)
      --page string      页码 (默认 1)
      --size string      每页数量 (默认 10)
```

## live — 直播

### 查看我的直播列表
```
Usage:
  dws live stream list [flags]
Example:
  dws live stream list --format json
```

---

## skill — 技能管理

悟空技能市场与企业技能库：搜索技能、安装到本地 Agent 目录、从本地目录或 zip 发布到企业技能库。

### 搜索技能

```
Usage:
  dws skill search [flags]
Example:
  dws skill search --query "周报"
  dws skill search --query "日报" --source "OrgInternal"
  dws skill search --query "日报" --source "DingtalkMarket OrgInternal"
Flags:
      --query string     搜索关键词 (必填)
      --source string    查询范围，空格分隔。备选值：DingtalkMarket（钉钉市场）、OrgInternal（企业内部）。为空默认查市场技能
```

> `--scopes` 已废弃（会打印 `Flag --scopes has been deprecated, 请使用 --source 替代` 警告），一律用 `--source`。
> 前置：`skill search` 依赖企业开通技能市场；未开通时报「当前企业暂未开放此功能」，这是企业级开关，非命令写法问题。

返回字段:
- `skillId` — 技能 ID（后续 `install` 需要）
- `name` — 技能唯一标识（SKILL.md 的 name）
- `displayName` — 人类可读名称
- `displayDescription` — 人类可读描述
- `version` — 最新版本号
- `relevanceScore` — 向量相关性分数
- `source` — 来源：`DingtalkMarket`（钉钉市场）/ `OrgInternal`（企业内部）
- `securityStatus` — 安全检测状态：`passed`（通过）/ `failed`（未通过）/ `checking`（检测中）

安全提示: 安全检测未通过的技能会标注 ⚠️ 警告，不建议安装。

前置: 已登录钉钉（未登录会由系统自动触发授权；可用 `dws auth status` 确认）（调用技能市场接口需 access token）。

兼容提示: `dws skill find` 会提示改用 `dws skill search --query <关键词>`。

### 安装技能

```
Usage:
  dws skill install <skillId> <target> [flags]
Example:
  dws skill install skill-123 claude     # 安装到 ~/.claude/skills/
  dws skill install skill-123 cursor     # 安装到 ~/.cursor/skills/
  dws skill install skill-123 codex      # 安装到 ~/.codex/skills/
  dws skill install skill-123 .          # 安装到当前目录
Args:
  <skillId>   技能 ID（必填，从 search 结果获取）
  <target>    目标 Agent：claude / cursor / codex / qoder / opencode 或 . 表示当前目录
```

流程: 下载技能包 → 解压 → 调用 real-cli 注册到悟空 SkillStore。

安全拦截: 安全检测未通过的技能默认拒绝安装，使用 `--force` 可强制安装。

前置: 已登录钉钉（未登录会由系统自动触发授权；可用 `dws auth status` 确认）；悟空 App 已安装。

兼容提示: `dws skill add` 会提示改用 `dws skill install <skillId> <target>`（位置参数）。

环境: 技能 API 默认 `https://mcp.dingtalk.com`；可通过 `DWS_SKILL_API_HOST` 覆盖。

## 意图判断

- 用户说"开发文档/API 文档/接口文档/调用报错" → `devdoc article search`
- 用户说"直播/我的直播" → `live stream list`
- 用户说"搜索技能/找技能/安装技能/技能市场" → `skill search` / `skill install`（按步骤衔接）

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `devdoc article search` | 文档链接 | 直接展示给用户 |
| `skill search` | `skillId`、名称、描述 | 用户选型后传给 `skill install <skillId> <target>` |
| `skill install` | 安装成功/失败信息 | 确认目标 Agent 目录已注册 |
| `skill publish` | 发布结果（成功或错误信息） | 确认企业技能库已更新 |

## 相关产品

- `dingtalk-calendar` (`references/calendar.md`) — 日历日程管理（含参与者/会议室）
- 视频会议（conference）— 当前 CLI 不提供对应 skill；发起/预约/邀请入会/会中控制请在钉钉客户端操作
