# DWS Doc 实验参数与命令名幻觉逐 case 观察

> 分析日期：2026-08-06
> 数据目录：`/Users/hyz/works/data/doc`
> 当前契约基线：`fix/param-hallucination@b94e2133`
> 说明：本报告是静态审计的实验附录。分类时先解析完整命令路径，再判断 flags；不能因为输出含 `unknown flag` 就直接认定为参数幻觉。

## 结论摘要

对 `/Users/hyz/works/data/doc` 下 6 组独立数据集逐 case 去重后，共审计 **388 个 case-run、1,644 次 `dws doc` 调用**。最终确认：

- 4 次调用存在当前仍未覆盖的参数名幻觉，共涉及 6 个错误 flag；
- 6 次调用使用了当前参数表已经能兜底的参数名；
- 7 次调用存在 shortcut 命令名幻觉，涉及 5 个不存在的 shortcut 名；
- 5 次调用使用不存在的普通路径 `doc fetch`；其中 4 次因为携带 flag，被父命令先报成 `unknown flag`，实际根因不是参数；
- 79 次调用使用实验基线当时不存在、但当前已经真实存在的命令，属于版本演进，不应配置 fallback；
- 其余主要是当前契约可接受调用，或认证、业务数据、值/约束等非参数名问题。

## 一、数据集与去重口径

| 数据集 | case-run | Doc 调用 |
|---|---:|---:|
| v2 mono | 65 | 277 |
| v2 multi | 65 | 258 |
| audit multi（精确 `000bc134`） | 65 | 260 |
| v3 mono | 64 | 264 |
| v3 multi | 64 | 300 |
| normalized trajectories | 65 | 285 |
| 合计 | 388 | 1,644 |

timeout rerun 已经并入各数据集最终结果，没有把 ZIP、Markdown、HTML 或报告副本重复计数。

## 二、总分类

| 分类 | 调用数 | 判断 |
|---|---:|---|
| 当前契约可接受 | 1,457 | 当前路径和参数可被真实 CLI 接受 |
| 版本演进：当前命令已存在 | 79 | 不是当前幻觉，不配 fallback |
| 非命令名/参数名错误 | 71 | 认证、网络、业务数据或其他错误 |
| 参数值或约束候选 | 15 | 需要独立业务语义复核，不计入参数名结论 |
| 当前参数别名已兜底 | 6 | 当前正式参数表已能归一 |
| 当前未覆盖参数名幻觉 | 4 | 本轮参数候选需要处理 |
| shortcut 命令名幻觉 | 7 | 2 条安全 rewrite、2 条 ambiguous、1 条跨层暂不支持 |
| 普通子命令路径幻觉 | 5 | 都是 `doc fetch`，现框架不能跨层 rewrite |

“参数值或约束候选”来自保守正则筛选，不能仅凭错误文本认定为参数名幻觉，因此本报告不把它们加入兜底数量。

## 三、已由当前参数表解决的 badcase

| 实验调用 | 次数 | 当前归一结果 | 验证 |
|---|---:|---|---|
| `doc +template-search --keyword ...` | 3 | `keyword → query` | 当前二进制 mock 回放成功 |
| `doc +doc-append --node ... --text ...` | 1 | `node → doc`，`text` 原生归入正文概念 | dry-run/payload 等价验证 |
| `doc +doc-append --node ... --content ...` | 1 | `node → doc`，`content → text` | dry-run/payload 等价验证 |
| `doc +comment-create --text ...` | 1 | `text → content` | dry-run/payload 等价验证 |

这 6 次不能再计为当前缺陷，说明现有参数概念层对高频跨 shortcut 名称迁移已经有效。

## 四、当前未覆盖的参数名幻觉

| Case | 实验调用 | 错误 flag | 正确契约 | 建议 |
|---|---|---|---|---|
| trajectories `dws_doc_0003` | `doc +inspect --include blocks` | `--include` | blocks 正文应使用 `doc +fetch`；inspect 没有通用 include | block，提示按 section 选择真实 flag/命令 |
| trajectories `dws_doc_0006` | `doc +inspect --include-info --include-versions` | `--include-info`、`--include-versions` | 基础 info 总是返回；历史 section 是 `--include-history` | block include-info；rewrite include-versions |
| trajectories `dws_doc_0013` | `doc +create --content-format markdown` | `--content-format` | `--doc-format markdown` | concept 自动映射 |
| trajectories `dws_doc_0025` | `doc +create --content-file PATH --content-format markdown` | `--content-file`、`--content-format` | `--content @relative-path`/stdin，或稳定 `doc create --content-file`；格式为 `--doc-format` | block file 参数；格式自动映射 |

4 次调用包含 6 个错误 flag。只有 `content-format` 和 `include-versions` 满足“同实体、同角色、原值透传”；另外 3 种 spellings 需要值变换、命令切换或本来就是冗余输入，必须 fail-closed。

## 五、shortcut 命令名幻觉

| 不存在的路径 | 次数 | 当前真实候选 | 处理结论 |
|---|---:|---|---|
| `doc +list-templates` | 1 | `doc +template-list` | 唯一只读语义，安全 rewrite |
| `doc +search-template` | 1 | `doc +template-search` | 动宾倒置，唯一只读语义，安全 rewrite |
| `doc +template` | 2 | `+template-list`、`+template-search`、`+create-from-template` | umbrella 名，ambiguous，停止执行 |
| `doc +version` | 2 | `+history-list`、`+history-save`、`+history-revert` | 包含读、写和高风险 revert，ambiguous，停止执行 |
| `doc +rename` | 1 | 普通稳定命令 `doc rename` | 现框架禁止 shortcut→非-shortcut 跨身份改写；暂不入表 |

候选 `command_path_fallbacks.json` 对这一批实验加入前 4 条：2 个 rewrite 和 2 个 ambiguous。fallback 只改变命令路径，不修改 flag/value；rewrite 后仍由真实目标命令完成参数和安全校验，ambiguous 在参数解析和 dispatch 前停止。

### 5.1 补充版本快照 badcase

后续补充 badcase 在“保存当前文档历史快照”的同一意图下，连续尝试了 `+create-version`、`+save-version`、`+snapshot`、`+export-pdf`、`+version-create`，最后才找到当时可用的 `+version-save`。按当前命令契约复核：

- `+create-version`、`+save-version`、`+snapshot`、`+version-create` 都表示保存当前版本快照，参数只需原样保留 `--node`，安全 rewrite 到当前 canonical `doc +history-save`；
- `+export-pdf` 表示另一类导出意图，而且需要补 `--export-format pdf`。当前 fallback 只允许改路径、不能注入 const 参数，因此保持 `unknown_shortcut`，不能错误改写到默认导出 docx 的 `+export`。

候选表据此再增加 4 条精确 rewrite，Doc 增量合计为 8 条。

隔离副本生成后逐条 mock 回放，四个名称都原样保留 `--node` 并实际进入 `save_doc_version`；`+export-pdf` 的负向回放仍返回 `unknown_shortcut`。这同时验证了兜底没有注入参数、没有把导出误当成历史版本保存。

## 六、`unknown flag` 掩盖了普通子命令错误

实验共出现 5 次 `doc fetch`。当前真实路径是 `doc +fetch`，普通路径 `doc fetch` 不存在：

- 4 次调用携带 `--node`、`--detail` 或输出 flag，父级 `doc` 先把它们报成 `unknown flag`；
- 1 次去掉 flag 后才明确显示 `unknown subcommand "fetch"`。

所以这 4 次是“子命令错误被 unknown flag 掩盖”，不是 4 个参数 hallucination。把 `node/detail` 塞进参数别名表既不能创建 `doc fetch`，还会污染正确命令。

现有命令 fallback 框架要求 source/target 保持相同 shortcut 身份，不能加入 `doc fetch → doc +fetch`；这是正确的安全边界。可选的未来方案是：

1. 为 `doc fetch` 声明真实 CLI alias/兼容叶子，并在命令身份层审核；或
2. 改进父命令错误解析顺序，在存在未识别 path token 时优先返回 unknown subcommand，并提示 `doc +fetch`。

## 七、版本演进项

79 次调用在实验 commit `000bc134` 不存在，但当前已经是真实命令：

| 当前真实路径 | 次数 |
|---|---:|
| `doc +fetch` | 38 |
| `doc +create-from-template` | 20 |
| `doc +create` | 8 |
| `doc +update` | 8 |
| `doc +inspect` | 2 |
| `doc +media-list` | 1 |
| `doc +history-save` | 1 |
| `doc +history-list` | 1 |

这些是 Schema/shortcut 覆盖扩展后的正常路径。为它们再配置 fallback 会与真实 Cobra 命令发生 source collision，因此必须排除。

## 八、建议同步的候选文件

- `docs/parameter-hallucination/doc/param_concepts.json`：完整正式表基线上的 Doc 参数候选；
- `docs/parameter-hallucination/doc/command_path_fallbacks.json`：完整正式表基线 + 8 条 Doc 命令名候选。

候选命令表不是只含 Doc 的增量文件；保留正式表的 Chat/OA 条目，可以在隔离副本执行完整生成和回归测试。正式同步时还需将 fallback 测试期望从 26 更新到 34，并补齐 8 条 Doc audit coverage。

## 九、风险与负面影响审查

- 不新增真实 CLI flag，不改变 Schema flag 权威；
- 不跨产品、不中途改变参数值，不读取文件，不查询 URL；
- 不把单值变多值，也不把版本、revision、comment、block、job/task ID 互换；
- 不自动选择带写入或高风险的 umbrella shortcut；
- rewrite 后目标命令的 required/enum/constraints/confirmation 继续生效；
- ambiguous/block 在执行前返回，不会触发业务调用；
- 不给已经真实存在的新 shortcut 配 fallback，避免碰撞和长期陈旧映射。
