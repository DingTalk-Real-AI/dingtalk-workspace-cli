#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
DIST_DIR="${DWS_PACKAGE_DIST_DIR:-$ROOT/dist}"
VERSION="${1:-${DWS_PACKAGE_VERSION:-}}"
RELEASE_COMMIT="${DWS_RELEASE_COMMIT:-}"

[ -n "$VERSION" ] || { printf 'expected release version is required\n' >&2; exit 2; }
SEMVER="${VERSION#v}"
printf '%s\n' "$SEMVER" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$' || {
  printf 'invalid release version: %s\n' "$VERSION" >&2
  exit 2
}
if [ -z "$RELEASE_COMMIT" ]; then
  RELEASE_COMMIT="$(git -C "$ROOT" rev-parse HEAD)" || {
    printf 'could not resolve release commit\n' >&2
    exit 1
  }
fi
printf '%s\n' "$RELEASE_COMMIT" | grep -Eq '^[0-9a-f]{40}$' || {
  printf 'release commit must be exactly 40 lowercase hexadecimal characters\n' >&2
  exit 2
}
CHECKSUMS="$DIST_DIR/checksums.txt"
EXPECTED_PLATFORM_ASSETS="
dws-darwin-amd64.tar.gz
dws-darwin-arm64.tar.gz
dws-linux-amd64.tar.gz
dws-linux-arm64.tar.gz
dws-windows-amd64.zip
dws-windows-arm64.zip
"
EXPECTED_ASSETS="$EXPECTED_PLATFORM_ASSETS
dws-skills.zip
"

[ -f "$CHECKSUMS" ] || { printf 'missing checksums.txt in %s\n' "$DIST_DIR" >&2; exit 1; }

tmp="$(mktemp -d "${TMPDIR:-/tmp}/dws-release-binary.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

# The public asset namespace is an exact set. GoReleaser may also leave local
# metadata files (artifacts.json/config.yaml/metadata.json), but the upload and
# npm staging globs never publish them.
printf '%s\nchecksums.txt\n' "$EXPECTED_ASSETS" | sed '/^$/d' | LC_ALL=C sort > "$tmp/expected-root"
for path in "$DIST_DIR"/dws-*.tar.gz "$DIST_DIR"/dws-*.zip "$CHECKSUMS"; do
  [ -f "$path" ] || continue
  basename "$path"
done | LC_ALL=C sort > "$tmp/actual-root"
if ! diff -u "$tmp/expected-root" "$tmp/actual-root"; then
  printf 'public release assets must contain exactly the supported files\n' >&2
  exit 1
fi

checksum_format_ok=1
awk '
  NF != 2 { bad = 1; next }
  $1 !~ /^[0-9a-fA-F]{64}$/ { bad = 1; next }
  { print $2 }
  END { if (bad) exit 1 }
' "$CHECKSUMS" > "$tmp/checksum-assets" || checksum_format_ok=0
[ "$checksum_format_ok" -eq 1 ] || {
  printf 'checksums.txt must contain only SHA-256 and filename pairs\n' >&2
  exit 1
}

skills_checksum_count="$(awk '$2 == "dws-skills.zip" { count++ } END { print count + 0 }' "$CHECKSUMS")"
[ "$skills_checksum_count" -eq 1 ] || {
  printf 'checksums.txt must contain dws-skills.zip exactly once (found %s)\n' "$skills_checksum_count" >&2
  exit 1
}
printf '%s\n' "$EXPECTED_ASSETS" | sed '/^$/d' | LC_ALL=C sort > "$tmp/expected-checksums"
LC_ALL=C sort "$tmp/checksum-assets" > "$tmp/actual-checksums"
if ! diff -u "$tmp/expected-checksums" "$tmp/actual-checksums"; then
  printf 'checksums.txt must describe exactly the supported release assets\n' >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$DIST_DIR" && sha256sum --check checksums.txt)
elif command -v shasum >/dev/null 2>&1; then
  (cd "$DIST_DIR" && shasum -a 256 --check checksums.txt)
else
  printf 'sha256sum or shasum is required\n' >&2
  exit 1
fi

verify_binary_version() {
  asset="$1"
  extract_dir="$tmp/extract-${asset}"
  mkdir -p "$extract_dir"
  case "$asset" in
    *.tar.gz)
      tar -xzf "$DIST_DIR/$asset" -C "$extract_dir"
      ;;
    *.zip)
      unzip -q "$DIST_DIR/$asset" -d "$extract_dir"
      ;;
    *)
      printf 'unsupported release archive: %s\n' "$asset" >&2
      return 1
      ;;
  esac
  case "$asset" in
    dws-darwin-amd64*) target_os=darwin; target_arch=amd64 ;;
    dws-darwin-arm64*) target_os=darwin; target_arch=arm64 ;;
    dws-linux-amd64*) target_os=linux; target_arch=amd64 ;;
    dws-linux-arm64*) target_os=linux; target_arch=arm64 ;;
    dws-windows-amd64*) target_os=windows; target_arch=amd64 ;;
    dws-windows-arm64*) target_os=windows; target_arch=arm64 ;;
  esac
  package_name="dws-v${SEMVER}-${target_os}-${target_arch}"
  package_root="$extract_dir/$package_name"
  case "$target_os" in
    windows) legacy_name=dws.exe; core_name=dws-core.exe ;;
    *) legacy_name=dws; core_name=dws-core ;;
  esac
  for entry in "$extract_dir"/* "$extract_dir"/.[!.]* "$extract_dir"/..?*; do
    [ -e "$entry" ] || [ -L "$entry" ] || continue
    entry_name="$(basename "$entry")"
    case "$entry_name" in
      "$package_name") [ -d "$entry" ] && [ ! -L "$entry" ] || return 1 ;;
      LICENSE|NOTICE|README.md|CHANGELOG.md) [ -f "$entry" ] && [ ! -L "$entry" ] || return 1 ;;
      "$legacy_name") [ -f "$entry" ] && [ ! -L "$entry" ] || return 1 ;;
      *)
        printf '%s contains unexpected archive-root entry %s\n' "$asset" "$entry_name" >&2
        return 1
        ;;
    esac
  done
  [ -d "$package_root" ] && [ ! -L "$package_root" ] || {
    printf '%s does not contain canonical package directory %s\n' "$asset" "$package_name" >&2
    return 1
  }
  [ -f "$extract_dir/$legacy_name" ] && [ ! -L "$extract_dir/$legacy_name" ] || {
    printf '%s does not contain legacy upgrade entry %s\n' "$asset" "$legacy_name" >&2
    return 1
  }
  cmp -s "$extract_dir/$legacy_name" "$package_root/libexec/$core_name" || {
    printf '%s legacy upgrade entry differs from canonical core\n' "$asset" >&2
    return 1
  }

  (cd "$ROOT" && go run ./scripts/build/package-manifest \
    --verify \
    --package-root "$package_root" \
    --version "v$SEMVER" \
    --commit "$RELEASE_COMMIT" \
    --edition open \
    --goos "$target_os" \
    --goarch "$target_arch") >/dev/null || return 1

  case "$target_os" in
    windows)
      launcher="$package_root/bin/dws.exe"
      core="$package_root/libexec/dws-core.exe"
      ;;
    *)
      launcher="$package_root/bin/dws"
      core="$package_root/libexec/dws-core"
      ;;
  esac
  for binary in "$launcher" "$core"; do
    LC_ALL=C grep -aFq "v$SEMVER" "$binary" || {
      printf '%s binary %s does not embed expected version v%s\n' "$asset" "${binary#"$package_root/"}" "$SEMVER" >&2
      return 1
    }
  done
  LC_ALL=C grep -aFq "$RELEASE_COMMIT" "$launcher" || {
    printf '%s launcher does not embed release commit %s\n' "$asset" "$RELEASE_COMMIT" >&2
    return 1
  }
  LC_ALL=C grep -aFq "$RELEASE_COMMIT" "$core" || {
    printf '%s core does not embed release commit %s\n' "$asset" "$RELEASE_COMMIT" >&2
    return 1
  }
  core_sha="$(sed -n 's/.*"core":{[^}]*"sha256":"\([0-9a-f]*\)".*/\1/p' "$package_root/package-manifest.json")"
  if [ "${#core_sha}" -ne 64 ] || ! LC_ALL=C grep -aFq "$core_sha" "$launcher"; then
    printf '%s launcher does not embed finalized core SHA-256\n' "$asset" >&2
    return 1
  fi
  core_size="$(sed -n 's/.*"core":{[^}]*"size":\([0-9]*\).*/\1/p' "$package_root/package-manifest.json")"
  if ! printf '%s\n' "$core_size" | grep -Eq '^[1-9][0-9]*$' || ! LC_ALL=C grep -aFq "$core_size" "$launcher"; then
    printf '%s launcher does not embed finalized core size\n' "$asset" >&2
    return 1
  fi

  library="$(cd "$ROOT" && go run ./scripts/build/runtime-payload materialize \
    "$core" "$extract_dir/cache" "$target_os" "$target_arch")" || return 1
  runtime_root="$(dirname "$library")"
  [ -f "$runtime_root/manifest.json" ] || {
    printf '%s does not contain an embedded runtime manifest\n' "$asset" >&2
    return 1
  }
  ps_count="$(find "$runtime_root/ps" -type f 2>/dev/null | wc -l | tr -d ' ')"
  [ "$ps_count" = 123 ] || {
    printf '%s contains %s ps files; expected 123\n' "$asset" "$ps_count" >&2
    return 1
  }
  target="$target_os/$target_arch"
  [ -f "$library" ] || {
    printf '%s does not contain its target runtime library\n' "$asset" >&2
    return 1
  }
  manifest_library_sha="$(sed -n 's/.*"library_sha256": "\([0-9a-f]*\)".*/\1/p' "$runtime_root/manifest.json")"
  if command -v sha256sum >/dev/null 2>&1; then
    actual_library_sha="$(sha256sum "$library" | awk '{print $1}')"
  else
    actual_library_sha="$(shasum -a 256 "$library" | awk '{print $1}')"
  fi
  [ "$manifest_library_sha" = "$actual_library_sha" ] || {
    printf '%s runtime library checksum does not match its manifest\n' "$asset" >&2
    return 1
  }
  grep -Fq "\"target\": \"$target\"" "$runtime_root/manifest.json" || {
    printf '%s runtime target does not match its manifest\n' "$asset" >&2
    return 1
  }
  library_count="$(find "$runtime_root" -maxdepth 1 -type f \( -name '*.dylib' -o -name '*.so' -o -name '*.dll' \) | wc -l | tr -d ' ')"
  [ "$library_count" = 1 ] || {
    printf '%s contains %s runtime libraries; expected exactly one\n' "$asset" "$library_count" >&2
    return 1
  }
  ps_digest="$({
    find "$runtime_root/ps" -type f | LC_ALL=C sort | while IFS= read -r path; do
      if command -v sha256sum >/dev/null 2>&1; then
        file_sha="$(sha256sum "$path" | awk '{print $1}')"
      else
        file_sha="$(shasum -a 256 "$path" | awk '{print $1}')"
      fi
      printf '%s  ps/%s\n' "$file_sha" "$(basename "$path")"
    done
  } | if command -v sha256sum >/dev/null 2>&1; then sha256sum; else shasum -a 256; fi | awk '{print $1}')"
  manifest_ps_digest="$(sed -n 's/.*"ps_manifest_sha256": "\([0-9a-f]*\)".*/\1/p' "$runtime_root/manifest.json")"
  [ "${#manifest_ps_digest}" -eq 64 ] || {
    printf '%s runtime manifest has an invalid ps checksum\n' "$asset" >&2
    return 1
  }
  [ "$ps_digest" = "$manifest_ps_digest" ] || {
    printf '%s ps payload checksum mismatch\n' "$asset" >&2
    return 1
  }
}

for asset in $EXPECTED_PLATFORM_ASSETS; do
  verify_binary_version "$asset"
done

printf 'Release artifacts verified for v%s.\n' "$SEMVER"
