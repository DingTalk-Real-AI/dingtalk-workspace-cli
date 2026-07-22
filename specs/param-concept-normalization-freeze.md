# 参数幻觉治理冻结清单

- 冻结名称：`param-hallucination-freeze-20260722`
- 基线：`origin/main` @ `076d77d`
- 工作分支：`fix/param-hallucination`
- 发布状态：仅本地冻结，不推送、不合并
- 规范入口：`specs/param-concept-normalization-spec.md`
- 既有评审：GitHub PR #744 已关闭，远端源分支保留

## 冻结目的

本检查点固定开源仓库中参数概念归一化的最终候选版本，作为后续迁移到内部 `dws-wukong` 仓库时的唯一代码基线。迁移时以本检查点的 tracked diff 为准，不从历史 CR、工作区草稿或评测报告反向拼装代码。

## 冻结范围

1. reviewed 参数概念源、closed schema、加载与校验。
2. 逐命令 alias / blocked / ambiguous 生成器及确定性生成表。
3. PreParse 中的语义归一化、保护态传播和冲突拦截。
4. 与原有格式归一、粘连拆分、拼写纠错链路的集成。
5. fixture、最终 payload、最终错误、dry-run 等测试与 policy 门禁。
6. 实现规范及本冻结清单。

## 明确不纳入

- `internal/helpers/calendar.go` 的试点改造：最终版本保持与 `origin/main` 一致。
- 未跟踪的参数字典草稿和工程汇报：它们是分析材料，不是运行时或 reviewed source。
- `DWS Skill 优化产出汇报-0721.md`：与本次代码冻结无关。
- 任何远端推送、PR/CR 或 `main` 合并。

## 迁移使用方式

1. 以本地 annotated tag `param-hallucination-freeze-20260722` 定位精确提交。
2. 使用 `origin/main...param-hallucination-freeze-20260722` 获取完整 tracked diff。
3. 在内部仓库重新绑定真实 Cobra 命令树并重新生成 alias 表，不直接假设生成物可跨仓库复制。
4. 重新执行本文件记录的验证项；内部仓库存在差异时，以 reviewed 源和测试语义为准适配。

## 冻结验证

冻结提交创建前的验证结果：

| 验证 | 结果 |
|---|---|
| `go generate ./internal/cli` | 通过；参数 alias、Agent metadata、Schema Catalog 均成功生成 |
| `./scripts/policy/check-generated-drift.sh` | 通过 |
| `./scripts/policy/check-param-concepts.sh` | 通过 |
| `./scripts/policy/check-param-alias-cooccurrence.sh` | 通过 |
| 参数归一化 focused tests | 通过；覆盖 fixture、读写 payload、保护错误、冲突、Calendar 原生兼容和选择性 dry-run |
| `go build -o /private/tmp/dws-param-freeze-build ./cmd` | 通过；显式输出路径用于避免二进制名 `cmd` 与源码目录重名 |
| `./scripts/policy/check-schema-catalog.sh` | 通过；22 products、572 tools |
| `DWS_PACKAGE_VERSION=0.0.0-test go test ./...` | 全部通过 |
