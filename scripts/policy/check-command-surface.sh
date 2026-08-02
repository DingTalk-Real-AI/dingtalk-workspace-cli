#!/bin/sh
set -eu

# Check command surface for drift.
#
# 硬门禁：校验内嵌 schema_catalog.json 的封闭结构（字段白名单 + 枚举值 +
# 交叉一致性），由 internal/cli/schema_catalog_structure.go 的
# TestEmbeddedSchemaCatalogStructure 实现。该门禁确保任何写入 catalog 的代码
# 路径都不会产出结构非法的工具条目。

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
BIN="${DWS_BIN:-$ROOT/dws}"

STRICT=0
if [ "${1:-}" = "--strict" ]; then
  STRICT=1
fi

cd "$ROOT"

if [ ! -x "$BIN" ]; then
  printf 'error: dws binary not found at %s (run make build first)\n' "$BIN" >&2
  exit 2
fi

# 硬门禁：内嵌 schema_catalog.json 封闭结构校验。
if ! go test ./internal/cli -run '^TestEmbeddedSchemaCatalogStructure$' -count=1; then
  printf 'embedded schema catalog structure check: FAILED\n' >&2
  exit 1
fi

# These utility commands are stable across the current open-source CLI shape.
EXPECTED_COMMANDS="auth cache completion version"
missing=0
for cmd in $EXPECTED_COMMANDS; do
  # Use `help <command>` so hidden commands are also validated.
  if ! "$BIN" help "$cmd" >/dev/null 2>&1; then
    printf 'missing command: %s\n' "$cmd" >&2
    missing=$((missing + 1))
  fi
done

# Full Schema canonical/primary/alias delivery is exercised once by
# check-schema-binary.sh. Keep this check focused on the basic command tree.
if [ "$STRICT" -eq 1 ] && [ "$missing" -gt 0 ]; then
  printf 'command surface check: %d missing commands\n' "$missing" >&2
  exit 1
fi

printf 'command surface check: ok\n'
