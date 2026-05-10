// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package output

import (
	"fmt"
	"io"
)

// writeCSV renders list-shaped results as RFC-4180 CSV.
//
// TODO(#252): implement. Planned approach (mirrors how `-f table` already
// flattens results, so reuse those helpers):
//
//  1. Locate the row list with the existing helpers — extractRowsFromMap /
//     rowsFromSlice in formatter.go already turn `{items:[{...},...]}` (or a
//     bare `[{...},...]`) into (headers []string, rows [][]string). Reuse them
//     verbatim so table and csv agree on column order and flattening rules.
//  2. Write headers as the first record, then each row, via encoding/csv.Writer
//     (it handles quoting/escaping of commas, quotes and newlines for us).
//  3. Nested objects / arrays in a cell: render as compact JSON (same as the
//     table renderer does today) so the column count stays stable.
//  4. Non-list payloads (a single object, a scalar): emit a two-column
//     key,value CSV — or return a clear error telling the user `-f csv` only
//     applies to list results. Pick one and cover it in the test.
//  5. Tests in csv_test.go: a happy-path list (incl. a field containing a
//     comma and a field containing CJK text), an empty list (headers only?),
//     and the non-list fallback. Wire `--fields` projection through as well.
//
// Until then, fail loudly rather than silently degrading to JSON so callers
// know the format isn't ready yet.
func writeCSV(_ io.Writer, _ any) error {
	return fmt.Errorf("output format %q is not implemented yet — see https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/issues/252", FormatCSV)
}
