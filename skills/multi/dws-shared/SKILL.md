---
name: dws-shared
description: 钉钉(DingTalk) MultiSkill 的轻量共享入口。Use when 用户泛称 DWS/钉钉操作但未明确产品、请求跨产品编排、需要 URL 类型预检或产品边界消歧。清晰的单产品操作优先使用对应 dingtalk-* 子 skill；本 skill 只提供全局执行契约和按需 reference 导航，不承载产品命令全集。
metadata:
  cli_version: ">=0.2.14"
  category: shared
  requires:
    bins:
      - dws
---

# DWS 共享执行契约

把本文件作为所有 DWS 操作的轻量公共前置条件。明确单产品请求直接使用对应
`dingtalk-*` skill；只有泛称 DWS、跨产品流程、URL 预检或意图不清时，才把本
skill 作为入口进行二级路由。

## 所有 DWS 操作必须遵守

- 只通过 `dws` CLI 操作钉钉产品；不要改用 curl、私有 HTTP API 或浏览器绕过。
- 所有结构化机器读取命令使用 `--format json`，按真实返回字段判断结果。
- 返回中的秒/毫秒时间戳必须转换为带时区的可读日期时间后再面向用户展示；默认使用
  当前会话时区（本环境为 `Asia/Shanghai`），必要时同时保留原始值，禁止只把裸时间戳
  交给 Agent 或用户判断具体日期。
- 不要猜测命令、flag、字段名、UUID、nodeId、userId、邮箱、手机号或 URL。
- 用户已明确产品内容意图时，意图优先于 URL 形态。`.md` 等本地格式文件按普通
  文件走 `drive`：先 `dws drive download` 下载到本地处理，再用 `dws drive upload`
  回传。
- 命令或参数不确定时，先运行最接近层级的 `dws <path> --help`；禁止把自然语言
  同义词直接拼成命令。
- 后续步骤使用的 ID 必须来自前一步真实返回；多人同名、多目标或类型不明时先消歧。
- 多账号场景只传组织时，使用该组织唯一 `isOrgCurrent=true` 的默认账号；多账号组织
  没有默认账号时先让用户指定账号，禁止选择第一项、最近登录或最近使用账号。账号
  选择与跨组织规则详见 `../dingtalk-profile/SKILL.md`。
- 不输出、转述或记录 token、refresh token、appSecret 等凭据。宿主已注入认证时
  不要求用户重新提供凭据。
- 写入操作必须符合用户明确意图。删除、审批决策及其他不可逆/高影响操作，先展示
  对象、动作和影响，获得明确同意后才追加 `--yes`；不得静默确认。
- 创建、更新、发送等写操作后，按产品 skill 的要求回读或查询状态，不得仅凭退出码
  宣称成功。

## 渐进加载

只读取当前任务需要的文件，不要一次性加载全部 shared references：

| 当前情况 | 必读内容 |
|---|---|
| 已明确单一产品 | 对应 `../dingtalk-*/SKILL.md`；不读路由 reference |
| 泛称 DWS、需要选择产品 | [routing.md](references/routing.md) |
| 跨产品、多步骤、汇总或报告 | [workflow-routing.md](references/workflow-routing.md) |
| 输入含 alidocs、shanji 等钉钉 URL 且类型不明 | [url-patterns.md](references/url-patterns.md) |
| 产品边界仍然难以判断 | [intent-guide.md](references/intent-guide.md) 的相关章节 |
| 认证、全局 flag 或输出格式问题 | [global-reference.md](references/global-reference.md) |
| 命令已经返回错误 | [error-codes.md](references/error-codes.md)；只查错误对应章节 |
| 怀疑能力不支持 | [capability-limits.md](references/capability-limits.md) |
| 批量/多源采集 | [conventions.md](references/best_practices/_common/conventions.md) |
| 固定短流程 | [lite-recipes.md](references/best_practices/_common/lite-recipes.md) 对应章节 |

产品命令、脚本和字段细节位于对应产品 skill，不在 `dws-shared` 重复维护。

## 本 skill 作为入口时的路由顺序

1. 先识别明确的产品内容意图；明确意图直接进入对应产品。仅当输入包含钉钉 URL
   且类型不明确或意图与链接类型可能冲突时，读取 `url-patterns.md` 识别节点类型。
2. 请求包含多个时序步骤、跨产品数据传递或汇总报告：即使 URL 已识别，也要读取
   `workflow-routing.md`，按行动指南组合需要的产品 skill；当前发布包不包含独立
   scenario skill。
3. 请求是单产品操作但产品不明确：读取 `routing.md`，再显式读取目标产品
   `SKILL.md`。
4. `doc/drive/wiki`、`aitable/sheet`、`calendar/conference/minutes` 等边界仍不清楚：
   只读取 `intent-guide.md` 的对应章节。
5. 仍无法判断时向用户追问，不要猜测产品或命令。

## 跨 skill 执行

- 正文中的相对 `Read` 链接是运行时依赖；`metadata.requires.skills` 不会自动加载。
- 选择目标产品后，以目标 skill 的命令、参数和风险规则为准。
- 多步骤流程按顺序传递真实返回值；可以并行的只读采集按对应 workflow/reference
  执行，写操作默认串行并逐步验证。
- 产品 skill 已内联的清晰操作直接执行；仅在遇到该 skill 未覆盖的参数或边界时读取
  更深层 reference。

## 错误最短路径

1. `unknown command` / `unknown flag`：运行对应层级 `--help`，修正后最多重试一次。
2. 认证或权限错误：读取 `global-reference.md` 与 `error-codes.md` 对应章节。
3. 其他错误：加 `--verbose` 重试一次；仍失败则停止并报告真实错误，不连续尝试替代
   命令。
4. 明确不支持的能力：说明边界，不通过其他接口绕过。
