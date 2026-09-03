# 导出在线文档

## 唯一推荐入口

```bash
dws doc +export --node <DOC_ID_OR_URL> --export-format docx --output ./exports/ --format json
dws doc +export --node <DOC_ID_OR_URL> --export-format markdown --output ./document.md --format json
dws doc +export --node <DOC_ID_OR_URL> --export-format pdf --output ./document.pdf --format json
```

`+export` 一次完成提交、轮询和原子下载。输出路径必须位于工作目录内，默认 no-clobber；目标已存在时返回 `LOCAL_FILE_EXISTS`，更换 `--output` 路径重试，没有覆盖用 flag。

`--export-format` 必填；全局 `--format json` 只控制 CLI 输出，不能代替业务导出格式。禁止依赖默认 docx，也不要猜测 `--type`。

异步状态 `INIT` 与 `PROCESSING` 都表示任务仍可继续轮询；只有终态失败才停止，不能把 `INIT` 当导出失败。

## 类型边界

- 在线文字文档（`adoc`）转为 docx/markdown/pdf：`+export`。
- 已存在的普通文件原样下载：切换到 `dingtalk-drive`，使用 drive download。
- 不要为了导出先把正文读到本地再重新生成文件。

## 失败处理

- 提交前权限/认证失败：原样报告并停止；不要尝试同义底层命令。
- 已返回 `jobId` 后轮询失败或超时：保留 `jobId`，只用 `+export-get --job-id <JOB_ID>` 恢复查询，禁止重新提交导出。
- 下载阶段失败：保留 `jobId` 和目标相对路径，用 `+export-get --job-id <JOB_ID> --output ./目标文件` 通过同一安全下载器恢复；不要直接 `curl` 临时 URL。
- 禁止安装 `pandoc`、`python-docx` 或其他依赖来隐式伪造导出结果。只有用户明确改成“本地生成文件”任务时，才可作为一个新的独立工作流处理。

只有需要显式接管异步 job 的恢复场景才使用 `+export-submit/+export-get`；正常导出不得手工编排它们。
