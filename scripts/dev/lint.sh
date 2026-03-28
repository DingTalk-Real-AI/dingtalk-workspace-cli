#!/bin/sh
set -eu

resolve_tool() {
  if command -v "$1" >/dev/null 2>&1; then
    command -v "$1"
  elif [ -x "$(go env GOPATH)/bin/$1" ]; then
    echo "$(go env GOPATH)/bin/$1"
  else
    return 1
  fi
}

# ── Format check ──────────────────────────────────────────
unformatted="$(find cmd internal test -name '*.go' -print0 2>/dev/null | xargs -0r gofmt -l)"
if [ -n "$unformatted" ]; then
  echo "$unformatted"
  echo "Go files are not formatted. Run 'make fmt'." >&2
  exit 1
fi

# ── golangci-lint v2 ─────────────────────────────────────
if GOLANGCI_LINT="$(resolve_tool golangci-lint)"; then
  echo "Running golangci-lint..."
  "$GOLANGCI_LINT" run --config .golangci.yml ./...
else
  echo "golangci-lint not found; install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.3" >&2
  exit 1
fi

echo "All lint checks passed."
