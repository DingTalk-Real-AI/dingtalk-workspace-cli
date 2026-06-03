# audit-ingest —— dws 审计转发的参考接收端

dws 开启审计转发后（`DWS_AUDIT_FORWARD_URL`），每次命令会 POST 一条审计事件到你的端点。
这个目录是一个**最小参考实现**，先让你本地把整条链路跑通，再照搬到阿里云 SLS。

## 接收契约（dws 这边固定）

```
POST /
Content-Type: application/json
Authorization: Bearer <token>        # 可选；下方 -token 设了就强校验
X-Dws-Audit-Schema: 2
Body: 一条审计事件 JSON
返回 2xx 即算成功（否则 dws 端按失败处理，本地文件仍是源头真相可回补）
```

## 1. 本地验证版（纯标准库，开箱即用）

```bash
# 起接收端
go run ./examples/audit-ingest -addr :8088 -token secret-token -out ingest.jsonl

# 另开一个终端，让 dws 真转发过来
export DWS_AUDIT_ENABLED=true
export DWS_AUDIT_FORWARD_URL=http://localhost:8088
export DWS_AUDIT_FORWARD_TOKEN=secret-token
export DWS_AUDIT_FORWARD_REDACT=none
dws minutes list mine --max 2 --format json
# 看 ingest.jsonl，每行一条审计事件
```

落地点只有一个函数 `fileSink.write()`，换成 SLS 写入就是生产版（见下）。

## 2. 阿里云 SLS 版（生产推荐：自带存储/检索/Dashboard/留存）

### 2.1 在 SLS 控制台开通

1. 阿里云控制台 → **日志服务 SLS** → 创建 **Project**（如 `dws-audit`）。
2. Project 下创建 **Logstore**（如 `events`），设留存天数（合规一般 180/365 天）。
3. 给 Logstore 开**索引**，把 `trace_id`、`command`、`subcommand`、`outcome`、`corp_id`、`agent_id`
   设为字段索引，方便检索与做 Dashboard。
4. 记下 **Endpoint**（如 `cn-hangzhou.log.aliyuncs.com`）、**Project**、**Logstore**，
   并准备一个有写权限的 **AccessKey**（建议用 RAM 子账号，仅授予 `log:PostLogStoreLogs`）。

### 2.2 把 write() 换成 SLS PutLogs

依赖：`go get github.com/aliyun/aliyun-log-go-sdk`

```go
import sls "github.com/aliyun/aliyun-log-go-sdk"

type slsSink struct {
	client             sls.ClientInterface
	project, logstore  string
}

func newSLSSink() *slsSink {
	c := sls.CreateNormalInterface(
		os.Getenv("SLS_ENDPOINT"),
		os.Getenv("SLS_AK"), os.Getenv("SLS_SK"), "")
	return &slsSink{client: c,
		project:  os.Getenv("SLS_PROJECT"),
		logstore: os.Getenv("SLS_LOGSTORE")}
}

// 把整条事件作为一个 event 字段，另抽几个 top-level 字段做索引。
func (s *slsSink) write(body []byte) error {
	var e map[string]any
	_ = json.Unmarshal(body, &e)
	str := func(k string) string { v, _ := e[k].(string); return v }
	contents := []*sls.LogContent{
		{Key: proto.String("event"), Value: proto.String(string(body))},
		{Key: proto.String("trace_id"), Value: proto.String(str("trace_id"))},
		{Key: proto.String("command"), Value: proto.String(str("command"))},
		{Key: proto.String("subcommand"), Value: proto.String(str("subcommand"))},
		{Key: proto.String("outcome"), Value: proto.String(str("outcome"))},
	}
	if org, ok := e["org"].(map[string]any); ok {
		if cid, ok := org["corp_id"].(string); ok {
			contents = append(contents, &sls.LogContent{Key: proto.String("corp_id"), Value: proto.String(cid)})
		}
	}
	lg := &sls.LogGroup{Logs: []*sls.Log{{
		Time:     proto.Uint32(uint32(time.Now().Unix())),
		Contents: contents,
	}}}
	return s.client.PutLogs(s.project, s.logstore, lg)
}
```

把 `handler.sink` 从 `*fileSink` 换成 `*slsSink` 即可（接口一致：`write([]byte) error`）。

### 2.3 部署：函数计算 FC（最省运维）

1. 阿里云 → **函数计算 FC** → 创建函数，运行时 Go，触发器选 **HTTP 触发器**。
2. 入口换成 FC 的 HTTP handler 形态（FC Go SDK 的 `RegisterHttpHandler`），逻辑就是上面的 `ServeHTTP`。
3. 环境变量配 `SLS_ENDPOINT/SLS_PROJECT/SLS_LOGSTORE/SLS_AK/SLS_SK` 和接收用的 `-token`。
4. 拿到 FC 的 HTTP 触发器公网地址，作为 `DWS_AUDIT_FORWARD_URL` 下发给各端 dws。

> 也可直接跑在 ECS / K8s 上（本目录二进制 + 反代 HTTPS），看你们运维习惯。
> SLS 本身就有查询、告警、Dashboard，落进去后“谁、什么时候、操作了什么、成没成”直接在 SLS 控制台查。

## 数据边界提醒

- 转发是**尽力而为**，本地 `audit.jsonl` 始终是源头真相，FC/SLS 偶发不可用可从本地回补。
- 合规全量审计建议进**企业自有** SLS；若要钉钉平台统一收，请走 `minimal` 档的匿名遥测，别把全量含身份数据集中到厂商侧。详见 `docs/audit.md`。
