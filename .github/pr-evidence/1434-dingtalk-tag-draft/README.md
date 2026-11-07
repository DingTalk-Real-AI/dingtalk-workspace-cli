# PR #1434 更新测试证据

被测代码：`e54df2c0dc6c53ef35f96ba5fc7f670f8066a324`。实际命令、时间及原始 JSONL 哈希见 `run.json`。

## Agent 测试报告：dingtalk、tag

4 个顶层测试、196 个子用例通过，0 失败、0 跳过。覆盖最终 Schema、Agent 选择声明、示例契约及实际 dry-run。

![Agent 测试报告 dingtalk tag](agent-report.png)

## 指令 CI 集成测试：deap

70 个顶层测试、233 个子用例通过，0 失败、0 跳过。覆盖新旧参数映射、静默兼容、Help、默认人设发布、空值校验、阶段失败与 Skill 更新边界。

![指令 CI 集成测试 deap](command-ci.png)

变更代码覆盖率门禁：changed code coverage: 100.0000% (62 executable statements; target 100.0000%)。该结果仅验证 changed-code gate，不代表全量或跨平台覆盖率已通过。

图片由真实本地测试事件渲染，不是 GitHub Actions 页面截图、真实模型或线上端到端验收。服务端尚未部署；完整远端 CI 以 PR 最新 Checks 为准。独立 CLI 进程此前在本机退出 137 的原因未查明，测试进程内验证不能替代独立进程验收。
