# MCP express 表达式函数

仅在 reference、fixed 和系统变量不能完成转换时使用本页。平台函数目录共 7 组 82 个函数。

表达式写入映射的 `expression`，不要写入 `source`：

```json
{
  "type":"express",
  "expression":"COALESCE(${@(\"node_start/$/name\")},'unknown')",
  "displayText":"为空时使用 unknown",
  "target":"$.Query.name"
}
```

变量引用：工具输入 `${@("node_start/$/<key>")}`，系统参数对象 `${@("system_node/$")}`。字符串字面量使用单引号。

## 序列化注意

- boolean 映射到 string 时是 `true/false`；double 运算结果可能带 `.0`，需要整数文本时组合 `INT` 或 `TEXT`。
- date 直接映射通常是毫秒时间戳；可读文本使用 `DATEFORMAT`。
- collection 直接映射可能只保留首项；映射到 string 前使用 `JOIN`。
- `deapRunId` 等会话参数在 tool debug 环境中可能为空。
- 平台推荐映射中出现但不在本目录的函数属于内部函数，不要凭空手写。

## 集合函数（10）

| 函数 | 参数 | 返回 | 作用 |
|---|---|---|---|
| `IN` | any, collection | boolean | 判断元素是否在集合中 |
| `INTER` | collection, collection... | collection | 集合交集 |
| `SUPPLE` | collection, collection... | collection | 集合补集 |
| `LIST` | any... | collection | 构造集合 |
| `UNION` | collection, collection... | collection | 集合并集 |
| `COALESCE` | any... | any | 返回首个非空值 |
| `LISTITEM` | collection, number | any | 按从 1 开始的下标取元素 |
| `SIZE` | collection | number | 集合大小 |
| `FOREACH` | collection, expression | collection | 对每项执行表达式，当前项用 `_` |
| `REPEAT` | number, any | collection | 重复元素构造集合 |

## 日期函数（13）

| 函数 | 参数 | 返回 | 作用 |
|---|---|---|---|
| `DATEDELTA` | date, number, unit? | date | 日期加减 |
| `DATEDIF` | date, date, unit? | number | 计算日期差；单位支持 y/M/d/h/m/s |
| `DATE` | string or number | date | 从日期文本或时间戳构造日期 |
| `DAY` | date | number | 日 |
| `HOUR` | date | number | 小时 |
| `MINUTE` | date | number | 分钟 |
| `MONTH` | date | number | 月 |
| `NETWORKDAYS` | date, date, holidays... | number | 计算工作日数量 |
| `NOW` | none | date | 当前时间 |
| `SECOND` | date | number | 秒 |
| `TIMESTAMP` | date | number | 转毫秒时间戳 |
| `YEAR` | date | number | 年 |
| `DATEFORMAT` | date, string | string | 按格式输出日期文本 |

## 逻辑函数（14）

| 函数 | 参数 | 返回 | 作用 |
|---|---|---|---|
| `AND` | boolean or collection, boolean... | boolean | 全部为真 |
| `EQ` | any, any | boolean | 相等 |
| `FALSE` | none | boolean | false |
| `GE` | number/date, number/date | boolean | 大于等于 |
| `GT` | number/date, number/date | boolean | 大于 |
| `IF` | boolean, any, any | any | 条件分支 |
| `ISEMPTY` | any | boolean | 判空；纯空格先 `TRIM` |
| `LE` | number/date, number/date | boolean | 小于等于 |
| `LT` | number/date, number/date | boolean | 小于 |
| `NE` | any, any | boolean | 不相等 |
| `NOT` | boolean | boolean | 逻辑取反 |
| `OR` | boolean or collection, boolean... | boolean | 任一为真 |
| `TRUE` | none | boolean | true |
| `XOR` | boolean, boolean | boolean | 异或 |

## 数学函数（15）

| 函数 | 参数 | 返回 | 作用 |
|---|---|---|---|
| `ABS` | number | number | 绝对值 |
| `AVERAGE` | number... | number | 平均值 |
| `CEILING` | number, significance | number | 向上舍入到指定基数倍数 |
| `FIXED` | number, digits | number | 向下保留指定位数 |
| `INT` | number | number | 向下取整 |
| `MAX` | number or collection, number... | number | 最大值 |
| `MIN` | number or collection, number... | number | 最小值 |
| `MOD` | number, divisor | number | 余数 |
| `PI` | none | number | 圆周率 |
| `POWER` | number, power | number | 幂 |
| `PRODUCT` | number... | number | 乘积 |
| `RAND` | none | number | `[0,1)` 随机数 |
| `ROUND` | number, digits | number | 四舍五入 |
| `SUM` | number or collection, number... | number | 求和 |
| `NUMRANGE` | number, start, end, mode? | boolean | 区间判断；mode 为 closed/open/leftOpen/rightOpen |

## 字符串函数（21）

| 函数 | 参数 | 返回 | 作用 |
|---|---|---|---|
| `CONCATENATE` | any... | string | 拼接 |
| `CONTAIN` | string, string | boolean | 包含判断 |
| `EXACT` | string, string | boolean | 区分大小写精确比较 |
| `LEFT` | string, number | string | 左侧字符 |
| `LEN` | string | number | 长度 |
| `LOWER` | string | string | 转小写 |
| `MID` | string, start, length | string | 从 1 开始截取中间字符 |
| `REPLACE` | string, start, count, string | string | 按位置替换 |
| `REPT` | string, number | string | 重复文本 |
| `RIGHT` | string, number | string | 右侧字符 |
| `SPLIT` | string, separator | collection | 分割文本 |
| `STARTWITH` | string, string | boolean | 前缀判断 |
| `TEXT` | any, format? | string | 转文本，可格式化日期 |
| `TRIM` | string | string | 去首尾空格 |
| `UPPER` | string | string | 转大写 |
| `VALUE` | string | number | 文本转数字 |
| `GETUUID` | none | string | 生成 UUID |
| `MD5` | string | string | MD5 小写十六进制摘要 |
| `JOIN` | collection, separator | string | 连接集合 |
| `TEXTREPLACE` | string, search, replacement | string | 按内容替换 |
| `UNESCAPEHTML` | string | string | 解码 HTML 实体 |

## JSON 函数（5）

| 函数 | 参数 | 返回 | 作用 |
|---|---|---|---|
| `JSONPARSE` | string | any | JSON 文本转对象 |
| `GET` | string, any | any | 按 key 取 JSON 值，适合特殊字符 key |
| `JACKSONJSONPATHEVAL` | any, string | any | 使用 JSONPath 取值 |
| `JSONTOSTRING` | any | string | JSON 对象序列化 |
| `SORTED_JSONSTRING` | any or string | string | 输出字段有序的 JSON 文本 |

## 系统函数（4）

| 函数 | 参数 | 返回 | 作用 |
|---|---|---|---|
| `USERID2UNIONID` | corpId, userId | string | userId 转 unionId |
| `UNIONID2USERID` | corpId, unionId | string | unionId 转 userId |
| `BATCHUSERID2UNIONID` | corpId, userId collection | collection | 批量 userId 转 unionId |
| `BATCHUNIONID2USERID` | corpId, unionId collection | collection | 批量 unionId 转 userId |

## 组合示例

```text
COALESCE(${@("node_start/$/nickname")},'anonymous')
DATEFORMAT(DATE(${@("node_start/$/timestamp")}), 'yyyy/MM/dd HH:mm:ss')
JOIN(SPLIT(${@("node_start/$/csv")},','),'-')
TEXT(INT(POWER(${@("node_start/$/base")},2)))
GET('operateUserId',${@("system_node/$")})
```

表达式配置后必须 `tool get` 回读，再用非空业务输入执行 `tool debug`。函数存在不代表下游目标类型会自动转换正确，最终以接口实际收到的值和 `toolOutput` 为准。
