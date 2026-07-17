# URL 粘贴场景

## URL 粘贴场景

用户直接粘贴表格 URL（无其他指令）:
- 先 probe：`dws doc info --node <URL> --format json`
- `extension=axls` → `list` + `range read`（读取第一个工作表数据）
- `extension=xlsx`/`xls`/`xlsm`/`csv` → 转 `dws drive download --node <URL> --output ./`

用户粘贴 URL + 附加指令:
- probe 为 `axls` → 按 Reference 索引路由到对应命令
- probe 为 xlsx/csv → 先 `dws drive download` 下载到本地，严禁调用 sheet 命令
