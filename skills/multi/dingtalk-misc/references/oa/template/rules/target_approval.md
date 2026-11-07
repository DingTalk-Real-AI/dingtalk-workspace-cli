# 选人规则：指定成员 — `target_approval`

> 适用节点：审批人（`approver`）、办理人（`handler`）、抄送人（`notifier`）

固定指定具体人员作为审批人/办理人/抄送人。

## 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 固定值 `"target_approval"` |
| `approvals` | array | 是 | 人员列表 |
| `approvals[].userName` | string | 是 | 姓名 |
| `approvals[].workNo` | string | 是 | 员工工号 |
| `isEmpty` | boolean | 是 | 固定 `false` |
| `actType` | string | 否 | `"and"`（会签）或 `""`（其他） |

## 使用场景

- 用户指定具体人名（如"张三审批"、"由李四处理"）
- 需要固定某些人作为抄送对象

## 成员查询（强制）

使用本规则时，**禁止**凭空猜测或手写 `workNo`。**必须**通过钉钉通讯录 MCP 按姓名查询到实际成员后再填入。分两步：

> 🚫 **禁止**把 `workNo` 写成 `TODO_...` 之类占位或任意编造值。若搜不到真实成员（人员不存在、同名未确认、MCP 不可用等），**宁可不生成该 `target_approval` 规则乃至该节点**，并在回复中说明缺失、请用户补充，**绝不**用未定义值填充。

### 第一步：`search_user_by_key_word` 按关键词搜索

入参 `keyWord`（姓名关键词），返回只含 `userId` 数组：

```json
{ "userId": ["TODO_USER_ID_1"] }
```

### 第二步：`get_user_info_by_user_ids` 获取详情

入参 `user_id_list`（上一步的 `userId` 列表），返回员工详细信息：

```json
{
  "success": true,
  "result": [
    {
      "orgEmployeeModel": {
        "orgUserId": "TODO_USER_ID_1",
        "orgUserName": "张三",
        "depts": [{ "deptName": "技术部门", "deptId": 1 }]
      }
    }
  ]
}
```

### 字段映射

| 返回字段 | 含义 | 映射到规则 |
|---------|------|-----------|
| `orgEmployeeModel.orgUserId` | 员工 ID（即搜索返回的 `userId`） | → `approvals[].workNo` |
| `orgEmployeeModel.orgUserName` | 员工姓名 | → `approvals[].userName` |
| `orgEmployeeModel.depts[].deptName` | 所属部门 | 仅用于多人同名时消除歧义 |

### 步骤

1. 调用 `search_user_by_key_word`，传入姓名关键词，得到 `userId` 列表。
2. 调用 `get_user_info_by_user_ids`，传入上述 `userId`，得到 `orgUserName` 与 `orgUserId`。
3. 取 `orgUserId` → `workNo`，`orgUserName` → `userName`。
4. 若搜索返回多个 `userId`（同名人员），结合 `orgUserName` / `deptName` **向用户确认具体人员**，不要自行选择。
5. 若未搜到任何成员，向用户确认姓名或提示该成员不存在，**不要**编造 `workNo`。

## 示例

```json
{
  "approvals": [
    {
      "userName": "张三",
      "workNo": "TODO_USER_ID_1"
    },
    {
      "userName": "李四",
      "workNo": "TODO_USER_ID_2"
    }
  ],
  "isEmpty": false,
  "actType": "and",
  "type": "target_approval"
}
```

## 注意事项

- `workNo` 为员工在企业内的唯一工号，**必须**取自 `get_user_info_by_user_ids` 返回的 `orgUserId`（见上文“成员查询”），不得手写
- 当指定多人时，`actType: "and"` 表示会签（所有人都需操作）
- 在抄送人（`notifier`）节点中使用时，可额外包含 `value` 和 `name` 字段
