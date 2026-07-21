#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"

# ── Format check ──────────────────────────────────────────
echo "Running gofmt check..."
make format-check

# ── go vet (built-in) ────────────────────────────────────
echo "Running go vet..."
go vet ./...

# ── staticcheck ──────────────────────────────────────────
resolve_tool() {
  if command -v "$1" >/dev/null 2>&1; then
    command -v "$1"
  elif [ -x "$(go env GOPATH)/bin/$1" ]; then
    echo "$(go env GOPATH)/bin/$1"
  else
    return 1
  fi
}

if STATICCHECK="$(resolve_tool staticcheck)"; then
  echo "Running staticcheck..."
  "$STATICCHECK" ./...
else
  echo "staticcheck not found; install: go install honnef.co/go/tools/cmd/staticcheck@latest" >&2
  exit 1
fi

echo "All lint checks passed."
