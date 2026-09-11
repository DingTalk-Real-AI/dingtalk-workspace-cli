# Runtime Payload Notice

This directory contains a redistributable runtime payload supplied for the
DingTalk Workspace CLI. The payload is distributed only as part of supported
DWS builds and must not be modified independently of its integrity manifest.

- Payload version: `20260909`
- Supported targets: macOS, Linux, and Windows on amd64 and arm64
- Distribution: five platform libraries; no `ps` directory or Win32 DLL
- Integrity sources: `manifest.json` and `SHA256SUMS`

## macOS library refresh (2026-09-11)

The macOS universal library was refreshed from the provider-supplied
`x7k2m9p4q1w8_Dynamic.zip`. The collection version remains `20260909`;
the embedded payload digest identifies this revision and triggers an upgrade
of an owned installation. Linux and Windows libraries are unchanged.

- Source archive SHA-256: `76c797afe4a58f3ff80b33e029d5ac5e7f22b4b4ed5e0eb6b5f45875b3f1fe0a`
- Library SHA-256: `bc9c5b94b710f043a448e1e89f12de7723a7d2468450f127c1de2efdc6b90742`
- Library size: 1,711,152 bytes
- Architectures: x86_64 and arm64; minimum macOS version: 11.0

The surrounding DWS source code remains licensed under Apache-2.0. The binary
payload remains subject to the rights granted by their
provider.
