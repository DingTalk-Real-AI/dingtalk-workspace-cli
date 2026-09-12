---
category: Security
---

- **AI 表格文件导入** (#1349) — 将上传白名单收紧为服务配置的精确 OSS Bucket 主机与导入对象路径，拒绝同区域其他 Bucket、区域根域和路径式 Bucket URL；复用公网传输 IP 策略，在上传前阻止共享地址段、保留地址及特殊 IPv6 目标。
