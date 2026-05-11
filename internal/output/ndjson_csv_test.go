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

func TestWriteCSV(t *testing.T) {
	cases := []struct {
		name    string
		payload any
		want    string
	}{
		{
			// Union of keys (sorted), missing values → empty cells, a field with
			// a comma gets quoted, CJK passes through verbatim, a nested array is
			// rendered as compact JSON with its quotes CSV-escaped.
			name: "list of objects",
			payload: []any{
				map[string]any{"id": "1", "name": "张三"},
				map[string]any{"id": "2", "name": "Bob, Jr."},
				map[string]any{"id": "3", "tags": []any{"x", "y"}},
			},
			want: "id,name,tags\n" +
				"1,张三,\n" +
				"2,\"Bob, Jr.\",\n" +
				"3,,\"[\"\"x\"\",\"\"y\"\"]\"\n",
		},
		{
			// {records:[...], total:N}: the list becomes the table; sibling
			// metadata (total) is broadcast as a trailing column on every row.
			name: "wrapped list with metadata",
			payload: map[string]any{
				"records": []any{map[string]any{"id": "1"}, map[string]any{"id": "2"}},
				"total":   2,
			},
			want: "id,total\n1,2\n2,2\n",
		},
		{
			// Empty list + metadata: still emit the header (data + meta) plus a
			// single row of empty data cells carrying the meta values.
			name: "empty wrapped list with metadata",
			payload: map[string]any{
				"records": []any{},
				"total":   0,
				"hasMore": false,
			},
			want: "value,hasMore,total\n,false,0\n",
		},
		{
			// A plain object → two-column key,value CSV with keys sorted.
			name:    "single object",
			payload: map[string]any{"ok": true, "name": "x"},
			want:    "key,value\nname,x\nok,true\n",
		},
		{
			name:    "scalar",
			payload: "hello",
			want:    "hello\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Write(&buf, FormatCSV, tc.payload); err != nil {
				t.Fatalf("Write(csv) error = %v", err)
			}
			if got := buf.String(); got != tc.want {
				t.Errorf("Write(csv) =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

// TestWriteCSVComposesWithFields guards that --fields projection (applied by
// WriteFiltered before Write) narrows the CSV columns.
func TestWriteCSVComposesWithFields(t *testing.T) {
	payload := map[string]any{
		"items": []any{
			map[string]any{"id": "1", "name": "Alice", "secret": "s1"},
			map[string]any{"id": "2", "name": "Bob", "secret": "s2"},
		},
	}
	var buf bytes.Buffer
	if err := WriteFiltered(&buf, FormatCSV, payload, "id,name", ""); err != nil {
		t.Fatalf("WriteFiltered(csv) error = %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "secret") || strings.Contains(got, "s1") {
		t.Errorf("--fields did not drop the secret column; got:\n%s", got)
	}
	if !strings.Contains(got, "id,name") || !strings.Contains(got, "Alice") {
		t.Errorf("expected projected columns id,name with values; got:\n%s", got)
	}
}

// TestTabularDetectsRealDingTalkEnvelopes guards against shipping a -f csv /
// -f ndjson that degrades to one-line-key-value for the envelope shapes the
// real product surface actually returns. Each case is a payload shape observed
// in production (contact / doc / mail / todo / chat search responses).
func TestTabularDetectsRealDingTalkEnvelopes(t *testing.T) {
	cases := []struct {
		name        string
		payload     map[string]any
		wantNDLines int    // expected line count from -f ndjson
		wantCSVHead string // first header line of -f csv
	}{
		{
			name: "result direct array (contact user search)",
			payload: map[string]any{
				"result":  []any{map[string]any{"name": "张三", "userId": "123"}, map[string]any{"name": "李四", "userId": "456"}},
				"success": true,
			},
			wantNDLines: 2,
			wantCSVHead: "name,userId,success",
		},
		{
			name: "documents top-level (doc search)",
			payload: map[string]any{
				"documents":     []any{map[string]any{"nodeId": "n1", "name": "A"}, map[string]any{"nodeId": "n2", "name": "B"}},
				"hasMore":       true,
				"nextPageToken": "tok",
			},
			wantNDLines: 2,
			wantCSVHead: "name,nodeId,hasMore,nextPageToken",
		},
		{
			name: "emailAccounts top-level (mail mailbox list)",
			payload: map[string]any{
				"emailAccounts": []any{map[string]any{"email": "a@b.com", "type": "ORG"}},
				"success":       "true",
			},
			wantNDLines: 1,
			wantCSVHead: "email,type,success",
		},
		{
			name: "todoCards under result wrapper (todo task list)",
			payload: map[string]any{
				"result": map[string]any{
					"todoCards": []any{
						map[string]any{"taskId": "t1", "subject": "做一做"},
						map[string]any{"taskId": "t2", "subject": "再做一做"},
					},
				},
			},
			wantNDLines: 2,
			wantCSVHead: "subject,taskId",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var nd bytes.Buffer
			if err := Write(&nd, FormatNDJSON, tc.payload); err != nil {
				t.Fatalf("ndjson write: %v", err)
			}
			ndLines := strings.Split(strings.TrimRight(nd.String(), "\n"), "\n")
			if len(ndLines) != tc.wantNDLines {
				t.Errorf("ndjson: got %d lines %q, want %d", len(ndLines), ndLines, tc.wantNDLines)
			}

			var c bytes.Buffer
			if err := Write(&c, FormatCSV, tc.payload); err != nil {
				t.Fatalf("csv write: %v", err)
			}
			gotHead := strings.SplitN(c.String(), "\n", 2)[0]
			if gotHead != tc.wantCSVHead {
				t.Errorf("csv header: got %q, want %q", gotHead, tc.wantCSVHead)
			}
		})
	}
}
