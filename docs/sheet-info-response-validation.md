# sheet info 的响应校验与回归

适用范围：本次修复后的 `dws sheet info`。该命令调用 `get_sheet` 获取表格信息，要求业务结果为非 nil 的 JSON 对象。正常空对象 `{}` 仍按既有行为输出，`nonEmptyRange` 为 `null`。

## 响应边界

`internal/helpers/sheet_info.go` 在坐标归一化前校验返回文本：空结果、JSON `null`、数组、标量、非 JSON 文本均使用仓库现有 API 错误类型，机器可读 reason 为 `MCP_TOOL_ERROR`、退出码为 1、operation 为 `sheet/get_sheet`。进程沿既有格式在 stderr 输出 JSON（`error.category=api`、`error.code=1`），stdout 保持为空。异常内容不会作为成功结果原样打印。认证、权限和业务错误仍由现有 MCP 解析与错误框架处理。命令不自动重放工具调用。

坐标归一化函数对 nil 输入安全返回；命令本身仍必须拒绝该输入，不能用 nil 保护代替响应校验。正常对象继续转换旧版 `lastNonEmptyRow` / `lastNonEmptyColumn`，或保留规范化的 `nonEmptyRange`。dry-run 不要求返回业务对象。

共享 transport 可能没有从 MCP 文本块解析出对象，runner/adapter 随后把 typed-nil map 序列化为文本 `null`。因此只测试直接注入 helper 的字符串不足以覆盖真实路径。该对象要求属于 `sheet info`，本次保留公共 transport/adapter 的其他工具语义。

## 回归入口

- `internal/helpers/sheet_info_response_test.go`：空结果、null、数组、标量、非 JSON、正常坐标、上游错误和 dry-run；测试不并行修改全局依赖。
- `internal/app/sheet_info_response_test.go`：受控本地 HTTP 响应经过真实 transport、runner、adapter、命令和错误渲染器，检查单次 `get_sheet`、结构化错误与退出码契约。
- `internal/helpers/sheet_info_include_test.go`：保留 `--include` 参数透传回归。

```sh
DWS_PACKAGE_VERSION=0.0.0-test go test -count=1 ./internal/helpers ./internal/app \
  -run 'TestCrossPlatformCoverageSheetInfo'
```

这些测试使用合成响应，可验证 CLI 的异常处理。真实服务响应及服务端根因需要服务端证据。
