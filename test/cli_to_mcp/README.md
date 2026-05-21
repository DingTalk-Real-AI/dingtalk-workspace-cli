# DWS CLI ↔ MCP 端到端评测（cli_to_mcp）

对开源 `dws` v1.0.30 做端到端验证：用 pytest 调用真实 `dws ... --format json` 命令，汇总通过率。

> ⚠️ **不会在 CI 上自动跑**。这是一个**手动**评测套件，需要登录的钉钉账号 + 真实测试数据。本仓库 CI 不接入是因为：
> 1. 测试断言依赖真实租户数据（如"base 数量 ≥ 1"），CI 上无对应账号
> 2. 把 token 注入 CI secrets 会被开源 PR 反推泄露
> 3. 测试套件不是 mock 模式

如果你想跑 Mock 模式（不需要 token，CI 也能跑），用 [`test/skill_static`](../skill_static/) 和 [`test/skill_e2e`](../skill_e2e/) 那两个 Go test（build tag `skill_verify`）。

## 1. 前置准备

```bash
# 1. 构建 dws 二进制
go build -o /tmp/dws ./cmd
sudo cp /tmp/dws /usr/local/bin/dws

# 2. 登录钉钉账号
dws auth login
# 按提示扫码授权

# 3. 验证登录态
dws auth status --format json
# 看到 "token_valid": true 即可
```

如果上面任何一步没做，pytest 会**自动 skip 整个会话**（不报错）。

## 2. 运行全部产品

```bash
cd test/cli_to_mcp/testcases
python3 -m pytest -v
```

或者用产品 runner（带通过率汇总）：

```bash
python3 testcases/run_all_tests.py
```

## 3. 只跑单产品

```bash
python3 -m pytest -v test/cli_to_mcp/testcases/aitable/
```

## 4. 覆盖的产品（开源 dws v1.0.30）

aiapp · aisearch · aitable · attendance · calendar · chat · contact · devdoc · ding · doc · drive · live · mail · minutes · oa · report · sheet · todo · wiki

## 5. 来源

本套件从内部 `dws-wukong/auto-test/cli_to_mcp/` 精筛而来。
开源 dws 不包含的产品（`contract / edu-* / finance / headhunter / recruit / tb / workbench / unified-toolkit / conference / bot`）的测试用例**未照搬**。
