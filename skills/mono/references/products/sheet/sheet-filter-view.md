# Sheet 筛选视图

## 边界与上下文

筛选视图是命名的个人视角，不改变原始数据，也不影响其他协作者；用户只说“筛选”且要求所有人看到时，默认进入全局 `sheet filter`，明确说“筛选视图/个人视图”才进入本阶段。所有命令要求当前任务真实 nodeId、sheetId；filterViewId 必须来自本次 list/create 返回。未知 sheetId 时只调用一次 +list-sheets 并向后复用。

column 是相对筛选视图 range 首列的 0 起始偏移，不是工作表绝对列号。例如视图从 C 列开始，column=0 指 C 列。视图 range 应包含表头行，否则筛选显示和列语义容易错位。

## 命令闭环

列出或查看：

    dws sheet filter-view list       --node <NODE_ID> --sheet-id <SHEET_ID> --format json

    dws sheet filter-view info       --node <NODE_ID> --sheet-id <SHEET_ID>       --filter-view-id <FILTER_VIEW_ID> --format json

创建带条件的视图：

    dws sheet filter-view create       --node <NODE_ID> --sheet-id <SHEET_ID>       --name "高销售额视图" --range "A1:E100"       --criteria '[{"column":0,"filterType":"values","visibleValues":["销售部"]},{"column":2,"filterType":"condition","conditions":[{"operator":"greater","value":"50000"}]}]'       --format json

criteria 是数组，支持三类：

- values：visibleValues 指定可见值。
- condition：conditions 最多 2 项，比较值用字符串；conditionOperator 取 and（默认）或 or。operator 必须使用 kebab-case：equal、not-equal、contains、not-contains、starts-with、not-starts-with、ends-with、not-ends-with、greater、greater-equal、less、less-equal。
- color：backgroundColor 与 fontColor 二选一；“标红/高亮”默认指背景色，只有明确说字体颜色时才用 fontColor。

不在上述枚举中的 operator 不得猜测；确有其他需求时只读取 filter-view create/update-criteria 的 compact Schema。

更新单列条件：

    dws sheet filter-view update-criteria       --node <NODE_ID> --sheet-id <SHEET_ID>       --filter-view-id <FILTER_VIEW_ID> --column 2       --filter-criteria '{"filterType":"condition","conditions":[{"operator":"greater","value":"100"}]}'       --format json

查看或删除条件：

    dws sheet filter-view list-criteria       --node <NODE_ID> --sheet-id <SHEET_ID>       --filter-view-id <FILTER_VIEW_ID> --format json

    dws sheet filter-view get-criteria       --node <NODE_ID> --sheet-id <SHEET_ID>       --filter-view-id <FILTER_VIEW_ID> --column 2 --format json

    dws sheet filter-view delete-criteria       --node <NODE_ID> --sheet-id <SHEET_ID>       --filter-view-id <FILTER_VIEW_ID> --column 2 --format json

delete-criteria 只清除一列条件并保留视图；对不存在的条件是幂等成功，但仍按删除类操作先确认。get-criteria 查询未设置条件的列会报错；list-criteria 在没有条件时返回空对象，两者不能互换。

update 至少传 name/range/criteria 之一。criteria 只替换数组中明确指定列的条件，未指定列保持不变；update-criteria 精确替换一列。先 info/list-criteria 读取当前状态，只提交用户要求的变化，不能用空 criteria 猜测“清空全部”。

## 删除与验证

delete 会永久删除整个筛选视图及其条件。先展示名称、filterViewId、range，得到明确同意后才加 --yes：

    dws sheet filter-view delete       --node <NODE_ID> --sheet-id <SHEET_ID>       --filter-view-id <FILTER_VIEW_ID> --yes --format json

create/update/update-criteria/delete-criteria 后用 info 或 list-criteria 验证；delete 后用 list 验证对象消失。空列表只有在命令明确成功且 envelope 完整时才是真空结果；非零、坏 JSON 或缺字段必须报错。
