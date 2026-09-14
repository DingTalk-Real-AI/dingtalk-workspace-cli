# AEM Go SDK offline snapshot

This directory contains the source packages required by the DWS CLI from the
private AEM Go SDK. The root module uses a local `replace` directive so builds
do not need access to `gitlab.alibaba-inc.com`.

- Upstream module: `gitlab.alibaba-inc.com/aes/aem-go-sdk`
- Upstream version: `v0.3.0`
- Upstream commit: `2b5103b2f8899fa6e96611389c655e189a107f3b`
- Snapshot packages: `aem`, `clitrack`, `internal/encoder`, `internal/sender`

DWS carries one reviewed privacy extension on top of the upstream snapshot:
`clitrack.Config.NoAutomaticDimensions` disables the SDK's device, operating
system, locale, session, and other automatic dimensions. The official DWS
entrypoint enables this mode and tests the final encoded payload as an exact
field whitelist.

The upstream `v0.3.0` source tree did not contain a `LICENSE`, `NOTICE`, or
`COPYING` file. No replacement license text has been invented in this snapshot.
Redistribution authorization is managed by the repository owners.
