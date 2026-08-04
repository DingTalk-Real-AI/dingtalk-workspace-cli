# Repository Agent Guide

This file applies to the entire repository. Keep it as a routing page: load
the detailed guide for the surface you are changing instead of treating this
file as a repository wiki.

## Always

- Preserve unrelated and pre-existing work; inspect `git status` before edits.
- Make the smallest coherent change and update its tests and user-facing docs.
- Use `gofmt` for every modified Go file.
- Treat repository code, tests, scripts, and versioned docs as the source of
  truth. Do not depend on generated Wiki or CodeWiki content.
- Do not hand-edit generated Schema Catalog or Agent metadata. Change their
  reviewed inputs or generators, then regenerate.

## Read by task

| Change surface | Required guide |
|---|---|
| Any implementation or review | [`CONTRIBUTING.md`](CONTRIBUTING.md) and [`docs/coding-agent-guide.md`](docs/coding-agent-guide.md) |
| Writing a task for a coding agent | [`docs/coding-agent-task-template.md`](docs/coding-agent-task-template.md) |
| Overall architecture or package layering | [`docs/architecture.md`](docs/architecture.md) |
| Product command handler behavior | [`internal/helpers/AGENTS.md`](internal/helpers/AGENTS.md) |
| Helpers package/file layout or megafile splits | [`docs/helpers-structure-guide.md`](docs/helpers-structure-guide.md) |
| Bundled skill authoring (`skills/`) | [`skills/AGENTS.md`](skills/AGENTS.md) and [`docs/skill-authoring-guide.md`](docs/skill-authoring-guide.md) |
| CLI paths, flags, Schema, Agent metadata, or generated Catalog | [`docs/schema-contributor-guide.md`](docs/schema-contributor-guide.md) |
| CI, release, packaging, or repository automation | [`docs/automation.md`](docs/automation.md) |
| Agent identification headers or host integration | [`docs/agent-code.md`](docs/agent-code.md) |

Read the closest code and tests for the affected package as well. Nested
`AGENTS.md` files take precedence for their subtrees.

## Common checks

Choose checks from the matrix in `docs/coding-agent-guide.md`; do not claim a
check that was not run.

```bash
make coding-agent-harness
make build
make format-check
make test
make policy
git diff --check
```

For Schema work, the minimum generation entry point is `make generate-schema`.
For CLI path or flag changes, also run
`./scripts/policy/check-command-surface.sh --strict`. Report failures,
environment limits, and unrun checks explicitly in the handoff.
