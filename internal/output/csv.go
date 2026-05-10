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
	"encoding/csv"
	"io"
)

// writeCSV renders a payload as RFC-4180 CSV.
//
// It mirrors the shape decisions `-f table` already makes (same helpers:
// normalizePayload / unwrapPrimaryObject / extractRowsFromMap / rowsFromSlice /
// formatValue), so column order and value flattening stay consistent between
// the two formats:
//
//   - a list of objects — either a bare [{...},...] or wrapped under a
//     well-known key ({items|results|data|records|...}) — becomes a header row
//     plus one row per element. The union of keys (sorted) is the column set;
//     missing values are empty cells; nested objects/arrays render as compact
//     JSON in the cell. Any sibling metadata of the list (total, hasMore, ...)
//     is dropped — CSV is the tabular slice, nothing else.
//   - a single object becomes a two-column `key,value` CSV.
//   - a non-uniform list or a scalar becomes a single-column `value` CSV.
//
// `--fields` projection composes for free: WriteFiltered applies SelectFields
// before Write reaches us, so the rows are already narrowed.
//
// encoding/csv.Writer handles quoting/escaping of commas, double quotes and
// embedded newlines; cell text goes through formatValue (which also strips
// terminal control sequences, same as the table renderer).
func writeCSV(w io.Writer, payload any) error {
	normalized, err := normalizePayload(payload)
	if err != nil {
		return err
	}

	cw := csv.NewWriter(w)

	switch typed := normalized.(type) {
	case map[string]any:
		if inner, ok := unwrapPrimaryObject(typed); ok {
			return writeKeyValueCSV(cw, inner)
		}
		if headers, rows, _, ok := extractRowsFromMap(typed); ok {
			return writeTableCSV(cw, headers, rows)
		}
		return writeKeyValueCSV(cw, typed)
	case []any:
		headers, rows, _ := rowsFromSlice(typed)
		return writeTableCSV(cw, headers, rows)
	case nil:
		// Nothing to write — emit an empty document rather than erroring.
		cw.Flush()
		return cw.Error()
	default:
		// Scalar: a single-cell, single-row CSV.
		if err := cw.Write([]string{formatValue(normalized)}); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	}
}

func writeTableCSV(cw *csv.Writer, headers []string, rows [][]string) error {
	if err := cw.Write(headers); err != nil {
		return err
	}
	for _, row := range rows {
		// rowsFromSlice / extractRowsFromMap already guarantee
		// len(row) == len(headers), but stay defensive against future callers.
		if len(row) != len(headers) {
			padded := make([]string, len(headers))
			copy(padded, row)
			row = padded
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func writeKeyValueCSV(cw *csv.Writer, m map[string]any) error {
	if err := cw.Write([]string{"key", "value"}); err != nil {
		return err
	}
	for _, key := range sortedMapKeys(m) {
		if err := cw.Write([]string{key, formatValue(m[key])}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
