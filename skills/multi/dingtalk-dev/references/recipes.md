# 端到端链路（recipes）

dev 的端到端任务都是「定位应用，改容器某节点，按审批需要走版本生效，最后回读验证」。每步先 `--dry-run` 确认再 `--yes`，参数查 `dws schema`，细节进对应 reference。

## 建一个钉钉里打开的网页应用

1. `dev app create --name <名>` 建应用，拿 unifiedAppId
2. `dev app webapp config` 配移动端/PC 首页（见 webapp.md）
3. `dev app version create` 建版本
4. `dev app version check-approval` 预检是否需审批
5. `dev app version publish` 发布（需审批时让用户选审批人）
6. 回读 `dev app version status` 到 `RELEASE` 才算生效

## 权限从申请到生效

1. `dev app permission list` 选 `scopeValue`（选择顺序见 permission.md）
2. `dev app permission add --scope-values <值>` 申请
3. 若是 `requiredApproval` 的权限，走版本：`version create`，再 `check-approval`，再 `publish --approver-user-id <用户选的>`，最后 `version status`
4. 免审权限直接开通，不必发版本

## 做一个答疑机器人并接到本地调试

1. `dev app create --name <名>` 创建应用，拿明确 `unifiedAppId`
2. `dev app robot config --unified-app-id <unifiedAppId>` 配置机器人能力；需要启用时再 `robot enable`
3. 线上使用闭环：`version create` → `check-approval` → `publish` → `version status`；`SELECT_APPROVER` 时必须等用户选择审批人，不默认取第一个
4. 本地调试/值守：`dev connect --unified-app-id <unifiedAppId>` 把机器人接到本地 agent（见 connect.md）；注意订阅事件前要先建联长连（见 event.md）
5. 若走无绑定的 `robot submit/result`，只有结果返回明确 `unifiedAppId` 才能继续版本发布
6. 完成态与缺 `unifiedAppId`、`SELECT_APPROVER` 等门禁判定见 [SKILL.md](../SKILL.md)「核心规则」：建联成功 + 版本进入 `RELEASE`/`AUDIT`/`UNDER_REVIEW` 才算完成

## 查「为什么没生效 / 机器人搜不到 / 权限加了还报错」

先 `dev app version status`——改配置不等于生效，未发到 `RELEASE` 就不生效。
