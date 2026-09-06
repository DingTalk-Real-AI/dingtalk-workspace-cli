# PR #1296 性能优化进展

更新时间：2026-09-06。状态：Draft，实施与验收中。

| 工作项 | 状态 | 当前证据 |
|---|---|---|
| 单二进制入口 | 已实现 | launcher/core/package manifest 代码撤回；GoReleaser、npm、Homebrew、install/upgrade 恢复单 `dws` |
| telemetry 退出不等待 | 已实现 | 阻塞 collector 单测证明命令返回不等发送；明确接受末事件丢失 |
| Schema verified cache | 已实现，Darwin clean-head 通过 | candidate builder 和 release linker contract 绑定同一 identity；cache user CPU p50 相对 live 降 97.6% |
| 产品按需装配 | 已实现，局部测试通过 | 修复先全量后筛选后，calendar 构树约 14.2→0.85 ms，alloc 169k→10.1k；config 约 0.36 ms |
| completion / 路由索引 | 已实现并审计 | 补全请求完整树；无 DWS 全局 completion callback；shortcut service 约 2.10 µs→7.4 ns |
| 插件/未知输入回退 | 已实现，聚焦测试通过 | clean path 证明无插件后跳过重复 loader；动态面、异常状态和未知输入完整回退 |
| Prepare/config/profile 去重 | 已证明保留 | 单 profile 解析一次、dry-run 零次；没有额外 PrepareCommandTree，不删除 auth/Safety 生命周期 |
| 固定 main 端到端 | Darwin clean-head 本地通过，待 CI | head `4a25ec8b...` 对固定基准 `6f71222b...`；12/12 入口 gate 和 44 场景五维矩阵通过 |
| Lark/GWS 新 head 对比 | Darwin clean-head 已运行 | wall 延迟五个可比场景快于 Lark、仍慢于 GWS；完整 Lark 诊断 36/40，help RSS 四项未过且样本数未达正式门槛 |
| Linux/Darwin full/race | 待 CI | 阻挡 Ready |

当前 clean-head 数据确认了 calendar 构树相对完整树约 94% 的时间下降、七个代表场景相对固定 main 全部提速，以及 telemetry default/opt-out 基本重合。Darwin 本地候选的 Schema、default-entry、五维报告均通过完整性检查；最终结论仍以同一 head 的两平台 CI artifact 为准。
