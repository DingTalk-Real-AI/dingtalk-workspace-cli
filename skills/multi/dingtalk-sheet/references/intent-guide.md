# sheet 局部意图消歧

本文件从单 Skill `intent-guide.md` 拆分而来，仅保留与本产品相关的跨产品消歧规则。

| 用户说... | 真实意图 | 应该用 | 不要用 | 理由 |
|---|---|---|---|---|
| "帮我建一个项目跟踪表" | 创建数据表格 | `aitable` | `doc` / `sheet` | 涉及结构化数据/行列操作，不是富文本文档或电子表格 |
| "创建一个电子表格" | 创建表格文档 | `sheet` | `aitable` | Excel 式表格/单元格操作，不是多维表记录 |
| "帮我读一下表格 A1:D10 的数据" | 读取单元格数据 | `sheet` | `aitable` | 按单元格区域读写，不是按记录查询 |
| "这个 alidocs 表格链接帮我看下"（粘贴原始 URL） | 先 probe 节点类型 | `dws doc info --node` → 按 `extension` 路由 | 直接调 `sheet` | `alidocs/i/nodes/{id}` 可能是文档/axls/able/xlsx 等，禁止凭 URL 猜类型 |
| "读一下这个 xlsx 的数据" / xlsx 节点链接 | 下载本地表格文件 | `dws drive download --node` | `sheet range read` | xlsx / xls / xlsm / csv 是上传的本地文件（`contentType=DOCUMENT`），sheet 命令只支持在线表格，必须下载后本地解析 |
| "把这个在线表格导出为 xlsx 文件" | 在线表格格式转换 | `dws sheet export` | `dws drive download` | `export` 是 axls → xlsx 的导出转换；`download` 只能下载已有的 xlsx 节点 |
