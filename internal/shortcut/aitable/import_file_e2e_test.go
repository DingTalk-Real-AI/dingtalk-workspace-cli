// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
)

type importCloseErrorBody struct {
	io.Reader
}

func (importCloseErrorBody) Close() error { return errors.New("close failed") }

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
		{"--base-id", "base", "--file", workbookPath, "--header-row", "0", "--yes"},
		{"--base-id", "base", "--file", workbookPath, "--field-mapping", `{"":"源列"}`, "--yes"},
		{"--base-id", "base", "--file", workbookPath, "--field-mapping", `{`, "--yes"},
		{"--base-id", "base", "--file", workbookPath, "--field-mapping", `{"目标":1}`, "--yes"},
		{"--base-id", "base", "--file", workbookPath, "--field-mapping", `{"目标":" "}`, "--yes"},
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
		"unified success":            {map[string]any{"ok": true, "outcome": "success"}, "success"},
		"incomplete unified":         {map[string]any{"ok": true, "data": map[string]any{"success": true}}, "invalid"},
		"unified conflict":           {map[string]any{"ok": false, "outcome": "success"}, "invalid"},
		"unified failure":            {map[string]any{"ok": false, "outcome": "failure"}, "failure"},
		"unified partial failure":    {map[string]any{"ok": false, "outcome": "partial_failure"}, "partial_failure"},
		"invalid successful failure": {map[string]any{"ok": true, "outcome": "partial_failure"}, "invalid"},
		"unified invalid outcome":    {map[string]any{"ok": true, "outcome": "other"}, "invalid"},
		"unified missing ok":         {map[string]any{"outcome": "success"}, "invalid"},
		"nested unified conflict":    {map[string]any{"ok": true, "outcome": "success", "data": map[string]any{"ok": true, "outcome": "pending"}}, "invalid"},
		"legacy pending":             {map[string]any{"data": map[string]any{"status": "pending"}}, "pending"},
		"legacy failure wins":        {map[string]any{"status": "success", "data": map[string]any{"status": "failure"}}, "failure"},
		"boolean failure":            {map[string]any{"result": map[string]any{"success": false}}, "failure"},
		"boolean success":            {map[string]any{"result": map[string]any{"success": true}}, "success"},
		"boolean conflict":           {map[string]any{"success": true, "result": map[string]any{"success": false}}, "invalid"},
		"unknown":                    {map[string]any{"data": map[string]any{"task": "running"}}, "unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := importFileOutcome(tc.payload); got != tc.want {
				t.Fatalf("outcome = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestImportFileDryRunAndFailureStages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.xlsx")
	if err := os.WriteFile(path, []byte("workbook"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{
		"initial":         {"--base-id", "base", "--file", path, "--dry-run", "--yes"},
		"initial options": {"--base-id", "base", "--file", path, "--table-id", "t", "--timeout", "10", "--header-row", "2", "--src-sheet-name", "Sheet1", "--field-mapping", `{"目标":"源"}`, "--dry-run", "--yes"},
		"resume":          {"--resume-import-id", "imp", "--dry-run", "--yes"},
	} {
		t.Run("dry run "+name, func(t *testing.T) {
			out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{}, "+import-file", args...)
			if err != nil || !strings.Contains(out, `"status": "planned"`) {
				t.Fatalf("dry run = %q, %v", out, err)
			}
		})
	}

	for name, step := range map[string]upsertByKeyStep{
		"prepare error":     {err: errors.New("prepare failed")},
		"prepare malformed": {text: `{}`},
		"unsafe URL":        {text: `{"uploadUrl":"http://example.com/upload","importId":"imp"}`},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: []upsertByKeyStep{step}}, "+import-file",
				"--base-id", "base", "--file", path, "--yes")
			if err == nil || out != "" {
				t.Fatalf("failure = %q, %v", out, err)
			}
		})
	}
}

func TestImportFileHTTPAndTerminalFailureStages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.xlsx")
	if err := os.WriteFile(path, []byte("workbook"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := importHTTPDo
	originalRequest := importNewRequestWithContext
	t.Cleanup(func() {
		importHTTPDo = original
		importNewRequestWithContext = originalRequest
	})
	prepare := upsertByKeyStep{text: `{"uploadUrl":"https://upload.example.test/put","importId":"imp"}`}

	t.Run("transport error", func(t *testing.T) {
		importHTTPDo = func(*http.Request) (*http.Response, error) { return nil, errors.New("signed URL secret") }
		_, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: []upsertByKeyStep{prepare}}, "+import-file",
			"--base-id", "base", "--file", path, "--yes")
		if err == nil || strings.Contains(err.Error(), "signed URL secret") {
			t.Fatalf("transport error = %v", err)
		}
	})

	t.Run("request construction", func(t *testing.T) {
		importNewRequestWithContext = func(context.Context, string, string, io.Reader) (*http.Request, error) {
			return nil, errors.New("construct failed")
		}
		_, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: []upsertByKeyStep{prepare}}, "+import-file",
			"--base-id", "base", "--file", path, "--yes")
		if err == nil {
			t.Fatal("request construction failure succeeded")
		}
		importNewRequestWithContext = originalRequest
	})

	t.Run("HTTP status", func(t *testing.T) {
		importHTTPDo = func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader("denied"))}, nil
		}
		_, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: []upsertByKeyStep{prepare}}, "+import-file",
			"--base-id", "base", "--file", path, "--yes")
		if err == nil {
			t.Fatalf("status error = %v", err)
		}
	})

	t.Run("response close", func(t *testing.T) {
		importHTTPDo = func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: importCloseErrorBody{Reader: strings.NewReader("")}}, nil
		}
		_, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: []upsertByKeyStep{prepare}}, "+import-file",
			"--base-id", "base", "--file", path, "--yes")
		if err == nil {
			t.Fatalf("close error = %v", err)
		}
	})

	for name, terminal := range map[string]string{
		"failure": `{"ok":false,"outcome":"failure"}`,
		"unknown": `{"data":{"task":"unknown"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			importHTTPDo = func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
			caller := &upsertByKeyCaller{steps: []upsertByKeyStep{prepare, {text: terminal}}}
			if _, err := runAITableCompositeCLI(t, caller, "+import-file", "--base-id", "base", "--file", path, "--yes"); err == nil {
				t.Fatal("terminal failure succeeded")
			}
		})
	}

	importHTTPDo = original
	if _, err := runAITableCompositeCLI(t, &upsertByKeyCaller{steps: []upsertByKeyStep{{err: errors.New("resume failed")}}}, "+import-file",
		"--resume-import-id", "imp", "--yes"); err == nil {
		t.Fatal("resume transport error succeeded")
	}
}

func TestImportFileRedirectAndUnifiedPendingBranches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/other", http.StatusFound)
	}))
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "data.xlsx")
	if err := os.WriteFile(path, []byte("workbook"), 0o600); err != nil {
		t.Fatal(err)
	}
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: mustJSONText(t, map[string]any{
		"uploadUrl": server.URL + "/put", "importId": "imp",
	})}}}
	if _, err := runAITableCompositeCLI(t, caller, "+import-file", "--base-id", "base", "--file", path, "--yes"); err == nil {
		t.Fatal("redirected upload succeeded")
	}

	ctx, _ := output.WithResultStore(context.Background())
	cmd := &cobra.Command{Use: "+import-file"}
	cmd.SetContext(ctx)
	output.SetCommandRollout(cmd, output.RolloutUnifiedActive)
	rt := shortcut.RuntimeContextForTest(cmd, shortcut.Shortcut{
		Service: "aitable", Command: "+import-file", Safety: contract.SafetySpec{Effect: "write"},
	})
	if err := finishImportFile(rt, newCompositeResult("import_file"), "resume", "imp", map[string]any{
		"ok": true, "outcome": "pending",
	}, 1); err != nil {
		t.Fatalf("unified pending: %v", err)
	}
}

func TestImportFilePureValidationBranches(t *testing.T) {
	directory := t.TempDir()
	empty := filepath.Join(directory, "empty.csv")
	unsupported := filepath.Join(directory, "data.json")
	valid := filepath.Join(directory, "data.xls")
	for path, data := range map[string][]byte{empty: {}, unsupported: []byte("x"), valid: []byte("x")} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{filepath.Join(directory, "missing.csv"), directory, empty, unsupported} {
		if file, _, err := openImportFile(path); err == nil {
			file.Close()
			t.Fatalf("openImportFile(%q) succeeded", path)
		}
	}
	file, _, err := openImportFile(valid)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()

	for _, raw := range []string{"https://example.com/upload", "http://localhost/upload", "http://127.0.0.1/upload"} {
		if err := validateImportUploadURL(raw); err != nil {
			t.Fatalf("validateImportUploadURL(%q): %v", raw, err)
		}
	}
	for _, raw := range []string{"not a url", "ftp://example.com/upload"} {
		if err := validateImportUploadURL(raw); err == nil {
			t.Fatalf("validateImportUploadURL(%q) succeeded", raw)
		}
	}

	value := sanitizeImportOutput([]any{
		"safe", "authorization: secret", map[string]any{"api-key": "secret", "count": 1},
	}, "payload").([]any)
	if value[0] != "safe" || value[1] != "<redacted>" || value[2].(map[string]any)["api-key"] != "<redacted>" {
		t.Fatalf("sanitizeImportOutput() = %#v", value)
	}
}
