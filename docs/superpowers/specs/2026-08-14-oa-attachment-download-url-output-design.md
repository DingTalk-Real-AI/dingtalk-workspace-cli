# OA Attachment Download URL Output Design

## Goal

Keep the existing command and MCP request unchanged while making the returned
OSS signed URL directly copyable from JSON output:

```text
dws oa approval attachment download-url
```

## Scope

Only `oa approval attachment download-url` changes. The other OA attachment
commands and the global JSON formatter retain their current behavior.

## Design

The command continues to invoke MCP server `oa`, tool
`get_attachment_download_url`, with the same arguments. Its leaf declaration
provides a command-specific `Call` callback that invokes the existing MCP
dispatcher with HTML escaping disabled when the selected output format is
JSON. This preserves literal `&` separators in `result.downloadUri` instead of
rendering them as `\u0026`.

For `raw`, `table`, and other non-JSON formats, the callback uses the existing
escaped dispatcher behavior so their current rendering remains unchanged.

The change does not alter the URL, decode or re-sign it, download the file, or
change global JSON serialization.

## Error Handling

Authentication, MCP transport, gateway, PAT, and business errors continue
through the existing dispatcher and retain their current classification and
output behavior.

## Verification

Add a `TestCrossPlatformCoverage*` regression test that executes the real Cobra
leaf in explicit JSON mode with a fake MCP result containing a signed URL. It
must verify:

- the request still targets `oa/get_attachment_download_url`;
- the exact request arguments remain unchanged, including omission of the
  optional boolean when the flag was not supplied;
- stdout contains literal `&OSSAccessKeyId=` and `&Signature=`;
- stdout contains no `\u0026` escape.

The fake caller must report JSON format (or the command must be executed with
`--format json`) so the test fails against the current escaped JSON path rather
than accidentally exercising raw MCP text output.

Run the focused OA attachment tests, format modified Go files, and rebuild the
CLI. No commit is created.
