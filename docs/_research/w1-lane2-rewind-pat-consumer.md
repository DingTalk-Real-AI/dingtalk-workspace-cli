---

# 《Rewind 客户端侧 PAT 消费方案报告》 — 研究团队 lane #2

## 0. 扫描结论先行

| 能力 | 承载点 | 谁来做 |
|---|---|---|
| PAT 获取 & 续期 | `dingtalk_core/login/cli_auth.rs` | dws CLI 本地 token.json（非 keychain） |
| PAT 权限错误解析 | `spark_loop/adapter/dws_pat_permission.rs` | 按 stderr JSON `code`/`error_code` 解三级风险 |
| 高敏异步授权路由 | `spark_loop/adapter/dws_pat_high_risk_registry.rs` | authRequestId → oneshot |
| dws 作为子进程的契约 | `spark_session_handler.rs::execute_dws_chmod_if_allowed` + `real_tools_adapter.rs::maybe_intercept_dws_pat_permission` | argv（非 env）+ exit_code 契约 |
| 身份/组织透传 | token 文件绑定 uid+org_id；subprocess 仅注入 `REWIND_*` trace | `cli_auth.rs` + `sandbox_exec.rs` |
| a2a 弹窗回执 | LWP `getCliAuthCode` + `/s/synca` objectType=1050000 | `spark_session_handler.rs` |

---

## 1. 文件清单与职责矩阵

| 文件 (行数) | 职责 | 关键接口 |
|---|---|---|
| `adapter/dws_pat_permission.rs` (406) | 解析 dws exit_code=4 时的 stderr JSON，产出 `DwsPatPermissionRequest`（低/中/高敏） | `parse_dws_pat_permission()` (行 111) |
| `adapter/dws_pat_high_risk_registry.rs` (117) | 高敏 authRequestId→oneshot 注册表，桥接前端按钮 & 同步协议推送 | `register`/`resolve`/`unregister` (行 33/57/45) |
| `adapter/workspace_compat.rs` (1261) | Spark Loop 专用 shell/read/write 工具输出 normalize + 授权范围外路径二次重试线索抽取（**不调 dws**） | `extract_outside_scope_shell_retry_paths` (行 423)，`normalize_spark_shell_payload` (行 313) |
| `adapter/workspace_compat_shell_retry_tests.rs` (49) | 上项的测试 | — |
| `adapter/real_tools_adapter.rs` (5842) | Tool 执行入口；shell 工具 exit_code=4 时拦截并发起 PAT 弹窗；retry/注入 authFailed | `maybe_intercept_dws_pat_permission` (行 892) |
| `adapter/spark_permission_registry.rs` (204) | Spark loop 授权答案注册表（call_id→AGUI msgId、conversation_id→预存 option/DWS action） | `register`/`take`/`pre_answer`/`pre_dws_action` |
| `adapter/spark_permission_registry_tests.rs` (136) | 上项的测试 | — |
| `dingtalk_core/login/manager.rs` (3392) | `LoginManager`，挂接 `CliAuthManager`；登出时清理 | `ensure_dws_auth_ready` (行 2389)，`cli_auth_manager()` (行 2025) |
| `dingtalk_core/login/global_llm.rs` (1157) | LLM credential 管理（DEAP/Normal/Auto），提供 `resolve_current_org_id` | `GlobalLlmManager` (行 62) |
| `dingtalk_core/organization_context/mod.rs` (2645) | 组织/身份 catalog、选择、持久化 | `OrganizationSelection`、`OrganizationIdentityOption` (行 41/142) |
| `dingtalk_core/mod.rs` (21) | 模块出口 | `pub use login::{LoginManager, ...}` |
| `dingtalk_core/models.rs` (35) | 文件上传 DTO（与 PAT 无直接关系） | — |
| `dingtalk_core/user_identity.rs` (569) | LWP 用户身份缓存（12h 刷新），cache_key=`org:<id>` 或 `corp:<id>` | `UserIdentityManager` (行 38)，`cache_key_for_request` (行 291) |

关联非扫描文件（客户端 PAT 链路必读）：

- `dingtalk_core/login/cli_auth.rs` — **PAT 生命周期的真实主体**（manager.rs 仅持有 handle）
- `dingtalk_core/login/device_service.rs:40-71` — `agent_unique_id = md5(open_id + corp_id + device_id)`
- `agent/session/session_handler.rs:41-52,72-129` — `SessionHandler` trait + `DwsChmodOutcome` 三态
- `agent/runtime/spark_loop/adapter/spark_session_handler.rs:1468-1844` — 中敏 `dws pat chmod` 执行 & 高敏异步等待
- `agent/runtime/sandbox_exec.rs:229-242` — 子进程追踪 env 注入约定
- `base/environment/dingtalk_cli.rs:8-20,110-112` — dws 二进制定位常量
- `agent/runtime/dws_detection.rs:11-72` — dws 调用识别（direct / shell-wrapped）
- `agent/runtime/agent_runtime/approval_adapter.rs:30,985-1066` — `DWS_ORG_UNAUTHORIZED` 阻断码

---

## 2. 端到端时序（登录 → 存储 → 注入 → 执行 → 失效）

```mermaid
sequenceDiagram
    autonumber
    participant UI as 前端/桌面 UI
    participant LM as LoginManager
    participant CAM as CliAuthManager
    participant LWP as LWP DingTalkCliAuthService
    participant DWS as dws CLI (本地子进程)
    participant FS as <dws_bin>/.dws/token.json
    participant RT as RealToolsRuntime<br/>(real_tools_adapter.rs)
    participant SH as SparkSessionHandler

    UI->>LM: QR 登录成功
    LM->>CAM: on_login_success()  cli_auth.rs:132
    CAM->>FS: clear_dws_token_file("login_success")  cli_auth.rs:264
    CAM->>LWP: getCliAuthCode({org_id})  cli_auth.rs:813-839
    LWP-->>CAM: { authCode }
    CAM->>DWS: dws auth exchange --code <authCode> --uid <uid>  cli_auth.rs:1136-1162
    DWS->>FS: 写 token.json (由 dws CLI 自己写)
    Note over CAM,FS: 客户端 **不直接读 token**；<br/>仅用 mtime 判定 20 天有效期<br/>(CLI_AUTH_VALIDITY_MS, cli_auth.rs:25)

    loop 每 6h reconcile / 到期前 24h 提前续
        CAM->>CAM: refresh_loop()  cli_auth.rs:274-353
        CAM->>LWP: getCliAuthCode（续期）
        CAM->>DWS: dws auth exchange ... （覆盖 token.json）
    end

    UI->>RT: Agent 调 shell 工具 (dws ...)
    RT->>DWS: spawn dws 子进程<br/>env: REWIND_SESSION_ID/MESSAGE_ID/REQUEST_ID
    DWS->>FS: 读 token.json
    alt 权限足够
        DWS-->>RT: exit_code=0，正常结果
    else exit_code=4 + PAT_*_NO_PERMISSION
        RT->>RT: maybe_intercept_dws_pat_permission()  real_tools_adapter.rs:892-1011
        RT->>SH: request_dws_pat_permission(conv_id, call_id, req)
        SH->>UI: AGUI 授权弹窗 (Low/Medium/High 三种 type)
        alt 中敏 — 用户选 allow_once/session/permanent
            SH->>DWS: dws pat chmod <scopes...> --grant-type <x> --agentCode <md5> [--session-id <conv>]<br/>spark_session_handler.rs:1778-1812
            DWS-->>SH: exit_code=0 → DwsChmodOutcome::Retry
            SH-->>RT: Retry
            RT->>DWS: 重跑原始工具 → 递归回 maybe_intercept
        else 高敏 — resend / skip_auth
            SH->>DwsPatHighRiskRegistry: register(authRequestId) → oneshot rx
            SH->>/s/synca: 订阅 objectType=1050000
            /s/synca-->>SH: { authRequestId, status: approved|rejected }
            SH->>Registry: resolve(authRequestId, status)
        end
    end

    UI->>LM: logout / org 切换
    LM->>CAM: on_logout() / on_org_identity_changed()  cli_auth.rs:152/917
    CAM->>FS: remove token.json  cli_auth.rs:936-1007
```

**关键常量**（`cli_auth.rs:25-35`）：

- `CLI_AUTH_VALIDITY_MS = 20 * 24h`（token 有效期）
- `CLI_AUTH_REFRESH_BEFORE_EXPIRY_MS = 24h`（提前续期阈值）
- `CLI_AUTH_RECONCILE_INTERVAL_MS = 6h`（兜底 reconcile 周期）
- `CLI_AUTH_REQUEST_MAX_ATTEMPTS = 3` + cooldown 10min
- `CLI_AUTH_MAX_EXCHANGE_ATTEMPTS = 4` → 触发 `dws auth login` 手动 fallback

**凭证存储位置**（`cli_auth.rs:1183-1185` + `base/environment/dingtalk_cli.rs`）：

```1183:1185:tauri-app/src-tauri/src/dingtalk_core/login/cli_auth.rs
fn dws_token_path_from_binary(binary: &Path) -> Option<PathBuf> {
    dingtalk_cli_token_path_from_binary(binary)
}
```

测试里给了显式路径（`cli_auth.rs:1447-1453`）：`<work_root>/.bin/dws/bin/.dws/token.json`。**不走 OS keychain**，仅依赖 dws CLI 自己的文件持久化 + 客户端 mtime 判新鲜度。

---

## 3. 权限模型（scope / high-risk）数据结构

### 3.1 三级风险 + Request 结构

```7:100:tauri-app/src-tauri/src/agent/runtime/spark_loop/adapter/dws_pat_permission.rs
#[derive(Debug, Clone, PartialEq)]
pub enum DwsPatRiskLevel {
    Low,
    Medium,
    High,
}
// ...
pub struct DwsPatPermissionRequest {
    pub risk_level: DwsPatRiskLevel,
    pub perm_code: String,               // PAT_{LOW|MEDIUM|HIGH}_RISK_NO_PERMISSION
    pub command: Option<String>,
    pub grant_options: Vec<String>,      // ["once","session","permanent"]，缺失默认 ["session"]
    pub required_scopes: Vec<String>,    // productCode.resourceType:operate
    pub auth_request_id: Option<String>, // 仅 High 携带
    pub display_name: Option<String>,
    pub product_name: Option<String>,
    pub required_scopes_raw: Vec<serde_json::Value>,
}
```

### 3.2 识别契约（与服务端一一对齐）

- 触发条件：`exit_code == 4` 并且 stderr 中存在一个合法 JSON 对象，其 `code` 或 `error_code` 字段命中 `PAT_*_NO_PERMISSION`（`dws_pat_permission.rs:111-147`）。
- **Scope 格式三兼容**（`dws_pat_permission.rs:172-194`）：
  1. `{ "scope": "aitable.record.read" }` — 直接用
  2. `{ productCode, resourceType, operate }` → `"productCode.resourceType:operate"`
  3. `{ productCode, operate }`（无 resourceType）→ `"operate"`
- **鲁棒 JSON 提取**（`dws_pat_permission.rs:120-146`）：逐个 `{` 尝试 `Deserializer::from_str` 的第一个 value，容忍 `[win-sandbox-exec]` 前缀/后缀日志行（测试证据 `:364-404`）。

### 3.3 管道兜底（exit_code 被 `| head` 覆盖）

```926:944:tauri-app/src-tauri/src/agent/runtime/spark_loop/adapter/real_tools_adapter.rs
let effective_exit_code = if exit_code != 4
    && command.as_deref().map(|c| c.contains('|')).unwrap_or(false)
    && (stderr_or_stdout.contains("\"code\":\"PAT_HIGH_RISK_NO_PERMISSION\"")
        || stderr_or_stdout.contains("\"code\": \"PAT_HIGH_RISK_NO_PERMISSION\"")
        || ...
        || stderr_or_stdout.contains("\"code\": \"PAT_LOW_RISK_NO_PERMISSION\""))
{
    tracing::warn!("[dws][permission] pipe_masked_exit_code ...");
    4
} else { exit_code };
```

### 3.4 高敏异步注册表

```16:28:tauri-app/src-tauri/src/agent/runtime/spark_loop/adapter/dws_pat_high_risk_registry.rs
pub struct DwsPatHighRiskRegistry {
    pending: Mutex<HashMap<String, oneshot::Sender<String>>>,
    processed: Mutex<HashSet<String>>,
}
```

- `register(authRequestId)` 返回 `oneshot::Receiver<String>`（`dws_pat_high_risk_registry.rs:33-42`）
- `resolve(authRequestId, "approved"|"rejected")`：先查 processed 去重，再 take Sender 发送（`:57-112`）
- 同步协议路径与按钮路径只会有一个成功

### 3.5 注意

"高风险命令清单"不在客户端；**客户端不持有高敏命令白/黑名单**。识别完全由服务端给出 `code=PAT_HIGH_RISK_NO_PERMISSION` 触发，客户端只做路由与 UI 渲染。

---

## 4. `workspace-cli` 作为被调子进程的契约

### 4.1 二进制定位（`base/environment/dingtalk_cli.rs`）

- 常量：`DINGTALK_CLI_BIN_NAME = "dws"` / `"dws.exe"`（`:15-18`）
- 候选路径（优先序）：
  1. `<exe_dir>/bin/dws` — 主 seed（`:217-220`）
  2. `<resource_dir>/dws/bin/dws`、`<resource_dir>/resources/dws/bin/dws` — 兼容 seed（`:226-270`）
  3. `<exe_dir>/dws/bin/dws` — 安装后目录（`:301-320`）
- 运行时托管根：`<work_root>/.bin/dws/bin/dws`（`managed_dingtalk_cli_root`, `:87-96`）
- 解析器：`backend_integration::dingtalk_cli::find_dingtalk_cli_binary(app)`

### 4.2 调用形态

| 场景 | 调用方 | 命令 | 特殊点 |
|---|---|---|---|
| **Auth bootstrap** | `cli_auth.rs::run_exchange_command:1136-1162` | `dws auth exchange --code <authCode> --uid <uid>` | `no_window_cmd`；读 exit_code 判成败 |
| **Manual auth fallback** | `cli_auth.rs::launch_manual_auth_login:1164-1174` | `dws auth login` | `spawn()` 起交互 |
| **PAT chmod 授权** | `spark_session_handler.rs:1778-1812` | `dws pat chmod <scope...> --grant-type once\|session\|permanent --agentCode <md5> [--session-id <conv_id>]` | tokio command；收 exit_code+stderr |
| **业务工具调用** | 通过 Spark Loop `shell` 工具（Agent 自主构造 `dws ...` 命令）| 任意 `dws` 子命令 | 受 sandbox_exec 环境包裹 |

### 4.3 stdin / stdout / stderr 契约

- stdin：一律 `Stdio::null()`（auth exchange）/ 或由上层 shell 工具决定
- stdout：成功时业务 JSON；`dws pat chmod` 成功打印 stdout 即算成功（`spark_session_handler.rs:1815-1821`）
- stderr：**结构化 JSON 优先**，PAT 错误走 stderr；客户端做鲁棒解析（允许日志前缀与 sandbox-exec 后缀）
- exit code：
  - `0` 成功
  - `4` PAT 权限不足（业务语义契约，参见 `dws_pat_permission.rs:116-118`）
  - 其他 失败
- 输出 `2>&1` 合流的兼容：real_tools_adapter 在 stderr 为空时回退读 stdout（`:907-916`）

### 4.4 Env 约定

只注入 **trace 追踪上下文**，不传身份 / PAT：

```228:242:tauri-app/src-tauri/src/agent/runtime/sandbox_exec.rs
pub fn with_request_context(
    mut self,
    session_id: impl Into<String>,
    message_id: impl Into<String>,
    request_id: impl Into<String>,
) -> Self {
    self.env.insert("REWIND_SESSION_ID".to_string(), session_id.into());
    self.env.insert("REWIND_MESSAGE_ID".to_string(), message_id.into());
    self.env.insert("REWIND_REQUEST_ID".to_string(), request_id.into());
    self
}
```

Windows 下另注 `WIN_SANDBOX_EXEC_VERBOSE=1` 用于 debug（`sandbox_exec.rs:244-259`）。

### 4.5 Retry 策略

- **PAT retry**：授权成功（`DwsChmodOutcome::Retry`）→ `tool.execute(call, ctx)` 重跑一次 → 递归回 `maybe_intercept_dws_pat_permission`（`real_tools_adapter.rs:968-995`）— 以处理"续授权后撞到下一个 PAT 错误码"的链式场景
- **Outside-scope retry**：非 dws 的普通 shell 失败时，从 stderr 抽出"被拒的绝对路径"并弹 sandbox 扩权框（`real_tools_adapter.rs:1013-1029` + `workspace_compat.rs:423-444`）— 该路径显式 skip dws：`detect_dws_invocation_in_input(&call.arguments) != NotDws` 即 bypass（`:1022-1025`）
- **授权失败注入**：`ChmodFailed(reason)` → 在原 payload 的 stderr JSON 内注入 `authAttempted=true / authFailed=true / authError=<reason>`（`real_tools_adapter.rs:731-758`），让 Agent 自然感知

### 4.6 灰度总开关

`general.disable_wukong_dws_new_permission = true` 时整条 PAT 拦截链跳过（`real_tools_adapter.rs:874-883`，`dws_detection.rs:40-46`）。

---

## 5. 身份 / 组织上下文传递

### 5.1 关键事实

**客户端不把 uid / org_id 通过 env 或 header 传给 dws**。身份绑定发生在 `dws auth exchange` 时，由 dws CLI 自行持久化到本地 `.dws/token.json`；之后每次 `dws ...` 子进程启动都由 dws CLI 自己读 token 自带身份。

| 字段 | 载体 | 证据 |
|---|---|---|
| `uid`（open_id）| `dws auth exchange --uid` 一次性传入 | `cli_auth.rs:842-851, 1136-1146` |
| `authCode` | `dws auth exchange --code` 一次性传入 | 同上 |
| `org_id` | 从 `resolve_current_org_id(app)` 解，仅用于 LWP `getCliAuthCode` 请求体 | `cli_auth.rs:872-884`，`:826-828` |
| `agentCode` (md5) | 仅 `dws pat chmod --agentCode <md5>` 时注入 | `spark_session_handler.rs:1786-1789` |
| `session-id` (conv_id) | 仅 `dws pat chmod --session-id <conv>` 时注入（grant_type=session）| `spark_session_handler.rs:1791-1794` |
| trace 上下文 | env: `REWIND_SESSION_ID/MESSAGE_ID/REQUEST_ID` | `sandbox_exec.rs:228-242` |

### 5.2 组织切换一致性

- `CliAuthManager.on_org_identity_changed` (`cli_auth.rs:917-934`) 会：bump epoch + cancel tasks + 清 expire_at_ms/last_exchange_at_ms + 重置 uid 缓存 + **删 token.json**
- 下一次调 dws → 命中 401 / token missing → `ensure_dws_auth_ready` 走 `getCliAuthCode` + `auth exchange` 重新绑定
- `did_org_switch(:1289-1291)` 仅在上一任 org_id>0 且与当前不同才视为切换，避免首次登录误判
- `compute_agent_unique_id` 优先用 `device_corp_id`，为空则取主组织 `corp_id`（`device_service.rs:40-56`）— 意味着 `agentCode` 跨 org 切换**通常会变**

### 5.3 SessionHandler trait（与 Spark/ACP 对齐）

```48:52:tauri-app/src-tauri/src/agent/session/session_handler.rs
pub enum DwsChmodOutcome {
    Retry,
    Rejected,
    ChmodFailed(String),
}
```

```122:129:tauri-app/src-tauri/src/agent/session/session_handler.rs
async fn request_dws_pat_permission(
    &self,
    _conversation_id: &str,
    _call_id: &str,
    _request: &DwsPatPermissionRequest,
) -> Result<DwsChmodOutcome, SessionPermissionError> {
    Ok(DwsChmodOutcome::Rejected)
}
```

trait 默认 `Rejected`，只有 `SparkSessionHandler` 覆盖真实交互（`spark_session_handler.rs:1468-1702`）。

---

## 6. 与服务端 a2a 的字段对齐表

| 客户端字段 | 服务端/协议来源 | 值域 | 用途 |
|---|---|---|---|
| `authCode` | LWP `DingTalkCliAuthService.get_cli_auth_code({org_id})` | 一次性短 code | 交换 long-lived token |
| `error_code` / `code` (stderr JSON) | dws CLI 打印 | `PAT_LOW/MEDIUM/HIGH_RISK_NO_PERMISSION` | 触发客户端 PAT 弹窗 |
| `data.grantOptions` | dws stderr | `["once","session","permanent"]` | 弹窗按钮生成，缺失默认 `["session"]` |
| `data.requiredScopes[*].scope` | dws stderr | `"productCode.resourceType:operate"` 或 `"productCode.resourceType.action"` | chmod 命令参数 |
| `data.requiredScopes[*].displayName` / `productName` | dws stderr | 展示文案 | AGUI 卡片 + 钉钉卡片 operation/productName 变量 |
| `data.authRequestId` | dws stderr（仅 High）| UUID | 客户端 oneshot 注册 key |
| `/s/synca` push `objectType` | 同步协议 | `1050000` | 高敏授权异步回执 |
| synca action | 同步协议 | `approved` / `rejected` | 触发 `DwsPatHighRiskRegistry::resolve` |
| `option_id` (ACP 响应) | 前端按钮 | `allow_once` / `allow_session` / `allow_permanent` / `reject_once` / `resend` / `skip_auth` | 决定 chmod grant-type（`spark_session_handler.rs:1747-1767`）|
| `BLOCK_CODE_DWS_ORG_UNAUTHORIZED` | ACP 审批 blocked_reason.code | 常量字符串 `"DWS_ORG_UNAUTHORIZED"` | 触发 `StopCurrentTurn`（`real_tools_adapter.rs:136, 1283-1294`，`approval_adapter.rs:30`）|
| stderr injection `authAttempted`/`authFailed`/`authError` | 客户端注入 | bool / bool / string | chmod 失败时让 Agent 知悉"已尝试"（`real_tools_adapter.rs:731-758`）|
| subprocess trace env | 客户端注入 | `REWIND_SESSION_ID`/`MESSAGE_ID`/`REQUEST_ID` | dws CLI 日志链路聚合 |

---

## 7. 客户端-服务端合约：哪些必须固化到开源 CLI、哪些留给下游 Agent

### 7.1 必须固化到开源 `dingtalk-workspace-cli` 层（避免每个三方 Agent 重发明轮子）

1. **凭证存储与续期协议**
   - `dws auth exchange --code <authCode> --uid <uid>` 的 argv 与 exit code 语义
   - token.json 的路径约定（`<binary_dir>/.dws/token.json`）+ **mtime 作为"续期时间"**的契约
   - 20 天有效期语义必须由 CLI 侧兜底（不能让每个 Agent 自己判断）
   - `dws auth login` 作为交互兜底路径

2. **PAT 错误语义**
   - `exit_code=4` 专用于 PAT 权限不足
   - stderr JSON schema：`{ code|error_code, data: { grantOptions, requiredScopes, authRequestId, displayName, productName } }`
   - requiredScopes 的**字符串标准化**（推荐统一为 `scope` 字段，弃用 `productCode+resourceType+operate` 组装）— 目前客户端做三格式兼容是技术债，应该由 CLI 侧规范化

3. **PAT chmod 命令契约**
   - `dws pat chmod <scope...> --grant-type <once|session|permanent> --agentCode <id> [--session-id <id>]`
   - 参数名、顺序、exit code 必须稳定

4. **管道输出不可丢错误**
   - 客户端目前对 `dws ... | head` 做了丑陋兜底（字符串搜索 `"code":"PAT_*"`）
   - 建议 CLI 层对 `--json` 输出做强格式保证，或增加 `--always-fail-pipe` 策略让 stderr 不丢

5. **trace 环境变量透传**
   - `REWIND_SESSION_ID / REWIND_MESSAGE_ID / REWIND_REQUEST_ID` 应该被 CLI 正式纳入 log 链路字段，让 dws 自身日志可被上游聚合

6. **组织切换的 token 失效信号**
   - CLI 应在感知 org 不匹配时返回特定 exit code（而非泛化 401），客户端可据此自动走 re-exchange，无需依赖"401 → 猜测 → 重跑"

### 7.2 可留给下游 Agent 自决

1. **弹窗 UI / i18n 文案**：客户端当前用 `acp.dwsPat.low.title` 等 key 在前端定义，CLI 不应干预
2. **授权选项的默认值**：`grant_options` 缺失时默认 `["session"]` 的策略可能每个 Agent 不同
3. **高敏异步通道**：`/s/synca objectType=1050000` 属于钉钉专属同步协议，三方 Agent 可用 Webhook/轮询代替
4. **agent 唯一 id 算法**：`md5(open_id+corp_id+device_id)` 是 Rewind 特定实现；CLI 只该收 `--agentCode`，不该干预算出来的方式
5. **灰度开关**（`general.disable_wukong_dws_new_permission`）：完全属于客户端的平滑升级手段
6. **重试/递归策略**：PAT 续授权后重跑原命令的递归、outside-scope 路径重试、workspace_compat 的 shell 输出 normalize — 都是 Agent 上层产品策略
7. **session_handler 三态**（`Retry / Rejected / ChmodFailed`）：Agent 侧的分发语义，不上升到 CLI

### 7.3 建议新增的 CLI 层能力（调研结论）

- `dws auth status --json`：返回 `{ uid, org_id, expire_at, remaining_days }`，消除客户端读 mtime 推断的脆弱性（现状见 `cli_auth.rs:1187-1234`）
- `dws pat status --json --scope <scope>`：客户端在发起工具前预检 scope，降低 exit_code=4 触发频率
- `dws auth logout`：目前客户端靠 `rm token.json`（`cli_auth.rs:1251-1287`）；应由 CLI 提供原子化退出
- 标准化 stderr：所有 PAT-类错误**单行 JSON**打印到 stderr（不混日志），或用单独 fd（如 fd=3）输出结构化事件流，避免 `[win-sandbox-exec]` 混杂与管道丢信号

---

## 附：未进入扫描但被引用的关键常量

- `TURN_STOP_CODE_DWS_ORG_UNAUTHORIZED = "DWS_ORG_UNAUTHORIZED"` — `real_tools_adapter.rs:136`
- `CLI_AUTH_VALIDITY_MS = 20 * 24 * 60 * 60 * 1000` — `cli_auth.rs:25`
- `DINGTALK_CLI_BIN_NAME = "dws"` (`"dws.exe"` on Windows) — `base/environment/dingtalk_cli.rs:15-18`
- Windows 托管路径：`<work_root>/.bin/dws/bin/dws`（`managed_dingtalk_cli_root`，`dingtalk_cli.rs:87-96`）
- macOS realdoc 路径：`<app>/Contents/Resources/resources/dingsync/realdoc.app/Contents/MacOS/realdoc` — `real_tools_adapter.rs:155-174`（dws 二进制则通过 `find_dingtalk_cli_binary` 独立查找）

---

### 回传 `musk-chief-operator` 的精炼结论

1. **凭证层脆弱点**：客户端靠 `token.json` 的 mtime 判 20 天新鲜度（`cli_auth.rs:1187-1234`、测试 `:1571-1617`）。如果用户手工 touch 过文件或时钟漂移，会误判。**建议 CLI 提供 `dws auth status --json`**。
2. **管道兜底是技术债**：`real_tools_adapter.rs:926-944` 用字符串匹配兜住 `| head` 覆盖 exit_code。CLI 侧应提供强结构化契约。
3. **身份不走 env/header**：这对"开源 CLI + 多 Agent 宿主"是优势（隔离好），但也意味着 **CLI 层必须拥有完整 auth 子命令链**（exchange / status / logout），否则三方 Agent 无法正确管理凭证生命周期。
4. **高敏异步通道强耦合钉钉 `/s/synca`**：不适合固化到开源 CLI；保留 oneshot + authRequestId 的 in-process 路由给宿主 Agent 实现。
5. **三格式 scope 兼容**应在 CLI 层收敛到单一 `scope` 字符串，避免每个宿主重复解析。

 Now I'm pulling together the complete picture. The PAT acquisition flow starts when login succeeds, triggering the auth manager to request a one-time authorization code from the DingTalk service using the organization ID, then exchanging that for the actual token.

The token gets stored locally in a JSON file with a 20-day expiration window, and the system automatically refreshes it when less than 24 hours remain, checking every 6 hours and falling back to manual login after four failed attempts. The permission model uses three risk levels tied to specific error codes returned by the CLI, supports multiple scope formats for flexibility, and includes grant options like one-time, session, or permanent access—with high-risk operations requiring an additional request ID for synchronization.

For high-risk operations, the system maintains an in-memory registry that pairs request IDs with async handlers, allowing the client to show a permission dialog while subscribing to sync protocol updates for approval or rejection. The workspace compatibility layer normalizes shell command outputs and detects when operations access paths outside the authorized scope, injecting appropriate error signals and localized user messages to guide the user.

The real_tools_adapter intercepts shell commands when the feature flag is enabled, checking for permission errors and triggering a PAT permission request flow with three possible outcomes: retry the tool, inject auth failure metadata into stderr, or return the original response unchanged. The chmod authorization command constructs a dws PAT request with the agent's unique identifier derived from user and device info, optionally including the session ID for session-scoped grants, relying entirely on the local dws CLI token for authentication rather than injecting environment variables.

Now I'm looking at how trace context flows through process environment variables while identity and organization information stay out of the dws communication layer—the dws CLI resolves these from its local token.json that was bound during the initial auth exchange. When the organization changes, the token gets cleared and the next dws call hits a 401, triggering a fresh token exchange. The client and server align on authorization request fields including grant options and required scopes, with scope formats handled for backward compatibility, and high-sensitivity authorizations use async callbacks to approve or reject based on the auth request ID. Organization-level authorization failures get mapped to a specific stop code that halts the current turn, while credentials use one-time auth code exchanges with tokens persisted locally.

The token remains valid for 20 days.