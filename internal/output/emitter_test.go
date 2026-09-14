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
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newTestEnvelope() *Envelope {
	return &Envelope{
		OK:      true,
		Outcome: OutcomeSuccess,
		Data:    map[string]any{"name": "DemoApp"},
	}
}

func TestWriteEnvelopeTo_JSONFullEnvelope(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteEnvelopeTo(&buf, newTestEnvelope(), FormatJSON, "", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if decoded["outcome"] != "success" {
		t.Fatalf("outcome = %v, want success", decoded["outcome"])
	}
	// ok 必须序列化为 JSON 布尔（禁止字符串布尔）。
	if okVal, isBool := decoded["ok"].(bool); !isBool || !okVal {
		t.Fatalf("ok = %v (%T), want true (bool)", decoded["ok"], decoded["ok"])
	}
	if _, hasData := decoded["data"]; !hasData {
		t.Fatalf("data key missing: %s", buf.String())
	}
}

func TestWriteEnvelopeTo_OmitEmptyNoNull(t *testing.T) {
	var buf bytes.Buffer
	env := &Envelope{OK: true, Outcome: OutcomeSuccess}
	if err := WriteEnvelopeTo(&buf, env, FormatJSON, "", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo: %v", err)
	}
	s := buf.String()
	for _, key := range []string{`"meta"`, `"error"`, `"identity"`, `"dry_run"`, `"_notice"`} {
		if strings.Contains(s, key) {
			t.Fatalf("empty field %s must be omitted, got %s", key, s)
		}
	}
	if strings.Contains(s, "null") {
		t.Fatalf("envelope must not emit null: %s", s)
	}
}

func TestWriteEnvelopeTo_NonJSONRendersDataOnly(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteEnvelopeTo(&buf, newTestEnvelope(), FormatTable, "", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo: %v", err)
	}
	s := buf.String()
	if strings.Contains(s, `"outcome"`) || strings.Contains(s, `"ok"`) {
		t.Fatalf("table/pretty formats must not emit the envelope wrapper: %s", s)
	}
	if !strings.Contains(s, "DemoApp") {
		t.Fatalf("data payload missing from table output: %s", s)
	}
}

func TestWriteEnvelope_CommandUsesCmdStdoutAndFormat(t *testing.T) {
	cmd := &cobra.Command{Use: "t"}
	cmd.Flags().StringP("format", "f", "", "")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := cmd.Flags().Set("format", "table"); err != nil {
		t.Fatalf("set format: %v", err)
	}
	if err := WriteEnvelope(cmd, newTestEnvelope(), FormatJSON); err != nil {
		t.Fatalf("WriteEnvelope: %v", err)
	}
	if strings.Contains(stdout.String(), `"outcome"`) {
		t.Fatalf("-f table must render data only, got %s", stdout.String())
	}

	// 默认 json：完整信封落 stdout。
	stdout.Reset()
	cmd2 := &cobra.Command{Use: "t2"}
	cmd2.Flags().StringP("format", "f", "", "")
	cmd2.SetOut(&stdout)
	cmd2.SetErr(&stderr)
	if err := WriteEnvelope(cmd2, newTestEnvelope(), FormatJSON); err != nil {
		t.Fatalf("WriteEnvelope: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if decoded["outcome"] != "success" {
		t.Fatalf("json envelope outcome = %v, want success: %s", decoded["outcome"], stdout.String())
	}
}

func TestWriteEnvelope_NilEnvelopeDoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{Use: "t"}
	cmd.Flags().StringP("format", "f", "json", "")
	cmd.SetOut(&buf)
	if err := WriteEnvelope(cmd, nil, FormatJSON); err != nil {
		t.Fatalf("WriteEnvelope: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if decoded["outcome"] != "failure" {
		t.Fatalf("nil envelope should serialize as failure: %s", buf.String())
	}
	// B208（轮6裁决⑨）：兜底信封必须是 I3 合法形态——ok:false/outcome:failure
	// 保留，error 明细存在且兜底信封自身通过 Validate()。
	if okVal, isBool := decoded["ok"].(bool); !isBool || okVal {
		t.Fatalf("nil fallback ok = %v (%T), want false (bool): %s", decoded["ok"], decoded["ok"], buf.String())
	}
	errObj, hasErr := decoded["error"].(map[string]any)
	if !hasErr {
		t.Fatalf("nil fallback must carry error detail (I3): %s", buf.String())
	}
	if errObj["type"] != "internal" {
		t.Fatalf("fallback error.type = %v, want internal: %s", errObj["type"], buf.String())
	}
	if msg, _ := errObj["message"].(string); msg != "nil envelope: framework fallback" {
		t.Fatalf("fallback error.message = %v, want %q: %s", errObj["message"], "nil envelope: framework fallback", buf.String())
	}
	if err := nilFallbackEnvelope().Validate(); err != nil {
		t.Fatalf("nil fallback envelope must pass Validate(), got %v", err)
	}
}

func TestWriteEnvelope_PaginationWireKeys(t *testing.T) {
	var buf bytes.Buffer
	env := newTestEnvelope()
	env.Data = []any{}
	env.Meta = &Meta{
		Count: NewCount(0),
		Pagination: &Pagination{
			EndpointExhausted: false,
			NextToken:         "cursor_xyz",
			Pages:             1,
			Items:             0,
		},
	}
	if err := WriteEnvelopeTo(&buf, env, FormatJSON, "", ""); err != nil {
		t.Fatalf("WriteEnvelopeTo: %v", err)
	}
	var decoded struct {
		Meta struct {
			Count      *int `json:"count"`
			Pagination struct {
				EndpointExhausted bool   `json:"endpoint_exhausted"`
				NextToken         string `json:"next_token"`
				Pages             int    `json:"pages"`
				Items             int    `json:"items"`
			} `json:"pagination"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if decoded.Meta.Count == nil || *decoded.Meta.Count != 0 {
		t.Fatalf("meta.count = %v, want 0: %s", decoded.Meta.Count, buf.String())
	}
	// 契约 §3：endpoint_exhausted 两态显式——false 必须可解码区分缺席。
	if decoded.Meta.Pagination.EndpointExhausted {
		t.Fatalf("endpoint_exhausted = true, want false: %s", buf.String())
	}
	if decoded.Meta.Pagination.NextToken != "cursor_xyz" {
		t.Fatalf("next_token = %q, want cursor_xyz: %s", decoded.Meta.Pagination.NextToken, buf.String())
	}
	// wire 层 snake_case 恒等：camelCase 键禁止出现。
	s := buf.String()
	for _, key := range []string{`endpointExhausted`, `nextToken`, `nextCommand`, `timedOut`} {
		if strings.Contains(s, key) {
			t.Fatalf("camelCase wire key %s forbidden: %s", key, s)
		}
	}
	// endpoint_exhausted:false 必须显式出现在 wire（无 omitempty）。
	if !strings.Contains(s, `"endpoint_exhausted": false`) && !strings.Contains(s, `"endpoint_exhausted":false`) {
		t.Fatalf("endpoint_exhausted must be explicit when false: %s", s)
	}
}
