# OpenHarmony support boundary

This repository supports a **compile-only** OpenHarmony target:

- target: `openharmony/arm64`
- toolchain: caller-provided executable Go 1.26.7 toolchain
- CGO: disabled (`CGO_ENABLED=0`)
- distribution: no official release, npm, or Homebrew artifact

This support proves that the CLI can be compiled as a static AArch64 ELF with the
public source tree. It does not claim real-device validation, production support,
or compatibility with every OpenHarmony system image.

## Build

Set `OHOS_GO` to an executable Go toolchain that reports `openharmony/arm64` from
`go tool dist list`:

```bash
OHOS_GO=/path/to/openharmony-go \
  DWS_PACKAGE_VERSION=1.2.3 \
  ./scripts/dev/build-openharmony.sh
```

The script uses `GOTOOLCHAIN=local`, `-trimpath`, and `CGO_ENABLED=0`. It injects
the version, full lowercase Git commit, and reproducible UTC build time into the
binary, then runs `scripts/dev/verify-openharmony-artifact.sh`.

Optional environment variables:

- `OHOS_OUTPUT` — output path; defaults to `dist/dws-openharmony-arm64`.
- `DWS_PACKAGE_VERSION` — version marker; defaults to `dev`.
- `DWS_GIT_COMMIT` — full lowercase commit SHA; it must resolve in the checkout.
- `DWS_BUILD_TIME` — UTC build timestamp; when omitted, it is derived from the
  selected commit.

The verifier fails closed unless the artifact is executable, ELF `ET_EXEC`,
AArch64, static, and free of both an interpreter and a dynamic section. It also
checks Go metadata for `GOOS=openharmony`, `GOARCH=arm64`, and `CGO_ENABLED=0`.

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

SafeChat is unavailable on OpenHarmony. Its vendor library is a CGO/static-library
backend supported only by the repository's Darwin, Linux, and Windows amd64/arm64
builds. OpenHarmony uses the existing stub: `Available()` is false and attempts
to open the backend return `ErrUnavailable`.

Do not add a private SafeChat binary, NDK, header, or crypto fallback to enable it.

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

OpenHarmony is intentionally excluded from GoReleaser, GitHub Release assets,
npm packages, and Homebrew formulas. The official release matrix remains the six
Darwin, Linux, and Windows amd64/arm64 targets. A caller may build and validate an
OpenHarmony binary locally with its own toolchain, but this repository does not
provide or host that toolchain.
