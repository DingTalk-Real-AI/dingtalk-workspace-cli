# dws 遥测接收端（函数计算 FC → SLS）

这是 [运维遥测](../../telemetry.md) 的**参考接收端**：dws 把一条遥测 JSON POST 过来，
SLS 不能直接收裸 POST（写入要签名），所以这里垫一个最小 HTTP 服务，校验 token 后用
`PutLogs` 写进 SLS。部署成函数计算（FC）的 **Web 函数**即可，不用关心 FC handler 签名。

```
dws  ──POST 一条 JSON──▶  本服务(FC Web 函数)  ──PutLogs──▶  SLS Logstore  ──▶ 大盘/告警
```

## 文件

- `app.py` — Flask 服务：`POST /` 校验 Bearer → 解析 JSON → 写 SLS；`GET /` 健康检查
- `requirements.txt` — 依赖（flask / gunicorn / aliyun-log-python-sdk）

## 一、先在 SLS 建库（控制台点几下）

1. 建 **Project**（如 `dws-ops`）和 **Logstore**（如 `dws-telemetry`），设留存天数。
2. 开索引：给 `command` / `subcommand` / `outcome` / `cli_version` / `corp_id` / `channel`
   设为 **text**；给 `duration_ms` / `exit_code` 设为 **long**（要做 P99 和聚合）。

## 两种运行模式（自动判断）

`app.py` 按环境变量自动切换，**不用改代码**：

| 模式 | 触发条件 | 行为 |
|---|---|---|
| **dry-run** | 缺任一 SLS 变量，或设 `TELEMETRY_DRYRUN=true` | 收到事件只打到 stdout（FC 会进函数日志），返回 204。**不依赖 aliyun-log SDK**，适合先验证管线 |
| **SLS** | `SLS_ENDPOINT`+`SLS_PROJECT`+`SLS_LOGSTORE` 都配齐 | 校验后 `PutLogs` 写进 Logstore |

`GET /` 健康检查会回显当前模式（`mode=dry-run` / `mode=sls`），部署后一眼可辨。

## 二、部署本服务为 FC Web 函数

1. 函数计算控制台 → 创建函数 → **Web 函数** → Python 运行时。
2. 上传本目录代码（含 `requirements.txt`，FC 会自动装依赖）。
3. **启动命令**填：`gunicorn -b 0.0.0.0:9000 app:app`，**监听端口** `9000`。
4. **先空跑验证（强烈建议）**：第一次只配 `INGEST_TOKEN`，**不配 SLS 变量**（或加
   `TELEMETRY_DRYRUN=true`）。部署后 `GET /` 应显示 `mode=dry-run`；把 dws 指过来跑
   几条命令，去 FC 的**函数日志**里能看到 `DRYRUN {...}` 行，就证明"客户端→FC"这段通了。
   这一步**不需要 SLS、不需要建库、不需要 SDK**。
5. **再接 SLS**：给函数**绑定一个服务角色**，授权 `AliyunLogFullAccess`（或更小的
   PutLogs 权限）——这样不用把 AccessKey 写进环境变量，FC 自动注入 STS 临时凭证，
   `app.py` 已优先读它。然后补上 SLS 环境变量，`GET /` 变成 `mode=sls` 即生效：

   | 变量 | 值 | 说明 |
   |---|---|---|
   | `SLS_ENDPOINT` | `cn-hangzhou.log.aliyuncs.com` | 按你的地域改 |
   | `SLS_PROJECT` | `dws-ops` | 第一步建的 Project |
   | `SLS_LOGSTORE` | `dws-telemetry` | 第一步建的 Logstore |
   | `INGEST_TOKEN` | 自己生成一串随机串 | 必须和 dws 侧 `DWS_TELEMETRY_TOKEN` 一致 |

6. 部署后拿到函数的 HTTP 触发器地址（形如 `https://xxx.cn-hangzhou.fcapp.run`）。

## 三、把 dws 接上

在跑 dws 的环境里（或由上层 agent 注入）：

```bash
export DWS_TELEMETRY_ENABLED=true
export DWS_TELEMETRY_URL="https://xxx.cn-hangzhou.fcapp.run"   # 上一步的函数地址
export DWS_TELEMETRY_TOKEN="<和 INGEST_TOKEN 相同的随机串>"
```

跑几条命令，到 SLS Logstore 查询页就能看到一条条记录。

## 四、本地先验证（可选，不依赖 FC / SLS）

最省事的本地验证用 `localsink.py`（纯标准库，零依赖），见
[telemetry.md 的「本地测试」](../../telemetry.md#本地测试零依赖不碰-sls)。

也可以直接本地跑本服务的 **dry-run 模式**（不配 SLS、不用装 aliyun-log）：

```bash
cd docs/telemetry/fc-sls-ingest
pip install flask                       # dry-run 只需 flask；aliyun-log 仅 SLS 模式才要
INGEST_TOKEN=dev python3 app.py         # 不配 SLS_* -> 自动 dry-run，监听 :9000
# 另开一个终端：
curl -s localhost:9000/                 # 应回显 mode=dry-run
curl -XPOST localhost:9000/ -H 'Authorization: Bearer dev' \
  -H 'Content-Type: application/json' \
  -d '{"schema_version":"1","command":"doc","outcome":"ok","duration_ms":42}'
# 返回 204；事件会以 DRYRUN {...} 打印在 app.py 的终端里。
```

要本地连真 SLS 验证，再补 `SLS_ENDPOINT/SLS_PROJECT/SLS_LOGSTORE` 和一组 AccessKey
（`pip install -r requirements.txt` 装上 aliyun-log），`GET /` 会变成 `mode=sls`。

## 五、配告警（SLS 控制台 → 告警）

| 告警 | 查询（示意） | 触发 |
|---|---|---|
| 错误率突增 | `* \| select count_if(outcome='error')*1.0/count(*) as err_rate` | err_rate > 0.05 |
| P99 延迟超标 | `* \| select approx_percentile(duration_ms, 0.99) as p99` | p99 > 3000 |
| 某命令大面积失败 | `* \| select command, count_if(outcome='error') c group by command order by c desc` | 单命令 c 突增 |
| 调用量跌零 | `* \| select count(*) as n` | n == 0（5 分钟窗口） |

通知渠道直接选钉钉机器人 webhook。

## 安全须知

- `INGEST_TOKEN` 用强随机串，并和 dws 侧保持一致；不要留空。
- 优先用 FC 服务角色（STS），不要把长期 AccessKey 写进环境变量。
- 本服务只接**匿名维度**数据，不含用户内容/身份——隐私边界由 dws 客户端保证。
