# Agent 测试报告 — event / PR #1402

测试代码 SHA：`9deacb29692d5d7f54f429c5cc784e35d7133435`。后续提交仅补充本目录测试证据。

- 9 个顶层测试、149 个子用例通过；七类事件分别通过实际 CLI Schema 检查。
- Agent 路由检查是确定性的声明边界测试；示例契约检查不等于线上模型评测，报告中的 dry_run 数量仅是示例分类，并未运行这些示例的 dry-run。
- OA 可选字段回归使用合成事件，70 组输入各验证 flatten / 非 flatten 两条路径；其他子用例验证订阅参数、dry-run、复用等行为。
- 未触发真实审批，也未声称验证线上服务端消息类型。

## 复现

```sh
DWS_PACKAGE_VERSION=0.0.0-test go test -json -count=1 ./internal/app -run '^(TestCrossPlatformCoverageEventAgentSelectionBoundaries|TestAgentExamplesContract|TestPersonalOA.*|TestCrossPlatformCoveragePersonalOAOptionalBusinessFieldsOutput)$'
./dws event schema user_oa_approval_instance_cc --flatten -f json
```

全部七次 CLI 命令及字段结果见 `cli-schema-checks.json`。完整测试事件流见 `agent-tests.jsonl`，易读日志见 `agent-tests.log`，运行元数据见 `run.json`。

`agent-report.png` 是 Chrome 对 `agent-report.html` 的真实页面截图；HTML 的计数和日志摘录来自上述本次运行记录。
