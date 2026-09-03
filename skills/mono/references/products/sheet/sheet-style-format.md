# Sheet 样式、数字格式与合并

## 三层职责

| 需求 | 路径 |
|---|---|
| 写少量值时给每格不同样式 | range update 的 cellStyles |
| 给一个区域统一或二维设置样式 | range set-style |
| 多区域原子设置样式 | range batch-set-style |
| 改单个 richText 片段外观 | richText 子项 style |

不要为了样式重写已有值。值与样式可分阶段：先写值并回读，再设置样式并验证。

`sheet create-with-data --styles` 的顶层单项只接受 `name`、`cell_styles`、`row_sizes`、`col_sizes`、`cell_merges`；未知键会在创建前拒绝。每项至少包含一种样式操作，且数据写入后的样式阶段按上述顺序执行、不是原子事务。

## 区域样式

    dws sheet range set-style       --node <NODE_ID> --sheet-id <SHEET_ID> --range "A1:D1"       --bg-color "#FFF2CC" --font-weight bold --h-align center       --word-wrap autoWrap --format json

只有需要逐格不同值时才用相应的二维 JSON flag，且数组维度必须与 range 一致。颜色使用有效十六进制；不要猜不在当前 compact Schema 中的枚举。

可直接使用的统一值参数包括 bg-color、font-size、h-align、v-align、font-color、font-weight、word-wrap、number-format、font-style、font-line、font-family、border-styles-json；逐格不同只对 bg/font-size/h-align/v-align/font-color/font-weight 使用对应 `*-json` 二维矩阵。关键枚举：h-align=left/center/right/general，v-align=top/middle/bottom，word-wrap=overflow/clip/autoWrap，font-weight=bold/normal，font-style=normal/italic，font-line=none/underline/line-through。

边框 JSON 只接受 top/bottom/left/right 四个边，每边只接受 style 和可选 color：

    --border-styles-json '{"top":{"style":"solid","color":"#000000"},"bottom":{"style":"medium"}}'

style 使用 solid/medium/thick/dashed/dotted/double/hair/none 等当前枚举；粗细包含在 style 中，不要发明 width。

批量同样式：

    dws sheet range batch-set-style       --node <NODE_ID>       --ranges '["Sheet1!A1:D1","Sheet2!A1:D1"]'       --font-weight bold --format json

--ranges 每项必须带工作表前缀，最多 100 项。不同区域不同样式用 --batch 配置文件。默认严格事务，任一失败整批回滚；只有用户明确接受部分成功才用 --continue-on-error，且必须逐项报告并验证，不能把 partial 称为成功。

`--ranges` 与 `--batch` 必须二选一。ranges 模式用命令行样式应用到所有范围；batch 模式的样式只从本地 JSON 文件读取，不能同时传任何命令行样式参数。batch 文件是数组，每项必须含 sheetId、range，可带与命令行同语义的 camelCase 字段，例如：

    [
      {"sheetId":"Sheet1","range":"A1:B3","bgColor":"#FFF2CC","fontWeight":"bold","borderStylesJson":"{\"bottom\":{\"style\":\"solid\"}}"},
      {"sheetId":"Sheet2","range":"C1:C5","numberFormat":"¥#,##0.00"}
    ]

每个区域必须满足 rows<=1000、cells<=30000；最多 100 个区域，所有区域累计不得超过 200000 单元格。超出时按独立批次拆分，不能把拆分后的多次调用描述为一次原子事务。

## 常用 number-format

| 目标 | code |
|---|---|
| 数字形态 ID、订单号、手机号、工号 | @ |
| 整数 / 两位小数 | 0 / 0.00 |
| 千分位 | #,##0 或 #,##0.00 |
| 人民币 / 美元 | ¥#,##0.00 / $#,##0.00 |
| 百分比 | 0% 或 0.00% |
| 日期 | yyyy-mm-dd |
| 日期时间 | yyyy-mm-dd hh:mm:ss |

数字格式只影响展示，不修复已经以浮点数损失精度的长 ID。此类值必须先以字符串写入并配合 @。

## 合并与取消合并

    dws sheet merge-cells       --node <NODE_ID> --sheet-id <SHEET_ID>       --range "A1:C3" --merge-type mergeRows --format json

merge-type 为 mergeAll（默认）、mergeRows 或 mergeColumns。合并只保留各合并块左上角的值；若其它格已有内容，先读并向用户说明数据丢失风险。不要用合并模拟视觉居中。

    dws sheet unmerge-cells       --node <NODE_ID> --sheet-id <SHEET_ID>       --range "A1:C3" --format json

合并或取消后用 info 的 mergedRanges 验证。行列操作可能破坏合并边界，应在进入 dimension 阶段前传播当前 mergedRanges。

## 完成条件

样式命令成功后，使用能够返回样式结构的最小范围读取；合并使用 info。验证关键字段而非整表回读。失败、回读不一致或 continue-on-error 中存在失败项时，明确报告未完成部分。
