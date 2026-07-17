# 适用范围

## 适用范围

`sheet` 仅支持钉钉在线电子表格（`contentType=ALIDOC`、`extension=axls`）。

| 文件类型 | 处理方式 |
|---------|---------|
| 在线电子表格（`axls`） | 走 `sheet` 全部命令 |
| 钉盘/文档中已有的 `xlsx` / `xls` / `xlsm` / `csv` 节点 | `dws doc download --node <ID> --output <路径>` 下载到本地；禁止把该节点直接传给工作表命令 |
| 本地 xlsx/xls 导入为在线表格 | `dws sheet import create --file <路径> --folder-token <ID>` 或 `--workspace <ID>`；转换后可用全部 `sheet` 命令 |
| 在线表格导出为 xlsx | `dws sheet export`（axls → xlsx 格式转换） |

用户贴原始 `alidocs` URL 时必须先 probe：`dws doc info --node <URL> --format json`，按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) 校验：
- `contentType=ALIDOC` + `extension=axls` → 继续走 `sheet`
- `extension=xlsx` / `xls` / `xlsm` / `csv` → 转 `dws doc download`；需要在线编辑时，再对下载后的本地 xlsx/xls 执行 `dws sheet import create`
