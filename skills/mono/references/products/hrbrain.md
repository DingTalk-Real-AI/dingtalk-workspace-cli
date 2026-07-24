# Hrbrain（组织大脑）

## 产品说明

Hrbrain 是钉钉组织大脑，提供人才池管理、员工档案查询、人才搜索三大能力，帮助 HR/管理者盘点人才、查询员工履历绩效、按条件检索人员。

**CLI 前缀**: `dws hrbrain`

> ⚠️ **权限粒度**：`talent-pool list` 等接口需要账号单独开通"人才池查看权限"，即使账号已登录且其他 hrbrain 接口可用，仍可能因该细粒度权限缺失返回 `errorCode=2002`（无人才池查看权限）。

## 命令总览

### talent-pool (人才池管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `talent-pool list` | 查询人才池列表 | - | 可选 `--keyword`、`--pool-type`、`--creator`、`--labels`（逗号分隔）、`--page`、`--page-size`（默认 1/20） |
| `talent-pool detail` | 获取人才池详情 | `--pool-code` | 根据人才池编码查询 |
| `talent-pool employees` | 查询人才池内人员列表 | `--pool-code` | 可选 `--page`、`--page-size`（默认 1/20） |

### profile (员工档案管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `profile metadata` | 查询员工档案元数据结构 | `--work-no` | 用于构造 `profile query` 的 `--data-queries` |
| `profile query` | 按模块批量查询员工档案数据 | `--work-no` `--data-queries` | `--data-queries` 为 JSON 数组，每项含 `modelCode`（档案模块编码）与 `fields`（字段编码列表） |
| `profile labels` | 获取员工标签 | `--staff-ids` | `--staff-ids` 为逗号分隔工号列表；可选 `--all-label` |
| `profile career` | 查询员工公司内职业历程 | `--work-no` | - |
| `profile performance` | 查询员工绩效记录 | `--work-no` | - |

### search (人才搜索)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `search employees` | 人才搜索 | - | 可选 `--keyword`、`--dept-name`、`--position-name`、`--job-level`、`--pool-code`、`--page`、`--page-size` |
| `search employees-structured` | 使用高级条件搜索人员 | `--origin-json` `--fields` | 建议先调用 `search fields` 获取有权限的字段与操作符列表；`--origin-json` 为搜索表达式 JSON，`--fields` 为返回列定义 JSON 数组；可选 `--order-by`（逗号分隔）、`--page`、`--page-size` |
| `search fields` | 获取高级搜索字段列表 | - | 获取当前操作人有权限使用的字段，用于构造 `search employees-structured` 的参数 |

## 意图判断

用户说"人才池/储备干部池":
- 查看列表 → `talent-pool list`
- 详情 → `talent-pool detail`
- 池内人员 → `talent-pool employees`

用户说"员工档案/档案数据/员工信息模块":
- 元数据结构 → `profile metadata`
- 批量查数据 → `profile query`（先 `metadata` 确认字段编码）

用户说"员工标签/人才标签":
- `profile labels`

用户说"职业历程/职业发展/内部履历":
- `profile career`

用户说"绩效记录/绩效考核历史":
- `profile performance`

用户说"搜人/人才搜索/按条件找人":
- 简单条件（关键词/部门/职务/职级）→ `search employees`
- 复杂组合条件 → 先 `search fields` 获取字段，再 `search employees-structured`

## 核心工作流

```bash
# 人才池列表与详情
dws hrbrain talent-pool list --page 1 --page-size 20 --format json
dws hrbrain talent-pool list --keyword "储备干部" --pool-type TYPE --creator USER_ID --format json
dws hrbrain talent-pool detail --pool-code POOL_CODE --format json
dws hrbrain talent-pool employees --pool-code POOL_CODE --page 1 --page-size 20 --format json

# 员工档案：先查元数据确定字段编码，再批量查数据
dws hrbrain profile metadata --work-no WORK_NO --format json
dws hrbrain profile query --work-no WORK_NO \
  --data-queries '[{"modelCode":"basic","fields":["name","dept"]}]' \
  --format json

# 员工标签、职业历程、绩效
dws hrbrain profile labels --staff-ids WORK_NO1,WORK_NO2 --all-label --format json
dws hrbrain profile career --work-no WORK_NO --format json
dws hrbrain profile performance --work-no WORK_NO --format json

# 人才搜索：简单条件
dws hrbrain search employees --keyword "张三" --page 1 --page-size 20 --format json
dws hrbrain search employees --dept-name "技术部" --job-level P7 --pool-code POOL_CODE --format json

# 人才搜索：高级条件，先获取字段列表
dws hrbrain search fields --format json
dws hrbrain search employees-structured \
  --origin-json '{"rules":[{"field":"name","operator":"contains","value":"张"}],"combinator":"and"}' \
  --fields '[{"label":"姓名","value":"name"}]' \
  --page 1 --page-size 20 --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `talent-pool list` | `poolCode` | `talent-pool detail` / `talent-pool employees` 的 `--pool-code` |
| `talent-pool employees` | `workNo` | `profile *` 系列命令的 `--work-no` |
| `profile metadata` | `modelCode`、`fields` | `profile query` 的 `--data-queries` |
| `search fields` | 字段 `value`/`label` 列表 | `search employees-structured` 的 `--fields`、`--origin-json` 中的 `field` |
| `search employees` | `workNo` | `profile *` 系列命令的 `--work-no` |

## 注意事项

- `--data-queries`、`--fields`、`--origin-json` 均为 JSON 字符串参数，必须是合法 JSON，否则命令直接报错。
- `--staff-ids`、`--labels`、`--order-by` 为逗号分隔字符串，非 JSON 数组。
- `--page`/`--page-size` 默认值为 1/20；全局安装的旧版本 `dws` 可能不识别分页 flag，需确认二进制版本已包含 hrbrain 分页支持。
- `talent-pool list` 需要账号单独开通人才池查看权限；返回 `errorCode=2002` 时提示用户联系管理员开通权限，而非重试或换 profile。
