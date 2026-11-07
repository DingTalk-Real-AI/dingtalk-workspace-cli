---
category: Fixed
---

- Initialize a missing platform prompt before publishing a local_agent employee, preserving existing prompts and stopping publication when draft lookup or initialization fails. Dry-run retains its request preview contract and includes the conditional steps without remote calls.
- Reject explicitly empty or whitespace-only save-draft text fields in both preview and execution, while preserving omitted fields. Clarify the coordinated server contracts for no-op draft saves and draft/published snapshot responses.
