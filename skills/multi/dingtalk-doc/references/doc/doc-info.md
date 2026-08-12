# 查看文档信息：`+inspect` Golden Route

```bash
dws doc +inspect --node <DOC_ID_OR_URL> --format json
dws doc +inspect --node <DOC_ID_OR_URL> --include-permissions --include-history --format json
```

只打开任务需要的 `--include-style/--include-permissions/--include-history/--include-media/--include-comments`，不要默认全取。正文读取使用 `+fetch`，不要用信息查询替代正文接口。

检查返回的真实 `nodeId`、类型、URL 和各可选步骤。若 URL 指向的不是在线文字文档，停止 doc 流程并切换到 drive、sheet、aitable、slides 或 wiki；禁止凭 URL 外形猜类型。

只有 `+inspect` 未公开所需的底层字段或必须获得原始响应时，才读取精确 leaf Schema 后使用原子信息命令。
