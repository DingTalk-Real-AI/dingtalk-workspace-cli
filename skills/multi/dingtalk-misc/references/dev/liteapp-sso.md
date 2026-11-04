# 轻应用免登（H5 身份接入）实现文档

> 适用对象：通过 `dws dev liteapp create` 创建的轻应用（H5 应用，创建即发放
> OAuth 凭证：clientId = appKey；客户端内免登使用 `snsapi_base`，OAuth `users/me` 路径必须申请 `openid Contact.User.Read`。
> 本文覆盖钉钉内 H5 与纯浏览器两种免登路径的完整实现，agent 可按此直接产出接入代码。
>
## 应用模型：轻应用与标准企业内部应用平行

轻应用（LiteApp）与标准企业内部应用是两个平行的应用类型，不能视为同一应用的不同配置，也不能交叉使用生命周期、回调地址或登录结论。

| 维度 | 轻应用 | 标准企业内部应用 |
|---|---|---|
| 生命周期 | `dws dev liteapp create/update/delete/list/detail` | 独立的标准应用命令域 |
| OAuth 回调 | 通过轻应用专属的 set/update 入口设置 | 使用标准应用自己的回调配置 |
| 回调配置 | 与标准应用不互通 | 与轻应用不互通 |
| 端内身份 | 以 `unionid` 为主，`userid` 不作为必填 | 通常以企业 `userid` 建立成员身份 |
| 端外 OAuth | 使用轻应用自己的 `clientId/appKey` 和回调登记 | 使用标准应用自己的 OAuth 配置 |

排查轻应用 `900103 应用不存在`、OAuth 回调失败或网页登录失败时，第一优先级检查轻应用自己的回调登记。当前 DWS 中，“set 轻应用”对应：

```bash
dws dev liteapp update <unifiedAppId> \
  --redirect-uris <url> \
  --dry-run --format json
```

确认预检目标和地址后，再对同一目标执行 `--yes`。`--redirect-uris` 是整体覆盖，不是追加。

> 官方基准文档：
> 端内 [企业内部网页应用免登](https://open.dingtalk.com/document/development/enterprise-internal-application-logon-free)、
> [requestAuthCode JSAPI](https://open.dingtalk.com/document/development/jsapi-request-auth-code)；
> 端外「网页方式登录应用（第三方网站）」（方式一构造链接 / 方式二 DTFrameLogin 内嵌二维码，
> 子接口 [获取用户token](https://open.dingtalk.com/document/development/obtain-user-token.md)、
> [获取用户通讯录个人信息](https://open.dingtalk.com/document/development/dingtalk-retrieve-user-information.md)）。
> 端内官方 Demo：web-app-sso.zip（前端 Vite + 后端 Spring Boot）。

## 前置条件（不满足则免登 JSAPI 直接失效）

1. **域名一致**：应用首页地址、重定向 URL（回调地址）、端内免登地址三者必须与
   运行免登 JSAPI 的页面域名一致。即 H5 页面要部署在你自己的域名上，创建时
   `--homepage-url` 填该域名（官方 Demo 推荐带 `?corpid=$CORPID$` 模板，钉钉打开时
   自动注入企业 corpId）；创建后改域名用 `dws dev liteapp update <unifiedAppId> --homepage-url <新地址>`，
   先 dry-run，确认后再加 `--yes`。
2. **端内免登必须在钉钉客户端内打开**：官方原话——免登操作需要在钉钉端内实现，
   端外无法正常调用免登组件。钉钉外浏览器走路径 B。
3. 应用需已发布（工作台可见），否则无法进入页面。
4. **建议缓存身份**：用 dd.setStorage / cookie 缓存用户信息，避免每次进应用都调免登。

## 原理与选型

| 路径 | 场景 | 原理 |
|---|---|---|
| A. 端内免登（官方标准，推荐） | H5 首页在钉钉客户端内打开 | 前端 `dd.requestAuthCode` 拿免登码 → 服务端用 appKey+secret 换轻应用用户身份，以 `unionid` 为必需主身份，`userid` 可空 |
| B. 端外网页登录（官方标准） | 钉钉外浏览器扫码/授权 | 方式一构造链接 / 方式二 DTFrameLogin 内嵌二维码，scope 显式声明 `openid Contact.User.Read` → authCode → userAccessToken → users/me（unionId=me） |

两个路径都以 **appKey（=clientId）+ appSecret（=clientSecret）** 标识应用；
凭证来自 `dws dev liteapp create` 或持续查询的 `dws dev liteapp credential <unifiedAppId>`，
勿放前端。secret 明文即使通过 CLI 查询到，也不得写入回答、日志、文档、邮件、群聊或代码仓库。

## 分流硬边界（排障高频根因）

两套 code 属于不同签发体系，**通道由发起登录的入口 URL 决定：禁止按 code 长度/形态猜链路，禁止失败后自动降级到任何非官方旧接口**。

| code 来源 | 唯一合法兑换链路 | 错送后果（真实报文） |
|---|---|---|
| 路径 A `dd.requestAuthCode` 免登码 | `gettoken` → `topapi/v2/user/getuserinfo` | 送 `users/me`：403 `Forbidden.AccessDenied.AccessTokenPermissionDenied`，`accessdenieddetail.requiredScopes=["Contact.User.Read"]`；送旧版 SNS 接口：errcode 40078「不存在的临时授权码」 |
| 路径 B `oauth2/auth`（scope 声明 `openid Contact.User.Read`）回调 code/authCode | `userAccessToken` → `users/me`（unionId=me） | scope 未声明 Contact.User.Read：token 不带该 scope，`users/me` 403 `AccessTokenPermissionDenied`；送任何旧版接口：40078/authCode.notFound |

- errcode 40078「不存在的临时授权码」= code 被送进错误的签发体系（或已消费/过期）。任何「users/me 失败降级旧接口」式 fallback 都只会制造第二个噪音错误，掩盖真实根因——保持官方链路，不要换接口。
- authCode/免登码均一次性：兑换失败由前端重新发起，服务端不重放旧 code。

## 路径 A：钉钉客户端内 H5 免登（官方标准，authCode → 轻应用用户身份）

### 第 1 步：引入官方客户端 SDK 并拿免登码

官方客户端 SDK 为 **dingtalk-jsapi**（全局对象 `dd`）。CDN 引入（放首页 `<head>`）：

```html
<script src="https://g.alicdn.com/dingding/dingtalk-jsapi/3.0.25/dingtalk.open.js"></script>
```

（生产环境也可 npm 引入：`npm install dingtalk-jsapi`。）

`dd.ready` 后用企业 corpId + 应用 clientId 换免登码（corpId 优先从首页地址的
`?corpid=$CORPID$` 模板取，钉钉打开应用时自动注入）。现行官方口径为
`dd.requestAuthCode`（clientId 必传）；旧版 SDK 挂在 `dd.runtime.permission.requestAuthCode`，
回调结构一致：

```js
// corpId 从首页地址的 ?corpid=$CORPID$ 模板注入（钉钉打开应用时自动替换）。
// 必须从当前 URL 读取，不要硬编码任何组织 ID——复制到别的组织会免登失败。
const corpId = new URLSearchParams(location.search).get('corpid')
const clientId = '<appKey>' // = create/credential 返回的 appKey；是标识非密钥，可入前端
if (!corpId) {
  // 提示：页面地址缺少 corpid 参数，请确认通过钉钉客户端工作台打开应用
  throw new Error('missing corpid')
}
dd.ready(() => {
  dd.requestAuthCode({
    corpId,
    clientId, // 企业内部应用传 AppKey；不是 AppSecret，也不是 unifiedAppId
    onSuccess: r => {
      fetch('/api/login?code=' + r.code) // 交给服务端换身份，code 不要落前端日志
    },
    onFail: err => {
      // 必须处理失败回调并给出可见提示；不写 onFail 时 SDK 静默失败，
      // 表现为「点击登录无任何反应」
    }
  })
})
```

### 第 2 步：服务端换取用户身份

```text
1) 换企业 access_token：
   GET https://oapi.dingtalk.com/gettoken?appkey=<appKey>&appsecret=<appSecret>
   → { access_token, expires_in(7200) }        // 建议缓存 7000 秒，别每次现取

2) 免登码换用户信息：
   POST https://oapi.dingtalk.com/topapi/v2/user/getuserinfo?access_token=<上一步 token>
   body: { "code": "<前端免登码>" }
   → errcode=0 后按 appKey 类型分两种返回形态（见下）
```

**返回形态按 appKey 类型固定**：

```text
标准企业内部应用 appKey → result: { userid, device_id, sys_level }
轻应用 appKey          → result: { name, unionid, sys, sys_level }   // 没有 userid
```

**轻应用 appKey 一定不返回企业 userid**（实测 2026-10-21）：轻应用的 appKey 是
`createOAuthApp(..., CLIENT_OAPI_ORG)` 创建、scope 仅 `snsapi_base` 的**个人身份型
OAuth 客户端**，`getuserinfo` 对它只回个人身份（name + unionid + 管理员级别）——与
用户是否本组织、权限是否生效（Contact.User.Read 已 authed=true 仍如此）均无关。
服务端实现：

```js
const result = json.result || {};
if (!result.unionid) throw new Error('getuserinfo 无 unionid'); // 唯一真失败条件
createSession({ unionid: result.unionid, userid: result.userid || null, name: result.name });
```

- **unionid 是轻应用端内免登的唯一主身份**；把「缺 userid」当失败（如报
  `user/getuserinfo no userid`）会挡掉所有端内登录。
- 标准应用返回 userid 时可直接用它建会话；需要姓名、头像等详情再调
  `POST topapi/v2/user/get`（body 带 userid）。
- 产品必须要企业 userid 时：换标准企业内部应用（`dws dev app create`）的 appKey，
  或另立需求给轻应用 client 增加企业通讯录能力。
- 免登码 5 分钟有效且一次性，兑换失败让前端重新 `requestAuthCode`，不重放。

> 官方 Demo：web-app-sso.zip（前端拿 code + 后端换用户身份的完整可运行示例，
> 从官方免登文档页下载）。轻应用分支以 unionid 为必需主身份、userid 可空；只有标准企业内部应用分支才按 userid 建会话。后端配置 clientId/clientSecret、前端配置 clientId 即可联调。

## 路径 B：端外网页登录（官方：网页方式登录应用）

> 企业内部应用**仅限本组织员工**登录，不支持跨组织授权；需要多企业用户请用第三方企业应用。

**前置条件**：

- 已申请 `Contact.User.Read` 权限点并生效：轻应用创建后申请通常直接生效；不要把标准应用的版本发布流程套用到轻应用。
- 授权链接 `scope` 必须**显式声明** `openid Contact.User.Read`（不声明则签出的 token 不带该 scope，`users/me` 必 403）。
- 轻应用 OAuth 回调地址必须通过轻应用自己的 set/update 入口登记，与 `redirect_uri` 逐字符一致；不能用标准应用的回调配置代替。

### 方式一：构造登录链接（PC 网页 / H5 通用）

302 引导用户访问：

```text
https://login.dingtalk.com/oauth2/auth?
  redirect_uri=<urlencode(回调地址)>
  &response_type=code
  &client_id=<appKey>
  &scope=openid%20Contact.User.Read
  &state=<防 CSRF 随机串>
  &prompt=consent
```

- `redirect_uri` 必须 urlencode。
- **scope 显式声明 user 权限点**：`openid Contact.User.Read`（空格分隔，需 URL 编码为
  `%20`）——不声明时签出的 token 不带该 scope，`users/me` 必 403 `AccessTokenPermissionDenied`。
- 也可用 `openid`（仅获 userid）或 `openid corpid`（额外返回用户所选组织 corpId，
  可配 `org_type=management` / `corpId` 过滤组织）。**不是 snsapi_base**。
- `state` 防 CSRF：随机生成、回调原样返回、必须校验。

### 方式二：内嵌二维码（DTFrameLogin）

```html
<script src="https://g.alicdn.com/dingding/h5-dingtalk-login/0.21.0/ddlogin.js"></script>
<div id="self_defined_element" style="width:300px;height:300px"></div>
```

```js
window.DTFrameLogin(
  { id: 'self_defined_element', width: 300, height: 300 },
  {
    redirect_uri: encodeURIComponent('<回调地址>'),
    client_id: '<appKey>',
    scope: 'openid Contact.User.Read',
    response_type: 'code',
    state: '<随机>',
    prompt: 'consent',
  },
  (loginResult) => {
    const { redirectUrl, authCode, state } = loginResult;
    window.location.href = redirectUrl;  // 或不跳转，直接把 authCode 交后端
  },
  (errorMsg) => { /* 必须展示失败原因（toast 等），不允许静默 */ }
);
```

- **同源要求**：嵌入二维码的页面必须与 `redirect_uri` 同源（协议、域名、端口一致），
  否则扫码后没有反应；排障先查同源、CDN（g.alicdn.com）可达性、浏览器缓存。
- 可用 `exclusiveLogin: true` + `exclusiveCorpId` 生成专属组织二维码，限制非本组织账号扫码。

### 回调与服务端兑换

用户同意授权后回调 `redirect_uri?code=xxx&authCode=xxx&state=xxx`
（**code 与 authCode 一致，取任一即可**；authCode 一次性，state 必须校验）。

```text
1) 换用户个人 token：
   POST https://api.dingtalk.com/v1.0/oauth2/userAccessToken
   Content-Type: application/json
   { "clientId": "<appKey>", "clientSecret": "<appSecret>",
     "grantType": "authorization_code", "code": "<authCode>" }
   → { accessToken, expireIn(7200), refreshToken }

2) 获取用户通讯录个人信息——unionId 参数值传字符串 "me"：
   GET https://api.dingtalk.com/v1.0/contact/users/me
   Header: x-acs-dingtalk-access-token: <userAccessToken>
   Scope prerequisite: `openid Contact.User.Read` (仅 `openid` 或 `openid corpid` 不能调用 users/me)
   → nick、avatarUrl、unionId、openId 等
```

**unionId 传 `me`**：该接口按 unionId 定位用户，查询当前授权人固定传字符串 `me`
（即 `GET /v1.0/contact/users/me`），不要先换算 unionId 再查。报错按官方链路排障，
**禁止降级任何旧接口**。

## 公开端点

使用官方生产端点：`mcp-gw.dingtalk.com`（MCP 网关）、`login.dingtalk.com`（登录授权）、`open-dev.dingtalk.com`（应用管理）。二维码 SDK 使用官方文档指定的版本及 CDN。

## 前置条件与常见坑

- **回调地址必须先登记**：通过轻应用专属入口执行 `dws dev liteapp update <unifiedAppId> --redirect-uris https://a/cb,https://b/cb --dry-run`，确认预览后再用同一命令加 `--yes` 执行；未登记的回调地址授权页/扫码页会直接拒绝。轻应用与标准应用回调配置不互通。
- **secret 只在服务端**：appSecret/userAccessToken 绝不能下发到前端。
- **OAuth scope**：调用 `GET /v1.0/contact/users/me` 必须使用 `openid Contact.User.Read`；仅 `openid` 或 `openid corpid` 不能调用 users/me。客户端内 authCode 免登仍使用 `snsapi_base`，两者不要混用。
- 需要手机号等敏感字段要走敏感权限申请，不在轻应用一期范围。
- **access_token 缓存**：企业 token 与 userAccessToken 都有有效期（7200s），
  均需缓存复用，禁止高频重取；authCode 一次性，需要复用先换 refreshToken。
- **仅本组织**：企业内部应用端外登录仅限本组织员工；判断用户归属用返回的 corpId
  与应用所属企业比对。
- **端外占位对象**：普通浏览器里 `window.dd` 可能存在占位对象，调用免登 JSAPI
  不回调、表现为「点击无反应」。进入路径 A 前先判断 UA 含 DingTalk，端外一律
  走路径 B；`onSuccess`/`onFail` 必须同时提供。
