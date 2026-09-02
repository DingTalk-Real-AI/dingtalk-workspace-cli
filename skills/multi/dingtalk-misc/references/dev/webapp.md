# 网页应用配置

```bash
dws dev app webapp get --unified-app-id <id> --format json
dws dev app webapp config --unified-app-id <id> \
  --homepage-url <移动端主页> \
  --pc-homepage-url <PC端主页> \
  --omp-url <管理后台地址> \
  --h5-page-type <类型> \
  --dry-run --format json
```

- `config` 至少传一个需要修改的字段，未要求的字段不要猜。
- 以 `get` 实际返回的 `data.configured` 和 URL 字段为准；`configured=false` 或 URL 缺失表示尚未配置，不是命令错误。不要依赖旧版“空对象 `{}`”判断。
- 最初请求只允许 dry-run。展示预检返回的应用 ID、完整 URL/页面类型参数及影响并取得用户对该预览的明确确认后，才把同一命令仅由 `--dry-run` 换成 `--yes`；参数变化就重新预检并确认，确认前不得真实配置。随后只回读一次 `webapp get`，以实际 URL 与 `h5PageType` 为准。
- 需要上线生效时继续走 [`version.md`](./version.md)。

已知路径不调用 group help；仅 flag 确有疑问时查一次精确 leaf compact Schema，Schema 不可用才查一次 leaf help。
