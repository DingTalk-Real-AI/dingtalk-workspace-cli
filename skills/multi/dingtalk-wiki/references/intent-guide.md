# wiki 局部意图消歧

本文件从单 Skill `intent-guide.md` 拆分而来，仅保留与本产品相关的跨产品消歧规则。

| 用户说... | 真实意图 | 应该用 | 不要用 | 理由 |
|---|---|---|---|---|
| "帮我看看知识库里的文件" | 知识库节点列表 | `wiki node list --workspace` | `drive list` | 明确"知识库"上下文 → wiki node list |
| "列出钉盘团队空间" | 列出钉盘空间 | `wiki space list --type orgSpace` | `drive list-spaces` | 空间管理归 wiki，drive list-spaces 已 deprecated |
| "在知识库里搜方案" | 空间内搜索 | `wiki node search --workspace` | `drive search` | 指定了空间上下文 → wiki node search |
| "搜一下有没有叫XX的文件" | 全局搜索 | `drive search` | `wiki node search` | 未指定空间 → drive search 全局聚合搜索 |
| "在知识库里创建一个文档" | 创建空文件实体 | `wiki node create --type adoc` | `doc create` | 空间内创建节点归 wiki；doc create 是向已有文档写入内容，不是创建文件节点 |
| "整理一下XX项目的所有讨论" | 跨源主题归档 | #5 generate-topic-report | #4 write-doc | #4 侧重单篇文档创作；按主题跨听记/群消息汇总属于工作汇报 |
