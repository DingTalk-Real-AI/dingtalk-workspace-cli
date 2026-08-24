# Profile CLI 参数幻觉复核（2026-08-21）

## 结论

Profile 已完成参数面复核，但**不需要新增中央 concept/override**。独立候选等价于最新线上正式表：
62 个 concept、349 个 command override、675 条 fixture。保留该候选是为了明确记录“已审、零新增”，
不是漏做。

## 判断依据

- `profile switch/use` 的组织选择已经由 Cobra 原生接受 `--corpId`，并原生兼容隐藏的
  `--corp-id`、`--corpid`、`--corp`；中央表不得重复声明真实 flag。
- `profile switch <selector|->` 的主选择器是位置参数，不属于参数 alias 表。
- `corpId:userId`、组织名、用户名和本地 profile 名之间存在唯一性与多账号约束，不能靠 argv
  同义词推断或自动选第一项。
- 全局 `--profile` 是一次性身份选择器，不应与 Agoal 的 `--profile-id` 或人员 ID concept 合并。

## 验证

独立候选 fresh generate 与嵌入式 PreParse fixture 通过；相对正式 latest-main 生成行为差异为 0。
分析依据：最新 Cobra/Help、`dingtalk-misc/references/profile.md`、生成器和 Runtime 选择逻辑。
