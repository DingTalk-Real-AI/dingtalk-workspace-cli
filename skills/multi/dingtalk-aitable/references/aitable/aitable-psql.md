# AI 表格 PostgreSQL 只读查询

## 适用场景

用户明确表达以下任一意图时，使用 `dws aitable psql`：

- 使用 SQL / PostgreSQL / `SELECT` 查询 AI 表格
- 查看 PostgreSQL 逻辑表清单或逻辑列类型
- 单表投影、过滤、聚合、分组、排序和分页
- 最多 8 张同 Base 数据表的 `INNER`、`LEFT`、`RIGHT`、`FULL OUTER JOIN`；`CROSS JOIN` 仅支持两张表
- `COUNT`、`SUM`、`AVG`、`MIN`、`MAX` 聚合，以及 `ROW_NUMBER`、`RANK`、`DENSE_RANK` 窗口函数

普通的“查几条记录”“按字段筛选记录”仍使用 `record query`。读取字段配置、选项、公式配置时仍使用 `field get`；只有用户关心 SQL 可查询列及 PostgreSQL 类型时才使用 `psql -t`。

## 命令模式

`-l`、`-t`、`-c` 三种模式互斥。`-t` 仅用于查看表结构；执行 SQL 时表由 `FROM` / `JOIN` 自动解析，无需传 `-t`。

### 命令生成约束

- 必须按以下模板生成完整命令，不得省略 `-d` 及其值。
- `<BASE_ID>`、`<TABLE_ID>` 是文档占位符；实际执行前必须替换为上下文中的真实 ID，并用单引号包裹。
- 仅使用 `-d`、`-l`、`-t`、`-c`；仅当用户显式要求查看全部扩展属性时才添加 `--all-properties`；禁止改写为 `--base-id`、`--table-id`、`--sql` 或 `--base`。
- 查看表结构时，默认只查看与 AI 表格页面一致的基础字段（默认属性），不得因字段类型复杂而自动添加 `--all-properties`。
- 不得使用不存在的 `dws aitable table list` 作为回退；表发现只能使用 `psql -d '<BASE_ID>' -l`。
- 本文已明确命令契约时，不得通过 `dws aitable psql --help` 判断功能是否存在。
- 执行前向用户展示命令时，必须展示即将执行的完整命令，不能简写为 `dws aitable psql -l`。
- 命令失败时保留并依据真实错误处理；不得据此猜测“命令尚未实现”或“功能未发布”。

```bash
# 列出 Base 内可通过 PostgreSQL 查询的逻辑表
dws aitable psql -d <BASE_ID> -l

# 默认查看与页面一致的基础字段；仅用户显式要求全部扩展属性时增加 --all-properties
dws aitable psql -d <BASE_ID> -t <TABLE_ID>
dws aitable psql -d <BASE_ID> -t <TABLE_ID> --all-properties

# 执行只读 SQL
dws aitable psql -d <BASE_ID> \
  -c 'SELECT * FROM "数据表1" LIMIT 10'
```

`psql` 输出是面向用户的 PostgreSQL 表格文本，不支持也不添加全局 `--format json`。这是 AI 表格 Skill 中“结构化读取使用 `--format json`”规则的明确例外。

## 自然语言路由

| 用户意图 | 执行方式 |
|---|---|
| “这个 AI 表格里有哪些可查询的数据表” | `psql -d <baseId> -l` |
| “数据表1有哪些 SQL 字段和类型” | 先 `-l` 解析真实 `tableId`，再 `psql -d <baseId> -t <tableId>` |
| “查询数据表1前 10 条”且上下文明确要求 SQL | 先核对逻辑表结构，再执行 `SELECT * ... LIMIT 10` |
| “把数据表1、数据表2和数据表3关联起来” | 先列出表并查看每张表的结构，再生成多表 JOIN SQL；表由 SQL 自动解析 |
| “按业务状态统计数量”或明确要求 SQL 聚合 | 先查看逻辑结构，再生成使用 `COUNT/SUM/AVG/MIN/MAX` 的分组或聚合 SQL |
| “按分组排名/生成行号” | 先查看逻辑结构，再生成使用 `ROW_NUMBER/RANK/DENSE_RANK ... OVER (...)` 的 SQL |

用户只给表名时，必须先用 `psql -l` 获取真实 `tableId`；零命中或重名时要求用户消歧，禁止猜测。编写 SQL 前必须用 `psql -t` 核对实际逻辑列名和 PostgreSQL 类型。

## SQL 生成规则

- 仅允许一条标准 PostgreSQL / pgsql 语法的只读 `SELECT`；禁止 `INSERT`、`UPDATE`、`DELETE`、DDL 和多语句。
- 必须使用 PostgreSQL 方言：禁止 MySQL/SQLite/Oracle 等非 PostgreSQL 写法，例如反引号标识符、`LIMIT offset,count`、`IFNULL`、`DATE_FORMAT`、`NVL`、`DUAL` 等。
- SQL 生成完成后、执行前必须自检：所有来自 AI 表格元数据的逻辑表名、逻辑字段名和中文结果别名均使用双引号；SQL 关键字、函数名、表别名不加引号。禁止依赖未加引号的中文标识符“恰好可被解析”。
- 双引号仅用于 SQL 标识符，单引号仅用于字符串字面量。例如必须生成 `SELECT COUNT(*) AS "记录数" FROM "核销明细" LIMIT 200`，不得生成 `SELECT COUNT(*) AS 记录数 FROM 核销明细`，也不得将字符串写成双引号。
- 命令中的完整 SQL 使用 shell 单引号包裹，SQL 标识符使用双引号；不得用 shell 外层双引号代替 SQL 标识符的双引号。
- `-c` 传入的 SQL 末尾不得包含分号（`;`）；服务会将 SQL 封装为子查询，尾部分号会导致 PostgreSQL 语法错误。
- JOIN 表必须属于同一个 Base；最多引用 8 张表，执行时由 SQL 的 `FROM` / `JOIN` 自动解析表。
- 支持 `INNER`、`LEFT`、`RIGHT`、`FULL OUTER JOIN`；`ON` 仅支持相同 PostgreSQL 标量类型的跨表字段等值比较及 `AND` 组合。
- `CROSS JOIN` 仅支持两张表，不能串联其他 JOIN，且不得携带 `ON` 或 `USING`。
- 支持聚合函数 `COUNT`、`SUM`、`AVG`、`MIN`、`MAX`；`GROUP BY` 仅支持列引用，`HAVING` 仅用于分组或聚合查询。
- 支持窗口函数 `ROW_NUMBER`、`RANK`、`DENSE_RANK`，以及上述聚合函数的 `OVER` 形式；不生成命名 `WINDOW`、窗口框架或窗口内 `DISTINCT`。
- 支持多列 `ORDER BY`、`ASC/DESC`、`NULLS FIRST/LAST`、`LIMIT` 和 `OFFSET`。
- 默认最多返回 200 行；可用 `--limit 1..1000` 和 `--timeout 1..60` 限制执行。
- 用户未要求全部数据时主动添加合理的 `LIMIT`；用户要求全量时也受 `--limit` 上限约束，应明确说明。

## 多表 JOIN 示例

```bash
dws aitable psql \
  -d '<BASE_ID>' \
  -c 'SELECT a."业务名称", COUNT(c."文本") AS "数量" FROM "数据表1" a LEFT JOIN "数据表2" b ON a."业务名称" = b."文本" LEFT JOIN "数据表3" c ON b."文本" = c."文本" GROUP BY a."业务名称"'
```

## 执行流程

1. 从用户提供且已确认是 AI 表格的 URL 提取 `baseId`；没有 URL 或 ID 时按现有 Base 搜索流程定位。
2. 执行 `psql -d <baseId> -l`，获取真实表名与 `tableId`。
3. 对 SQL 涉及的每张表执行 `psql -d <baseId> -t <tableId>`，默认仅查看与页面一致的基础字段；仅当用户显式要求查看全部扩展属性时增加 `--all-properties`。
4. 根据用户自然语言生成一条只读 PostgreSQL SQL；遇到重名或语义不明确先询问。
5. 执行 `psql -d ... -c ...`，由 SQL 中的 `FROM` / `JOIN` 确定目标表，并将原始 psql 表格结果返回用户。

不得为了模拟真实用户而要求用户自己编写 SQL、提供 fieldId，或在每次对话中粘贴 dws 命令；Agent 应完成表和列发现、SQL 生成与执行。
