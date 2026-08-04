# Coding Agent Task Template

Copy this block into an issue or coding-agent request. One task should have one
primary outcome; split unrelated outcomes instead of hiding them in acceptance
criteria.

```text
Task kind: bug | feature | refactor | docs | policy | release
Goal (one primary outcome):
Current behavior and evidence:
Acceptance criteria:
In scope (packages/files/surfaces):
Out of scope:
Compatibility constraints:
Interface impact (commands/flags/output/errors/exit codes/Schema):
Safety or data-mutation constraints:
Expected validation:
Known environment limitations:
```

For a command or remote-interface task, add the smallest concrete invocation
and contract evidence available:

```text
CLI path and example argv:
Current --help or Schema excerpt:
Remote method and request/response shape, if relevant:
Expected stdout/stderr and exit behavior:
Mutation preview/confirmation behavior:
```

Do not paste credentials, tokens, private endpoints, or production business
data. Use redacted fixtures and say which evidence is unavailable. Save the
filled block and run `make coding-agent-task TASK=path/to/task.md` to validate
it before implementation.
