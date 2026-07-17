---
name: dingtalk-sheet
description: 钉钉电子表格。Use when 用户说 电子表格/导入Excel为在线表格/工作表/单元格读写/单元格追加/查找/公式/超链接/插入图片/浮动图片/sheet。Distinct from dingtalk-aitable(AI表格/多维表/字段类型)、dingtalk-doc(普通文档)。命令前缀：dws sheet。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉电子表格 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

## 前置条件 — 执行操作前必读

> **`use_skill(dws-shared)`** — 认证、全局参数（`--format json` / `--yes`）、错误码、URL 模板、跨产品消歧、安全规则与 capability 边界。**执行任何 `dws` 命令前先读；** 单产品的清晰命令可直接用本 skill。

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[sheet.md](references/sheet.md)。
> 高频模块：[触发门禁](references/sheet/sheet-triggers.md) · [读取](references/sheet/sheet-read-index.md) · [写入](references/sheet/sheet-write-index.md) · [格式](references/sheet/sheet-format-index.md) · [查询](references/sheet/sheet-query-index.md) · [可视化](references/sheet/sheet-visual-index.md) · [导入](references/sheet/sheet-import.md) · [导出](references/sheet/sheet-export-index.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 来自独立于 Runtime Schema 的公开 catalog。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 `dws shortcut list --service sheet --format json` 读取参数、约束、风险和示例，并以 `dws sheet <shortcut> --help` 核对当前 Cobra flags；不要对 `+` 路径调用 `dws schema`。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws sheet +list-sheets` | read | 获取表格文档中全部工作表列表 |
| `dws sheet +read` | read | 读取工作表指定范围的结构化单元格数据 |
<!-- VISIBLE_SHORTCUTS_END -->

## 意图表

| 用户说 | 命令 |
|--------|------|
| "创建电子表格" | `dws sheet create --name "<标题>"` |
| "导入本地 Excel 为在线表格" | `dws sheet import create --file <xlsx或xls> --folder-token <ID>` 或 `--workspace <ID>` |
| "新建工作表" | `dws sheet new --node <nodeId或URL> --name "<sheet名>"` |
| "读取单元格" | `dws sheet range read --node <nodeId或URL> --sheet-id <sheetId> --range A1:B2` |
| "写入单元格" | `dws sheet range update --node <nodeId或URL> --sheet-id <sheetId> --range A1:B2 --values '[[..]]'` |
| "结构化读取 / DataFrame 读取" | `dws sheet table-get --node <nodeId或URL> [--sheet-id <sheetId>]` |
| "结构化写入 / DataFrame 写入" | `dws sheet table-put --node <nodeId或URL> --sheets '<JSON>'` |
| "创建透视表 / 数据透视" | `dws sheet pivot-table create --node <nodeId或URL> --source "'Sheet1'!A1:D100" --properties '<JSON>'` |
| "查看透视表" | `dws sheet pivot-table list --node <nodeId或URL> --sheet-id <sheetId>` |
| "显示 / 隐藏网格线" | `dws sheet show-gridline|hide-gridline --node <nodeId或URL> --sheet-id <sheetId>` |
| "写入超链接" | `dws sheet range update --node <nodeId或URL> --sheet-id <sheetId> --range A1 --values '[[{"type":"text","text":"显示文本","hyperlink":{"type":"path","link":"https://..."}}]]'` |
| "追加一行" | `dws sheet append --node <nodeId或URL> --sheet-id <sheetId> --values '[[..]]'` |
| "查找 / 替换" | `dws sheet find --node <nodeId或URL> --sheet-id <sheetId> --query "<关键词>"` / `dws sheet replace --node <nodeId或URL> --sheet-id <sheetId> --find "<旧值>" --replacement "<新值>"` |
| "精确匹配搜索 / 完全等于" | `dws sheet find --node <nodeId或URL> --sheet-id <sheetId> --query "<关键词>" --match-entire-cell` |
| "搜索公式文本" | `dws sheet find --node <nodeId或URL> --sheet-id <sheetId> --query "<公式片段>" --match-formula` |
| "正则搜索 / 不区分大小写" | `dws sheet find --node <nodeId或URL> --sheet-id <sheetId> --query "<regexp>" --use-regexp --match-case=false` |
| "插入图片到单元格" | `dws sheet write-image --node <nodeId或URL> --sheet-id <sheetId> --range A1 --file <图片路径>` |
| "创建浮动图片" | 先 `dws sheet media-upload --node <nodeId或URL> --file <图片路径>` 获取 `resourceUrl`，再 `dws sheet create-float-image --node <nodeId或URL> --sheet-id <sheetId> --src "<resourceUrl>" --range A1 --width <宽> --height <高>` |

## URL 与 ID 前置

- 用户直接粘贴 `alidocs.dingtalk.com` URL 时，先执行 `dws doc info --node "<URL>" --format json` 探测；只有 `contentType=ALIDOC` 且 `extension=axls` 才继续走 `sheet`。
- `spreadsheetv2` 链接不要截取短 path segment，必须把完整 URL 原样传给 `--node`。
- 写入类命令必须先用 `dws sheet list --node <nodeId或URL> --format json` 取得真实 `sheetId`；不要猜 `Sheet1`、`sheet1`、`0`。
- 所有 sheet 子命令使用 `--node` 参数；不要写 `--node-id`、`--file-id` 或把 JSON 字段名 `id` 当成节点值传入。

## 高频硬约束

- `sheet create` 后必须从返回结果提取真实 `nodeId` 或文档 URL，后续 `range update/read` 原样传给 `--node`；如果返回里同时有 `nodeId` 和 `url`，优先用 `nodeId`。
- 写入区域前先 `sheet list --node <nodeId> --format json` 获取真实 `sheetId`。`range update` 返回的 `success` / `updatedRows` / `updatedCells` / `message` 可作为写入确认；仅在任务需要内容校验且用户未禁止额外读取时调用 `range read`。
- `range update` 的 `--values` 必须是二维 JSON，行列数量与 `--range` 完全一致；批量样式用 `range batch-set-style --batch <json文件>`。
- 整格超链接只能用 `range update` 的 cell-level `hyperlink` 字段：`{"type":"text","text":"显示文本","hyperlink":{"type":"path","link":"https://..."}}`。不要走 `doc`，不要用 `--hyperlinks`，不要把整格链接写成 richText 片段链接。
- 搜索必须用 `sheet find`，不要用 `range read` 后本地过滤；“精确/完全匹配/等于”必须加 `--match-entire-cell`，“搜公式”必须加 `--match-formula`，“正则且不区分大小写”必须同时加 `--use-regexp --match-case=false`。
- 出现 `nodeId 格式不合法` 时，回到 `doc info --node <URL>` 或 `sheet create` 的返回重新提取真实 ID，禁止复用无效值重试。

## 跨产品协作

- 多维表 / 字段类型 → 切到 `dingtalk-aitable`
- 把表数据写进文档 → 切到 `dingtalk-doc`
## 局部意图与 Recipe

- [局部意图消歧](references/intent-guide.md)。
