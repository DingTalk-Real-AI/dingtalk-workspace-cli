#!/bin/bash
# scripts/dev/fetch_mcp_metadata.sh
#
# Optional diagnostic: pulls live MCP tools/list and writes a local dump.
# This is NOT a Schema delivery input. schema_mcp_metadata.json is retired;
# production Schema assembles from Contract/ParamDecl/Interface + Cobra only.
#
# Prerequisites:
#   dws auth login   (valid access token required)
#
# Usage:
#   make fetch-mcp-metadata
#   # or directly:
#   scripts/dev/fetch_mcp_metadata.sh
#   scripts/dev/fetch_mcp_metadata.sh /tmp/mcp-tools-dump.json
#
# The script tries to extract the access token from the DWS auth store.
# If that fails (e.g. keychain-based storage), set DWS_ACCESS_TOKEN manually:
#   export DWS_ACCESS_TOKEN=<your-token>
#   scripts/dev/fetch_mcp_metadata.sh

set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

OUTPUT="${1:-artifacts/mcp_metadata_diagnostic.json}"
case "$OUTPUT" in
	internal/cli/schema_mcp_metadata.json|*/internal/cli/schema_mcp_metadata.json)
		echo "Error: refusing to write retired Schema pin $OUTPUT" >&2
		echo "  Use a diagnostic path under artifacts/ instead." >&2
		exit 1
		;;
esac

# --- 1. Check auth ---
AUTH_JSON=$(dws auth status --format json 2>/dev/null || echo '{}')
if ! echo "$AUTH_JSON" | grep -q '"authenticated": *true'; then
    echo "Error: not authenticated. Run 'dws auth login' first." >&2
    exit 1
fi
echo "✓ Authenticated." >&2

# --- 2. Try to extract access token ---
CONFIG_DIR="${DWS_CONFIG_DIR:-$HOME/.dws}"
TOKEN_FILE="$CONFIG_DIR/token"

if [ -z "${DWS_ACCESS_TOKEN:-}" ] && [ -f "$TOKEN_FILE" ]; then
    # Try file-based token store (plaintext JSON)
    DWS_ACCESS_TOKEN=$(python3 -c "
import json, sys
try:
    d = json.load(open('$TOKEN_FILE'))
    t = d.get('accessToken') or d.get('access_token') or ''
    print(t)
except:
    pass
" 2>/dev/null || echo "")
fi

if [ -z "${DWS_ACCESS_TOKEN:-}" ]; then
    echo "Warning: could not auto-extract access token." >&2
    echo "  Token file: $TOKEN_FILE" >&2
    echo "  If using keychain, set DWS_ACCESS_TOKEN manually:" >&2
    echo "    export DWS_ACCESS_TOKEN=<your-token>" >&2
    echo "  Continuing without auth (may fail)..." >&2
else
    echo "✓ Got access token (${#DWS_ACCESS_TOKEN} chars)." >&2
fi

export DWS_ACCESS_TOKEN

mkdir -p "$(dirname "$OUTPUT")"

# --- 3. Run the Go tool (diagnostic dump only) ---
go run ./cmd/fetch_mcp_metadata \
    -output "$OUTPUT" \
    2>&1

echo "" >&2
echo "Diagnostic dump written to $OUTPUT (not a Schema delivery input)." >&2
