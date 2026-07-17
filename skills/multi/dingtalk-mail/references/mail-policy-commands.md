# 自动回复、名单与收信规则命令

### 获取自动回复配置

获取当前用户的邮件自动回复配置，包括是否启用、生效时间、回复范围和回复内容。

```
Usage:
  dws mail auto-reply get [flags]
Example:
  dws mail auto-reply get --email user@company.com
Flags:
      --email string   用户的邮箱地址 (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `enabled` | bool | 是否启用自动回复 (true=启用, false=禁用) |
| `startTime` | string | 自动回复开始时间 |
| `endTime` | string | 自动回复结束时间 |
| `scope` | string | 回复范围: "contact"(仅联系人) 或 "all"(所有人) |
| `content` | string | 自动回复内容 |

### 更新自动回复配置

更新或设置用户的邮件自动回复配置。所有参数均为必填。建议先通过 `auto-reply get` 获取当前配置，再传入需要修改的字段值。

```
Usage:
  dws mail auto-reply update [flags]
Example:
  dws mail auto-reply update --email user@company.com --enabled true \
    --start "2026/07/01 09:00:00 +0800" --end "2026/07/07 18:00:00 +0800" \
    --scope all --content "出差中，请稍后联系"
  dws mail auto-reply update --email user@company.com --enabled false \
    --start "2026/07/01 09:00:00 +0800" --end "2026/07/07 18:00:00 +0800" \
    --scope all --content "已关闭自动回复"
Flags:
      --email string       用户的邮箱地址 (必填)
      --enabled string     是否启用自动回复: true/false (必填)
      --start string       自动回复开始时间，格式: YYYY/MM/DD HH:MM:SS +ZZZZ (必填)
      --end string         自动回复结束时间，格式: YYYY/MM/DD HH:MM:SS +ZZZZ (必填)
      --scope string       回复范围: contact(仅联系人)/all(所有人) (必填)
      --content string     自动回复内容 (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `success` | boolean | 更新是否成功 |
| `result` | object | 更新结果，成功时为空对象 |
| `errorCode` | string | 错误码（仅失败时存在） |
| `errorMsg` | string | 错误信息（仅失败时存在） |

### 个人收信白名单管理

#### 列出白名单

```
Usage:
  dws mail allow-list list [flags]
Example:
  dws mail allow-list list --email user@company.com
Flags:
      --email string   用户的邮箱地址 (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `total` | number | 白名单总数 |
| `entries` | string[] | 白名单地址列表（邮件地址如 `123@domain.com`，域名如 `@domain.com`，域名前需加 `@`） |
| `success` | boolean | 调用是否成功 |
| `errorCode` | string | 错误码（仅失败时存在） |
| `errorMsg` | string | 错误信息（仅失败时存在） |

#### 添加白名单

```
Usage:
  dws mail allow-list add [flags]
Example:
  dws mail allow-list add --email user@company.com --entries a@b.com,@spam.com
Flags:
      --email string    用户的邮箱地址 (必填)
      --entries string  逗号分隔的地址列表，支持邮件地址(如123@domain.com)或域名(如@domain.com)
```

#### 移除白名单

```
Usage:
  dws mail allow-list remove [flags]
Example:
  dws mail allow-list remove --email user@company.com --entries a@b.com,@spam.com
Flags:
      --email string    用户的邮箱地址 (必填)
      --entries string  逗号分隔的地址列表，支持邮件地址(如123@domain.com)或域名(如@domain.com)
```

### 个人收信黑名单管理

#### 列出黑名单

```
Usage:
  dws mail block-list list [flags]
Example:
  dws mail block-list list --email user@company.com
Flags:
      --email string   用户的邮箱地址 (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `total` | number | 黑名单总数 |
| `entries` | string[] | 黑名单地址列表（邮件地址如 `123@domain.com`，域名如 `@domain.com`，域名前需加 `@`） |
| `success` | boolean | 调用是否成功 |
| `errorCode` | string | 错误码（仅失败时存在） |
| `errorMsg` | string | 错误信息（仅失败时存在） |

#### 添加黑名单

```
Usage:
  dws mail block-list add [flags]
Example:
  dws mail block-list add --email user@company.com --entries spam@bad.com,@junk.com
Flags:
      --email string    用户的邮箱地址 (必填)
      --entries string  逗号分隔的地址列表，支持邮件地址(如123@domain.com)或域名(如@domain.com)
```

#### 移除黑名单

```
Usage:
  dws mail block-list remove [flags]
Example:
  dws mail block-list remove --email user@company.com --entries spam@bad.com,@junk.com
Flags:
      --email string    用户的邮箱地址 (必填)
      --entries string  逗号分隔的地址列表，支持邮件地址(如123@domain.com)或域名(如@domain.com)
```

### 收信规则管理

#### 列出收信规则

列出当前用户的所有收信规则，包括规则名称、启用状态、条件、动作和排序。

```
Usage:
  dws mail rule list [flags]
Example:
  dws mail rule list --email user@company.com
Flags:
      --email string   用户的邮箱地址 (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `total` | int | 规则总数 |
| `rules` | List[] | 规则列表 |
| `rules[].id` | string | 规则 ID |
| `rules[].name` | string | 规则名称 |
| `rules[].enabled` | bool | 是否启用 |
| `rules[].conditions` | List[] | 规则条件列表 |
| `rules[].actions` | List[] | 规则动作列表 |
| `rules[].order` | int | 规则排序 |

#### 创建收信规则

创建一条新的收信规则。支持设置规则名称、启用状态、匹配条件和执行动作。

> **`--conditions` 和 `--actions`** 为 JSON 数组字符串。

**界面与参数对应关系：**

| 界面元素 | CLI 参数 / JSON 字段 | 说明 |
|----------|---------------------|------|
| 规则名称 | `--name` | 必填，规则的显示名称 |
| 如果满足以下「全部」条件 | `--conditions` | 多个条件之间为 **AND** 关系（即所有条件都满足才触发） |
| ├ 对象下拉（发件人） | `object: "from"` | 条件匹配的对象，可选值见下方 |
| ├ 操作下拉（包含） | `operation: "include"` | 匹配方式，可选值见下方 |
| └ 关键词输入框 | `keyword: "a@test.com"` | 匹配的具体值 |
| 执行以下操作 | `--actions` | 条件满足后执行的动作列表 |
| ├ 动作下拉（移动到文件夹） | `action: "ActSavetoFolder"` | 动作类型，可选值见下方 |
| └ 参数选择（收件箱） | `parameters: ["2"]` | 动作的参数，如目标文件夹 ID |

**条件逻辑说明：**

- `--conditions` 数组中的多个条件之间为 **AND（且）** 关系，即所有条件都满足才触发规则
- 同一个条件对象（如 `from`）内部的 `or` 数组中多个表达式之间为 **OR（或）** 关系
- 同一个 `and` 数组中的多个子条件之间为 **AND（且）** 关系

**条件对象 (object) 与合法操作类型 (operation) 组合：**

| object | 合法 operation | 说明 |
|--------|---------------|------|
| `from` | `include`(包含), `exclude`(不包含), `oneof`(是联系人之一), `noneof`(不是联系人之一) | 匹配发件人地址或名称 |
| `to` | `include`(包含), `exclude`(不包含), `oneof`(是联系人之一), `noneof`(不是联系人之一) | 匹配收件人地址或名称 |
| `subject` | `include`(包含), `exclude`(不包含) | 匹配邮件主题 |
| `attachment` | `exist`(是否存在附件) | keyword="1" 表示有附件，keyword="0" 表示无附件 |
| `x-aliyun-size` | `greater`(大于), `less`(小于) | 邮件大小，单位为 **字节(Bytes)**（1KB=1024, 1MB=1048576）；可组合使用表示范围区间 |

**操作类型 (operation) 详细说明：**

| 值 | 界面显示 | 适用 object | 说明 |
|----|---------|------------|------|
| `include` | 包含 | from, to, subject | 字段包含关键词 |
| `exclude` | 不包含 | from, to, subject | 字段不包含关键词 |
| `oneof` | 是联系人之一 | from, to | 字段值在给定联系人列表中 |
| `noneof` | 不是联系人之一 | from, to | 字段值不在给定联系人列表中 |
| `greater` | 大于 | x-aliyun-size | 数值大于阈值，单位字节(Bytes) |
| `less` | 小于 | x-aliyun-size | 数值小于阈值，单位字节(Bytes) |
| `exist` | 存在 | attachment | keyword="1" 表示有附件，keyword="0" 表示无附件 |

**动作类型 (action) 可选值：**

| 值 | 界面显示 | parameters 说明 | 前置依赖 |
|----|---------|----------------|----------|
| `ActSavetoFolder` | 移动到文件夹 | 目标文件夹 ID，如 `["2"]`（2=收件箱） | 需先通过 `dws mail folder list` 获取文件夹 ID |
| `ActFlagMail` | 标记标签 | 标签 ID 列表，逗号分隔，如 `["102,11,1"]` | 需先通过 `dws mail tag list` 获取标签 ID |
| `ActFlagMail2` | 标记已读 | `"asread"`(标记已读)，服务端仅支持标记已读，不支持标记未读 | 无 |
| `ActReply` | 自动回复 | 回复内容文本，如 `["感谢您的来信"]` | 无 |

**条件 JSON 结构说明：**

每个条件由 `object`（匹配对象）和 `or`（OR 表达式列表）组成，`or` 内嵌 `and`（AND 条件列表）。

| 字段 | 说明 |
|------|------|
| `object` | 条件对象，取值及合法 operation 见上方组合表 |
| `or` | OR 表达式列表，同一 object 下多个 or 项之间为 **OR** 关系 |
| `and` | AND 条件列表，同一 or 项内多个 and 子条件之间为 **AND** 关系 |
| `operation` | 操作类型，必须与 object 合法组合（见上方组合表） |
| `keyword` | 关键词/阈值；attachment+exist 时 "1"=有附件/"0"=无附件；x-aliyun-size 时单位为字节(Bytes)，如 1KB=1024, 1MB=1048576 |
| `ignoreCase` | 是否忽略大小写（布尔值，仅 from/to/subject + include/exclude 时需要） |

**完整 conditions JSON 示例：**

```json
[
  {"object":"from","or":[
    {"and":[{"operation":"oneof","keyword":"a@test.com","ignoreCase":true}]},
    {"and":[{"operation":"oneof","keyword":"b@test.com","ignoreCase":true}]}
  ]},
  {"object":"subject","or":[{"and":[{"operation":"include","keyword":"报告","ignoreCase":true}]}]},
  {"object":"attachment","or":[{"and":[{"operation":"exist","keyword":"1"}]}]},
  {"object":"x-aliyun-size","or":[{"and":[{"operation":"greater","keyword":"1024"},{"operation":"less","keyword":"10240"}]}]}
]
```

> 上例表示：发件人是 a@test.com **或** b@test.com **且** 主题包含"报告" **且** 有附件 **且** 大小在 1KB(1024字节)~10KB(10240字节) 之间。
>
> **同一 object 下匹配多个值的 OR 写法：** 在 `or` 数组中放多个 `and` 项（每个 `and` 对应一个匹配值），而非在一个 `and` 中放多个条件。例如上方 `from` 条件中，两个邮箱地址分别作为独立的 `and` 项放在 `or` 数组中，表示"满足任一即可"。

**完整 actions JSON 示例：**

```json
[
  {"action":"ActSavetoFolder","parameters":["2"]},
  {"action":"ActFlagMail","parameters":["102,11,1"]},
  {"action":"ActFlagMail2","parameters":["asread"]},
  {"action":"ActReply","parameters":["感谢您的来信，我将尽快回复"]}
]
```

> **注意：** 使用 `ActSavetoFolder` 前需先通过 `dws mail folder list` 获取文件夹 ID；使用 `ActFlagMail` 前需先通过 `dws mail tag list` 获取标签 ID。

```
Usage:
  dws mail rule create [flags]
Example:
  dws mail rule create --email user@company.com --name "VIP邮件标记" --enabled true \
    --conditions '[{"object":"from","or":[{"and":[{"operation":"include","keyword":"vip@company.com","ignoreCase":true}]}]}]' \
    --actions '[{"action":"ActFlagMail2","parameters":["asread"]}]'
  dws mail rule create --email user@company.com --name "大附件归档" \
    --conditions '[{"object":"x-aliyun-size","or":[{"and":[{"operation":"greater","keyword":"10485760"}]}]}]' \
    --actions '[{"action":"ActSavetoFolder","parameters":["6"]}]'
Flags:
      --email string       用户的邮箱地址 (必填)
      --name string        规则名称 (必填)
      --enabled string     是否启用: true/false (必填)
      --conditions string  规则条件 JSON 数组 (可选)
      --actions string     规则动作 JSON 数组 (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `success` | bool | 创建是否成功 |
| `errorCode` | string | 错误码 |
| `errorMsg` | string | 错误消息 |
| `id` | string | 新建规则 ID |

#### 更新收信规则

更新已有的收信规则。**除 `--conditions` 外所有参数均为必填**。

> **建议工作流：** 先通过 `dws mail rule list` 获取当前规则的完整配置，再传入需要修改的字段值。
>
> `--conditions` 为空或不传表示命中所有邮件（无条件匹配）。`--actions` 格式同 create 命令。

```
Usage:
  dws mail rule update [flags]
Example:
  dws mail rule update --email user@company.com --id <ruleId> --name "新规则名" --enabled true \
    --actions '[{"action":"ActSavetoFolder","parameters":["6"]}]'
  dws mail rule update --email user@company.com --id <ruleId> --name "全量归档" --enabled false \
    --conditions '[{"object":"subject","or":[{"and":[{"operation":"include","keyword":"报告","ignoreCase":true}]}]}]' \
    --actions '[{"action":"ActSavetoFolder","parameters":["6"]}]'
Flags:
      --email string       用户的邮箱地址 (必填)
      --id string          规则 ID (必填)
      --name string        规则名称 (必填)
      --enabled string     是否启用: true/false (必填)
      --conditions string  规则条件 JSON 数组 (可选，为空表示命中所有邮件)
      --actions string     规则动作 JSON 数组 (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `success` | bool | 更新是否成功 |
| `errorCode` | string | 错误码 |
| `errorMsg` | string | 错误信息 |
| `result` | object | 更新结果 |

#### 删除收信规则

删除指定的收信规则。

```
Usage:
  dws mail rule delete [flags]
Example:
  dws mail rule delete --email user@company.com --id <ruleId>
Flags:
      --email string   用户的邮箱地址 (必填)
      --id string      规则 ID (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `success` | bool | 删除是否成功 |
| `errorCode` | string | 错误码 |
| `errorMsg` | string | 错误信息 |
| `result` | object | 删除结果 |

#### 调整收信规则排序

调整指定收信规则的排序位置，向上(up)或向下(down)移动。

```
Usage:
  dws mail rule adjust [flags]
Example:
  dws mail rule adjust --email user@company.com --id <ruleId> --direction up
  dws mail rule adjust --email user@company.com --id <ruleId> --direction down
Flags:
      --email string      用户的邮箱地址 (必填)
      --id string         规则 ID (必填)
      --direction string  调整方向: up/down (必填)
```

**返回字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `success` | bool | 调整是否成功 |
| `errorCode` | string | 错误码 |
| `errorMsg` | string | 错误消息 |
| `result` | object | 调整结果 |
