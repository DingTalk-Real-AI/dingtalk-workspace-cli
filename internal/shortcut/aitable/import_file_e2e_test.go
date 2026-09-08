// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
)

type importCloseErrorBody struct {
	io.Reader
}

func (importCloseErrorBody) Close() error { return errors.New("close failed") }

// useImportUploadTestServer is intentionally a test-only seam. Production
// validation never accepts loopback or plaintext upload targets.
func useImportUploadTestServer(t *testing.T, server *httptest.Server) {
	t.Helper()
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	originalValidator, originalDo := importUploadURLValidator, importHTTPDo
	importUploadURLValidator = func(raw string) error {
		uploadURL, err := url.Parse(raw)
		if err != nil || uploadURL.Scheme != serverURL.Scheme || uploadURL.Host != serverURL.Host {
			return errors.New("test upload URL is outside the local server")
		}
		return nil
	}
	importHTTPDo = server.Client().Do
	t.Cleanup(func() {
		importUploadURLValidator = originalValidator
		importHTTPDo = originalDo
	})
}

func TestCrossPlatformCoverageImportFileRequiresConfirmationBeforeUpload(t *testing.T) {
	putCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		putCalls++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	useImportUploadTestServer(t, server)

	path := filepath.Join(t.TempDir(), "data.xlsx")
	if err := os.WriteFile(path, []byte("workbook"), 0o600); err != nil {
		t.Fatal(err)
	}
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{
		text: mustJSONText(t, map[string]any{"uploadUrl": server.URL + "/put", "importId": "imp-1"}),
	}}}
	out, err := runAITableCompositeCLI(t, caller, "+import-file", "--base-id", "base", "--file", path)
	if out != "" {
		t.Fatalf("unconfirmed import output = %q, want empty", out)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "confirmation_required" {
		t.Fatalf("unconfirmed import error = %#v, want confirmation_required", err)
	}
	if len(caller.calls) != 0 || putCalls != 0 {
		t.Fatalf("unconfirmed import performed side effects: MCP calls=%#v PUT calls=%d", caller.calls, putCalls)
	}
}

func TestCrossPlatformCoverageImportFileCompletesUploadAndImportE2E(t *testing.T) {
	uploaded := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if _, exists := r.Header["Content-Type"]; exists {
			t.Errorf("Content-Type header must be absent: %#v", r.Header.Values("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		uploaded = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	useImportUploadTestServer(t, server)

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
		useImportUploadTestServer(t, server)
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
	for _, raw := range []string{"http://example.com/upload", "https://user:secret@example.com/upload", "https://example.com/upload"} {
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

func TestCrossPlatformCoverageImportFileDryRunAndFailureStages(t *testing.T) {
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

func TestCrossPlatformCoverageImportFileHTTPAndTerminalFailureStages(t *testing.T) {
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
	prepare := upsertByKeyStep{text: `{"uploadUrl":"https://alidocs-notable.cn-zhangjiakou.oss.aliyuncs.com/put","importId":"imp"}`}

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

func TestCrossPlatformCoverageImportFileRedirectAndUnifiedPendingBranches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/other", http.StatusFound)
	}))
	t.Cleanup(server.Close)
	useImportUploadTestServer(t, server)
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

func TestCrossPlatformCoverageImportFilePureValidationBranches(t *testing.T) {
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

	for _, raw := range []string{
		"https://alidocs-notable.cn-zhangjiakou.oss.aliyuncs.com/upload",
		"https://alidocs-notable-sg.ap-southeast-1.oss.aliyuncs.com/upload",
	} {
		if err := validateImportUploadURL(raw); err != nil {
			t.Fatalf("validateImportUploadURL(%q): %v", raw, err)
		}
	}
	for _, raw := range []string{
		"not a url",
		"ftp://example.com/upload",
		"http://localhost/upload",
		"https://example.com/upload",
		"https://127.0.0.1/upload",
		"https://alidocs-notable.cn-zhangjiakou.oss.aliyuncs.com:444/upload",
	} {
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

func TestCrossPlatformCoverageImportFileRejectsUntrustedUploadBeforePUT(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.xlsx")
	if err := os.WriteFile(path, []byte("private workbook bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	originalDo := importHTTPDo
	putCalls := 0
	importHTTPDo = func(*http.Request) (*http.Response, error) {
		putCalls++
		return nil, errors.New("unexpected PUT")
	}
	t.Cleanup(func() { importHTTPDo = originalDo })

	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{
		text: `{"uploadUrl":"https://attacker.example/upload","importId":"imp"}`,
	}}}
	if output, err := runAITableCompositeCLI(t, caller, "+import-file", "--base-id", "base", "--file", path, "--yes"); err == nil || output != "" {
		t.Fatalf("untrusted upload = output:%q err:%v", output, err)
	}
	if putCalls != 0 || len(caller.calls) != 1 || caller.calls[0].tool != "prepare_import_upload" {
		t.Fatalf("untrusted upload sent file bytes: PUT=%d MCP=%#v", putCalls, caller.calls)
	}
}

func TestCrossPlatformCoverageImportUploadDialerPinsPublicAddress(t *testing.T) {
	originalLookup, originalDial := importUploadLookupIPAddr, importUploadDial
	t.Cleanup(func() {
		importUploadLookupIPAddr = originalLookup
		importUploadDial = originalDial
	})

	t.Run("rejects private DNS answer before dialing", func(t *testing.T) {
		importUploadLookupIPAddr = func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		}
		dialed := false
		importUploadDial = func(context.Context, string, string) (net.Conn, error) {
			dialed = true
			return nil, errors.New("unexpected dial")
		}
		if _, err := dialTrustedImportUpload(context.Background(), "tcp", "alidocs-notable.cn-zhangjiakou.oss.aliyuncs.com:443"); err == nil || dialed {
			t.Fatalf("private DNS answer = err:%v dialed:%v", err, dialed)
		}
	})

	t.Run("rejects untrusted host and lookup failure", func(t *testing.T) {
		if _, err := dialTrustedImportUpload(context.Background(), "tcp", "attacker.example:443"); err == nil {
			t.Fatal("untrusted host dial succeeded")
		}
		importUploadLookupIPAddr = func(context.Context, string) ([]net.IPAddr, error) {
			return nil, errors.New("lookup failed")
		}
		if _, err := dialTrustedImportUpload(context.Background(), "tcp", "alidocs-notable.cn-zhangjiakou.oss.aliyuncs.com:443"); err == nil {
			t.Fatal("lookup failure dial succeeded")
		}
	})

	t.Run("returns final connection failure", func(t *testing.T) {
		importUploadLookupIPAddr = func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		}
		importUploadDial = func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("connect failed")
		}
		if _, err := dialTrustedImportUpload(context.Background(), "tcp", "alidocs-notable.cn-zhangjiakou.oss.aliyuncs.com:443"); err == nil {
			t.Fatal("connection failure dial succeeded")
		}
	})

	t.Run("dials resolved IP rather than hostname", func(t *testing.T) {
		importUploadLookupIPAddr = func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		}
		importUploadDial = func(_ context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" || address != "8.8.8.8:443" {
				t.Fatalf("dial = %s %s", network, address)
			}
			client, server := net.Pipe()
			t.Cleanup(func() { server.Close() })
			return client, nil
		}
		connection, err := dialTrustedImportUpload(context.Background(), "tcp", "alidocs-notable.cn-zhangjiakou.oss.aliyuncs.com:443")
		if err != nil {
			t.Fatal(err)
		}
		connection.Close()
	})
}

func TestCrossPlatformCoverageImportUploadClientRejectsRedirects(t *testing.T) {
	if err := rejectImportUploadRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("redirect error = %v", err)
	}
	client := newImportUploadHTTPClient()
	if client.Timeout != importUploadRequestTimeout {
		t.Fatalf("import upload timeout = %s, want %s", client.Timeout, importUploadRequestTimeout)
	}
	if transport, ok := client.Transport.(*http.Transport); !ok || transport.Proxy != nil {
		t.Fatalf("import upload transport must use direct connections: %#v", transport)
	}
}
