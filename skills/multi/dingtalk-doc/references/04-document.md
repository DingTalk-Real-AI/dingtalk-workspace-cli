# 文档组合任务

本页只描述跨步骤组合任务。单一创建、读取、更新、导出或媒体操作直接使用根 Skill Golden Route，不要加载本页。

## 创建成稿

1. 把最终正文写到工作目录内的 `body.md` 或 `body.json`。
2. 执行一次 `dws doc +create --name "<标题>" --content @body.md --format json`。
3. 使用返回的 `verified/nodeId` 判断完成；只有用户明确要求额外结构验收时再做局部 `+fetch`。

复杂排版才按需读取 style/JSONML Reference；执行入口仍使用 `+create`。禁止调用已删除的创建脚本。

## 导入文件

```bash
dws doc +import --file ./report.docx --folder <FOLDER_ID> --format json
```

导入是服务端格式转换。禁止先读文件内容再走 `create + update`。单纯保存普通文件切换到 `dingtalk-drive` 上传。

## 查找并读取

```bash
dws doc +fetch --query "<唯一标题>" --format json
```

`+fetch --query` 会跨页解析唯一在线文字文档。零命中、多候选或类型不是 `adoc` 时停止并按返回候选消歧；不要继续调用 drive/wiki/aisearch 做无界穷举。

## 更新章节

1. `+fetch --node <ID> --scope section|keyword` 读取最小必要上下文。
2. 普通编辑用 `+update`；重要覆盖用 `+checkpoint-update`。
3. 使用 shortcut 的验证结果；`partial_success/unknown` 时只恢复缺失步骤。

## 从模板创建

1. `+template-search --query <名称>`。
2. 唯一命中才取得 `templateId`；多候选要求用户选择。
3. `+create-from-template --template-id <ID>` 只执行一次。

模板保形复制已有文档是另一种任务：使用 drive copy 复制源文档，再只修改副本。禁止 `doc read → doc create` 重建富格式模板。

## 导出并归档

1. 在线文字文档使用 `+export` 导出到工作目录。
2. 检查 `localPath/sizeBytes`。
3. 用户要求归档到钉盘时，再用 `dingtalk-drive` 上传该文件。

导出失败不得安装本地转换依赖或直接下载临时 URL。

## 文档转消息/待办

1. 使用局部 `+fetch` 提取必要内容。
2. 在目标产品中解析真实用户/群/任务标识。
3. 根据目标产品 Runtime gate 确认后写入。

跨产品步骤必须复用稳定 ID；失败后不能重放已经成功的文档写入步骤。
