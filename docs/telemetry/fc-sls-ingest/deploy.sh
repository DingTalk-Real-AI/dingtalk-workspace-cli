#!/usr/bin/env bash
# Deploy the dws telemetry ingest to Alibaba Cloud Function Compute via Serverless Devs.
#
# Prereqs (one-time):
#   npm i -g @serverless-devs/s
#   s config add                 # add an Aliyun credential alias named "default"
#
# Usage:
#   INGEST_TOKEN=<random> ./deploy.sh                      # dry-run-first deploy (no SLS)
#   INGEST_TOKEN=<random> SLS_ENDPOINT=cn-hangzhou.log.aliyuncs.com \
#     SLS_PROJECT=dws-ops SLS_LOGSTORE=dws-telemetry ./deploy.sh   # write to SLS
set -euo pipefail
cd "$(dirname "$0")"

: "${INGEST_TOKEN:?set INGEST_TOKEN to a random shared secret (must match dws-side DWS_TELEMETRY_TOKEN)}"
export SLS_ENDPOINT="${SLS_ENDPOINT:-}" SLS_PROJECT="${SLS_PROJECT:-}" SLS_LOGSTORE="${SLS_LOGSTORE:-}"

echo "==> building (installs flask/gunicorn/aliyun-log from requirements.txt)"
s build --use-docker || s build

echo "==> deploying"
s deploy -y

echo "==> function info (look for the http trigger URL)"
s info

echo
echo "Verify:"
echo "  curl <URL>/                 # expect: mode=dry-run (or mode=sls once SLS_* set)"
echo "  curl -XPOST <URL>/ -H \"Authorization: Bearer \$INGEST_TOKEN\" \\"
echo "    -H 'Content-Type: application/json' -d '{\"schema_version\":\"1\",\"command\":\"doc\",\"outcome\":\"ok\",\"duration_ms\":42}'"
echo "Then set DWS_TELEMETRY_URL=<URL> + DWS_TELEMETRY_TOKEN=\$INGEST_TOKEN (or bake via ldflags)."
