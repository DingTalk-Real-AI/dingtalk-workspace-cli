# dev 命令树功能测试报告

> 日期：2026-06-12 ~ 06-13 ｜ 分支：`feat/dws-dev` ｜ 被测二进制：本地源码构建
> 结论：**功能层 96/96 PASS；真实数据层 dev doc 全通过，dev app/connect 因预发网关 key 不可得降级（已确认，留内网验收）**。发现并修复缺陷 1 个（`version create` 缺 `--version` 必填校验）。

## 范围与方法

- 被测对象：本次重命名与参数优化后的全部 dev 命令树——`dws dev app *`（CRUD/凭证/网页应用/权限/成员/安全/机器人/版本，34 条）、`dws dev connect`、`dws dev doc search`，以及旧名移除验证。
- 方法：CLI 功能层测试。`--dry-run` 验证 flag 解析、CLI→MCP 参数映射、命令路由；错误用例验证必填校验、写守卫（无 `--yes` 拦截）、未知渠道/未知 flag 拒绝；别名用例验证 `--app-id`/`--permission`/`--scope`/`--limit`/`--offset` 隐藏兼容。
- 执行：`test/dev_functional/run_cases.py`，20 并发（ThreadPool），单用例超时 30s。
- 边界声明：devapp 是 helper-only 产品，真实 MCP 调用需内部版注入 endpoint（或 `DINGTALK_DEVAPP_MCP_URL`），本报告不含真实上游调用；上游契约（字段语义、错误码）以预发真机验收为准。

## 结果总览

| 命令组 | 用例 | 通过 | 覆盖要点 |
|--------|------|------|----------|
| 根/旧名 | 4 | 4 | dev 三支柱 help；`devapp`/`app` 旧名确认移除 |
| app list | 5 | 5 | 无参/按名/agentId/分页映射/排序 |
| app get | 3 | 3 | 两种定位 + 缺定位报错 |
| app create | 4 | 4 | 全字段/缺 name/`--type` 确认已删/写守卫 |
| app update | 3 | 3 | 改名/无更新字段报错/缺定位报错 |
| 生命周期 | 5 | 5 | delete/inactive/active + 守卫 + 缺定位 |
| credentials | 3 | 3 | 两种定位 + 缺定位报错 |
| webapp | 5 | 5 | get/单字段/多字段/无字段报错/守卫 |
| permission list | 8 | 8 | 默认分页/`--page` 换算 offset（3,50→100,50）/旧 flag 兼容/keyword/scope 及别名/search 别名/缺定位 |
| permission add | 5 | 5 | 批量/两种别名/缺参/守卫 |
| permission remove | 6 | 6 | 单条原样出参/批量 results 聚合/两种别名/缺参/守卫 |
| member | 8 | 8 | 新主名/`--app-id` 兼容/缺参报 `--unified-app-id`/trim/守卫 |
| security | 6 | 6 | 三字段各自/组合/无字段报错/守卫 |
| robot | 13 | 13 | 同步建/异步 submit+result/get/config/update/enable/offline/缺参/守卫/`robot connect` 确认已移除 |
| version | 8 | 8 | create/list/get/check-approval/publish(approver+confirm-sensitive)/status/缺 version 报错/守卫 |
| dev connect | 5 | 5 | 显式凭证 dry-run/unified-app-id 凭证源/未知渠道拒绝/缺凭证报错/agent 参数全开 |
| dev doc | 4 | 4 | --query/位置参数/分页/缺参 |
| pretty | 1 | 1 | pretty 格式不崩溃 |
| **合计** | **96** | **96** | |

执行耗时：累计 107.7s（20 并发墙钟约 8s），最慢用例 cred-agent 3.1s。

## 缺陷与修复

| # | 严重度 | 描述 | 修复 |
|---|--------|------|------|
| 1 | 中 | `dev app version create` 不传 `--version` 通过 CLI 校验，空版本号会直接下发上游 | `devapp.go` 补必填校验，报 `--version is required`；用例 ver-create-missing 回归通过 |

## 复跑方式

```bash
go build -o /tmp/dws-dev ./cmd
python3 test/dev_functional/run_cases.py /tmp/dws-dev test/dev_functional/results.jsonl
```

用例与预期内联在 `run_cases.py`（id / 命令 / 期望退出码 / 输出必含子串），新增命令时在 CASES 里加行即可。

## 真实数据验证（2026-06-13）

- 认证：`auth status` 真实校验通过（token 有效，corp ding8196...b85）。
- `dev doc search` 真实调用 **3/3 PASS**（devdoc 是正常发现的 MCP 服务，不依赖 op-app 网关）：
  - `--query "errcode 40035"` → 返回真实文档命中（hasMore 翻页字段正常）
  - 位置参数 `"机器人回调" --size 3` → 命中「机器人接收消息」等相关文档，size 生效
  - 出参结构与 output-schema.md 描述一致（`result.items[].title/url` + `success`）
- `dev app *` / `dev connect` 真实调用被阻断：devapp 固定指向 `pre-mcp-gw.dingtalk.com/server/op-app`，网关要求带 `?key=` 的完整地址（按安全设计不入库），本机日志/历史/配置均无该 key，裸地址连接被网关断开（EOF，有 recovery 事件记录）。已与负责人确认采用降级方案：真实链路留待内网环境验收，重点验 permission remove 批量真实聚合、connect 真实建联、version publish 审批链路。注入方式：`DINGTALK_DEVAPP_MCP_URL='<含key地址>' /tmp/dws-dev dev app list`。

## cursor 分页改造验证（2026-06-15）

4 个分页命令（app list / permission list / version list / doc search）对外改为游标分页（`--cursor` + `--page-size`，出参注入 `nextCursor`/`hasMore`）。上游仍 page/offset，CLI 合成 cursor 顶替，上游上线真游标后透传。设计规格见 `docs/cursor-pagination-design.md`。

- 功能用例扩到 **106 个全 PASS**（新增 10 个 cursor 用例：解码续翻、跨命令 cursor 拒绝、非法 cursor 拒绝、legacy flag 兼容，覆盖 4 个命令）。
- cursor 单测 7 个全 PASS（编解码往返、跨工具拒绝、非法拒绝、满页合成/到底/上游透传/nil 安全）。
- 真实数据验证：`dev doc search` 三页连翻（`--page-size 2` → `nextCursor` 续翻），三页首条互不相同、`hasMore` 正确翻转，到底返回空 cursor。
- dev app/permission/version 的 cursor 解码经 dry-run 验证（cursor→currentPage/offset 换算正确，跨命令 cursor 报「不属于当前命令」）；真实游标续翻待内网验收。

## 遗留

- 真实上游调用（预发 endpoint + 真实凭证）未覆盖，需内部版环境验收：重点验 permission remove 批量逐条调用的真实聚合、connect 真实建联、version publish 审批链路。
- `dws dev connect` 正式（非 dry-run）模式为前台长驻进程，不适合自动化用例，建联 e2e 需人工或专用 harness。
