# 发布手册（预发 / 正式）

发布只走一条链路：本地脚本负责封板、验证并推送 annotated tag；GitHub Actions 负责构建和发布最终产物。不要直接运行 `goreleaser release`，也不要手工补打或移动 tag。

发布前必须完成平台治理：目标 GitHub 仓库已启用 immutable releases，`main` 要求 `CI Gate`，操作机已安装并登录 `gh`。本地脚本会在封 tag 前通过 API 检查 immutable releases、当前 SHA 的 `CI Gate` 和在途 Release；`v*` tag ruleset 仍需仓库管理员预先配置并由操作人确认。

## 日常只用一个入口

安装发布 Skill 后直接运行：

```bash
dws-release
```

零参数会进入引导模式。仓库内的等价入口是 `./scripts/release/dws-release.sh`。第一次使用只需配置一次生产发布远端，命令会把远端名及其规范化仓库身份一起保存在当前 Git 仓库中：

```bash
dws-release config --remote origin
```

之后命令按仓库状态自动走到正确步骤：缺少精确 CHANGELOG 章节时只生成模板并停止；补全、提交并合入 `main` 后，再运行同一条命令就会安全快进本地 `main` 并执行完整预检。若同名 remote 后续被改指向其他仓库会直接拒绝。只有显式增加 `--publish` 才会进入 tag 发布，且底层仍要求最终版本确认。

## 发布模型

```text
main 上的候选代码 + beta CHANGELOG
  → vX.Y.Z-beta.N（预发验证）
  → 只允许补正式 CHANGELOG，源码不得再变化
  → vX.Y.Z（正式发布）
```

正式版必须显式指定本次验证过的 beta。脚本会比较两者：除 `CHANGELOG.md` 外只要有任何文件变化，就拒绝正式发布。这样预发测过的代码、命令树和正式发布的代码是同一份。

## 预发发布

运行统一入口：

```bash
dws-release v1.2.3-beta.1
```

如果 CHANGELOG 尚不存在，该命令只生成模板并停止。补全内容、删除所有 `TODO`，提交后通过 PR 合入 `main`；然后重新运行完全相同的命令，它会执行完整预检：

```bash
dws-release v1.2.3-beta.1
```

预检包含测试、策略检查、旧正式版命令树兼容检查、全平台打包、npm 安装验证，以及 macOS 环境下的 Homebrew 安装验证。通过后发布：

```bash
dws-release v1.2.3-beta.1 --publish
```

命令会在所有预检完成后要求再次输入完整版本号。统一入口不提供跳过确认的参数。

## 正式发布

beta 验证通过后，运行正式版入口：

```bash
dws-release v1.2.3 --from-beta v1.2.3-beta.1
```

首次运行只生成正式版 CHANGELOG 并停止。补全内容、删除 `TODO`，提交后通过 PR 合入 `main`；重新运行同一条命令做完整预检，确认后增加 `--publish`：

```bash
dws-release v1.2.3 --from-beta v1.2.3-beta.1
dws-release v1.2.3 --from-beta v1.2.3-beta.1 --publish
```

`FROM_BETA` 不会自动推断，并会写入 stable annotated tag 的 `From-Beta` 元数据，CI 会再次读取和验证。

## CHANGELOG 契约

每个 tag 必须有唯一、非空且不含 `TODO/TBD` 的精确章节：

```markdown
## [1.2.3-beta.1] - 2026-07-11

### Changed

- 本次 beta 验证的用户可见变化。
```

正式版使用 `## [1.2.3] - YYYY-MM-DD`。该章节会直接成为 GitHub Release Notes。

## CI/CD 保证

- 只接受 `vX.Y.Z-beta.N` 和 `vX.Y.Z`，且新版本必须高于上一正式版。这里的“上一正式版”必须同时具备公开非草稿 GitHub Release 和同 tag/commit 的成功 Release workflow；只有 tag、没有交付成功的孤儿版本会阻断后续发布，要求先重跑补齐。
- tag 必须是 annotated tag；本地脚本在推送前重新确认 HEAD 与远端 `main` 完全一致，CI 允许其后 `main` 前进，但要求封板提交仍位于 `main` 历史中。
- 日常 CI 和发布前都会对比“最新已交付正式版”的完整命令树；若长时间预检期间该 baseline 发生变化，会针对新的 baseline 重新比较。
- GoReleaser 只构建；Darwin 重签、checksums 重算和 npm 安装验证通过后，才统一上传 GitHub Release 的最终产物。
- 六个平台归档会逐个解包并核验二进制内嵌版本；公开资产集合、checksums 集合和 npm tarball integrity 都必须精确一致。npm tarball 固定由 npm `10.9.2` 打包，避免重跑时因 runner 自带 npm 漂移产生不同字节。
- stable 发布到 npm `latest`，更新 OSS `latest.txt` 和共享安装脚本；prerelease 发布到 npm `beta`，只更新 OSS `beta.txt`，不会覆盖稳定入口。
- Release workflow 使用一个最多容纳 100 个 pending run 的串行 publication queue；本地入口仍要求上一条 Release 完成后才能封下一个 tag。
- 本地 tag push 失败时会删除本次新建的本地 tag。tag 一旦成功推送，后续发布归 CI 所有，禁止改 tag 指向或复用版本号。

npm 补发只允许从默认分支触发 Release workflow 的 `repair_npm_version`。它只支持启用 immutable releases 后、由本流水线成功产出的公开 immutable release：目标必须是 `main` 历史中的 annotated tag，并且同 commit 的 `Build immutable GitHub Release` job 已成功。即使后续 npm 分发失败，这个独立的产物封存边界仍可作为补发依据。补发会用目标 commit 的 npm 模板重组包，逐平台核验资产和二进制版本，再发布到隔离的 `backfill` dist-tag，不会回滚 `latest` / `beta`。历史 mutable release 不进入自动补发路径，避免把可被替换的资产带入 npm。

OSS/Gitee 分发失败时直接重跑该 tag 的 `Publish npm and mirrors` failed job；各步会复用 immutable GitHub 资产并保持 channel 单调。独立 Gitee release workflow 和本地直发脚本已停用，避免绕开 publication queue 或用重新构建的不同字节覆盖镜像。

OSS 的 `latest.txt` / `beta.txt` 当前是镜像频道元数据；仓库内安装器仍从 GitHub/Gitee 解析版本，不能把 OSS pointer 当成已接入的安装通道。

Homebrew 当前只属于本机预检/手工公式通道：预检会在当前 macOS 架构真实安装，但 Release workflow 不发布 tap，CI 生成的单主机公式也不应当作 Darwin 双架构正式交付。正式自动交付范围是 GitHub Release、npm、OSS，以及显式开启时的 Gitee fallback；Homebrew 双架构 tap 发布需另立需求。

## 平台治理前置

仓库管理员还需要在 GitHub 平台配置两项不可由脚本替代的规则：

- `main` 必须要求精确的 `CI Gate`；tag workflow 也会通过 Checks API 再确认该封板 SHA 已通过。
- 必须启用 immutable releases；它只保护启用后发布的 release，因此应在第一次使用新流水线前配置。为 `v*` 增加 tag ruleset，限制创建权限，并在 release 发布前保护 tag 的短暂窗口。

immutable releases 或 `CI Gate` 缺失时，发布脚本会自动拒绝封 tag。tag ruleset 可能来自组织层，脚本不自动推断其最终作用范围；管理员确认不能省略，脚本约定也不能替代平台强制。
