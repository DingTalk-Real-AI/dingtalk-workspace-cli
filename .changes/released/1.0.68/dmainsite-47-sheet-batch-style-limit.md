---
category: Fixed
---

- **Sheet batch style limit** — `sheet range batch-set-style` 的 `--ranges` 和 `--batch` 统一最多 20 项，与服务端 `batch_update` 上限一致；超限在本地返回 validation（退出码 3），提示手动分批。同步 Help/Schema 与 Skill 说明，合法输入继续一次提交。
