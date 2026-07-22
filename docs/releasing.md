# 发布手册（预发 / 正式）

发布只走一条受控链路：GitHub Actions 的 `Release` workflow 负责版本分配、封板、构建、签名和下游发布；Homebrew 以 workflow 自动创建的 Formula PR 经独立审核合入为交付边界。本地 `dws-release` 仍是兼容入口，但不再要求某一台固定电脑承担打包；不要直接运行 `goreleaser release`，也不要手工补打、移动或复用 tag。

发布前必须完成平台治理：目标 GitHub 仓库已启用 immutable releases，`main` 精确要求 `CI` workflow 的九个 context：`Lint`、`Test`、`Coverage`、`Policy`、`Edition`、`Interface Integrity`、`AI Behavior`、`CLI Smoke`、`Mock MCP`。云端和本地入口都会在封 tag 前检查 immutable releases、当前 SHA 的全部九个 context 和在途 Release；`v*` tag ruleset 仍需仓库管理员预先配置。

## 推荐入口：GitHub 云端发布

任何具有仓库写权限、因而可以手动运行 Actions workflow 的成员，都可以基于当时最新的 `main` 发起发布：

1. 在 GitHub Actions 打开 `Release`，选择 `Run workflow`，分支必须是默认分支 `main`。
2. `release_operation=plan`，选择 `release_channel=beta|stable`；仅在开始新 beta 线时选择 `release_bump=patch|minor|major`。
3. workflow summary 会给出唯一的下一版本。把对应的精确 `CHANGELOG.md` 章节通过 PR 合入 `main`。
4. 再次运行，改为 `release_operation=publish`，并输入 `PUBLISH beta` 或 `PUBLISH stable`。

`plan` 是纯只读操作，不创建 tag、预留版本号或生成包。CHANGELOG 合入期间若另一个发布先占用了该版本，`publish` 会重新分配并因 CHANGELOG 章节不匹配而拒绝，需要重新 plan。`publish` 会先再次确认 dispatch SHA 仍是当前 `main`、Code Admission 和平台治理均通过，再由唯一的 write job 使用 GitHub API 原子创建 annotated tag；同一次 run 随即进入既有的跨平台构建、GitHub/npm、可选 OSS/Gitee 发布和 Homebrew PR DAG。内置 `GITHUB_TOKEN` 创建的 tag 不依赖第二条 workflow 被再次触发。

OSS 镜像默认不参与发布 DAG，适用于尚未创建 Bucket 的仓库。云端封板会把当时的仓库变量 `ENABLE_OSS_MIRROR=true` 记录为不可变 tag 元数据 `OSS-Mirror: enabled`，否则记录为 `deferred`；后续发布和撤回只读取该 sealed policy，不读取变量的当前值。`enabled` 继续对缺失凭据、无效 Bucket、上传、pointer 和撤回失败保持 fail-closed；`deferred` 明确跳过不存在的渠道。为避免补发后撤回遗漏，deferred 版本暂不接受 `repair_oss_version`，启用 OSS 只影响后续新 tag，直到补齐可审计的不可变 repair 证明。

## 自动版本规则

- beta：如果存在尚未封正式版的最高版本线，自动取 `beta.N+1`；否则从最新已分配正式版按所选 patch/minor/major 开新线并取 `beta.1`。
- stable：先锁定最高开放版本线上的最新已分配 beta，再要求它已成功交付且未撤回；不会跳过失败/撤回的最新 beta 去选择更早版本。正式版 core 与该 beta 完全相同。
- `vX.Y.Z`、`vX.Y.Z-beta.N` 一经分配就永久占用。撤回时创建 `withdrawn/v...` 墓碑，原编号永不复用。
- 例如撤回 `v1.0.53-beta.5` 后，下一 beta 是 `v1.0.53-beta.6`；撤回正式版 `v1.0.53` 后，下一 patch 修复线是 `v1.0.54-beta.1`，验证后再发布 `v1.0.54`。
- 如果最新 beta 已撤回，禁止直接用更早 beta 晋级正式版；必须先构建下一个 beta。

## 全平台撤回与回滚

已公开版本出现问题时，在 GitHub Actions 运行 `Withdraw release`，分支必须选择当前默认分支 `main`，并填写：

- `version`：精确版本，例如 `v1.0.53` 或 `v1.0.53-beta.5`。
- `reason`：8–300 字符的单行公开原因。
- `confirmation`：精确输入 `WITHDRAW <version>`，例如 `WITHDRAW v1.0.53`。

该 workflow 使用与发布相同的串行 publication lock，并进入受保护的 `release-withdrawal` environment。它只接受已经由 Release workflow 完整交付的 public immutable release，自动选择同一渠道中最新的、更早且未撤回的完整版本作为回退目标，然后按以下顺序执行：

1. 先创建永久 annotated tag `withdrawn/<version>`，记录原 tag object、commit、原因、申请人和 workflow run。这个墓碑是版本号永久占用记录，永不移动、永不删除。
2. 先验证 Homebrew Formula；若它仍指向问题版本，先创建回退 PR，再继续其他渠道撤回。这样 PR 创建失败时只留下可安全续跑的墓碑，不会先造成渠道分裂。若 Formula 尚未指向问题版本或已经处于安全版本，则直接校验。
3. GitHub Release 先标记为 withdrawn；npm 精确版本执行 `deprecate`，并把 `latest` / `beta` dist-tag 回退；只有目标 tag 封存了 `OSS-Mirror: enabled` 时，OSS 才会先补齐回退版本资产，再移动 `latest.txt` / `beta.txt` 并删除问题版本目录；启用 Gitee 时同样先补齐回退 Release，再删除问题 Release 和 tag。
4. npm 以及目标 tag 启用或发布时配置的镜像渠道均已验证安全后，删除 GitHub 上的问题 Release 和原 `v...` tag，并验证 `/releases/latest` 对正式版回到安全版本。若本次创建了 Homebrew PR，run 最后故意保持失败，直到另一名维护者审核合入；合入后，从新的 `main` 使用完全相同的 version、reason 和 confirmation 重跑并完成。永久 `withdrawn/v...` 墓碑始终保留。

GitHub、npm、OSS、Gitee 和 Homebrew 的“回滚”指新的安装、升级和渠道解析不再拿到问题版本。已经装到用户电脑上的二进制无法被服务端强制降级；用户必须重新安装回退版本、安装后续修复版，或使用 CLI 自带的本地 rollback 能力。npm 不执行 `unpublish`：问题版本保留明确的弃用警告，但 `latest` / `beta` 不再指向它；即使 registry 允许删除，已发布过的版本号也不会重新使用。

撤回前必须存在同一渠道中更早、完整交付且未撤回的安全版本；若目标是该渠道第一个版本、没有安全候选，workflow 会在创建墓碑或修改任何渠道前 fail closed，需要先决定明确的替代策略。CLI 本地 rollback 也只有在本机仍保留上一次升级备份时可用。

撤回以“精确版本”为单位，不会因为正式版曾由某个 beta 晋级就隐式级联修改另一个渠道。若同一缺陷同时存在于正式版及其 beta，应先撤回正式版，再撤回对应 beta，并分别使用各自的精确确认串；每次都只会把该渠道回退到自己的安全候选。

撤回正式版 `v1.0.53` 后，`v1.0.53` 仍被墓碑视为已分配。下一次 patch 发布从 `v1.0.54-beta.1` 开始，验证后晋级 `v1.0.54`。撤回 `v1.0.53-beta.5` 后，同一开放版本线继续为 `v1.0.53-beta.6`；不会退回或复用 `beta.5`。

## 兼容入口：本地发布

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
  → 补正式 CHANGELOG；允许继续通过 PR 合入新 commit
  → vX.Y.Z（正式发布，封板提交必须包含该 beta 提交）
```

云端入口自动选择本次最新、已交付且未撤回的 beta；本地入口必须显式指定。流水线要求该 beta 已成功交付、未撤回，且 beta 提交必须位于正式发布封板提交的历史中——不能跳过 beta 直接发正式版，但允许在 beta 之后把经过 review 合入 `main` 的 commit 一起发布。

## 预发发布

运行统一入口：

```bash
dws-release v1.2.3-beta.1
```

如果 CHANGELOG 尚不存在，该命令只生成模板并停止。补全内容、删除所有 `TODO`，提交后通过 PR 合入 `main`；然后重新运行完全相同的命令，它会执行完整预检：

```bash
dws-release v1.2.3-beta.1
```

预检包含测试、策略检查、旧正式版命令树兼容检查、全平台打包、npm 安装验证，以及 macOS 环境下的 Homebrew 安装验证。它还会从默认分支触发一次无发布权限的 `Release governance preflight`，用正式流水线相同的身份检查该精确 commit 的九个 Code Admission context 和 immutable releases。通过后会在当前 Git worktree 的私有 Git 状态目录写入一个有效期六小时的证明，绑定版本、精确 commit、发布仓库、beta/stable 基线和远端 `main`：

```bash
dws-release v1.2.3-beta.1 --publish
```

若源码、版本、远端身份和 stable 基线均未变化，`--publish` 会复用该证明，只执行远端契约、发布身份和最终治理复核，不再重复测试与打包。也可以直接运行 `--publish`；没有可复用证明时只会完整执行一次预检。命令在封 tag 前仍要求再次输入完整版本号，统一入口不提供跳过确认的参数。

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

本地入口的 `FROM_BETA` 不会自动推断；云端入口会按上述规则唯一选择。两种入口都会把它写入 stable annotated tag 的 `From-Beta` 元数据，CI 会再次读取和验证。

## CHANGELOG 契约

每个 tag 必须有唯一、非空且不含 `TODO/TBD` 的精确章节：

```markdown
## [1.2.3-beta.1] - 2026-07-11

### Changed

- 本次 beta 验证的用户可见变化。
```

正式版使用 `## [1.2.3] - YYYY-MM-DD`。该章节会直接成为 GitHub Release Notes。

## CI/CD 保证

- 只接受 `vX.Y.Z-beta.N` 和 `vX.Y.Z`，且新版本必须高于上一正式版。这里的“上一正式版”必须同时具备公开非草稿 GitHub Release 和同 tag/commit 的成功 Release workflow；只有 tag、没有交付成功的孤儿版本会阻断后续发布，要求走受保护恢复补齐。云端 tag 会固定 `Release-Run`、requester、commit 和版本分配指纹，交付验证按该精确 run/attempt 及完整 job graph 取证，不接受任意 `workflow_dispatch`。历史版本若曾通过专用 recovery workflow 完成交付，只能使用仓库内 `delivered-stable-recoveries.json` 中精确到 tag、commit、run、workflow SHA 与 attempt 的 reviewed 证据。
- tag 必须是 annotated tag；本地脚本要求封板提交已通过 PR 合入并包含在远端 `main` 历史中，发布只推送 tag。CI 允许其后 `main` 继续前进，但始终要求封板提交位于 `main` 历史中。
- 日常 CI 和发布前都会对比“最新已交付正式版”的完整命令树；若长时间预检期间该 baseline 发生变化，会针对新的 baseline 重新比较。
- GoReleaser 只构建；Darwin 重签、checksums 重算和 npm 安装验证通过后，才统一上传 GitHub Release 的最终产物。
- 六个平台归档会逐个解包并核验二进制内嵌版本；公开资产集合、checksums 集合和 npm tarball integrity 都必须精确一致。npm tarball 固定由 npm `10.9.2` 打包，避免重跑时因 runner 自带 npm 漂移产生不同字节。
- stable 发布到 npm `latest`；prerelease 发布到 npm `beta`。启用 `ENABLE_OSS_MIRROR=true` 后，stable 同步 OSS `latest.txt` 和共享安装脚本，prerelease 只同步 OSS `beta.txt`，不会覆盖稳定入口。
- Release workflow 使用一个最多容纳 100 个 pending run 的串行 publication queue；版本规划、云端封板、发布、恢复、修复和撤回共享同一发布锁。
- 本地 tag push 失败时会删除本次新建的本地 tag。远端 tag 一旦创建，后续发布归 CI 所有；发布中途失败时走受保护恢复，禁止改 tag 指向或复用版本号。只有已经公开版本经过受保护的全渠道撤回并留下永久 `withdrawn/...` 墓碑后，撤回 workflow 才会在最后一步删除原 tag。

npm 补发只允许从默认分支触发 Release workflow 的 `repair_npm_version`。它只支持启用 immutable releases 后、由本流水线成功产出的公开 immutable release：目标必须是 `main` 历史中的 annotated tag，并且同 commit 的 `Build immutable GitHub Release` job 已成功。即使后续 npm 分发失败，这个独立的产物封存边界仍可作为补发依据。补发会用目标 commit 的 npm 模板重组包，逐平台核验资产和二进制版本，再发布到隔离的 `backfill` dist-tag，不会回滚 `latest` / `beta`。历史 mutable release 不进入自动补发路径，避免把可被替换的资产带入 npm。

已启用的 OSS 或 Gitee 分发失败且 GitHub immutable Release、npm 已交付时，从受保护的默认分支触发
Release workflow，并且只填写 `repair_oss_version` 或 `repair_gitee_version` 之一。channel
repair 会精确绑定失败 tag run 的最新 attempt，且 OSS repair 要求 tag 的 sealed policy 为 `enabled`；contract、构建、Developer ID 签名、
immutable GitHub 发布和 npm delivery 必须全部成功，且只能有一个 OSS/Gitee 下游失败，
随后才会下载并重新校验原始资产、修复所选镜像。OSS repair 必须匹配失败的 OSS step；
Gitee repair 还允许其 job 因该 OSS 失败而 skipped，此时只代表 Gitee backfill 成功，
不会把仍未修复的 OSS 标成成功。该证据不能用于 beta → stable 或
stable baseline，后两者仍要求整条 Release 成功或受保护 recovery 成功。不要重跑旧
attempt 的单个 failed job，以免在 attempts 之间拼接交付证据。独立 Gitee release
workflow 和本地直发脚本已停用，避免绕开 publication queue 或用重新构建的不同字节覆盖镜像。

## 既有 tag 的紧急恢复

云端封板或本地 tag push 已成功、但 Release workflow 失败且 GitHub Release 尚未公开时，不要新建临时 workflow、移动 tag 或跳过门禁。在最新且干净的 `main` worktree 运行：

```bash
dws-release recover v1.2.3-beta.1
```

命令会自动解析 annotated tag object、peeled commit，以及 tag 绑定的失败云端 run 或最近一次匹配的失败 tag-push run；也可以用 `--failed-run <run-id>` 精确指定。确认完整版本号后，它从默认分支触发受保护的恢复模式并等待完成。恢复模式必须满足：

- 输入精确绑定原 annotated tag object、commit 和失败的 sealed `Release` run；云端 run 还必须与 tag 内的 run ID、attempt、requester 完全一致，commit 必须仍在 `main` 历史中。
- 目标只允许不存在 GitHub Release 或仍为 Draft；已经公开的版本不能全量重建：单个下游故障走对应的 channel repair，版本本身有问题则走受保护的全平台 withdrawal。
- `release-recovery` environment 必须限制为受保护分支、配置至少一名 required reviewer，并禁止自审；workflow 会通过 API 复核这些设置，未配置时 fail closed。
- 恢复复用正常的 contract、构建、Developer ID 签名、资产校验、immutable 发布、Homebrew、npm，以及已启用的 OSS jobs，不存在 recovery 专用 publisher 或门禁跳过。
- 如果 GitHub Release 已在 recovery 中封存、后续 Homebrew/npm 校验发生瞬时失败，只重跑该 run 的 failed jobs；流水线仅在隐藏 run marker、tag object、commit 和 finalized artifact 字节全部精确一致时复用公开 Release。

成功的默认分支恢复 run 会成为后续 beta → stable 和 stable baseline 验证的可审计交付证据；历史临时分支恢复仍只接受 reviewed manifest 中的固定证据。

云端 seal 后不要使用 GitHub 的 “Re-run failed jobs” 作为交付修复：annotated tag 永久绑定最初的 run attempt，普通 rerun 不会成为可接受的交付证据。GitHub Release 尚未公开时走上述 protected recovery；已经公开且仅 npm/OSS/Gitee 某一渠道失败时走对应 repair；版本内容本身有问题时走 withdrawal。

OSS 的 `latest.txt` / `beta.txt` 是镜像频道元数据；当前仓库安装器仍主要从 GitHub/Gitee 解析版本。启用 OSS 后，发布和撤回把它作为受控分发渠道处理，保证一旦外部消费者接入该 pointer，也不会继续解析到已撤回版本；未启用时两条流程都明确跳过不存在的 OSS 渠道。

Release workflow 会生成 Darwin/Linux 双架构 Formula，并分别为 stable/beta 打开 Homebrew PR；tap 的默认分支仍以独立审核合入为交付边界。撤回 workflow 使用相同模板和回退版本 checksums 打开反向 PR；问题 GitHub Release 会先被移除以阻止新安装，永久墓碑和 workflow 日志承担审计/续跑依据。

## 平台治理前置

仓库管理员还需要在 GitHub 平台配置以下不可由脚本替代的规则：

- `main` 必须精确要求 `Lint`、`Test`、`Coverage`、`Policy`、`Edition`、`Interface Integrity`、`AI Behavior`、`CLI Smoke`、`Mock MCP` 九个 Code Admission context；tag workflow 也会通过 Checks API 再确认该封板 SHA 上九项全部成功。
- 必须启用 immutable releases；它只保护启用后发布的 release，因此应在第一次使用新流水线前配置。为 `v*` 增加 tag ruleset，限制创建权限，并在 release 发布前保护 tag 的短暂窗口。
- tag ruleset 还必须覆盖 `withdrawn/v*`：只允许受保护的撤回 workflow 创建墓碑，禁止更新或删除墓碑；同时应允许 Release workflow 创建新的 `v*`，允许撤回 workflow 在全部渠道回退后删除精确的问题 `v*`。若组织级规则阻止这两个 workflow 的预期动作，发布或撤回会 fail closed，不能靠手工移动 tag 绕过。
- 配置 `RELEASE_GOVERNANCE_TOKEN` Actions secret，只授予目标仓库 `Administration: read`；内置 `GITHUB_TOKEN` 不具备 immutable-releases API 所需的仓库治理权限。每次本地预检和 tag workflow 都使用这一个身份进行 fail-closed 验证。
- 配置 `APPLE_CERTIFICATE_P12_BASE64`、`APPLE_CERTIFICATE_PASSWORD` 和具备发布权限的 `NPM_TOKEN`；撤回还要求该 npm 身份能够执行 `deprecate` 和修改 dist-tag。
- 启用 OSS 镜像时，先创建有效 Bucket，再设置仓库变量 `ENABLE_OSS_MIRROR=true`，并配置 `OSS_ACCESS_KEY_ID`、`OSS_ACCESS_KEY_SECRET`、`OSS_ENDPOINT`、`OSS_BUCKET`，按需配置 `OSS_PREFIX`。启用后发布保持 fail-closed；撤回身份必须能够补齐安全版本资产、写 `latest.txt` / `beta.txt` 并删除问题版本前缀。尚未 provision Bucket 时保持该变量未设置或不等于 `true`，新 tag 会封存 `OSS-Mirror: deferred` 并跳过 OSS；该版本不能通过现有 repair 流程事后改成启用。
- 若启用 Gitee fallback，设置 `ENABLE_GITEE_UPLOAD_FALLBACK=true`，并配置 `GITEE_TOKEN`、`GITEE_USER`、`GITEE_REPO`；该身份必须能够创建和删除目标仓库的 Release 与 tag。
- 单独配置 `HOMEBREW_PR_TOKEN`，优先使用仅授权本仓库且具备 `Contents: write`、`Pull requests: write` 的 fine-grained PAT；若组织策略不允许该账号使用 fine-grained PAT，则回退到仅带 `public_repo` scope 的专用 classic PAT。治理预检和 tag contract 会验证 token 身份、classic scope，并用 `[skip ci]` 临时分支和 draft PR 完成真实写权限 canary，随后立即关闭 PR、删除分支；任何清理失败都会 fail closed。门禁也会拒绝与治理 token 复用。
- 创建 `release-recovery` environment，只允许受保护分支，设置 required reviewer、禁止自审并关闭管理员绕过。workflow 会读取 environment 的 required-reviewer、prevent-self-review 和 protected-branch 规则；规则缺失时紧急恢复会失败，正常 beta/stable tag 发布不受影响。
- 创建 `release-withdrawal` environment，只允许受保护分支，设置至少一名 required reviewer、禁止申请人自审并关闭管理员绕过。撤回 workflow 会通过 API 复核这些规则；任何一项缺失都会在触碰 npm、OSS、Gitee、Homebrew 或 GitHub Release 前失败。
- 仓库或组织的 Actions 策略必须允许 `Release` 与 `Withdraw release` workflow 的 `GITHUB_TOKEN` 获得各 job 声明的 `contents: write`。若上述发布凭证采用 environment secret，确认 `release-withdrawal` 审批完成后能够读取撤回所需的 npm、OSS、Gitee 和 Homebrew 凭证。

immutable releases，或任一 Code Admission context 缺失、未成功时，发布脚本会自动拒绝封 tag。tag ruleset 可能来自组织层，脚本不自动推断其最终作用范围；管理员确认不能省略，脚本约定也不能替代平台强制。
