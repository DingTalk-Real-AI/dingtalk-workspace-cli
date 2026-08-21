# wiki 局部意图消歧

本文件从单 Skill `intent-guide.md` 拆分而来，仅保留与本产品相关的跨产品消歧规则。

| 用户说... | 真实意图 | 应该用 | 不要用 | 理由 |
|---|---|---|---|---|
| "帮我看看知识库里的文件" | 知识库节点列表 | `dws wiki +node-list --workspace <ID>` | `dws drive +list` | 明确“知识库”上下文，使用 Wiki 层级 |
| "浏览已知钉盘/文档空间目录" | 普通存储目录 | `dws drive +list` | `dws wiki +node-list` | 已有 space/folder 目标，文件层级属于 Drive |
| "列出/发现钉盘企业空间或我的文件空间" | Drive 存储空间发现 | `dws wiki space list --type orgSpace|mySpace`，取 spaceId/rootFolderId 后回 Drive | `dws wiki +space-list --type orgWikiSpace|myWikiSpace` | managed Wiki leaf 兼容存储空间发现；Drive ID 不能当 workspaceId |
| "列出组织知识库" | 列出组织知识库容器 | `dws wiki +space-list --type orgWikiSpace --page-all` | `dws drive +list` | 明确知识库容器，使用当前 Wiki 类型枚举 |
| "在知识库里搜方案" | 空间内搜索 | `dws wiki +node-search --workspace <ID> --query <词>` | `dws drive +search` | 指定知识库上下文，使用 Wiki 节点搜索 |
| "搜一下有没有叫XX的文件" | 全局搜索 | `dws drive +search --query <词>` | `dws wiki +node-search` | 未指定知识库，使用 Drive 全局聚合搜索 |
| "在知识库里创建一个文档" | 创建空文件实体 | `dws wiki +node-create --workspace <ID> --name <名称> --type adoc` | `dws doc create` | 空间内创建节点归 Wiki；正文写入才切 Doc |
| "整理一下XX项目的所有讨论" | 跨源主题归档 | #5 generate-topic-report | #4 write-doc | #4 侧重单篇文档创作；按主题跨听记/群消息汇总属于工作汇报 |
