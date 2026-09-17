---
category: Fixed
---

- **OA approval event fields** — preserve `staff_id`, `activity_id`, `corp_id`, and `business_id` from the business payload in `event consume --flatten` for all seven OA approval events, and expose `cc_time` for approval instance CC events. Optional fields remain omitted when absent; an explicitly provided zero `cc_time` is preserved.
