# 错误码说明
全产品错误参考 + 调试流程。Agent 遇到错误时查阅此文档。

## 错误返回格式

```json
{"success": false, "code": "InvalidParameter", "message": "baseId is required"}
{"success": false, "code": "AUTH_TOKEN_EXPIRED", "message": "Token验证失败"}
{"success": false, "code": "PermissionDenied", "message": "无权限访问该资源"}
```

## 错误分类与 Agent 行为

### 可自行修复
- 参数缺失 / 格式错误 / ID 无效 → 检查参数后修正重试

### 需用户介入
- 权限不足 / 资源不存在 / 配额超限 → 报告完整错误信息给用户，不要自行尝试替代方案

## 通用错误
- 请求超时 — 网络慢或服务端响应慢 → `--timeout 60` 重试
- 网络连接失败 — 无法连接 MCP Server → 用最简命令验证: `dws contact user get-self --format json`

---

## aitable 高频错误

> 参数体系: `baseId / tableId / fieldId / recordId`。CLI flag 用 kebab-case（`--base-id`），JSON 内用 camelCase（`baseId`）。

- 参数缺失 / 无效请求 — 还在用旧参数 `dentryUuid` / `--doc` / `--sheet` → 改用 `--base-id` / `--table-id` / `--field-id` / `--record-ids`
- 参数传了但服务端没收到 — flag 用了 camelCase（如 `--baseId`）→ flag 用 kebab-case: `--base-id <ID>`
- `record query --filters` 无结果 — 单选/多选过滤用了 option name 而非 id → 先 `field get` 读取 options，用 option id 过滤
- record create/update 失败 — `cells` key 用了字段名（应为 fieldId）；特殊字段格式错误 → 先 `table get` 拿字段目录；url 传 `{"text":"..","link":".."}`
- 更新选项后历史数据异常 — 更新 options 没传完整列表 / 没保留原 id → 先 `field get` 取完整配置，保留已有 option 的 id
- `cannot delete the last table` — 该表是 Base 最后一张表 → 先新建表再删旧表，或用 `base delete`
- `formula` 类型 `not supported yet` — 部分字段类型暂不支持 API 创建 → 复杂字段拆开单独创建，先建基础结构

**排查链路**: `base list` → `base get`(→tableId) → `table get`(→fieldId) → `record query`(→recordId)。别跳步，别猜 ID。

**批量上限**: record 100 条 / field 15 个 / table·field 详情 10 个。

---

## doc 高频错误

- 文档不存在 / nodeId 无效 — nodeId 或 URL 不正确、文档已删除 → `doc search` 或 `doc list` 重新获取正确 nodeId
- 无下载权限 — 文档分享设置不允许 → 报告用户，建议联系文档所有者
- `update --mode overwrite` 意外清空 — overwrite 会清空原内容后重写 → 默认用 `--mode append`，overwrite 前必须跟用户确认
- 块编辑 blockId 无效 — blockId 过期或文档结构已变 → 先 `block list` 刷新获取最新 blockId
- `CONTENT_TRUNCATED` — 分片写入持续超时，分片大小已减半至最小阈值（5000 字符）仍无法成功 → 后端服务可能过载或网络异常。已写入部分内容可通过 `doc read --node <ID>` 查看，待后端恢复后从断点处用 `doc update --mode append` 继续追加

---

## calendar 高频错误

- **误用顶层 `dws calendar` / 臆造 `calendar list`** — 只输入 `dws calendar` 或尝试不存在的 `calendar list` 会打印大段 Usage，易导致上下文暴涨与响应变慢 → **改用** `dws calendar event list --start "<ISO>" --end "<ISO>" --format json`，或 `python scripts/calendar_today_agenda.py ...`；详见 [calendar.md](../../dingtalk-calendar/references/calendar.md)「CLI 命令树与黄金路径」「反模式（禁止）」
- 时间格式错误 — 未使用 ISO-8601 格式 → 标准格式: `2026-03-10T14:00:00+08:00`
- 会议室搜索报错 / 返空 — 企业会议室超 100 条未分组查询 → 先 `room list-groups` → 按 `--group-id` 逐组搜索
- 参与者 / 会议室添加失败 — eventId 不正确 → 先 `event list` 或 `event create` 获取正确 eventId
- `roomId invalid` / 订房失败 — 把会议室**展示名**或用户口语当成了 `roomId` → **只能**使用 `room search` 返回 JSON 中的 `rooms[].roomId` 填入 `room add --rooms`；不得以中文名、楼层编号文案充当 ID
- `unknown flag: --query`（会议室）— `room search` **不支持**按名称搜索 → 先 `room list-groups` 再按分组 `room search`，在返回列表中匹配名称后取 `roomId`（见 calendar.md）

---

## chat 高频错误

- 参数互斥报错 — `--group` 与 `--user` / `--users` 同时传入 → 群聊用 `--group`，单聊用 `--user`/`--users`，二者互斥
- 群不存在 — openconversation_id 不正确 → `chat search --query "群名"` 获取正确 ID
- 机器人无法添加到群 — 当前用户非群管理员 → 报告给用户，需群管理员操作
- `send` 消息 text 参数缺失 — text 是位置参数，不是 flag → text 直接跟在 flags 后: `send --group <ID> "内容"`

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
