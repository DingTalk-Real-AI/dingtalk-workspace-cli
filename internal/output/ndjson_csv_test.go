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
	"bytes"
	"strings"
	"testing"
)

func TestNormalizeFormatRecognizesNDJSONAndCSV(t *testing.T) {
	if got := normalizeFormat("ndjson", FormatJSON); got != FormatNDJSON {
		t.Errorf("normalizeFormat(ndjson) = %q, want %q", got, FormatNDJSON)
	}
	if got := normalizeFormat("CSV", FormatJSON); got != FormatCSV {
		t.Errorf("normalizeFormat(CSV) = %q, want %q", got, FormatCSV)
	}
}

func TestWriteNDJSON(t *testing.T) {
	cases := []struct {
		name      string
		payload   any
		wantLines []string
	}{
		{
			name:      "top-level array",
			payload:   []any{map[string]any{"id": "1"}, map[string]any{"id": "2"}},
			wantLines: []string{`{"id":"1"}`, `{"id":"2"}`},
		},
		{
			name:      "wrapped list",
			payload:   map[string]any{"items": []any{map[string]any{"id": "1"}, map[string]any{"id": "2"}}, "count": 2},
			wantLines: []string{`{"id":"1"}`, `{"id":"2"}`},
		},
		{
			name:      "scalar-ish object",
			payload:   map[string]any{"ok": true},
			wantLines: []string{`{"ok":true}`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Write(&buf, FormatNDJSON, tc.payload); err != nil {
				t.Fatalf("Write(ndjson) error = %v", err)
			}
			got := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
			if len(got) != len(tc.wantLines) {
				t.Fatalf("got %d lines %q, want %d %q", len(got), got, len(tc.wantLines), tc.wantLines)
			}
			for i, want := range tc.wantLines {
				if strings.TrimSpace(got[i]) != want {
					t.Errorf("line %d = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}

// TODO(#252): replace this with real coverage once writeCSV is implemented —
// happy-path list (incl. a comma-bearing and a CJK field), empty list, and the
// non-list fallback. For now it just pins the not-implemented contract.
func TestWriteCSVNotImplementedYet(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, FormatCSV, []any{map[string]any{"id": "1"}})
	if err == nil {
		t.Fatal("expected -f csv to return a not-implemented error; got nil — did you implement writeCSV? then update this test (#252)")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("error = %q, want it to mention 'not implemented'", err)
	}
}
