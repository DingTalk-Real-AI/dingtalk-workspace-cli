# 考勤排班操作参考 (attendance-schedule)

> 本文档由 `attendance.md` 路由调用。覆盖两类排班操作：
> 1. **排班导入**（写操作）：当用户提到"排班"、"导入排班"、"安排班次"、"设置排班"、"调班"、"换班"、"排休"时
> 2. **排班查询导出**（只读操作）：当用户提到"查看排班"、"排班表"、"导出排班"、"排班记录"时
>
> 不适用于：班次定义查询（用 `attendance class search`）、考勤组配置（用 `attendance group get`）。

## 强制门禁（必须先读完本文档才能执行）

**任何排班操作都必须经过本文档定义的工作流，严禁绕过本文档直接调用 `dws attendance schedule import` 命令。**

违反将出现以下任一问题：
1. 未按"阶段 1"确认考勤组 → 把固定班制考勤组当排班制操作，接口报错
2. 未按"阶段 3"校验班次 → 传入不属于该考勤组的班次 ID，导致排班数据错乱
3. 未按"阶段 4"回显确认 → 用户未看到排班内容就直接执行，排错了无法回退
4. 未按"阶段 2"解析人员 → 传入错误的 userId，导致排班到错误的人
5. 未经用户确认就执行排班 → 排班是写操作，一旦执行就会覆盖原有排班

**执行前自检（必须能在心中回答）**：
- [ ] 考勤组是排班制（TURN）吗？
- [ ] 员工都属于该考勤组吗？
- [ ] 班次都属于该考勤组可用的班次吗？
- [ ] 用户已经确认了排班内容吗？

如果上述任何一项答不出，**回到本文档对应章节重新阅读**，禁止凭记忆/想象组装命令。

**前提**：当前用户必须是钉钉考勤管理员，否则排班接口返回权限错误。

## 业务约束（必须深刻理解）

> **这两条约束是排班的根基，贯穿整个工作流的每一步。**

1. **用户只能属于一个考勤组**：每个员工有且只有一个考勤组，不存在"选择考勤组"的场景。直接通过 `dws attendance rules` 查询即可唯一确定。
2. **排班只能排考勤组关联的班次**：考勤组绑定了固定的班次列表（`shiftVOList`），排班时只能从这些班次中选择，不能使用企业其他考勤组的班次，更不能编造班次。

**由此推导出的执行顺序**：必须先查清考勤组和它关联的班次，再去收集日期、人员等其他参数。

## 核心原则

Agent 解析用户意图（考勤组、员工、日期范围、班次安排），完成校验后，**调用 Python 脚本执行排班**。
- **先查后排**：任何排班操作的第一步都是查询考勤组及其关联班次，拿到真实数据后再进行后续参数收集
- **脚本自包含**：考勤组校验、班次校验、员工校验、回显确认、调用排班 API 全部由脚本内部完成
- **Agent 职责**：先查考勤组和关联班次，再解析用户意图、获取必要的 ID（员工 userId）、组装脚本参数
- **脚本职责**：二次校验、回显排班表格、等待用户确认、执行排班、输出结果摘要
- 排班是**写操作**，必须经过回显确认后才能执行

## 严格禁止 (NEVER DO)

- **禁止直接调用 `dws attendance schedule import`**，必须通过脚本执行
- 禁止凭历史记忆复用任何 ID（考勤组 ID、班次 ID、userId），必须从当次命令返回值中提取
- 禁止在未确认考勤组类型为排班制（TURN）的情况下执行排班
- 禁止在班次未经校验的情况下执行排班
- 禁止跳过用户确认直接执行排班
- **禁止在未向用户展示完整排班明细表格（含每天的班次名称）的情况下请求确认**。用户必须先看到"谁、哪天、上什么班"才能做出确认决策
- 禁止编造任何字段值或用户姓名
- 禁止直接输出裸 userId，脚本已内置 userId → 姓名转换
- **禁止直接输出裸 classId 数字**，必须展示班次名称（如"早班"），用户不理解 classId 是什么

## 严格要求 (MUST DO)

- 所有 `dws` 命令必须携带 `--format json`
- 必须先确认考勤组类型为 TURN（排班制），否则拒绝执行
- 必须通过考勤组详情获取绑定班次列表，校验班次 ID 属于该考勤组
- 必须在执行排班前向用户回显排班内容并获得确认
- 任何接口失败必须向用户清晰报错，禁止静默吞掉

## 涉及工具

| 工具 | 用途 | 安全等级 |
|------|------|---------|
| `dws attendance group search` | 搜索考勤组（按名称/类型） | 只读 |
| `dws attendance group get` | 查询考勤组全量信息（含绑定班次列表） | 只读 |
| `dws attendance group filtered-get` | 查询考勤组详情（成员列表） | 只读 |
| `dws attendance class search` | 查询班次列表（ID→名称映射） | 只读 |
| `dws attendance class get` | 查询班次详情 | 只读 |
| `dws attendance schedule import` | 导入排班记录（**仅由脚本内部调用**） | 写操作（危险） |
| `dws attendance schedule get` | 查询现有排班记录 | 只读 |
| `dws aisearch person` | 按姓名搜索用户获取 userId | 只读 |
| `dws contact user get` | 批量查询 userId → 用户信息 | 只读 |
| `dws contact dept search` | 搜索部门获取 deptId | 只读 |
| `dws contact dept list-members` | 获取部门成员 userId 列表 | 只读 |

## 意图判断

### 排班操作类型

| 用户说 | 操作类型 | 处理方式 |
|--------|---------|---------|
| "帮我给研发组排下周的班" / "排班" / "安排班次" | 批量排班 | 走本文档工作流 |
| "帮我把张三下周一改成早班" / "调班" / "换班" | 单人调班 | 走本文档工作流（单人模式） |
| "帮我把李四下周三排休" | 排休 | 走本文档工作流（isRest=Y） |

### 易混淆场景

| 用户说 | 应路由到 |
|--------|---------|
| "查看下周的排班" / "排班表" / "导出排班" / "导出排班表" / "XX考勤组的排班" | **本文档「排班查询导出工作流」**（走脚本） |
| "有哪些班次" / "班次列表" | `dws attendance class search`（查询班次定义） |
| "我属于哪个考勤组" | `dws attendance rules`（查询考勤规则） |
| "导出考勤报表" / "导出考勤" / "考勤明细" / "出勤汇总" （**不含"排班"二字**） | `attendance-report.md`（报表 skill） |

> **关键区分**："导出排班表" ≠ "导出考勤报表"。判断标准：句中含"排班"→ 本文档；不含"排班"且说的是"考勤报表/考勤数据/出勤统计" → `attendance-report.md`。

## 工作流

### 阶段 0: 先查考勤组和关联班次，再收集缺失参数

> **核心逻辑：先查后问。** 用户只属于一个考勤组，排班只能排该考勤组关联的班次。所以第一步永远是查清考勤组和它的班次，拿到真实数据后再向用户收集其他信息。

**步骤 0a — 查询考勤组（必须最先执行）**：

```bash
# 自动获取当前用户的考勤组（用户只属于一个考勤组，无需选择）
dws attendance rules --date <今天日期> --format json
# → 从返回中提取 groupId
```

如果是给指定员工排班，先查该员工的 userId，再查其考勤组。

**步骤 0b — 查询考勤组详情和关联班次（必须在收集班次选择前完成）**：

```bash
# 获取考勤组详情（含绑定的班次列表）
dws attendance group get --group-id <groupId> --format json
# → 校验 groupVO.type 必须为 TURN（排班制）
# → 从 groupVO.shiftVOList 提取关联的班次
```

**提取结果**：
- 考勤组名称：`groupVO.name`
- 考勤组类型：`groupVO.type`（必须为 TURN）
- 关联班次列表：`groupVO.shiftVOList[].shiftSetting.{shiftId, shiftName}`

**步骤 0c — 收集缺失参数**：

拿到考勤组和关联班次后，再向用户收集缺失的参数。排班所需的四个参数：
- **考勤组**：已在 0a 自动获取，无需询问
- **班次**：已在 0b 获取关联班次列表，展示给用户选择
- **员工范围**（必填）：指定员工姓名 / 部门 / 考勤组全员 / "给我排班"
- **日期范围**（必填）：具体日期 / 日期范围（如"下周"、"5月19日到5月23日"）

**只收集真正缺失的参数**，用户已经提供的不要重复询问；多个缺失参数可集中询问。日期选项必须展示实际计算后的日期范围，班次选项必须在运行时从 `shiftVOList` 动态生成，并以真实 `shiftId` 作为值、真实 `shiftName` 作为展示名称；可额外提供“排休”。只有一个关联班次时直接使用，无需再次选择。

   **动态选项原则（严格执行）**：
   - 班次选项**必须且只能**来自考勤组的 `shiftVOList`，**严禁编造任何班次名称**（如"正常班"、"早班"、"晚班"、"全部排XX班"等都是编造）
   - 每个班次选项的 `id` 必须是 `shiftVOList` 中的真实 `shiftId`，`label` 必须是真实的 `shiftName`
   - 如果 `shiftVOList` 为空，降级从 `groupVO.classIds` + `class get` 按 ID 精确查询（不是全局搜索）
   - **只收集缺失的参数**：用户已经提供的参数不要重复询问
   - **如果考勤组只有一个班次，直接使用该班次，不需要询问用户选择**
   - **禁止用 `class search` 全局搜索来给用户展示班次选项**——全局班次列表包含不属于该考勤组的班次，用户选了也会被校验拒绝

### 阶段 1: 确认考勤组（必须为排班制）

> **业务事实：用户只属于一个考勤组。** 不存在"选考勤组"的场景，直接通过 `dws attendance rules` 自动获取即可。只有在用户明确指定了一个考勤组名称时，才用 `group search` 按名称确认。

**方式 A — 自动获取（默认方式，适用于绝大多数场景）**：
```bash
# 用户只属于一个考勤组，直接查询即可确定
dws attendance rules --date <今天日期> --format json
# → 返回中提取 groupId，然后用 group get 获取详情
dws attendance group get --group-id <groupId> --format json
```

**方式 B — 用户明确指定考勤组名称时**：
```bash
dws attendance group search --query "<考勤组名称>" --type TURN --format json
```

**校验规则**：
1. 考勤组类型必须为 **TURN（排班制）**，如果是 FIXED（固定班制）或 NONE（自由工时），拒绝并提示"该考勤组不是排班制，无法进行排班操作"
2. 确认考勤组后，**必须立即获取其关联班次**（`groupVO.shiftVOList`），后续所有班次选项都从这里取

**提取信息**：考勤组 ID（`groupId`）、考勤组名称（`name`）、**关联班次列表（`shiftVOList`）**

### 阶段 2: 获取员工列表

**场景 A — 指定员工姓名**：
```bash
dws aisearch person --keyword "<员工姓名>" --dimension name --format json
```

**场景 B — 按部门查询**：
```bash
dws contact dept search --query "<部门名>" --format json
dws contact dept list-members --depts <deptId> --format json
```

**场景 C — 考勤组全员**：
```bash
dws attendance group filtered-get --group-id <groupId> --member --format json
```

**场景 D — 用户已给 userId 列表**：直接跳过本步。

### 阶段 3: 校验班次（必须是考勤组关联的班次）

> **业务约束：排班只能排考勤组关联的班次。** 阶段 0/1 已经通过 `dws attendance group get` 拿到了 `shiftVOList`，本阶段直接使用该数据校验，**不需要也不应该再调用 `class search` 全局搜索**。

**班次数据来源**（已在阶段 0 或阶段 1 获取）：
- `groupVO.shiftVOList[].shiftSetting.shiftId` — 班次 ID
- `groupVO.shiftVOList[].shiftSetting.shiftName` — 班次名称

**禁止调用 `dws attendance class search` 全局搜索班次**——全局搜索会返回不属于该考勤组的班次，即使用户指定了班次名称，也必须在 `shiftVOList` 中匹配，而不是全局搜索。

**校验规则**：
1. 用户选择的班次 `shiftId` 必须存在于 `shiftVOList` 中；所有候选都必须由 `shiftVOList` 动态生成
2. 如果用户通过自然语言指定了班次名称（如"排早班"），必须在 `shiftVOList` 中**按名称模糊匹配**，找到对应的 `shiftId`
3. 如果用户指定的班次不在 `shiftVOList` 中，**必须拒绝**，并列出该考勤组关联的全部班次让用户重新选择
4. 仅当 `shiftVOList` 为空时，才降级从 `groupVO.classIds` + `dws attendance class get` 按 ID 精确查询（仍然不是全局搜索）

**提取信息**：班次 ID（`classId` / `shiftId`）、班次名称（`shiftName`）

### 阶段 4: 回显排班内容并获得明确确认

> **[硬性门禁]** 必须先向用户展示**完整的排班明细表格**（包含每个人、每天、具体班次名称），再获得用户明确确认。
> **禁止在用户还不知道"谁、哪天、上什么班"的情况下请求确认**。

**步骤 4a — 展示排班明细（必须在请求确认之前）**：

请求确认前，**必须先**向用户展示排班内容。表格中**必须包含班次名称**（如"早班 09:00-18:00"），不能只展示 classId 数字：

```
排班预览

考勤组: <考勤组名称>（ID: <groupId>）
排班日期: <startDate> ~ <endDate>

| 员工姓名 | 日期 | 星期 | 班次 | 是否排休 |
|---------|------|------|------|---------|
| 张三 | 2026-05-19 | 周一 | 早班 09:00-18:00 | 否 |
| 张三 | 2026-05-20 | 周二 | 早班 09:00-18:00 | 否 |
| 张三 | 2026-05-21 | 周三 | 排休 | 是 |
| 李四 | 2026-05-19 | 周一 | 晚班 18:00-02:00 | 否 |
| ... | ... | ... | ... | ... |

共 <N> 条排班记录
```

**自查清单（展示表格前必须确认）**：
- [ ] 表格中有员工姓名（不是 userId）
- [ ] 表格中有具体日期和星期几
- [ ] 表格中有班次名称（不是 classId 数字）
- [ ] 排休的记录标注了"排休"
- [ ] 表格涵盖了所有待排班的员工和日期

**步骤 4b — 获得明确确认（必须在展示表格之后）**：

说明排班将覆盖所选日期的现有记录，并等待用户明确确认。确认后进入阶段 5；用户取消或未明确同意时不执行导入。

### 阶段 5: 调用脚本执行排班

```bash
python scripts/attendance_schedule_import.py \
  --group-id <groupId> \
  --schedules '<JSON数组>' \
  --confirm
```

参数说明：
- `--group-id`（必填）：考勤组 ID
- `--schedules`（必填）：排班记录 JSON 数组，每条记录包含 `userId`、`workDate`、`classId`、`isRest`
- `--confirm`（必填）：表示用户已确认，脚本收到此标志才会执行排班

脚本内部自动处理：
1. 二次校验考勤组类型（必须为 TURN）
2. 从考勤组详情提取绑定班次，二次校验班次 ID 属于该考勤组
3. 格式化 workDate 为 `yyyy-MM-dd HH:mm:ss`
4. 调用 `dws attendance schedule import` 执行排班
5. 输出执行结果摘要（含全部排班明细）

### 阶段 6: 返回结果给用户

- 将脚本 stdout 输出的摘要信息原样转告用户
- 如果脚本输出 warning，原样转告用户
- 如果执行失败，将 stderr 错误信息转告用户

## 排班记录 JSON 格式

每条排班记录的字段说明：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `userId` | string | 是 | 员工的 userId |
| `workDate` | string | 是 | 排班日期，格式 YYYY-MM-DD |
| `classId` | int | 是 | 班次 ID（从 `class search` 获取） |
| `isRest` | string | 是 | 是否排休，`Y`=排休 / `N`=正常上班 |

排休时 `classId` 传 0，`isRest` 传 `Y`。

## API 返回结构注意事项（Agent 必读）

> 以下是实际执行中多次踩坑的关键数据结构说明。**禁止凭直觉假设字段在顶层**，必须按本节描述的嵌套路径提取。

### `dws attendance group get` 返回结构

```
run_dws 解包后的结构（unwrap_result 去掉 success/result 包装后）：
{
  "groupVO": {           ← 关键！type/name/classIds 等字段在这一层
    "type": "TURN",      ← 考勤组类型
    "name": "研发组",
    "classIds": [1290384739, ...],  ← 绑定的班次 ID 列表
    "shiftVOList": [     ← 排班制特有，班次详情
      {
        "shiftSetting": {
          "shiftId": 1290384739,  ← 班次 ID（与 classIds 对应）
          "shiftName": "早班 09:00-18:00"
        }
      }
    ],
    "selectedClass": [...],  ← 部分环境使用此字段
    ...
  },
  ...其他顶层字段...
}
```

**提取规则**：
- 考勤组类型：`result["groupVO"]["type"]`
- 考勤组名称：`result["groupVO"]["name"]`
- 绑定班次 ID 列表：`result["groupVO"]["classIds"]`
- 班次名称：`result["groupVO"]["shiftVOList"][N]["shiftSetting"]["shiftName"]`
- **禁止从 result 顶层直接取 type/name/classIds，那里没有这些字段**

### `dws attendance class search` 返回结构

```
run_dws 解包后可能为以下之一：
1. 直接 list[dict]: [{id, name, ...}, ...]
2. {"data": [...]} 或 {"items": [...]} 或 {"classList": [...]}
```

**注意**：如果 `class search` 返回 0 条记录，不一定是错误——可能是当前账号没有班次管理权限。此时从 `group get` 的 `shiftVOList` 中也可获取班次名称。

### `dws aisearch person` 搜索同名问题

同一个姓名可能返回**多个不同 userId**（如主管理员账号 vs 子管理员账号）。必须通过以下方式确认正确的 userId：
1. 检查目标考勤组的成员列表：`dws attendance group filtered-get --group-id <id> --member`
2. 取成员列表中存在的那个 userId

**禁止直接取搜索结果的第一条 userId，必须与考勤组成员列表交叉验证。**

### `dws contact user get` 可能的权限错误

`resolve_user_names`（userId→姓名转换）可能遇到 `SECURITY_CHECK_INVOKE_FAILED` 错误。这只影响**展示层**，不影响排班数据的正确性：
- 脚本已内置降级处理：权限失败时直接使用 userId 替代姓名
- **不要因为姓名获取失败就中止排班流程**

### Agent 常见错误模式（严禁）

| 错误做法 | 正确做法 |
|------------|------------|
| `result.get("type")` 从顶层取类型 | `result["groupVO"]["type"]` |
| `result.get("classIds")` 从顶层取班次 | `result["groupVO"]["classIds"]` 或 `result["groupVO"]["shiftVOList"]` |
| 用 `python3 -c "..."` inline 脚本解析 JSON | 调用已有的 Python 脚本（`attendance_schedule_import.py`） |
| 人名搜到多个结果直接取第一个 | 与考勤组成员列表交叉验证 |
| 姓名获取失败就中止流程 | 降级用 userId 展示，继续执行排班 |
| 直接调用 `dws attendance schedule import` | 必须通过 `attendance_schedule_import.py` 脚本 |

## 错误处理

| 错误 | 原因 | 处理方式 |
|------|------|---------|
| 权限错误（403） | 当前账号非管理员 | 提示需要管理员权限，不要重试 |
| 考勤组不是排班制 | 考勤组类型为 FIXED 或 NONE | 提示"该考勤组不是排班制，无法排班" |
| 班次不在可用列表中 | classId 无效 | 列出可用班次让用户重新选择 |
| userId 无效 | 用户 ID 错误或已离职 | 提示具体哪个用户无效 |
| 脚本执行失败 | 接口异常/配置问题 | 将 stderr 错误信息转告用户 |
| SECURITY_CHECK_INVOKE_FAILED | userId→姓名转换权限不足 | 仅影响展示，降级用 userId，不中止流程 |
| class search 返回空列表 | 账号无班次管理权限 | 从 `group get` 的 `shiftVOList` 提取班次名称 |

## 使用示例

### 示例 1: 给指定员工排班
**用户说**: "帮我给张三下周一到周五排早班，考勤组是研发组"

```bash
# 1. 确认考勤组并获取关联班次（先查后排）
dws attendance group search --query "研发组" --type TURN --format json
# → 拿到 groupId
dws attendance group get --group-id <groupId> --format json
# → 从 groupVO.shiftVOList 获取关联班次列表
# → 在 shiftVOList 中匹配"早班"，拿到对应的 shiftId 作为 classId
# ⚠️ 禁止用 class search 全局搜索班次

# 2. 获取员工 userId
dws aisearch person --keyword "张三" --dimension name --format json

# 3. 回显确认（Agent 向用户展示排班表格）
# ... 用户确认 ...

# 4. 调用脚本执行
python scripts/attendance_schedule_import.py \
  --group-id 123456 \
  --schedules '[{"userId":"user001","workDate":"2026-05-19","classId":789,"isRest":"N"},{"userId":"user001","workDate":"2026-05-20","classId":789,"isRest":"N"},{"userId":"user001","workDate":"2026-05-21","classId":789,"isRest":"N"},{"userId":"user001","workDate":"2026-05-22","classId":789,"isRest":"N"},{"userId":"user001","workDate":"2026-05-23","classId":789,"isRest":"N"}]' \
  --confirm
```

### 示例 2: 给员工排休
**用户说**: "帮我把李四下周三排休"

```bash
# 1. 先查考勤组和关联班次（即使排休也需要确认考勤组）
dws attendance rules --date 2026-05-15 --format json
# → 拿到 groupId
dws attendance group get --group-id <groupId> --format json
# → 确认是排班制（TURN）

# 2. 获取员工 userId
dws aisearch person --keyword "李四" --dimension name --format json

# 3. 回显确认 → 用户确认 → 执行
python scripts/attendance_schedule_import.py \
  --group-id 123456 \
  --schedules '[{"userId":"user002","workDate":"2026-05-21","classId":0,"isRest":"Y"}]' \
  --confirm
```

### 示例 3: 部门批量排班
**用户说**: "帮我给研发部全员下周排早班"

```bash
# 1. 先查考勤组并获取关联班次（先查后排）
dws attendance rules --date 2026-05-15 --format json
# → 拿到 groupId
dws attendance group get --group-id <groupId> --format json
# → 从 groupVO.shiftVOList 中匹配"早班"，拿到 shiftId
# ⚠️ 禁止用 class search 全局搜索班次

# 2. 获取部门成员
dws contact dept search --query "研发部" --format json
dws contact dept list-members --depts <deptId> --format json

# 3. 回显确认 → 用户确认 → 执行
python scripts/attendance_schedule_import.py \
  --group-id 123456 \
  --schedules '[...]' \
  --confirm
```

## 配套脚本

| 脚本 | 用途 | CLI 参数 |
|------|------|---------|
| [attendance_schedule_import.py](../scripts/attendance_schedule_import.py) | 排班导入（含校验、回显、执行） | `--group-id --schedules --confirm` |
| [attendance_schedule_export.py](../scripts/attendance_schedule_export.py) | 排班查询导出（分批查询、排班表 Excel） | `--users --start --end [--output]` |

---

## 排班查询导出工作流

> 当用户提到"查看排班"、"排班表"、"导出排班"、"排班记录"时，走此工作流。
> **禁止直接调用 `dws attendance schedule get`**，必须通过脚本执行，脚本自动处理分批、姓名转换、班次名称转换、排班表格式输出。

### 查询导出 — 阶段 1: 参数收集

1. **员工范围**（必填）：需获取 userId 列表
   - 指定员工姓名 → `dws aisearch person` 获取 userId
   - 指定部门 → `dws contact dept search` + `dws contact dept list-members`
   - 指定考勤组全员 → `dws attendance group filtered-get --member`
   - 用户已给 userId 列表 → 直接使用
2. **日期范围**（必填）：开始日期 ~ 结束日期（YYYY-MM-DD）
   - 用户说"下周" → 计算下周一到周日
   - 用户说"本月" → 计算本月 1 日到月末
   - 任何缺失信息必须追问

### 查询导出 — 阶段 2: 调用脚本

```bash
python scripts/attendance_schedule_export.py \
  --users <userId1,userId2,...> \
  --start <YYYY-MM-DD> \
  --end <YYYY-MM-DD> \
  [--output <output_path.xlsx>]
```

参数说明：
- `--users`（必填）：userId 列表，逗号分隔
- `--start`（必填）：开始日期，格式 YYYY-MM-DD
- `--end`（必填）：结束日期，格式 YYYY-MM-DD
- `--output`（可选）：输出文件路径，默认 `attendance_schedule_<start>_<end>.xlsx`

脚本内部自动处理：
1. **分批查询**：超过 20 人自动分批调用 `dws attendance schedule get`
2. **班次名称转换**：classId → className（优先从记录中提取，缺失时回退 class search）
3. **姓名转换**：userId → 员工姓名
4. **排班表格式**：日历表（行=员工，列=日期，单元格=班次名称）
5. **Excel 输出**：钉钉风格美化排版

### 查询导出 — 阶段 3: 返回结果给用户

- 将脚本 stdout 输出的摘要信息（人数、日期、记录数、预览表格）原样转告用户
- 提醒用户完整排班表已导出到 Excel 文件
- 如果执行失败，将 stderr 错误信息转告用户

### 查询导出示例

**用户说**: "帮我导出研发组下周的排班表"

```bash
# 1. 获取考勤组成员
dws attendance group search --query "研发组" --format json
dws attendance group filtered-get --group-id <groupId> --member --format json

# 2. 调用脚本导出
python scripts/attendance_schedule_export.py \
  --users user001,user002,user003 \
  --start 2026-05-18 \
  --end 2026-05-24
```

**用户说**: "帮我查看张三和李四本月的排班"

```bash
# 1. 获取 userId
dws aisearch person --keyword "张三" --dimension name --format json
dws aisearch person --keyword "李四" --dimension name --format json

# 2. 调用脚本导出
python scripts/attendance_schedule_export.py \
  --users user001,user002 \
  --start 2026-05-01 \
  --end 2026-05-31
```
