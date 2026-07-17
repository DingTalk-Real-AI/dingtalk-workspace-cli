# drive 局部意图消歧

本文件从单 Skill `intent-guide.md` 拆分而来，仅保留与本产品相关的跨产品消歧规则。

| 用户说... | 真实意图 | 应该用 | 不要用 | 理由 |
|---|---|---|---|---|
| "参照这个生成同样的 / 按模板生成 / 复刻 X / 同样的模板 X 月份的" + 已有 alidocs URL | 模板保形生成同形态变体 | `drive copy + drive rename + doc block update` → 见 [模板保形生成](../../dingtalk-doc/references/04-document.md#模板保形生成) | `doc read + doc create`（重写链） | adoc → markdown 是有损投影，read+create 会丢行高/单元格背景色/字号；copy 在 adoc 层保形复制后只在副本上局部修改 |
| "读一下这个 xlsx 的数据" / xlsx 节点链接 | 下载本地表格文件 | `dws drive download --node` | `sheet range read` | xlsx / xls / xlsm / csv 是上传的本地文件（`contentType=DOCUMENT`），sheet 命令只支持在线表格，必须下载后本地解析 |
| "把这个在线表格导出为 xlsx 文件" | 在线表格格式转换 | `dws sheet export` | `dws drive download` | `export` 是 axls → xlsx 的导出转换；`download` 只能下载已有的 xlsx 节点 |
| "帮我把这个文件传到网盘" | 钉盘上传 | `drive upload` | — | 文件上传是存储层操作，归 drive |
| "上传文件到钉盘/我的文件" | 钉盘上传 | `drive upload` | — | 提到"钉盘/网盘/我的文件"→ drive |
| "上传文件"（未指定目标） | 默认钉盘 | `drive upload` | — | 未明确目标时默认上传到钉盘 |
| "帮我看看知识库里的文件" | 知识库节点列表 | `wiki node list --workspace` | `drive list` | 明确"知识库"上下文 → wiki node list |
| "列出钉盘团队空间" | 列出钉盘空间 | `wiki space list --type orgSpace` | `drive list-spaces` | 空间管理归 wiki，drive list-spaces 已 deprecated |
| "在知识库里搜方案" | 空间内搜索 | `wiki node search --workspace` | `drive search` | 指定了空间上下文 → wiki node search |
| "搜一下有没有叫XX的文件" | 全局搜索 | `drive search` | `wiki node search` | 未指定空间 → drive search 全局聚合搜索 |
| "整理一下XX项目的所有讨论" | 跨源主题归档 | #5 generate-topic-report | #4 write-doc | #4 侧重单篇文档创作；按主题跨听记/群消息汇总属于工作汇报 |
