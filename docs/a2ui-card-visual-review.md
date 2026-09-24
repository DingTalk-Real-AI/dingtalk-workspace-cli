# Built-in A2UI visual review

`dws card review` ships its review rubric with DWS. Authors do not need a
separate design Skill or plugin. It checks the A2UI file offline and returns
protocol validation, deterministic visual diagnostics, author questions, and
client checks as structured JSON:

```bash
dws card review --file card.a2ui.json --design-archetype report
dws card review --file card.a2ui.json --design-archetype approval --minimum-validation-width 360
```

`dws card send` runs the same deterministic visual preflight before any remote
call and includes `visualReview` in dry-run and send results. Use the same
context flags on `send` as on `review`. A deterministic visual error blocks the
send; warnings describe risks for the author to assess. The CLI does not
invent the business meaning of a metric or infer the intended hierarchy from
component count. The author checks ask whether the main fact, relationships,
status meaning, copy, and actions are clear.

`staticStatus` reports `pass`, `pass_with_warnings`, or `fail`. The separate
`status: needs_client_review` and `clientRenderingVerified: false` remain even
when static checks pass. Inspect the final card in the real target client at
the intended desktop and mobile widths and in applicable themes. The local
`card preview` is a structural reference; a send receipt does not prove visual
quality or client rendering. Record a screenshot and concrete observations
outside this command when completing visual acceptance.
