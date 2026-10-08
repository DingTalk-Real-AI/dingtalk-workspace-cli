# Go validator implementation contract

`lint` and `explain` run in-process in Go against protocol resources embedded in the binary. They do not require an installed Skill, Python, or a Profile. `preview` combines local validation, current-user lookup, and the existing send API. Build the DWS checkout after installing this module and verify its new binary.

This file is maintained at `dws-aicard/internal/card/a2ui/README.md` in the `dingtalk-aicard` source repository and installed at `internal/card/a2ui/README.md` in `dingtalk-workspace-cli`. Unless explicitly identified as a DWS path, `spec/`, `shared/`, `tools/`, and `skills/dingtalk-aicard/scripts/` below belong to the `dingtalk-aicard` repository. Those directories and Python scripts are not installed into DWS.

## Protocol and structural validation

The public distribution is rooted at `spec/`; the upstream maintenance source is `card-docs/protocol/a2ui/open/`. `tools/sync_protocol.py --source` imports upstream; without a source it rebuilds only from this repository. DingTalk AI Card V0.8 is based on A2UI 1.0, while message `version` remains `v1.0`. `shared/.protocol-sync.json` records portable provenance, versions, and digests. Self-check must cover the message envelope, seven Catalog shards, three common-type files, and internal validation rules.

Structural validation requires a standard JSON Schema Draft 2020-12 engine supporting `$ref`, `oneOf`, `anyOf`, `allOf`, `if/then`, and `unevaluatedProperties`. Python uses `jsonschema` with `referencing.Registry`; standard-library `re` and the bundled Unicode 17 character tables support the current protocol's XID expression without changing the Schema. Character-table digests use normalized content so wrapping may differ. References resolve only against bundled resources; external references fail without network access.

Validate complete messages through `agent-to-renderer.json`. Its `Component` definition combines shared fields with the Catalog for complete components. Do not validate a component through a Catalog root or a handwritten interpreter that implements only some schema keywords. General lint checks protocol structure only; it adds no card semantics or design rules.

## Input and validation boundaries

- By default, accept a nonempty message array. `--fragment` additionally wraps one component, a component array, or one message.
- Check JSON syntax and schema-defined properties, required fields, types, enums, combined constraints, and DingTalk extensions.
- General lint neither merges data nor executes functions. It does not check roots, reference closure, cycles, duplicate IDs, binding initial values, inferred return types, design effects, or delivery state.
- Add no limits on JSON, component-tree, or function depth beyond the protocol. If resources prevent completion, report validation as incomplete instead of incorrectly declaring the input valid or invalid.
- Emit no design or runtime warnings. DWS does not offer `--strict`, `--lock-file`, or `lint --explain`; Python keeps `--strict` for compatibility.
- Agent authoring guidance remains separate from client rendering and interaction evidence.

## Output contract

```json
{"valid":false,"diagnostics":[{"code":"schema.type_mismatch","severity":"error","pointer":"/0/updateComponents/components/0/bold","message":"Field type mismatch","hint":"Correct the field according to the bundled protocol","keyword":"type","schemaPointer":"/properties/bold/type"}],"metrics":{},"renderingVerified":false}
```

`metrics` is reserved for output compatibility and is currently always an empty object.

`pointer` addresses the original input and retains JSON Pointer `~0` and `~1` escaping; do not invent paths such as `/components/<id>`. A missing required field points to its containing object. Locate unknown fields individually. Structural diagnostics may include `keyword` and `schemaPointer`.

Structural errors use `schema.*` codes such as `type_mismatch`, `missing_required`, `unknown_property`, and `bad_enum`. General lint does not emit `reference.*`, `binding.*`, `action.*`, or design diagnostics. Code and fixtures maintain the exact contract; this document does not duplicate a brittle diagnostic-code count.

## Explicit new-card preflight

`--preflight new-card` checks initialization and the final component snapshot after structural validation. Top-level `valid` still represents only Schema validity. Components are isolated by Surface and merged across messages: a later message may replace an ID, but duplicate IDs within one message are errors. Protocol-derived `childRefs` locate the root, reachable static references, template targets, and static cycles. Dynamic template edges do not participate in static cycle detection; preflight neither expands templates nor evaluates relative bindings.

Unreachable components produce only a `reference.unreachable_component` warning; their subgraphs are not checked as part of the current root tree. Only errors invalidate `preflight.valid` or block preview. Binding values, template expansion, intermediate frames, and client effects remain unverified. The `resources` mode does not require an incremental update to be reference-closed on its own.

Python reads expected codes, severities, and original-input pointers from `dws-aicard/internal/card/a2ui/testdata/preflight-references.json` in `dingtalk-aicard`; Go reads the installed copy at `internal/card/a2ui/testdata/preflight-references.json` in DWS. Their entry points are the source repository's `tools/test_preflight_references.py` and DWS's `TestCrossPlatformCoverageAicardReferencePreflightSharedCases`.

Python exits 0 for success, 1 for invalid input, and 2 for file, environment, protocol, or incomplete-validation errors. Even a `--format json` failure emits structured output. DWS uses its `{ok,outcome,data,error,meta}` envelope and exit codes: successful reports are in `data`, validation failures in `error.details`, and environment errors carry no validity conclusion. `tools/conformance.py` checks both output and exit code.

The Python reference distinguishes `input.dependencies_unavailable` (missing dependency), `input.python_environment_unavailable` (dependency import failure), and `input.protocol_unavailable` (damaged protocol). `setup_env.py` prepares the environment and returns `pythonExecutable`; lint never installs dependencies implicitly. A damaged protocol must not be relabeled as a dependency-installation problem. `input.validation_incomplete` means local resources were insufficient to reach a validation conclusion.

`dws aicard explain <name>` parses one generated `explain.json` into a name table with raw JSON definitions, then decodes the requested definition. It does not initialize the validator or compile Schema. This is capability-level lazy loading, not per-file loading: the complete bundle is embedded and its JSON syntax is parsed on first lookup. `lint --self-check` first checks the exact explain bundle SHA-256 against the independent digest in validator assets, then checks every definition, protocol manifest, references and Schema compilation. Missing, extra, or truncated definitions fail even when the protocol manifest is unchanged. Named lookups still decode only the requested definition. Generation checks verify exact bundle content. Minimal examples for all 47 components must pass fragment validation.

## Verification

Run the native checks from the DWS repository root. `make build` creates the `./dws` binary used below.

```bash
make build
DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/card/a2ui -count=1
DWS_PACKAGE_VERSION=0.0.0-test go test ./internal/helpers -run Aicard -count=1
./dws aicard lint --self-check --format json
```

On macOS, if the linker cannot find an SDK library, rerun the build with `SDKROOT="$(xcrun --sdk macosx --show-sdk-path)" make build`.

Run Python reference and differential checks from the `dingtalk-aicard` repository root. The commands below assume the two checkouts are siblings; adjust `../dingtalk-workspace-cli/dws` if they are elsewhere. `setup_env.py` returns the dedicated interpreter as `pythonExecutable`.

```bash
AICARD_PYTHON="$(python3 skills/dingtalk-aicard/scripts/setup_env.py | python3 -c 'import json,sys; result=json.load(sys.stdin); assert result["ready"]; print(result["pythonExecutable"])')"
test -x "$AICARD_PYTHON"
"$AICARD_PYTHON" tools/test_aicard_lint.py
"$AICARD_PYTHON" tools/test_preflight_references.py
"$AICARD_PYTHON" tools/build_go_assets.py --check
"$AICARD_PYTHON" tools/conformance.py
"$AICARD_PYTHON" tools/conformance.py --cmd '../dingtalk-workspace-cli/dws aicard lint --file {file} --format json' --explain-cmd '../dingtalk-workspace-cli/dws aicard explain {name} --format json'
```

Conformance checks the exact registered file set under `spec/` and the generated-artifact hashes in `shared/.protocol-sync.json`. Use a source checkout whose files match that manifest; unrelated files under `spec/` or stale generated assets cause a drift error before validator comparisons begin. See `CONTRIBUTING.md` in `dingtalk-aicard` for the source rebuild workflow.

The baseline covers DingTalk examples, invalid structural cases, structurally valid runtime/design boundaries, pattern regression fixtures in `shared/fixtures/patterns/`, official-compatibility cases, synthesized examples, indexes, and DWS Skill consistency. `shared/fixtures/expectations.json` locks expected validity, diagnostic severity, code, and pointer; missing or unregistered inputs fail. `DIVERGENCES` explicitly records DingTalk differences from official tests. The current DingTalk Schema is authoritative; do not relax it merely to match upstream outcomes.

Go `--emit` returns only encoded message strings; it writes no file and cannot be combined with `--fragment`. `--self-check` independently checks embedded resources. Preview applies the same initialization, public catalogId, and reference rules as explicit `new-card` preflight without changing general lint.

`tools/build_go_assets.py` generates one explain bundle and separate validator metadata with Unicode XID ranges from the Python reference. No per-name files, custom offset index or compressed storage format are needed. The DWS Skill contains no Python scripts. Go uses `jsonschema/v6` and `regexp2`. Internal rules are synthesized on copies, and constant branches are evaluated early to avoid repeatedly computing recursive arguments without changing successful-branch annotations. Generator `--check` and full conformance detect drift.

## Supported integration baseline

The official module is `github.com/DingTalk-Real-AI/dingtalk-workspace-cli`.
The installer defaults to the sibling `../dingtalk-workspace-cli`; for a custom
checkout or Git worktree always specify `--dws /absolute/path/to/checkout`.
Compatibility acceptance targets official main with this extension installed.
The inspected main baseline is `7de51a87`; feature-branch results do not prove
released-version compatibility. All installer API seam checks must pass,
including `RegisterPublicNamed`, a supported `RuntimeSchemaConstraints`
struct or alias, typed help documentation and the test execution API.
An older checkout missing those APIs is rejected before writing; `--force`
does not bypass the API checks. Use a compatible checkout instead of weakening
preflight or copying generated Skill files to simulate native command support.

From the AICard repository, inspect the plan with
`python3 tools/install_to_dws.py --dws /absolute/path/to/checkout --check`, then
run the same command without `--check` to install and verify. Verification runs
`go build ./...`, helper vet, all A2UI package tests, and helper tests matching
`Aicard`. The helper filter is separate from the unfiltered validator package.
`--check` alone proves neither compilation nor installed runtime behavior.

## Base64 compatibility change

Resource preflight emits `resource.base64_image_unverified` with severity
`warning` for inline Base64 images. Relative to master, this adds a warning; master did not reject these images. Valid encoding
alone does not prove image decoding, client support or transport size safety.
Malformed data URIs and invalid/empty Base64 retain their existing errors.
Prefer HTTPS for new authoring; this policy does not change public Schema
validity or impose an invented per-image byte limit.
