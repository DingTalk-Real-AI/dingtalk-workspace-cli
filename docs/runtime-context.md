# Runtime context

DWS embeds runtime payload `20260908` in its single executable. Five libraries
cover macOS, Linux, and Windows on amd64 and arm64; macOS shares a universal
library. Each target includes 123 data files. Win32 is not supported.

## Materialization and recovery

The loader resolves the executable's symbolic links before choosing its parent
directory. Homebrew therefore uses the real binary in `libexec`, not the link in
`bin`. It never selects the process working directory.

```text
<resolved-executable-directory>/
  dws (or dws.exe)
  <platform-library>
  ps/<123 files>
  .dws-runtime-manifest.json
  .dws-runtime.lock
```

A nonblocking cross-process lock serializes adjacent publication. For an existing
ready bundle with the same payload digest, DWS reads the trusted manifest directly
from the verified embedded archive, compares it with the ownership record, and
checks every published resource once. Reuse does not create a staging directory
or extract another copy of the payload.

Installation and repair extract the embedded container into a hidden temporary
directory beside the executable. The dedicated ownership manifest reserves the fixed resource
names before publication (`pending`) and commits the verified result afterward
(`ready`). Existing valid resources are reused. Owned, interrupted or damaged
resources can be repaired; unknown files and symbolic links are never replaced.
Interrupted staging directories are not load sources.

If the directory cannot be resolved or written, another publisher holds its
lock, or publication fails (including a loaded Windows DLL), DWS uses a verified
content-addressed cache:

```text
<user-cache>/dws/runtime-context/20260908/<payload-sha256>/
```

Both paths verify the manifest, library checksum, all 123 data files, and their
aggregate digest before returning a library. Previous-version caches are left
alone and are not used. Failure of both locations leaves the context unavailable
without blocking login or business requests.

```mermaid
flowchart TD
    A[Embedded payload] --> B[Resolve executable symlinks]
    B --> C[Lock and verify existing adjacent resources]
    C --> L{Ready bundle matches embedded manifest?}
    L -- Yes --> F[Load verified library]
    L -- No --> M[Stage, verify and publish owned resources]
    M --> D{Publication succeeds?}
    C -- Conflict or lock failure --> E[Verify and materialize private cache]
    D -- Yes --> F
    D -- No --> E[Verify and materialize private cache]
    E --> F
    E -- Failure --> G[Continue without context]
    F --> H[Initialize once and retain immutable Result]
    H --> I[Redacted doctor diagnostics]
    H --> J[Existing business request header]
    H --> K[Browser login URL]
```

## Browser login URLs

`Result.AttachToURL(rawURL string) (string, bool)` attaches `callerUmt` and
`caller=dws` together only when the context is ready and its value is valid.
It uses URL encoding, preserves other query parameters and fragments, and
replaces duplicate parameters with one value each. Invalid HTTP(S) URLs or an
unavailable context return the original URL and `false`.

OAuth's initial browser URL and `/api/status` reauthorization URL use one
snapshot. The page consumes the complete `authorizeUrl` directly. Device Flow
resolves one snapshot before its retry loop and reuses it for up to three
attempts. The original verification response remains unchanged.

Terminal output, manual links and logs use original URLs. Browser-launch errors
report a neutral category instead of the launcher's error text. The private
value is not persisted or exposed by a token getter. Doctor reports only state,
payload version, length and a short fingerprint.

Callback and redirect URIs, device-code requests, polling, token exchange and
refresh do not receive these query parameters. The existing business-request
`x-dingtalk-ext` header behavior is unchanged. Only `k9Xm2pQv` is called; the
other seven exports are checked for integrity but are not invoked.

## Validation

Run the runtime payload policy script, targeted runtime/auth tests, six
`CGO_ENABLED=0` cross-builds, `make build`, the complete Go suite and `make policy`.
Release archives and npm/Homebrew installers continue to distribute one binary.
Payload injection precedes final code signing. Local ad-hoc signing does not
replace the official Apple Developer ID release verification.
