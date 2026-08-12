# 错误与恢复说明

只查当前错误对应章节。先读取结构化字段，再决定修正、重试、请求用户介入或停止。

## 错误返回格式

```json
{
  "error": {
    "code": 3,
    "category": "validation",
    "message": "missing required flag(s): --base-id",
    "reason": "...",
    "retryable": false,
    "hint": "...",
    "actions": ["..."]
  }
}
```

错误写到 stderr。`code` 是 CLI 退出码，`category` 是稳定大类：`api`、`auth`、
`validation`、`discovery`、`internal`。其他字段按错误类型出现，不保证每次都有。

## 错误分类与 Agent 行为

### 可自行修复

- `category=validation` 且 `available_flags`/`hint` 给出明确修正：核对 leaf Help，修正
  一次；不要猜 flag。
- 自然目标返回 `details.candidates`：零命中或多候选都停止并消歧，禁止选择第一项。
- `reason=confirmation_required`：完整协议见 [confirmation.md](./confirmation.md)
  （展示摘要 → 用户显式同意 → 原始命令追加 `--yes`；禁止静默重试、管道喂答案、换成更弱命令）。

### 需用户介入

- 权限不足、资源不存在、配额/权益不足：报告 `server_error_code`、`trace_id`、`hint`
  和 `action_url`（如有），不自行改身份或尝试替代接口。
- profile 不存在或同组织多账号无默认：让用户指定 `corpId:userId`，不要选最近账号。
- 未知投递状态：停止重发，先查询状态；没有状态查询能力时如实报告未知。

## 重试规则

- `retryable=true`：遵守 `retry_after_seconds` 或 `next_retry_at`；运行时可能已经完成
  HTTP 重试，调用方只做一次有界重试并保留相同幂等键。
- `retryable=false`：不要重试。
- 未出现 `retryable`：不要从 `category`、HTTP 文案或“看起来像临时错误”推断可重试；
  使用 `--verbose` 收集诊断后停止。
- 超时不会因为盲目增大 `--timeout` 自动变安全。只有任务确实允许更长等待且操作状态可判定时，
  才显式调整超时。

## 认证与权限

- 精确的 access-token 拒绝信号会由 Runtime 自动刷新并最多重放一次；不要再包一层“重试两次”。
- `reason=auth_refresh_failed` 或 `auth status` 返回 `authenticated=false`：保留原错误，运行
  `dws auth status --profile <same-profile> --format json`；按返回 `hint` 恢复，必要时请用户登录。
- HTTP/RPC 403 与普通权限不足不会触发 token 刷新；不要通过切 profile、bot 或 webhook
  身份绕过。
- AppKey/AppSecret 缺失只用于应用凭据配置问题；不要向普通业务用户索要凭据。

---

## aitable 高频错误

> 参数体系: `baseId / tableId / fieldId / recordId`。CLI flag 用 kebab-case（`--base-id`），JSON 内用 camelCase（`baseId`）。

- 参数缺失 / 无效请求 — 还在用旧参数 `dentryUuid` / `--doc` / `--sheet` → 改用 `--base-id` / `--table-id` / `--field-id` / `--record-ids`
- 参数传了但服务端没收到 — flag 用了 camelCase（如 `--baseId`）→ flag 用 kebab-case: `--base-id <ID>`
- `record query --filters` 无结果 — 单选/多选过滤用了 option name 而非 id → 先 `field get` 读取 options，用 option id 过滤
- record create/update 失败 — `cells` key 用了字段名（应为 fieldId）；特殊字段格式错误 → 先 `field get` 拿字段目录；url 传 `{"text":"..","link":".."}`
- 更新选项后历史数据异常 — 更新 options 没传完整列表 / 没保留原 id → 先 `field get` 取完整配置，保留已有 option 的 id
- `cannot delete the last table` — 该表是 Base 最后一张表 → 先新建表再删旧表，或用 `base delete`
- `formula` 类型 `not supported yet` — 部分字段类型暂不支持 API 创建 → 复杂字段拆开单独创建，先建基础结构

**排查链路**: `base list` → `base get`(→tableId) → `field get`(→fieldId) → `record query`(→recordId)。别跳步，别猜 ID。

**批量上限**: record 100 条 / field 15 个 / table·field 详情 10 个。

---

## doc 高频错误

- 文档不存在 / nodeId 无效 — nodeId 或 URL 不正确、文档已删除 → `drive search` / `wiki node search` 或 `wiki node list` 重新获取正确 nodeId
- 无下载权限 — 文档分享设置不允许 → 报告用户，建议联系文档所有者
- `update --mode overwrite` 意外清空 — overwrite 会清空原内容后重写 → 默认用 `--mode append`，overwrite 前必须跟用户确认
- 块编辑 blockId 无效 — blockId 过期或文档结构已变 → 先 `block list` 刷新获取最新 blockId
- `CONTENT_TRUNCATED` — 分片写入持续超时，分片大小已减半至最小阈值（5000 字符）仍无法成功 → 后端服务可能过载或网络异常。已写入部分内容可通过 `doc read --node <ID>` 查看，待后端恢复后从断点处用 `doc update --mode append` 继续追加

---

## calendar 高频错误

- **误用顶层 `dws calendar` / 臆造 `calendar list`** — 只输入 `dws calendar` 或尝试不存在的 `calendar list` 会打印大段 Usage，易导致上下文暴涨与响应变慢 → **改用** `dws calendar event list --start "<ISO>" --end "<ISO>" --format json`，或加载 `dingtalk-calendar` 后按其日程查询 recipe 执行；详见 `dingtalk-calendar`「CLI 命令树与黄金路径」「反模式（禁止）」
- 时间格式错误 — 未使用 ISO-8601 格式 → 标准格式: `2026-03-10T14:00:00+08:00`
- 会议室搜索报错 / 返空 — 企业会议室超 100 条未分组查询 → 先 `room list-groups` → 按 `--group-id` 逐组搜索
- 参与者 / 会议室添加失败 — eventId 不正确 → 先 `event list` 或 `event create` 获取正确 eventId
- `roomId invalid` / 订房失败 — 把会议室**展示名**或用户口语当成了 `roomId` → **只能**使用 `room search` 返回 JSON 中的 `rooms[].roomId` 填入 `room add --rooms`；不得以中文名、楼层编号文案充当 ID
- `unknown flag: --query`（会议室）— `room search` **不支持**按名称搜索 → 先 `room list-groups` 再按分组 `room search`，在返回列表中匹配名称后取 `roomId`（见 calendar.md）

---

## chat 高频错误

- 群聊读取优先 `chat +chat-messages --group <群名或ID>`；单聊先解析唯一用户 ID 再读取。
  自然群名多候选时读取 `details.candidates` 并让用户消歧，不取第一项。
- 普通发送优先 `chat +dm`、`chat +send-to-group` 或 `chat +messages-send`；发送类 Shortcut
  的最终 Schema 要求确认时，必须先确认再加 `--yes`。
- `--group`、`--user`、`--open-dingtalk-id` 等目标参数互斥：按 leaf Help 只传一类目标。
- `+messages-send` 的 `--text`/`--markdown`/`--media-id`/`--file` 互斥；身份、目标、
  凭据和幂等参数还受 user/bot/webhook 能力矩阵约束。
- `ok=false`、`partial=true`、非空 `failures` 或分页仍有 continuation 都不是完整成功；
  必须保留 ledger，不能只返回已成功部分。
- 机器人不在群或当前用户无管理权限：报告真实失败，需要群管理员处理；不改用当前用户身份发送。
- 原子 `chat message send` 只用于 Shortcut 未覆盖的底层消息类型/原始字段。其正文可用
  `--text`，不要把某个历史位置参数写法当成所有发送入口的规则。

---

## oa 高频错误

- approve/reject 缺少 taskId — 未先获取审批任务 → 先 `approval tasks --instance-id <ID>` 获取 taskId
- list-initiated 缺少 processCode — 未查询审批表单 → 先 `approval list-forms` 或 `detail` 获取 processCode
- 撤销审批失败 — 非本人发起的审批 → `revoke` 只能撤销自己发起的审批

---

## report 高频错误

> 参数体系：`templateId / reportId`。CLI flag 用 kebab-case（`--template-id` / `--contents-file`）。`contents` 数组每项含 `key/sort/content/contentType/type` 五个字段，`key/sort/type` 必须严格对齐 `template get` 返回的 `field_name/field_sort/field_type`。

- `INPUT_INVALID_JSON` — `--contents` 或 `--contents-file` 内容非合法 JSON → 检查数组结构 `[{key, sort, content, contentType, type}]`
- `INPUT_FILE_NOT_FOUND` — `--contents-file` 路径错 / sandbox OS 风格不匹配 → 先确认 sandbox OS（Windows: `C:\...` / macOS|Linux: `/...`）后改写
- `INPUT_MISSING_PARAM` — `--template-id` 或 contents 必填缺失 → 先跑 `dws report template list --format json` 取 templateId
- `MCP_TOOL_ERROR` + `server_error_code: PARAM_ERROR` — 服务端业务校验失败（templateId 错 / `key` 不在模版定义 / 类型不匹配 / 必填空 等多种形态都返回这一个码，且服务端不区分子原因）→ 不要靠错误信息排查具体字段，按提交链路重新走 `template list → template get → entry submit`；连续 ≥ 2 次失败必须停止重试，降级 final_reply
- `MCP_TOOL_ERROR` + 其他 server_error_code — 查看 `technical_detail`；如出现不可读错误（仅含 `root.success当前值`），降级 final_reply 引导用户手动操作

**排查链路**：`template list` → `template get`（取 `result.report_template_fields[]`，每项含 `field_name/field_sort/field_type`）→ 拼 `--contents`：`field_name → key`、`field_sort → sort`、`field_type → type`，再填 `content` 与 `contentType` → `entry submit`。**别跳步、别猜字段名、别自己改写 key 名。**

---

## contact / drive / mail 高频错误

- contact: `dept list-children` 报错 — `--id` 传了非整数值 → deptId 必须为整数，从 `dept search` 获取
- drive: 文件不存在 — dentryUuid 不正确 → `drive list` 逐级浏览获取正确 ID
- drive: 上传失败 / uploadId 无效 — 跳过了 `upload-info` 步骤 → 必须先 `upload-info` 获取上传凭证，再 `commit`
- drive: 文件名报错 — `--file-name` 缺少扩展名 → 必须包含扩展名: `report.pdf`
- mail: 发件地址不正确 — 未先查询可用邮箱 → 先 `mailbox list` 获取邮箱地址
- mail: KQL 搜索无结果 — 查询语法错误 → 字段值含空格用双引号: `subject:\"周报\"`

---

## 通用排查三步法

1. **确认 ID** — 从最顶层资源逐级获取，不猜 ID、不跳步
2. **确认参数** — flag 用 kebab-case，JSON 用 camelCase；特殊字段查产品参考文档确认格式
3. **确认限制** — 检查批量上限和已知约束（各产品注意事项见对应产品参考文档）
