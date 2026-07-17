# URL → NODE_ID

## URL → NODE_ID

| URL 格式 | 提取方式 |
|----------|---------|
| `.../i/nodes/{id}` 或 `.../i/nodes/{id}?query` | 取路径末段作 NODE_ID（忽略 query） |
| `.../spreadsheetv2/{key}/...` | **完整 URL 原样传 `--node`**，禁止提取 path segment |

参数不确定时先 `dws sheet <命令> --help`。
