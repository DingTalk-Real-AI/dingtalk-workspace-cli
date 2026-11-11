---
category: Fixed
---

- **AI 表格丢响应后的原操作恢复**：含创建的 `+record-upsert` 在发送前原子保存原 token、稳定身份、环境及目标摘要，保存失败不发送；超时后保留原恢复文件，按原 token 只读核对创建组，更新组按原 recordId 独立读回。
