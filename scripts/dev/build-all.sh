#!/bin/bash
# Cross-platform build script (goreleaser-free alternative)
# Usage: ./scripts/dev/build-all.sh
#        VERSION=1.0.0 ./scripts/dev/build-all.sh
set -eu

cd "$(dirname "$0")/../.."

VERSION=${VERSION:-"0.0.0-SNAPSHOT"}
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

DIST_DIR="dist"
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

LDFLAGS="-s -w"
LDFLAGS="$LDFLAGS -X github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app.version=v${VERSION}"
LDFLAGS="$LDFLAGS -X github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app.gitCommit=${COMMIT}"
LDFLAGS="$LDFLAGS -X github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app.buildTime=${BUILD_TIME}"

platforms=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
    "windows/arm64"
)

echo "==> Building dws v${VERSION} (commit: ${COMMIT})"

for platform in "${platforms[@]}"; do
    GOOS=${platform%/*}
    GOARCH=${platform#*/}

    output_name="dws"
    archive_name="dws-${GOOS}-${GOARCH}"

    if [ "$GOOS" = "windows" ]; then
        output_name="dws.exe"
    fi

    echo "  • building ${GOOS}/${GOARCH}..."
    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build \
        -buildmode=pie -trimpath \
        -ldflags="$LDFLAGS" \
        -o "$DIST_DIR/$output_name" \
        ./cmd

    cd "$DIST_DIR"
    if [ "$GOOS" = "windows" ]; then
        zip -q "${archive_name}.zip" "$output_name" -j ../LICENSE ../NOTICE ../README.md ../CHANGELOG.md
    else
        tar -czf "${archive_name}.tar.gz" "$output_name" -C .. LICENSE NOTICE README.md CHANGELOG.md
    fi
    rm "$output_name"
    cd ..
done

echo "==> Calculating checksums"
cd "$DIST_DIR"
shasum -a 256 ./*.tar.gz ./*.zip > checksums.txt
cd ..

echo "==> Build complete! Artifacts in $DIST_DIR/"
ls -lh "$DIST_DIR"
