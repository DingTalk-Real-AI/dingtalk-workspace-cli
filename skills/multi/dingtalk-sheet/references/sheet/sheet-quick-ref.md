# 场景速查

## 场景速查

| 用户要做 | 正确命令 | 动手前读 | 禁止写法 |
|---------|---------|----------|----------|
| 快速查看数据 / 大表分批读取 | `csv-get` | [sheet-read-data](./sheet-read-data.md) | 不传 `--range` 全量 `range read` 大表 |
| 少量精确写入、公式、超链接、富文本、数据验证 | `range update` | [sheet-write-data](./sheet-write-data.md)、[sheet-formula](./sheet-formula.md) | 用 `csv-put` / `append` 写公式或富格式 |
| 批量纯值写入 / CSV 粘贴 | `csv-put` | [sheet-write-data](./sheet-write-data.md) | 为大块纯值手写巨大 `--values` JSON |
| 追加记录到末尾 | `append` | [sheet-write-data](./sheet-write-data.md) | 手算最后一行后 `range update` |
| 查找 / 替换 | `find` / `replace` | [sheet-search-replace](./sheet-search-replace.md) | 读全表后本地过滤或手写替换 |
| 清空 / 排序 / 填充 / 复制移动区域 | `range clear` / `range sort` / `range fill` / `range copy-to` / `range move-to` | [sheet-range-operations](./sheet-range-operations.md) | 用读写组合模拟服务端原子操作 |
| 合并、冻结、分组、行高列宽 | `sheet info` + 结构命令 | [sheet-workbook](./sheet-workbook.md)、[sheet-dimension-operations](./sheet-dimension-operations.md) | 从 `range read` / CSV 空值推断结构 |
| 多个原子写操作组合 | `batch-update` | [sheet-batch-operations](./sheet-batch-operations.md) | 多次独立调用导致半成品 |
| 图片写入单元格 / 浮动图片 | `write-image` / `media-upload` + `create-float-image` | [sheet-media-image](./sheet-media-image.md) | 用 `range update` 写图片 |
| 条件高亮 / 标红 / 数据条 / 色阶 | `cond-format` | [sheet-conditional-format](./sheet-conditional-format.md) | 用静态 `set-style` 冒充条件格式 |
