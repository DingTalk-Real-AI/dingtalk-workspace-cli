# Release fragments

普通功能、修复和面向用户的行为变更不要再修改根目录 `CHANGELOG.md` 的
`Unreleased` 区域。每个 PR 在本目录新增一个独立的 Markdown fragment，避免
并行 PR 争用同一文件。

文件名使用能唯一定位变更的短名，通常是 PR 号，例如
`1234-chat-reply-mentions.md`。文件名必须匹配
`^[a-z0-9][a-z0-9._-]*\.md$`，且必须是普通文件，不能是符号链接。本目录顶层
只接受 `README.md`、`released/` 和符合该规则的 fragment：fragment 一律平铺在
顶层，不接受任何其它子目录，本目录自身也不能被替换成文件或符号链接。其余条目
会被 CI 直接拒绝而不是忽略，以免非法条目跳过校验后拖垮下一个 PR。文件格式
严格如下：

```markdown
---
category: Added
---

- **Chat reply mentions** (#1234) — supports mentioning selected members.
```

`category` 只能是 `Added`、`Changed`、`Deprecated`、`Removed`、`Fixed` 或
`Security`。正文至少包含一个 Markdown 列表项，且不得包含 `TODO` 或 `TBD`。

发布 beta 时，`scripts/release/prepare-changelog.sh` 会按分类和文件名稳定排序，
将未归档 fragments 汇总为唯一的版本章节，并移动到
`.changes/released/<version>/`。beta 发布后若有新 fragments 合入并直接准备 stable，
stable 封板会把它们追加到明确的 post-beta 小节，并归档到正式版本目录；没有新
fragments 时仍只生成原有 beta 晋级模板。因此 release-seal PR 是唯一会修改
`CHANGELOG.md` 的 PR；它同时归档已消费的 fragments，供审计追溯。
归档只能在同一个 release-seal PR 中以原样移动完成；CI 会拒绝直接修改、
删除或重写已归档文件。

无需面向用户发布说明的改动不添加 fragment。评审者根据改动是否可见来判断该
例外是否成立。
