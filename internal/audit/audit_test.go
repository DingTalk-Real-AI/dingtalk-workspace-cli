package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileSinkEmit(t *testing.T) {
	dir := t.TempDir()
	writer, err := NewDateRotatingWriter(dir, 90)
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(dir)
	sink := NewFileSink(writer, chain, nil)
	defer sink.Close()

	evt := &Event{
		Timestamp:   time.Now(),
		ExecutionID: "abc123",
		Actor:       Actor{UserID: "u1", CorpID: "c1"},
		Product:     "calendar",
		Command:     "list_events",
		Endpoint:    "https://api.example.com/mcp",
		Result:      "success",
		DurationMs:  150,
		CLIVersion:  "1.0.47",
		OS:          "darwin",
		Arch:        "arm64",
	}

	if err := sink.Emit(evt); err != nil {
		t.Fatal(err)
	}

	if evt.Hash == "" {
		t.Error("expected hash to be set")
	}
	if evt.PrevHash != "" {
		t.Error("first event should have empty prev_hash")
	}

	file, err := LatestAuditFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ExecutionID != "abc123" {
		t.Errorf("got execution_id=%s, want abc123", decoded.ExecutionID)
	}
	if decoded.Hash == "" {
		t.Error("decoded hash should not be empty")
	}
}

func TestChainIntegrity(t *testing.T) {
	dir := t.TempDir()
	writer, err := NewDateRotatingWriter(dir, 90)
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(dir)
	sink := NewFileSink(writer, chain, nil)

	for i := 0; i < 5; i++ {
		evt := &Event{
			Timestamp:   time.Now(),
			ExecutionID: "exec-" + string(rune('a'+i)),
			Actor:       Actor{UserID: "u1", CorpID: "c1"},
			Product:     "test",
			Command:     "cmd",
			Result:      "success",
			DurationMs:  int64(i * 10),
			CLIVersion:  "1.0.0",
			OS:          "linux",
			Arch:        "amd64",
		}
		if err := sink.Emit(evt); err != nil {
			t.Fatal(err)
		}
	}
	sink.Close()

	file, err := LatestAuditFile(dir)
	if err != nil {
		t.Fatal(err)
	}

	valid, brokenAt, err := VerifyFile(file)
	if err != nil {
		t.Fatalf("verify error: %v", err)
	}
	if !valid {
		t.Errorf("expected valid chain, broken at line %d", brokenAt)
	}
}

func TestChainDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	writer, err := NewDateRotatingWriter(dir, 90)
	if err != nil {
		t.Fatal(err)
	}
	chain := NewChain(dir)
	sink := NewFileSink(writer, chain, nil)

	for i := 0; i < 3; i++ {
		evt := &Event{
			Timestamp:   time.Now(),
			ExecutionID: "exec-" + string(rune('0'+i)),
			Actor:       Actor{UserID: "u1", CorpID: "c1"},
			Product:     "test",
			Command:     "cmd",
			Result:      "success",
			DurationMs:  100,
			CLIVersion:  "1.0.0",
			OS:          "linux",
			Arch:        "amd64",
		}
		if err := sink.Emit(evt); err != nil {
			t.Fatal(err)
		}
	}
	sink.Close()

	file, err := LatestAuditFile(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Tamper with the file: modify a character in the second line
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	// Find second newline and change a char after it
	lines := splitLines(data)
	if len(lines) < 2 {
		t.Fatal("expected at least 2 lines")
	}
	// Corrupt the second line by changing first char of product
	var evt2 map[string]any
	json.Unmarshal([]byte(lines[1]), &evt2)
	evt2["product"] = "tampered"
	tampered, _ := json.Marshal(evt2)
	lines[1] = string(tampered)

	corrupted := []byte(lines[0] + "\n" + lines[1] + "\n" + lines[2] + "\n")
	os.WriteFile(file, corrupted, 0o600)

	valid, brokenAt, _ := VerifyFile(file)
	if valid {
		t.Error("expected invalid chain after tampering")
	}
	if brokenAt != 2 {
		t.Errorf("expected break at line 2, got %d", brokenAt)
	}
}

func TestRetention(t *testing.T) {
	dir := t.TempDir()

	// Create old files
	oldDate := time.Now().AddDate(0, 0, -100).Format("20060102")
	recentDate := time.Now().AddDate(0, 0, -10).Format("20060102")
	os.WriteFile(filepath.Join(dir, "audit-"+oldDate+".jsonl"), []byte("old"), 0o600)
	os.WriteFile(filepath.Join(dir, "audit-"+recentDate+".jsonl"), []byte("recent"), 0o600)

	writer, err := NewDateRotatingWriter(dir, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()

	// Give async pruning a moment
	time.Sleep(50 * time.Millisecond)

	if _, err := os.Stat(filepath.Join(dir, "audit-"+oldDate+".jsonl")); !os.IsNotExist(err) {
		t.Error("expected old file to be pruned")
	}
	if _, err := os.Stat(filepath.Join(dir, "audit-"+recentDate+".jsonl")); err != nil {
		t.Error("recent file should still exist")
	}
}

func TestRedactEvent(t *testing.T) {
	evt := Event{
		Actor:         Actor{UserID: "uid123", Name: "张三", CorpID: "corp1", CorpName: "公司A"},
		Product:       "calendar",
		Command:       "list",
		ParamsSummary: `{"date":"2026-01-01"}`,
		Result:        "success",
	}

	hashed := RedactEvent(evt, RedactHashed)
	if hashed.Actor.Name == "张三" {
		t.Error("name should be hashed")
	}
	if hashed.ParamsSummary != "" {
		t.Error("params should be cleared in hashed mode")
	}
	if hashed.Actor.UserID != "uid123" {
		t.Error("user_id should remain in hashed mode")
	}

	minimal := RedactEvent(evt, RedactMinimal)
	if minimal.Actor.UserID == "uid123" {
		t.Error("user_id should be hashed in minimal mode")
	}
	if minimal.Endpoint != "" {
		t.Error("endpoint should be cleared in minimal mode")
	}
}

func TestNopSink(t *testing.T) {
	var s NopSink
	if err := s.Emit(&Event{}); err != nil {
		t.Error("NopSink.Emit should not error")
	}
	if err := s.Close(); err != nil {
		t.Error("NopSink.Close should not error")
	}
}

func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, string(data[start:i]))
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}
