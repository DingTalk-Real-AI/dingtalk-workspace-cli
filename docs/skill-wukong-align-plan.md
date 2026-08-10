# DWS multi-skill **内容框架**对齐方案（相对 dws-wukong develop）

> 状态：**执行中** — Phase 1–3 已落地；M2/M3 已补；**M1 recovery 闭环已从 skill 删除（不做移植）**。  
> 合同短文：[skill-content-framework.md](skill-content-framework.md)  
> 质检规格：[skill-mono-multi-qa.md](skill-mono-multi-qa.md)  
> 机读合同：`skills/content-qa/mono-multi-coverage.yaml`  
> 门禁：`make skill-mono-multi-content`（独立门禁；默认 `make policy` 按设计不包含该检查）
>
> 撰写 / 收窄 / 质检增补 / 执行：2026-08-05  
> 工作树：`/Users/john/GolandProjects/open-source/dws-multi-skill-align`  
> 分支：`feat/multi-skill-framework-align`（自 `origin/main` @ `a37e6e68`）  
> **本分支范围：只做 skill 内容的这个框架**（目录布局、文档契约、共享内容约定、zip 内容树合同、**相对 mono 的内容质检**）。  
> **不做**安装/升级引擎、agent-home、脚本 skill-install 行为翻转。
>
> 对照仓：
>
> | 仓 | 路径 | 基线 |
> |---|---|---|
> | DWS OSS CLI（本工作树） | `dws-multi-skill-align` | `origin/main` |
> | dws-wukong | `~/GolandProjects/open-source/dws-wukong` | `origin/develop` @ `ab76629a`（调研时） |
> | 行为参考（**另一分支**） | `dws-skill-mode-migration` @ `402429ac`/`d5c8982c` | 安装默认 multi / upgrade 强制 multi —— **不在本分支排期** |
> | 内容缺口留档（参考） | 同迁移分支 `docs/skill-capability-completion.md`（M1–M6 / X1 等） | **仅作质检目标线索**，非本分支权威 |

---

## 0. TL;DR

1. **本分支 = skill 内容框架 + 相对 mono 的内容质检**：固化 `skills/multi` 组织合同，并用 **mono 单 skill 布局作对照基准**做覆盖/结构/漂移门禁（文档 + CI 内容护栏）。
2. **对齐悟空**：只取内容树组织概念；质检以 **DWS-native** 设计为主（已有 policy/测试可复用）。悟空 `validate-multiskill-bundle.py` 仅借鉴「frontmatter / 断链 / requires」类检查思路，**不**移植 bundle/安装校验。
3. **安装/升级行为**与 `402429ac`/`d5c8982c` → **单独 follow-up 分支**，本方案只登记。
4. 质检 **不改**默认安装哪棵树；只保证 multi 内容相对 mono **可解释、可覆盖、可回归**。

### 0.1 IN SCOPE

| 类别 | 包含 |
|---|---|
| 内容树结构 | `skills/mono/` 与 `skills/multi/<name>/` 目录合同 |
| 单 skill 约定 | `SKILL.md` frontmatter / 契约块 / Golden Route；`references/`；可选 `scripts/` |
| 共享内容 | `dingtalk-shared` 职责与被引用方式；与 mono 全局文映射（文档级） |
| 命名与集合 | `dingtalk-*` + `dingtalk-shared`；相对悟空的共有/独有清单（文档） |
| Zip **内容布局合同** | `mono/` / `multi/` / 根 mono 副本的内容含义与树形状；不改安装默认 |
| **Mono↔multi 内容质检** | 覆盖、结构、漂移三类门禁；复用/扩展现有 policy 与测试；缺口修复属内容编辑（另批或同分支内容 Phase） |
| 内容架构文档 | 本文件 + 可选短文（架构合同 + 质检矩阵） |

### 0.2 OUT OF SCOPE

| 类别 | 去向 |
|---|---|
| 安装默认 multi、upgrade always-multi | Follow-up 分支（`402429ac`/`d5c8982c`） |
| `LocateSkillsRoot` / `skill_setup` / `paths.go` / `skillhome` / install 脚本行为 | 同上 |
| 安装/运行时 manifest、state.json、mode 切换、telemetry header | 拒绝或行为分支 |
| 悟空 `_install.sh` / dual / Qwen / RewindDesktop / pod | 拒绝 |
| 非 skill 内容的 CLI 功能（schema/shortcut 代码等） | 拒绝 |
| 把质检做成「改安装默认值」的后门 | 拒绝 |

---

## 1. 内容现状盘点

### 1.1 DWS `skills/mono`（质检对照基准 · 单 skill）

```text
skills/mono/
├── SKILL.md
├── references/
│   ├── products/<area>.md|…/   # 产品能力面（质检「覆盖」主源）
│   ├── error-codes.md、…  # 全局协议（无 recovery 闭环）
│   └── best_practices/…
└── scripts/
```

### 1.2 DWS `skills/multi`（内容主体）

```text
skills/multi/
├── dingtalk-shared/     # 跨产品契约 / routing / 全局协议应落点
└── dingtalk-*/     # 19 产品 + 各 references、scripts
```

仅 DWS 有（悟空无）：dev, event, hrbrain, markdown, pat, profile, skill。

### 1.3 悟空 `dingtalk-skills/`（内容组织对照，非质检权威）

Flat `dingtalk-*` + `dingtalk-shared`；单 skill 骨架同构。**不作为 mono 覆盖基准**（集合更小、不同源）。

### 1.4 Zip 内容布局合同

| Zip 路径 | 内容含义 |
|---|---|
| `<root>/` | mono 副本（兼容） |
| `<root>/mono/` | 显式 mono 内容源 |
| `<root>/multi/` | 与 `skills/multi/` 同构 |

质检可断言「源树形状」；**不**断言安装面默认选哪棵。

### 1.5 现有 DWS skill 内容质检资产（复用清单）

| 资产 | 作用 | 与 mono↔multi 质检关系 |
|---|---|---|
| `scripts/policy/check-skill-commands.sh` + `skill-command-check/` | Skill 文内 `dws …` 命令路径存在性 | **复用**（命令真实性）；非覆盖映射 |
| `scripts/policy/check-skill-context-budget.sh` | chat/event/mono/`dingtalk-shared` 上下文预算与冷启动约束 | **复用**（结构/预算）；可扩展 shared 引用规则 |
| `scripts/policy/check-multi-im-skill-chain.sh` + `multi-im-skill-chain/` | IM 意图单默认路由、retired scripts、handoff | **复用**（chat/event 链）；面窄 |
| `test/unit/skill_docs_policy_test.go` | 退役命令、event 扁平输出契约等 | **复用**；可加 mono↔multi 断言 |
| `test/unit/whiteboard_skill_docs_test.go` | mono/multi whiteboard recipes **字节一致** | **样板**：产品面「同源文件」门禁范式 |
| `test/skill_static`（`-tags skill_verify`） | 文内命令 vs Cobra；multi 查 flag | **复用**（opt-in 深度）；非 CI 默认全量时可保持 tags |
| `test/skill_e2e` / `test/run_skill_tests.py` | 执行层 / 用例驱动 | **偏行为**；本分支质检默认不依赖 e2e |
| `Makefile` → `policy` 含 context-budget、multi-im-skill-chain；`skill-command-integrity` 独立 | 已有 CI 钩子 | 新门禁优先挂同类 policy / `test/unit` |

**缺口（尚无的门禁）**：系统的「mono `references/products/*` → multi 目录/文」覆盖表；frontmatter 全集完备性；orphan scripts。全局协议中 **确认门禁 / Schema 教学已补**；**recovery 闭环已从 skill 移除（不再作为缺口）**。

### 1.6 悟空侧类比质检

| 悟空 | 说明 | 本分支 |
|---|---|---|
| `scripts/validate-multiskill-bundle.py` | 校验 **已打好的 bundle zip**：frontmatter keys/category、`requires`、markdown 断链、scenario 编排 | **Adapt 思路** → DWS 源树（`skills/multi` + 对照 mono），不跑 zip 安装语义 |
| `sync-monolith-to-multiskill.py` | mono→multi 派生 | **不**作默认质检手段；DWS 直接维护 multi |

结论：**DWS-native mono↔multi 质检**；悟空仅参考检查维度。

---

## 2. Diff（内容组织 + 质检视角）

### 2.1 已同构

Flat `dingtalk-*` + `dingtalk-shared`；`SKILL.md` + `references/`（+ 可选 `scripts/`）。

### 2.2 分叉与已知内容风险（质检要盯的）

| 风险 ID | 现象（线索） | 质检类型 |
|---|---|---|
| **C-cov** | mono `products/*` 能力面在 multi 无对应 skill/reference，或未登记「有意省略」 | 覆盖 |
| **C-struct** | multi 缺 frontmatter 字段、`references/`、`DWS_RUNTIME_CONTRACT`、对 `dingtalk-shared` 引用不一致 | 结构 |
| **C-drift-global** | 曾关注 recovery / 确认 / Schema；现确认与 Schema 已在 `dingtalk-shared`，**recovery skill 文档已删除** | 漂移（协议） |
| **C-drift-orphan** | multi（或 mono）scripts/refs 无文档引用；或 routing 指向无索引产品（留档 X1/M6） | 漂移（孤儿） |
| **C-pair** | 应对齐的成对文件（如 whiteboard recipes）内容不一致 | 漂移（成对） |

### 2.3 Reject

悟空安装包校验整文件照搬、内容集 19→12 砍产品、安装行为门禁冒充内容质检。

---

## 3. Goals / Non-goals

### 3.1 Goals

1. 固化 multi **内容目录合同**与 mono↔multi **映射说明**。  
2. 建立 **质检矩阵**（覆盖 / 结构 / 漂移）并以 mono 为对照基准；有意省略必须 reviewed 登记。  
3. **复用** §1.5 资产；新增门禁走 `scripts/policy` 或 `test/unit`，内容-only。  
4. （可选）纯内容元数据；**禁止**被安装引擎读取改行为。  
5. 质检失败 → 修 **内容**或更新「有意省略」表，不改 setup/upgrade。

### 3.2 Non-goals

安装/升级翻转；cherry-pick 行为提交；取消产品；悟空客户端；非 skill CLI 功能；用质检驱动默认 multi 安装。

---

## 4. 分期（内容框架 + 质检 · 均无安装引擎）

> 批准前 **零编码**（含不实现新 gates）。**已执行**：Phase 1–3 见文首状态。

### Phase 0 — 方案冻结（本文）

| | |
|---|---|
| **范围** | 本文件；§7（含质检轨）勾选 |
| **验收** | owner 重新批准 → ✅「现在开始执行」 |

### Phase 1 — Multi 内容目录合同 + 架构短文 ✅

| | |
|---|---|
| **范围** | `skills/multi` 目录合同；与悟空内容树对照表；zip `multi/` 同构合同 |
| **触达** | `docs/skill-content-framework.md` |
| **验收** | 可指导「如何新增 dingtalk-* 内容目录」 |

### Phase 2 — Mono↔multi **内容质检规格**（矩阵 + 缺口基线） ✅

| | |
|---|---|
| **范围** | 质检规格 + 覆盖/omit 机读表 + 缺口 disposition |
| **触达** | `docs/skill-mono-multi-qa.md`、`skills/content-qa/mono-multi-coverage.yaml` |
| **验收** | 矩阵可人工抽查；缺口均有 disposition |

### Phase 3 — 质检落地：CI 内容护栏（复用 + 新 gate） ✅

| | |
|---|---|
| **范围** | G1–G4 自动门禁 |
| **触达** | `test/unit/mono_multi_skill_content_test.go`、`scripts/policy/check-mono-multi-skill-content.sh`、`Makefile` |
| **验收** | `make skill-mono-multi-content` 绿；已知缺口走 reviewed omit |

### Phase 4 — 可选：内容包元数据 + 缺口修复波次

| | |
|---|---|
| **范围 A** | 纯内容 layout/skill 列表元数据（人不读安装器） |
| **范围 B** | 按 Phase 2 disposition **修内容**：确认 / Schema 已补；**recovery skill 文档已删除（wontfix 移植）**；orphan 脚本仍走 allowlist（M4 等） |
| **验收** | 元数据不驱动安装；修复项关闭对应质检失败或转入 omit |

### 延期登记（非本分支）

| 主题 | 载体 |
|---|---|
| 默认 multi + upgrade always-multi | 行为分支 ← `402429ac`/`d5c8982c` |
| skillhome / 安装面 bootstrap | 行为分支 |

---

## 5. Port / Adapt / Reject

| 项 | 决策 | 说明 |
|---|---|---|
| flat + `dingtalk-shared` 内容模型 | **Port** | 已有；合同 + 质检加固 |
| 悟空 bundle frontmatter/断链/requires 检查维度 | **Adapt** | 做成 DWS 源树门禁，不校验 bundle zip/安装 |
| whiteboard 式 mono/multi 成对一致 | **Port（范式）** | 推广到 reviewed 文件对 |
| `validate-multiskill-bundle.py` 整脚本 | **Reject** | 绑定悟空 zip/Qwen 语义 |
| `_install.sh` / dual / overlay | **Reject** | 非内容 |
| 行为 cherry-pick | **Defer** | 另分支 |

---

## 6. 与 `402429ac` / `d5c8982c`

| | |
|---|---|
| 本分支 cherry-pick？ | **否** |
| 质检是否替代行为翻转？ | **否** |
| 行为分支 | 另开；可与内容/质检并行 |

---

## 7. 批准清单（请重新勾选）

**范围**

- [x] 本分支 = skill **内容**框架 + **mono↔multi 内容质检**（§0.1）；无安装/升级引擎  
- [x] `402429ac`/`d5c8982c` 及 setup/paths/install 脚本行为 **不在本分支**  
- [x] 取消产品与悟空客户端链路仍拒绝  

**内容框架 Phase**

- [x] **Phase 1**：multi 目录合同 + 悟空内容树对照短文  

**质检轨 Phase**

- [x] **Phase 2**：质检矩阵 + mono↔multi 覆盖/缺口基线规格（先文档，可执行）  
- [x] **Phase 3**：CI 内容护栏（G1–G4）—— 本迭代做 / 拆 PR / 只要规格暂不落地  
- [x] 质检失败处置原则：修内容或 reviewed omit，**不**改安装默认  

**可选**

- [ ] **Phase 4A** 纯内容元数据：做 / 不做 / 以后  
- [x] **Phase 4B** recovery skill 文档 **removed/wontfix**；确认/Schema 已补；剩余 orphan（M4 等）仍 defer / allowlist  

**Follow-up 知悉**

- [ ] 安装默认 multi + upgrade always-multi → **另一分支**  

---

## 8. 下一步

**Phase 1–3 已落地**（合同短文 + 质检规格 + `skills/content-qa` + CI 门禁）。  
Phase 4B：recovery 已删除（不做移植）；确认/Schema 已补。剩余 defer：orphan scripts（M4 等）、LICENSE/NOTICE（M5）、Phase 4A 元数据。  
安装默认 multi 等行为仍走 **另一分支**。

---

*锚点：`skills/mono`、`skills/multi`、§1.5 policy/测试、wukong `dingtalk-skills/`（组织对照 only）。*
