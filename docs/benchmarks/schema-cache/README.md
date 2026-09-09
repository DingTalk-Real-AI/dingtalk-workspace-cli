# Schema cache measurements

This directory is a how-to, not an archive. Do not commit per-commit suite
dumps, `*-local/` run trees, dated `json`/`txt`/`gz` reports, or other
scratch evidence here. Write those to a throwaway path or CI artifact.

These scripts record development evidence, not production release acceptance.
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

To enforce the file-stage budgets on a saved log, run:

```sh
python3 scripts/dev/check-schema-cache-benchmark.py /path/to/file-hit.txt
```

New candidates inject the same identity into launcher and core. Pass
`--require-schema-fast-path` to the process verifier to require exact output from
a byte-identical launcher copy in a directory with no core. This prevents a
silently delegated cache hit from masquerading as proof of the thin path.

Linux wait4 peak RSS includes pre-exec memory. `schema-cache-process-measure.py`
forks the measured child from a new small interpreter; the coordinator ignores
the sampler's own inherited usage. Timing starts inside that sampler immediately
before spawning the candidate. A test retains 128 MiB in the coordinator and
verifies this does not inflate child RSS. The 100 MiB candidate gate is unchanged.

Default telemetry, competitive public/native entry, hostile-environment sandbox,
Linux native evidence, and final release signing remain separate gates. Reviewed
numbers for the current architecture live in
[`docs/rfc-schema-runtime-cache-performance.md`](../../rfc-schema-runtime-cache-performance.md).
