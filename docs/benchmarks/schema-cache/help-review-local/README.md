# PR #1296 帮助入口复审

复审从 `f2a8b9d7` 开始，期间 PR 已前进到 `c0f3aaca`。仍为 Draft，最终发布验收未完成。

- **P1，已由 c0f3aaca 修复：声明帮助漏掉 contract 和 whiteboard。**
  `resolveVisibleProducts` 原来未读取补充端点，独立声明根因此过滤掉这两个已注册的公开命令。
  原测试先创建 runtime root，提前注入全局端点，掩盖了缺项。
  用 Go overlay 恢复 f2a8b9d7 的 `internal/app/root_help.go`，运行新的独立进程测试，
  en/zh 都失败；同一测试使用修正代码则通过。原生 CI 34009073470 的实际封装失败与此吻合。
  正确修复是在同一可见性投影中读取补充端点，保持只过滤已有 Cobra 节点，并隔离语言测试进程。
- **P2，诊断补强：构建失败后缺少实际输出。** 原 `root-help-proof.json` 只保留“不一致”文本，
  不保留预期/实际 stdout、stderr 或非零退出码。本次改动在失败时保存这些字段、完整字节数和
  SHA-256，每段最多保留 1 MiB 并标明截断；生成器错误和 diagnostics 也保留输出。
  五项封装控制测试通过，含真实子进程输出漂移、stderr、非零退出、生成器失败及 core 变更。

隔离 worktree 固定为 f2a8b9d7 加四个文件的修正，具体源码摘要见 [source.json](source.json)。
这两份 Go 源码与 c0f3aaca 相同，但候选的提交元数据仍为 f2a8b9d7；不能把本地测量称为
c0f3aaca 的精确制品证明。帮助相关 race 通过（22.358 s）。完整 runtime payload 注入与
ad-hoc 签名后的真实 core，en 4,760 字节、zh 4,766 字节，均与模型渲染完全相同。

默认 tracker 保持开启，八模式各 30 次、共 240 次交错采样全部完成。Schema CPU/RSS 两项、
帮助和版本对同包 core/不可变原始基线的八项 5% 门槛全部通过。core-free 默认版本与 en/zh
帮助验证也通过。完整样本见 [default-entry-report.json](default-entry-report.json)。

| 本机 wall p50 / p95（ms） | launcher | 原始基线 |
|---|---:|---:|
| help | 164.067 / 197.543 | 394.908 / 408.209 |
| version | 161.248 / 175.079 | 400.043 / 440.008 |

本机网络及其他负载未隔离；这些数字不能替代两平台 CI、竞争性比较、clock/attempted-I/O
审计或最终 Developer ID/notarized 安装包验收。当前 c0f3aaca 的原生 CI 为 34011126327，
本报告形成时仍在运行。所有归档文件的摘要见 [evidence.json](evidence.json)。
