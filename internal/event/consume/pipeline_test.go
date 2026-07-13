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

package consume

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func TestPipeline_FormatNDJSONToStdout(t *testing.T) {
	var buf bytes.Buffer
	p, err := BuildPipeline(FormatNDJSON, "", nil, &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	for i := 0; i < 3; i++ {
		_ = p.Deliver(transport.Event{Type: transport.FrameTypeEvent, EventID: "x", EventType: "y", Data: "{}"})
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 NDJSON lines, got %d", len(lines))
	}
	for _, line := range lines {
		var ev transport.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Errorf("not valid JSON: %v\n%s", err, line)
		}
	}
}

func TestPipeline_OutputDirFallback(t *testing.T) {
	dir := t.TempDir()
	p, err := BuildPipeline(FormatNDJSON, dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	_ = p.Deliver(transport.Event{EventType: "im.message.receive_v1", EventID: "ev1", ReceivedAtUnixMS: 100})
	_ = p.Deliver(transport.Event{EventType: "im.message.receive_v1", EventID: "ev2", ReceivedAtUnixMS: 200})

	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 files in %s, got %d", dir, len(entries))
	}
}

func TestPipeline_RouteWithStdoutFallback(t *testing.T) {
	root := t.TempDir()
	imDir := filepath.Join(root, "im")
	var stdoutBuf bytes.Buffer
	routes, err := ParseRoutes([]string{`^im\.=dir:` + imDir})
	if err != nil {
		t.Fatal(err)
	}
	p, err := BuildPipeline(FormatNDJSON, "", routes, &stdoutBuf)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	_ = p.Deliver(transport.Event{EventType: "im.message.receive_v1", EventID: "ev1", ReceivedAtUnixMS: 100})
	_ = p.Deliver(transport.Event{EventType: "approval.task", EventID: "ev2", ReceivedAtUnixMS: 200})

	// im event routed to imDir
	if entries, _ := os.ReadDir(imDir); len(entries) != 1 {
		t.Errorf("expected 1 file in im dir, got %d", len(entries))
	}
	// approval event fell through to stdout
	if !strings.Contains(stdoutBuf.String(), `"event_id":"ev2"`) {
		t.Errorf("approval event missing from stdout:\n%s", stdoutBuf.String())
	}
}

func TestPipeline_RouteWithOutputDirFallback(t *testing.T) {
	root := t.TempDir()
	imDir := filepath.Join(root, "im")
	defaultDir := filepath.Join(root, "default")
	routes, _ := ParseRoutes([]string{`^im\.=dir:` + imDir})
	p, _ := BuildPipeline(FormatNDJSON, defaultDir, routes, nil)
	defer p.Close()

	_ = p.Deliver(transport.Event{EventType: "im.message.receive_v1", EventID: "ev1", ReceivedAtUnixMS: 100})
	_ = p.Deliver(transport.Event{EventType: "approval.task", EventID: "ev2", ReceivedAtUnixMS: 200})

	if entries, _ := os.ReadDir(imDir); len(entries) != 1 {
		t.Errorf("im events should go to imDir, got %d files", len(entries))
	}
	if entries, _ := os.ReadDir(defaultDir); len(entries) != 1 {
		t.Errorf("unmatched events should go to default dir, got %d files", len(entries))
	}
}

func TestPipeline_CompactFormat(t *testing.T) {
	var buf bytes.Buffer
	p, _ := BuildPipeline(FormatCompact, "", nil, &buf)
	defer p.Close()
	_ = p.Deliver(transport.Event{EventType: "im.message.receive_v1", EventID: "ev1", Data: `{"message":{"chat_id":"oc_x","message_id":"om_y","content":"hi"}}`})
	line := strings.TrimRight(buf.String(), "\n")
	var out map[string]any
	if err := json.Unmarshal([]byte(line), &out); err != nil {
		t.Fatalf("compact output not valid JSON: %v", err)
	}
	if out["chat_id"] != "oc_x" {
		t.Errorf("compact missed chat_id: %+v", out)
	}
}
