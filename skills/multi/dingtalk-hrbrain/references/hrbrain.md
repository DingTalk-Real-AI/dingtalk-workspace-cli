# Hrbrain（组织大脑）

## 产品说明

Hrbrain 是钉钉组织大脑，提供人才池管理、员工档案查询、人才搜索三大能力。

**CLI 前缀**: `dws hrbrain`

## 命令总览

### talent-pool (人才池管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `talent-pool list` | 查询人才池列表 | - | 可选 `--keyword`、`--pool-type`、`--creator`、`--labels`（逗号分隔）、`--page`、`--page-size` |
| `talent-pool detail` | 获取人才池详情 | `--pool-code` | - |
| `talent-pool employees` | 查询人才池内人员列表 | `--pool-code` | 可选 `--page`、`--page-size` |

### profile (员工档案管理)

| 命令 | 用途 | 必填参数 | 备注 |
|------|------|----------|------|
| `profile metadata` | 查询员工档案元数据结构 | `--work-no` | 用于构造 `profile query` 的 `--data-queries` |
| `profile query` | 按模块批量查询员工档案数据 | `--work-no` `--data-queries` | `--data-queries` 为 JSON 数组，每项含 `modelCode`、`fields` |
| `profile labels` | 获取员工标签 | `--staff-ids` | 逗号分隔工号列表；可选 `--all-label` |
| `profile career` | 查询员工公司内职业历程 | `--work-no` | - |
| `profile performance` | 查询员工绩效记录 | `--work-no` | - |

### search (人才搜索)

| 命令 | 用途 | 必填参数 |
|------|------|----------|
| `search employees` | 人才搜索（简单条件） | - |
| `search employees-structured` | 高级结构化搜索 | `--origin-json` `--fields` |
| `search fields` | 获取高级搜索字段列表 | - |

## 意图判断

- "人才池/储备干部池" → `talent-pool list/detail/employees`
- "员工档案/档案数据" → `profile metadata/query`
- "员工标签" → `profile labels`
- "职业历程/内部履历" → `profile career`
- "绩效记录" → `profile performance`
- "搜人/按条件找人" → `search employees`（简单）或 `search fields` + `search employees-structured`（复杂）

## 常用命令

```bash
dws hrbrain talent-pool list --page 1 --page-size 20 --format json
dws hrbrain talent-pool detail --pool-code POOL_CODE --format json
dws hrbrain talent-pool employees --pool-code POOL_CODE --format json
dws hrbrain profile metadata --work-no WORK_NO --format json
dws hrbrain profile query --work-no WORK_NO --data-queries '[{"modelCode":"basic","fields":["name","dept"]}]' --format json
dws hrbrain profile labels --staff-ids WORK_NO1,WORK_NO2 --format json
dws hrbrain profile career --work-no WORK_NO --format json
dws hrbrain profile performance --work-no WORK_NO --format json
dws hrbrain search employees --keyword "张三" --format json
dws hrbrain search fields --format json
dws hrbrain search employees-structured --origin-json '{"rules":[{"field":"name","operator":"contains","value":"张"}],"combinator":"and"}' --fields '[{"label":"姓名","value":"name"}]' --format json
```

## 安全规则

- `--data-queries`、`--fields`、`--origin-json` 必须是合法 JSON；`--staff-ids`、`--labels`、`--order-by` 是逗号分隔字符串。
- `talent-pool list` 需要账号单独开通人才池查看权限（`errorCode=2002` 时提示用户联系管理员开通）。
