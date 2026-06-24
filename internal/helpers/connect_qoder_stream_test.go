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
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeQoderStreamStub(t *testing.T, dir string) string {
	t.Helper()
	logPath := filepath.Join(dir, "qoder-starts.log")
	script := `#!/usr/bin/env python3
import json
import os
import sys

with open(os.environ["DWS_QODER_STUB_LOG"], "a", encoding="utf-8") as f:
    f.write("START " + json.dumps(sys.argv[1:], ensure_ascii=False) + "\n")

if "--input-format" in sys.argv:
    for raw in sys.stdin:
        raw = raw.strip()
        if not raw:
            continue
        msg = json.loads(raw)
        if msg.get("type") == "control_request":
            rid = msg.get("request_id", "")
            print(json.dumps({
                "type": "control_response",
                "response": {
                    "subtype": "success",
                    "request_id": rid,
                    "response": {"agents": []},
                },
            }), flush=True)
            continue
        if msg.get("type") == "user":
            content = msg.get("message", {}).get("content", "")
            print(json.dumps({
                "type": "assistant",
                "message": {"content": [{"type": "text", "text": "seen " + content}]},
            }), flush=True)
            print(json.dumps({
                "type": "result",
                "subtype": "success",
                "message": {"content": [{"type": "text", "text": "done " + content}]},
            }), flush=True)
else:
    prompt = sys.argv[-1] if len(sys.argv) > 1 else ""
    print(json.dumps({
        "type": "result",
        "subtype": "success",
        "message": {"content": [{"type": "text", "text": "one-shot " + prompt}]},
    }))
`
	path := filepath.Join(dir, "qodercli")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write qoder stub: %v", err)
	}
	return logPath
}

func TestQoderForwarderKeepsStreamJSONProcessAlive(t *testing.T) {
	t.Setenv("DWS_CONNECT_NO_INSTALL", "1")
	t.Setenv("DWS_AGENT_CMD", "")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DWS_AGENT_TIMEOUT_MS", "3000")
	stubDir := t.TempDir()
	logPath := writeQoderStreamStub(t, stubDir)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DWS_QODER_STUB_LOG", logPath)

	fwd, err := forwarderForChannel("qoder", "qoder-client", connectAgentOptions{Memory: true})
	if err != nil {
		t.Fatalf("forwarderForChannel(qoder): %v", err)
	}
	if closer, ok := fwd.(forwarderCloser); ok {
		defer closer.close()
	}
	sf, ok := fwd.(streamingForwarder)
	if !ok || !sf.canStream() {
		t.Fatalf("qoder forwarder should support streaming, got %T", fwd)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got1, err := sf.forwardStream(ctx, "conv-a", "first", func(string) {})
	if err != nil {
		t.Fatalf("first forward: %v", err)
	}
	got2, err := sf.forwardStream(ctx, "conv-a", "second", func(string) {})
	if err != nil {
		t.Fatalf("second forward: %v", err)
	}
	if got1 != "done first" || got2 != "done second" {
		t.Fatalf("replies = %q / %q, want done first / done second", got1, got2)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read qoder stub log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("qodercli starts = %d, want 1; log:\n%s", len(lines), raw)
	}
	if !strings.Contains(lines[0], "--input-format") || !strings.Contains(lines[0], "stream-json") {
		t.Fatalf("qodercli should start in stream-json input mode, log:\n%s", raw)
	}
}

func TestParseQoderPersistentLineReadsResultField(t *testing.T) {
	delta, final, done := parseQoderPersistentLine(`{"type":"result","subtype":"success","result":"OK"}`)
	if delta != "" || final != "OK" || !done {
		t.Fatalf("delta=%q final=%q done=%v, want final OK done", delta, final, done)
	}
}
