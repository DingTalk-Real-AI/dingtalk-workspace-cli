---
category: Changed
---

- Git hooks 改为通过 `make setup-hooks` 显式启用，提交前仅检查暂存区格式与空白，不修改工作区。
- 发布辅助脚本在退出时可靠清理临时目录和锁，并为下载增加有界重试、超时和阶段诊断。
