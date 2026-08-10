# DWS Skill 内容框架合同

> 本分支权威合同：`skills/mono` / `skills/multi` 的**内容组织**与 zip 内容树形状。  
> 不做安装/升级行为约定。质检见 [skill-mono-multi-qa.md](skill-mono-multi-qa.md)。  
> 对齐调研：[skill-wukong-align-plan.md](skill-wukong-align-plan.md)。

## 1. 两棵内容树

| 树 | 路径 | 角色 |
|---|---|---|
| **mono**（单 skill） | `skills/mono/` | 单一 `SKILL.md` 入口 + `references/products/*` 产品面 + 全局协议 |
| **multi**（多 skill） | `skills/multi/` | 平铺 `dingtalk-*` 产品 skill + 必选 `dingtalk-shared` |

Agent / 安装面选哪棵树由**行为分支**决定；本文件只规定树内合同。

## 2. Multi 目录合同（如何新增一个产品 skill）

新建 `skills/multi/<name>/` 时必须满足：

1. **命名**
   - 产品 skill：`dingtalk-<product>`（小写、连字符）
   - 共享 skill：仅允许 `dingtalk-shared`
2. **根文件**
   - 必有 `SKILL.md`（YAML frontmatter + 正文）
   - `references/` 推荐；无 reference 的 skill（如极简 profile）须在质检 omit 表登记
   - `scripts/` 可选；脚本须被本 skill 树内某 `.md` 引用，或进入 orphan allowlist
3. **Frontmatter 最小集**（产品 / shared）
   - `name`：与目录名一致
   - `description`：非空，含触发意图与边界
   - `metadata.category`：`product` 或 `shared`（允许历史写法把 `cli_version` 放在 frontmatter 顶层）
   - `metadata.requires.bins`：含 `dws`
4. **契约块**
   - 产品 skill 推荐内嵌 `<!-- DWS_RUNTIME_CONTRACT_START -->…END -->` **或** 明确 PREREQUISITE 指向 `dingtalk-shared`
   - `dingtalk-shared` 承载跨产品路由与全局协议落点
5. **与 mono 映射**
   - 每个 mono `references/products/<stem>`（文件或目录）必须在
     `skills/content-qa/mono-multi-coverage.yaml` 有 `coverage` 或 `omit_coverage` 行

### 2.1 推荐骨架

```text
skills/multi/dingtalk-example/
├── SKILL.md
├── references/
│   ├── example.md          # 主产品面
│   └── …                   # 子章节 / 意图表
└── scripts/                # 可选；须被 md 引用
    └── example_helper.py
```

## 3. Mono 目录合同（质检对照基准）

```text
skills/mono/
├── SKILL.md
├── references/
│   ├── products/           # 覆盖质检主源
│   ├── error-codes.md      # 全局协议示例
│   ├── error-codes.md
│   └── …
└── scripts/
```

- `references/products/` 下每个顶层 stem（`.md` 去后缀或子目录名）计入覆盖索引。
- 同 stem 的 `.md` + 子目录视为同一产品面（如 `doc.md` + `doc/`）。

## 4. 共享内容（`dingtalk-shared`）

| 职责 | 落点 |
|---|---|
| 跨产品路由 / 工作流 | `references/routing.md`、`workflow-routing.md`、`intent-guide.md` |
| 运行时最小契约长文 | `references/runtime-contract.md`（受 context-budget 约束） |
| 全局协议（确认门禁 / Schema 教学等） | `references/`；见质检基线 |
| 与 mono 全局文同名迁移 | `error-codes`、`url-patterns`、`capability-limits`、`channel-login`、`global-reference`、`recipes/`（`conventions.md`、`meta.md`、`lite-catalog.md`） |

产品专属规则（如 AI 表格 `field-rules`）允许下沉到对应 `dingtalk-*`，须在覆盖表注明。

## 5. Zip 内容布局合同（形状，非安装默认）

发布物 `dws-skills.zip`（及 embed 同源）内容树形状：

| Zip 路径 | 含义 |
|---|---|
| `<root>/` | mono 内容副本（兼容旧面） |
| `<root>/mono/` | 与 `skills/mono/` 同构 |
| `<root>/multi/` | 与 `skills/multi/` 同构 |

质检可断言源树形状；**不**断言安装器默认解压哪棵。

## 6. 与悟空 `dingtalk-skills/` 对照（组织概念 only）

| 维度 | DWS `skills/multi` | 悟空 `dingtalk-skills/`（develop） |
|---|---|---|
| 布局 | flat `dingtalk-*` + `dingtalk-shared` | 同构 flat |
| 集合 | 产品 skill + shared（含 event/profile/…；dev/skill 等长尾落在 misc） | 更小产品集（如 attendance/report 独立目录） |
| 质检权威 | **mono 单 skill 树** | 不作为 DWS 覆盖基准 |
| 不移植 | `_install.sh` / bundle / dual / Qwen overlay | — |

悟空独有命名（如 `dingtalk-attendance`）在 DWS 中由 `dingtalk-misc` 承接对应 mono `attendance*` / `report` / `oa` / `sheet` / `dev` 等面——见覆盖表。

## 7. 变更流程

1. 改 / 增内容 → 更新 `skills/content-qa/mono-multi-coverage.yaml`（coverage 或 omit）  
2. 跑 `make skill-mono-multi-content`（该独立门禁不包含在默认 `make policy` 中）
3. 失败则修内容或更新 reviewed omit（disposition + 原因），**禁止**用安装默认值绕过  
