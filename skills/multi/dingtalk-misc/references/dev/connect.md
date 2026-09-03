# 本地机器人连接器

`dws dev connect` 只管理本地 Stream 调试/值守，不创建应用、不发布版本。状态类任务只看 DWS 返回，不扫描 Docker、pm2、launchctl、系统进程、源码目录或 `~/.dws`。

## 状态、重启、停止

```bash
dws dev connect list --json --format json
dws dev connect status --robot-client-id <list返回的clientId> --json --format json
dws dev connect restart --robot-client-id <list返回的clientId> --dry-run --format json
dws dev connect stop --robot-client-id <list返回的clientId> --dry-run --format json
```

执行顺序：

1. 先执行一次 `connect list --json`。按 `appName`、明确的 `unifiedAppId` 或 `clientId` 唯一选择记录；名称缺失时才用一次 `app list --name` 取得 `unifiedAppId` 后与记录匹配。`connect list` 本身不接受定位 flag。
2. 管理现有记录时优先把该记录的 `clientId` 传给 `--robot-client-id`；只有记录没有 clientId 时才用它自己的 `unifiedAppId`。当前实现的守护进程目录优先按 clientId 建立，把 AppKey/clientId 塞进 `--unified-app-id` 会误报 `not_running`。
3. 只对唯一目标做一次 `status --json`。用户说“如果 down 就重启”时，仅当状态为 `down/degraded` 才先 dry-run、再执行一次 restart，随后 status 回读一次；`healthy` 时不重启。
4. 用户要求停止时，仅当实例明确存在才先 dry-run、再执行 stop，随后 status 回读一次。
5. restart/stop 支持 `--dry-run` 且正式执行不需要 `--yes`；status 不使用 dry-run。实例不存在时直接说明，不替用户新建。不存在 `dws dev app connect ...`。

## 新建连接

```bash
# 先预检渠道、凭证来源与本地 CLI 依赖
dws dev connect --unified-app-id <id> --channel <channel> --dry-run --format json

# 用户明确要后台值守且预检通过时启动 daemon
dws dev connect --daemon --unified-app-id <id> --channel <channel> --format json
```

前台 connect 是长驻进程，不要在评测/对话 shell 里阻塞运行；需要可管理实例时用 daemon。daemon 启动后重新执行 `list --json`，用新记录的 `clientId` 做 status/restart/stop 定位。dry-run 的 `completionState=LOCAL_DEBUG_ONLY`、`doesNotPublish=true` 表示只完成本地调试准备。凭证优先由 `unifiedAppId` 自动取得，密钥不落盘、不回显；缺本地渠道 CLI 时按 dry-run 的 `installed/autoInstall/installHint` 报告，未经用户要求不扩展成系统安装排障。

同一 restart/connect 错误且记录、参数、状态没变化时停止，不重复 help、环境探测或重启。已知路径直接执行；仅当前 leaf flag 确有疑问时查一次 compact Schema，Schema 不可用才查一次精确 leaf help。
