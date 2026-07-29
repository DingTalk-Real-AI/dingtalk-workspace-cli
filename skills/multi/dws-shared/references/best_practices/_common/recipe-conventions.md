# Recipe 规范

> 每个 recipe 是一个 **SKILL.md**，格式与产品 skill 同等身份。

## 架构三层

```
Layer 1: _common/conventions.md     → 全局共享（认证、安全规则、字段术语）
                                       相当于 gws-shared/SKILL.md

Layer 2: products/*.md              → 产品能力（命令用法、flags、示例）
                                       相当于 gws-gmail/SKILL.md, gws-docs/SKILL.md

Layer 3: recipe-xxx/SKILL.md        → 组合配方（固定步骤 + 声明依赖）
                                       相当于 recipe-send-team-announcement/SKILL.md
```

## Recipe SKILL.md 格式

```yaml
---
name: recipe-<verb>-<object>
description: "一句话描述"
metadata:
  category: "recipe"
  domain: "<领域编号>"
  requires:
    bins:
      - dws
    skills:
      - <product-1>
      - <product-2>
---
```

**正文**只包含：
1. 标题 + 一句话描述
2. `> **PREREQUISITE:** ...` 提示加载哪些 skill
3. `## Steps` — 编号步骤，每步一条 `dws` 命令

**不包含**（这些留给 conventions.md 和 best_practice.md）：
- 安全门控/决策分支/踩坑提醒
- 失败处理策略
- 数据佐证

## 命名规则

```
recipe-<verb>-<object>[-<modifier>]
```

| 动词 | 含义 | 示例 |
|------|------|------|
| send | 发送 | recipe-send-message |
| create | 创建 | recipe-create-todo |
| query | 查询 | recipe-query-doc |
| write | 写入 | recipe-write-doc |
| resolve | 解析 | recipe-resolve-contact |
| generate | 生成 | recipe-generate-daily-report |
| share | 分享 | recipe-share-doc-and-notify |

## 路由规则

Agent 识别意图后：
1. 匹配到 recipe → 加载 recipe 声明的 skills → 按 Steps 执行
2. 无匹配 recipe → 走领域 best_practice.md 策略指南
3. 只需单个命令 → 直接查 products/*.md
