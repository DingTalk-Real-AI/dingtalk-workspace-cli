#!/bin/sh
set -eu

# ── Format check ──────────────────────────────────────────
unformatted="$(find cmd internal test -name '*.go' -print0 2>/dev/null | xargs -0r gofmt -l)"
if [ -n "$unformatted" ]; then
  echo "$unformatted"
  echo "Go files are not formatted. Run 'make fmt'." >&2
  exit 1
fi

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
