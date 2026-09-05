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

The native feedback workflow runs these checks on the exact PR head for both
enabled targets. It records the source tree and build recipe, verifies core
and launcher runtime versions against the manifest, and compares the two
identity JSON files. This feedback does not authorize release cache injection.
To enforce the file-stage budgets on a saved log, run:

```sh
python3 scripts/dev/check-schema-cache-benchmark.py /path/to/file-hit.txt
```

The multiprocess candidate report exercises four independent CLI processes on
one cache directory for cold startup, damaged Meta and damaged Registry. Every
output is compared with authoritative assembly and both final artifact digests
are checked. It uses the earlier `v0.0.0-perf` candidate and does not prove newer
source changes. Its 60 process samples use a fresh, small sampler for each child. A CLI test
binary compilation overlapped part of this run: the timings are not idle-machine
performance acceptance. The report records this limitation.

Linux wait4 peak RSS includes pre-exec memory. The first native run forked from
the coordinator after retaining full JSON exports, producing identical 400 MB
RSS floors in both modes. Those RSS samples are not valid candidate acceptance.
`schema-cache-process-measure.py` now forks the measured child from a new small
interpreter; the coordinator ignores the sampler's own inherited usage. Timing
starts inside that sampler immediately before spawning the candidate. A test
retains 128 MiB in the coordinator and verifies this does not inflate child RSS.
The 100 MiB candidate gate is unchanged and needs a fresh Linux native result.

The `native-b62b2c0d/` directories contain the complete clean-source native
candidate records and reports from run 33987231960. Both platforms passed
concurrency, parity and process CPU/RSS, and their identity JSON is byte-equal.
Both failed the 5 ms Meta budget (macOS 5.054 ms; Linux 7.530 ms). The run failed;
these are development evidence only. Later thin-launcher changes are not covered.

After removing duplicate metadata maps and sorting from Meta validation, the
independent local `file-hit-subset` run measured Meta 3.770 ms / 3.87 MB and
selected product 5.332 ms / 4.32 MB. Exact alias-expansion and product-subset
validation remain covered. Linux and final signed-binary acceptance are pending.

New native candidates inject the same identity into launcher and core. Pass
`--require-schema-fast-path` to the process verifier to require exact output from
a byte-identical launcher copy in a directory with no core. This prevents a
silently delegated cache hit from masquerading as proof of the thin path.

Imported benchmark text trims trailing whitespace on the CPU model header; numerical samples are unchanged.

`2026-09-06-darwin-arm64-upstream-file-hit.txt/.json` records the seven independent warm file-hit rounds at `606b9f87` after upstream synchronization (1,370 tools, Go 1.25.9). Both local stage budgets pass; these samples are not final-process or Linux evidence.

`native-89c38222/` contains exact-head native candidate results from run `33989255990`: both core-free thin launchers pass byte parity and process CPU/RSS budgets; macOS passes file-hit budgets, Linux Meta remains 5.874 ms against 5 ms. These candidates predate upstream synchronization and the user-shortcut diagnostic fix.

`2026-09-06-darwin-arm64-dto-v2-file-hit.txt/.json` measures the DTO v2 worktree against its base commit and exact runtime source overrides: 1,370 tools, unchanged Go/protobuf toolchain, seven independent file-hit rounds. The Linux budget and final binary proof must be rerun.
