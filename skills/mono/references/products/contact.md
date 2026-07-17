# 通讯录 (contact) 命令参考

> **CRITICAL — 命令合法性**：contact 二级子命令包括 `user` / `dept` / `label` / `relation` / `org` / `account`。
> 不存在 `contact search`、`contact find`、`contact list`、`contact get`、`contact user find/list`。
> 构造命令前必须确认路径在下方「命令总览」中存在；不确定时，**根据意图对照下方「意图判断」选择正确命令**。
>
> **CRITICAL — 创建企业 vs 创建企业账号（必须优先匹配长模式）**：
> - 用户说"创建企业**账号** / 新建企业**账号** / 开通企业**账号** / **专属账号** / **企业登录账号**" → **`account create`**（创建企业专属登录账号）
> - 用户说"创建企业 / 新建企业 / 开通企业 / 初始化企业"（**不含"账号"二字**）→ **`org create`**（创建企业组织本身）
> - **判断口径**：先检查 query 是否含"账号"关键词；含则必须路由 `account create`，**禁止**路由 `org create`。
>
> **CRITICAL — 根部门**：钉钉根部门 `deptId=1`。`dept` 系列命令查根部门统一传 `--dept 1` 或 `--depts 1`，不要传 `self / me / root / 0`。
>
> **CRITICAL — 搜人首选 aisearch**：凡是"找人/搜人/谁负责 XX/某事项/某项目的人/上级/下级/团队成员"——**第一反应**是 `dws aisearch person`（详见 [aisearch.md](./aisearch.md)），不是 `contact user search`，**更不是反问用户要文档链接**。典型反例：用户说"查询集团推进事项"= 问"集团推进事项这个职责/项目下的人是谁"，正确做法是 `dws aisearch person --keyword "集团推进事项" --dimension duty --format json`。

## 命令总览

### user (人员查询)

#### 获取当前用户信息
```
Usage:
  dws contact user get-self [flags]
Aliases:
  get-self, self, me, whoami, current
Example:
  dws contact user get-self
  dws contact user self       # 别名
  dws contact user me         # 别名
  dws contact user whoami     # 别名
  dws contact user current    # 别名
Notes:
  - 触发词：我是谁 / 我的信息 / 我的 userId / 当前用户 / 本人 / self / me / whoami
  - 顶层亦已挂 `dws contact get-self / user-self / current-user` 提示，误写会引导到正确命令
  - **禁止**用 `dws contact user get --ids me/self/current` 代替（会报错）；正确用法是 `get-self` 或其别名
```

#### 按关键词搜索用户
```
Usage:
  dws contact user search [flags]
Example:
  dws contact user search --query "张三"
Flags:
      --query string   搜索关键词 (必填)
Returns: (顶层 `result` 为列表，每项包含以下字段)
  name             string   成员姓名
  nick             string   成员昵称
  flowerName       string   花名（无花名时为 null）
  userId           string   成员 ID（仅同事关系时返回）
  title            string   员工职位（仅同事关系时返回）
  openDingTalkId   string   当前用户视角下的目标用户唯一标识，不可跨用户共享；可用于发消息等好友关系场景的操作
```

> **CAUTION:** 多人同名时禁止默认选第一个 — `user search` 不返回部门信息，须追加 `contact user get --ids userId1,userId2,...` 获取部门/职位后请用户确认。详见 [08-directory.md](../best_practices/08-directory.md)「多命中」。

#### 按手机号搜索用户
```
Usage:
  dws contact user search-mobile [flags]
Example:
  dws contact user search-mobile --mobile 13800138000
Flags:
      --mobile string   手机号 (必填)
```

#### 批量获取用户详情
```
Usage:
  dws contact user get [flags]
Example:
  dws contact user get --ids userId1,userId2
Flags:
      --ids string   用户 ID 列表，逗号分隔 (必填)
Notes:
  - **禁止**将 `self/me/current/whoami` 作为 userId 传入；查自己请用 `dws contact user get-self`
```

#### 邀请员工加入企业
```
Usage:
  dws contact user invite [flags]
Example:
  dws contact user invite --org-user-name "张三" --org-user-mobile "13800138000" --depts '[{"deptId":1}]'
Flags:
      --org-user-name string    员工在企业内的名称 (必填)
      --org-user-mobile string  员工手机号 (必填)
      --depts string            员工所属部门列表 JSON 数组（可选），格式: [{"deptId":1}]
Notes:
  - 通过手机号邀请单个员工加入当前企业
  - 认证信息（corpId、optUserId）由系统自动注入，无需手动传入
```

### profile (用户档案 / 花名册)

#### 查询花名册有权限的字段列表
```
Usage:
  dws contact user profile fields
Example:
  dws contact user profile fields
Flags:
  无
```

查询花名册有权限的字段列表，根据当前用户查询花名册有权限的字段列表。认证信息（corpId、optUserId）由系统自动注入，无需手动传入。

> **前置条件：需操作人具备花名册管理权限。** 无权限时返回业务错误 `操作人无花名册管理权限`（非命令写法问题，不要改参数）。

#### 查询员工花名册字段信息（个人档案）
```
Usage:
  dws contact user profile get [flags]
Example:
  dws contact user profile get --staff-id STAFF_ID
  dws contact user profile get --staff-id STAFF_ID --fields fieldCode1,fieldCode2
Flags:
      --staff-id string  查询员工 ID（可选）
      --fields string    指定字段集合, 逗号分隔, 可通过 profile fields 获取（可选）
```

查询员工花名册字段信息，根据当前用户指定员工和字段列表，查询相应管理范围内员工的字段值信息。
花名册字段包含：试用/转正信息、个人/家庭信息、学历信息、银行卡/合同信息、紧急联系人和其他企业自定义信息。

> **与 `contact user get` 的区别**：`user get` 返回组织管理信息（部门、主管、管理员权限），`user profile get` 返回个人档案信息（学历、家庭、银行卡等）。
>
> **前置条件：需操作人具备花名册管理权限。** 无权限时返回业务错误 `操作人无花名册管理权限`（非命令写法问题，不要改参数）。

### dismission (离职员工)

#### 分页获取离职员工列表
```
Usage:
  dws contact user dismission search [flags]
Example:
  dws contact user dismission search
  dws contact user dismission search --name "张三"
  dws contact user dismission search --start 2026-01-01 --end 2026-03-31
  dws contact user dismission search --depts 123456,789012 --page 1 --limit 50
Flags:
      --name string      员工姓名，模糊搜索（可选）
      --start string     离职日期查询范围开始，格式 YYYY-MM-DD（可选）
      --end string       离职日期查询范围结束，格式 YYYY-MM-DD（可选）
      --depts string     部门 ID 列表，逗号分隔（可选）
      --hide-retirement  是否隐藏退休，默认 true（可选）
      --hide-partner     是否隐藏合作伙伴，默认 false（可选）
      --page int         页码，从 1 开始（可选，默认 1）
      --limit int        页大小，200 以内（可选，默认 20）
```

查询离职员工列表，支持按员工姓名、离职日期范围、部门进行过滤。认证信息（corpId、optUserId）由系统自动注入，无需手动传入。
`--start` 和 `--end` 必须同时设置或同时不设置，不允许只传其中一个。

> **前置条件：需操作人具备已离职人员花名册管理权限。** 无权限时返回业务错误 `操作人无已离职人员花名册管理权限`（非命令写法问题，不要改参数）。

### dept (部门查询)

#### 搜索部门
```
Usage:
  dws contact dept search [flags]
Example:
  dws contact dept search --query "技术部"
Flags:
      --query string   搜索关键词 (必填)
Returns: (顶层 key 为 `deptList`，非 `result`，为列表，每项包含以下字段)
  deptId           int      部门 ID
  deptName         string   部门名称（命中关键词会用 <red>…</red> 高亮包裹，需自行去标签）
```

#### 获取部门详情
```
Usage:
  dws contact dept get-info [flags]
Example:
  dws contact dept get-info --dept 12345
Flags:
      --dept string   部门 ID (必填)
Notes:
  - **钉钉根部门 `deptId=1`**；查根部门用 `--dept 1`
```

#### 查看子部门
```
Usage:
  dws contact dept list-children [flags]
Example:
  dws contact dept list-children --dept 1           # 枚举根部门下的一级部门
  dws contact dept list-children --dept 12345       # 枚举指定部门的直属子部门
Flags:
      --dept string   父部门 ID (必填)
Returns:
  success          bool     调用是否成功
  result           list     直属子部门列表，每项包含以下字段：
    deptId         int      子部门 ID
    deptName       string   子部门名称
Notes:
  - **钉钉根部门 `deptId=1`**；查询一级部门请用 `--dept 1`
  - 仅返回**直属**（直接下一级）子部门，不递归；需要逐层下钻请对子 deptId 继续调用本命令
  - 受组织架构可见性控制：仅返回调用者**有权限查看**的子部门
  - 父部门不可见或无子部门时返回 result=[] 空列表（非错误）
```

#### 查看部门成员
```
Usage:
  dws contact dept list-members [flags]
Example:
  dws contact dept list-members --depts 12345,67890
  dws contact dept list-members --depts 1              # 根部门
Flags:
      --depts string   部门 ID 列表，逗号分隔 (必填)
Returns: (顶层 key 为 `deptUserList`，非 `result`，为列表，每项形如 { "userInfo": { "name": ..., "userId": ... } })
  userInfo.name    string   成员姓名
  userInfo.userId  string   成员 ID
Notes:
  - **钉钉根部门 `deptId=1`**；查根部门直属成员用 `--depts 1`
  - 仅返回**本部门**直接成员，**不含下级部门**成员；需含下级请先 `dept list-children` 枚举子部门，再对子 deptId 分别/合并调用 `list-members`
  - 受组织架构可见性控制；`--depts` 支持逗号分隔批量查询多个部门
  - 跨层级成员展开见 [08-directory.md](../best_practices/08-directory.md) 的 `cross-level-dept-members` recipe
```

### label (角色查询)

> **角色ID = labelId**：用户提到"角色ID"时，均指通讯录 label 系统中的角色ID，**不是部门ID也不是userId**。查角色成员用 `label list-members --id <角色ID>`，不要传给 `dept get-info` 或 `user get`。

#### 获取企业所有角色列表
```
Usage:
  dws contact label list
Example:
  dws contact label list
Flags:
  无
Notes:
  - 无需参数，返回当前企业全部角色，按 `groupName` 分组，每组 `labels[]` 里每个角色是 `{labelId, name}`（字段名是 `name`，不是 `labelName`）
  - 用于不知道准确角色名称时先浏览全部角色
  - 典型场景：用户说“企业所有主管/查所有管理员/财务人员有哪些”→ 先 label list 浏览全部角色，匹配目标角色后 label list-members 获取成员
```

#### 根据角色名称查询角色
```
Usage:
  dws contact label get [flags]
Example:
  dws contact label get --names "管理员"
  dws contact label get --names "管理员,财务"
Flags:
      --names string   角色名称，逗号分隔 (必填)
Notes:
  - 精确匹配角色名称，不支持模糊搜索
  - 支持同时查询多个角色名称，逗号分隔
  - 无需分页
```

#### 查询角色下的成员
```
Usage:
  dws contact label list-members [flags]
Example:
  dws contact label list-members --id 12345
Flags:
      --id string   角色 ID (必填)
Notes:
  - 根据角色ID直接查询成员列表；已有角色ID时直接用 `--id <labelId>`
  - 不知道角色ID时：先 `dws contact label get --names "角色名"` 或 `dws contact label list` 获取 labelId
```

### org (企业管理)

#### 创建企业
```
Usage:
  dws contact org create [flags]
Example:
  dws contact org create --org-name "我的企业" --creator-username "张三"
Flags:
      --org-name string          企业名称 (必填)
      --creator-username string  创建者在企业内的名称，对应 creatorUsername (必填)
Notes:
  - 创建一个新的钉钉企业，当前用户将成为该企业的创建者
  - 认证信息（corpId、optUserId）由系统自动注入，无需手动传入
```

### account (企业账号管理)

#### 创建企业专属账号
```
Usage:
  dws contact account create [flags]
Example:
  dws contact account create --org-user-name "张三" --login-id "zhangsan001" --org-user-mobile "13800138000" --email "zhangsan@example.com" --dept-ids "1,2,3" --send-pwd-via-sms
Flags:
      --org-user-name string    员工在企业内的名称 (必填)
      --login-id string         登录号 (必填)，请勿包含手机号等联系方式
      --org-user-mobile string  员工手机号（可选）
      --email string            邮箱（可选）
      --dept-ids string         要加入的部门 ID 列表，逗号分隔（可选）
      --send-pwd-via-sms        是否通过手机短信/邮件发送登录邀请（可选，默认 false）
Notes:
  - 为当前企业创建一个专属登录账号
  - 登录号请勿包含手机号，否则可能被运营商拦截短信
  - 认证信息（corpId、optUserId）由系统自动注入，无需手动传入
```

## 意图判断

> **搜人首选 `aisearch person`**：凡是“找人/搜人/找同事/谁负责/上级/下级”均优先用 [aisearch person](./aisearch.md)，以下场景才用 contact。

用户说"我是谁/我的信息/我的 userId/当前用户/本人/self/me/whoami" → `user get-self`（无需参数；禁止用 `user get --ids me/self` 代替）
用户需要 userId 给其他产品使用（发消息/建待办/约日程）→ `user search`（按名字）或 `user search-mobile`（按手机号）
用户说"查用户详情/部门/主管/管理员" → `user get`（需 userId，返回组织管理信息）
用户说"邀请员工/添加员工/加人/新员工入职/拉人进企业" → `user invite`（需手机号 + 企业内名称，部门可选）
用户说"花名册字段/有哪些字段/字段列表" → `user profile fields`
用户说"花名册/员工档案/学历/家庭/银行卡/紧急联系人/合同" → `user profile get`（需 staffId，返回个人档案信息）
用户说"离职员工/离职名单/离职人员/已离职" → `user dismission search`
用户说"创建企业账号/新建企业账号/开通企业账号/专属账号/企业登录账号"（含"账号"）→ `account create`（需员工名称 + 登录号；手机号可选）
用户说"创建企业/新建企业/开通企业/初始化企业"（不含"账号"）→ `org create`（需企业名称 + 创建者名称）
用户说"找部门/哪个部门" → `dept search`
用户说"部门详情/部门信息/部门多少人" → `dept get-info`（返回部门ID、部门名称、部门人数；需 deptId，若只有部门名称需先 `dept search`）
用户说"子部门/下设部门/部门有哪些下级部门/枚举二级部门" → `dept list-children`（需父 deptId；只有部门名先 `dept search`）
用户说"部门有谁/部门成员/人员名单" → `dept list-members`（需 deptId；**仅本部门不含下级**，含下级先 `dept list-children` 再合并查）
用户查询涵盖"角色"（主管/管理员/财务/HR/总经理等任意角色名）→ 统一走 `contact label` 链路，按下方决策树选命令：
- 不知道角色名 / 枚举所有角色 → `label list`
- 已知角色名，查ID或成员 → 先 `label get --names <名>` 拿ID，查成员再调 `label list-members --id <ID>`；**精确匹配无结果时降级 `label list` 模糊匹配**
- 已知角色ID 查成员 → `label list-members --id <ID>`

> [!IMPORTANT]
> **角色查询 3 步决策树**（不依赖字串匹配，按语义判定）：
> 1. 不知道角色名 / 要枚举所有角色（列出企业有哪些角色、每个角色的名称和ID、不确定叫什么角色、负责XX的人有哪些等） → `label list`
> 2. 已知角色名 要查角色ID → `label get --names`
> 3. 已知角色ID 要查成员 → `label list-members --id`
>
> 任何含"角色"一词的查询默认走 `contact label` 链路。**唯一例外**：终点是"某个人是否具备某权限"（如"张三是不是管理员"）→ `user get`。禁止路由到 `user profile fields`（那是花名册字段）、`dept list-members` 筛选、或 OA/chat 模块。

> [!IMPORTANT]
> **角色查人 vs 查某人的角色信息 — 判断口径**：先判断用户的终点是"人"还是"属性"：
> - 终点是**人**（"管理员角色有哪些人""管理员下都有谁""查XX角色的成员"）→ 角色维度查人，**必须**走 `contact label` 链路（`label get`/`label list` → `label list-members`），**禁止**通过 `dept list-members` 筛选 `isAdmin` 等字段替代
> - 终点是**属性**（"张三是不是管理员""查某人的主管/管理员权限"）→ 已知 userId 查个人详情，走 `user get`（返回 isAdmin/leader 等字段）
>
> 反例对照：
> - "管理员角色下都有哪些人" → `label get --names 管理员` → `label list-members`（终点=角色下的**人员列表**）
> - "我想知道管理员这个角色下都有谁" → `label get --names 管理员` → `label list-members`（终点=角色下的**人员列表**）
> - "张三是不是管理员" → `user get --ids <userId>`（终点=某个人的**属性**）
> - "查一下张三的管理员权限" → `user get --ids <userId>`（终点=某个人的**属性**）
> - "角色ID为55808858的角色下有哪些成员" → `label list-members --id 55808858`（已有角色ID直接查成员）
>
> **角色 = 通讯录 label**：用户提到"角色"（角色ID/角色成员/角色名称/企业角色/查角色下的人）时，均指通讯录组织角色，应走 `contact label` 链路。OA 审批只管审批流程（待审批/同意/拒绝），**不支持**查询角色成员；群角色(chat group-role)只管群内身份，不涉及企业组织角色。
用户说"我关注了谁/我的特别关注列表/我的星标联系人/特别关注的人有哪些" → `relation list-my-followings`

> [!IMPORTANT]
> **易混淆硬规则**：`relation list-my-followings` **只**返回"我特别关注的人员列表"（一组 openDingTalkId），**不**返回任何消息内容。
>
> **禁止路由到本命令的场景**（query 中同时包含『关注/特别关注/星标』和以下任一消息域动词/名词时，必须路由到 [`chat message list-focused`](./chat.md)）：
> - 动词类：**发**了什么、**说**了什么/啥、**聊**了什么、**讲**了什么
> - 名词类：**消息**、**聊天**、**动态**、**最新内容**
>
> **判断口径**：先扫描 query 是否含上述动词/名词；含则路由到 `chat message list-focused`，**不论** query 主语是否为"我特别关注的人"。
>
> 反例对照：
> - "我特别关注的人有哪些" → `relation list-my-followings`（终点=人员列表）
> - "我特别关注的人**最近发了什么消息**" → `chat message list-focused`（含"发""消息"）
> - "我关注的人**最近都说了啥**" → `chat message list-focused`（含"说"）

组合场景（多子部门、跨层级成员、强消歧）见 [08-directory.md](../best_practices/08-directory.md)。

## 核心工作流

```bash
# 1. 查看自己的信息 — 提取 userId
dws contact user get-self --format json

# 2. 按名字搜索同事或好友 — 可提取 同事的userId，或好友的openDingTalkId
dws contact user search --query "张三" --format json

# 3. 查看部门结构 — 提取 deptId
dws contact dept search --query "技术部" --format json

# 4. 查看部门详情（部门ID、名称、人数）
dws contact dept get-info --dept <deptId> --format json

# 5. 查看直属子部门 — 提取子 deptId 列表
dws contact dept list-children --dept <父deptId> --format json

# 6. 查看部门成员
dws contact dept list-members --depts <deptId> --format json

# 7. 获取企业所有角色列表 — 不知道角色名时先浏览
dws contact label list --format json

# 8. 根据角色名称查询角色
dws contact label get --names "管理员" --format json

# 9. 查询角色下的成员
dws contact label list-members --id <labelId> --format json

# 10. 查询花名册有权限的字段列表
dws contact user profile fields --format json

# 11. 根据字段 code 查询指定员工的花名册信息
dws contact user profile get --staff-id <STAFF_ID> --fields fieldCode1,fieldCode2 --format json

# 12. 查询所有可见字段的花名册信息
dws contact user profile get --staff-id <STAFF_ID> --format json

# 13. 查询全部离职员工
dws contact user dismission search --format json

# 14. 按姓名/时间范围/部门筛选离职员工
dws contact user dismission search --name "张三" --format json
dws contact user dismission search --start 2026-01-01 --end 2026-03-31 --format json
dws contact user dismission search --depts 123456,789012 --hide-retirement=false --format json

# 15. 创建企业
dws contact org create --org-name "我的企业" --creator-username "张三" --format json

# 16. 创建企业专属账号
dws contact account create --org-user-name "张三" --login-id "zhangsan001" --format json

# 17. 邀请员工加入企业
dws contact user invite --org-user-name "张三" --org-user-mobile "13800138000" --depts '[{"deptId":1}]' --format json
```

## 上下文传递表

| 操作 | 提取 | 用于 |
|------|------|------|
| `user get-self/search` | `userId` | 其他产品中的 --users/--executor 参数 |
| `user get-self/search` | `orgAuthEmail` | mail message send 的 --to/--cc (跨产品) |
| `user get-self/search` | `userId` | profile get 的 --staff-id |
| `user profile fields` | `fieldCode` | profile get 的 --fields |
| `label list` | `labelId` / `name` | `label get --names` 或 `label list-members --id` |
| `label get` | `labelId` | `label list-members` 的 --id |
| `dept search/list-children` | `deptId` | dept get-info/list-children/list-members 的 --dept/--depts |
| `dept search/list-children` | `deptId` | dismission search 的 --depts |
| `dept search/list-children` | `deptId` | user invite 的 --depts 或 account create 的 --dept-ids |

## 注意事项

- `user get-self` 是获取 userId 的最快方式，其他产品的 --users/--executor 都需要 userId
- `user get --ids` 和 `dept list-members --depts` 都支持批量查询，逗号分隔
- `user get` 返回组织管理信息（部门、主管、管理员权限），`user profile get` 返回个人档案信息（学历、家庭、银行卡等），注意区分
- `user profile get` 的 `--staff-id` 可通过 `user get-self`、`user search` 或 `aisearch person` 获取
- `user profile get` 的 `--fields` 可通过 `user profile fields` 获取可用字段 code 列表；不填则查询所有可见字段
- 建议先执行 `user profile fields` 获取可用字段列表，再根据需要的字段 code 执行 `user profile get`
- `user dismission search` 的 `--start`/`--end` 必须同时设置或同时不设置，不允许只传其中一个
- `user dismission search` 默认隐藏退休人员（`--hide-retirement` 默认 true），默认展示合作伙伴（`--hide-partner` 默认 false）
- `label list` 无需参数，适用于不知道准确角色名称的场景；当用户说“查所有主管/主管理员/财务”等角色类型人员时，优先 `label list` 列出企业全部角色，LLM 灵活匹配目标角色后调用 `label list-members`
- 角色类查询（主管、管理员、财务、HR 等任意角色）优先走 label 链路，而非 dept list-members 或 aisearch person；label 精确命中角色维度，返回完整名单
- `label get` 是精确匹配角色名称，不支持模糊搜索；支持逗号分隔同时查询多个角色名称
- **`label get` 精确匹配无结果时的降级策略**：若 `label get --names "XX"` 返回空结果，必须降级调用 `label list` 获取全部角色列表，从中模糊匹配包含XX关键词的角色（如用户说"管理员"可匹配到"主管理员"和"子管理员"），再对匹配到的角色调用 `label list-members`
- `label list-members` 需要先通过 `label list` 或 `label get` 获取 labelId，再用 --id 查询角色下的成员
- `org create`、`account create`、`user invite` 都是写操作；执行前确认当前 profile 对应的企业以及名称、手机号、登录号和部门信息

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [contact_dept_members.py](../../scripts/contact_dept_members.py) | 按部门名称搜索并列出所有成员 | `python contact_dept_members.py --query "技术部"` |
