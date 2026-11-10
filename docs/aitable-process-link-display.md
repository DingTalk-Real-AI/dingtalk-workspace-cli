# AI 表格过程链接展示与最终交付

## 范围与承载面

本说明约定 AI 表格业务回执的链接保留、阶段展示和最终交付。实现复用既有命令与 MCP 调用，补齐 Base 列表／搜索的导航字段透传，不增加权限或业务写入。

- [Go 指引](../internal/helpers/aitable_navigation.go)由命令声明进入 Help 和 Schema。
- [Mono 主 Skill](../skills/mono/SKILL.md)发布入口规则；[Mono 产品参考](../skills/mono/references/products/aitable.md)和[Multi Skill](../skills/multi/dingtalk-aitable/SKILL.md)保持公共产品段一致。
- 服务端摘要、CLI 指引和实际加载的 Skill 分别验证，不能假设每条 CLI 回执都会保留 MCP 顶层 summary。统一结果路径提取业务 data；bootstrap 的步骤回执另有原文保留测试。

## 阶段展示与最终交付

收到业务回执后，对用户要求处理且有当前有效、角色匹配入口的对象，逐对象展示具名完整链接。不同对象不合并为一条 Base 入口，不以父级入口替代已有目标深链；过程内已展示的同对象同有效入口可去重。内部自动对象不自动成为展示或交付目标，用户明确要求的对象也不因自动创建而被排除。

入口使用当前对象的 url、身份匹配的 links，或已确认可用的 shareFormUrl。按对象类型和对象 ID 判断链接身份，不按 relation 字面含义判断：身份匹配的 links 即使 relation=parent 仍是当前对象入口，只有链接身份确属父级时才称父级入口。仅有父级入口时须明确说明，不冒充目标直达。

最终回复前，按用户请求、已确认范围和当前状态清点最终目标，按宿主规则逐目标完成登记与呈现，并核对正文链接，不以父级入口合并替代。成功回执、relation=target 或过程对象不自动成为最终产物。过程已展示链接或宿主已呈现卡片，最终 Markdown 正文仍须保留最终目标的有效具名完整链接（`[对象名称](完整URL)`）；过程去重不免除此要求。缺少适用入口时说明缺口。普通链接不替代登记；用户明确指定其他格式时按其要求。

宿主须同步替换“正文不重复卡片 URL”规则；保留正文链接不触发重复登记。个人宿主配置须另行核对，不随 DWS 仓库发布。

业务核验、登记和展示状态分别判断。业务回执或展示请求提交不证明卡片已登记、已渲染或面板已打开；成功登记不重复，未知登记不自动重发。

## URL、名称与状态

业务 URL 的参数顺序、嵌套编码及百分号编码大小写须逐字符保留；%3D 与 %3d 即使访问等价也不可互换。逐对象使用本条回执的完整原始 URL，不解码后重编码、不套用其他对象 URL 替换 ID、不截断或补全。上传签名 URL、token 等凭据按既有规则脱敏。

仅当存在明确识别为 JSON 格式的原始业务回执文件时，展示前使用本地 JSON 解析器从该文件提取待展示 URL 并严格比较。CSV、XLS、XLSX 等导入输入附件不是 JSON 业务回执；没有上述 JSON 回执文件时，直接使用当前命令的结构化回执核对，不因非 JSON 附件阻断成功结果展示，也不伪造核对源文件。

名称须对应同项 URL 的对象。缺名使用对象类型或已知任务标签，仅在仍无法区分时附 ID，不编造名称。独立读回优先，当前显式 null／空值不由旧回执覆盖。

保留 pending、unknown、partial、STOP 等原状态；链接不证明业务成功、定位、权限、分享或流程启用。填写者只使用已确认可用的填写页；向管理者交付管理页时注明“管理页，非填写入口”。分享未知或入口未就绪时说明缺口，不为补链接重放写入、开启分享或扩大权限。

交付给填写者且填写入口未就绪时，只说明缺口，不展示或登记管理页链接；即使注明非填写入口，也不将管理页列为可交付内容。只有用户明确要求管理操作或接收者为管理者时，才交付当前可用的管理页。

## Base 列表／搜索

六个读入口分别在 Help 和 full／compact Schema 发布同一条件回退规则：

| 原子命令 | 别名 | Shortcut |
| --- | --- | --- |
| aitable base list | aitable list | aitable +base-list |
| aitable base search | aitable search | aitable +base-search |

+base-list 与 +base-search 的投影归一化 baseId/baseName，并按 Base 原样保留已返回的 url/links，包括显式 null、空值和 links 扩展字段。缺失字段保持缺失；CLI 不生成回退 URL，也不把另一 Base 的链接补给当前对象。

仅当本次结果未返回目标 url/links 时，消费者才可按命令明确规则，使用本次真实 baseId 构造 https://alidocs.dingtalk.com/i/nodes/{baseId}，将 baseId 作为路径段编码一次。已有身份匹配链接时优先使用原值，不按模板重建。该许可不适用于表单或其他对象，也不证明权限、分享、定位或写入状态。

## 回归与验证边界

- Help／Schema 回归核对共享指引的写入口，以及六个 Base 读入口的条件回退；写入口实际执行 `--help`，与 full／compact Schema 一并检查最终正文链接要求，拒绝旧的正文省略规则。对象身份判断同时覆盖 Go 指引和 Skill。
- 列表／搜索命令路径回归覆盖仅 url、仅 links、两者都有、缺失、显式 null 和空值，并检查同页不同 Base 的隔离及分页证据。
- bootstrap 使用手写缩略回执 fixture 检查步骤 summary、同一 Base 下不同表的导航及 URL 编码。该 fixture 只证明模拟传输路径，不证明服务端生产文案或部署状态。
- 导入回归覆盖 CSV、XLS、XLSX 不透明字节上传、无 Content-Type 头、两步调用序列及结构化回执的导航保留；工作簿内容解析由模拟服务的边界承担，不据此声称已验证真实解析。
- 导入导航断言先检查 links 的类型、长度及首项结构；异常时输出完整的合成业务回执诊断，避免索引或类型断言 panic。
- 配套 Agent 场景分别验证对象自身 URL、仅 links 和真实缺链回退，沿用现有 Judge 判分；CLI loopback 精确核对输出原值，不宣称具有通用的 URL 字面评分约束。
- 最终正文链接另做 Mono／Multi 定向验收：只检查最后一轮的具名原始链接，覆盖卡片已呈现、多目标含内部对象及填写入口未就绪。合成回执不代替真实宿主呈现验收。

从仓库根目录执行：

```bash
DWS_PACKAGE_VERSION=0.0.0-test go test -race -count=1 ./internal/shortcut/aitable ./internal/aitablenavigation
DWS_PACKAGE_VERSION=0.0.0-test go test -count=1 ./internal/helpers ./internal/app -run 'TestCrossPlatformCoverage.*Navigation'
./scripts/policy/check-mono-multi-skill-content.sh
./scripts/policy/check-skill-context-budget.sh
./scripts/policy/check-skill-commands.sh
make generate-schema
./scripts/policy/check-generated-drift.sh
./scripts/policy/check-schema-catalog.sh
make build
git diff --check
```

本地 Mock、loopback、契约与内容测试不代表真实 Agent 判分、线上 MCP 状态或宿主呈现验收。验收时应记录服务端、CLI、Skill 与宿主的实际版本及缓存状态，核对实际加载的规则和实际可见的链接；不得用历史结果、业务成功或展示请求提交替代当前环境的登记与呈现证据。
