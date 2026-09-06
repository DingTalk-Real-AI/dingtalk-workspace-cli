#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
cd "$root"
base_ref=${1:-}
[ -n "$base_ref" ] || {
  printf '%s\n' 'usage: check-launcher-capability-allowlist.sh BASE_REF [HEAD_REF]' >&2
  exit 2
}
head_ref=${2:-HEAD}
git rev-parse --verify "${base_ref}^{commit}" >/dev/null
git rev-parse --verify "${head_ref}^{commit}" >/dev/null

changed=$(git diff --name-only "$base_ref" "$head_ref" -- \
  cmd/dws-launcher/main.go \
  internal/launcher/capabilities.go internal/launcher/help_tracking.go \
  internal/launcher/schema.go internal/launcher/version_tracking.go \
  internal/schemafastpath/schema.go)
[ -n "$changed" ] || exit 0

required='docs/rfc-schema-runtime-cache.md
internal/launcher/dependencies_test.go
internal/launcher/launcher_test.go'
printf '%s\n' "$required" | while IFS= read -r path; do
  if git diff --quiet "$base_ref" "$head_ref" -- "$path"; then
    printf '%s\n' "launcher capability changed without reviewed contract update: $path" >&2
    exit 1
  fi
done

grep -Fq 'TestCrossPlatformCoverageCapabilityAllowlist' internal/launcher/launcher_test.go
grep -Fq 'TestCrossPlatformCoverageLauncherRuntimeDependencies' internal/launcher/dependencies_test.go
grep -Fq '新增快路径或扩大现有 argv/环境适用范围的合入顺序固定为' docs/rfc-schema-runtime-cache.md
