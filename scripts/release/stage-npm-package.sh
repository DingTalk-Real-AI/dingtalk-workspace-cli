#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
DEFAULT_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
SOURCE_ROOT="${DWS_PACKAGE_SOURCE_ROOT:-$DEFAULT_ROOT}"
DIST_DIR="${DWS_PACKAGE_DIST_DIR:-$DEFAULT_ROOT/dist}"
VERSION="${1:-${DWS_PACKAGE_VERSION:-}}"

[ -n "$VERSION" ] || { printf 'package version is required\n' >&2; exit 2; }
SEMVER="${VERSION#v}"
printf '%s\n' "$SEMVER" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$' || {
  printf 'invalid npm package version: %s\n' "$VERSION" >&2
  exit 2
}

PKG_ROOT="$DIST_DIR/npm/dingtalk-workspace-cli"
rm -rf "$PKG_ROOT"
mkdir -p "$PKG_ROOT/assets" "$PKG_ROOT/bin"

cp "$SOURCE_ROOT/build/npm/install.js" "$PKG_ROOT/install.js"
cp "$SOURCE_ROOT/build/npm/bin/dws.js" "$PKG_ROOT/bin/dws.js"
cp "$SOURCE_ROOT/build/npm/README.md" "$PKG_ROOT/README.md"
sed "s|__VERSION__|$SEMVER|g" "$SOURCE_ROOT/build/npm/package.json.tmpl" > "$PKG_ROOT/package.json"

command -v node >/dev/null 2>&1 || { printf 'node is required to validate the npm manifest\n' >&2; exit 1; }
node - "$PKG_ROOT/package.json" "$SEMVER" <<'NODE'
const fs = require("fs");
const [manifestPath, version] = process.argv.slice(2);
const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
const expectedScripts = { postinstall: "node install.js" };
if (manifest.name !== "dingtalk-workspace-cli") {
  throw new Error(`unexpected npm package name: ${manifest.name}`);
}
if (manifest.version !== version) {
  throw new Error(`unexpected npm package version: ${manifest.version}`);
}
if (JSON.stringify(manifest.scripts) !== JSON.stringify(expectedScripts)) {
  throw new Error(`unexpected npm lifecycle scripts: ${JSON.stringify(manifest.scripts)}`);
}
if (!manifest.bin || manifest.bin.dws !== "./bin/dws.js") {
  throw new Error("unexpected npm binary entrypoint");
}
NODE

copied=0
for artifact in "$DIST_DIR"/dws-*.tar.gz "$DIST_DIR"/dws-*.zip "$DIST_DIR"/checksums.txt; do
  [ -f "$artifact" ] || continue
  cp "$artifact" "$PKG_ROOT/assets/"
  copied=$((copied + 1))
done
[ "$copied" -gt 0 ] || { printf 'no release assets found in %s\n' "$DIST_DIR" >&2; exit 1; }
[ -f "$PKG_ROOT/assets/dws-skills.zip" ] || { printf 'dws-skills.zip is missing\n' >&2; exit 1; }
[ -f "$PKG_ROOT/assets/checksums.txt" ] || { printf 'checksums.txt is missing\n' >&2; exit 1; }

printf 'Staged npm package %s at %s\n' "$SEMVER" "$PKG_ROOT"
