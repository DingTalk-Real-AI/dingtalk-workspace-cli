---
category: Fixed
---

- 数字员工 Skill 创建和更新使用同一个已打开的文件完成 ZIP 校验与流式上传，避免校验后重新打开路径时把替换文件或符号链接目标上传。上传仅读取校验时的文件大小，句柄在操作结束（包括 dry-run 和失败）后关闭。
- 补充普通文件替换和符号链接替换的 create/update 回归测试，保留打开前对命名管道等特殊文件的拒绝；增加 `dingtalk_tag.skill_package_validated`、`dingtalk_tag.file_upload` 本地诊断事件，仅记录大小、阶段、接口路径、成功状态与耗时，不记录本地路径、文件内容、身份或凭据。
