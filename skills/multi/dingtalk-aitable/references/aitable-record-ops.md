# AI 表格记录操作

仅在根 Skill 的记录 Golden Route 参数不足，或需要字段值格式、删除、历史、分享、附件细节时读取。本文件不负责 Base/Table 选路。

## 输入与字段契约

- `cells` 的 key 使用当前 Table 的真实 fieldId；字段不明时只读取目标字段配置一次。按真实字段类型写值，只读字段不得写入。
- 创建字段时使用服务端真实枚举：单选是 `singleSelect`；人民币货币字段是 `type:"currency"` 与 `config:{"currencyType":"CNY","formatter":"FLOAT_2"}`，不要猜 `select` 或 `config.symbol`。若任务核心是复杂字段配置，应从根 Skill 直接选择 field Reference，而不是读完本文件再跳转。
- 若任务核心是 filters/sort/date 操作符，应从根 Skill直接选择 filter-sort Reference；本文件不再复制 filter payload，避免一个 Case 连读两个 Reference。
- 大 JSON 使用原子命令支持的 cwd 内相对 `--records-file`；Shortcut 是否接受文件输入以其精确 leaf Schema 为准，不猜造 flag。

## 查询

```bash
dws aitable +record-query --base-id <B> --table-id <T> --record-ids <R1,R2>
dws aitable +record-query --base-id <B> --table-id <T> --record-ids <R1,R2> --field-ids <F_NAME,F_STATUS>
dws aitable +record-query --base-id <B> --table-id <T> --query "关键词" --limit 100
dws aitable record query --base-id <B> --table-id <T> --all --page-limit 50
```

- `record-ids` 用于稳定 ID 精确读取；`query` 用于全文搜索。需要条件筛选的任务应把 filter-sort 作为该 Case 唯一 Reference。
- 用户要求“只返回/仅查看”指定列时，查询必须在工具层传 `--field-ids <ID1,ID2>`。不要先拉取全部字段再只在最终答复中删列；字段投影既是结果契约，也是降低响应 token 的手段。
- `+record-query` 必须传真实 `base-id` 和 `table-id`。URL 先用 `+url-resolve`；名称先用 `+resolve-base` / `+resolve-table` 唯一解析，禁止自动选第一项。
- 单页 `limit` 为 1-100。返回 `data.records`、`data.hasMore`，存在后续页时还会返回 `data.nextCursor`；把该值原样传给下一次 `--cursor`。
- `+record-query` 不提供 `--all`；明确要求全量时改用原子 `record query --all --page-limit <N>`。达到页上限后仍有 `hasMore=true` 代表截断，应从返回 cursor 续跑；只有 `hasMore=false`，或按 `record-ids` 查询且所有请求 ID 均已返回时，才能声称结果完整。

## 结果与恢复

- 批量结果必须检查 completed、failed、verification 与 checkpoint；`partial_success` 不是完成，只从 checkpoint 续跑。
- 写入效果未知时按真实 recordId 回读，不重放成功批次；只有独立读回与目标一致时才能报告已完成。

## 新增

当前没有 `+record-create`，使用原子命令：

```bash
dws aitable record create --base-id <B> --table-id <T> \
  --records '[{"cells":{"fldText":"内容"}}]' --format json
```

长 JSON 写到 cwd 内相对文件后使用 `--records-file ./records.json`。从真实返回的 `data.newRecordIds[]` 取 recordId，再用 `+record-query --record-ids` 回读；用户限定返回列时同时传 `--field-ids`。不要从输入顺序、名称或行号推断 ID。

## 更新、同步与批量修改

已知 recordId：

```bash
dws aitable +record-update --base-id <B> --table-id <T> \
  --records '[{"recordId":"<R>","cells":{"fldStatus":"完成"}}]'
```

每条更新只传 `recordId` 与本次确实需要修改的 `cells`。禁止先读取完整旧记录，再把所有旧字段回填到更新 payload；未修改字段不得出现，避免覆盖并发修改或触发只读字段错误。

`+record-update` 自动按 100 条分片并逐批回读；该 shortcut 只接受 `--records`。超长文件输入需要改用原子 `record update --records-file`，不要给 shortcut 猜造 flag。

数值/货币写入使用 JSON number；读回的等价十进制字符串（例如 `90000` 与 `"90000.00"`）视为同一业务值。单选、多选可按 option name 或真实 option ID 写入，读回可能展开为 `{id,name}` 或对象数组；`+record-update` 会按单选 ID/name、无序多选集合做语义比较。若仍返回 `unknown`/read-back mismatch，禁止重放更新，只执行返回的唯一 `nextCommand` 独立核验。

按业务唯一键同步使用 `+record-upsert-by-key`：0 条创建、1 条更新、多条冲突停止。非字符串键把 `--key-value` 改为 `--key-value-json`。

```bash
dws aitable +record-upsert-by-key --base-id <B> --table-id <T> \
  --key-field-id <F> --key-value <值> --cells '<CELLS_JSON>'
```

按条件批改使用 `+record-bulk-patch`，必须提供 filters/query/record-ids 中至少一种选择条件，并设置合理 `--max-matches`；禁止无边界整表写。

```bash
dws aitable +record-bulk-patch --base-id <B> --table-id <T> \
  --query <关键词> --patch '<CELLS_JSON>' --max-matches <N>
```

## 删除

```bash
dws aitable +record-delete --base-id <B> --table-id <T> --record-ids <R1,R2>
```

删除不可逆。只使用已确认的真实 recordId；shortcut 自动分片并验证记录已不存在。未知结果按 recordId 回读，不重放已完成批次。

## 常用字段值

| 字段类型 | 写入值 |
|---|---|
| 文本、单选 | 字符串；单选使用已有选项名称 |
| 多选 | 选项名称数组 |
| 数字、评分 | JSON number |
| 复选框 | boolean |
| 日期 | 按字段配置要求的时间值；不凭展示文本猜格式 |
| URL | 按当前字段 Schema 要求的对象或字符串 |
| 人员、关联记录 | 使用真实 userId/recordId，不用姓名代替 |
| 附件 | 先用 `+attachment-put` 获得 AITable 附件 token，再写字段 |

公式、查找引用、创建人/时间、修改人/时间等只读字段不得写入。字段类型不明时只读取目标字段配置一次。

## 历史、分享与主键文档

- 记录历史：`dws aitable +record-history-list --base-id <B> --table-id <T> --record-id <R>`。已有真实 recordId 直接执行，不扫描 Help 或产品 Catalog。
- 批量记录分享：`dws aitable +record-share-links --base <B> --table <T> --record-ids <R1,R2>`；单条也可用 `+record-share-url`。
- 用户要求把分享链接“发给”联系人时，AITable 的职责在链接生成后结束；随后加载 `dingtalk-chat`，用 `dws chat +dm --to <姓名> --text <包含全部链接的文本>` 对每位收件人分别发送并检查真实回执。只解析联系人或只生成 URL 都不算完成。
- 主键文档：`+record-primary-doc-get` / `+record-primary-doc-create`

创建主键文档必须显式传 primaryDoc 类型的 `--field-id`；字段类型不明时先读取目标字段。正文读写切到 Doc；这里仅管理记录与文档关联。
