# advperm — 高级权限管理

控制 Base 的高级权限总开关，并管理自定义角色（增删改查 + 子角色权限规则）。
适用场景："如何控制谁能看/改 Base 数据"、"开启/关闭高级权限"、"新建/修改/删除角色"、"按字段或行配置权限"。

## 命令一览

| 命令 | 用途 |
|------|------|
| `advperm enable` | 开启 Base 高级权限总开关 |
| `advperm disable` | 关闭 Base 高级权限总开关（高危） |
| `advperm role-list` | 列出 Base 下全部角色 |
| `advperm role-get` | 获取单角色完整配置 |
| `advperm role-create` | 创建自定义角色 |
| `advperm role-update` | 增量更新自定义角色（PATCH 语义） |
| `advperm role-delete` | 删除自定义角色（不可逆） |
| `+role-members-add` | 向自定义角色增量追加成员 |
| `+role-members-update` | 完整替换角色成员并显式保留默认角色 |
| `+role-copy` | 复制自定义角色配置（不复制成员） |
| `+role-query-user` | 查询用户实际命中的高级权限角色 |

> `--base-id` 是 Agent 与 Schema 发布的唯一公开参数；`--base` 仅是历史兼容别名，Skill 和示例不得使用。

## 运行时配置

- 不在命令或 Skill 中硬编码 HSF 地址、版本、uid、orgId、userId、corpId 或 operatorUid；这些可信上下文由 MCP 运行时注入。
- 本文档 4 个角色成员命令（role-members-add/update、role-copy、role-query-user 的原子命令与 Shortcut）直连 `aitable` 主服务 MCP，不经 `aitable-helper` 兼容路由、不触发能力发现；MCP 端点由 `~/.dws/mcp_url`（或 `DWS_CONFIG_DIR/mcp_url`）首行决定。只需完成 `dws auth login` 并使用当前环境的端点配置。
- 新增工具对应 MCP 工具名为 `add_role_members`、`update_role_members`、`copy_role`、`query_user_roles`；CLI 已封装为下文命令，不需要手填 HSF interfaceName、version 或 DTO 映射。

## 命令详情

### advperm enable — 开启高级权限

```bash
dws aitable advperm enable --base-id BASE_ID --format json
```

返回 `{baseId, enabled: true}`。

只有开启后角色配置才会真正限制成员的可访问范围；关闭状态下角色配置仍可读但不生效。

### advperm disable — 关闭高级权限（高危）

```bash
dws aitable advperm disable --base-id BASE_ID --yes --format json
```

返回 `{baseId, enabled: false}`。关闭后所有角色配置即刻失效，全员回退到默认权限。涉及多人协作或敏感数据务必和用户二次确认，建议先 `role-list` 留底。

### advperm role-list — 列出全部角色

```bash
dws aitable advperm role-list --base-id BASE_ID --format json
```

返回结构：

```json
{
  "data": {
    "enabled": true,
    "defaultRole": { "mode": 0 },
    "roles": [
      {
        "roleId": "10685308981",
        "name": "可查看角色",
        "roleType": "custom",
        "system": false,
        "subRoles": [
          {
            "authLevel": "read",
            "targetId": "HMEaRQ4",
            "targetType": "sheet",
            "config": { "actions": 268435455 },
            "display": {
              "authLevelLabel": "仅查看",
              "targetTypeLabel": "数据表",
              "permissionScopeNote": "...",
              "actionsLabels": ["新增视图", "删除视图", "修改视图"],
              "actionsNote": "..."
            }
          }
        ]
      }
    ]
  }
}
```

关键字段：

- `roleType`：`custom`（自定义） / `system_editor` / `system_reader` / `5000`（owner） / `4000`（manager）。
- `system`：boolean，true 表示系统角色（不可删）。
- `subRoles[].display.*`：服务端返回的人类可读标签，可直接拼接给用户阅读，无需自行映射枚举。
- `roles[].members` 返回该角色当前成员；配置回读成功不等于成员实际权限已验证，仍需对应身份执行真实访问测试。
- 新建 Base 默认 `enabled=false`，开启后只有 `owner` / `manager` 两个 meta 角色；`system_editor` / `system_reader` 需要在 Web UI 给成员授权"可编辑/可查看"后才会被服务端自动生成。

`role-list` / `role-get` 不需要管理员权限，普通成员也可读。

### advperm role-get — 获取单角色配置

```bash
dws aitable advperm role-get --base-id BASE_ID --role-id ROLE_ID --format json
```

返回结构同 `role-list` 中单个 role 对象（含完整 `subRoles[].config` 字段/行级规则与 `display.*` 标签）。

### advperm role-create — 创建自定义角色

```bash
# 仅指定 name，子角色由服务端按默认（none）填充
dws aitable advperm role-create --base-id BASE_ID --name "市场可读" --format json

# 创建时即指定 sub-roles（推荐——避免再走一次 role-update）
dws aitable advperm role-create --base-id BASE_ID --name "市场可读" \
  --sub-roles '[{"targetId":"<sheetId>","targetType":"sheet","authLevel":"read"}]' --format json
```

| flag | 必填 | 说明 |
|------|:---:|------|
| `--name` | ✅ | 角色名称 |
| `--role-type` | | 角色类型字符串（留空由服务端决定默认值，如 `custom`） |
| `--flow-type` | | 流程类型字符串（按业务需要） |
| `--sub-roles` | | JSON 数组：`[{targetId, targetType, authLevel, appId?, config?}]`，详见下方"sub-roles 子字段"段 |

返回新建角色的完整配置（同 `role-get` 出参格式，含自动生成的 default subRoles）。
系统角色无法通过本命令创建。

### advperm role-update — 增量更新自定义角色（PATCH 语义）

```bash
# 只改名
dws aitable advperm role-update --base-id BASE_ID --role-id ROLE_ID --name "新名字"

# 只改 sheet 子角色 authLevel，name 不传保持不变
dws aitable advperm role-update --base-id BASE_ID --role-id ROLE_ID \
  --sub-roles '[{"targetId":"<sheetId>","targetType":"sheet","authLevel":"edit-own"}]'
```

| flag | 必填 | 说明 |
|------|:---:|------|
| `--role-id` | ✅ | 目标自定义角色 ID（数字 long 字符串） |
| `--name` | | 新角色名称；不传不修改 |
| `--role-type` / `--flow-type` | | 可选 |
| `--sub-roles` | | JSON 数组，**PATCH 合并语义**：按 `(targetId, targetType)` 合并到现有 subRoles，入参中的 sub 整体替换该 sub，**入参未提及的 sub 保留不变**（无需先调 `role-get` 自行 merge） |

**系统角色禁止更新**（包括 owner / manager / system_editor / system_reader）。

### sub-roles 子字段

每个 sub-role 描述「角色对某个权限目标的访问粒度」：

| 字段 | 类型 | 说明 |
|------|------|------|
| `targetId` | string | 目标资源 ID（数据表 → `tableId`；仪表盘 → `dashboardId`；应用 → `appId`） |
| `targetType` | string | `sheet` / `dashboard` / `app` |
| `authLevel` | string | `manage` / `edit-own` / `edit-custom-field` / `edit-field-range` / `read` / `none` |
| `appId` | string（可选） | 仅 `targetType=app` 时使用 |
| `config` | object（可选） | 字段/行级细化规则；含 `actions`（位图）/ `rows` / `cells`。结构与 `role-get` 出参 `subRoles[].config` 对齐 |

### advperm role-delete — 删除自定义角色（不可逆）

```bash
dws aitable advperm role-delete --base-id BASE_ID --role-id ROLE_ID --yes --format json
```

要求同时满足：

1. 该 Base 已开启高级权限（`role-list` 返回 `enabled=true`）。
2. 当前 dws 登录用户是该 Base 的管理员/Owner。
3. `--role-id` 是 `role-list` 返回的数字 long 字符串（如 `"10685308981"`），且对应角色 `system=false`。

不可逆，删前先 `role-get` 留底。

### 角色成员与复制

先执行 `+role-list` 读取 `enabled`、`defaultRole`、角色及其 `members`；涉及单个角色时再执行 `+role-get`。成员写入及复制只能操作自定义角色，需 Base 管理员权限。成员 ID 必须来自可信解析结果或用户提供的可信 ID，并核对下表类型；用户必须填 uid，不可把 userId 当 uid，禁止猜测或轮换 ID 试错。复制配置不要求取得成员 ID。

以下规则同样适用于原子命令 `advperm role-members-add`、`advperm role-members-update`、`advperm role-copy`、`advperm role-query-user`。

| 参数/字段 | JSON 类型 | 契约 |
|---|---|---|
| `baseId` | String | Base 唯一标识 |
| `roleId` / `sourceRoleId` / `defaultRoleId` / `targetUid` | String | long 形态字符串，不是 JSON number；`targetUid` 为用户 uid |
| `members` | array | 最多 200 个对象，每项必须含非空 String `id` 和 `memberType` |
| `members[].id` | String | 随成员类型填写，见下表；用户填 uid（不是 userId） |
| `members[].memberType` | String | 使用下表字符串编码，例如 `"2"`，不是数字 `2`，也不是 `user` / `conv` |
| `members[].memberIdBelongOrgId` | number | 可选，Java Long 整数，不加引号；同文档组织可省略，服务端回填 docOrgId；跨组织必须填成员所属组织 ID |

| memberType 字符串编码 | 成员类型 | id 含义（String） |
|---|---|---|
| `"1"` | 群 | 群 id |
| `"2"` | 用户 | uid，不是 userId |
| `"3"` | 组织 | orgid |
| `"4"` | 部门 | 部门 id |
| `"5"` | 虚拟组 | 虚拟组 id |
| `"6"` | 角色组 | 角色组 id |
| `"8"` | 动态组 | 动态组 id |

```bash
# 增量追加同组织用户：不会移除现有成员
 dws aitable +role-members-add --base-id BASE_ID --role-id ROLE_ID \
  --members '[{"id":"<UID>","memberType":"2"}]'

# 全量替换成员（跨组织示例，123456 需替换为成员所属组织 ID）
# 必须带完整目标列表和刚回读的默认角色；[] 表示明确清空
 dws aitable +role-members-update --base-id BASE_ID --role-id ROLE_ID \
  --members '[{"id":"<UID>","memberType":"2","memberIdBelongOrgId":123456}]' \
  --default-role-mode 0

# 仅当当前 defaultRole.mode=1 时同时传当前默认角色 ID
 dws aitable +role-members-update --base-id BASE_ID --role-id ROLE_ID --members '[]' \
  --default-role-mode 1 --default-role-id CURRENT_DEFAULT_ROLE_ID

# 复制配置（不复制成员）与查询用户实际命中的角色
 dws aitable +role-copy --base-id BASE_ID --source-role-id ROLE_ID --name "市场可读副本"
 dws aitable +role-query-user --base-id BASE_ID --target-uid TARGET_UID
```

成员配置规则：

- `+role-members-add` 是增量追加；`+role-members-update` 是完整覆盖，绝不能用后者仅传一个待追加成员。二者均需管理员，返回写后最新成员列表 `{roleId, members}`。
- DWS 的 `+role-members-update` 强制 `--members`：非空全替换，`[]` 明确清空，不接受 `null`；同时强制 `--default-role-mode` 为 `0`（无兜底）、`1`（指定自定义角色）或 `2`（继承文档权限）。mode 为 `1` 时必须带非空 long 形态字符串 `--default-role-id`；按刚回读值传入，防止误改默认角色。
- MCP `update_role_members` 的 members 可省略（不改），传入时须为 array：`[]` 清空，非空全替换；defaultRoleMode/defaultRoleId 都省略时不改默认角色；**这不放宽上述 DWS 必填约束**。
- `+role-copy` 沿用源角色的 `roleType`、`flowType`、`subRoles`，不复制成员，系统角色不可复制；`name` 省略或空字符串使用「源角色名 副本」，返回新角色完整配置。
- `+role-query-user` 的 `--target-uid` 可选，省略或空字符串查询当前调用人，仅指定查询对象，绝不覆盖调用身份。返回 `{enabled, roles}`，roles 仅含角色配置，不含成员（members 恒为空）；高级权限关闭时 roles 通常为空，不能把此接口当成员列表。
- 写操作只调用一次；超时、8001 或结果不确定时先用 `+role-list`/`+role-get` 回读，未确认幂等前不得自动重试。

## 能力边界

| 能力 | 状态 |
|------|------|
| 开/关高级权限 | ✅ 需管理员 |
| 列出 / 读取角色 | ✅ 普通成员也可读 |
| 创建自定义角色 | ✅ 需管理员 |
| 增量修改角色（PATCH 语义，不清空未传字段） | ✅ 需管理员 |
| 删除自定义角色 | ✅ 需管理员 |
| 修改/删除系统角色 | ❌ 服务端禁止；只能在 AI 表格 Web 端操作 |
| 向角色追加成员 | ✅ `+role-members-add`，需管理员；写后回读 |
| 完整替换角色成员 | ✅ `+role-members-update`，需管理员；必须显式保留 defaultRole |
| 复制自定义角色 | ✅ `+role-copy`，不复制成员；系统角色禁止 |
| 查询用户命中角色 | ✅ `+role-query-user`，只读 |

## 错误码速查

| 场景 | code | type | message |
|------|------|------|---------|
| advperm 关闭时调用写接口（如 `role-delete` / `role-create` / `role-update`） | `ADVANCED_PERMISSION_DISABLED` | `USER_ERROR` | `Advanced permission is disabled for base <BASE>, please enable it via setAdvancedPermission before managing roles` |
| 非管理员调用 `enable` / `disable` / `role-create` / `role-update` / `role-delete` | `401` | `AUTH_ERROR` | `the current user must be a manager (administrator) of this base to manage roles or advanced permission` |
| 删除/更新系统角色（`system=true`） | `600` | `USER_ERROR` | `Illegal argument` |
| 操作不存在的数字 roleId（get/update/delete） | `600` | `USER_ERROR` | `Illegal argument` |
| 传非数字 roleId（如 `owner` / `manager`） | `INVALID_PARAMS` | `INPUT_ERROR` | `roleId is required` |
| `role-create` 缺 `--name` | `INVALID_PARAMS` | `INPUT_ERROR` | `name is required` |
| `--sub-roles` JSON 不是数组 / 解析失败 | （CLI 层拦截） | — | `--sub-roles 解析失败 ...` / `--sub-roles 必须是 JSON 数组` |
| `--base-id` 无法解析 | `INVALID_BASE_ID` | `INPUT_ERROR` | `baseId cannot be resolved to docId` |

> `600 / Illegal argument` 同时覆盖"操作系统角色"和"操作不存在 roleId"两种情况。拿到 `600` 时先 `role-list` 自查目标 roleId 是否存在、是否 `system=true`，再据此引导用户。

## 典型工作流

### 排查"成员看不到某些字段/记录"

```bash
dws aitable advperm role-list --base-id BASE_ID --format json
# 若 enabled=false：高级权限未开，所有规则不生效，与用户确认是否需要 enable

dws aitable advperm enable --base-id BASE_ID --format json
dws aitable advperm role-list --base-id BASE_ID --format json
# 看 roles[] 里有哪些自定义角色

dws aitable advperm role-get --base-id BASE_ID --role-id ROLE_ID --format json
# 检查 subRoles[].config 中的字段/行级权限规则
```

### 新建一个"市场可读"角色

```bash
# 1. 确保高级权限已开
dws aitable advperm enable --base-id BASE_ID --format json

# 2. 拿目标 sheet 的 tableId
dws aitable table get --base-id BASE_ID --format json

# 3. 创建角色 + 指定 sheet 子角色 authLevel=read
dws aitable advperm role-create --base-id BASE_ID --name "市场可读" \
  --sub-roles '[{"targetId":"<tableId>","targetType":"sheet","authLevel":"read"}]' \
  --format json
# → 返回新角色完整配置，含 roleId，记下后续 patch / delete 使用
```

### 升级角色权限（read → edit-own），保留其他配置

```bash
# 只传 sub-roles，name 等其他字段保持不变（PATCH 语义）
dws aitable advperm role-update --base-id BASE_ID --role-id ROLE_ID \
  --sub-roles '[{"targetId":"<tableId>","targetType":"sheet","authLevel":"edit-own"}]' \
  --format json
```

### 改角色名（不影响权限规则）

```bash
dws aitable advperm role-update --base-id BASE_ID --role-id ROLE_ID --name "新名字"
```

### 清理废弃角色

```bash
dws aitable advperm role-list --base-id BASE_ID --format json
dws aitable advperm role-delete --base-id BASE_ID --role-id ROLE_ID --yes --format json
```

### 关闭高级权限（恢复全员可见）

```bash
dws aitable advperm role-list --base-id BASE_ID --format json > /tmp/roles-backup.json
dws aitable advperm disable --base-id BASE_ID --yes --format json
```
