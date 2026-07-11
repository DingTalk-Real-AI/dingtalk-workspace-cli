#!/usr/bin/env bash
# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0
#
# Sync release artifacts to a China-accessible OSS mirror. Channel pointers are
# mirror metadata; repository installers currently resolve GitHub/Gitee and do
# not consume these OSS pointers directly.
#
# Mirror layout produced:
#   oss://$OSS_BUCKET/$OSS_PREFIX/download/<version>/dws-<os>-<arch>.tar.gz|.zip
#   oss://$OSS_BUCKET/$OSS_PREFIX/download/<version>/checksums.txt
#   oss://$OSS_BUCKET/$OSS_PREFIX/download/<version>/dws-skills.zip
#   oss://$OSS_BUCKET/$OSS_PREFIX/latest.txt            (stable version only)
#   oss://$OSS_BUCKET/$OSS_PREFIX/beta.txt              (prerelease version only)
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

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/release-lib.sh"

DIST_DIR="${DIST_DIR:-dist}"
OSS_PREFIX="${OSS_PREFIX:-dws}"

# ── Gate: skip cleanly when credentials are not configured ────────────────────
missing=""
for v in OSS_ACCESS_KEY_ID OSS_ACCESS_KEY_SECRET OSS_ENDPOINT OSS_BUCKET; do
  eval "val=\${$v:-}"
  [ -z "$val" ] && missing="$missing $v"
done
if [ -n "$missing" ]; then
  if [ "${DWS_REQUIRE_OSS:-0}" = "1" ]; then
    echo "❌ OSS mirror sync is required but credentials are missing:${missing}" >&2
    exit 1
  fi
  echo "ℹ️  OSS mirror sync skipped — missing:${missing}"
  echo "   Set these as repo secrets to auto-publish releases to the China mirror."
  exit 0
fi

# ── Resolve version ──────────────────────────────────────────────────────────
VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}"
CHANNEL="${DWS_RELEASE_CHANNEL:-$(release_channel_for_version "$VERSION")}"
release_validate_version_channel "$CHANNEL" "$VERSION"
echo "📦 Syncing release ${VERSION} → oss://${OSS_BUCKET}/${OSS_PREFIX}/download/${VERSION}/"

# Never move a channel pointer to an incomplete or mixed-version directory,
# even when this helper is invoked outside the normal workflow.
DWS_PACKAGE_DIST_DIR="$DIST_DIR" "$SCRIPT_DIR/verify-release-artifacts.sh" "$VERSION"

# ── Ensure ossutil is available ───────────────────────────────────────────────
OSSUTIL="${OSSUTIL:-ossutil}"
OSSUTIL_INSTALL_DIR=""
if ! command -v "$OSSUTIL" >/dev/null 2>&1; then
  echo "⬇  ossutil not found; installing a verified temporary copy ..."
  case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=mac ;;
    *) printf 'unsupported ossutil operating system: %s\n' "$(uname -s)" >&2; exit 1 ;;
  esac
  arch="$(uname -m)"
  case "$arch" in x86_64|amd64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; esac
  case "${os}-${arch}" in
    linux-amd64) expected_ossutil_sha256=3ae4d9fc85a7a6e9f5654d1599766f1a3a42a3692870887b5ae9338d582ef65a ;;
    linux-arm64) expected_ossutil_sha256=f6c95ba0c2d2ef30290af686ce4d706c701f4734ce8090bee4288a77e3f1d764 ;;
    mac-amd64) expected_ossutil_sha256=8437fdd3ef1a3eb12310f61fcf1c00a5bff5cdab47b4fea815527472e7cf896c ;;
    mac-arm64) expected_ossutil_sha256=058fd048f321f8c80def8b748030531646eefe3a82837bf16b581ba7d9c84ac7 ;;
    *) printf 'unsupported ossutil architecture: %s-%s\n' "$os" "$arch" >&2; exit 1 ;;
  esac
  ossutil_archive="$(mktemp "${TMPDIR:-/tmp}/ossutil-2.3.0.XXXXXX.zip")"
  ossutil_extract="$(mktemp -d "${TMPDIR:-/tmp}/ossutil-2.3.0.XXXXXX")"
  env -u OSS_ACCESS_KEY_ID -u OSS_ACCESS_KEY_SECRET \
    curl -fsSL \
      "https://gosspublic.alicdn.com/ossutil/v2/2.3.0/ossutil-2.3.0-${os}-${arch}.zip" \
      -o "$ossutil_archive"
  if command -v sha256sum >/dev/null 2>&1; then
    actual_ossutil_sha256="$(sha256sum "$ossutil_archive" | awk '{print $1}')"
  else
    actual_ossutil_sha256="$(shasum -a 256 "$ossutil_archive" | awk '{print $1}')"
  fi
  [ "$actual_ossutil_sha256" = "$expected_ossutil_sha256" ] || {
    rm -rf "$ossutil_archive" "$ossutil_extract"
    printf 'ossutil archive checksum mismatch for %s-%s\n' "$os" "$arch" >&2
    exit 1
  }
  unzip -qo "$ossutil_archive" -d "$ossutil_extract"
  found="$(find "$ossutil_extract" -name ossutil -type f | head -1)"
  [ -n "$found" ] || {
    rm -rf "$ossutil_archive" "$ossutil_extract"
    printf 'verified ossutil archive does not contain the executable\n' >&2
    exit 1
  }
  OSSUTIL_INSTALL_DIR="$(mktemp -d "${TMPDIR:-/tmp}/dws-ossutil.XXXXXX")"
  cp "$found" "$OSSUTIL_INSTALL_DIR/ossutil" && chmod +x "$OSSUTIL_INSTALL_DIR/ossutil"
  rm -rf "$ossutil_archive" "$ossutil_extract"
  OSSUTIL="$OSSUTIL_INSTALL_DIR/ossutil"
fi

oss_cp() {
  # oss_cp <local-file> <oss-key>
  "$OSSUTIL" cp -f \
    --access-key-id "$OSS_ACCESS_KEY_ID" \
    --access-key-secret "$OSS_ACCESS_KEY_SECRET" \
    --endpoint "$OSS_ENDPOINT" \
    "$1" "oss://${OSS_BUCKET}/$2"
}

oss_get() {
  # oss_get <oss-key> <local-file>
  "$OSSUTIL" cp -f \
    --access-key-id "$OSS_ACCESS_KEY_ID" \
    --access-key-secret "$OSS_ACCESS_KEY_SECRET" \
    --endpoint "$OSS_ENDPOINT" \
    "oss://${OSS_BUCKET}/$1" "$2"
}

base="${OSS_PREFIX}/download/${VERSION}"

if [ "$CHANNEL" = "stable" ]; then
  pointer_name="latest.txt"
else
  pointer_name="beta.txt"
fi
current_pointer_file="$(mktemp "${TMPDIR:-/tmp}/dws-current-pointer.XXXXXX")"
pointer_file="$(mktemp "${TMPDIR:-/tmp}/dws-release-pointer.XXXXXX")"
pointer_status_file="$(mktemp "${TMPDIR:-/tmp}/dws-pointer-status.XXXXXX")"
trap 'rm -f "$current_pointer_file" "$pointer_file" "$pointer_status_file"; [ -z "$OSSUTIL_INSTALL_DIR" ] || rm -rf "$OSSUTIL_INSTALL_DIR"' EXIT HUP INT TERM
rm -f "$current_pointer_file"
current_version=""
update_pointer=1
if oss_get "${OSS_PREFIX}/${pointer_name}" "$current_pointer_file" >"$pointer_status_file" 2>&1; then
  [ -s "$current_pointer_file" ] || {
    cat "$pointer_status_file" >&2
    echo "❌ OSS ${pointer_name} was read successfully but is empty; refusing to publish." >&2
    exit 1
  }
  current_version="$(tr -d '[:space:]' < "$current_pointer_file")"
  release_validate_version_channel "$CHANNEL" "$current_version" || {
    printf '❌ OSS %s contains an invalid %s channel version: %s\n' "$pointer_name" "$CHANNEL" "$current_version" >&2
    exit 1
  }
  if [ "$current_version" != "$VERSION" ]; then
    if release_version_is_greater "$VERSION" "$current_version"; then
      :
    elif release_version_is_greater "$current_version" "$VERSION"; then
      update_pointer=0
      echo "ℹ️  OSS ${pointer_name} already points to newer ${current_version}; assets will be repaired without moving it."
    else
      echo "❌ Cannot order OSS ${pointer_name}=${current_version} against ${VERSION}." >&2
      exit 1
    fi
  fi
elif grep -Eq '(^|[^[:alnum:]])NoSuchKey([^[:alnum:]]|$)' "$pointer_status_file"; then
  printf 'ℹ️  OSS %s does not exist yet; creating it.\n' "$pointer_name"
else
  cat "$pointer_status_file" >&2
  printf '❌ Could not read OSS %s; refusing to move the channel pointer.\n' "$pointer_name" >&2
  exit 1
fi

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

# ── Publish a channel-specific pointer ───────────────────────────────────────
# A beta must never move latest.txt: that pointer is consumed by ordinary
# installs and is the stable channel contract.
pointer_summary="${pointer_name}"
if [ "$update_pointer" -eq 1 ]; then
  echo "$VERSION" > "$pointer_file"
  oss_cp "$pointer_file" "${OSS_PREFIX}/${pointer_name}"
else
  pointer_summary="${pointer_name} unchanged at ${current_version}"
fi

# Shared installer entrypoints are stable-only. A prerelease must not replace
# the stable entry layer, even though those scripts currently fetch from
# GitHub/Gitee rather than this asset mirror.
installer_summary=""
if [ "$CHANNEL" = "stable" ] && [ "$update_pointer" -eq 1 ]; then
  for s in install.sh install.ps1 install-skills.sh; do
    [ -f "scripts/$s" ] && oss_cp "scripts/$s" "${OSS_PREFIX}/$s"
  done
  installer_summary=" + stable install scripts"
fi

echo "✅ Synced ${uploaded} artifact(s) + ${pointer_summary}${installer_summary} to the OSS mirror."
echo "   Mirror path: ${OSS_PREFIX}/download/${VERSION}/"
