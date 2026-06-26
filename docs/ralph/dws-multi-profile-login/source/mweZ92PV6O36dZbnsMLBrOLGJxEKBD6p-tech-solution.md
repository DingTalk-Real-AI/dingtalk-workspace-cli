<!--
source: https://alidocs.dingtalk.com/i/nodes/mweZ92PV6O36dZbnsMLBrOLGJxEKBD6p?utm_scene=person_space
nodeId: mweZ92PV6O36dZbnsMLBrOLGJxEKBD6p
title: dws 多组织 CLI 技术方案
logId: 213ee25c17824452404885344e08e2
note: temporary signed image URLs were redacted before local persistence.
-->

## 一、设计前提：现状已具备的基础

dws 现有凭证体系已为多组织准备好大半，缺口集中在「单槽 → 多槽 \+ 当前上下文指针」。
- token 本身已是组织绑定。`TokenData` 已含 `CorpID / CorpName / UserID / UserName / ClientID / RefreshToken / ExpiresAt / RefreshExpAt`（internal/auth/token.go）。一份 token = 一个组织 = 一个 profile。
- 当前为单槽 keychain。service 为 `dws-cli`、account 固定 `auth-token`（internal/keychain/keychain.go）。一次只存一份，这是「只支持单组织」的真正卡点。
- 已有键控多槽先例。client secret 用 `client-secret:<clientID>` 多槽存（internal/auth/keychain\_store.go）。多 profile 直接复刻同一模式，不引入新机制。
- 存储已分层。keychain 存密文（macOS 系统钥匙串存 DEK 加 AES-256-GCM 密文；Linux 文件 DEK；Windows DPAPI）；`token.json` 仅为宿主探测用的标记文件；`identity.json` 只存 agentId，不含组织信息。
- 运行时注入点单一。启动时 `LoadTokenData` 一次，取 `UserID/CorpID` 注入（internal/app/root.go），`$corpId/$unionId` 运行时默认值从 token 解析。这是切换 profile 唯一要改的地方。
- real/dev 已分流。real 模式由宿主 hook 接管 Save/Load/DeleteToken 并隐藏 login；dev 模式走 keychain 加 OAuth。

## 二、与飞书 CLI 的对照（命名取向依据）

完全沿用飞书的命名与拆分：`auth` 管凭证动作（token），`profile` 管选哪个身份。在 dws 语境里，一个 profile 就是一个已登录组织（corp）。逐项对照：
- 概念：飞书一个 profile = 一个 app/租户身份；dws 一个 profile = 一个已登录组织（corp）。
- 多身份存储：飞书用 MultiAppConfig 的 Apps 数组（明文）加 keychain 密钥分离；dws 用 profiles.json 明文元数据加 keychain 多槽密文，同样分离。
- 再次登录：飞书再跑一次 `auth login` 即新增身份；dws 再跑一次 `auth login` 即新增 profile，无需专门 flag。
- 列表：飞书 `profile list`；dws `profile list`。
- 持久切换：飞书 `profile use <name>`（改 CurrentApp，原值存 PreviousApp）；dws `profile use <name>`（改 currentProfile，原值存 previousProfile）。
- 切回上一个：飞书 `profile use -`；dws `profile use -`。
- 一次性切换：飞书全局 `--profile <name>`；dws 全局 `--profile <name>`，一次性、不持久化。
- 状态：飞书 `auth status`；dws `auth status`。
- 业务域授权：飞书 `--domain calendar,task` 加 `--recommend`；dws `--domain calendar,im,doc` 加 `--recommend`。
- 登录方式：飞书 device flow 加 `--no-wait`；dws 沿用现有 loopback 加 `--device`。

profile 名默认取 corpName（可重复时回退带 corpId 后缀）；底层稳定键始终是 corpId，profile 名只作展示与选择用，等同飞书「profile 名作选择、appId 作稳定键」。

## 三、指令命名与参数（最终）

与飞书一致：`auth` 组加 `profile` 组加全局 `--profile`。

### 3.1 dws auth（凭证动作）
- `dws auth login [--device] [--force] [--domain <d1,d2>] [--recommend]`
    - 登录一个组织（profile）。首次登录 = 主 profile（primary）；之后对新组织 login 即新增一个 profile 并设为当前；对同组织重复 login = 刷新。
    - 组织身份由 OAuth 授权账号决定，CLI 自动从返回取 corpId/corpName，无需手填。
    - `--domain` 指定本次授权的业务域（日程/消息/文档/待办/通讯录等），`--recommend` 仅请求默认推荐域。对齐飞书。
- `dws auth logout [--profile <name>] [--all]`
    - 退出登录。默认退当前 profile（二次确认）；`--profile` 指定某个；`--all` 退出所有非主 profile。主 profile 不可退出。
    - 退出 = 删该 corp 的 keychain 槽，加远端 revoke，加从 profiles.json 移除。
- `dws auth status [--profile <name>]`
    - 查看当前（或指定）profile 的认证状态，含 refresh token 有效性、自动刷新。
- 兼容保留：`auth export / import / reset` 维持现状，按当前 profile 操作。

### 3.2 dws profile（身份/组织管理，命名同飞书）
- `dws profile list`（别名 `ls`） 
    - 列出已登录 profile：主 profile 标记、当前标记、授权业务域、状态（已授权/已过期/已失效）、有效期。
    - 仅展示已登录 profile，不拉取「用户全部组织列表」。靠 profiles.json 渲染，不解密 keychain。
- `dws profile use <name>`
    - 切换当前 profile 并持久化：写 currentProfile，原值存 previousProfile。
    - 参数可为 profile 名或 corpId；无参时给 TUI 列表选择。
- `dws profile use -`
    - 切回上一个 profile（用 previousProfile 做 toggle），对标飞书 `profile use -`。

### 3.3 全局 flag：跨组织取数
- 全局 `--profile <name>`
    - 任意取数指令加此 flag，单次从指定 profile 取数，一次性、不改 currentProfile。等价 PRD 的「--组织corp ID」，命名对标飞书 `--profile`。
    - 值可为 profile 名或 corpId。
- 不提供 `--all-orgs` 内置聚合。跨组织聚合由 agent 编排（见第五节）。

### 3.4 与 PRD 草案的差异
- 组织管理用独立 `profile` 组：与飞书 `auth` 加 `profile` 的拆分与命名完全一致。
- 去掉 `login --associated`：飞书风格下 login 本身就是新增，无需该 flag。
- `logout --associated` 改为 `logout --profile / --all`。
- 切换命令用 `profile use`（含 `-` 切回），跨组织一次性取数用全局 `--profile`，替代草案的 `--corp`。
- 去掉未登录组织引导：本期只展示已登录 profile。

## 四、凭证管理技术方案（核心）

### 4.1 存储模型：单槽 → 键控多槽（向前兼容）
- keychain 改键控：token account 从固定 `auth-token` 扩为 `auth-token:<corpId>`，复刻现有 `client-secret:<clientID>` 模式。每个 profile 一份独立密文，加密方式（DEK 加 AES-256-GCM）完全不变。corpId 作稳定键（profile 可改名，键不变）。
- 新增明文注册表 profiles.json（放 configDir，不含任何 token，只存元数据加指针），对齐飞书 MultiAppConfig：
```
{
  "primaryProfile":  "corpA",   // 主 profile 的 corpId，不可退出
  "currentProfile":  "corpB",   // 当前 = 上一次 profile use 选中的
  "previousProfile": "corpA",   // 上一个，支持 profile use - 切回
  "profiles": [
    {
      "corpId": "corpA", "name": "A科技",
      "userId": "...", "userName": "...",
      "status": "active",
      "authorizedDomains": ["calendar", "im", "doc"],
      "refreshExpAt": "...", "updatedAt": "..."
    }
  ]
}
```
- config 与 secret 分离：`profile list` 读 profiles.json 即可渲染，不碰 keychain；token 全程只在 keychain。

### 4.2 向前/向后兼容（硬约束）
- 向后兼容：新 CLI 读旧的单槽 `auth-token` 仍可用（见 4.3 优先级末位回退）。首次多 profile 使用时，把旧单槽读出，用其 CorpID 落成 `auth-token:<corpId>` 并标 primary，复用现有 legacy 迁移钩子（keychain\_store.go 的 EnsureMigration），用户无感。
- 向前兼容：新 CLI 始终把「当前 profile（无则主）」的 TokenData 镜像写一份到旧单槽 `auth-token`，并照常写 token.json 标记。这样旧版二进制、real 宿主即使不认识 profiles.json，也能在主 profile 上照常工作。
- profiles.json 为增量文件，旧版忽略，不破坏旧逻辑。

### 4.3 当前上下文解析（唯一运行时改动，无环境变量）

每次请求按优先级选 token：
```
--profile flag  >  currentProfile  >  primaryProfile  >  旧单槽 auth-token（兼容回退）
```
- `--profile` 一次性、不写回 currentProfile；只有 `profile use` 才更新 currentProfile/previousProfile。
- 落点为 `LoadTokenData` 与 root.go 注入处：把「加载固定槽」改为「解析出 corpId 再加载对应槽」。改动面集中、极小。

### 4.4 real / dev 双模（本期边界）
- dev（独立 CLI，本期落地）：CLI 自跑 OAuth（loopback/device），每个 profile 的 TokenData 写进 `auth-token:<corpId>`，更新 profiles.json。多 profile 逻辑仅在非嵌入（!IsEmbedded）时启用。
- real（嵌悟空，本期不动）：edition 的 Save/Load/DeleteToken hook 签名保持不变；token 仍由宿主单组织颁发/刷新，行为与现状完全一致。多 profile 能力本期不在 real 暴露。后续如需 real 多组织，再单独评审 hook 协议扩展，且必须向前兼容。

### 4.5 刷新与安全底线
- 刷新按槽独立：每个 TokenData 自带 refresh\_token 加 clientID，GetAccessToken 只刷被选中的那一槽，互不影响。
- 权限跟人走、CLI 不放大：每个 profile 独立 OAuth 独立 consent，token 数据范围 = 用户在该组织真实可见范围。
- 业务域粒度：授权域记在 profiles.json 的 authorizedDomains；撤销单个域保留其他域 token，退出整个 profile 才清该 corp 的 keychain 槽。

## 五、运行时数据获取与聚合（重点）

CLI 只提供底层原语，跨组织聚合由 agent（Claude Code）编排，符合「脚本只取数、触发/判断/编排在 agent 原生跑」的产品铁律。
- 底层原语：CLI 提供「单 profile 一次取数」，默认走 currentProfile，或带 `--profile` 指定。
- 聚合流程（agent 编排）： 
    1. `dws profile list` 拿到已授权 profile 加各自授权域。
    2. 对每个满足条件的 profile 带 `--profile <name>` 调一次取数。
    3. 合并结果并标注来源组织，按业务域自然序排列。
- 部分失败降级：某 profile 调用失败时，agent 正常返回已成功的，对失败的标注「暂不可用，可稍后重试」。
- 不内置 `--all-orgs`：保持泛化性，编排逻辑留在 agent，遇到未预设情况能现场判断。

## 六、边界与异常（对齐 PRD 第六节）
- 凭证过期：该 profile 槽刷新失败，profiles.json 标 expired，下次取数提示重登并给快捷入口。
- 被移出组织：refresh 返回失效，标 revoked，清该 corp 槽与缓存，从 list 移除。
- 部分失败：见第五节降级。
- 主 profile 保护：`auth logout --all` 与 `profile use` 均不动 primaryProfile；主 profile 不可退出。

## 七、落地改动点（文件级，仅 dev 路径）
- internal/keychain、internal/auth/keychain\_store.go：token account 扩为 `auth-token:<corpId>`，新增按 corp 的 Save/Load/Delete/List；保留旧单槽镜像写。
- internal/auth/token.go：LoadTokenData/SaveTokenData/DeleteTokenData 内部按解析出的 corpId 选槽；edition hook 签名不变（real 路径原样）。
- 新增 internal/auth/profiles.go：profiles.json 读写，加 currentProfile/previousProfile 解析，加优先级链。
- internal/app/auth\_command.go：`logout` 加 `--profile/--all`，`status` 加 `--profile`，`login` 加 `--domain/--recommend`。
- 新增 internal/app/profile\_command.go：`dws profile list / use`（含 `use -` 切回）。
- internal/app/root.go：注入处改为「解析当前 profile 后加载对应槽」；注册全局 `--profile`（仅 !IsEmbedded 生效）。

## 八、待确认 / 后续
- real 模式多组织：本期不做，留作后续独立评审，扩展时 hook 协议须向前兼容。
- 业务域到 scope 映射表：`--domain` 落地需要一份 dws 业务域到钉钉 OAuth scope 的映射（可参考飞书 domain 到 scope 注册表的做法）。
