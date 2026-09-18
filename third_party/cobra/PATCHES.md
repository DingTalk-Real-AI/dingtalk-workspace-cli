# DWS Cobra dependency patch

Based on `github.com/spf13/cobra v1.10.2` from the Go module cache, verified by
the module checksum retained in the repository's root `go.sum`:

```text
h1:DMTTonx5m65Ic0GOoRY2c16WCbHxOOw6xxezuLaBpcU=
```

The local replacement includes all root Go sources and tests, the complete
`doc` package, original module files, license, README, contribution and security
documents. Website/assets and other non-package repository files are omitted.
`UPSTREAM.sha256` records every included upstream file before patching.

The production changes are recorded in `command-validation.patch`. When a
parent fails to parse flags, `Traverse` invokes that parsing command's effective
`FlagErrorFunc` once. A non-nil handler result replaces the parser error; nil
retains the original parser error and stops execution. Both cases return the
parsing command instead of nil, so `ExecuteC` returns the correct error owner
and prints that command's help path. Its early error branch respects both the
root's and the parsing command's `SilenceErrors` settings.

The upstream `TestTraverseWithBadParentFlags` expectation is updated in the same
reversible patch to require the parsing command. Additional DWS dependency tests
cover direct traversal, execution, handler fallback, help attribution and all
root/group silence combinations. Remaining arguments and the successful
traversal path are unchanged. No DWS imports or classification policy are
introduced into Cobra.

The traversal patch was approved on 2026-09-06. The subsequent review extends
the patch for late-created commands with `ValidationStage` and
`SetValidationErrorFunc` / `ValidationErrorFunc`. Native execution invokes the
nearest handler only when Args (including Find’s legacyArgs path), required
flags or flag groups fail. A nil
handler result preserves the original failure; without a handler all existing
upstream behavior remains unchanged. Successful validation and business hooks
do not call it. DWS installs its classifier from corecmd and removes early
PreRun constraint checks; lazily generated commands inherit the same adapter.
Public standalone ValidateArgs/ValidateRequiredFlags/ValidateFlagGroups and
direct hook calls retain their upstream behavior outside Execute/ExecuteC.

This replaces the
earlier design constraint of keeping Cobra unchanged; native `Execute` /
`ExecuteC`, parent local flags and traversal semantics remain supported.

Validation from the repository root:

```sh
./scripts/policy/check-cobra-patch.sh
./scripts/policy/check-typed-validation-errors.sh
```

The first gate reverses the documented patch in a temporary copy, verifies the
upstream file hashes, and runs all Cobra and `doc` tests (including the new
dependency regression). The second additionally proves DWS typed error
behavior and success-path semantics via real `ExecuteC` calls. Root
`go test ./...` alone does not recurse into this nested module.
The `Cobra compatibility` workflow runs this gate plus generated-declaration,
assembly-determinism and Schema gates for affected pull requests, including
Drafts, and changes on main. It supplements the existing admission checks
without changing their rules.

When upgrading Cobra, compare upstream traversal and native validation behavior, rerun both
gates and the application suite, and refresh provenance explicitly. Remove the
local replacement when upstream provides the required failure-stop and inherited validation-adapter semantics;
do not stack unrelated changes in this copy.

Builds must retain the root module's replacement and this directory. An overlay
using a separate main module must explicitly select this patched dependency
and run the same boundary gate; dependency-module `replace` directives are not
inherited by another main module. Check the effective selection with
`go list -m -json github.com/spf13/cobra`.
