# drive list --latest 设计 — 按修改时间取最新 N 个文件

> 前置：`--depth` 递归 BFS 骨架（[internal/helpers/drive_depth.go](../internal/helpers/drive_depth.go)），
> `--latest` 整体复用其遍历、限流补偿、SIGINT 与进度输出，不新建第二套递归编排。

## 1. 背景与目标

**痛点**：定位「某人最新的日报」目前要三步——列目录 → 自己按时间排序 → 逐个辨认。
Agent 侧的实际做法是 `drive list --limit 50` 全量拉回上下文再比时间，既不可靠（翻不到的页
可能有更新的文件），又浪费上下文。

**目标**：一条命令直达。

```bash
dws drive list --folder <ID> --pattern "*日报*" --latest 1
```

**心智模型**：`--latest N` = 按修改时间取最新 N 个**文件**；叠加 `--pattern` 时 =
**名称匹配的文件中**最新 N 个（先按 pattern 筛，再取 Top-N）。

## 2. 参数契约

| 项 | 定案 |
|---|---|
| 形态 | `--latest N`，Int，默认 0（不传 = 不启用，存量行为字节级不变） |
| 取值范围 | 1~50，越界返回 `CodeInvalidParam`（退出码 3） |
| 命令载体 | 仅 `drive list`，不新增 `drive find` 子命令 |
| 候选范围 | **仅文件**。文件夹不参与 Top-N（递归时仍用于下钻与组树） |

**上限取 50 的理由**：与服务端每页硬上限 50 对齐；「定位最新」场景的 N 天然小（1~10 量级），
N 达百/千级属于「导出清单」场景，应由 `--order-by modifyTime --limit --cursor` 翻页或 `--depth`
承接。未来放宽只需改校验常量——钉盘单层 1000 条扫描内、其余路径 BFS 2000 条内都天然支持更大的 N。

### 2.1 组合矩阵

| 组合参数 | 行为 |
|---|---|
| `--space-id` / `--folder` | ✅ 限定扫描范围 |
| `--pattern` | ✅ 名称匹配的文件中最新 N 条 |
| `--thumbnail` | ✅ |
| `--depth` > 1 | ✅ 全树遍历后客户端排序取 Top-N |
| `--workspace` | ✅ 知识库路由客户端排序（执行模型与钉盘不同，见 §3.2） |
| `--versions` | ❌ 版本列表按版本序返回，无跨文件 Top-N 语义 |
| `--order-by` / `--order` / `--limit` / `--cursor` | ❌ `CodeInvalidParam` |

互斥报错统一附等效改写，避免 Agent 拿到报错只能盲试：

```
--latest 不能与 --order-by 同时使用：如需自定义排序请改用
--order-by modifyTime --order desc --limit 3
```

### 2.2 两条路由的能力差异

| | 钉盘（`list_files`） | 知识库（`list_nodes`，`--workspace`） |
|---|---|---|
| 服务端排序 | ✅ `orderBy=modifyTime&order=desc` | ❌ 入参无 orderBy |
| 时间字段 | `modifyTime`（毫秒时间戳） | `updateTime`（毫秒时间戳） |
| 类型字段 | `type=FILE/FOLDER` | `nodeType=file/folder` |
| 能否凑够即停 | ✅ | ❌ 必须拉全量再排 |

两侧时间字段格式相同（毫秒整数），归一只是字段名映射 + 数值比较。归一化实现见
[internal/helpers/drive_time.go](../internal/helpers/drive_time.go)：按
`modifiedTime → modifyTime → modified_time → gmtModified → lastModifiedTime → updateTime`
顺序探测，值支持 `float64` / `json.Number` / 数字字符串 / RFC3339 字符串，`<= 0` 视为无效。
多写几种形态是为了对上游字段命名变化留冗余，成本只有一次线性探测。

服务端排序**不区分类型**——`orderBy=modifyTime` 的结果里文件夹会混在前列。这就是「候选仅文件、
客户端剔除」这条防线的必要性来源，两条路由都要做。

## 3. 执行模型

### 3.1 钉盘单层（`depth == 1`）：服务端排序 + 客户端扫描凑 N

```
list_files orderBy=modifyTime&order=desc（每页 50）
    → 逐页筛：剔除文件夹 + pattern 匹配
    → 终止（先到先停）：
       a) 凑够 N 条
       b) 扫描达上限 1000 条（50 × 20 页）
       c) 翻页到底（nextToken 空）
```

无 pattern 时通常一次请求命中：N ≤ 50、每页 50，首页必凑满。

**为什么这里不复用 BFS**：有无服务端排序决定能否提前终止，能提前终止就值得单独写循环。
钉盘服务端有序，无 pattern 时恒定 1 次请求；若复用 BFS 则无条件全量拉取，最高频路径从
O(1) 退化到 O(目录大小)，不划算。知识库反过来——本就必须拉全量，复用 BFS 纯赚。

### 3.2 知识库单层（`--workspace`，`depth == 1`）：复用 BFS 的 depth=1 特例

```
runDriveListDepth(route=newDocDepthRoute(), maxDepth=1)   ← 整段复用，零新写循环
（拉完当前文件夹所有页：双条件终止 + 单文件夹页数上限 + 全局 2000 条
  + 限流补偿 + SIGINT + --quiet 进度，全部现成）
    → latest 后置处理器（与 depth>1 共享同一个）
```

防线沿用 BFS 既有上限（全局 2000 条 + 单文件夹页数上限），不引入独立扫描常量——
「1000 条 / 20 页」只属于钉盘单层的提前终止循环。

`depth == 1` 时后置处理器剥掉 `depth` / `parentId` / `rel_path` / `sortTime` 装饰字段，
使两条路由的单层 latest 输出结构一致，也与不带 latest 的普通 `list` 一致（装饰字段仅
`--depth > 1` 时出现，维持现有契约）。

### 3.3 递归（`depth > 1`）：BFS 后置排序

```
现有 BFS 全树遍历（请求数与 --depth --pattern 完全一致，零新增请求）
    → 现有 pattern 全量过滤
    → 新增：仅文件 → 时间归一 → 排序 → 截断 N
```

服务端排序只在单文件夹内有效，全树 Top-N 必须遍历完成后客户端排序，无法提前终止。
但遍历成本 = 现有 `--depth` 遍历，`--latest` 只增加内存排序与截断。

**排序 tie-break 链**（`sort.SliceStable`，三级）：

```
sortTime desc  →  rel_path asc  →  fileId/nodeId asc
```

单靠时间不够：同一次批量操作产生的文件时间戳常常完全相同，只按时间排会让输出顺序随
上游返回顺序漂移，CI 断言无法稳定。补上 `rel_path` 与 ID 两级后输出完全确定。
无时间戳的项（`sortTime == 0`）排在末尾，不因缺字段被静默丢弃。

## 4. 防线与部分结果语义

| 场景 | 行为 | 退出码 |
|---|---|---|
| 钉盘单层扫满 1000 条未凑满 N | 输出已找到部分，stderr 提示「已扫描 1000 条，找到 X/N 条，建议 --folder 缩小范围」 | 0 |
| 知识库单层 / 递归触 2000 条全局上限 | **拒绝产出 Top-N**：`LATEST_SCAN_TRUNCATED`，stdout 为空 | 非 0 |
| SIGINT 中断 | 沿用 depth 既定语义：partial 照吐（`truncated=true`） | 130 |
| 0 条匹配 | 空 `items[]`，与现有 list 空结果一致 | 0 |

这两条看似矛盾的处理，分界线是**排序基是否完整**：

- 钉盘单层「凑不满 N」是**结果不满**而非任务失败。服务端已按时间有序，已扫描的 1000 条
  就是最新的 1000 条，Top-N 依然正确，部分结果对「找最新文档」仍然可用 → 退出码 0。
- 触到 2000 条上限是**结果可能错误**。BFS 的目录序与修改时间无关，未扫描区域完全可能含
  更新的文件，截断集上的 Top-N 不是全局最新，与命令承诺的「最近修改的 N 个文件」语义不符
  → 报错，且 stdout 必须为空。输出一个看起来正常、实际可能错的 Top-N 比报错危险得多。

不带 `--latest` 的递归仍沿用 `truncated: true` + partial 语义、退出码 0——那里没有全局
排序承诺，部分结果是诚实的。这条回归护栏有独立测试锁定。

stderr 提示不止说明发生了什么，还要给出可直接执行的后续命令（如
`dws drive list --folder <子目录ID> --pattern "..." --latest N`），便于 Agent 拿到提示后自主继续。

## 5. 输出契约

- 结构与普通 `drive list` 相同（`items[]`），不新增业务字段
- 带 `--latest` 时按修改时间 desc 排列（第 1 条最新）；`rel_path` 树序仅在不带 latest 时保持
- `depth > 1` 时每条仍带 `depth` / `parentId` / `rel_path`；单层剥除
- 已知信封差异：钉盘单层 latest 顶层仅 `{items}`；知识库单层与两条路由的 `depth > 1` 走
  BFS 发射函数，顶层保留 `{items, maxDepth, truncated, errors}`。这是复用 BFS 编排的既定
  代价，不为形式一致重构发射函数——`truncated` / `errors` 对消费方有信息价值
- **不返回 `nextToken` / `hasMore`**，Top-N 不支持续页，两条理由：
  1. **游标失真**——客户端过滤后「服务端扫描位置」与「结果位置」对不上，透出扫描游标等于
     重复 `--order-by` + `--cursor` 的既有能力；知识库与 depth 路径是客户端全量排序，
     根本没有可续的游标
  2. **排名快照漂移**——两次调用之间文件有变动，「第 11~20 名」会重叠或遗漏；排名分页要保真
     需要服务端快照，成本不成比例

  翻「更早的」用等效原语：`--order-by modifyTime --order desc --limit N --cursor <TOKEN>`
- 进度与提示走 stderr，stdout 仅纯 JSON；`--quiet` 可关进度

## 6. 代码结构与复用

| 新增 / 改动 | 复用来源 |
|---|---|
| flag 注册 + `validateDriveListLatest`（互斥 / 边界 / 引导文案） | `validateDriveListDepth` / `driveDepthExclusiveError` 同构 |
| `runDriveListLatest` 钉盘单层扫描循环（唯一新循环） | 页大小 / 分页解析 / pattern 匹配全部复用现有 helper |
| 知识库单层拉取 | `runDriveListDepth(maxDepth=1)` + `newDocDepthRoute` 整段复用，零新写 |
| `applyDriveListLatest` 后置处理器 | 新写一份，三处共享（知识库单层 / 钉盘 depth>1 / 知识库 depth>1），挂在现有 BFS 输出组装点，BFS 主循环仅新增一行 `sortTime` 写入 |
| `driveModifiedMillis` / `driveToMillis` 时间归一 | 新写，纯函数无副作用 |
| pattern 匹配 | 现有 `matchDriveNamePattern`，零改动 |
| 不带 `--latest` 的路径 | 字节级不变 |

`--latest` 是纯客户端能力，不透传服务端，与 `--depth` 的声明风格一致。

## 7. 测试矩阵

Go 单测，全部 `TestCrossPlatformCoverage*` 前缀（覆盖率门禁选择规则）：

| # | 用例 | 断言要点 |
|---|---|---|
| 1 | 钉盘单层正向 | ≤ N 条、全为文件、时间倒序、凑够即停不翻第二页 |
| 2 | `--pattern` + `--latest` | 名称过滤后再取 Top-N |
| 3 | `--depth 2 --latest 3` | 走 BFS，不注入服务端排序 |
| 4 | `--workspace --latest` | 路由到 `list_nodes`，单层剥装饰字段、多层保留 `rel_path` |
| 5 | 互斥 | `--order-by` / `--order` / `--limit` / `--cursor` / `--versions` 各自报错 + 等效改写文案 |
| 6 | 边界 | `--latest 0` / `51` → `CodeInvalidParam` |
| 7 | 截断即拒绝 | `LATEST_SCAN_TRUNCATED` + **stdout 为空** + 请求次数恰为 2000/50 + 未访问的文件夹从未被请求 |
| 8 | 无 latest 的截断回归 | `truncated: true` + 满额 partial（护栏，防止 7 的实现污染既有语义） |
| 9 | 凑不满 N | 退出码 0 + stderr 提示 |
| 10 | 扫描上限兜底 | 全是文件夹时靠 1000 条上限停机，不无限翻页 |
| 11 | 时间归一 | 四种输入类型 + 无效值 + key 优先级 + 靠前 key 不可解析时的回落 |
| 12 | tie-break | 相同 `sortTime` 时按 `rel_path`、再按 ID 稳定排序 |

## 8. 后续服务端演进（登记项，不阻塞当前版本）

| # | 演进项 | 效果 |
|---|---|---|
| S1 | `list_files` 增加名称过滤字段 | 单层 pattern + latest 退化为一次请求，1000 条扫描防线退役 |
| S2 | `list_nodes` 增加 orderBy | 知识库单层恢复「凑够即停」，并消除触顶时 Top-N 的精确性弱化 |
| S3 | 服务端递归 API | BFS 整块退役，全树 Top-N 下沉服务端 |

S1 落地后本方案自然向「单请求服务端形态」收敛；在那之前，客户端 Top-N 是唯一能给出
正确答案的形态。
