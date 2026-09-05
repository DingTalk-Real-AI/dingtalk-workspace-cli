# Schema cache candidate measurements

These artifacts record development evidence, not production release acceptance.
The JSON binds the measured launcher and core by SHA-256 and preserves all 60
interleaved process samples. Both modes explicitly set `DO_NOT_TRACK=1`; the live
mode additionally sets `DWS_SCHEMA_CACHE_DISABLE=1` on the same executable.

From the repository root, on native macOS arm64 or Linux amd64:

```sh
python3 scripts/dev/build-schema-cache-candidate.py --output /absolute/new/candidate
python3 scripts/dev/verify-schema-cache-binary.py \
  --binary /absolute/new/candidate/dws-v0.0.0-schema-cache-candidate-darwin-arm64/bin/dws \
  --proof /absolute/new/candidate/identity.json \
  --output /absolute/new/candidate/process-report.json
GOTOOLCHAIN=go1.25.9 go test ./internal/cli/schemaruntime \
  -run '^$' -bench '^BenchmarkRealSchemaFileHit$' -benchtime=1s -count=7
```

Use `linux-amd64` in the package directory on Linux. Run performance measurements
after other builds/tests have finished. The builder refuses to reuse an existing
output directory, pins Go 1.25.9, injects the runtime payload, finalizes the core
before pinning its digest into the launcher, and writes the package manifest.
macOS signatures are ad-hoc; no Developer ID credentials or notarization are used.

The file benchmark includes secure directory traversal and close, envelope and
payload authentication, protobuf decoding/conversion, and lookup/index work.
The selected-product stage starts from already authenticated Meta; add the Meta
stage for a first query. These are warm OS page-cache measurements. Each printed
`ns/op` is a Go benchmark average, not a per-invocation percentile.

The initial September 6 candidate predates the Meta comparator optimization and
the missing-user-cache-directory fix. Its process CPU/RSS gates passed. Its Meta
file-hit median across seven benchmark averages was 5.95 ms, exceeding the RFC's
5 ms budget. Later source changes require a fresh candidate and new measurements.
Default telemetry, competitive public/native entry, hostile-environment sandbox,
Linux native evidence, and final release signing remain separate gates.

After replacing reflective Meta equality with exact field comparisons, the
optimized file-hit run (after the full test process ended) measured 4.52 ms /
6.36 MB per Meta load and 5.30 ms / 4.33 MB per selected product load, using
the median of seven benchmark averages. Both local file-stage budgets passed.
