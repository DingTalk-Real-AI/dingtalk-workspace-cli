# Bundled script recipes

Use this file only for dashboard/chart, import, export, bulk fields, or attachment workflows.
Use `scripts/aitable_ops.py` as the stable entry; do not read implementation files. Use operation-level `--help` only for unclear arguments.

For create-Base-then-script, create and verify the Base, then pass its returned `baseId`; choose a short name if omitted.

| Intent | Exact command |
|---|---|
| Create dashboard, optionally charts | `python3 <Skill绝对目录>/scripts/aitable_ops.py dashboard <baseId> "<dashboardName>" [--chart-specs-file <workspace JSON file>]` |
| Import CSV/XLS/XLSX as a new table | `python3 <Skill绝对目录>/scripts/aitable_ops.py import-new <baseId> <file>` |
| Append JSON/CSV to an existing table | `python3 <Skill绝对目录>/scripts/aitable_ops.py import-records <baseId> <tableId> <file> [--batch-size 100]` |
| Export Base/table/view | `python3 <Skill绝对目录>/scripts/aitable_ops.py export <baseId> --scope all\|table\|view [--table-id <id>] [--view-id <id>] [--output <path>]` |
| Add up to 15 fields | `python3 <Skill绝对目录>/scripts/aitable_ops.py add-fields <baseId> <tableId> <fields.json>` |
| Upload attachment | `python3 <Skill绝对目录>/scripts/aitable_ops.py upload-attachment <baseId> <file>` |

## Contracts

- `dashboard` creates the dashboard/charts, chains returned IDs, performs final readback, and emits `dws-skill-script-ledger/v1`. `--chart-specs-file` is a workspace-local file containing one JSON array; it is not inline JSON. Each item accepts `name`, `chart_type`, `table_id`, `measure_type`, `measure_field_id`, `dimension_field_id`, `aggregation`, and `view_id`. Do not add a guessed dashboard command after it.
- `import-new` runs prepare → secure PUT → import task and checks each business status. A returned HTTP upload URL is upgraded to HTTPS without changing host, path, or query. It is not interchangeable with `import-records`, and a deterministic failure is not a reason to expand the same flow manually.
- `import-records` requires CSV headers to be field IDs; use JSON for typed boolean/array/object values. It checks batch results and reads back returned record IDs.
- `export --scope table|view` requires `--table-id`; view also requires `--view-id`. The unified ledger exposes the real `taskId`, `polledTimes`, `savedPath`, and `fileSize`; treat success plus a non-empty saved file as the completion evidence, without reading source or substituting `ls` for task polling. Do not overwrite unless the user explicitly requests it.
- `add-fields` accepts at most 15 items and reports partial failure. `upload-attachment` only returns `fileToken`; write that token to the attachment field and read the record back.
- Preserve nonzero exit, partial-success ledger, timeout, and incomplete readback in the final answer. Do not replace a failed deterministic workflow with guessed atomic commands unless its reported error proves that the script contract is unavailable.
