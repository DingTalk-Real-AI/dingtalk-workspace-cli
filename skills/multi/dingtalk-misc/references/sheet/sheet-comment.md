# sheet comment（表格单元格评论：list / create / reply / update / delete）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../sheet.md`](../sheet.md) — 命令路由 + 场景索引 + 意图判断 + 全局约束
>
> **同任务常配合**：`dws aisearch person`（查 `--mention` 用 userId）/ [`sheet-workbook.md`](sheet-workbook.md)（未知工作表名称时先 `list` 确认真实名称）

---

## 适用范围

- 仅支持钉钉在线电子表格（`extension=axls`）。`xlsx` / `xls` / `csv` 等本地表格不支持评论。
- 评论锚定在**单元格位置**：`create` / `list` 通过 `--sheet-id`（工作表 ID 或名称）+ `--range`（单元格坐标）定位；`reply` / `update` / `delete` 通过 `--comment-key` 操作，不依赖单元格位置。
- `create` / `list` 通过单元格位置定位；`reply` / `update` / `delete` 通过前一步返回的 `commentKey` 定位评论线程，不需要重新传单元格位置。
- 评论正文与单元格位置由服务端关联。Agent 只使用命令返回的 `commentKey` 继续回复、更新或删除，不解析或拼接内部关联字段。

---

## sheet comment list（查询表格评论列表）

```
Usage:
  dws sheet comment list [flags]
Example:
  dws sheet comment list --node <SHEET_ID>
  dws sheet comment list --node <SHEET_ID> --sheet-id Sheet1 --range A2
  dws sheet comment list --node <SHEET_ID> --resolve-status unresolved
  dws sheet comment list --node <SHEET_ID> --cursor <TOKEN>
Flags:
      --node string            目标表格的标识，支持传入 URL 或 ID (必填)
      --limit int              每页返回的评论数量，默认 50，最大 50
      --cursor string          分页游标，从上一次请求的返回结果中获取 (首次请求不传)
      --resolve-status string  按解决状态过滤: resolved (已解决) / unresolved (未解决)
      --sheet-id string        工作表 ID 或名称，如 Sheet1（与 --range 一起指定时按单元格过滤）
      --range string           单元格位置，A1 表示法，如 A2、B5:C10（与 --sheet-id 一起指定时按单元格过滤）
```

- 同时传入 `--sheet-id` 和 `--range` 时，仅返回该单元格的评论（服务端按单元格精确过滤）；不传则返回表格全部单元格评论。

---

## sheet comment create（创建单元格评论）

```
Usage:
  dws sheet comment create [flags]
Example:
  dws sheet comment create --node <SHEET_ID> --sheet-id Sheet1 --range A2 --content "这个数字有问题"
  dws sheet comment create --node <SHEET_ID> --sheet-id Sheet1 --range A2 --content "请核实" --mention uid1,uid2
Flags:
      --node string      目标表格的标识，支持传入 URL 或 ID (必填)
      --sheet-id string  工作表 ID 或名称，如 Sheet1 (必填)
      --range string     单元格位置，A1 表示法，仅支持单个单元格，如 A2 (必填)
      --content string   评论的文字内容，纯文本 (必填)
      --mention string   被 @ 的用户 uid 列表，逗号分隔
```

- 未知工作表名称时，先 `dws sheet list --node <SHEET_ID> --format json` 确认真实名称，禁止臆测 `Sheet1` / `0` / `default`。

---

## sheet comment reply（回复评论）

```
Usage:
  dws sheet comment reply [flags]
Example:
  dws sheet comment reply --node <SHEET_ID> --comment-key <COMMENT_KEY> --content "已核实"
  dws sheet comment reply --node <SHEET_ID> --comment-key <COMMENT_KEY> --content "比心" --emoji
  dws sheet comment reply --node <SHEET_ID> --comment-key <COMMENT_KEY> --content "请确认" --mention uid1,uid2
Flags:
      --node string         目标表格的标识，支持传入 URL 或 ID (必填)
      --content string      回复的文字内容，表情回复时填写表情名称 (必填)
      --comment-key string  被回复评论的 commentKey，格式: {13位毫秒时间戳}{32位UUID}，可从 list/create 结果获取 (必填)
      --emoji               设为 true 时作为表情贴图回复 (默认 false)
      --mention string      被 @ 的用户 uid 列表，逗号分隔
```

- 回复自动归属到被回复评论所在的单元格线程，无需再传 `--sheet-id` / `--range`。

---

## sheet comment update（更新评论）

```
Usage:
  dws sheet comment update [flags]
Example:
  dws sheet comment update --node <SHEET_ID> --comment-key <COMMENT_KEY> --content "已按最新数据修正"
Flags:
      --node string         目标表格的标识，支持传入 URL 或 ID (必填)
      --comment-key string  待更新评论的 commentKey，格式: {13位毫秒时间戳}{32位UUID}，可从 list/create 结果获取 (必填)
      --content string      更新后的评论文字内容，纯文本 (必填)
```

---

## sheet comment delete（删除评论）

> [强制] 危险操作：删除不可恢复。必须先向用户展示操作摘要并获得明确同意，用户同意后才加 `--yes` 执行。

```
Usage:
  dws sheet comment delete [flags]
Example:
  dws sheet comment delete --node <SHEET_ID> --comment-key <COMMENT_KEY> --yes
Flags:
      --node string         目标表格的标识，支持传入 URL 或 ID (必填)
      --comment-key string  待删除评论的 commentKey，格式: {13位毫秒时间戳}{32位UUID}，可从 list/create 结果获取 (必填)
```

## 关键说明

- `--mention` 接受 `userId` 列表（逗号分隔），需要先用 `dws aisearch person --query "<姓名>" --dimension name` 拿到 userId。
- `--comment-key` 是 13 位毫秒时间戳 + 32 位 UUID 的拼接字符串，从 `list` / `create` 返回中提取，用于 `reply` / `update` / `delete`。
- `create` / `list`（按单元格过滤）的 `--sheet-id` 是工作表 ID 或名称；未知时先 `dws sheet list` 确认，禁止臆测。
- `reply` 加 `--emoji` 时 `--content` 填表情名称（如 `比心`、`赞`），不是文字内容。
- `delete` 是不可逆操作；AI Agent 必须先向用户展示操作摘要并获得明确同意，同意后才追加 `--yes`。

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| `commentList[].commentKey` | `comment reply/update/delete` 的 `--comment-key` |
| `comment create` 的 `commentKey` | `comment reply/update/delete` 的 `--comment-key` |
| [`sheet-workbook.md`](sheet-workbook.md) `sheet list` 的工作表名称 | `comment create/list` 的 `--sheet-id` |
| `dws aisearch person` 的 `userId` | `comment create/reply` 的 `--mention` |

## 常用模板

```bash
# 查看表格全部单元格评论
dws sheet comment list --node <SHEET_ID> --format json

# 仅看某个单元格的评论
dws sheet comment list --node <SHEET_ID> --sheet-id Sheet1 --range A2 --format json

# 仅看未解决的评论
dws sheet comment list --node <SHEET_ID> --resolve-status unresolved --format json

# 在单元格上创建评论（未知工作表名先 sheet list 确认）
dws sheet list --node <SHEET_ID> --format json
dws sheet comment create --node <SHEET_ID> --sheet-id Sheet1 --range A2 --content "这个数字有问题" --format json

# 创建评论 + @人（先 aisearch person 拿 userId）
dws aisearch person --query "张三" --dimension name --format json
dws sheet comment create --node <SHEET_ID> --sheet-id Sheet1 --range A2 --content "请确认" --mention <uid1>,<uid2> --format json

# 文字回复
dws sheet comment reply --node <SHEET_ID> --comment-key <COMMENT_KEY> --content "已核实" --format json

# 表情回复（--content 填表情名称）
dws sheet comment reply --node <SHEET_ID> --comment-key <COMMENT_KEY> --content "比心" --emoji --format json

# 更新评论
dws sheet comment update --node <SHEET_ID> --comment-key <COMMENT_KEY> --content "已按最新数据修正" --format json

# 删除评论（不可逆；必须用户确认后再加 --yes）
dws sheet comment delete --node <SHEET_ID> --comment-key <COMMENT_KEY> --yes --format json
```

## 参考

- [`../sheet.md`](../sheet.md)（如何路由到本命令族）
- [`./sheet-workbook.md`](sheet-workbook.md)（取工作表名称）
- `dws aisearch person`（取 mention 用的 userId，跨产品命令）
