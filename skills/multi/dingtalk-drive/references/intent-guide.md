# Drive 局部意图消歧

| 用户表达 | 应用 | 不应用 | 理由 |
|---|---|---|---|
| 全局找文件、最近文件、浏览已知“我的文件”/文档空间目录 | Drive | Wiki node | 未限定知识库 workspace，属于普通存储域 |
| 列出/发现钉盘企业空间或“我的文件”空间 | managed `dws wiki space list --type orgSpace|mySpace` 发现 spaceId/rootFolderId 后回 Drive | `wiki +space-list` 的 orgWikiSpace/myWikiSpace | Drive 存储空间前置；spaceId、rootFolderId 与知识库 workspaceId 不可互换 |
| 明确知识库内列节点、移动节点、搜索节点 | Wiki | Drive | workspace 内层级由 Wiki 管理 |
| 普通文件或文件夹移动、重命名、删除 | Drive | Doc | 节点存储动作不修改正文 |
| 普通文件创建独立副本 | Drive download→upload | Drive `+copy` | 当前 `+copy` 会拒绝普通钉盘文件，避免把 `.dlink` 快捷方式伪装成副本 |
| adoc 正文读取、编辑或导出 | Doc | Drive download | Drive 只管理节点；在线文档需要格式转换 |
| 本地 xlsx/xls/csv 节点下载后分析 | Drive + 本地工具 | Sheet range read | 上传的普通文件不是 axls 在线表格 |
| axls 在线表格导出为 xlsx | Sheet export | Drive download | export 执行格式转换 |
| able 记录、字段、视图 | AITable | Drive | 表内业务数据不属于存储节点动作 |
| able 仅复制结构、删除 Base | AITable `+base-copy --base-id <ID> --target-folder-id <真实ID> --only-struct` / `+base-delete --base-id <ID>` | Drive copy/delete | 当前 main 要求真实目标文件夹 ID；缺少时停止，禁止发明 `--target-root`、完整复制后逐表删数据或用 Drive 猜根 ID |
| Base 内 Table/Dashboard/Section 节点操作 | AITable `+table-*` / `+section-*` | Drive | nsheet 节点不是独立 Drive dentry |
| 整个 Base 移到普通文件夹、外层存储重命名 | Drive | AITable 表内命令 | 这是 Base 外层存储位置/名称动作 |
| PDF、DOCX、XLSX 等普通文件的评论查询或生命周期操作 | Drive `comment list-v2/create-v2/reply/update/delete/batch-query/list-replies/resolve/restore/react-reply` | Doc/Sheet comment | 默认进入新评论体系；普通文件固定使用文件级 global topic，不支持划词或单元格锚点；仅在用户明确要求旧评论兼容行为时使用 deprecated 的 `list/create` |
| Word/Markdown/Text 转在线文档 | Doc `+import` | Drive upload | import 会创建在线文档；upload 只保留原文件 |
| “上传文件”但未指定目标 | Drive `+upload` | — | 默认按普通文件上传到钉盘 |
| “照这个文档做一份同样格式的” | Drive `+copy` + `+rename`，再由 Doc 局部更新副本 | 读取后重建 | 先复制可保留在线文档版式 |

URL 本身不能证明产品类型。当前 Drive shortcut 的公开参数主要接受 ID；只有 URL 时先用 `dws drive info --node <URL> --format json` 预检并取真实 nodeId/nodeType。
