# feat: 新增企业运动健康（钉钉运动）CLI 产品

## 变更内容

- 新增顶层产品命令 `dws sport`，把已上架 MCP 市场「企业运动健康服务（钉钉运动）」的 22 个工具接入为原子 CLI 命令：
  - `sport org status|orgs`：步数榜开通状态、用户所属企业列表
  - `sport step org-info|org-total|emp-ranking|dept-ranking|dept-emp-ranking|emp-info|emp-infos|dept-infos`：企业步数概况、区间总步数、员工/部门排行与多日明细
  - `sport competition create|config|rule-detail|instances|current-instance|next-instance|announce`：比赛创建（写操作）、配置/规则/期次查询、开赛播报（写操作，无幂等）
  - `sport stats signup|participants-daily|participants|reached|trend`：报名、参与、达标与逐日趋势统计
- `pkg/edition/default.go` 的 `openSupplementServers` 登记 server ID=sport（MCP 市场服务的稳定网关地址），并显式注册 Cobra 命令树（`internal/helpers/register_sport.go`）。
- `sport competition create` 在 CLI 侧本地校验：当前仅支持 FIXED（RECURRING 因服务端 Schema 与描述冲突被阻断）、yyyyMMdd 日期合法性、达标天数不超过单期天数、dailyLimit ≥ goalValue；确认门为 user_required（创建不可删除、播报无幂等）。

## 测试证据

- `DWS_PACKAGE_VERSION=0.0.0-test go test -count=1 ./internal/helpers -run 'TestSport'`：命令树覆盖全部 22 个工具、create 参数映射与本地校验边界
- `make build && ./dws sport --help`、`./dws schema --cli-path '<path>' --compact`：Help 与 Schema 一致
- 真实业务调用待目标组织身份实测（步数查询、比赛创建、播报各一条链路）
