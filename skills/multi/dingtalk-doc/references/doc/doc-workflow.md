# 核心工作流

## 核心工作流

> 步骤性指引。"读哪些文件"组合参见上方 §场景索引；命令详细参数参见对应子文件。

```bash
# ── 工作流 1: 定位并阅读文档 ──
dws drive search --query "<关键词>" --format json                 # 1. 搜索定位 nodeId（或 wiki node search）
dws doc info --node <DOC_ID> --format json                    # 2. 元信息（含 contentType）
dws doc read --node <DOC_ID> --format json                    # 3. 读 markdown 正文

# ── 工作流 2: 创建文档（含分片自动写入）──
dws drive folder create --name "项目资料" --format json           # 1. (可选) 创建文件夹
dws doc create --name "项目周报" --content-file /tmp/x.md \
  --folder <DOC_FOLDER_NODE_ID> --format json                 # 2. 创建 + 写入
dws doc read --node <DOC_ID> --format json                    # 3. 回读校验（必须）

# ── 工作流 3: 局部改写（保真，首选 JSONML）──
dws doc block list --node <DOC_ID> --content-format jsonml             # 1. 取 uuid
dws doc block list --node <DOC_ID> --content-format jsonml --block-id <UUID>   # 2. 读子树
dws doc block update --node <DOC_ID> --block-id <UUID> \
  --content-format jsonml --element '[...]'                            # 3. 写回（uuid 必须 == --block-id）
dws doc read --node <DOC_ID> --content-format jsonml                   # 4. 回读

# ── 工作流 4: 整篇 overwrite（仅在新骨架重写时）──
dws doc read --node <DOC_ID> --content-format jsonml --output /tmp/doc.json   # 1. 读出当前 JSONML
# 修改 /tmp/doc.json 中的 jsonml 数组
dws doc update --node <DOC_ID> --content-file /tmp/doc.json \
  --content-format jsonml --mode overwrite                             # 2. 写回（默认不做并发检查）
dws doc read --node <DOC_ID> --content-format jsonml                   # 3. 回读
# 担心被并发覆盖时，可加 --revision <N> 触发并发检查（详见 doc-update.md）

# ── 工作流 5: 上传独立文件 vs 插入附件到文档正文 ──
dws drive upload --file ./report.pdf --folder <ID>             # 上传作为独立文件（存储层）
dws doc media insert --node <DOC_ID> --file ./report.pdf       # 上传并作为附件块插入正文（内容层）

# ── 工作流 6: 导入本地文件为在线文档 ──
dws doc import --file ./report.docx --format json                  # 一体化导入（推荐）
dws doc import --file ./notes.md --folder <FOLDER_ID> --format json  # 指定目标文件夹
dws doc import --file ./data.xlsx --workspace <WS_ID> --format json  # 指定目标知识库

# ── 工作流 7: 下载 vs 导出（先 info 判 contentType）──
dws doc info --node <NODE_ID> --format json                    # 必须先查 contentType
dws doc export --node <NODE_ID> --output ~/downloads/          # contentType=ALIDOC 走 export（内容层）
dws drive download --node <NODE_ID> --output ~/downloads/      # contentType≠ALIDOC 走 drive download（存储层）

# ── 工作流 8: 评论 + 划词（@人 需 userId）──
dws contact user search --query "张三" --format json            # 取 userId
dws doc comment create --node <DOC_ID> --content "请确认" --mention <uid> --format json
dws doc block list --node <DOC_ID> --format json                # 划词需先取 blockId / paragraph.text
dws doc comment create-inline --node <DOC_ID> --block-id <BLOCK_ID> \
  --start 0 --end 10 --content "建议调整" --selected-text "原文" --format json

# ── 工作流 9: 权限授予（节点级，已迁移到 drive）──
dws contact user search --query "张三" --format json            # 取 userId
dws drive permission add --node <DOC_ID> --users <uid1>,<uid2> --role EDITOR --format json
dws drive permission list --node <DOC_ID> --format json        # 校验

# ── 工作流 10: 文件操作（已迁移到 drive）──
# 第一步：获取 nodeId
#   方式 A（优先）：用户直接提供 URL / nodeId → 直接传 --node
#   方式 B：按关键字找：dws drive search --query "项目周报" --format json
#   方式 C：按文件夹遍历：dws drive list --workspace <WS_ID> --format json
# 第二步：执行
dws drive copy   --node <DOC_ID_OR_URL> --folder <TARGET_FOLDER> --format json
dws drive move   --node <DOC_ID_OR_URL> --folder <TARGET_FOLDER> --format json
dws drive rename --node <DOC_ID_OR_URL> --name "新名称" --format json
dws drive delete --node <DOC_ID_OR_URL> --yes --format json
```

```bash
# ── 工作流 11: 文档版本管理 ──
dws doc version save --node <DOC_ID> --format json                 # 1. 手动保存版本快照
dws doc version list --node <DOC_ID> --format json                 # 2. 查看版本列表（提取 version 号）
dws doc version revert --node <DOC_ID> --version <N> --yes --format json  # 3. 回滚到指定版本（危险操作，需 --yes）
```
