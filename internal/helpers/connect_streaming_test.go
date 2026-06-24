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

import "testing"

func TestParseStreamLineCC(t *testing.T) {
	d, f := parseStreamLine("cc", `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"你好"}}}`)
	if d != "你好" || f != "" {
		t.Fatalf("text_delta: d=%q f=%q", d, f)
	}
	// thinking deltas are internal reasoning — must NOT surface
	d, f = parseStreamLine("cc", `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"hmm"}}}`)
	if d != "" || f != "" {
		t.Fatalf("thinking_delta must be ignored: d=%q f=%q", d, f)
	}
	d, f = parseStreamLine("cc", `{"type":"result","result":"最终答案"}`)
	if d != "" || f != "最终答案" {
		t.Fatalf("result: d=%q f=%q", d, f)
	}
}

func TestParseStreamLineQoder(t *testing.T) {
	d, f := parseStreamLine("qoder", `{"type":"assistant","subtype":"message","message":{"content":[{"type":"text","text":"第一段"}]}}`)
	if d != "第一段\n\n" || f != "" {
		t.Fatalf("assistant: d=%q f=%q", d, f)
	}
	d, f = parseStreamLine("qoder", `{"type":"result","subtype":"success","message":{"content":[{"type":"text","text":"1、2、3"}]}}`)
	if d != "" || f != "1、2、3" {
		t.Fatalf("result: d=%q f=%q", d, f)
	}
	d, f = parseStreamLine("qoder", `{"type":"result","subtype":"success","result":"OK"}`)
	if d != "" || f != "OK" {
		t.Fatalf("result field: d=%q f=%q", d, f)
	}
	// garbage tolerated
	if d, f := parseStreamLine("qoder", `{not json`); d != "" || f != "" {
		t.Fatalf("garbage: d=%q f=%q", d, f)
	}
}

func TestStreamSpecsCoverage(t *testing.T) {
	for ch, want := range map[string]string{
		"claudecode": "cc", "codebuddy": "cc", "workbuddy": "cc",
		"qoder": "qoder", "qoderwork": "qoder",
		"codex": "", "gemini": "", "opencode": "",
	} {
		spec := agentSpecs[ch]
		if spec.streamParser != want {
			t.Errorf("%s streamParser = %q, want %q", ch, spec.streamParser, want)
		}
		if (want != "") != (len(spec.streamArgvTail) > 0) {
			t.Errorf("%s streamArgvTail presence mismatch", ch)
		}
	}
}
