# SQL Sheet API Key 管理

`dws aitable api-key ...` 管理指定 Base 的 SQL Sheet 访问凭据，使用当前登录用户的身份。

## 命令

| 意图 | 命令 | 关键参数 |
|---|---|---|
| 创建访问凭据 | `api-key create` | `--base-id`；确认后再追加 `--yes` |
| 查询有效凭据 | `api-key list` | `--base-id` |
| 撤销指定凭据 | `api-key revoke` | `--base-id --key-id`；确认后再追加 `--yes` |

三个命令均要求 Base 的管理权限（MANAGER），每次选择一个 profile。使用 `--dry-run` 预览请求；预览不调用远端服务。

## 返回结果与标识

- create 的 `data` 包含 `keyId`、`status=ACTIVE`、`createdAt` 和完整 `apiKey`。`apiKey` 仅在创建成功时返回一次，按用户指定的方式交付或保存，日志和后续上下文只保留 `keyId`。
- list 的 `data.keys` 包含 0 或 1 个有效凭据的元数据：`keyId`、`status=ACTIVE`、`createdAt`，以及可选的 `createdBy={userId,corpId}`。`createdAt` 为 Unix 毫秒时间戳。
- revoke 的 `keyId` 使用同一 Base 的 create/list 真实返回，格式为小写规范 UUID。成功返回 `data.keyId` 和 `data.revoked=true`；指定凭据不存在或已经撤销时同样成功。

## 创建与撤销

- 每个 Base 最多一个有效凭据。已有凭据时，create 返回 `SQL_SHEET_API_KEY_ALREADY_EXISTS`，使用 list 核对当前状态。
- 三个命令都不自动重试。创建超时、断网或结果未知时先 list 核对，再由用户决定后续操作；一次空列表不足以判断在途创建已失败。
- list 只查询元数据，无法恢复完整 `apiKey`。需要替换凭据时，先确认旧 `keyId` 和撤销影响，撤销后再按用户授权创建新凭据。
