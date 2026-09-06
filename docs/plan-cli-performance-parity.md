# PR #1296 性能专项执行计划

后续框架专项见 [DWS 命令框架性能专项 Plan](plan-command-framework-performance.md)：先修正工厂选择，再开展 completion/I/O 审计、同源路由索引、插件发现延迟与 Prepare 去重证明。该文列出的 `mountLegacyPublicCommandsFor` 先全量构建问题属于当前 PR 的 P0。

## 合并前

- [x] 冻结单二进制产品形态并撤回 launcher/core package。
- [x] 明确 telemetry no-wait 与末条事件丢失取舍。
- [x] 接入保守的产品按需装配，保留完整树回退。
- [x] 候选和正式发布共用 Schema identity linker contract。
- [x] 固定 PR-base main `6f71222b9b07c760cdb5f376b24dab9155e62094`。
- [ ] 完成 app/helpers/shortcut/cmd、Schema component、packaging 与全仓测试。
- [ ] 完成两平台 race 和 native candidate proof。
- [ ] 生成固定 main 的 default/opt-out 端到端报告。
- [ ] 生成 Schema、help、业务命令、整体 RSS 与 Lark/GWS 五维报告。
- [ ] 将新 head 的证据链接写入性能附件和 PR 正文。

## 测量要求

每个场景至少 30 次交错样本；候选和基线绑定 exact commit/binary digest。首次调用、warm cache、default telemetry、DO_NOT_TRACK 分开。stdout/stderr 必须逐字节稳定，失败样本不能计为快速成功。

代表场景：Schema leaf/overview/`--all`、root help、version、calendar leaf help、calendar list dry-run、calendar mock、config list，以及一个常用 get。public npm wrapper 的 RSS 包含 Node 与 native 子进程。

## 保留或撤回优化的规则

按需装配须达到 RFC 的 70%/10% 构树门槛并通过回退等价测试，否则撤回。Prepare/config/profile 只有 profile 显示重复且删除后有可复现收益才修改；几 ms 以下且增加生命周期风险的变化不合入。

Lark/GWS 数据只判断下一步性能方向。当前 PR 的 Ready 由固定 main 回归、正确性和发布合同决定。
