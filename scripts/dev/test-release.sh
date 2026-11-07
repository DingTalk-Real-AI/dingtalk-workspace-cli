#!/usr/bin/env bash
set -euo pipefail

ROOT="$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)"
VERSION="${VERSION:-v0.0.0-SNAPSHOT}"

cd "$ROOT"
echo "==> Building the pinned cross-platform release snapshot..."
make package VERSION="$VERSION"

echo "==> Verifying artifacts..."
echo "Binary version:"
./dist/dws-darwin-arm64/dws version 2>/dev/null || ./dist/dws-linux-amd64/dws version

echo "npm package.json:"
cat dist/npm/dingtalk-workspace-cli/package.json | grep '"version"'

echo "dws-skills.zip:"
ls -lh dist/dws-skills.zip

echo "==> npm publish dry-run..."
cd dist/npm/dingtalk-workspace-cli
npm publish --access public --dry-run

echo "==> All checks passed!"
