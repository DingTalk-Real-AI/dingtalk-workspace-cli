# AEM Go SDK offline snapshot

This directory contains the source packages required by the DWS CLI from the
private AEM Go SDK. The root module uses a local `replace` directive so builds
do not need access to `gitlab.alibaba-inc.com`.

- Upstream module: `gitlab.alibaba-inc.com/aes/aem-go-sdk`
- Upstream version: `v0.4.0`
- Upstream commit: `4a6f824d78312308359f85fa474897430a61681e`
- Snapshot packages: `aem`, `clitrack`, `internal/encoder`, `internal/sender`

DWS carries these explicit local extensions on top of the upstream snapshot:

- `clitrack.Config.NoAutomaticDimensions` and the core
  `disable_auto_dimensions` option suppress device, operating system, locale,
  session, and other automatic dimensions.
- `clitrack.Config.NoAttribution` suppresses both probing and publication of
  p2/p3. DWS enables both privacy controls and tests the final encoded payload
  against the existing exact field whitelist.
- `clitrack.Execution`, `Tracker.ReportExecution`, and `Tracker.Close` support
  reporting a completed command from a detached sender. The original command,
  duration, timestamp, exit code, and sanitized error are retained, without
  printing or exiting. Reserved fields still cannot be overwritten through
  `ExtraFields`.

The root command process does not initialize the tracker. The DWS sender uses
the blocking `Close` only in its isolated, time-limited process. The upstream
`Run` API and its default 300 ms flush behavior remain available for other SDK
callers. See `docs/clitrack-integration.md` in the DWS repository for the actual
entrypoint contract. Run `make test-aem` from the repository root to exercise
the upstream and local SDK tests; this target is also part of `make policy`.

The upstream `v0.4.0` source tree did not contain a `LICENSE`, `NOTICE`, or
`COPYING` file. No replacement license text has been invented in this snapshot.
Redistribution authorization is managed by the repository owners.
