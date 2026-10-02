# 选人规则：从连接器获取 — `target_connect_approval`

> 适用节点：审批人（`approver`）、办理人（`handler`）

通过钉钉连接器动态获取审批人。连接器在运行时被调用，根据连接器配置的逻辑返回审批人列表。

## 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `type` | string | 是 | 固定值 `"target_connect_approval"` |
| `sysParams` | object | 是 | 系统参数，运行时由引擎填充占位符 |
| `sysParams.corpId` | string | 是 | 企业 ID |
| `sysParams.appUuid` | string | 是 | 应用 ID（通常与 corpId 相同） |
| `sysParams.staffId` | string | 是 | 固定占位符 `"#userId"`，运行时替换为发起人 userId |
| `sysParams.actionInstanceId` | string | 是 | 连接器实例 ID，格式为 `G-A-INST-` 前缀 |
| `sysParams.scene` | string | 是 | 固定值 `"approval"` |
| `extParams` | object | 是 | 扩展参数 |
| `extParams.procInstId` | string | 是 | 固定占位符 `"#procInstId"`，运行时替换为审批实例 ID |
| `extParams.actionName` | string | 是 | 连接器动作名称 |
| `sync` | boolean | 是 | 是否同步调用，通常 `true` |
| `isEmpty` | boolean | 是 | 是否允许空审批人，通常 `false` |
| `actType` | string | 否 | 留空 `""` |

## 使用场景

- 用户说"从连接器获取审批人"、"通过连接器动态审批"
- 需要根据外部系统或自定义逻辑动态确定审批人
- 审批人来源不是固定人员或组织架构，而是连接器集成的第三方服务

## 前置条件

使用本规则前，**必须**已在钉钉管理后台 → 连接 → 连接器中创建并发布了对应的连接器实例。用户需提供：

1. **连接器实例 ID**（`actionInstanceId`）：`G-A-INST-` 前缀的字符串，从连接器管理页面获取
2. **连接器动作名称**（`actionName`）：连接器中配置的动作名称

> 🚫 **禁止**凭空编造 `actionInstanceId`。若用户未提供真实连接器实例 ID，在 JSON 中使用 `"TODO_ACTION_INSTANCE_ID"` 占位并在待用户填写清单中说明；不要在 JSON 代码块中写 `//` 注释。

## 示例

```json
{
  "type": "target_connect_approval",
  "sysParams": {
    "corpId": "TODO_CORP_ID",
    "appUuid": "TODO_APP_UUID",
    "staffId": "#userId",
    "actionInstanceId": "TODO_ACTION_INSTANCE_ID",
    "scene": "approval"
  },
  "extParams": {
    "procInstId": "#procInstId",
    "actionName": "TODO_ACTION_NAME"
  },
  "sync": true,
  "isEmpty": false,
  "actType": ""
}
```

## 注意事项

- `sysParams.corpId` 和 `sysParams.appUuid` 为企业 ID，创建模板时由服务端自动填充，Agent 构造时可使用占位值或从模板详情中获取
- `sysParams.staffId` 必须为 `"#userId"` 占位符，不可替换为具体 userId
- `sysParams.actionInstanceId` 是连接器实例的唯一标识，**必须**取自用户提供的真实值
- `extParams.procInstId` 必须为 `"#procInstId"` 占位符，运行时由引擎替换
- `extParams.actionName` 为连接器动作名称，需与连接器配置中的动作名一致
- 连接器需已发布且可用，否则运行时调用会失败
- 通常配合 `noneActionerAction: "admin"` 使用，当连接器返回空结果时转交管理员
