// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageImportFileCompletesUploadAndImportE2E(t *testing.T) {
	uploaded := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if contentType := r.Header.Get("Content-Type"); contentType != "" {
			t.Errorf("Content-Type = %q, want empty", contentType)
		}
		body, _ := io.ReadAll(r.Body)
		uploaded = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	path := filepath.Join(t.TempDir(), "data.xlsx")
	if err := os.WriteFile(path, []byte("workbook"), 0o600); err != nil {
		t.Fatal(err)
	}
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: mustJSONText(t, map[string]any{"uploadUrl": server.URL + "/put", "importId": "imp-1"})},
		{text: `{"success":true,"data":{"importedCount":3}}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+import-file", "--base-id", "base", "--file", path, "--table-id", "table", "--yes")
	if err != nil || uploaded != "workbook" {
		t.Fatalf("import file = output:%q err:%v uploaded:%q", out, err, uploaded)
	}
	for _, want := range []string{`"importId": "imp-1"`, `"status": "service_confirmed"`, `"importedCount": 3`} {
		if !strings.Contains(out, want) {
			t.Fatalf("import output missing %s: %s", want, out)
		}
	}
	if strings.Contains(out, "uploadUrl") || len(caller.calls) != 2 || caller.calls[0].tool != "prepare_import_upload" || caller.calls[1].tool != "import_data" {
		t.Fatalf("import calls/output = calls:%#v output:%s", caller.calls, out)
	}
}

func TestCrossPlatformCoverageImportFileResumeAndUnknownE2E(t *testing.T) {
	t.Run("resume only calls import_data", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"success":true,"data":{"done":true}}`}}}
		out, err := runAITableCompositeCLI(t, caller, "+import-file", "--resume-import-id", "imp-1", "--timeout", "30", "--yes")
		if err != nil || !strings.Contains(out, `"mode": "resume"`) || len(caller.calls) != 1 || caller.calls[0].args["timeout"] != 30 {
			t.Fatalf("resume = output:%q err:%v calls:%#v", out, err, caller.calls)
		}
	})

	t.Run("pending preserves same import id", func(t *testing.T) {
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"ok":true,"outcome":"pending","data":{"taskState":"RUNNING"}}`}}}
		out, err := runAITableCompositeCLI(t, caller, "+import-file", "--resume-import-id", "imp-pending", "--yes")
		if err != nil || !strings.Contains(out, `"outcome": "pending"`) || !strings.Contains(out, `"status": "pending"`) || !strings.Contains(out, `--resume-import-id imp-pending`) {
			t.Fatalf("pending import = output:%q err:%v", out, err)
		}
	})

	t.Run("trigger uncertainty preserves import id", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
		t.Cleanup(server.Close)
		path := filepath.Join(t.TempDir(), "data.csv")
		if err := os.WriteFile(path, []byte("a,b\n1,2\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: mustJSONText(t, map[string]any{"uploadUrl": server.URL, "importId": "imp-unknown"})},
			{err: errors.New("timeout")},
		}}
		out, err := runAITableCompositeCLI(t, caller, "+import-file", "--base-id", "base", "--file", path, "--yes")
		if err == nil || out != "" || !strings.Contains(err.Error(), "status unknown") {
			t.Fatalf("unknown import = output:%q err:%v", out, err)
		}
	})
}

func TestCrossPlatformCoverageImportFileValidationAndRedaction(t *testing.T) {
	caller := &upsertByKeyCaller{}
	workbookPath := filepath.Join(t.TempDir(), "data.xlsx")
	if err := os.WriteFile(workbookPath, []byte("workbook"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--base-id", "base", "--file", filepath.Join(t.TempDir(), "data.json"), "--yes"},
		{"--resume-import-id", "imp", "--table-id", "table", "--yes"},
		{"--base-id", "base", "--yes"},
		{"--resume-import-id", "imp", "--timeout", "0", "--yes"},
		{"--base-id", "base", "--file", workbookPath, "--field-mapping", `{"":"源列"}`, "--yes"},
	} {
		if out, err := runAITableCompositeCLI(t, caller, "+import-file", args...); err == nil || out != "" {
			t.Fatalf("invalid args %v = output:%q err:%v", args, out, err)
		}
	}
	sanitized := sanitizeImportOutput(map[string]any{
		"uploadUrl": "https://example.test?signature=secret",
		"nested":    map[string]any{"accessToken": "token", "count": 2, "message": "upload to https://example.test?signature=secret"},
	}, "").(map[string]any)
	if sanitized["uploadUrl"] != "<redacted>" || sanitized["nested"].(map[string]any)["accessToken"] != "<redacted>" ||
		sanitized["nested"].(map[string]any)["message"] != "<redacted>" {
		t.Fatalf("sanitized = %#v", sanitized)
	}

	csvPath := filepath.Join(t.TempDir(), "data.csv")
	if err := os.WriteFile(csvPath, []byte("name\nAda\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--base-id", "base", "--file", csvPath, "--header-row", "2", "--yes"},
		{"--base-id", "base", "--file", csvPath, "--src-sheet-name", "Sheet1", "--yes"},
	} {
		if out, err := runAITableCompositeCLI(t, caller, "+import-file", args...); err == nil || out != "" {
			t.Fatalf("invalid CSV options %v = output:%q err:%v", args, out, err)
		}
	}
	for _, raw := range []string{"http://example.com/upload", "https://user:secret@example.com/upload"} {
		if err := validateImportUploadURL(raw); err == nil {
			t.Fatalf("unsafe upload URL accepted: %s", raw)
		}
	}
}

func TestCrossPlatformCoverageImportFileOutcomeCompatibility(t *testing.T) {
	for name, tc := range map[string]struct {
		payload map[string]any
		want    string
	}{
		"unified success":         {map[string]any{"ok": true, "outcome": "success"}, "success"},
		"incomplete unified":      {map[string]any{"ok": true, "data": map[string]any{"success": true}}, "invalid"},
		"unified conflict":        {map[string]any{"ok": false, "outcome": "success"}, "invalid"},
		"nested unified conflict": {map[string]any{"ok": true, "outcome": "success", "data": map[string]any{"ok": true, "outcome": "pending"}}, "invalid"},
		"legacy pending":          {map[string]any{"data": map[string]any{"status": "pending"}}, "pending"},
		"legacy failure wins":     {map[string]any{"status": "success", "data": map[string]any{"status": "failure"}}, "failure"},
		"boolean failure":         {map[string]any{"result": map[string]any{"success": false}}, "failure"},
		"boolean conflict":        {map[string]any{"success": true, "result": map[string]any{"success": false}}, "invalid"},
		"unknown":                 {map[string]any{"data": map[string]any{"task": "running"}}, "unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := importFileOutcome(tc.payload); got != tc.want {
				t.Fatalf("outcome = %q, want %q", got, tc.want)
			}
		})
	}
}
