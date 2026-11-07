# PR #1391 CR 核对记录

代码核对版本：`e1520768a39e414311ee99a272be058c93b7a055`。
本记录描述代码与本地回归验证，不替代维护者审批或最新 GitHub Checks。

| CR 问题 | 当前实现与验证 |
| --- | --- |
| [显式选择后续应用时丢失 ClientSecret](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pull/1391#issuecomment-5695878456) | 已有代码按选定 clientID 搜索全部候选，复用匹配密钥。`TestCrossPlatformCoverageExternalExchangeReviewExplicitLaterCandidateReusesSecret` 通过。 |
| [multipart 名称 CRLF 注入](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pull/1391#issuecomment-5709721632) | 已有代码在创建 pipe 和写入 goroutine 前校验名称。`TestCrossPlatformCoverageUploadMultipartRejectsCRLFInNames` 通过。 |
| [队列容量拒绝后去重记录导致重投丢失](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pull/1391#issuecomment-5750637543) | 已有代码在持锁状态下先检查会话和队列容量，再持久化 accepted。`TestCrossPlatformCoverageEmployeeCapacityRejectionAllowsRedelivery` 覆盖两种容量拒绝及后续重投，并通过 race 检测。 |
| [ZIP 校验后重新打开路径可能上传替换文件](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pull/1391#issuecomment-5762612280) | 本次修复让 ZIP 校验和上传使用同一打开的句柄，以 SectionReader 限制读取长度，操作结束后关闭。create/update × 普通文件/符号链接替换四项测试，修复前均上传了替换内容而失败，修复后均通过，并通过 race 检测。 |

独立复核发现打开前缺少文件类型检查会让 FIFO 阻塞；已保留打开前的非普通文件拒绝和打开后的 fstat 检查。`TestCrossPlatformCoverageEmployeeSkillFIFORejectedBeforeOpen` 修复前 3 秒超时，修复后通过；另有关闭句柄及目录句柄拒绝测试。FIFO 用例仅编译于支持该系统调用的平台；Windows 保留普通文件替换测试。

```sh
DWS_PACKAGE_VERSION=0.0.0-test go test -race ./internal/helpers -run '^TestCrossPlatformCoverageEmployee(SkillUploadKeepsValidatedFile|SkillValidationRejects.*Handle|SkillFIFORejectedBeforeOpen|CapacityRejectionAllowsRedelivery)$' -count=1 -timeout=2m
DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/auth ./internal/apiclient -run '^TestCrossPlatformCoverage(ExternalExchangeReviewExplicitLaterCandidateReusesSecret|UploadMultipartRejectsCRLFInNames)$' -count=1
DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/helpers -run '^TestCrossPlatformCoverage.*(Deap|Employee)' -count=1 -timeout=10m -covermode=atomic -coverprofile=helpers-coverage.txt
./scripts/policy/check-coverage-gate.sh --base-ref 9f755b7206ed117a4afb8c5d506fa3c9de2606c8 --changed-only --scope-buildable --diff-profile helpers-coverage.txt
```

本地 macOS arm64 / Go 1.26.1：上述测试通过；本次修复的变更代码覆盖率为 100.0000%（49 条可执行语句）。完整 Agent 和命令测试的执行命令、时间、状态见 [run.json](run.json) 与 [测试报告](README.md)。跨平台结果以该 PR 最新 SHA 的远端作业为准。

新增本地 Debug 日志锚点：`dingtalk_tag.skill_package_validated`（file_size）、`dingtalk_tag.file_upload`（stage、target_path、success、duration_ms），未记录凭据或文件内容。可按事件名检索 CLI 本地日志；这不是服务端 SLS 日志。

边界：此修复保证校验后不重新打开路径，防止该窗口内路径/符号链接替换；它不是文件字节快照，未声称解决同一 inode 上的并发原地写入。
