# AI 表格导入上传目标校验

`+import-file` 的 `uploadUrl` 来自 MCP 响应。用户确认上传不等于信任响应中的任意地址；读取本地文件后，必须在 HTTP PUT 前校验目标，拒绝时不得触发 `import_data`。

## 服务端依据

本规则核对了 `lippi-doc-notable` 提交 `9f47e404fe20560d47f09dc9042e534526ecd390`：

- `lippi-doc-notable-hsf-service/src/main/java/com/dingtalk/doc/notable/hsf/impl/provider/ImportApiServiceImpl.java` 的 `getImportUploadUrl` 生成 `notable/mcp_import_temp/<32 位小写十六进制 UUID>/<文件名>` 对象 key，并将 `Content-Length` 加入 PUT 签名。
- `lippi-doc-notable-integration/src/main/java/com/dingtalk/doc/notable/integration/oss/OssManager.java` 使用配置的 `bucketName` 和 `endpoint` 调用 OSS Java SDK，没有启用路径式 Bucket（SLD）。SDK 将 Bucket 加在 endpoint 主机前，对象 key 放在路径中。
- `lippi-doc-notable-start/src/main/resources/application-production.properties` 和 `application-staging.properties` 配置 `alidocs-notable` / `cn-zhangjiakou.oss.aliyuncs.com`；对应 `-sg` 配置使用 `alidocs-notable-sg` / `ap-southeast-1.oss.aliyuncs.com`；本地 `application.properties` 使用 `alidocs-notable-test` / `cn-zhangjiakou.oss.aliyuncs.com`。

使用本地 OSS Java SDK 3.17.4 的 `determineFinalEndpoint` / `determineResourcePath` 做了无网络校验：默认 `SLD=false`，以上三组配置均生成对应精确 Bucket 主机；空格、中文、`+`、`#`、`?` 文件名的转义与 Go 正例夹具一致。

因此只接受这三种配置对应的精确 Bucket 主机。区域根域、任何额外子域、其他 Bucket、跨区域组合及路径式 Bucket URL 均拒绝，包括把已知 Bucket 放进区域根域路径的写法。未来新增部署配置应在核实所有权及签名格式后补充精确主机和回归，不得恢复区域后缀通配。

路径校验采用 URL 解码后的对象路径，拒绝额外层级、空文件名及 `.` / `..`。校验不改写请求 URL、转义文件名或签名 query；签名有效性仍由 OSS 校验。HTTPS、无用户信息、端口 443、无 fragment 的约束同时生效。

## 连接约束与回归

上传直连且拒绝重定向。DNS 返回的所有地址必须通过 `helpers.IsPublicTransferIP`；拨号直接使用已校验的 IP，TLS 仍使用原主机校验证书。该策略与白板下载共用，显式排除 [IANA IPv4 特殊用途网段](https://www.iana.org/assignments/iana-ipv4-special-registry/) 和 [IANA IPv6 特殊用途网段](https://www.iana.org/assignments/iana-ipv6-special-registry/)，包括 `100.64.0.0/10`、基准测试、文档、NAT64 和 6to4 地址；IPv4 映射地址使用相同策略，IPv6 只接受当前 `2000::/3` 分配空间中的普通公网地址。

`import_upload_security_test.go` 覆盖确认后的拒绝流程（零 PUT、零导入）、同区域攻击者 Bucket、区域根域路径式地址、路径穿越变体、DNS 公网与特殊地址混合响应，以及三个合法主机的签名 URL 原样传递。测试使用合成文件与凭据、注入 HTTP/DNS/拨号，不上传到真实 OSS；本地契约验证不代表线上部署验收。
