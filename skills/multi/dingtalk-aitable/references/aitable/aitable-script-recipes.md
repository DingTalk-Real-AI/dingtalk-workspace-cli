# Bundled script recipes

Use this file only for dashboard/chart, import, export, bulk fields, or attachment workflows.
The stable entry point is `scripts/aitable_ops.py`; do not read it or the delegated `.py` files before execution. Run `python3 <Skill绝对目录>/scripts/aitable_ops.py --help` or the operation-level `--help` only when an argument is unclear.

For a create-Base-then-script workflow, create the Base first and take `baseId` from its structured response. If the user did not specify the container name, choose a short descriptive name and continue instead of asking only for that name. Verify the Base with `dws aitable base get --base-id <baseId> --format json`, then pass that exact `baseId` to this entry point.

| Intent | Exact command |
|---|---|
| Create dashboard, optionally charts | `python3 <Skill绝对目录>/scripts/aitable_ops.py dashboard <baseId> "<dashboardName>" [--chart-specs <workspace JSON>]` |
| Import CSV/XLS/XLSX as a new table | `python3 <Skill绝对目录>/scripts/aitable_ops.py import-new <baseId> <file>` |
| Append JSON/CSV to an existing table | `python3 <Skill绝对目录>/scripts/aitable_ops.py import-records <baseId> <tableId> <file> [--batch-size 100]` |
| Export Base/table/view | `python3 <Skill绝对目录>/scripts/aitable_ops.py export <baseId> --scope all\|table\|view [--table-id <id>] [--view-id <id>] [--output <path>]` |
| Add up to 15 fields | `python3 <Skill绝对目录>/scripts/aitable_ops.py add-fields <baseId> <tableId> <fields.json>` |
| Upload attachment | `python3 <Skill绝对目录>/scripts/aitable_ops.py upload-attachment <baseId> <file>` |

## Contracts

- `dashboard` creates the dashboard/charts, chains returned IDs, performs final dashboard/chart readback, and emits `dws-skill-script-ledger/v1`. Do not add a second guessed dashboard command after it. `--chart-specs` is a JSON array; each item accepts `name`, `chart_type`, `table_id`, `measure_type`, `measure_field_id`, `dimension_field_id`, `aggregation`, and `view_id`.
- `import-new` runs prepare → HTTPS PUT → import task and checks each business status. It is not interchangeable with `import-records`.
- `import-records` requires CSV headers to be field IDs; use JSON for typed boolean/array/object values. It checks batch results and reads back returned record IDs.
- `export --scope table|view` requires `--table-id`; view also requires `--view-id`. The unified ledger exposes the real `taskId`, `polledTimes`, `savedPath`, and `fileSize`; treat success plus a non-empty saved file as the completion evidence, without reading source or substituting `ls` for task polling. Do not overwrite unless the user explicitly requests it.
- `add-fields` accepts at most 15 items and reports partial failure. `upload-attachment` only returns `fileToken`; write that token to the attachment field and read the record back.
- Preserve nonzero exit, partial-success ledger, timeout, and incomplete readback in the final answer. Do not replace a failed deterministic workflow with guessed atomic commands unless its reported error proves that the script contract is unavailable.
