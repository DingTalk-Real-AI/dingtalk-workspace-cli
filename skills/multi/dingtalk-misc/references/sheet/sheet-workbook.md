# Sheet 工作簿与工作表

## 本阶段范围

用于创建工作簿、列出/新增/重命名/复制/删除工作表，以及冻结、隐藏、顺序、网格线和结构信息。值读写留在上层 sheet.md；样式、行列和对象操作进入各自阶段。

## 最短命令表

| 目标 | 命令 |
|---|---|
| 创建空工作簿 | dws sheet create --name <NAME> --format json |
| 创建并写初始数据 | dws sheet create-with-data --name <NAME> --values <2D_JSON> --format json |
| 列出工作表 | dws sheet +list-sheets --node <NODE_ID> --format json |
| 工作表结构详情 | dws sheet info --node <NODE_ID> --sheet-id <SHEET_ID> --format json |
| 新建工作表 | dws sheet new |
| 改名、隐藏、排序、冻结 | dws sheet update |
| 复制工作表 | dws sheet copy |
| 删除工作表 | dws sheet delete-sheet |
| 显示/隐藏网格线 | dws sheet show-gridline / hide-gridline |

有初始数据时用 create-with-data，它已经包含创建、默认工作表探活、写入和回读；不要再额外 list 一次。空表才用 create。

create-with-data 的成功率契约：

- `--values` 与 `--sheets` 必须且只能提供一个；只有空表需求才改用 create。
- values 是非空二维 JSON，只含 string/number/boolean/null，最多 30000 单元格且编码后不超过 2M 字符。
- sheets 每项必须用 camelCase，只接受 name/columns/data/dtypes/formats/cellStyles/startCell/mode/header/allowOverwrite；创建阶段不接受 sheetId。name、columns 必填，data 每行宽度必须等于 columns，dtypes/formats 键必须来自 columns。
- `--styles` 顶层使用 `{"styles":[...]}`；配 sheets 时样式项数量、顺序和 name 必须一一对应，配 values 时只能有一项。执行顺序是 cell_styles、row_sizes、col_sizes、cell_merges，整体非原子。
- 所有 JSON、枚举和预算都在创建前校验。若后续探活/写入/样式失败，错误中的已创建 nodeId 必须保留并报告，用于续做或经确认后清理；不能再次 create 产生重复文档。

## ID 与上下文

- create / create-with-data 返回的 nodeId 是后续唯一文档标识，立即复用；不要从 URL 字符串截取或从历史任务复用。
- --folder / --workspace 接受 Drive fileId UUID 或可解析的 alidocs URL，不接受数字 dentryId。
- 需要 sheetId 时优先复用 create-with-data 探活返回值；上下文没有才执行一次 +list-sheets。
- 用户给的是工作表名称也要按完整标题唯一匹配；禁止猜 Sheet1、sheet1、0、default。
- info 返回 mergedRanges、冻结、尺寸、隐藏和可选 groups 等结构信息；CSV 空格不能代替结构查询。`--include` 按需取 row_heights、col_widths、groups；同时检查 nonEmptyRange、默认行高列宽和隐藏行列，避免为了完整结构无界输出。

## 常见闭环

    dws sheet +list-sheets --node <NODE_ID> --format json
    dws sheet new --node <NODE_ID> --name "明细" --format json
    dws sheet +list-sheets --node <NODE_ID> --title "明细" --format json

更新工作表属性前，先从 list/info 读取当前状态；只提交用户要求变更的字段。复制后使用响应返回的新 sheetId，不按名称猜测。重命名、移动顺序或删除可能使后续名称/位置引用失效，必须把新状态传播给下一阶段。

update 至少传 name/index/hidden/frozen-row-count/frozen-column-count/tab-color 之一。name 最长 100 字符且不能含 `/ \\ ? * [ ] :`；index 从 0 开始；冻结数不得越过实际行列边界；不能隐藏所有工作表。tab-color 使用 `#RRGGBB`，显式空字符串表示清除颜色。copy 的 index 也从 0 开始；未给 name 时由系统生成，必须使用返回的新 sheetId。

## 删除与验证

delete-sheet 会永久删除目标工作表。执行前展示 nodeId、sheetId/标题和影响范围，得到明确同意后才加 --yes。隐藏工作表必须先取消隐藏；不能删除最后一个可见工作表。不要删除工作簿中未确认的同名表，也不要把“删除整个在线文件”误路由到 delete-sheet。

所有结构修改都用匹配的读操作验证：

- new/copy/update/delete-sheet：+list-sheets；
- 冻结、隐藏、尺寸、合并：info；
- 网格线：info 或命令返回的明确状态。

响应非零、JSON 无法解析或缺失预期对象均为失败，不得当作空列表或成功。
