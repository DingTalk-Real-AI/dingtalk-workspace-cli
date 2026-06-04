# 运维遥测（Telemetry）

dws 可以为**每一次命令调用**上报一条**匿名、纯维度**的运维指标，用于监控
错误率、延迟、命令分布和版本/平台健康度。它是审计（[audit](./audit.md)）的运维侧
对应物，但刻意做得**小得多**：

- 只采**粗维度**，绝不采对象名、自由文本、peer id、设备指纹、自然语言原文。
  没有"脱敏档"，因为压根没有敏感字段可脱。
- **独立于审计**：和 `DWS_AUDIT_*` 互不相关，可以只开遥测不开合规审计。
- **默认全关**。不设 `DWS_TELEMETRY_ENABLED` 时，dws 不产生任何遥测，热路径零影响。

> 这是开源 CLI，集中上报必须 **opt-in + 明确告知**。默认不上报一个字节。

## 启用

| 环境变量 | 说明 | 示例 |
|---|---|---|
| `DWS_TELEMETRY_ENABLED` | 启用遥测（需同时配 URL 才生效） | `true` |
| `DWS_TELEMETRY_URL` | 上报端点，每次调用 POST 一条 JSON | `https://telemetry.example.com/dws` |
| `DWS_TELEMETRY_TOKEN` | 端点的 Bearer 鉴权（可选） | `xxxxx` |
| `DWS_TELEMETRY_TIMEOUT_MS` | 单次上报超时上限，毫秒（默认 1500） | `1500` |

## 上报字段（全部）

```json
{
  "schema_version": "1",
  "ts": "2026-06-04T11:38:24+08:00",
  "trace_id": "76a04f9eba0ad00c",   // == 传输层 execution_id，可与服务端日志 join
  "corp_id": "ding...",              // 租户维度，best-effort（取自登录 token）
  "cli_version": "1.0.34",           // 版本健康："这版本是不是把某命令搞挂了"
  "channel": "openclaw",             // 哪个 agent/集成在调用（DWS_CHANNEL）
  "os": "darwin",                    // 粗平台，非 PII
  "module": "doc",
  "command": "doc",
  "subcommand": "create_document",
  "outcome": "ok",                   // ok | error
  "err_class": "",                   // outcome=error 时的错误分类
  "exit_code": 0,
  "duration_ms": 73                  // 调用墙钟耗时，用于 P99
}
```

**刻意不采**（看这个 struct 就能验证隐私边界）：用户身份（user_id/姓名）、
对象名/id、自由文本、设备 id/序列号、请求/响应 body。

## 接收端契约

任何 HTTP 服务都能接：

```
POST /
Content-Type: application/json
Authorization: Bearer <token>        # 对应 DWS_TELEMETRY_TOKEN
X-Dws-Telemetry-Schema: 1
Body: 一条遥测事件 JSON
返回 2xx 即成功
```

## 本地测试（零依赖，不碰 SLS）

上 SLS 之前，先在本机把整条链路跑通。用 `fc-sls-ingest/localsink.py`
（纯 Python 标准库，不用 `pip install` 任何东西）当接收端：

```bash
# 1. 起本地接收端（带一个测试 token）
cd docs/telemetry/fc-sls-ingest
TOKEN=dev python3 localsink.py          # 监听 127.0.0.1:8799，落盘 /tmp/dws_telemetry.jsonl

# 2. 另开一个终端，把 dws 指向它
export DWS_TELEMETRY_ENABLED=true
export DWS_TELEMETRY_URL=http://127.0.0.1:8799
export DWS_TELEMETRY_TOKEN=dev

# 3. 跑几条命令（--mock 不联网、不需要真实后端，也会触发上报）
dws doc create --title 测试 --mock
dws drive list --mock
```

接收端会实时打印每条事件，并追加到 `/tmp/dws_telemetry.jsonl`。验证要点：

- 事件含 `command/outcome/duration_ms/cli_version/channel/os` 等维度；
- 把命令参数（如 `--title 测试`）和报文对照，确认**内容没出现在报文里**；
- 不带 token POST 应被拒（401）。

落盘后可以本地先模拟一把"大盘"会算的指标：

```bash
python3 - <<'PY'
import json, collections
rows=[json.loads(l) for l in open('/tmp/dws_telemetry.jsonl') if l.strip()]
by=collections.defaultdict(lambda:{'n':0,'err':0,'dur':[]})
for r in rows:
    k=f"{r['command']} {r['subcommand']}"; b=by[k]
    b['n']+=1; b['err']+=(r['outcome']!='ok'); b['dur'].append(r.get('duration_ms',0))
for k,v in sorted(by.items(), key=lambda x:-x[1]['n']):
    d=v['dur']; print(f"{k:<26}调用{v['n']:>4} 失败{v['err']:>3} avg{sum(d)//len(d):>5}ms max{max(d):>5}ms")
PY
```

> 说明：遥测只在命令真正进入 MCP 调用阶段才上报。若命令在参数解析层就报错
> （未到调用），不会产生遥测——这是预期行为。

## 开源代码与内部资源的边界（公私边界）

dws 是开源仓库，但**遥测数据进哪个 SLS、绑哪个内部应用，是部署方自己的事，不进仓库**。
这条边界是设计出来的，不是巧合：

| | 在哪 | 包含什么 | 进仓库吗 |
|---|---|---|---|
| dws 二进制 + 本目录 FC/local 参考代码 | 公开仓库 | 只会 POST 到 `DWS_TELEMETRY_URL`；**无 endpoint、无密钥、无应用名** | ✅ |
| SLS Project / FC 实例 / 真实 URL+token | 部署方内部基础设施 | 真实地址、鉴权、日志库；阿里内部还需绑定一个内部应用 | ❌ 永不进仓库，靠环境变量注入 |

代码里**绝不硬编码任何厂商上报地址**，URL 一律运行时从环境变量读取。所以"代码公开"
与"数据落到部署方内部 SLS"天然解耦：换部署方只是换一组环境变量，仓库无需改动，
也看不到任何一方的真实配置。

> 阿里内部场景：SLS Project 需挂在一个 AONE 应用下（资源治理要求）。把它绑到 dws
> 后端所属的应用（如钉钉 MCP 网关应用）即可；这个绑定关系、真实 URL 与 token 全部
> 留在内部，公开仓库不感知。

## 接入阿里云 SLS（生产推荐）

SLS（日志服务）自带写入 / 存储 / 检索 / Dashboard / 告警，是运维监控的标准选型：

1. **建库**：SLS 控制台建 Project + Logstore（如 `dws-telemetry`），设留存天数；
   给 `command` / `subcommand` / `outcome` / `cli_version` / `corp_id` / `channel`
   开字段索引，`duration_ms` 设为 long 型索引（要算 P99）。
2. **建接收端点**：用**函数计算 FC** HTTP 触发器最省运维——校验 Bearer 后把 body
   作为一条日志 `PutLogs` 写进 Logstore（整条 JSON 放 `event` 字段，另抽
   `command`/`outcome`/`duration_ms`/`cli_version` 做索引列）。
3. **下发**：把 FC 地址作为 `DWS_TELEMETRY_URL` 配到各端 dws。

### 上手就能用的 4 条告警（SLS 告警规则）

| 告警 | SLS 查询（示意） | 触发 |
|---|---|---|
| 错误率突增 | `* \| select count_if(outcome='error')*1.0/count(*) as err_rate` | err_rate > 5% |
| P99 延迟超标 | `* \| select approx_percentile(duration_ms, 0.99) as p99` | p99 > 3000 |
| 某命令大面积失败 | `* \| select command, count_if(outcome='error') c group by command order by c desc` | 单命令 c 突增 |
| 调用量跌零 | `* \| select count(*)` | 5 分钟内 == 0 |

告警通知渠道直接选钉钉机器人。

## 数据落在哪 / 两条流

- **不开 = 不出本机。** dws 不内置任何厂商默认上报地址。
- **企业自有监控**：`DWS_TELEMETRY_URL` 指向企业自己的 SLS ingest。
- **平台侧统一监控**：URL 指向钉钉的遥测 ingest——技术可行，但必须 opt-in + 告知。
  因为本遥测**只含匿名维度**，隐私边界天然干净，适合做平台运维大盘。
- 合规全量留痕是另一条线，走 [audit](./audit.md) 的企业自有 sink，别和遥测混。
