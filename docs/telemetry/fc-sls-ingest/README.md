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

## 二、部署本服务为 FC Web 函数

1. 函数计算控制台 → 创建函数 → **Web 函数** → Python 运行时。
2. 上传本目录代码（含 `requirements.txt`，FC 会自动装依赖）。
3. **启动命令**填：`gunicorn -b 0.0.0.0:9000 app:app`，**监听端口** `9000`。
4. 给函数**绑定一个服务角色**，授权 `AliyunLogFullAccess`（或更小的 PutLogs 权限）。
   这样就不用把 AccessKey 写进环境变量——FC 会自动注入 STS 临时凭证，`app.py` 已优先读它。
5. 配 **环境变量**：

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

## 四、本地先验证（可选，不依赖 FC）

```bash
cd docs/telemetry/fc-sls-ingest
python3 -m venv .venv && . .venv/bin/activate
pip install -r requirements.txt
export SLS_ENDPOINT=... SLS_PROJECT=... SLS_LOGSTORE=... INGEST_TOKEN=dev
export ALIBABA_CLOUD_ACCESS_KEY_ID=... ALIBABA_CLOUD_ACCESS_KEY_SECRET=...
python app.py            # 监听 :9000
# 另开一个终端：
curl -XPOST localhost:9000/ -H 'Authorization: Bearer dev' \
  -H 'Content-Type: application/json' \
  -d '{"schema_version":"1","command":"doc","outcome":"ok","duration_ms":42}'
# 返回 204 即写入成功；去 SLS 控制台查 dws-telemetry。
```

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
