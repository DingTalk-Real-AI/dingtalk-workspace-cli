# 命令别混淆规范

> 本文档**只做一件事**：在 AI Code Review「Agent 友好性检查」中，
> 检查本次 MR 新增/修改的命令名是否与**已有命令**易混淆。
>
> **数据源**：开源版用 `dws help-all`（或遍历 `dws <product> --help`）实时
> 生成当前命令快照，AI 必须以该快照为唯一基准做命名混淆比对。也可参考
> [`docs/command-index.md`](../command-index.md)（如已维护）。
>
> **本文档不约束**动词白名单、flag 命名、Description 写法、产品域归属等其它维度 —
> 那些应交由 review 人和上游开发约定，AI 不强制。

---

## 一、混淆的定义

新增/修改的命令名，**与标准指令集中已有命令满足以下任一条件**，即视为"易混淆"：

| 类型 | 反例 | 正例 |
|------|------|------|
| **A. 子命令名 ≈ 已有命令 + flag** | 已有 `dws contact list --user xxx`，新增 `dws contact list-by-user` | 复用已有命令 + flag |
| **B. 同一资源下同义动词并存** | 已有 `dws aitable record query`，又新增 `dws aitable record search` | 同一资源选定一个动词 |
| **C. 单复数混用** | 已有 `dws drive file get`，新增 `dws drive files list` | 与已有保持一致 |
| **D. 跨产品域重复定义同语义命令** | `dws doc table create` 与 `dws aitable table create` 都创建多维表 | 明确归属一个产品域 |
| **E. 拼写极相似** | 已有 `dws aitable record list`，新增 `dws aitable records list` | 与已有保持一致 |

---

## 二、不算混淆的情况（避免误报）

> 以下情况**不要报警**，它们是 dws 项目里合法的现状。

1. **跨资源、跨产品域**复用同一动词 — 例如 `dws aiapp query` 与 `dws aitable record query` 并存，没问题
2. **`query` 与 `search` 跨资源并存** — 在 dws 里这是两个合法且并存的动词（`query` 多用于按条件返回单条/聚合，`search` 多用于关键字检索列表）。**只有同一资源下两者都存在且语义重叠时才算混淆**
3. **不同产品域用了不同的 flag 命名风格**（如 `--cursor`+`--size` vs `--page`+`--size`）— 属于历史现状，不算混淆
4. 别名（`Aliases`）注册的命令 — 这就是为了用户方便而故意做的"等价命名"，不算混淆

---

## 三、AI 的检查方法（强制流程）

1. **解析 diff**：找出本次 MR 中新增/修改的 cobra.Command 定义
   - 重点看 `Use:` / `Aliases:` 字段
   - 开源版命令有两种来源：
     - **动态发现**（`internal/compat/dynamic_commands.go`）：从 mse `toolOverrides` 自动生成，diff 时关注 `skills/references/products/*.md` 的命令引用变化
     - **静态 helper**（`internal/helpers/*.go`）：以 `&cobra.Command{...}` 字面量定义 + `parent.AddCommand(child)` 挂载
   - 顺着 `AddCommand` 调用链 / `toolOverrides.cliName + group`，推断出完整命令路径：`dws <product> <resource> <action>`
   - 顺着 `AddCommand` 调用链，推断出完整命令路径：`dws <product> <resource> <action>`

2. **对比命令快照**：在 `dws help-all`（或现有 [`docs/command-index.md`](../command-index.md)）输出中按以下顺序查找
   - 先找**完全相同**的命令路径 → 这是修改而非新增，跳过混淆检查
   - 再找**同产品域同资源**下的所有命令（如新增 `dws aitable record xxx`，则查所有 `dws aitable record *`）
   - 最后找**跨域同名**的命令（如新增 `dws doc table create`，全文搜 `table create`）

3. **判断是否命中"一、混淆的定义" A-E 任一类型**

4. **若命中**，在 CR 维度三输出告警，格式：
   ```
   ⚠️ 命令命名混淆：
   - 新增命令：`dws xxx yyy zzz`
   - 已有命令：`dws aaa bbb ccc`（引用标准指令集中的命令字符串）
   - 混淆类型：B. 同一资源下同义动词并存
   - 建议：复用已有命令 / 选定一个动词 / 改名为 `dws xxx yyy other-action`
   ```
   注：不要求精确给出标准指令集中的行号（指令集没有稳定的行号锚点），引用命令字符串本身即可。

5. **若未命中**，输出："本次新增命令未发现与已有命令的命名混淆。"

---

## 四、维护说明

- **命令快照**：由当前 `dws` 二进制实时遍历 `--help` 输出生成，AI CR 时按需采集；命令树由 mse `toolOverrides` 和本地 helpers 共同决定，发版前可生成快照 diff 到 `docs/command-index.md`
- **本文档**：规则若需调整，团队对齐后更新本文件；不要在文件里堆积"AI 觉得应该这样"的主观规范
