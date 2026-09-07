# PR #1296 性能优化进展

更新时间：2026-09-07。状态：Draft，单树实现已完成本地定向验证，等待 clean-head 两平台验收。

| 工作项 | 状态 | 当前证据 |
|---|---|---|
| 单二进制入口 | 已实现 | launcher/core/package manifest 已撤回，发布恢复单 `dws` |
| 单一完整 runtime tree | 已实现 | argv 产品选择、route index、插件资格分流与 selective tests 已删除；help/version/Schema/config/业务共享完整树 |
| root help | 已实现 | 删除独立 model/snapshot package，直接遍历 runtime Cobra tree；输出回归测试通过 |
| Schema verified cache | 已实现 | cache 只由正常 schema handler 使用；Cobra 前置 Schema fast path 已删除 |
| 完整树内存优化 | 本地门槛通过 | 相对父提交 `c0131d35`：16.43→12.68 MB/op（-22.8%），164.2k→148.1k alloc/op（-9.8%） |
| 完整树延迟 | 本地无实质回退 | clean commit 三轮中位数 13.50→13.54 ms/op（+0.3%）；正式看 Go 1.25.9 CI |
| telemetry 退出不等待 | 已实现 | 阻塞 collector 单测证明命令返回不等发送；明确接受末事件丢失 |
| Prepare/config/profile | 保留 | 未发现有收益且可安全删除的重复生命周期 |
| root help RSS / Lark | 待 clean-head CI | 旧 selective-head 的 2.5% 延迟与 RSS 数据已失效，不能用于当前单树结论 |
| Linux/Darwin full/race | 待 CI | 阻挡 Ready |

当前实现把架构复杂度从“双模式构树 + 多条快路径”收回到一棵完整树。局部 microbenchmark 已达到 RFC 的 allocation 门槛；端到端 Schema、help、业务命令与整体 RSS 必须在推送后的相同 head 重新测量。
