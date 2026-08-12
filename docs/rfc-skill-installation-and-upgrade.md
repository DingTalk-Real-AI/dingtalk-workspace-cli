# RFC：DWS 预制 Skill 安装、升级与模式迁移

| 字段 | 内容 |
|---|---|
| 状态 | Accepted / as implemented |
| 生效范围 | DWS CLI、升级器、npm 与平台安装脚本 |
| 事实源 | 本 RFC 与当前代码；两者冲突时以代码和测试为准 |
| 关联合同 | [Skill 内容框架](skill-content-framework.md)、[Mono↔Multi 内容质检](skill-mono-multi-qa.md) |

## 1. 背景

DWS 同时通过 CLI、升级器、npm、Shell 和 PowerShell 分发预制 Skill。multi
成为默认布局后，所有入口必须对安装集合、模式互斥、失败退出、缓存发布和目录
所有权保持一致。此前分散的调研、迁移计划、阶段性 roadmap 和 rollout 文档容易
相互冲突；本 RFC 将最终行为收敛为一个长期合同。

## 2. 目标与非目标

### 2.1 目标

- 新装与升级默认使用 multi 布局，mono 在兼容期内保留显式 opt-in。
- 每次升级使用当前版本的官方清单全量覆盖预制 Skill。
- 删除或替换任何目录前先创建可恢复备份，备份失败不修改该 Agent 目标。
- 只清理能够证明由 DWS 管理的目录，不通过名称前缀推断所有权。
- 所有安装入口对部分失败返回非零状态，不误报整体成功。
- 安装预览、确认和实际执行使用同一份计划。

### 2.2 非目标

- 不建设独立的 `dws skill mode status|set|rollback` 产品面。
- 不持久化用户对预制 Skill 的本地删除或排除意图。
- 不提供跨所有 Agent 目标的事务式回滚。
- 不把市场 Skill 纳入预制 Skill 的升级和清理范围。

## 3. 业内调研

对主流 CLI 与 Agent Skill 分发方式的公开实现进行归纳后，可以得到以下共性：

| 观察 | 对 DWS 的启示 |
|---|---|
| 多个产品能力通常以同级 Skill 目录安装，由 Agent 按目录发现 | multi 使用平铺的产品 Skill，并保留一个共享 Skill 承载公共协议 |
| CLI 本体安装和 Agent Skill 安装是两个生命周期 | DWS 可以在 CLI 安装、setup 和 upgrade 中触发 Skill 同步，但二者的失败与状态必须分别报告 |
| 生态安装器通常天然采用 multi，不提供 mono/multi 状态机 | DWS 的模式切换保持为重新执行 setup，不新增长期驻留的 mode lifecycle |
| 市场 Skill 与 CLI 预制 Skill 可能落在同一 Agent 根目录 | 必须使用统一所有权元数据识别受管目录，名称前缀不能作为删除依据 |
| 多 Skill 更新常以新清单刷新官方集合 | DWS 使用当前 bundle 官方清单全量覆盖，新增 Skill 自动加入，本地删除不视为持久化排除 |
| 制品可能需要同时服务无运行时依赖、离线和多镜像环境 | DWS 保留 embed、zip 和平台安装脚本，不把单一生态包管理器设为唯一入口 |
| 中断的复制和原地覆盖容易破坏最后一个可用版本 | 缓存与 Go upgrade 的 Agent 目标采用 staging publish；发布失败自动恢复该目标的完整旧集合 |
| Agent 通常以 `SKILL.md` 为入口，其他文件按引用或工具规则按需读取 | 安装元数据使用不被内容引用的隐藏文件，并保证其内容不包含 Agent 指令 |

本节只保留可复用的工程结论，不记录具体产品、仓库、版本或逐项能力对照，也不构成
DWS 对任何外部实现的持续兼容义务。后续设计以 DWS 自身约束和本 RFC 的行为合同为准。

## 4. 布局合同

| 模式 | Agent 目录布局 | 选择方式 |
|---|---|---|
| multi（默认） | `<agent-home>/dingtalk-*/` 与必选 `dingtalk-shared/` | 默认；`dws skill setup --mode multi` |
| mono（兼容） | `<agent-home>/dws/` | `dws skill setup --mode mono` 或安装器的 mono opt-in |

模式切换通过重新执行 setup 完成。安装 multi 前备份并移除 mono 的 `dws/`；安装
mono 前只备份并移除能够证明由 DWS 管理的 multi 目录。两个方向都不提供隐式、
不可恢复的删除。

## 5. 官方集合与升级策略

当前版本 bundle 中的 multi 目录清单是升级集合的唯一权威来源。普通 upgrade 和
`--force` 都安装并覆盖该版本的全部官方预制 Skill：

- 本地删除的预制 Skill 会在下一次升级恢复；
- setup 时通过 `--exclude` 暂时排除的 Skill 会在下一次升级恢复；
- 新版本新增的官方 Skill 会自动安装；
- 用户对预制 Skill 的本地修改会被官方版本覆盖；
- `dingtalk-shared` 始终随官方集合安装。

`~/.dws/skills-state.json`（设置 `DWS_CONFIG_DIR` 时位于该目录）不参与安装集合
求解，也不保存排除策略。它既记录结果快照，也集中记录 multi Skill 的所有权和
provenance，供安全清理、诊断与后续迁移使用。

## 6. 目录所有权

每次 multi setup 或 upgrade 全部成功后，DWS 在统一的
`~/.dws/skills-state.json` 中写入：

```json
{
  "version": "v0.2.14",
  "official_skills": ["dingtalk-aitable"],
  "updated_skills": ["dingtalk-aitable"],
  "managed_skills": [
    {
      "name": "dingtalk-aitable",
      "version": "v0.2.14",
      "source": "dws-upgrade",
      "digest": "sha256:<64 个十六进制字符>",
      "digest_scope": "skill-directory-v1"
    }
  ],
  "updated_at": "2026-08-11T12:34:56Z"
}
```

每条 `managed_skills` 记录代表一个由 DWS 管理的官方 Skill。`version` 记录安装该
副本的 DWS/发布包版本，`source` 记录安装入口，`digest` 是对 bundle 中 Skill 目录
全部普通文件按相对路径排序后计算的内容摘要。摘要用于诊断和来源追踪，不作为后续
升级的完整性门禁；用户修改 Skill 内容后，DWS 仍保有明确管理权并能在下一次升级时
覆盖恢复。

清理 stale Skill 或切换到 mono 时，只接受以下所有权证据：

1. Skill 名称存在于统一状态的 `managed_skills` 中；
2. 统一状态上线前曾发布过的官方 Skill 精确名称集合。

历史集合是冻结的迁移清单，包含 `dws-shared` 以及已退役、折叠或仍在发布的旧官方
目录名。仅有 `dingtalk-*` 前缀不构成所有权证据。因此，市场或用户创建的
`dingtalk-custom` 等非官方精确名称目录不会被迁走。

### 6.1 对 Agent 的影响

Skill 目录内不再放置 DWS 所有权文件，也不增加非通用 frontmatter 字段。支持的
Agent 仍只需以 `SKILL.md` 发现和加载 Skill；统一元数据位于 Agent Skill 目录之外，
不会成为提示词上下文或影响 Agent 行为。

## 7. Setup：Plan → Confirm → Execute

`dws skill setup` 分为三个阶段：

1. **Plan**：只读计算目标、安装集合以及所有待备份路径；
2. **Confirm**：`--dry-run` 和交互确认渲染同一份计划；
3. **Execute**：确认后严格执行计划中的备份和安装。

安全要求：

- 非交互环境未传 `--yes` 时拒绝执行；
- 用户拒绝确认时必须零文件写入；
- 备份失败时跳过整个 Agent 目标，不开始铺设相反布局；
- 同一目标先完成所有必要备份，再复制新集合；
- multi Skill 必须在同级 staging 中完成复制，再原子发布到正式目录；
- 任意 `skipped > 0` 都返回非零退出码，并且不写入完整成功快照；
- 一个 Agent 目标失败不阻止其他目标尝试，但最终结果仍为失败。

## 8. Upgrade 与恢复语义

升级器对每个 Agent 目标执行：

1. 只读计算对面布局、过期受管 Skill 和同名官方 Skill；
2. 在目标文件系统的 staging 中复制完整新集合；
3. staging 全部成功后，才将旧集合移入备份目录；
4. 逐项发布 staging；任一发布失败时删除已发布的新目录，并逆序恢复该目标的全部旧目录；
5. 仅在没有目标失败且至少一个目标成功时更新状态快照。

Go upgrade 当前提供 **单 Agent 目标级事务恢复**：复制失败发生在旧目录移动前；
备份中途失败会恢复此前已移动的目录；发布中途失败会恢复该目标的完整旧集合。不同
Agent 目标仍彼此独立，一个目标失败不会回滚此前已经成功升级的其他目标，这与
“不提供跨所有 Agent 目标的事务式回滚”非目标保持一致。

## 9. 备份合同

- 路径：`~/.dws/skill-backups/<UTC 时间戳>/...`；
- 主要操作：同一文件系统内使用 rename 移动；
- 失败语义：备份失败时原目录保持不变，目标安装失败；
- 可见性：计划和执行日志显示原路径与备份路径；
- 保留策略：自动修剪，仅保留最近 5 批。

备份是安装安全机制，不等于独立 rollback 产品。需要切回 mono 时重新运行
`dws skill setup --mode mono`。

## 10. 缓存与制品

发布制品和二进制内嵌内容同时携带 mono 与 multi 源树。`~/.dws/skills/` 只是
setup 在未显式指定 `--source` 时的本地回退缓存。

缓存刷新必须采用同级 staging + publish：

1. 在 staging 中完整复制并验证新树；
2. 发布前保留旧缓存；
3. 通过 rename 发布新缓存；
4. 复制或发布失败时保留或恢复旧缓存；
5. 空、缺失或损坏的 bundle 不能擦除有效缓存。

## 11. 安装入口一致性

以下入口都遵守本 RFC：

| 入口 | 默认模式 | 失败合同 |
|---|---|---|
| `dws skill setup` | multi | 部分失败返回非零；不写完整成功状态 |
| `dws upgrade` | bundle 含 multi 时安装 multi | 目标失败返回失败；下次全量重试 |
| `scripts/install.sh` | multi | 任一检测到的目标失败则脚本非零 |
| `scripts/install.ps1` | multi | 任一检测到的目标失败则脚本非零 |
| `scripts/install-skills.sh` | multi | 任一检测到的目标失败则脚本非零 |
| npm `install.js` | multi | 任一检测到的目标失败则 postinstall 失败 |

Homebrew 不直接向 Agent home 铺设 Skill；安装 CLI 后由 setup 执行相同流程。

## 12. 验收与回归门禁

合入和后续修改至少覆盖：

- mono → multi、multi → mono 互斥切换；
- 状态上线前的官方 multi 目录切换 mono 时能够被精确迁移；
- 未登记的同前缀市场/用户 Skill 在刷新和切换后仍存在；
- 统一状态中登记的过期官方 Skill 被备份并移除；
- 备份、复制、统一状态写入、缓存 publish 故障注入；
- 非交互确认拒绝与显式 `--yes`；
- 部分失败返回非零且不写错误状态快照；
- 复制失败不留下 Agent 可见的残缺官方目录；
- 普通 upgrade 恢复被删除的预制 Skill，并安装新增官方 Skill；
- Windows、macOS、Linux 的路径和覆盖率门禁；
- npm、Shell、PowerShell 与包管理器安装冒烟。

## 13. 后续演进

- 收敛各安装入口中的 Agent home 清单，减少跨语言复制；
- 如确有运维需求，可单独设计备份查看和显式恢复命令；
- mono 的物理删除必须作为独立变更，在 multi 内容、安装入口和迁移回归稳定后推进；
- `managed_skills` 字段若演进，必须同步更新所有安装入口和跨平台回归。
