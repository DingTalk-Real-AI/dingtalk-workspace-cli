# External code login: review and verification

This change is based on `v0.0.0-build.17.1` (`82cd8959`). It makes
`auth exchange` usable in an empty employee sandbox: accept an externally
issued code, verify the employee identity online, and persist credentials
that subsequent DWS processes can use.

## Review scope

- `internal/app/auth_exchange.go`: public help, stdin input, application
  selection flags, expected identity assertions, dry-run, and safe output.
- `internal/auth/external_exchange.go`: resolve the application before
  consuming the code; preserve existing default profiles under the save lock.
- `internal/auth/managed_exchange.go`: share the verified exchange/persistence
  core while retaining managed login's required identity and supervisor checks.
- `internal/auth/oauth_helpers.go`: direct OAuth with explicit credentials,
  without persisting application configuration before identity verification.
- Tests cover incomplete/mismatched identities, input conflicts, application
  selection, refresh, and later commands using the saved employee profile.

`--mcp` is removed without an alias. Callers must use `--client-id default`
for the official application or a concrete matching application ID. Stored
`Source=mcp` is intentionally retained for refresh routing. See
[the command guide](auth-exchange.md) for the three entry points and examples.

## Revision 2: reviewer findings addressed

- ClientSecret now joins the existing locked Token/Profile snapshot and rollback.
  Failure-injection cases cover secret, identity, profile, and marker writes,
  both with and without a previously saved secret. Snapshot/read failures stop
  the operation; preflight failures do not consume a code.
- External app configuration is loaded uncached from the supplied `configDir`.
  Tests deliberately poison the global cache and use different directories.
- Direct credentials retain actual `flag` / `app` / `env` / `default` provenance
  after persistence. Managed credentials retain `Source=mcp`.
- `SanitizeCommand` masks authorization codes in both space-separated and
  equals-sign forms. Nine real-binary fixture cases cover success/failure/dry-run
  across both argument forms and stdin, checking stdout/stderr/performance JSON.
- `--mcp` remains removed as the agreed interface change; callers must migrate.

The latest local binary is `0.0.0-build.17.1-exchange.local.2`. Final focused auth
and app tests and the native binary regression passed. At 15:15 on 2026-09-10
(Asia/Shanghai), a newly issued real test-employee code was exchanged in empty,
independent configuration/credential directories. Separate processes passed
`auth status`, `contact user get-self`, `contact user get`, `calendar book list`,
and explicit-profile `get-self`; the employee identity matched and the original
supervisor's default profile was preserved. No supervisor credentials were
copied into the sandbox. The code came from the DWS employee endpoint, not HSF.

The two Helpers tests reported by the reviewer passed unchanged on this Mac:
`TestExecForwarderStreamRetriesMissingSessionOnce` and
`TestExecForwarderDoesNotRetryNonSessionErrors`. This is local evidence only;
it does not identify the cause of the reviewer's failures. Generated drift and
assembly determinism passed. An earlier full-suite/Schema attempt was stopped
following a visible enterprise security prompt. No security controls were
disabled; final retry runs invoke executables natively without launch adapters
or test overlays. Final retry results:

- `go test ./internal/auth -run 'TestExternalExchange|TestManagedExchange' -count=1 -timeout=90s`: PASS.
- `go test ./internal/app -run 'TestSanitizeCommand|TestAuthExchange' -count=1 -timeout=90s`: PASS.
- `go test ./internal/auth -count=1 -timeout=120s`: PASS (entire auth package).
- `python3 scripts/dev/test-auth-exchange-local.py --binary <local.2-binary>`: PASS, native execution, including all nine leak checks.
- `./scripts/policy/check-schema-catalog.sh`: PASS, unmodified script and assertions,
  no overlay or architecture adapter (28 products, 1039 tools; CLI, homology,
  Helpers and app gates passed).
- The complete repository-wide suite was interrupted earlier and was not
  completed on this revision; do not carry forward the historical 103-package
  count as this revision's result. Its remaining coverage and the known fork
  workflow-asset failures must be reviewed before release.

Tested local.2 binary SHA256:
`b8c880e941bb2e5abba0eadf7df2e29f45dbe8f1b6f4e97edbfd2b7df4fa06ef`.
The real fresh token had not naturally expired at this verification time;
simulated refresh and real fresh-token commands are separate evidence.

## Previous revision verification (local.1, historical)

- Local macOS arm64 package: `0.0.0-build.17.1-exchange.local.1`.
- Full Go suite: 103 packages passed, including the affected auth, app,
  helpers, and CLI packages. `test/scripts` had 33 failing cases concerning
  missing workflow/release/open-source assets in this fork; the full suite
  is therefore not green. Those assets were not added or tests disabled.
- Generated drift, assembly determinism, and Schema policy passed
  (28 products / 1039 tools). On the test Mac, temporary Go executables
  intermittently exited with signal 9; the successful policy run used explicit
  `arch -arm64` launch adapters, including a temporary test overlay for a nested
  generator launch. No assertions or repository policy files were changed.
- The isolated localhost binary regression passed for default, explicit,
  and omitted client IDs. Separate processes exercised refresh and normal
  `contact user get-self` calls using default and explicit profiles.
- A real new employee authorization code was exchanged using the packaged
  binary in empty configuration and credential directories, with no copied
  supervisor credentials. Separate processes successfully ran `auth status`,
  `contact user get-self`, `contact user get --ids <employee-user-id>`, and
  `calendar book list`. The returned identity matched the target employee;
  its own primary calendar reported owner access. Exact-profile selection
  also passed. The original supervisor's default profile was preserved.
- Persisted profile and encrypted token metadata contained matching real
  client ID, corporation ID, and employee user ID; both access and refresh
  credentials were present and valid. No codes, tokens, employee identifiers,
  or credential stores are included in this review document.

## Remaining release validation

The real code came from the DWS employee authorization endpoint, not HR's HSF
issuing entry point. HR must still validate its code/application pairing,
deployment environment, and required business permissions. The newly exchanged
real token had not naturally expired at verification time: simulated refresh
passed, and a separate pre-existing employee profile refreshed successfully,
but neither proves natural-expiry refresh of this particular new token.

This commit prepares the development branch for review; it does not constitute
a production release or completion of HR's target-environment acceptance.
