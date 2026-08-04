# Skill Authoring Guide

Contract for the bundled agent skills under `skills/`. Skills are embedded via
`skills/embed.go` and installed by `dws skill setup`, so every edit ships with
the binary. Keep skill prose concise: long command references belong in
`references/`, not in `SKILL.md`.

## Layout

| Path | Role |
|---|---|
| `skills/mono/` | Single bundled skill (stable mode) with shared `references/` and `scripts/` |
| `skills/multi/dingtalk-<product>/` | One skill per product (experimental mode) |
| `skills/multi/dws-shared/` | Shared prerequisite: auth, global flags, routing, safety |
| `skills/embed.go` | Embeds `mono` + `multi`; do not add new roots |

A product skill directory contains:

```text
dingtalk-<product>/
  SKILL.md          # frontmatter + concise routing/usage prose
  references/       # long command references, playbooks
  scripts/          # executable recipes (python), kept minimal
```

## SKILL.md contract

Frontmatter:

```yaml
---
name: dingtalk-<product>
description: <触发场景>. Use when … Distinct from <相邻skill>(…). 命令前缀：dws <product>。
cli_version: ">=<minimum dws version>"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---
```

Body rules:

- State the safety rules directly in concise prose; there is no injected
  preamble mechanism in this repository.
- State the `dws-shared` prerequisite for multi skills.
- Route by intent: shortcuts table first when one covers the scenario, then
  scripts/recipes, then atomic commands with `dws schema` / `--help`.
- Every referenced `dws` command must exist in the current binary; verify with
  `dws <cmd> --help` and keep prose version-agnostic ("以当前 dws 二进制为准").
- Mutating commands must point at the leaf Schema `confirmation` contract; do
  not invent confirmation rules in prose.

## Dual-write rule

Skill behavior described in prose must match the CLI it references. When a
command, flag, or confirmation contract changes, update the affected `SKILL.md`
/ `references/` in the same change; when skill routing changes, check whether
Schema selection hints (`internal/cli/schema_hints/`) need a reviewed update
per [`schema-contributor-guide.md`](schema-contributor-guide.md).

## Validation

| Change | Check |
|---|---|
| Any skill edit | `make skill-command-integrity` |
| Referenced CLI surface changed | `./scripts/policy/check-command-surface.sh --strict` |
| Schema hints touched | `make generate-schema` + schema gates |
| Recipe scripts | run the script's own smoke path or `test/skill_e2e` when applicable |

`make skill-command-integrity` builds `scripts/policy/skill-command-check` and
verifies every `dws` command referenced by skills resolves against the current
binary. Run it before handoff; do not claim a command exists without it.
