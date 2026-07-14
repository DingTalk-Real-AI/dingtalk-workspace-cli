package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditTailRejectsNonPositiveLines(t *testing.T) {
	for _, n := range []string{"0", "-1"} {
		cmd := newAuditTailCommand()
		cmd.SetArgs([]string{"--lines", n})
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		err := cmd.Execute()
		if err == nil {
			t.Fatalf("--lines %s: expected error, got nil", n)
		}
		if !strings.Contains(err.Error(), "正整数") {
			t.Fatalf("--lines %s: unexpected error: %v", n, err)
		}
	}
}

func TestTailFileReturnsLastN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit-20260101.jsonl")
	if err := os.WriteFile(path, []byte("a\nb\nc\nd\ne\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, err := tailFile(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0] != "d" || lines[1] != "e" {
		t.Fatalf("got %v, want [d e]", lines)
	}
}

func TestExportCSVWritesHeaderAndRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit-20260101.jsonl")
	rec := `{"timestamp":"2026-01-01T00:00:00Z","execution_id":"e1","actor":{"user_id":"u1","corp_id":"c1"},"product":"calendar","command":"event_list","result":"success","duration_ms":12,"hash":"h","prev_hash":""}`
	if err := os.WriteFile(path, []byte(rec+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	exportErr := exportCSV([]string{path})
	w.Close()
	os.Stdout = stdout

	if exportErr != nil {
		t.Fatalf("exportCSV error: %v", exportErr)
	}
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	out := string(buf[:n])
	if !strings.Contains(out, "timestamp,execution_id") {
		t.Fatalf("missing CSV header, got: %q", out)
	}
	if !strings.Contains(out, "e1") || !strings.Contains(out, "event_list") {
		t.Fatalf("missing CSV row data, got: %q", out)
	}
}

// TestExportCSVFailsOnMalformedJSON guards the reviewer's V9 finding: a corrupt
// JSONL line must surface an error with file/line evidence instead of being
// silently skipped while the command exits 0.
func TestExportCSVFailsOnMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit-20260101.jsonl")
	good := `{"timestamp":"2026-01-01T00:00:00Z","execution_id":"e1","actor":{"user_id":"u1"},"product":"calendar","command":"event_list","result":"success","duration_ms":1,"hash":"h","prev_hash":""}`
	if err := os.WriteFile(path, []byte(good+"\nnot-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	exportErr := exportCSV([]string{path})
	w.Close()
	os.Stdout = stdout
	// Drain the pipe so the writer never blocks.
	buf := make([]byte, 4096)
	_, _ = r.Read(buf)

	if exportErr == nil {
		t.Fatal("expected error on malformed JSONL, got nil")
	}
	if !strings.Contains(exportErr.Error(), "解析审计记录失败") {
		t.Fatalf("error missing parse context: %v", exportErr)
	}
	if !strings.Contains(exportErr.Error(), ":2") {
		t.Fatalf("error missing line evidence: %v", exportErr)
	}
}
