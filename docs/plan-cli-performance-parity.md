# PR #1296 性能专项执行计划

详细阶段见 [DWS 完整命令树性能专项 Plan](plan-command-framework-performance.md)。

## 合并前

- [x] 冻结单二进制、单进程、完整 runtime tree 的产品形态。
- [x] 撤回 launcher/core package、argv 产品选择和 root help projection。
- [x] 删除 Schema 的 Cobra 前置执行路径；cache 接入正常 schema handler。
- [x] 明确 telemetry no-wait 与末条分析事件可能丢失的取舍。
- [x] 完整树 B/op 和 allocs/op 达到本地父提交门槛。
- [x] 撤回 compile-time Schema identity 生产与发布封印；各端在安装/首次 schema 生成本机 identity 并写 cache。
- [x] 固定 PR-base main `6f71222b9b07c760cdb5f376b24dab9155e62094`。
- [ ] 完成 app/helpers/shortcut/corecmd/cli、packaging 与全仓测试。
- [ ] 完成 Darwin/arm64、Linux/amd64 race。
- [ ] 完成固定 main 的 default/opt-out 端到端对照（结论写入 RFC 附件；dump 不入库）。
- [ ] 完成 Schema、help、业务命令、整体 RSS 与 Lark/GWS 五维对照（结论写入 RFC 附件；dump 不入库）。
- [ ] 验证 root help RSS 相对父提交不回退，且与 Lark 的差值满足 RFC。
- [ ] 将 clean head 的 CI/RFC 证据链接写入性能附件和 PR 正文。

## 测量要求

每个场景至少 30 次随机交错；接近门槛时 100 次。候选、父提交和固定 main 使用相同 toolchain/target，绑定 exact commit 与 binary digest。首次调用、warm cache、default telemetry、DO_NOT_TRACK 分开。stdout/stderr 必须匹配 oracle，失败样本不能计作快速成功。

代表场景：完整树 microbenchmark、Schema leaf/overview/`--all`、root help、version、calendar leaf help、calendar dry-run/mock、config 和常用 get。public npm wrapper 的 RSS 包含 Node 与 native child。

## 保留或撤回规则

完整树优化须保持所有公开命令合同，并相对专项父提交至少降低 20% B/op 和 8% allocs/op。任何需要按 argv 构树、独立 help/Schema fast path 或第二份 command surface 的优化直接撤回。Prepare/config/profile 只有 profile 显示重复且删除后有可复现收益才修改。

Lark/GWS 说明下一步方向；固定 main 回归、正确性、Schema cache 合同和发布证明共同决定 Ready。
