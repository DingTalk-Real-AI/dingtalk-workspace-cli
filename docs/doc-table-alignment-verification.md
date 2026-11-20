# Markdown table alignment readback

Markdown exports can omit table alignment markers even when the stored JSONML paragraphs retain the requested alignment. Comparing only the exported Markdown rejects these writes and encourages unnecessary mutation replay.

For Markdown content with explicitly aligned table columns, create, update and checkpoint verification now read both Markdown and full JSONML on each existing bounded verification attempt. JSONML must contain a root and the expected table, row and column shape. Every paragraph in each relevant cell must have the requested alignment (an omitted paragraph alignment means left). Ambiguous merged cells, missing content, incomplete readbacks, malformed JSONML and unavailable reads are not accepted. Append verifies the corresponding trailing tables; overwrite rejects extra tables.

Only after that check are the proven alignment values restored in the comparison AST. Rendered HTML equivalence and the existing structural comparison then verify the Markdown content; authored values and structure are not ignored. Mention target verification limits remain unchanged. The compact verification receipt includes the JSONML alignment source and SHA-256 rather than its complete body.

The text fingerprint also joins original AST text fragments before normalizing whitespace. A parser split at escaped punctuation must not invent an extra space. Explicit line breaks remain whitespace and authored word separation remains significant.

There are no new commands, flags, write retries, permissions or confirmation policies. JSONML content writes keep their existing verification path; Markdown without authored table alignment or highlight does not add a JSONML read. Empty creates retain metadata verification, while an empty overwrite still reads and verifies content.

Validation: fail-first positive and negative regressions, all tests in `internal/shortcut/doc`, `go vet ./internal/shortcut/doc`, and an offline historical fixture with full JSONML. Controls cover correct/absent/wrong alignment, changed values and missing body, malformed/fragment/incomplete JSONML, multiple tables, append, create, one-write behavior, read failures, empty overwrites, entity encoding and escaped arithmetic. Historical fixture bodies are not committed.


## Native highlight and literal text

DingTalk renders authored `==marked text==` as a background-highlight span. The verifier parses only authored Markdown delimiters (respecting escapes, code, link destinations and delimiter flanking), then uses the existing semantic comparison with the corresponding HTML span. It does not rewrite the submitted content. Readback Markdown is not reinterpreted as authored highlight syntax: an exporter may return literal equals without their input escapes.

Every authored highlight additionally requires a complete JSONML root. Ordered visible-text positions, inner text and the native highlight color must match; a missing/shifted highlight, wrong color, missing text, damaged literal identifier, malformed/incomplete root or failed JSONML read remains unverified. The position projection preserves normalized word boundaries and block boundaries; existing full Markdown structure/content checks still run, including table alignment. The public receipt shape and write count are unchanged. Highlight-only verification does not claim table-alignment evidence.

Use code spans or escaped equals when identifiers must retain literal `==`; unescaped authored delimiters retain native formatting semantics. Historical unescaped evidence validates format semantics, not preservation of literal identifiers. Focused regressions cover native and punctuation-adjacent highlighting, literal code/escapes, strict negative controls, append, combined aligned tables, create/update with a single mutation, and failed JSONML reads. A local historical fixture is checked offline; private bodies are never committed or sent upstream.
