# dws dev 命令集 · Agent 人肉手工评测集（10 条复合用例）

> 性质：**人肉手工评测集**——由测评人逐条手工跑、肉眼核对、人工判分，不是自动化脚本。
> 用途：评测 agent（加载 `dingtalk-dev` 技能后）能否正确处理开放平台 dev 任务。
> 特点：10 条**复合用例**，每条串多个子任务，一条覆盖一类完整场景；10 条合起来覆盖全部 34 个子命令 + 8 类横切行为。
> 约定：所有命令应带 `--format json`；写操作应先 `--dry-run` 预览、用户确认后再 `--yes`；应用定位只用 `--unified-app-id`。

## 手工评测流程

逐条执行，每条三步：

1. **发起**：在一个干净的 agent 会话里，把该条的「用户说」原样发给 agent（不给额外提示）。
2. **观察**：看 agent 选了哪些命令、什么 flag、做了哪些判断/追问。
3. **判分**：对照「通过判据」人工打分。复合用例含多个判据，**全部满足才记 PASS**；部分满足记 PASS\*（半通过）并在备注写清缺哪条。记一行 `用例# | PASS / PASS* / FAIL | 备注（错在哪）`。

> 「易错点」是常见扣分项，重点盯。建议每次技能改动后整套重跑，对比上次。

## 覆盖矩阵

| 用例 | 覆盖的子命令 | 横切行为 |
|------|-------------|---------|
| C1 建应用配齐基础 | app create / get / credentials get / update | dry-run/yes、定位符、密钥脱敏 |
| C2 列表与定位 | app list | cursor 分页、按名定位、多命中候选 |
| C3 生命周期 | app disable / enable / delete | 写后回读、appStatus、pretty 标签、confirm-name 防误删 |
| C4 网页应用到生效 | webapp get / config | 生效模型（改配置≠生效） |
| C5 版本发布全流程 | version create / list / get / check-approval / publish / status | 生效模型、审批人由用户拍板 |
| C6 权限全流程 | permission list / add / remove | 过滤分页、生效模型、批量聚合出参 |
| C7 成员与安全 | member list / add / remove、security config | 整组覆盖语义 |
| C8 机器人与建联 | robot submit / result / get / config / enable / disable、dev connect | 异步轮询、robot info not exist、建联依赖预检、长驻进程 |
| C9 事件与文档排查 | event list / subscribe / unsubscribe、dev doc search | 错误码透传、文档 RAG |
| C10 意图消歧 | （不进 dev，先澄清） | 泛词边界、转其它技能出口 |

---

## 用例

### C1. 新建应用并配齐基础
- **用户说**：「建一个内部应用叫 DemoApp，描述『内部测试』；建好后给我看看它的详情，把它的 AppKey/AppSecret 也取出来；对了名字再改成 DemoApp2。」
- **覆盖**：`app create` / `get` / `credentials get` / `update`；dry-run/yes、定位符、密钥脱敏。
- **期望（分步）**：
  1. `app create --name DemoApp --desc 内部测试 --dry-run` → 给用户看 `invocation.params` 确认 → `--yes`，记下返回的 `unifiedAppId`。
  2. `app get --unified-app-id <id> --format json` 看详情。
  3. `credentials get --unified-app-id <id> --format json` 取凭证。
  4. `app update --unified-app-id <id> --name DemoApp2 --dry-run` → `--yes`。
- **通过判据**：每个写操作先 dry-run 再 yes；全程用 `unifiedAppId` 定位；取凭证走 `credentials get`（不是 app get）；`clientSecret/appSecret` 按敏感处理、不明文写进回答。
- **易错点**：不 dry-run 直接 yes；把 secret 打印给用户；用 `app get` 当取凭证。

### C2. 应用列表与按名定位
- **用户说**：「列出我们企业的开放平台应用，一页 20 条，有下一页继续翻；再帮我找名字叫『早晚会』的那个应用，看它详情。」
- **覆盖**：`app list`；cursor 分页、按名定位、多命中。
- **期望（分步）**：
  1. `app list --page-size 20 --format json`；出参有 `nextCursor` 则续翻 `--cursor <上次 nextCursor>` 直到为空。
  2. `app list --name 早晚会 --format json` 找 `unifiedAppId` → 唯一命中后 `app get --unified-app-id <id>`。
- **通过判据**：首次不传 `--cursor`，续翻原样回传 `nextCursor`，不自己构造/解析、不跨命令复用；用 list 过滤拿 id 再 get；多条命中时展示候选让用户选、不取第一条。
- **易错点**：用 `--page/--offset` 翻页；`app get --name xxx`（get 不接受 name 定位）。

### C3. 应用生命周期（停用 / 启用 / 删除）
- **用户说**：「先把 DemoApp2 停用，确认停好了告诉我；然后再启用回来；最后这个应用不要了，删掉。」
- **覆盖**：`app disable` / `enable` / `delete`；写后回读、appStatus、pretty、confirm-name。
- **期望（分步）**：
  1. `disable --unified-app-id <id> --dry-run` → `--yes` → 回读 `app get`（可 `--format pretty` 看 `appStatusText`），确认 `appStatus=0` 才算停用完成。
  2. `enable --dry-run` → `--yes` → 回读确认 `appStatus=1`。
  3. 删除：先 `app get` 展示摘要 → `delete --dry-run` → 真删需 `--confirm-name <应用真实名>`（与定位到的名一致）+ `--yes`。
- **通过判据**：写成功 ≠ 状态已变，每步回读 appStatus（0停/1激活/2待激活/3过期）；删除前展示摘要并让用户确认；confirm-name 匹配才删，读不到应用名时中止（fail-closed）。
- **易错点**：看到 success 就回报已停/已删不回读；不带 confirm-name 直接删。

### C4. 网页应用配置到生效
- **用户说**：「给这个应用配个钉钉里打开的移动端首页 https://example.com/m，配完要真正能用。」
- **覆盖**：`webapp config` / `get`；生效模型。
- **期望（分步）**：`webapp config --unified-app-id <id> --homepage-url https://example.com/m --dry-run` → `--yes` → `webapp get` 回读；明确说明「改配置 ≠ 线上生效，需走版本通道」：`version create → check-approval → publish`（详见 C5）。
- **通过判据**：先 dry-run 再 yes；配完回读 webapp get；主动点明需发版本才生效，不谎称「已生效」。
- **易错点**：配完直接说已生效，不提版本通道。

### C5. 版本发布全流程（含选审批人）
- **用户说**：「我刚改了配置，发个版本上线；先看下历史版本和这次要发的版本详情；需要审批的话我来选审批人。」
- **覆盖**：`version create` / `list` / `get` / `check-approval` / `publish` / `status`；生效模型、审批人由用户拍板。
- **期望（分步）**：
  1. `version create --unified-app-id <id> --version <号> --desc <说明> --yes`，记 `versionId`（新应用 `version list` 空时先 create，不要误判无可发布）。
  2. `version list` 看历史、`version get --version-id <id>` 看详情。
  3. `version check-approval --version-id <id>`（预检，不发布，返回是否需审批 + 候选审批人）。
  4. 把候选审批人列表给用户选 → `version publish --version-id <id> --approver <用户选的> --yes`（含高敏权限加 `--confirm-sensitive`）。
  5. `version status --version-id <id>` 跟踪到 `versionStatus=RELEASE` 才算生效。
- **通过判据**：check-approval 不实际发布；审批人由用户拍板、agent 不默认取第一个；发布后回读 status 到 RELEASE。
- **易错点**：跳过 check-approval 直接 publish；agent 自己选审批人；version list 空就说没东西可发。

### C6. 权限全流程（查 / 申请 / 批量取消）
- **用户说**：「查下跟『机器人发消息』有关、还没开通的权限；开通其中合适的那个，要真正生效；再把另外两个不需要的权限点 A、B 一起取消掉。」
- **覆盖**：`permission list` / `add` / `remove`；过滤分页、生效模型、批量聚合。
- **期望（分步）**：
  1. `permission list --unified-app-id <id> --keyword 机器人发消息 --status UNAUTHED --page-size 50` 找 `scopeValue`（150+ 时用 `nextCursor` 续翻）。
  2. `permission add --permissions <scopeValue> --dry-run` → `--yes`；若 `requiredApproval=true`，走版本通道生效（接 C5）。
  3. `permission remove --permissions A,B --dry-run` → `--yes`，读出参 `{results, ok, total, failedCount}` 逐条判断。
- **通过判据**：只传 `scopeValue`（不传 API/分组名）；用 keyword+status 过滤、分页不漏；需审批的明确走版本；批量取消读 `ok/failedCount` 报告部分失败，不只看命令成功。
- **易错点**：把 API 名当权限点；add 后就说开通了；批量 remove 漏报部分失败。

### C7. 成员与安全配置
- **用户说**：「把 userId 张三、李四加成这个应用的开发者，加完看下成员列表，回头把李四移除；另外给应用加一个登录重定向地址 https://b.example.com/cb，别把原来的地址冲掉。」
- **覆盖**：`member list` / `add` / `remove`、`security config`；整组覆盖。
- **期望（分步）**：
  1. `member add --unified-app-id <id> --user-ids 张三id,李四id --member-type DEVELOPER --dry-run` → `--yes` → `member list` 回读 → `member remove --user-ids 李四id --member-type DEVELOPER --dry-run` → `--yes`。
  2. 安全配置：提醒 `--redirect-urls` 是**整组覆盖、不是追加**——要保留原地址需把旧+新一起传：`security config --redirect-urls <旧1,旧2,新> --dry-run` → `--yes`。
- **通过判据**：`--user-ids` 逗号分隔、`--member-type` 必填、用 userId 不用姓名；识别整组覆盖语义、避免只传新地址冲掉旧的；未提供的字段（如 ip-whitelist）不动。
- **易错点**：漏 `--member-type`；security 只传新 redirect-urls 把旧的清空。

### C8. 机器人建号、配置与本地建联
- **用户说**：「帮我建一个叫『小助手』的答疑机器人；另外这个现有应用还没机器人，给它也配上并启用；最后把机器人接到我本地的 Claude Code 调试。」
- **覆盖**：`robot submit` / `result` / `get` / `config` / `enable` / `disable`、`dev connect`；异步轮询、robot info not exist、建联依赖预检、长驻进程、密钥脱敏。
- **期望（分步）**：
  1. 新建：`robot submit --name <应用名> --robot-name 小助手 --desc <功能> --dry-run` → `--yes`（拿 taskId）→ 按 `intervalSeconds` 轮询 `robot result --task-id <taskId>`，只有 `SUCCESS` 才用返回 `robotCode/clientId/clientSecret`（敏感）。
  2. 现有应用：`robot get` 若 `robotStatus=UNCONFIGURED` → `robot config --unified-app-id <id> --name ... --mode STREAM --dry-run` → `--yes`（upsert 首次即创建）→ 回读 `robot get` 看 `robotStatus=ONLINE` → 需要时 `robot enable`（停用 `robot disable`）。
  3. 建联：`dev connect --channel auto --robot-client-id x --robot-client-secret y --dry-run` 看出参 `cli` 字段做依赖预检；正式 connect 是前台长驻进程，对话里跑要后台运行并告诉用户怎么停，或引导自己开终端。
- **通过判据**：走异步 submit/result（同步建号已下线），轮询到 SUCCESS 再用凭证；未配置时走 config 不是 enable；config 是 upsert；写后回读 `robotStatus`；建联先 dry-run 预检、处理好长驻/缺凭证（先 submit/result 建号）。
- **易错点**：找「同步一次建好」的命令；WAITING 就用凭证；robot info not exist 时去 enable；前台直接起 connect 卡住对话。

### C9. 事件订阅与上游错误排查
- **用户说**：「让这个应用订阅『群成员入群』事件，订阅完看下当前订阅了哪些，再把它取消掉；对了我之前发版本报了个 errcode 62012，这是啥意思？」
- **覆盖**：`event list` / `subscribe` / `unsubscribe`、`dev doc search`；错误码透传、文档 RAG。
- **期望（分步）**：
  1. `event list --unified-app-id <id> --page-size 20 --format json` 取 `eventCode` → `event subscribe --unified-app-id <id> --event-codes chat_add_member_org --dry-run` → `--yes` → `event list` 回读 → `event unsubscribe --unified-app-id <id> --event-codes chat_add_member_org --dry-run` → `--yes`。事件码不确定先 `event list` 翻页查。
  2. 错误码：业务错误 `ServiceResult.success=false` 原样透传 `errorCode/errorMsg`，再 `dev doc search --keyword "errcode 62012 <message>" --format json` 做官方文档 RAG，结论基于命中条目。
- **通过判据**：`--event-codes` 逗号分隔，写操作先 dry-run；`event list` 使用 `hasMore/nextCursor` 翻页；不编造事件码/错误含义；先透传原始错误再走 RAG，结论不臆测、不编不存在的命令。
- **易错点**：编事件码；把事件回调地址塞进事件订阅命令；凭空解释错误码。

### C10. 意图消歧（泛词边界）
- **用户说**：「帮我建个机器人。」（无任何开放平台上下文）
- **覆盖**：泛词消歧、边界与角色。
- **期望**：`应用`/`机器人` 是泛词——先追问确认是不是开发者后台的「企业内部应用机器人」，还是工作台应用、或群里发消息的机器人（→ `dingtalk-chat`）；确认是开放平台场景后才走 dev 流程（接 C8）。
- **通过判据**：不直接假设走 dev，先澄清；能正确指向其它技能出口。
- **易错点**：上来就 `robot submit`，没确认是不是开放平台场景。

---

## 备注

- 10 条合起来覆盖全部 34 个子命令 + 8 类横切行为（见覆盖矩阵）。
- 评测可分两层：**静态**——无环境，只看 agent 选的命令/flag/判断是否符合「期望/通过判据」；**真机**——有联调环境时核对真实出参。
- 真机注意：`dev connect` 正式连接是长驻进程；`version publish`/`app delete` 等写操作请用占位应用或停在 dry-run，避免动真实数据。
