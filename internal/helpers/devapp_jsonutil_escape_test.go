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
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/jsonutil"
)

// TestDevAppEnvelopeJSONMatchesJSONUtilEscaping 是队列 B121 的信封 JSON 与
// jsonutil 转义一致性断言：信封 JSON 经 jsonutil.MarshalIndent（HTML 转义
// 关闭）序列化，& < > 与 URL 查询分隔符原样保留，不出现 \u003c/\u0026 等
// HTML 转义序列。业务载荷含 URL 时不应被转义破坏。
func TestDevAppEnvelopeJSONMatchesJSONUtilEscaping(t *testing.T) {
	content := map[string]any{
		"unifiedAppId": "u-1",
		"homepageUrl":  "https://example.com/redirect?dest=a&from=b&raw=<payload>",
		"name":         "Demo<App>&Co",
	}
	out, errBuf, err := runDevAppFamily(t, devAppFamilyContentRunner(content),
		"dev", "app", "get", "--unified-app-id", "u-1")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
	}
	s := out.String()

	// ① 载荷 URL/特殊字符原样保留（HTML 转义关闭）。
	for _, want := range []string{"https://example.com/redirect?dest=a&from=b&raw=<payload>", "Demo<App>&Co"} {
		if !strings.Contains(s, want) {
			t.Fatalf("envelope JSON must preserve raw %q (HTML escaping disabled):\n%s", want, s)
		}
	}
	// ② 不得出现 HTML 转义序列。
	for _, banned := range []string{"\\u003c", "\\u003e", "\\u0026", "\\u003d", "\\u002f"} {
		if strings.Contains(s, banned) {
			t.Fatalf("envelope JSON must not HTML-escape to %q:\n%s", banned, s)
		}
	}

	// ③ 与 jsonutil.MarshalIndent 对同一数据载荷的序列化逐字一致（转义规则同源）。
	env := map[string]any{
		"ok":      true,
		"outcome": "success",
		"data":    content,
	}
	wantJSON, err := jsonutil.MarshalIndent(env, "", "  ")
	if err != nil {
		t.Fatalf("jsonutil.MarshalIndent: %v", err)
	}
	// 信封 Data 是 content 的强类型 map，序列化键序稳定：比对 data 片段即可。
	var gotData struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &gotData); err != nil {
		t.Fatalf("stdout not a JSON envelope: %v\n%s", err, out.String())
	}
	wantDataStruct := struct {
		Data map[string]any `json:"data"`
	}{Data: content}
	wantDataJSON, err := jsonutil.MarshalIndent(wantDataStruct, "", "  ")
	if err != nil {
		t.Fatalf("jsonutil.MarshalIndent(data): %v", err)
	}
	gotDataJSON, err := jsonutil.MarshalIndent(gotData, "", "  ")
	if err != nil {
		t.Fatalf("jsonutil.MarshalIndent(got): %v", err)
	}
	if string(gotDataJSON) != string(wantDataJSON) {
		t.Fatalf("envelope data serialization diverges from jsonutil:\nGOT:\n%s\nWANT:\n%s",
			gotDataJSON, wantDataJSON)
	}
	_ = wantJSON
}
