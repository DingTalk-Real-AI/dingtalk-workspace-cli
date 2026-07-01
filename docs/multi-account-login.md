# 钉钉同组织多账号登录说明

本文说明本次新增的本地多账号能力、涉及的 CLI 命令，以及如何登录、切换和退出账号。

## 新增能力

`dws` 现在用 `profileId` 作为本地账号标识。新的钉钉登录在拿到用户信息后会生成：

```text
profileId = corpId:userId
```

因此同一台机器可以保存同一个钉钉组织下的多个账号。例如：

```text
ding7779cf9da65ca5ea:226841201924277381
ding7779cf9da65ca5ea:011352590165863362195
```

`corpId` 仍然兼容，但只有在本地唯一匹配一个 profile 时才建议使用。如果同一个 `corpId` 下有多个账号，请使用完整 `profileId` 或唯一 profile 名。

## 相关命令

### 登录

```bash
dws auth login --recommend
```

本地开发时如果要使用当前源码构建出的二进制：

```bash
./dws auth login --recommend
```

`--recommend` 是正确拼写；不要写成 `--recommand`。

### 查看已登录账号

```bash
dws profile list --format json
```

输出中重点看：

```json
{
  "profileId": "dingxxx:user123",
  "corpId": "dingxxx",
  "userId": "user123",
  "userName": "张三"
}
```

### 切换默认账号

```bash
dws profile switch 'dingxxx:user123'
```

兼容命令：

```bash
dws profile use 'dingxxx:user123'
```

也可以使用唯一的 profile 名或唯一 `corpId`：

```bash
dws profile switch "钉钉"
dws profile switch dingxxx
```

如果 `corpId` 对应多个账号，会产生歧义，请改用完整 `profileId`。

### 单次命令指定账号

不想修改默认账号，只想让一次业务命令使用某个账号：

```bash
dws --profile 'dingxxx:user123' contact user get-self
```

同一个命令也可以对多个账号执行：

```bash
dws --profile 'dingxxx:user123,dingxxx:user456' contact user get-self
```

### 查看指定账号认证状态

```bash
dws auth status --profile 'dingxxx:user123'
```

### 退出指定账号

```bash
dws auth logout --profile 'dingxxx:user123'
```

不带 `--profile` 时会退出本机所有已登录 profile：

```bash
dws auth logout
```

## 同组织登录两个账号的流程

1. 登录第一个账号：

```bash
dws auth login --recommend
```

2. 在浏览器或钉钉授权页切换到第二个钉钉账号。

3. 再登录一次：

```bash
dws auth login --recommend
```

4. 查看结果：

```bash
dws profile list --format json
```

期望看到同一个 `corpId` 下多个不同 `profileId`。

## 存储与隔离

非敏感 profile 元数据保存在：

```text
profiles.json
```

其中：

```text
primaryProfile/currentProfile/previousProfile = profileId
profiles[].profileId = corpId:userId
```

token 材料保存在 profile 维度的 keychain slot：

```text
dingtalk-workspace-profile-<profileId>
```

账号相关的 runtime/tools/detail 缓存按 profile 隔离：

```text
<edition-partition>/profile/<profileId>
```

全局 registry 缓存仍按 edition 共享，不按账号重复保存。

## 旧数据兼容

旧版本可能已经存在 `profileId = corpId` 的本地数据。新版加载 profile 时，如果该 profile 已有 `userId`，会自动升级为：

```text
corpId:userId
```

因此通常不需要手动删除本地配置。
