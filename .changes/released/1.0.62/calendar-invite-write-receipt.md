---
category: Fixed
---

- **Calendar invitation receipts** — fixes false `readback_attendee_missing` failures from `calendar +invite` and `calendar +book --with` when participant responses omit user IDs and display names differ from directory names. Invitations require an explicit successful write receipt and report `verified=false`; `+invite` adds `acknowledged=true`, while `+book --with` adds `attendeesAcknowledged=true` and retains event readback through `eventVerified=true`.
