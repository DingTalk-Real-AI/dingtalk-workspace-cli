# Sheet 条件格式

## 边界

条件格式是随单元格值变化的规则对象，不是一次性静态样式。只要求固定外观时进入 sheet-style-format。ruleId 必须来自本任务 create/list；所有操作使用真实 nodeId、sheetId。

若用户明确要求新增“判断结果/是或否”辅助列，必须先写可见辅助列，再基于辅助列创建规则；不能用一步 formulaCondition 隐藏掉用户要求的数据产物。

## 创建前

- 读取最小数据范围，确认首行、空值和数据类型。
- ranges 是 JSON 数组，使用精确 A1 范围，不扩大到无界整列。
- 日期或公式条件要处理空单元格，避免空值被当作 0/日期触发。日期到期示例使用 `=AND(E1<>"",E1<=TODAY())`；相对引用随行变化，只有明确固定比较一个格时才用绝对引用。
- 每条规则的 condition 只能选一种结构。常用精确形态：
  - numberCondition：operator=equal/not-equal/greater/greater-equal/less/less-equal/between/not-between；value1 必填，between/not-between 还需 value2。
  - textCondition：operator=contains/not-contains/starts-with/ends-with，value 为文本。
  - emptyCondition/errorCondition/duplicateCondition：operator 分别取 is-empty/is-not-empty、error/no-error、duplicate/unique。
  - formulaCondition：`{"formula":"=A1>100"}`。
  - rankCondition：value、isPercent、isBottom；averageCondition：isAbove、andEqual；stdevCondition：value、isAbove、andEqual。
  - dataBarCondition：minPoint/maxPoint，各点 type 取 auto/maxmin/number/percent/percentile/formula，可带 value；样式单独用 data-bar-style。
  - iconSetCondition：iconSet 数组项包含 criteria `{type,value,gtOrEqual}` 和 icon `{type:"id",value:...}`，可带 showIconOnly。
  - colorScaleCondition：criterias 为 2 或 3 项，每项 `{type,value?,color}`。
- cell-style 只使用 backgroundColor、fontColor、bold、italic、strikethrough。“标红/高亮/染色”默认 backgroundColor，只有明确“字体红”才用 fontColor。不确定扩展字段时读 create 的 compact Schema。

## 创建

数值阈值示例：

    dws sheet cond-format create       --node <NODE_ID> --sheet-id <SHEET_ID>       --ranges '["A2:A100"]'       --condition '{"numberCondition":{"operator":"greater","value1":"80"}}'       --cell-style '{"backgroundColor":"#FFCDD2","fontColor":"#B71C1C","bold":true}'       --format json

辅助列示例，分两个阶段：

    dws sheet range update       --node <NODE_ID> --sheet-id <SHEET_ID> --range "H2:H4"       --values '[[{"type":"text","text":"=IF(A2>B2,\"是\",\"否\")"}],[{"type":"text","text":"=IF(A3>B3,\"是\",\"否\")"}],[{"type":"text","text":"=IF(A4>B4,\"是\",\"否\")"}]]'       --format json

    dws sheet cond-format create       --node <NODE_ID> --sheet-id <SHEET_ID>       --ranges '["A2:H4"]'       --condition '{"formulaCondition":{"formula":"=$H2=\"是\""}}'       --cell-style '{"backgroundColor":"#FFECEC"}'       --format json

先回读 H2:H4 的公式和值，再创建规则。不要把公式条件视觉正确等同于辅助列已经存在。

数据条样式示例：

    --condition '{"dataBarCondition":{"minPoint":{"type":"auto"},"maxPoint":{"type":"auto"}}}' --data-bar-style '{"fill":["#4CAF50","#F44336"],"isGradient":true}'

三色色阶示例：

    --condition '{"colorScaleCondition":{"criterias":[{"type":"maxmin","color":"#F44336"},{"type":"percentile","value":"50","color":"#FFEB3B"},{"type":"maxmin","color":"#4CAF50"}]}}'

## 查询、更新、删除

    dws sheet cond-format list       --node <NODE_ID> --sheet-id <SHEET_ID>       --rule-id <RULE_ID> --format json

update 是部分更新：至少传 ranges、condition、cell-style、data-bar-style 之一，未传字段保持不变；传 condition 会替换原条件类型。先 list 单个对象确认当前类型与范围，再只提交用户要求的字段，完成后重新 list。

    dws sheet cond-format update       --node <NODE_ID> --sheet-id <SHEET_ID>       --rule-id <RULE_ID>       --condition '{"numberCondition":{"operator":"greater","value1":"90"}}'       --format json

删除规则不删除原始数据，但会永久移除视觉规则；规则已不存在时按幂等成功处理。展示 ruleId、范围和条件，得到明确同意后：

    dws sheet cond-format delete       --node <NODE_ID> --sheet-id <SHEET_ID>       --rule-id <RULE_ID> --yes --format json

create/update 后 list 验证 ID、ranges、condition 和样式；delete 后验证规则消失。命令失败、JSON 损坏或列表缺字段不能解释为空规则。
