# Schema Command Execution Issue Fixes

This document records issues found while packaging, building, and executing commands from `dws schema`, plus the fixes and verification evidence.

## 2026-07-09

### Verification summary

- Built local CLI with `./scripts/dev/build.sh`.
- Executed all commands discovered from `dws schema --format json` with `--dry-run --yes --format json`: 359 total, 359 passed.
- Wrote smoke artifacts to `tmp/schema-command-smoke.jsonl` and `tmp/schema-command-smoke.md`.
- Ran Go tests: `go test ./internal/cli ./internal/compat ./internal/output ./internal/helpers ./test/contract ./test/scripts -count=1`.
- Ran full packaging: `make package`; generated platform archives, `dist/dws-skills.zip`, npm package staging, and Homebrew formula staging.

### Issue 1: `make package` could not resolve package version

- Symptom: `make package` built cross-platform archives, then `scripts/release/post-goreleaser.sh` failed with `could not resolve package version - set DWS_PACKAGE_VERSION or create a git tag`.
- Cause: `scripts/dev/build-all.sh` defaulted `VERSION` to `0.0.0-SNAPSHOT`, but the Makefile did not pass that version into `post-goreleaser.sh` as `DWS_PACKAGE_VERSION`.
- Fix: Added `VERSION ?= 0.0.0-SNAPSHOT` to `Makefile` and passed it to both build and post-packaging steps.
- Verification: `make package` completed successfully and staged release artifacts under `dist/`.

### Issue 2: packaging tests used fake Darwin archives with signing enabled

- Symptom: `go test ./test/scripts -run TestPostGoreleaser` failed when `post-goreleaser.sh` tried to unpack and sign fake archive fixtures.
- Cause: tests seed placeholder archive files, while post-packaging now performs Darwin ad-hoc signing that requires real tar archives containing a binary.
- Fix: Added `DWS_SKIP_DARWIN_SIGNING=1` for controlled packaging tests and set explicit `DWS_PACKAGE_VERSION` in those tests.
- Verification: `go test ./test/scripts -count=1` passed.

### Issue 3: helper schema exposed flags that commands do not accept

- Symptom: smoke validation found schema parameters missing from command help, including `dev.create_dev_app --app-type` and `dev.publish_dev_app_version --precheck-only`.
- Cause: helper-only schema was rendered from live MCP parameters without intersecting those parameters with the actual Cobra command flags.
- Fix: Filtered live helper parameters to command-registered flags, preserved aliases/hints, and added schema tests for the helper command tree.
- Verification: full schema smoke passed with no `schema_flag_mismatch` results.

### Issue 4: dry-run was not consistently honored through inherited/root flags

- Symptom: several commands attempted real execution during smoke even though `--dry-run` was present, notably dynamic command and compat pipeline paths.
- Cause: dynamic command execution only checked local flags in some paths, and compat pipeline dispatch did not short-circuit dry-run before calling the pipeline runner.
- Fix: Added inherited/root persistent flag lookup for `--dry-run` and made compat pipeline routes emit a dry-run plan instead of executing.
- Verification: full schema smoke passed, including previously failing `chat message download-media` and `minutes record` paths.

### Issue 5: schema omitted real business `--json` flags

- Symptom: aitable view update commands accepted `--json`, but schema omitted it for some commands.
- Cause: runtime schema filtering treated all `json`/`params` names as generic payload plumbing, which also removed hardcoded business flags named `json`.
- Fix: Restricted generic payload filtering to exact generic usage text so command-specific `--json` flags remain visible.
- Verification: `dws schema aitable.view_update_filter --format json` includes `json`, and full schema smoke passed.

### Issue 6: required-parameter inference was incomplete or too aggressive

- Symptom: dry-run execution failed on commands whose schema did not mark the minimum executable parameter set, while some conditional fields were inferred as unconditionally required.
- Cause: schema generation relied on usage text only, without enough hints for command-specific minimum payloads or exclusions for conditional phrasing.
- Fix: Added command-specific schema hints for aitable/calendar/dev/group-chat commands and refined required inference to ignore conditional phrases such as optional, mutually exclusive, and at-least-one wording.
- Verification: targeted rerun of the 20 previously failing paths passed, then full schema smoke passed 359/359.

### Issue 7: smoke harness generated empty payloads for non-empty update commands

- Symptom: after schema fixes, a few aitable commands still failed smoke because generated values were syntactically valid but semantically empty, such as `--records []` or `--json {}`.
- Cause: the smoke harness had generic JSON samples and did not know which commands require non-empty objects or arrays.
- Fix: Generated non-empty record payloads and command-specific non-empty JSON samples for aitable aggregate/card/field-widths/timebar updates.
- Verification: targeted smoke for the remaining 20 paths passed 20/20, and the rebuilt CLI passed full smoke 359/359.

### Issue 8: mail user smoke depended on bound mailbox auto-detection

- Symptom: `mail.search_mail_users` failed in dry-run smoke on accounts without an enterprise mailbox, even after `dws auth login`, because the command attempted mailbox auto-detection when `--email` was omitted.
- Cause: schema correctly marks `email` as optional for normal use, but the smoke harness used required parameters only; this made dry-run depend on the tester account's mailbox binding.
- Fix: Added a smoke-only extra flag rule so `mail.search_mail_users` supplies a sample `--email` while leaving the runtime schema unchanged.
- Verification: `dws mail user search --email user@example.com --keyword keyword-smoke --dry-run --yes --format json` passed; full smoke passed 359/359 after this fix.
