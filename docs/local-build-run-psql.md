# Agent 本地调试 DWS AITable `psql` 操作手册

本文是本地调试 `dingtalk-workspace-cli` 的 Agent 执行手册。目标是：构建当前分支的 `./dws`，将 AI 表格请求路由到目标 MCP，并由操作者执行 `psql` 查询、向 Agent 回传结果。

> 文中的 `<...>` 均为必须替换的占位符。使用前，操作者必须替换为自己的工作目录、MCP 地址、`uid`、`orgId`、Base ID、表名、tableId 和字段名；不得复用其他人的身份或业务数据。

## 1. 固定约束

1. 工作目录由操作者替换为自己的本地仓库路径：`<DWS_REPO_DIR>`。
2. 使用仓库根目录构建出的 `./dws`；不得使用 PATH 中已安装的 `dws`。
3. 默认沿用 `~/.dws`；只有需要隔离认证、缓存或 MCP 配置时，才按团队要求设置 `DWS_CONFIG_DIR`。
4. Agent 是否可以执行远程命令由当前会话的授权范围决定；未获得明确授权时，Agent 只提供完整命令，由操作者执行后回传结果。
5. `-c` 传入的 SQL 末尾不得包含分号（`;`）；服务会将 SQL 封装为子查询，尾部分号会导致 PostgreSQL 语法错误。
6. 避免在回复、文档或提交记录中暴露 token、密码和敏感业务数据。

## 2. 目标 AITable MCP 配置

本地调试只通过独立的 `DWS_CONFIG_DIR/mcp_url` 覆盖 MCP 地址；生产 MCP 主机固定为 `mcp-gw.dingtalk.com`，AI 表格服务 ID 以 `internal/syncdata/endpoints.go` 中 `aitable` 的当前值为准。`DINGTALK_AITABLE_MCP_URL` 不是 CLI 的运行时配置项，不得依赖它覆盖端点。

```bash
export DWS_CONFIG_DIR='<LOCAL_DWS_CONFIG_DIR>'
mkdir -p "$DWS_CONFIG_DIR"
printf '%s\n' 'https://mcp-gw.dingtalk.com/server/<AITABLE_SERVER_ID>?uid=<UID>&orgId=<ORG_ID>' > "$DWS_CONFIG_DIR/mcp_url"
```

`DWS_CONFIG_DIR` 必须指向操作者自己的本地目录，禁止把该目录或其中的 `mcp_url`、身份认证数据提交到 Git。

说明：

- 生产 AI 表格端点会自动完成 Streamable HTTP 握手、保留 `uid`/`orgId` 查询参数并维护 `Mcp-Session-Id`；提交代码、文档和默认配置均不得使用测试或预发域名。
- HTTPS 端点无需设置 `DWS_ALLOW_HTTP_ENDPOINTS=1`。
- `mcp_url` 是当前配置目录的全局 MCP 配置。完成调试后，按团队环境要求删除、恢复或切换该文件。
- URL 不应包含 token；`uid` 和 `orgId` 必须替换为当前操作者实际值，且不得复制到公开文档、提交记录或外部渠道。

## 3. 构建与本地运行

在仓库根目录执行：

```bash
cd <DWS_REPO_DIR>
make build
```

构建成功后验证本地二进制：

```bash
./dws --help
./dws aitable psql --help
```

代码变更后的最小验证：

```bash
make build
make format-check
```

需要执行全量测试时：

```bash
make test
```

不要使用 `go build ./cmd`，它会因默认输出名与 `cmd/` 目录冲突而失败。`make build` 会生成仓库根目录的 `./dws`。

## 4. 调试上下文模板

使用前将下表替换为当前操作者实际要查询的 AI 表格信息：

| 项目 | 当前值 |
| --- | --- |
| Base ID | `<BASE_ID>` |
| 目标逻辑表名 | `<TABLE_NAME>` |
| 目标表 tableId | `<TABLE_ID>` |
| 分组字段 | `<GROUP_COLUMN>` |
| 状态字段 | `<STATUS_COLUMN>` |
| 日期字段 | `<DATE_COLUMN>` |

SQL 使用逻辑表名，例如 `FROM "<TABLE_NAME>"`。`<TABLE_ID>` 仅用于 `-t` 查看结构；SQL 中应优先使用已验证的逻辑表名。

## 5. Agent 与用户的 `psql` 协作流程

### 5.1 用户确认表清单

Agent 让用户执行：

```bash
./dws aitable psql \
  -d '<BASE_ID>' \
  -l
```

用户把完整输出贴回。Agent 从输出中确认表名与 tableId，禁止猜测。

### 5.2 用户确认逻辑列

Agent 让用户执行：

```bash
./dws aitable psql \
  -d '<BASE_ID>' \
  -t '<TABLE_ID>' \
  --all-properties
```

用户把完整输出贴回。Agent 必须以输出中的 `Column` 和 `Type` 编写 SQL；不得使用 Field ID，不得猜测英文列名。

### 5.3 用户执行最小查询

表与字段确认后，先运行最小查询：

```bash
./dws aitable psql \
  -d '<BASE_ID>' \
  -c 'SELECT "<GROUP_COLUMN>", "<STATUS_COLUMN>", "<DATE_COLUMN>" FROM "<TABLE_NAME>" LIMIT 10'
```

最小查询成功后，再添加日期过滤、聚合、排序和窗口函数。每次只增加一个复杂度层级，便于定位失败点。

## 6. 聚合查询模板

操作者需将以下占位符替换为经 `-t` 验证过的真实表名、字段名和值。是否由 Agent 执行该命令，遵循当前会话授权范围。

```bash
./dws aitable psql \
  -d '<BASE_ID>' \
  -c '
SELECT
    "<GROUP_COLUMN>" AS category_name,
    COUNT(*) AS total_count,
    SUM(
        CASE
            WHEN "<STATUS_COLUMN>" = '\''<SUCCESS_VALUE>'\'' THEN 1
            ELSE 0
        END
    ) AS matched_count,
    SUM(
        CASE
            WHEN "<STATUS_COLUMN>" = '\''<SUCCESS_VALUE>'\'' THEN 1
            ELSE 0
        END
    ) * 100.0 / COUNT(*) AS matched_rate,
    RANK() OVER (
        ORDER BY
            SUM(
                CASE
                    WHEN "<STATUS_COLUMN>" = '\''<SUCCESS_VALUE>'\'' THEN 1
                    ELSE 0
                END
            ) * 100.0 / COUNT(*) DESC
    ) AS rank_no
FROM
    "<TABLE_NAME>"
WHERE
    "<DATE_COLUMN>" >= '\''<START_DATE>'\''
    AND "<DATE_COLUMN>" < '\''<END_DATE>'\''
GROUP BY
    "<GROUP_COLUMN>"
HAVING
    COUNT(*) >= <MIN_GROUP_SIZE>
ORDER BY
    matched_rate DESC
LIMIT <LIMIT>
'
```

SQL 是否可执行还取决于 PostgreSQL 语法、AI 表格逻辑列类型、字段映射和后续 SQL 改写结果。出现函数相关错误时，先保留完整错误信息，再用最小 `SELECT` 逐步缩小问题范围。

## 7. 错误处理规则

| 返回现象 | Agent 的下一步 |
| --- | --- |
| `Function is not allowed: <函数>` | 记录完整错误并核对当前服务版本；该错误来自服务端 SQL 校验策略，需确认目标环境是否已部署支持该函数的版本。 |
| `Invalid PostgreSQL query` | 回退到最小查询；先核对 `-l` 和 `-t` 的真实表、列与类型，再逐步恢复 SQL。 |
| 未知表/表名歧义 | 让用户重新执行 `-l`；SQL 中使用真实逻辑表名并加双引号。 |
| 未知列 | 让用户重新执行 `-t <TABLE_ID> --all-properties`；禁止用猜测的英文名或 Field ID。 |
| 权限或结果为空 | 核对用户当前身份、表级/字段级/行级高级权限；不要将其误判为 SQL 语法错误。 |
| 超时 | 缩小日期范围、增加 `WHERE`、减少 JOIN，并在用户允许时设置 `--timeout`（1～60 秒）。 |

## 8. Agent 回复规范

当用户要求排查 `psql` 失败时，Agent 必须：

1. 明确说明无法自行连接远程服务。
2. 给出用户可直接复制执行的完整 `./dws aitable psql` 命令。
3. 说明命令目的和预期输出。
4. 等待用户贴回输出后再给出下一步，不跳过表和字段发现阶段。
5. 不在回复、文档或提交信息中打印 token、密码或其他凭证。

## 9. 本地代码定位

- CLI 入口和 `psql` 参数路由：`internal/helpers/aitable_psql.go`
- MCP 工具名：`otable_pg_list_tables`、`otable_pg_describe_table`、`otable_pg_execute`
- AI 表格 PG 查询使用规范：`skills/mono/references/products/aitable/aitable-psql.md`
- 本地构建约定：`AGENTS.md` 与 `Makefile`
