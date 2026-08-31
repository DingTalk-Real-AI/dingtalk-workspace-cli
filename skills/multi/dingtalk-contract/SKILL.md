---
name: dingtalk-contract
description: 钉钉智能合同。Use when 用户要查询或创建合同台账、批量导入合同、按听记起草合同、发起合同审查、归档合同、管理合同项目、相对方或账款。不做普通钉盘文件管理（走 dingtalk-drive）、听记内容查询（走 dingtalk-minutes）、OA 审批处理（走 dingtalk-misc）。命令前缀：dws contract。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉智能合同 Skill

## 前置条件

> 执行任何 `dws` 操作前，先完整读取 [`dingtalk-shared`](../dingtalk-shared/SKILL.md)，遵循其中的账号、确认、结构化输出和错误处理契约。

> 按当前任务读取 [contract.md](references/contract.md) 中对应章节，不要一次加载全部命令。智能合同当前是隐藏 vendor extension，不进入 Agent Schema；参数与可用性以精确 leaf 的 `--help` 为准。

所有调用使用 `dws contract ... --format json`。不要使用旧的 dingtalk 二级入口，不要直接调用 MCP、HTTP API 或自行猜测工具名。

## 意图路由

| 用户意图 | 命令族 | 读取 reference 章节 |
|---|---|---|
| 查合同列表、详情、状态数量、创建台账 | `dws contract record ...` | 台账 |
| 从钉盘模板批量导入合同、查导入结果 | `dws contract import ...` | 批量导入 |
| 查审批模板或台账分类 | `process-templates` / `file-directories` | 基础资料 |
| 根据 AI 听记和模板起草合同 | `draft` | 合同起草 |
| 解析合同、创建审查任务、查审查结果或权益 | `review ...` | 合同审查 |
| 管理合同项目、导入导出项目 | `project ...` | 项目管理 |
| 管理相对方、工商信息、风险、导入导出 | `subject ...` | 相对方管理 |
| 管理合同收付款账款 | `account ...` | 账款管理 |
| 将合同文件归档 | `archive` | 合同归档 |

## 执行规则

1. 先定位真实对象。更新、删除、导出、归档前，先通过对应 `list` / `get` / `detail` 获取真实 ID 和当前状态；禁止根据名称猜 ID。
2. 复杂请求只通过 `--file <json>` 或 `--file -` 传递。提交前检查 JSON 是对象，字段名、必填项和枚举符合 reference 与当前 leaf help；不要把整个请求 JSON 拼成未声明的 flags。
3. 时间单位不得混用：台账与项目的筛选日期使用 ISO-8601；账款 `executionDate`、账款筛选 `--exec-start/--exec-end`、归档 `archiveTime` 使用 Unix 毫秒时间戳。
4. 异步操作保存创建响应中的真实 `taskId`，再调用配套结果命令。结果未完成时只报告处理中；仅在返回明确可重试状态时轮询，并遵守服务端重试间隔。
5. 删除项目、相对方、账款以及归档等不可逆或高影响操作，在执行前说明对象和影响并获得用户确认；确认后才在 Runtime gate 要求时添加 `--yes`。
6. 创建或更新后优先使用详情查询回读。没有对应详情接口时，保留原始回执并明确说明未能独立回读验证。
7. 列表存在分页参数时按返回继续翻页，直到满足用户范围或服务端表明结束；不要把第一页当成完整结果。

## 标准流程

### 查询合同

1. 用 `record list` 按时间、状态或查询维度缩小范围。
2. 零结果直接说明；多候选先列出名称、状态和合同 ID，让用户消歧。
3. 需要完整字段时，用真实 `contractId` 调用 `record get`。

### 创建台账或归档

1. 查 `file-directories`、`process-templates` 或钉盘文件信息，取得真实目录、模板和文件 ID。
2. 按 reference 构造 JSON 文件，向用户确认提交对象与关键字段。
3. 创建使用 `record create`；归档使用 `archive`。执行后根据返回的真实 ID 回读或报告状态。

### 合同审查

1. 可先执行 `review benefit` 确认权益。
2. 不确定审查类型或推荐模型时，先用 `review analysis` 解析文件。
3. 用 `review create` 创建任务，保存真实 `taskId` 和 `reviewType`。
4. 用 `review result` 查询结果；未完成不得宣称审查已完成。

### 批量导入

1. 从模板命令或已有钉盘文件取得真实 `fileId` / `spaceId`。
2. 创建导入任务并保存真实 `taskId`。
3. 使用同一命令族的 `*-result` 查询结果，逐项报告成功、失败和处理中状态。

## 跨产品边界

- 读取 AI 听记的 `taskUuid` 走 `dingtalk-minutes`；得到真实 ID 后再调用 `contract draft`。
- 查找、上传或下载钉盘合同文件走 `dingtalk-drive`；合同台账、审查与归档仍由本 skill 处理。
- 合同 OA 审批实例的查询、同意、拒绝、转交或撤销走 `dingtalk-misc`，不要与 `process-templates` 混淆。
