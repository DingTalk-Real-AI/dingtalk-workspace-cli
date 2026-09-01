#!/bin/sh
set -eu

"$(dirname "$0")/../policy/check-runtime-payload.sh" --allow-unsupported-tools
go build -buildmode=pie -trimpath -ldflags="-s -w" -o dws ./cmd

case "$(uname -s)" in
  Darwin) runtime_os=darwin ;;
  Linux) runtime_os=linux ;;
  MINGW*|MSYS*|CYGWIN*) runtime_os=windows ;;
  *) printf 'Runtime payload is unavailable for this platform.\n'; exit 0 ;;
esac

case "$(uname -m)" in
  x86_64|amd64) runtime_arch=amd64 ;;
  arm64|aarch64) runtime_arch=arm64 ;;
  *) printf 'Runtime payload is unavailable for this architecture.\n'; exit 0 ;;
esac

"$(dirname "$0")/../build/prepare-runtime-payload.sh" "$runtime_os" "$runtime_arch" .
