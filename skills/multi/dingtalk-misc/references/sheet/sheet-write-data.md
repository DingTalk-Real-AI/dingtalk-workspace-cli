# Sheet 写入数据

## 进入本阶段的条件

仅在任务需要富文本、单元格超链接、数据验证、per-cell 样式、结构化 table 写入，或需要在 csv-put / range update / append 之间选择时读取本文件。普通二维值写入优先遵循上层 sheet.md。

## 选择最短写入路径

| 目标 | 命令 | 关键边界 |
|---|---|---|
| 超过 5 行或 20 个单元格的纯值/公式 | dws sheet csv-put | stdin 优先；最多 2M 字符、30000 单元格；覆盖已有内容必须显式 --allow-overwrite |
| 少量值、富文本、链接、数据验证、逐格样式 | dws sheet range update | --values 必须与目标范围行列数完全一致 |
| 末尾追加同构记录 | dws sheet append | 每一行列数一致；不要先读取并手算最后一行 |
| columns/data/dtypes/formats 协议 | dws sheet table-put | 多工作表用 --sheets；总单元格数含表头不超过 30000 |

所有既有工作表写入都必须使用真实 sheetId。当前上下文没有时只调用一次：

    dws sheet +list-sheets --node <NODE_ID> --format json

按完整标题唯一匹配；禁止猜 Sheet1、0、default。后续阶段复用同一个 nodeId、sheetId 和 profile。

## 小范围与富格式写入

    dws sheet range update --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B2" --values '[[{"type":"text","text":"名称"},{"type":"text","text":"链接"}],[{"type":"text","text":"项目"},{"type":"text","text":"详情","hyperlink":{"type":"path","link":"https://example.com"}}]]' --format json

硬约束：

- 二维数组中的每格必须是 JSON object，不能直接传 string/number/boolean/null。写值时 type 只取 text 或 richText，数字和布尔值也按 text 字符串传递；只改 hyperlink/dataValidation/cellStyles 时可以省略 type，以保留原值。
- 用 {} 跳过单元格并保留原值；清空单个格传 {"type":"text","text":""}，清空整片范围用 range clear。
- 单元格级 hyperlink 与 richText 片段链接不要混用。取消整格链接使用 `hyperlink:{"type":"none"}`；不传 hyperlink 表示保留原链接，不能用 null 猜测清除语义。
- richText 仅在确实需要多片段样式、片段链接、附件或图片时使用；媒体 resourceId/resourceUrl 必须来自本任务 media-upload 的真实返回。
- dataValidation 是三态：不传表示保留原规则；传 `{"dataValidation":{"type":"none"}}` 表示清除；传 `dropdown` 或 `checkbox` 表示写入新规则。dropdown 的 `options` 与 `sourceRange` 必须且只能传一个；sourceRange 使用 `{"sheetId":"真实ID","a1Notation":"A1:A3"}`，不能传猜测名称。
- cellStyles 适合“写值时顺带给少量格设置样式”，也可单独传 `{"cellStyles":{...}}` 只改样式并保留原值；整片统一样式进入 sheet-style-format 阶段。
- 公式以 = 开头；需要字面量等号时前加单引号。
- 单次建议不超过 1000 行、5000 个单元格；总量不得超过当前命令契约的 30000 单元格。超出时按连续不重叠范围拆分，每块分别回读。
- 目标范围与 mergedRanges 冲突时，range update 会返回 `MERGED_CELLS_CONFLICT`。先用 info 定位冲突范围，必要时经用户同意取消合并，写入后再按原意恢复；不能把错误当成空结果。
- SourceRange 下拉在结构移动后必须回读 `sourceRangeStatus`。同一 cell 的值/样式可能已写入，但下拉创建失败时服务端仍可能返回 success=true 并把失败写在 message；必须同时检查 message 和 range read 的 dataValidation。

## 大块纯值写入

优先 stdin，避免大 JSON 和 shell 转义：

    dws sheet csv-put       --node <NODE_ID> --sheet-id <SHEET_ID> --start-cell A1       --csv - --format json

先比较目标范围是否已有内容。只有用户授权覆盖，或该范围由本任务新建且尚未交付时，才加 --allow-overwrite。达到 30000 单元格上限时按不重叠连续块分批，每块写后只回读该块；不要把截断或部分写入称为成功。

CSV 必须使用 ASCII 英文逗号 `,`；中文逗号 `，` 不会分列，会把整行写进一个单元格。目标区域含合并单元格时 csv-put 会打散合并并写入，这与 range update 的冲突失败语义不同；需要保留合并时先记录 mergedRanges，写完再恢复。csv-put 只承载值和公式，不承载样式、超链接、richText 或 dataValidation。

### 自动类型转换与文本保真

普通 CSV 写入沿用默认自动类型转换，示例省略该 flag：

```bash
dws sheet csv-put --node <NODE_ID> --sheet-id <SHEET_ID> --start-cell A1 \
    --csv 'name,score\nAlice,95\nBob,87'
```

用户明确要求“保留前导零 / 不要转日期 / 按文本原样导入 / 禁止类型推断”时才添加 `--auto-convert=false`：

```bash
dws sheet csv-put --node <NODE_ID> --sheet-id <SHEET_ID> --start-cell A1 \
    --auto-convert=false \
    --csv $'id,date,total\n001,2026/8/1,"=SUM(1,2)"'
```

`--auto-convert=false` 只关闭非公式字段的类型推断：`001`、`12.10`、`1E3`、`2026/8/1`、`85%`、`TRUE` 都按原始文本写入；RFC 4180 解码后首字符为 `=` 的字段仍是公式，`'=...` 则作为普通文本保留前置单引号。用户希望识别数字、日期、百分比或布尔时，省略 `--auto-convert`，使用默认 `true`；不要给所有 `csv-put` 无条件添加该参数。有明确列类型要求时改用 `table-put` 的 `dtypes` / `formats`。

## 结构化 table 写入

    dws sheet table-put --node <NODE_ID> --sheets '{"name":"订单","columns":["订单号","金额"],"data":[["9007199254740993",12.5]],"dtypes":{"订单号":"object","金额":"float64"},"formats":{"订单号":"@","金额":"0.00"}}' --format json

table-put 只有 --sheets 数据入口，接受单个 spec、spec 数组或 `{ "sheets": [...] }`，也可用 @文件和 stdin；不要发明 --columns/--data 等顶层 flag。table-put 不放入 batch-update。

单个 sheet spec 的最小契约：

- name 与 sheetId 二选一；name 不存在时创建同名工作表，sheetId 优先且不得猜测。
- columns 必填、非空、列名去空白后非空且不重复；data 默认空数组，每行宽度必须等于 columns 长度。
- startCell 默认 A1；mode 取 overwrite（默认）或 append。header 在 overwrite 默认 true、append 默认 false；append 到空表且未显式设置时写表头。
- allowOverwrite 默认 true；需要保护已有值时显式 false。不要把 csv-put 的默认 false 套到 table-put。
- dtypes、formats 的键必须来自 columns；单元格值只用 string/number/boolean/null。复用 table-get 时，data 中 `{}` 按空位/null 处理。
- 单表最多 30000 单元格，包含表头。table-put 不支持 dataValidation、hyperlink、richText、附件或单元格图片；这些能力改用 range update/write-image。

商品 ID、订单号、手机号、工号以及超过 2^53-1 的整数必须以 JSON 字符串传入，并在 dtypes/formats 中按列名声明 object 与 @，避免精度损失。写完用 table-get 验证 columns/data/dtypes/formats；table-get 不返回 cellStyles，样式另用 range read 验证。

## 写后验证

写命令成功只证明请求已接收。必须读回最小受影响范围：

    dws sheet csv-get       --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:B2"       --format json

检查 returnedRange、hasMore、truncationReasons 和关键值。公式先用 value-render-option=formula 确认公式文本，再在需要时用 raw_value 或 formula-verify 验证计算结果。富文本、链接或数据验证使用 range read 获取 per-cell 结构；样式与合并用对应对象读命令。

对“写成功后立即读为空”的一致性延迟，只重试只读校验，采用 0ms、250ms、500ms、1s 的有界退避；不得重放非幂等写入。四次仍不一致则明确失败，不把空结果伪装成成功。
