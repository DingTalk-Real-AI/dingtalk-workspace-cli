#!/usr/bin/env bash
set -euo pipefail

layer="source"
binary=""
out=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --layer) layer="${2:?missing --layer value}"; shift 2 ;;
    --binary) binary="${2:?missing --binary value}"; shift 2 ;;
    --out) out="${2:?missing --out value}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
if [[ -z "$out" ]]; then
  out="$(mktemp -d "${TMPDIR:-/tmp}/dws-card-acceptance.XXXXXX")"
fi
mkdir -p "$out"
out="$(cd "$out" && pwd)"

run_source() {
  cd "$repo_root"
  DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/card/a2ui/... -count=1
  DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/helpers -run '^TestCrossPlatformCoverageCard' -count=1
  printf '{"layer":"source","status":"PASS"}\n' > "$out/source.json"
}

ensure_binary() {
  if [[ -n "$binary" ]]; then
    binary="$(cd "$(dirname "$binary")" && pwd)/$(basename "$binary")"
    return
  fi

  # Do not create and immediately execute an anonymous binary under /tmp.
  # Endpoint protection treats that pattern as low reputation and may prompt
  # on every new hash. Build through the repository's normal pipeline at its
  # stable, ignored path; the temporary output directory remains data-only.
  binary="$repo_root/dws"
  cd "$repo_root"
  DWS_PACKAGE_VERSION=0.0.0-test make build
}

run_dws() {
  # Schema assembly is intentionally broad. Keep the disposable acceptance
  # process inside a predictable footprint on developer machines that already
  # run DingTalk, browsers, and IDEs; callers can override either value.
  GOMEMLIMIT="${DWS_CARD_ACCEPTANCE_GOMEMLIMIT:-512MiB}" \
    GOGC="${DWS_CARD_ACCEPTANCE_GOGC:-20}" \
    "$binary" "$@"
}

run_preview() {
  ensure_binary
  local spec="$repo_root/internal/helpers/testdata/a2ui-form-spec.json"
  local rich_spec="$repo_root/internal/helpers/testdata/a2ui-information-spec.json"
  run_dws card compose --file "$spec" --output "$out/messages.json" --format json > "$out/compose.json"
  run_dws card lint --file "$out/messages.json" --mode create --format json > "$out/lint.json"
  run_dws card preview --file "$out/messages.json" --output "$out/preview.html" --format json > "$out/preview.json"
  run_dws card compose --file "$rich_spec" --output "$out/information-messages.json" --format json > "$out/information-compose.json"
  run_dws card lint --file "$out/information-messages.json" --mode create --format json > "$out/information-lint.json"
  run_dws card preview --file "$out/information-messages.json" --output "$out/information-preview.html" --format json > "$out/information-preview.json"

  require_text "$out/preview.html" 'data-binding-path="/form/version"'
  require_text "$out/preview.html" 'data-binding-path="/form/platforms"'
  require_text "$out/preview.html" 'data-binding-path="/form/environment"'
  require_text "$out/preview.html" '<button type="button" class="ButtonControl primary" data-event-name="form_submit"'
  require_text "$out/preview.html" 'new CustomEvent("dws-a2ui-preview-action"'
  require_text "$out/preview.html" 'id="interaction-result"'
  reject_text "$out/preview.html" ' disabled'
  reject_text "$out/preview.html" 'fetch('
  reject_text "$out/preview.html" 'XMLHttpRequest'
  reject_text "$out/preview.html" '<textarea class="TextField"'
  reject_text "$out/preview.html" '<div class="ChoicePicker">'
  reject_text "$out/preview.html" '### 变更摘要'
  local interaction_execution="unavailable"
  if command -v node >/dev/null 2>&1; then
    node "$repo_root/scripts/card/preview-interaction-check.mjs" "$out/preview.html" > "$out/interaction.json"
    interaction_execution="executed"
  fi
  require_text "$out/information-preview.html" 'data-component="Image"'
  require_text "$out/information-preview.html" 'data-component="File"'
  require_text "$out/information-preview.html" '<blockquote>'
  require_text "$out/information-preview.html" '<h2>核心结论</h2>'
  require_text "$out/information-preview.html" '--space-2:8px'
  require_text "$out/information-preview.html" '--space-3:12px'
  require_text "$out/information-preview.html" '.Card>.Column{gap:var(--space-2)}'
  require_text "$out/information-preview.html" '.CollapsiblePanel details{padding:0 var(--space-3)}'
  require_text "$out/information-preview.html" '.CollapsiblePanel summary::-webkit-details-marker{display:none}'
  require_text "$out/information-preview.html" '.CollapsiblePanel details[open]>summary::before{transform:rotate(45deg)}'
  require_text "$out/information-preview.html" '.PanelContent{position:relative;margin:0;padding:0;overflow:auto}'
  require_text "$out/information-preview.html" '.InlineIcon{display:inline-flex;width:16px;height:16px'
  require_text "$out/information-preview.html" '.ButtonControl{border-radius:999px'
  require_text "$out/information-preview.html" '.token-common_level2_base_color{color:#4c5561}'
  require_text "$out/information-preview.html" '.CollapsiblePanel.indented .PanelContent{padding-left:calc(var(--space-2) + 1px)}'
  require_text "$out/information-preview.html" '.CollapsiblePanel.indented .PanelContent::before{position:absolute;top:var(--space-3);bottom:var(--space-3);left:0;width:1px;border-radius:1px;background:#d9dde3;content:""}'
  reject_text "$out/information-preview.html" '.CollapsiblePanel details[open]>summary{border-bottom:'
  reject_text "$out/information-preview.html" 'list-style-position:outside'
  for component in Button File Markdown TextField Card Row Column Tag Divider Image Text ChoicePicker; do
    if ! grep -Fq "data-component=\"$component\"" "$out/preview.html" "$out/information-preview.html"; then
      echo "acceptance failed: component $component is absent from the preview matrix" >&2
      exit 1
    fi
  done
  bash "$repo_root/scripts/card/render-recipe-gallery.sh" --binary "$binary" --out "$out/gallery" > "$out/gallery.log"
  require_text "$out/gallery/gallery-status.json" '"status":"PASS"'
  printf '{"layer":"preview","status":"PASS","cards":5,"components":12,"previewKind":"reference_preview","interactionSimulation":true,"interactionExecution":"%s","gallery":"gallery/index.html","remoteSideEffects":false,"realRenderer":false}\n' "$interaction_execution" > "$out/preview-status.json"
}

require_text() {
  local file="$1"
  local expected="$2"
  if ! grep -Fq -- "$expected" "$file"; then
    echo "acceptance failed: $file does not contain $expected" >&2
    exit 1
  fi
}

reject_text() {
  local file="$1"
  local rejected="$2"
  if grep -Fq -- "$rejected" "$file"; then
    echo "acceptance failed: $file unexpectedly contains $rejected" >&2
    exit 1
  fi
}

case "$layer" in
  source) run_source ;;
  preview) run_preview ;;
  all) run_source; run_preview ;;
  *) echo "unsupported layer: $layer (use source, preview, or all)" >&2; exit 2 ;;
esac

echo "A2UI acceptance artifacts: $out"
