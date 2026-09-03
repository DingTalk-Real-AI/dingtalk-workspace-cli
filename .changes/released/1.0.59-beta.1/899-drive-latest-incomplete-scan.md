---
category: Fixed
---

- **Drive `--latest` refuses incomplete Top-N** (#899) — `dws drive list --latest` used to
  exit 0 with a "Top-N" computed over a partially scanned tree whenever a directory read
  failed mid-recursion (permission denied, API error), letting an incomplete set pose as the
  globally newest files. Truncation at the 2000-item scan cap and mid-recursion directory
  failures now both fail closed (`LATEST_SCAN_TRUNCATED` / `LATEST_SCAN_INCOMPLETE`), report
  the first failing folder with its depth and reason, and emit a recovery command that
  reproduces the original candidate set — query domain, `--folder`, `--pattern`, `--type`,
  `--start` and `--end` are all carried over. On POSIX shells each user-supplied value is
  quoted so a URL query string or a shell metacharacter cannot change how the copied command
  parses. On Windows no quoting form is safe for both `cmd.exe` and PowerShell, so values
  containing metacharacters are not inlined at all: the command carries a placeholder and the
  original value is shown on a separate line marked as data rather than an executable command.
  Unrecoverable errors under `--latest` return the root cause instead of a partial result.
  Remote-controlled folder names and server error text are stripped of ANSI escapes and
  control characters before they reach the plain-text stderr message. The internal `sortTime`
  sort key no longer leaks into `drive list --depth` output on any path.
