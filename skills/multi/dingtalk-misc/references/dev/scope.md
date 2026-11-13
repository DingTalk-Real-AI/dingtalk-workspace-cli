# 应用可见范围（开放平台应用）

面向开放平台应用（企业内部应用/三方应用）的可见人员范围，不是钉盘、文档或知识库的分享范围。

## 查询

```bash
dws dev app scope get --unified-app-id <unifiedAppId> --format json
```

- 只读；返回部门/角色/人员等可见范围配置。

## 设置（全量替换语义）

```bash
# 第 1 步（可选）：本地调用预览，--dry-run 不发起任何远端调用，不是服务端预检
dws dev app scope set --unified-app-id <unifiedAppId> \
  --user-visible-scopes <scopeValue1,scopeValue2> \
  --precheck-only --dry-run --format json

# 第 2 步：服务端预检——真实调用，只生成计划不写入；经用户确认后执行
dws dev app scope set --unified-app-id <unifiedAppId> \
  --user-visible-scopes <scopeValue1,scopeValue2> \
  --precheck-only --yes --format json

# 第 3 步：核对计划（尤其将被移除的范围）并确认后，正式执行
dws dev app scope set --unified-app-id <unifiedAppId> \
  --user-visible-scopes <scopeValue1,scopeValue2> \
  --precheck-only=false --yes --format json
```

- `--dry-run` 是本地调用预览（不访问服务端）；服务端预检必须用 `--precheck-only --yes` 真实调用，返回计划与替换影响，不会写入。
- `scope set` 是**全量替换**：本次未传入的既有范围会被移除；增量添加前先 `scope get` 确认现状并在同一次调用带上完整集合。
- 范围列表支持 CSV 或 JSON 数组；`[]` 或 `,` 转换后为空会在本地直接拒绝。
- `--is-hidden` 控制应用可见性；显式传 `--is-hidden=false` 会被保留传递，不会丢失。
- 正式执行前必须完成服务端预检并取得用户对计划（含将被移除范围）的确认；参数变化需重新预检。
