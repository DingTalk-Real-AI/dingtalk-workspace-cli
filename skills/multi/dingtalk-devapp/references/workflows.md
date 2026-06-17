# 操作流程

端到端操作流程，组合多个命令完成完整业务场景。

## 创建应用并完成基础配置

```text
1. 创建应用
   dws devapp create --name "应用名" --desc "描述" --type internal --dry-run --format json
   → 确认后加 --yes
   → 记录返回的 unifiedAppId 和 appKey

2. 确认创建成功
   dws devapp get --unified-app-id <unifiedAppId> --format json

3. 读取凭证（获取 appKey/appSecret）
   dws devapp credentials get --unified-app-id <unifiedAppId> --format json

4. 配置网页应用（按需）
   dws devapp webapp config --unified-app-id <unifiedAppId> --homepage-url <URL> --dry-run --format json
   → 确认后加 --yes

5. 验证网页应用配置
   dws devapp webapp get --unified-app-id <unifiedAppId> --format json
   → 确认返回 homepageUrl/pcHomepageUrl 等

6. 申请权限（按需）
   dws devapp permission list --unified-app-id <unifiedAppId> --keyword "关键词" --format json
   → 从返回中选择 scopeValue
   dws devapp permission add --unified-app-id <unifiedAppId> --permissions <scopeValue> --dry-run --format json
   → 确认后加 --yes

7. 添加开发成员（按需）
   dws devapp member add --unified-app-id <unifiedAppId> --users <userId> --member-type DEVELOPER --dry-run --format json
   → 确认后加 --yes

8. 验证成员
   dws devapp member list --unified-app-id <unifiedAppId> --format json

9. 配置安全设置（按需）
   dws devapp security config --unified-app-id <unifiedAppId> --ip-whitelist <IP> --redirect-url <URL> --dry-run --format json
   → 确认后加 --yes
```

## 权限管理全流程

```text
1. 查看所有权限
   dws devapp permission list --unified-app-id <unifiedAppId> --format json

2. 按关键词搜索未开通权限
   dws devapp permission list --unified-app-id <unifiedAppId> --keyword "机器人" --status UNAUTHED --format json

3. 查看某个权限覆盖的 API
   dws devapp permission list --unified-app-id <unifiedAppId> --scope <scopeValue> --format json

4. 申请权限
   dws devapp permission add --unified-app-id <unifiedAppId> --permissions <scopeValue> --dry-run --format json
   → 确认后加 --yes
   → requiredApproval=true 的权限：写入版本变更，需后续走版本发布审核

5. 验证
   dws devapp permission list --unified-app-id <unifiedAppId> --scope <scopeValue> --format json
   → 确认 authed=true

6. 取消不需要的权限
   dws devapp permission remove --unified-app-id <unifiedAppId> --permissions <scopeValue> --dry-run --format json
   → 确认后加 --yes
```

## 应用生命周期

```text
停用（保留数据，暂停服务）:
  dws devapp get --unified-app-id <unifiedAppId> --format json     ← 确认目标
  dws devapp inactive --unified-app-id <unifiedAppId> --dry-run --format json
  → 确认后加 --yes
  dws devapp get --unified-app-id <unifiedAppId> --format json     ← 验证状态

启用（恢复已停用应用）:
  dws devapp active --unified-app-id <unifiedAppId> --dry-run --format json
  → 确认后加 --yes
  dws devapp get --unified-app-id <unifiedAppId> --format json     ← 验证状态

删除（不可逆，异步生效）:
  dws devapp get --unified-app-id <unifiedAppId> --format json     ← 展示摘要
  dws devapp delete --unified-app-id <unifiedAppId> --dry-run --format json
  → 用户明确确认后加 --yes
  dws devapp list --format json                                     ← 验证已消失
```

## 修改应用信息

```text
1. 定位应用（若只有名称）
   dws devapp list --name "应用名" --format json
   → 必须唯一命中

2. 修改
   dws devapp update --unified-app-id <unifiedAppId> --name "新名" --desc "新描述" --dry-run --format json
   → 确认后加 --yes

3. 验证
   dws devapp get --unified-app-id <unifiedAppId> --format json
```

## 成员管理

```text
1. 查看当前成员
   dws devapp member list --unified-app-id <unifiedAppId> --format json

2. 添加成员
   dws devapp member add --unified-app-id <unifiedAppId> --users <userId1,userId2> --member-type DEVELOPER --dry-run --format json
   → 确认后加 --yes

3. 验证
   dws devapp member list --unified-app-id <unifiedAppId> --format json

4. 移除成员
   dws devapp member remove --unified-app-id <unifiedAppId> --users <userId> --member-type DEVELOPER --dry-run --format json
   → 确认后加 --yes

5. 验证
   dws devapp member list --unified-app-id <unifiedAppId> --format json
```

## 安全配置

```text
1. 配置 IP 白名单 / 重定向 URL / 免登 URL
   dws devapp security config --unified-app-id <unifiedAppId> \
     --ip-whitelist 192.0.2.10,192.0.2.11 \
     --redirect-url https://callback.example.invalid/callback \
     --sso-url https://sso.example.invalid/sso \
     --dry-run --format json
   → 确认后加 --yes

注意：只下发显式提供的字段，未提供的不覆盖。
```

## 版本发布与审批（选审批人）

```text
1. 创建版本（权限变更等已通过 permission add 写入版本）
   dws devapp version create --unified-app-id <unifiedAppId> --version 1.0.1 --desc "变更说明" --yes --format json
   → 记录返回的 versionId

2. 审批预检：获取审批要求和候选审批人列表
   dws devapp version check-approval --unified-app-id <unifiedAppId> --version-id <versionId> --format json
   → 服务端 precheckOnly 预检，不实际发布
   → 返回中包含是否需要审批、候选审批人（userId + 姓名）

3. 把候选审批人列表展示给用户，让用户选择
   ⚠️ 必须由用户拍板选哪个审批人，agent 不要自行挑选或默认取第一个

4. 用选中的审批人发起发布审批
   dws devapp version publish --unified-app-id <unifiedAppId> --version-id <versionId> --approver <用户选中的userId> --yes --format json
   → 含高敏权限时需追加 --confirm-sensitive
   → 审批流程会自动推送给该审批人

5. 跟踪审批/发布状态
   dws devapp version status --unified-app-id <unifiedAppId> --version-id <versionId> --format json
   → 审批通过后应用才正式发布；机器人等能力需发布后才可被搜索/加群/路由消息
```

## 通用规则

所有操作遵循：

```text
意图消歧 → 定位唯一应用 → dry-run 预览 → 用户确认 → --yes 执行 → 透传结果
```

- 多应用命中时展示候选，不自动选择
- `ServiceResult.success=false` 原样透传 `errorCode/errorMsg`
- 待实现的 event 命令遇到时报告功能待上线
