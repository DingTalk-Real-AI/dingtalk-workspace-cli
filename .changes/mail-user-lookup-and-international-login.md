---
category: Added
---

- **Mail employee lookup by corporate email** — adds `mail user get --org-email`
  and `mail user batch-get --org-emails`, with Schemas for
  `mail.get_user_by_org_email` and `mail.batch_get_users_by_org_emails`.
  Batch lookup accepts 1–100 addresses before deduplication and returns matched
  employees plus `notFoundOrgEmails`. Both declare required input mappings,
  read-only safety, and result fields, and use platform-injected operator and
  organization identity.
- **Mail batch partial results** — preserves confirmed employee and not-found
  results when individual inputs or response records fail. Partial output
  separates `succeeded`, `failed` and `unknown` entries (exit code 7). An explicit
  server rejection of an invalid batch address falls back to at most 100
  deduplicated single-address lookups, including errors classified by the
  runtime transport; global authentication, permission and
  connection failures do not trigger that fallback. Unconfirmed results are
  never reported as not-found.
  During fallback, global failures retain their original classification and
  exit status, with confirmed progress in structured error
  `details.partialResult`. Cancellation and raw PAT authorization errors are
  returned to the framework unchanged instead of becoming partial success.
- **Large integer output precision** — preserves integer IDs beyond the exact
  `float64` range when decoding MCP text responses and through `--jq`, `--fields`
  and formatted output.
- **International login is English-first** — `dws auth login --intl` now renders
  its terminal copy, the DingTalk authorization page (`lang=en-US`) and the
  callback success page in English without requiring `DWS_LANG=en`. A locale
  inherited from `LANG` no longer forces Chinese output; set `DWS_LANG=zh`
  explicitly to keep the Chinese copy. Domestic (`.com`) login keeps following
  `DWS_LANG`/`LANG` as before.
