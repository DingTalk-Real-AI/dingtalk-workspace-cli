# lippi-open-dev-front 批量授权页改造交接稿

日期: 2026-06-16

## 当前状态

- 目标仓库: `http://gitlab.alibaba-inc.com/dingding/lippi-open-dev-front.git`
- 目标页面: `/src/app/personalAuthorization/index.jsx`
- 已通过 SSH clone 到: `/Users/xuan/lippi-open-dev-front`
- 前端实现分支: `codex/batch-auth-page-flow`
- 已在真实页面落代码，并通过 `npm ci` 与 `npm run build`；build 只有既有 asset size warning。
- 额外链路修复: `/Users/xuan/lippi-open-app-dev` 的 BFF 需要透传 `selectedScopes`，否则页面取消勾选会被 PAT core 兼容语义批准为全部 scopes。

## 链路目标

`dws auth login --recommend` 和所有 PAT 批量授权指令缺少 `--yes` 时，都进入同一条 Device Code Flow。
这里的“批量授权指令”限定为 `dws pat chmod` 的多 scope、`--product/--products/--domain/--domains` 产品线授权、`--recommend` 推荐授权；业务层 `batch-delete`、`batch-send`、AI 表格批量更新等仍走各自的业务确认或敏感操作逻辑，不进入 PAT 授权页。

1. CLI 调 `pat.batch_plan`，只拿服务端选出的 `selectedScopes`。
2. CLI 调 `pat.batch_grant`，带 `startFlow=true,noWait=true`。
3. PAT core 返回 `PAT_BATCH_AUTH_PENDING`，包含 `flowId/userCode/uri/authUrl`。
4. CLI 打开 `uri`，页面通过 `flowId/userCode` 拉取 flow 详情。
5. 页面展示所有 flow 中保存的 scopes，点击同意后调用 approve。
6. CLI 轮询到 `APPROVED` 后重试原授权动作。

这和 lark-cli 的 agent split-flow 保持同一类能力: lark-cli 有 `auth login --recommend`、`auth login --domain <domain>`、`auth login --scope <scope...>`、`auth login --exclude <scope>`、`auth login --no-wait --json` 返回授权 URL/设备码，再用 `--device-code` 完成；DWS 本地 CLI 场景可直接打开页面并轮询，JSON/host-owned 场景仍输出结构化 pending。

Ralph-cli 参考点不是页面能力，而是工程编排方式: 用 PRD 作为 SSOT，显式记录任务账本、验收项、阻塞证据和失败分析。本次 PRD 也采用这种结构，避免跨 CLI/GW/PAT/frontend 多仓改造时把“已实现的服务端链路”和“待拿权限落地的前端页面”混在一起。

## 服务端契约

### `pat.batch_grant` pending 返回

```json
{
  "success": false,
  "code": "PAT_BATCH_AUTH_PENDING",
  "message": "部分权限需要用户确认",
  "data": {
    "flowId": "flow-xxx",
    "userCode": "ABCD-EFGH",
    "uri": "https://...#/personalAuthorization?flowId=flow-xxx&userCode=ABCD-EFGH",
    "authUrl": "https://...#/personalAuthorization?flowId=flow-xxx&userCode=ABCD-EFGH",
    "pollIntervalSeconds": 3,
    "expiresInSeconds": 600
  }
}
```

### `queryFlowDetail`

页面只可信任 `flowId/userCode`，不要从 URL 或客户端参数里拼 `orgId/uid`。BFF/HSF 继续由登录态派生 `uid/orgId`。

```json
{
  "agentCode": "qoderwork",
  "appName": "DingtalkWorkSpace",
  "appIcon": "https://...",
  "userName": "小钉",
  "userAvatar": "https://...",
  "status": "PENDING",
  "batchFlow": true,
  "defaultGrantType": "permanent",
  "scopes": [
    {
      "scope": "calendar.event:read",
      "displayName": "读取日程",
      "productCode": "calendar",
      "productName": "日历",
      "riskLevel": 0,
      "effectiveRiskLevel": 0,
      "riskDesc": "低风险",
      "operationSummary": "读取你的日历数据"
    }
  ]
}
```

### `handleFlowAction`

approve 必须携带页面当前选中的 `selectedScopes`。旧前端不传该字段时，PAT core 兼容为批准 flow 中全部 scopes；新批量页必须显式传，保证 UI 勾选状态与授权真值一致。

```json
{
  "flowId": "flow-xxx",
  "userCode": "ABCD-EFGH",
  "action": "approve",
  "selectedScopes": [
    "calendar.event:read",
    "aitable.record:write"
  ]
}
```

拒绝:

```json
{
  "flowId": "flow-xxx",
  "userCode": "ABCD-EFGH",
  "action": "reject"
}
```

服务端规则:

- `selectedScopes == null`: 兼容旧前端，批准 flow 中服务端持久化的全部 scopes。
- `selectedScopes.isEmpty()`: 返回 `INVALID_PARAMETER`，前端应在 0 项时禁用“同意并授权”并引导用户拒绝。
- 非空: 必须全部属于 flow scopes，否则返回 `INVALID_PARAMETER`。
- approve/reject 完成后只清理当前 flow 拥有的 pending keys，避免误删同 uid/agent/scope 的其它 pending flow。
- partial approve 返回 `approvedScopes` 与 `rejectedScopes`；未选 scope 会进入 `rejectedScopes`，表示被本次用户操作取消。
- 如果 approve 写入阶段失败，PAT core 返回 `PAT_BATCH_PARTIAL_FAILURE` 并保持 flow 为 `PENDING`，前端展示错误并允许重试；once token 会按 flowId 归属清理，permanent/session grant 依赖事务与幂等逻辑吸收重试。

## 页面改造建议

### 1. 识别批量 flow

在旧 `personalAuthorization/index.jsx` 查询详情后:

```jsx
const scopes = Array.isArray(detail?.scopes) ? detail.scopes : [];
const isBatch = scopes.length > 1;
```

单 scope 保持旧页面。多 scope 进入批量布局。

### 2. scope 分组适配

```jsx
function groupScopes(scopes = []) {
  const map = new Map();
  scopes.forEach((item) => {
    const productCode = item.productCode || "unknown";
    const productName = item.productName || item.productCode || "其他权限";
    if (!map.has(productCode)) {
      map.set(productCode, {
        productCode,
        productName,
        summary: item.operationSummary || item.displayName || "",
        scopes: [],
      });
    }
    map.get(productCode).scopes.push(item);
  });
  return Array.from(map.values());
}
```

### 3. 批量权限组件

默认全选，允许取消产品组或单个 scope。至少选中 1 项才允许点击“同意并授权”；如果用户不想授权任何项，点击“拒绝”。

```jsx
function BatchScopeList({ scopes, checkedScopes, onCheckedScopesChange }) {
  const [expanded, setExpanded] = React.useState({});
  const groups = React.useMemo(() => groupScopes(scopes), [scopes]);
  const checkedSet = React.useMemo(() => new Set(checkedScopes), [checkedScopes]);

  const toggleScope = (scope, checked) => {
    const next = new Set(checkedSet);
    if (checked) {
      next.add(scope);
    } else {
      next.delete(scope);
    }
    onCheckedScopesChange(Array.from(next));
  };

  const toggleGroup = (group, checked) => {
    const next = new Set(checkedSet);
    group.scopes.forEach((item) => {
      if (checked) {
        next.add(item.scope);
      } else {
        next.delete(item.scope);
      }
    });
    onCheckedScopesChange(Array.from(next));
  };

  return (
    <div className="pa-scope-panel">
      <div className="pa-section-title">可授权的权限</div>
      {groups.map((group) => {
        const open = expanded[group.productCode];
        const groupScopes = group.scopes.map((item) => item.scope);
        const checkedCount = groupScopes.filter((scope) => checkedSet.has(scope)).length;
        const groupChecked = checkedCount === groupScopes.length;
        const groupIndeterminate = checkedCount > 0 && checkedCount < groupScopes.length;
        return (
          <div className="pa-product-row" key={group.productCode}>
            <div className="pa-product-main">
              <input
                className="pa-checkbox"
                type="checkbox"
                checked={groupChecked}
                ref={(node) => {
                  if (node) {
                    node.indeterminate = groupIndeterminate;
                  }
                }}
                onChange={(event) => toggleGroup(group, event.target.checked)}
                aria-label={`${group.productName} ${checkedCount}/${groupScopes.length} 已选择`}
              />
              <button
                type="button"
                className="pa-product-button"
                onClick={() => setExpanded((prev) => ({ ...prev, [group.productCode]: !open }))}
              >
                <span className="pa-product-name">{group.productName}</span>
                <span className="pa-product-count">{checkedCount}/{group.scopes.length} 项</span>
                <span className="pa-product-arrow" aria-hidden>{open ? "⌃" : "›"}</span>
              </button>
            </div>
            <div className="pa-product-desc">
              {group.summary || group.scopes.map((scope) => scope.displayName || scope.scope).slice(0, 2).join("、")}
            </div>
            {open && (
              <div className="pa-scope-detail-list">
                {group.scopes.map((scope) => (
                  <div className="pa-scope-detail" key={scope.scope}>
                    <input
                      className="pa-checkbox"
                      type="checkbox"
                      checked={checkedSet.has(scope.scope)}
                      onChange={(event) => toggleScope(scope.scope, event.target.checked)}
                      aria-label={`${scope.displayName || scope.scope} 已选择`}
                    />
                    <div className="pa-scope-copy">
                      <div className="pa-scope-name">{scope.displayName || scope.scope}</div>
                      <div className="pa-scope-code">{scope.scope}</div>
                      {scope.riskDesc && <div className="pa-scope-risk">{scope.riskDesc}</div>}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
```

如果目标项目有 Checkbox/Icon/Button 组件，应替换原生 input 和字符箭头，保持旧授权页和钉钉 Design Token 的视觉一致性，不要引入新视觉体系。

### 4. approve/reject 调用

复用旧页面已有的 action API，只改 payload:

```jsx
const [checkedScopes, setCheckedScopes] = React.useState([]);

React.useEffect(() => {
  setCheckedScopes(scopes.map((item) => item.scope).filter(Boolean));
}, [scopes]);

async function approve() {
  if (checkedScopes.length === 0) {
    return;
  }
  await handleFlowAction({
    flowId,
    userCode,
    action: "approve",
    selectedScopes: checkedScopes,
    // grantType 可不传，PAT core 会使用 flow 创建时持久化的 defaultGrantType。
    ...(grantType ? { grantType } : {}),
  });
  setStatus("APPROVED");
}

async function reject() {
  await handleFlowAction({ flowId, userCode, action: "reject" });
  setStatus("REJECTED");
}
```

不要把 `orgId/uid/agentCode` 作为可信入参传给 action。`selectedScopes` 只是用户选择结果，服务端必须校验它是 flow scopes 的子集。

### 5. 文案与状态

- 标题: `{appName || "DingtalkWorkSpace"}`
- 副标题: `请求以下钉钉账号进行授权`
- 列表标题: `可授权的权限`
- 主按钮: `同意并授权`
- 次按钮: `拒绝`
- 0 项选中: 主按钮禁用，辅助文案“请选择至少 1 项权限，或拒绝本次授权”。
- 空 scope: 展示“没有需要授权的权限”，主按钮禁用。
- `APPROVED`: 标题“授权已完成”，说明“可回到命令行继续”，禁用操作按钮。
- `REJECTED`: 标题“已拒绝授权”，说明“命令行将收到拒绝结果”，禁用操作按钮。
- `EXPIRED`: 标题“授权已过期”，说明“请回到命令行重新发起授权”，禁用操作按钮。
- `ERROR`: 展示错误说明，保留“重试加载”按钮；不要重复提交 approve。

### 6. 样式方向

保持截图中的居中白色授权卡片、8px 左右圆角、浅灰权限面板、DingTalk 蓝色主按钮。不要做营销页或额外 hero。页面首屏应直接是授权确认。

AI-native / 钉钉授权页约束:

- 首屏元素控制在 7 个以内: 应用图标、应用名、账号行、权限面板、辅助说明、拒绝按钮、同意按钮。
- 权限面板内部滚动，卡片最大高度建议 `min(720px, calc(100vh - 160px))`。
- 多产品默认只展示产品组摘要，明细折叠；展开不推动底部按钮离屏。
- 底部操作固定在卡片底部，移动端 375px 宽度下仍可直接拒绝/同意。
- 终态页不再展示可编辑 checkbox，避免用户误以为还能修改。

建议 CSS 类:

```css
.pa-scope-panel {
  margin-top: 12px;
  padding: 12px 16px;
  background: #f7f8fa;
  border-radius: 8px;
  max-height: min(420px, calc(100vh - 360px));
  overflow-y: auto;
}

.pa-product-row {
  padding: 12px 8px;
}

.pa-product-main {
  display: flex;
  align-items: center;
  gap: 12px;
}

.pa-product-button {
  flex: 1;
  display: flex;
  align-items: center;
  min-width: 0;
  border: 0;
  background: transparent;
  padding: 0;
  text-align: left;
  cursor: pointer;
}

.pa-product-name {
  font-weight: 600;
  color: #1f2329;
}

.pa-product-count {
  margin-left: 8px;
  color: #8f959e;
}

.pa-product-arrow {
  margin-left: auto;
  color: #8f959e;
}

.pa-product-desc,
.pa-scope-code,
.pa-scope-risk {
  color: #8f959e;
  font-size: 13px;
  line-height: 20px;
}

.pa-scope-detail-list {
  margin: 10px 0 0 32px;
  padding: 8px 12px;
  border-radius: 6px;
  background: #fff;
}

.pa-scope-detail + .pa-scope-detail {
  border-top: 1px solid #eff0f2;
}

.pa-scope-detail {
  display: flex;
  gap: 10px;
  padding: 8px 0;
}

.pa-scope-copy {
  min-width: 0;
}
```

### 7. 验收用例

1. 单 scope flow 仍渲染旧授权页。
2. 多 scope flow 按 `productCode/productName` 分组，默认全部勾选展示。
3. 点击产品组 checkbox 可整组选择/取消；点击单 scope checkbox 可单项选择/取消。
4. 点击产品行可展开/收起 scope 明细。
5. 选中数为 0 时“同意并授权”禁用。
6. 点击“同意并授权”发送 `flowId/userCode/action/selectedScopes`，不发送 `orgId/uid`；批量授权时长由 flow detail 的 `defaultGrantType` 展示并由 PAT core 兜底使用。
7. 服务端拒绝 flow 外 scope，前端展示错误且不进入成功终态。
8. 点击“拒绝”只发送 reject action。
9. `APPROVED/REJECTED/EXPIRED/ERROR` 终态不可重复提交。
10. 移动端 375px 宽度下产品名、数量、描述不溢出，底部按钮不离屏。
