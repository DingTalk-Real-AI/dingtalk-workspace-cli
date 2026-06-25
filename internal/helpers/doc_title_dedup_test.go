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

package helpers

import (
	"strings"
	"testing"
)

func TestStripLeadingDuplicateTitleHeading(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		docName  string
		want     string
		stripped bool
	}{
		{
			name:     "exact match stripped",
			content:  "# 2026-06-10 Ari晚会简报\n\n聚焦今日变化。\n",
			docName:  "2026-06-10 Ari晚会简报",
			want:     "聚焦今日变化。\n",
			stripped: true,
		},
		{
			name:     "leading blank lines tolerated",
			content:  "\n\n# Title\nbody",
			docName:  "Title",
			want:     "body",
			stripped: true,
		},
		{
			name:     "case insensitive match",
			content:  "# weekly REPORT\nbody",
			docName:  "Weekly Report",
			want:     "body",
			stripped: true,
		},
		{
			name:     "atx closing hashes",
			content:  "# Title #\nbody",
			docName:  "Title",
			want:     "body",
			stripped: true,
		},
		{
			name:     "title-only content becomes empty",
			content:  "# Title",
			docName:  "Title",
			want:     "",
			stripped: true,
		},
		{
			name:     "different heading kept",
			content:  "# 背景\nbody",
			docName:  "2026-06-10 Ari晚会简报",
			want:     "# 背景\nbody",
			stripped: false,
		},
		{
			name:     "h2 not touched",
			content:  "## Title\nbody",
			docName:  "Title",
			want:     "## Title\nbody",
			stripped: false,
		},
		{
			name:     "no heading kept",
			content:  "plain text\n# Title later",
			docName:  "Title",
			want:     "plain text\n# Title later",
			stripped: false,
		},
		{
			name:     "name ending with hash not over-trimmed",
			content:  "# C#\nbody",
			docName:  "C",
			want:     "# C#\nbody",
			stripped: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, stripped := stripLeadingDuplicateTitleHeading(tc.content, tc.docName)
			if stripped != tc.stripped {
				t.Fatalf("stripped = %v, want %v", stripped, tc.stripped)
			}
			if got != tc.want {
				t.Fatalf("content = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDocCreateStripsDuplicateTitleHeading verifies the end-to-end behavior:
// `doc create --name X --content "# X\n..."` must not forward the duplicate
// H1 to the MCP tool — the platform renders the document name as the page
// title, so keeping it would display two headings.
func TestDocCreateStripsDuplicateTitleHeading(t *testing.T) {
	runner := &docCommandRunner{}
	root := newDocTestRoot(runner)

	_, errOut, err := executeDocCommand(t, root,
		"create", "--name", "2026-06-10 Ari晚会简报",
		"--content", "# 2026-06-10 Ari晚会简报\n\n聚焦今日变化。")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	got, _ := runner.last.Params["markdown"].(string)
	if got != "聚焦今日变化。" {
		t.Errorf("markdown param = %q, want duplicate H1 stripped", got)
	}
	if !strings.Contains(errOut, "已自动移除") {
		t.Errorf("stderr = %q, want a note about the removed heading", errOut)
	}
}

// TestDocCreateKeepsDistinctHeading ensures the guard never eats an H1 that
// differs from the document name.
func TestDocCreateKeepsDistinctHeading(t *testing.T) {
	runner := &docCommandRunner{}
	root := newDocTestRoot(runner)

	_, _, err := executeDocCommand(t, root,
		"create", "--name", "晚会简报", "--content", "# 背景\n正文")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	got, _ := runner.last.Params["markdown"].(string)
	if got != "# 背景\n正文" {
		t.Errorf("markdown param = %q, want content untouched", got)
	}
}

// TestDocCreateTitleOnlyContentOmitsMarkdown: when the body is nothing but
// the duplicate H1, the markdown param should be omitted entirely instead of
// sending an empty string.
func TestDocCreateTitleOnlyContentOmitsMarkdown(t *testing.T) {
	runner := &docCommandRunner{}
	root := newDocTestRoot(runner)

	_, _, err := executeDocCommand(t, root,
		"create", "--name", "晚会简报", "--content", "# 晚会简报")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, ok := runner.last.Params["markdown"]; ok {
		t.Errorf("markdown param = %v, want omitted", runner.last.Params["markdown"])
	}
}

// TestDocCreateStripsDuplicateTitleJSONML verifies the end-to-end JSONML path:
// `doc create --content-format jsonml` must drop a leading h1 whose text equals
// --name before forwarding the body to update_document, otherwise the rich
// document shows the page title twice.
func TestDocCreateStripsDuplicateTitleJSONML(t *testing.T) {
	runner := &docCommandRunner{responses: []map[string]any{{"nodeId": "NODE_X"}}}
	root := newDocTestRoot(runner)

	body := `{"jsonml":[["h1",{},"命令树参考"],["p",{},"正文"]]}`
	_, errOut, err := executeDocCommand(t, root,
		"create", "--name", "命令树参考",
		"--content-format", "jsonml", "--content", body)
	if err != nil {
		t.Fatalf("execute: %v\nstderr:\n%s", err, errOut)
	}
	if len(runner.all) != 2 {
		t.Fatalf("calls = %d, want 2 (create + update)", len(runner.all))
	}
	if runner.all[1].Tool != "update_document" {
		t.Fatalf("second tool = %q, want update_document", runner.all[1].Tool)
	}
	got, _ := runner.all[1].Params["jsonml"].(string)
	if strings.Contains(got, "命令树参考") {
		t.Errorf("jsonml = %q, want duplicate h1 stripped", got)
	}
	if !strings.Contains(got, "正文") {
		t.Errorf("jsonml = %q, want body content kept", got)
	}
	if !strings.Contains(errOut, "已自动移除") {
		t.Errorf("stderr = %q, want a note about the removed heading", errOut)
	}
}

// TestStripLeadingDuplicateTitleJSONML covers the JSONML-path counterpart: a
// leading h1 node whose text equals the document name is removed; everything
// else is left untouched.
func TestStripLeadingDuplicateTitleJSONML(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		docName  string
		want     string
		stripped bool
	}{
		{
			name:     "root-wrapped duplicate h1 stripped",
			body:     `["root",{},["h1",{},"晚会简报"],["p",{},"正文"]]`,
			docName:  "晚会简报",
			want:     `["root",{},["p",{},"正文"]]`,
			stripped: true,
		},
		{
			name:     "nested leaf text matches and stripped",
			body:     `["root",{},["h1",{"uuid":"x"},["span",{"data-type":"text"},["span",{"data-type":"leaf"},"晚会简报"]]],["p",{},"正文"]]`,
			docName:  "晚会简报",
			want:     `["root",{},["p",{},"正文"]]`,
			stripped: true,
		},
		{
			name:     "bare body without root wrapper",
			body:     `[["h1",{},"标题"],["p",{},"正文"]]`,
			docName:  "标题",
			want:     `[["p",{},"正文"]]`,
			stripped: true,
		},
		{
			name:     "case insensitive match",
			body:     `["root",{},["h1",{},"Weekly REPORT"],["p",{},"x"]]`,
			docName:  "weekly report",
			want:     `["root",{},["p",{},"x"]]`,
			stripped: true,
		},
		{
			name:     "distinct heading kept",
			body:     `["root",{},["h1",{},"背景"],["p",{},"正文"]]`,
			docName:  "晚会简报",
			want:     `["root",{},["h1",{},"背景"],["p",{},"正文"]]`,
			stripped: false,
		},
		{
			name:     "non-h1 leading node kept",
			body:     `["root",{},["h2",{},"晚会简报"],["p",{},"正文"]]`,
			docName:  "晚会简报",
			want:     `["root",{},["h2",{},"晚会简报"],["p",{},"正文"]]`,
			stripped: false,
		},
		{
			name:     "invalid json untouched",
			body:     `not json`,
			docName:  "x",
			want:     `not json`,
			stripped: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := stripLeadingDuplicateTitleJSONML(tc.body, tc.docName)
			if ok != tc.stripped {
				t.Fatalf("stripped = %v, want %v", ok, tc.stripped)
			}
			if got != tc.want {
				t.Fatalf("body = %q, want %q", got, tc.want)
			}
		})
	}
}
