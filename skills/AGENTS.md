# Bundled Skills Agent Guide

This file applies to `skills/`. Read the root `AGENTS.md` and
[`docs/coding-agent-guide.md`](../docs/coding-agent-guide.md) first. The full
authoring contract lives in
[`docs/skill-authoring-guide.md`](../docs/skill-authoring-guide.md); this file
is the scoped summary.

## Hard rules

- `skills/mono` is the stable single-skill layout; `skills/multi/dingtalk-*`
  are per-product experimental skills; `dws-shared` is the prerequisite.
- Keep `SKILL.md` concise: frontmatter (`name`, `description` with triggers +
  `Distinct from` + 命令前缀, `cli_version`), routing prose, and pointers.
  Long references go to `references/`, recipes to `scripts/`.
- Never remove the `<!-- SAFETY_PREAMBLE_INJECT -->` marker or hand-write the
  injected preamble.
- Every referenced `dws` command must exist in the current binary; run
  `make skill-command-integrity` before handoff.
- Dual-write: CLI behavior changes update skill prose in the same change;
  skill routing changes check Schema selection hints per
  [`docs/schema-contributor-guide.md`](../docs/schema-contributor-guide.md).
- Do not add new embed roots; `skills/embed.go` embeds `mono` + `multi` only.
