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
- 用户已明确要求时，dry-run 核对后用同一参数加 `--yes --format json`，随后只回读一次 `webapp get`；以回读的实际 URL 与 `h5PageType` 为准。
- 需要上线生效时继续走 [`version.md`](./version.md)。

已知路径不调用 group help；仅 flag 确有疑问时查一次精确 leaf compact Schema，Schema 不可用才查一次 leaf help。
