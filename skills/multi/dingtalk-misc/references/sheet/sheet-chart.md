# Sheet 浮动图表

## 真对象约束

用户要求图表时必须创建 Sheet chart 对象，不能用单元格字符、静态图片或本地绘图冒充。chartId 必须来自本任务 create/list；所有操作复用真实 nodeId、sheetId。

常见映射：分类比较用 column/bar，趋势用 line，构成用 pie/doughnut，两个连续变量关系用 scatter。当前支持的精确类型只有：column、bar、columnStacked、barStacked、line、lineStacked、area、areaStacked、areaPercentStacked、pie、doughnut、scatter、radar。

当前不支持 combo/组合图或双轴组合图。用户要求此类图表时不得发送 `type:"combo"`，也不能谎称已经创建；应说明限制，并在用户接受时改为两个受支持的独立图表。用户已指定其他受支持类型时不擅自替换。

## 创建前

1. csv-get 读取数据范围，确认表头、类别列和系列列。
2. info 查看已用行列与结构。
3. 选择不遮挡数据的 position；宽高为正数。
4. 每个 series/category 的 A1 引用必须位于同一工作表上下文，范围长度匹配。

## 创建与查询

    dws sheet chart create       --node <NODE_ID> --sheet-id <SHEET_ID>       --properties '{
        "position":{"row":12,"col":"A"},
        "dimensions":{"width":600,"height":400},
        "chart":{
          "type":"column",
          "series":[{"name":"B1","value":["B2:B10"]}],
          "category":["A2:A10"],
          "title":{"show":true,"text":"销售数据"}
        }
      }' --format json

properties 顶层必须同时包含 position、dimensions、chart，可选 offset。硬约束：

- position.row 和 position.col 都必填；row 是 0-based 锚点行，col 必须是列字母（如 A、AA），不支持数字列号。
- dimensions.width/height 都必填且为正数。
- chart.type 必须取上面的支持枚举。
- chart.series 必须是非空数组；每项都必须有非空的 value 范围数组。series.name 可引用表头格，category 是类别范围数组。
- series/name/category 使用 A1 表示法；不带工作表前缀时使用 `--sheet-id` 对应工作表。引用跨表数据时显式带工作表前缀，不从当前名称猜测。

复杂轴、图例、颜色等字段只在用户需要时加入，不确定时读取 chart create 的 compact Schema；不得从其他表格产品照搬字段。当前常用配置面：

| 对象 | 已知字段与枚举 |
|---|---|
| `title` | `show:boolean`、`text:string` |
| `legend` | `show:boolean`；`pos` 仅为 `t` / `b` / `l` / `r` / `none` |
| `catAx` / `valAx` | `show:boolean`、`pos:l/t/b/r`、`titleConfig:{show,title}`、`axisMin` / `axisMax` 为 `number` / `null`、`splitLine:boolean`、`minorSplitLine:boolean`、`axisLabel:boolean`、`axisLine:boolean` |

未列出的字段或枚举以精确 leaf Schema 为准，不猜测；更新时仍须先读完整对象再整体回写。

创建后保存返回 chartId，并验证：

    dws sheet chart list       --node <NODE_ID> --sheet-id <SHEET_ID>       --chart-id <CHART_ID> --format json

检查类型、数据范围、标题、位置和尺寸。列表成功但找不到刚创建 ID，不能称为完成。

## 更新是 PUT

chart update 的 properties 为整体覆盖，不是 patch。必须先 list 单个 chart，保留完整 position/dimensions/chart，在本地只改目标字段后整体回写：

    dws sheet chart update       --node <NODE_ID> --sheet-id <SHEET_ID>       --chart-id <CHART_ID> --properties '<完整_PROPERTIES_JSON>'       --format json

只提交局部配置会把未传字段恢复默认或删除。更新后再次 list 单个对象并比较关键字段。

## 删除

删除不可恢复。先 list 获取名称、chartId、数据范围和位置，展示摘要并获得明确同意，然后：

    dws sheet chart delete       --node <NODE_ID> --sheet-id <SHEET_ID>       --chart-id <CHART_ID> --yes --format json

最后 list 验证对象消失。非零、坏 JSON、缺失对象或错误 ID 不得当作成功。
