# 指令 CI 集成测试证据 — PR #1097（6 条危险命令 user_required 确认门禁）

测试代码 SHA：`bfdff7809304dc8698a6a9bf39440eb80ce25ad7`（2026-09-18，macOS arm64，本机 Go 工具链）。

## 复现

```sh
# calendar：event / attendee / room delete 三条确认门禁
DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/helpers -run 'TestCalendar(EventDelete|AttendeeDelete|RoomDelete)RequiresConfirmationBeforeToolCall' -count=1 -v

# chat：group members remove
DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/helpers -run 'TestChatGroupMembersRemoveRequiresConfirmationBeforeToolCall' -count=1 -v

# minutes：replace-text
DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/helpers -run 'TestMinutesReplaceTextRequiresConfirmationBeforeToolCall' -count=1 -v

# doc：permission update
DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/helpers -run 'TestDocPermissionUpdateRequiresConfirmationBeforeToolCall' -count=1 -v

# 全局同源门禁：user_required Safety 与运行时确认门禁同源（406 leaves）
DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/cli/homology -run 'TestUserRequiredSafetyHomologyWithRuntimeGate' -count=1 -v
```

每条用例验证：确认前零 MCP 远端调用；`--yes` 确认后以精确参数调用正确工具。

- `command-ci.log` — 本次运行的真实命令与输出
- `command-ci.png` — 四个产品分段 + 全局同源门禁的组合截图（Chrome 对
  `command-ci.html` 的真实页面截图，内容来自上述日志）
- `command-ci-<product>.png` — calendar / chat / minutes / doc 单产品截图
