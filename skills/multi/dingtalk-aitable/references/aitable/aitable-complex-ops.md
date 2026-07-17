# 复杂操作

## 复杂操作

### 仪表盘 / 图表（建议顺序）

```bash
# 1) 先看配置模板（JSONC）
dws aitable dashboard config-example --format json
dws aitable chart widgets-example --format json

# 2) 先拿 dashboard，再拿 chart 详情
dws aitable dashboard get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --format json
dws aitable chart get --base-id <BASE_ID> --dashboard-id <DASHBOARD_ID> --chart-id <CHART_ID> --format json
```

要点：

- `dashboard get` 返回的 `charts[].chartId` 可直接给 `chart get` 使用。
- `dashboard share get` 可能返回 `404`（资源不存在或未开通），需按可重试错误处理，不要误判为参数拼错。
- `chart share get` 可正常返回 `enabled/shareUrl`，用于分享状态判断。

### 导出数据（两阶段轮询）

`export data` 常见为异步任务：首次调用可能只返回 `taskId`，需要继续轮询。

```bash
# 第一步：创建任务（业务格式与输出格式分开）
dws aitable export data --base-id <BASE_ID> --scope table --table-id <TABLE_ID> --export-format excel --timeout-ms 1000 --format json

# 第二步：拿 taskId 继续轮询，直到返回 downloadUrl
dws aitable export data --base-id <BASE_ID> --task-id <TASK_ID> --timeout-ms 3000 --format json
```

参数约束

- 创建任务统一需要 `base-id + scope + export-format`
- `scope=all`：无额外 ID
- `scope=table`：必须 `table-id`
- `scope=view`：必须同时 `table-id + view-id`
