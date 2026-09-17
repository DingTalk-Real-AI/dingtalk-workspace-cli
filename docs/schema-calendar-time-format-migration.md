# Calendar time format migration

Calendar event create/update accept RFC3339 timestamps for timed events and
`yyyy-MM-dd` dates for all-day events. Publishing only `format: date-time` rejects
the latter; omitting all format information loses a useful machine-readable
contract.

`contract.ParamDecl.AnyOf` declares format-only alternatives for a string flag.
For calendar time parameters the delivered parameter keeps `type: string` and
publishes:

```json
{"anyOf":[{"format":"date"},{"format":"date-time"}]}
```

The declaration is validated before annotations are applied. Alternatives are
unique, nonempty and sorted for deterministic delivery; they cannot coexist with
an explicit top-level format. The declaration flows through typed assembly,
field provenance and the snapshot adapter. It supersedes single-format help
inference. This limited facility is not an arbitrary JSON Schema overlay.
Runtime validation and help determine which format applies for `is-all-day`.

The base-owned Schema checker preserves `anyOf` in normalized contracts and
rejects unreviewed changes. `compatibleReviewedCalendarTimeFormats` authorizes
only these one-way changes:

| Tool | Parameters | Before | After |
| --- | --- | --- | --- |
| `calendar.create_calendar_event` | `start`, `end` | `format: date-time` | exact date/date-time `anyOf`; no top-level format |
| `calendar.update_calendar_event` | `start`, `end` | `format: date-time`, no `required_when` | same union; `required_when: is-all-day is explicitly provided (true or false)` |

Authorization also requires the expected primary CLI path, string type and RPC
property, plus a newly introduced optional boolean `is-all-day` parameter mapped
to `isAllDay`, without a published default. Every other published time-parameter
field must be identical. Other tool-level checks still run. Removing the format
without the union, changing the union, reversing the migration, changing defaults
or requiredness, or using another tool/parameter is not authorized.

This follows the existing `reviewedParameterTypeChanges` approach: land the
framework support and exact authorization in main first, with the calendar
surface unchanged. Only then may the calendar feature consume the declaration.
The authoritative script builds the checker from the Git-owned base, so a
candidate cannot authorize itself. No workflow, required check or branch
protection is disabled. Future migrations require separate review.
