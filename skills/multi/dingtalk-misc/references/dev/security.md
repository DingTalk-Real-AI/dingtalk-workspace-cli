# 应用安全配置

安全配置只有一个写入口；不存在 `dws dev app security get`，不要试探或调用它。

```bash
dws dev app security config --unified-app-id <id> \
  --ip-whitelist <ip1,ip2> \
  --redirect-urls <url1,url2> \
  --sso-urls <url1,url2> \
  --dry-run --format json
```

- 至少提供一个配置字段；未提供的字段不变。
- 显式传入的列表是该字段整组覆盖，不是追加。若用户说“加上”且现值可从前序应用详情/配置结果获得，合并旧值后再提交；拿不到现值时说明覆盖风险，不编造旧值。
- 用户已明确要求时，dry-run 核对无误后把同一命令改为 `--yes --format json`。以 config 的结构化成功结果为写入证据；需要说明应用总体状态时回读一次 `app get`，不要发明 security get。
- 需要配置生效时继续走 [`version.md`](./version.md)。

已知路径直接执行；仅 flag 不确定时查一次 `dev app security config` compact Schema，必要时一次 leaf help。
