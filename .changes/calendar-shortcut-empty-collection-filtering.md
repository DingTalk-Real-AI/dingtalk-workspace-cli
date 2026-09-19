---
category: Fixed
---

- **Calendar shortcut empty collections** (#1095) — read shortcuts now filter
  item-level placeholders (missing/non-object entries and rows without a
  recognizable identity) from business arrays, mirroring the atomic commands'
  defensive filtering in `callSortedCalendarEvents`/`callFilteredBusyStatus`,
  instead of failing with `malformed_collection_item`. A legitimately empty
  range now projects to `count: 0` with an empty list for `calendar +agenda`,
  `+search-event`, `+attendee-list`, `+room-search`, `+room-find`,
  `+room-groups`, `+freebusy`, `+book-list`, `+book-search` and `+suggestion`,
  regardless of which empty-page sentinel shape the service returns.
  Structure-level drift (missing/non-array collection, missing pagination
  evidence) and all write-path verification still fail closed.
