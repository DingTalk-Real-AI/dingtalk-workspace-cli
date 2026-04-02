#!/bin/bash
set -e

VERSION="v1.0.6"

echo "==> Running GoReleaser snapshot..."
goreleaser release --snapshot --clean

echo "==> Running post-goreleaser.sh..."
DWS_PACKAGE_VERSION=$VERSION ./scripts/release/post-goreleaser.sh

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