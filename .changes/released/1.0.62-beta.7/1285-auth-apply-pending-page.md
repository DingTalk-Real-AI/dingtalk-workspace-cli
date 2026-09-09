---
category: Changed
---

- **CLI auth apply pending page** (#1285) — the browser apply flow now lands on a
  dedicated approval-pending page that polls and auto-redirects after approval;
  duplicate apply requests are idempotent, and all local callback pages and API
  responses are served with `Cache-Control: no-store` to avoid stale state after
  a page refresh.
- **CLI access denial copy** (#1285) — terminal denial reasons are now split by
  whether the path is applyable. `cli_not_enabled` keeps the apply flow and shows
  a personal-scope message ("you do not yet have CLI data access") with the
  approver picker relabeled to "select approver"; the inline success message was
  replaced by a redirect to the pending page. The non-applyable `user_forbidden`
  and `user_not_allowed` reasons now share a single consolidated terminal message
  ("this organization has not enabled CLI data access") across the browser page and
  both login transports (OAuth browser flow and device flow).
