#!/usr/bin/env bash
# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0
#
# Sync release artifacts to a China-accessible OSS mirror so that the install
# scripts' DWS_RELEASE_BASE switch can serve domestic users.
#
# Mirror layout produced (matches install.sh RELEASE_BASE="<base>/<version>/<file>"):
#   oss://$OSS_BUCKET/$OSS_PREFIX/download/<version>/dws-<os>-<arch>.tar.gz|.zip
#   oss://$OSS_BUCKET/$OSS_PREFIX/download/<version>/checksums.txt
#   oss://$OSS_BUCKET/$OSS_PREFIX/download/<version>/dws-skills.zip
#   oss://$OSS_BUCKET/$OSS_PREFIX/latest.txt            (plain version string)
#
# So with the CDN/custom domain CNAME'd to the bucket, China users run:
#   DWS_RELEASE_BASE=https://<domain>/$OSS_PREFIX/download \
#   DWS_VERSION=<version> \
#   curl -fsSL https://<domain>/$OSS_PREFIX/install.sh | sh
#
# Required environment (CI secrets):
#   OSS_ACCESS_KEY_ID, OSS_ACCESS_KEY_SECRET, OSS_ENDPOINT, OSS_BUCKET
# Optional:
#   OSS_PREFIX   (default: dws)
#   DIST_DIR     (default: ./dist)
#   VERSION      (default: derived from `git describe`)
#
# Gating: if the required OSS_* vars are unset, the script exits 0 with a notice
# (so it can be wired into release.yml without breaking forks that lack secrets).

set -eu

DIST_DIR="${DIST_DIR:-dist}"
OSS_PREFIX="${OSS_PREFIX:-dws}"

# ── Gate: skip cleanly when credentials are not configured ────────────────────
missing=""
for v in OSS_ACCESS_KEY_ID OSS_ACCESS_KEY_SECRET OSS_ENDPOINT OSS_BUCKET; do
  eval "val=\${$v:-}"
  [ -z "$val" ] && missing="$missing $v"
done
if [ -n "$missing" ]; then
  echo "ℹ️  OSS mirror sync skipped — missing:${missing}"
  echo "   Set these as repo secrets to auto-publish releases to the China mirror."
  exit 0
fi

# ── Resolve version ──────────────────────────────────────────────────────────
VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}"
echo "📦 Syncing release ${VERSION} → oss://${OSS_BUCKET}/${OSS_PREFIX}/download/${VERSION}/"

# ── Ensure ossutil is available ───────────────────────────────────────────────
OSSUTIL="${OSSUTIL:-ossutil}"
if ! command -v "$OSSUTIL" >/dev/null 2>&1; then
  echo "⬇  ossutil not found; installing to ./.ossutil ..."
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "$arch" in x86_64|amd64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; esac
  curl -fsSL "https://gosspublic.alicdn.com/ossutil/v2/2.0.0/ossutil-2.0.0-${os}-${arch}.zip" -o /tmp/ossutil.zip
  unzip -qo /tmp/ossutil.zip -d /tmp/ossutil-extract
  found="$(find /tmp/ossutil-extract -name ossutil -type f | head -1)"
  mkdir -p ./.ossutil && cp "$found" ./.ossutil/ossutil && chmod +x ./.ossutil/ossutil
  OSSUTIL="./.ossutil/ossutil"
fi

oss_cp() {
  # oss_cp <local-file> <oss-key>
  "$OSSUTIL" cp -f \
    --access-key-id "$OSS_ACCESS_KEY_ID" \
    --access-key-secret "$OSS_ACCESS_KEY_SECRET" \
    --endpoint "$OSS_ENDPOINT" \
    "$1" "oss://${OSS_BUCKET}/$2"
}

base="${OSS_PREFIX}/download/${VERSION}"

# ── Upload binaries + checksums + skills ──────────────────────────────────────
uploaded=0
for f in "$DIST_DIR"/dws-*.tar.gz "$DIST_DIR"/dws-*.zip "$DIST_DIR"/checksums.txt; do
  [ -f "$f" ] || continue
  oss_cp "$f" "${base}/$(basename "$f")"
  uploaded=$((uploaded + 1))
done

if [ "$uploaded" -eq 0 ]; then
  echo "❌ No artifacts found in ${DIST_DIR}. Run the build (goreleaser) first." >&2
  exit 1
fi

# ── Publish the latest pointer (for DWS_VERSION=latest resolution on mirror) ──
echo "$VERSION" > /tmp/dws-latest.txt
oss_cp /tmp/dws-latest.txt "${OSS_PREFIX}/latest.txt"

# ── Publish the install scripts themselves (entry layer) ──────────────────────
for s in install.sh install.ps1 install-skills.sh; do
  [ -f "scripts/$s" ] && oss_cp "scripts/$s" "${OSS_PREFIX}/$s"
done

echo "✅ Synced ${uploaded} artifact(s) + install scripts + latest.txt to the OSS mirror."
echo "   China install:"
echo "     DWS_RELEASE_BASE=https://<your-domain>/${OSS_PREFIX}/download DWS_VERSION=${VERSION} \\"
echo "       curl -fsSL https://<your-domain>/${OSS_PREFIX}/install.sh | sh"
