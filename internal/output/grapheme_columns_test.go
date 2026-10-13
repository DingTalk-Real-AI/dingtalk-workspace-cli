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
	"encoding/csv"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestCrossPlatformCoverageTablePreservesFlagAndSkinToneClusters(t *testing.T) {
	for _, tc := range []struct{ name, cluster string }{{"flag", "🇨🇳"}, {"skin-tone", "👍🏽"}} {
		t.Run(tc.name, func(t *testing.T) {
			original := strings.Repeat(tc.cluster, 40)
			payload := []map[string]any{{"name": original, "id": "one"}, {"name": "ok", "id": "two"}}
			var table bytes.Buffer
			if err := Write(&table, FormatTable, payload); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(table.String(), strings.Repeat(tc.cluster, 29)+"…") {
				t.Fatalf("table split or shortened visible clusters:\n%s", table.String())
			}
			width := -1
			for _, line := range strings.Split(strings.TrimSuffix(table.String(), "\n"), "\n") {
				if width < 0 {
					width = ansi.StringWidth(line)
				} else if got := ansi.StringWidth(line); got != width {
					t.Fatalf("table border/data columns differ: got%d want%d: %q", got, width, line)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageTruncateUsesCompleteGraphemes(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		width      int
		want       string
	}{
		{"flag-fits", "🇨🇳", 2, "🇨🇳"}, {"skin-tone-fits", "👍🏽", 2, "👍🏽"},
		{"flag-prefix", "🇨🇳abcd", 3, "🇨🇳…"}, {"skin-tone-prefix", "👍🏽abcd", 3, "👍🏽…"},
		{"combining", "e\u0301abcd", 2, "e\u0301…"}, {"ascii", "abcd", 3, "ab…"},
		{"cjk", "钉钉", 3, "钉…"}, {"one", "abcd", 1, "…"}, {"unbounded", "🇨🇳", 0, "🇨🇳"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncate(tc.text, tc.width); got != tc.want {
				t.Fatalf("truncate(%q,%d)=%q want%q", tc.text, tc.width, got, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageGraphemeRawFormatsPreserveContent(t *testing.T) {
	original := strings.Repeat("🇨🇳", 40) + " 👍🏽 e\u0301"
	payload := []map[string]any{{"id": "one", "name": original}}
	for _, format := range []Format{FormatJSON, FormatCSV} {
		t.Run(string(format), func(t *testing.T) {
			var raw bytes.Buffer
			if err := Write(&raw, format, payload); err != nil {
				t.Fatal(err)
			}
			switch format {
			case FormatJSON:
				var decoded []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}
				if err := json.Unmarshal(raw.Bytes(), &decoded); err != nil {
					t.Fatal(err)
				}
				if len(decoded) != 1 || decoded[0].ID != "one" || decoded[0].Name != original {
					t.Fatalf("JSON changed content: %s", raw.String())
				}
			case FormatCSV:
				records, err := csv.NewReader(strings.NewReader(raw.String())).ReadAll()
				if err != nil {
					t.Fatal(err)
				}
				if len(records) != 2 || len(records[0]) != 2 || len(records[1]) != 2 || records[0][0] != "id" || records[0][1] != "name" || records[1][0] != "one" || records[1][1] != original {
					t.Fatalf("CSV changed content: %q", records)
				}
			}
		})
	}
}

func BenchmarkTableGraphemeColumns(b *testing.B) {
	for _, size := range []struct {
		name string
		rows int
	}{{"small", 2}, {"page", 100}} {
		for _, text := range []struct{ name, value string }{{"ascii", "Quarterly report"}, {"clusters", "🇨🇳 👍🏽 café"}} {
			b.Run(size.name+"/"+text.name, func(b *testing.B) {
				payload := make([]map[string]any, size.rows)
				for i := range payload {
					payload[i] = map[string]any{"id": "row", "name": text.value}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := Write(io.Discard, FormatTable, payload); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
