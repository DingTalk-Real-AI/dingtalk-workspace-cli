# OpenHarmony support

This repository supports the `openharmony/arm64` target:

- target: `openharmony/arm64`
- toolchain: go1.26.7 OpenHarmony toolchain (see Provisioning)
- CGO: release builds enable the SafeChat crypto backend via the pinned
  OpenHarmony NDK (`DWS_OPENHARMONY_CGO=1`); the binary stays a static
  self-signed ELF (`-extldflags -static`, no PT_INTERP/DYNAMIC/NEEDED)
- packaging: the release pipeline builds, self-signs, and verifies the package
  for every beta and stable release
- distribution: `dws-openharmony-arm64.tar.gz` ships as a GitHub Release asset
  (beta and stable); no npm or Homebrew artifact

The resulting binary has been verified on a HarmonyOS PC (AArch64, HongMeng
Kernel): CLI startup, `--help`, and authenticated chat commands all work.
This is not yet production support — validate on your own devices before
relying on it.

## Build

Set `OHOS_GO` to an executable Go toolchain that reports `openharmony/arm64` from
`go tool dist list`:

```bash
OHOS_GO=/path/to/openharmony-go \
  DWS_PACKAGE_VERSION=1.2.3 \
  ./scripts/dev/build-openharmony.sh
```

The script uses `GOTOOLCHAIN=local` and `-trimpath`, injecting the version,
Git commit, and reproducible UTC build time. `DWS_OPENHARMONY_CGO` selects the
channel: `0` builds a pure-Go static `ET_EXEC` (SafeChat stub, `-d -s -w`);
`1` links the SafeChat backend through the pinned OpenHarmony NDK clang with
`-extldflags -static` (still no `PT_INTERP`/`PT_DYNAMIC`/`NEEDED`); `auto`
picks `1` when the NDK is available. `ohos-cgo-probe.sh` isolates
toolchain/NDK link problems from the full build.

Optional environment variables:

- `OHOS_OUTPUT` — output path; defaults to `dist/dws-openharmony-arm64`.
- `DWS_PACKAGE_VERSION` — version marker; defaults to `dev`.
- `DWS_GIT_COMMIT` — full lowercase commit SHA; it must resolve in the checkout.
- `DWS_BUILD_TIME` — UTC build timestamp; when omitted, it is derived from the
  selected commit.

The verifier fails closed unless the artifact is executable, ELF `ET_EXEC`,
AArch64, static, and free of both an interpreter and a dynamic section. It also
checks Go metadata for `GOOS=openharmony` and `GOARCH=arm64` (CGO mode 0 or 1).

## Provisioning the toolchain

When `OHOS_GO` is unset, the build script provisions the toolchain from an
archive via `scripts/dev/provision-ohos-go.sh`:

```bash
OHOS_GO_ARCHIVE=/path/to/ohos-go.tar.gz \
  DWS_PACKAGE_VERSION=1.2.3 \
  ./scripts/dev/build-openharmony.sh
```

- `OHOS_GO_ARCHIVE` — path or HTTPS URL of a `.tar.gz` containing the toolchain
  (any layout with a `bin/go` inside).
- `OHOS_GO_SHA256` — expected SHA-256; **required for HTTPS URLs**, validated
  for local files when set.
- `OHOS_GO_ROOT` — cache location; defaults to
  `${TMPDIR:-/tmp}/dws-ohos-go`. A validated toolchain is reused on later runs.

This repository does not host the toolchain. Obtain a go1.26.7 OpenHarmony
toolchain (one whose `go tool dist list` includes `openharmony/arm64`) from the
OpenHarmony SIG / Huawei distribution channels, and prefer pinning
`OHOS_GO_SHA256` for reproducible builds.

## Packaging and self-signing

To produce a self-signed OpenHarmony package archive (`dws-openharmony-arm64.tar.gz`)
and its SHA-256 checksum:

```bash
OHOS_GO=/path/to/openharmony-go \
  DWS_PACKAGE_VERSION=1.2.3 \
  ./scripts/dev/package-openharmony.sh
```

The packaging script:

1. Compiles `dws` using `scripts/dev/build-openharmony.sh` with `-d -s -w` to produce a pure static `ET_EXEC` ELF without dynamic loader or glibc dependencies.
2. Compiles and runs the pure-Go OpenHarmony code signer (`cmd/binary-sign-tool`), which embeds a 4096-aligned `.codesign` section with fs-verity SHA-256 root hash and descriptor self-signature (`-selfSign 1`).
3. Enforces static ELF properties when `DWS_REQUIRE_ELF_STATIC=1` (no `PT_INTERP`, no `PT_DYNAMIC`, no `NEEDED`).
4. Bundles `dws`, `LICENSE`, `NOTICE`, `README.md`, and `CHANGELOG.md` with deterministic file timestamps into `dist/openharmony/dws-openharmony-arm64.tar.gz`.
5. Emits `dist/openharmony/checksums.txt`.

## Capability limits

### SafeChat

SafeChat message crypto ships in OpenHarmony release builds: the vendor
`lib/openharmony_arm64/libsafechat.a` is linked by the CGO channel above, the
same model as the Darwin, Linux, and Windows builds. Pure-Go builds
(`DWS_OPENHARMONY_CGO=0`) keep the fail-closed stub: `Available()` is false and
opening the backend returns `ErrUnavailable`. The pinned NDK comes from the
`ohos-ndk-26.0.0.851-toolchain` fork release; its SHA-256 is verified before
use (`provision-ohos-ndk.sh`).

### Runtime native payload

The native runtime payload and its cache are unsupported on OpenHarmony. The
OpenHarmony build must remain independent of private runtime libraries and must
not be added to the release payload matrix.

### Authentication storage

The portable file-backed keychain implementation is available to OpenHarmony and
reuses the same encrypted local storage model as the other file-backed targets.
Platform keychains or native integrations that are unavailable on OpenHarmony are
not implied by this statement.

## Release boundary

Every beta and stable release publishes `dws-openharmony-arm64.tar.gz` (with its
SHA-256 recorded in `checksums.txt`) as a GitHub Release asset. The package is
built and self-signed by `scripts/release/build-openharmony-release.sh` from the
pinned OpenHarmony toolchain; GoReleaser still owns only the six Darwin, Linux,
and Windows amd64/arm64 targets because it cannot drive the OpenHarmony
toolchain. OpenHarmony stays out of npm packages and Homebrew formulas, and this
repository does not provide or host the toolchain.
