#!/bin/sh
set -eu

PACKAGE_DIR="${1:-}"
OUTPUT="${2:-}"

[ -n "$PACKAGE_DIR" ] && [ -n "$OUTPUT" ] || {
  printf 'usage: pack-npm-package.sh <package-dir> <output.tgz>\n' >&2
  exit 2
}
[ -f "$PACKAGE_DIR/package.json" ] || {
  printf 'npm package manifest not found: %s/package.json\n' "$PACKAGE_DIR" >&2
  exit 1
}
command -v npx >/dev/null 2>&1 || { printf 'npx is required\n' >&2; exit 1; }
command -v node >/dev/null 2>&1 || { printf 'node is required\n' >&2; exit 1; }
NPM_PACK_VERSION="10.9.2"

output_dir="$(CDPATH= cd -- "$(dirname -- "$OUTPUT")" && pwd)"
output_name="$(basename -- "$OUTPUT")"
rm -f "$output_dir/$output_name"

pack_json="$(
  npx --yes --package "npm@$NPM_PACK_VERSION" -- \
    npm pack "$PACKAGE_DIR" --pack-destination "$output_dir" --json --ignore-scripts
)"
pack_metadata="$(printf '%s' "$pack_json" | node -e '
let input = "";
process.stdin.on("data", (chunk) => { input += chunk; });
process.stdin.on("end", () => {
  const entries = JSON.parse(input);
  if (!Array.isArray(entries) || entries.length !== 1) {
    throw new Error("npm pack did not return exactly one package");
  }
  const entry = entries[0];
  if (!entry.filename || !entry.integrity) {
    throw new Error("npm pack output is missing filename or integrity");
  }
  process.stdout.write(`${entry.filename}\n${entry.integrity}\n`);
});
')"
packed_name="$(printf '%s\n' "$pack_metadata" | sed -n '1p')"
integrity="$(printf '%s\n' "$pack_metadata" | sed -n '2p')"
[ -f "$output_dir/$packed_name" ] || {
  printf 'npm pack did not create expected tarball: %s\n' "$packed_name" >&2
  exit 1
}
if [ "$packed_name" != "$output_name" ]; then
  mv "$output_dir/$packed_name" "$output_dir/$output_name"
fi
printf '%s\n' "$integrity"
