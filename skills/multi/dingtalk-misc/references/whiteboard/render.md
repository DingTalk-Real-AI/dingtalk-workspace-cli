# OpenNodes V1 创建前 SVG 预渲染

`dws whiteboard render` 是纯本地语义预览。它读取与创建相同的 OpenNodes source，写出
确定性 SVG，不访问任何图片、字体或外链资源，也不调用白板 MCP。

这是本地文件写入操作，不是只读命令。先确认 `--output` 目标和写入授权，再执行；
非交互环境取得授权后才可追加 `--yes`。覆盖已有文件还须明确授权覆盖并添加 `--force`，
`--force` 本身不代替确认。命令不支持 `--dry-run`，传入时会拒绝执行且不写文件。
本地文件写入授权不代表用户已审阅 SVG，也不授权创建远端白板；后者仍按下述流程单独确认。

预渲染前校验 run.text 中的非法换行（\n、\r、U+2028、U+2029），包括 Frame 标题。
报错会指出节点 ID 和字段路径；将多行拆为多个 paragraph block，空行用
`{"type":"paragraph","runs":[{"text":""}]}`，保留对齐及 marks/link 后重新渲染和确认。
此校验不是完整服务端 Schema 验证；能生成 SVG 不等于服务端一定接受。

Agent 使用 OpenNodes 带内容创建时必须先走此流程，不能因用户未要求预览而跳过。
渲染失败、命令不可用或无法展示产物时停止并报告阻塞，不直接创建。

```bash
dws whiteboard render --source @whiteboard.json \
  --output ./whiteboard-preview.svg --format json
```

结果中的 `artifactPath` 交给 Agent UI 展示；同时展示 `disclaimer`、`fidelity` 和全部
`warnings`。`exact` 表示存在确定映射，`approximate` 表示字体、图形或路径为近似，
`placeholder` 表示至少一个节点只能显示虚线占位框。它们都不是像素级一致承诺。

**生成 SVG 后必须暂停并等待用户回复。** 展示 SVG、fidelity 和全部 warnings 后，询问是否按当前预览创建或需要修改，并结束本轮写入流程；不得在同一轮自动创建。最初的“编写课表/创建白板”只授权准备方案，不代表确认尚未看过的 SVG。Agent 自检通过、sourceDigest 匹配或用户暂未回复都不算确认。

用户要求修改时，更新 source、重新 render、展示新版，再次等待明确确认；旧版确认失效。确认针对当前版本，不能通过跳过摘要、换创建入口或自行添加 --yes 绕过。

用户明确确认当前预览后，才可添加 --yes，并必须使用原 source 和摘要创建：

```bash
dws whiteboard create-with-content --name "<白板名称>" \
  --source ./whiteboard.json --request-id <STABLE_REQUEST_ID> \
  --expected-source-digest <RENDER_SOURCE_DIGEST> --format json
```

摘要不一致说明预览后 source 已变化；停止创建并重新 render、展示和确认。图片、Vector、
未知节点以及不安全 Path 会显示占位，不得因 warnings 为空之外的任何原因声称最终效果
与钉钉白板完全一致。`--force` 只用于明确原子替换已有本地 SVG；默认禁止覆盖。
