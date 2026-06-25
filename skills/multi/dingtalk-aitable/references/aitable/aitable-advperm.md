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

> 所有子命令的 `--base-id` 必填，可用隐藏别名 `--base`。

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
- 不返回角色成员列表；如需"成员-角色"映射请去 AI 表格 Web 端。
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

## 能力边界

| 能力 | 状态 |
|------|------|
| 开/关高级权限 | ✅ 需管理员 |
| 列出 / 读取角色 | ✅ 普通成员也可读 |
| 创建自定义角色 | ✅ 需管理员 |
| 增量修改角色（PATCH 语义，不清空未传字段） | ✅ 需管理员 |
| 删除自定义角色 | ✅ 需管理员 |
| 修改/删除系统角色 | ❌ 服务端禁止；只能在 AI 表格 Web 端操作 |
| 角色 ↔ 成员绑定 | ❌ CLI 暂不支持，需在 AI 表格 Web 端 → Base 设置 → 高级权限 → 角色管理面板手动完成 |

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
