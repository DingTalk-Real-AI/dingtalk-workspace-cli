# 读取文档：`+fetch` Golden Route

## 唯一推荐入口

```bash
dws doc +fetch --node <DOC_ID_OR_URL> --format json
dws doc +fetch --query "项目周报" --scope keyword --keyword "结论" --format json
dws doc +fetch --node <PUBLIC_URL> --password <ACCESS_PASSWORD> --format json
dws doc +fetch --node <DOC_ID> --version <VERSION> --format json
```

- 已知 ID 或 URL：传 `--node`。
- 只知道标题：传 `--query`；跨页解析必须唯一命中，否则停止并要求用户选择。
- `--node` 与 `--query` 必须且只能提供一个。
- 默认 `--detail simple --scope full`，适合普通阅读，避免加载不必要的 JSONML。

## 互联网公开文档与历史版本

- **互联网公开文档**：公开链接直接传 `--node`；文档开启了密码保护时，通过 `--password <ACCESS_PASSWORD>` 提供访问密码，普通文档无需传入。密码只进入读取请求，不会回显在读取结果里（`--dry-run` 预览会包含所传参数，注意输出环境）。
- **历史版本**：`--version <N>` 读取指定历史版本内容；版本号从 `dws doc +version-list` 获取，`0` 表示文档初始版本，需要文档编辑权限（EDITOR 及以上）；缺省读最新版。注意区分：`revision` 是文档编辑版本号（JSONML 读取响应返回、供 `+update --expected-revision` 条件写使用），不是历史版本号，`+fetch` 不支持 `--revision`。
- **跨组织文档**：非互联网公开的跨组织文档，`+fetch` 整体被组织边界拦截（提示「不支持跨组织访问数据」），最新版与历史版本内容都读不到；互联网公开（含密码）文档的 `+fetch` 不受影响，但 `+version-list` 与写操作仍会被拦截，版本号无法从列表获取。
- 只要看历史内容、不打算恢复时用 `--version`；要把文档整体恢复到历史版本才用 `+version-revert`（危险操作，需确认）。

## 局部读取

```bash
dws doc +fetch --node <DOC_ID> --scope outline --detail with-ids --format json
dws doc +fetch --node <DOC_ID> --scope section --start-block-id <BLOCK_ID> --detail full --format json
dws doc +fetch --node <DOC_ID> --scope range --start-block-id <A> --end-block-id <B> --detail full --format json
dws doc +fetch --node <DOC_ID> --scope tags --tags table,img --detail full --format json
dws doc +fetch --node <DOC_ID> --scope keyword --keyword "风险|结论" --context-before 120 --context-after 240 --format json
```

只有需要块 ID、revision 或 JSONML 保真结构时才提高 `--detail`；先读取最小必要范围，避免把整篇大文档放入上下文。

整篇读取使用默认 scope 或 `--scope full`；`full` 不是关键词，禁止写成 `--keyword full`。

## 最小读取漏斗

按用户已经给出的线索选最短路径，不先拉全文：

1. 已知稳定 ID/URL，且任务确实涉及整篇：直接默认 `full + simple`，一次返回 Markdown。
2. 用户给出具体术语、错误码或同义词：直接 `keyword`；`foo|bar` 是 OR，`context-before/after` 是字符数。该模式会缩小 Agent 输出与 token，但当前 Runtime 仍需取得正文后做本地投影，不把它误述为减少上游传输。
3. 用户指向某章但没有 block ID：先 `outline --detail with-ids`，再用返回的真实标题 ID 执行 `section`。通常两次小结果比反复处理整篇更稳定。
4. 已知起止 block：直接 `range`；只找表格、图片等结构时用 `tags`。
5. 只有确需全文总结、全局一致性检查或整篇保真改写时，才读取完整内容。

`simple` 用于阅读；`with-ids` 用于定位下一次写操作；`full` 只用于必须保留样式、引用或 JSONML 结构的编辑。局部结果只能证明所选范围，不得据此声称已检查整篇。

## 后续路由

- 读后修改：把稳定的 `nodeId` 交给 [`doc-update.md`](doc-update.md) 的 `+update` 或 `+checkpoint-update`。
- 附件/图片：先用 [`doc-media.md`](doc-media.md) 的 `+media-list` 取得稳定 `resourceId`，再下载；不要复用正文中的临时签名 URL。
- 富结构专家编辑：确实需要原始 JSONML 或 shortcut 未公开的参数时，先读取精确 leaf Schema，再使用原子 `doc read`/`doc block`。

禁止把原子 `doc read` 当作默认入口，也不要在读取后无条件整篇回写。筛选结果只用于读取，不能把虚拟 fragment 容器整体写回文档；同一次任务里已拿到稳定 `nodeId` 后，后续调用直接复用，避免再次按标题搜索。
