<!--
source: https://alidocs.dingtalk.com/i/nodes/MyQA2dXW7oOA63YacZgBYPbmWzlwrZgb?utm_scene=person_space
nodeId: MyQA2dXW7oOA63YacZgBYPbmWzlwrZgb
title: 方案补充
logId: 0bab027317824452412095066e094d
note: temporary signed image URLs were redacted before local persistence.
-->

本文只补《dws 多组织 CLI 技术方案》中“多组织登录”落地时需要防漏的部分，不展开权限授权、额外命令扩展或跨组织业务能力。

本页目标很窄：让同一个自然人可以在本机 DWS 登录多个钉钉组织，并能稳定选择当前组织运行。

### 1\. 本页边界

只做：

| 范围 | 说明 |
|------|------|
| 多组织登录 | 同一用户可重复 `dws auth login`，每次登录一个组织 profile。 |
| 多槽 token 存储 | 每个组织一份独立 token slot，避免互相覆盖。 |
| 当前组织上下文 | 用 `currentProfile/primaryProfile/previousProfile` 管理当前组织。 |
| profile 切换 | 支持 `profile list`、`profile use &lt;name&gt;`、`profile use -`。 |
| 单次命令临时指定组织 | 支持全局 `--profile &lt;name&gt;`，只影响本次命令。 |
| 旧版本兼容 | 保留 legacy `auth-token` 镜像，避免旧 binary 或宿主直接失效。 |

不做：

| 不做项 | 原因 |
|---------|------|
| 权限授权链路 | 与“登录多个组织”不是同一层问题，本页不展开。 |
| 额外命令扩展 | 不是完成多组织登录的必要条件。 |
| 跨组织业务处理 | 多组织登录只提供 profile 原语；业务层能力另行设计。 |
| 自动发现用户所有所属组织 | 只展示用户已经主动登录过的组织 profile。 |

### 2\. 最小登录链路

多组织登录链路保持简单：
1. 用户执行 `dws auth login`。
2. CLI 走现有 OAuth loopback 或 `--device` 设备流。
3. 登录成功后拿到 `TokenData`，其中包含 `CorpID / CorpName / UserID / UserName / ClientID / RefreshToken / ExpiresAt / RefreshExpAt`。
4. CLI 用 `CorpID` 判断这是新组织还是已有组织。
5. 新组织：写入新的 token slot，并新增 profile 元数据。
6. 已有组织：刷新该 profile 的 token 和元数据，不新增重复 profile。
7. 首次登录的组织成为 `primaryProfile`。
8. 最近一次登录成功的组织成为 `currentProfile`。

### 3\. 本地数据模型

目标数据仍分两层：token 进安全存储，profile 元数据进明文配置。

| 数据 | 目标位置 | 内容 |
|------|------------|------|
| 用户 token | Keychain `auth-token:&lt;corpId&gt;` | 当前组织的完整 `TokenData`。 |
| profile 元数据 | `profiles.json` | profile 名、corpId、corpName、userId、userName、状态和时间戳。 |
| 当前组织 | `profiles.json.currentProfile` | 当前默认使用的 profile。 |
| 主组织 | `profiles.json.primaryProfile` | 首次登录的 profile，用于兜底和保护。 |
| 上一个组织 | `profiles.json.previousProfile` | 支持 `profile use -`。 |
| legacy 兼容槽 | Keychain `auth-token` | 镜像当前可用 profile，给旧逻辑兜底。 |
| marker | `token.json` | 只标记有登录态，不存 token。 |

建议 `profiles.json` 只保存非敏感字段：
```json
{
  "version": 1,
  "primaryProfile": "ding-a",
  "currentProfile": "ding-b",
  "previousProfile": "ding-a",
  "profiles": [
    {
      "name": "org-a",
      "corpId": "ding-a",
      "corpName": "A 组织",
      "userId": "user-a",
      "userName": "张三",
      "status": "active",
      "lastLoginAt": "2026-06-25T12:00:00+08:00",
      "lastUsedAt": "2026-06-25T12:00:00+08:00"
    }
  ]
}
```

### 4\. 当前 profile 解析规则

每次命令运行前只做一件事：解析本次应该使用哪个 profile。

优先级：
```text
--profile flag > currentProfile > primaryProfile > legacy auth-token
```

规则：
- `--profile <name>` 只影响本次命令，不写 `currentProfile`。
- `profile use <name>` 才持久修改 `currentProfile`。
- `profile use -` 使用 `previousProfile` 做切回。
- `auth login` 登录新组织后，可以把新组织设为 `currentProfile`。
- 如果 `currentProfile` 不可用，回退到 `primaryProfile`。
- 如果 `profiles.json` 不存在但 legacy `auth-token` 存在，执行一次迁移初始化。

### 5\. 命令行为补充

主技术方案已经定义了命令名，本页只补行为边界。

| 命令 | 行为边界 |
|------|------------|
| `dws auth login` | 登录一个组织；新组织新增 profile，老组织刷新 profile。 |
| `dws auth status` | 查看当前 profile 状态。 |
| `dws auth status --profile &lt;name&gt;` | 查看指定 profile，不改变当前 profile。 |
| `dws auth logout` | 默认退出当前 profile。 |
| `dws auth logout --profile &lt;name&gt;` | 只退出指定 profile，不影响其他组织。 |
| `dws profile list` | 只读 `profiles.json` 渲染列表，不解密所有 token。 |
| `dws profile use &lt;name&gt;` | 切换当前 profile，更新 `previousProfile`。 |
| `dws profile use -` | 切回上一个 profile。 |
| 全局 `--profile &lt;name&gt;` | 单次覆盖 profile，不持久化。 |

### 6\. 实现防漏点

这些是完成多组织登录主链路时必须检查的点。

| 点位 | 要求 |
|------|------|
| token 保存 | 不能再覆盖固定 `auth-token`；必须写到 `auth-token:&lt;corpId&gt;`。 |
| token 读取 | 先解析 profile，再按 corpId 读取对应 slot。 |
| token 刷新 | 只刷新当前 profile 的 token slot。 |
| 并发刷新 | 同一个 corpId 刷新需要加锁，避免 refresh token 覆盖。 |
| runtime token cache | 不能继续是全局单值；必须按 profile/corpId 隔离，或命令级绑定。 |
| plugin UserContext | 注入 `UserID/CorpID` 前必须先完成 profile resolution。 |
| legacy mirror | 每次 current profile 变化或登录成功后，更新 legacy `auth-token` 镜像。 |
| `token.json` | 继续只做 marker，不写入组织列表或 token。 |
| `auth reset` | 清理所有 profile 元数据、所有 `auth-token:&lt;corpId&gt;` slot、legacy slot 和 marker。 |

### 7\. 最小验收用例

只验收多组织登录本身。

| 用例 | 期望结果 |
|------|------------|
| 首次 `auth login` | 创建 `primaryProfile=currentProfile`，写入一个 `auth-token:&lt;corpId&gt;`。 |
| 第二个组织 `auth login` | 新增第二个 profile，不覆盖第一个组织 token。 |
| 同组织重复 `auth login` | 刷新该组织 token，不新增重复 profile。 |
| `profile list` | 能看到所有已登录组织，且 current/primary 标记正确。 |
| `profile use B` | 当前组织切到 B，previous 记录切换前的 A。 |
| `profile use -` | 当前组织切回 A。 |
| `--profile B` 执行业务命令 | 使用 B 的 token，但 currentProfile 仍保持原值。 |
| A/B 两个 profile 连续执行命令 | token、`UserID/CorpID`、运行结果不串组织。 |
| access token 过期 | 只刷新当前 profile 的 token slot。 |
| refresh token 过期 | 当前 profile 标记为 expired，提示重新登录该组织。 |
| `auth logout --profile B` | 删除 B 的 token slot 和 profile 元数据，不影响 A。 |
| legacy 单槽迁移 | 旧 `auth-token` 被初始化为一个 primary/current profile。 |

### 8\. 结论

本补充页只要求把多组织登录闭环做完整：
```text
多次登录组织 -> 多槽保存 token -> profiles.json 管当前组织 -> 运行时按 profile 取 token -> profile 切换和临时覆盖不串组织
```

权限授权、额外命令扩展、跨组织业务处理都不放进本补充页。
