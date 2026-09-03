package helpers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

func executeDriveEdge(t *testing.T, caller *scriptedToolCaller, args ...string) error {
	t.Helper()
	oldDeps := deps
	oldArgs := os.Args
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	t.Cleanup(func() {
		deps = oldDeps
		os.Args = oldArgs
	})
	root := newDriveCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	os.Args = append([]string{"dws", "drive"}, args...)
	ctx, _ := output.WithResultStore(context.Background())
	executed, err := root.ExecuteContextC(ctx)
	if err != nil {
		return err
	}
	_, _, err = output.EmitStoredResult(executed)
	return err
}

func TestCrossPlatformCoverageParseDriveUploadInfoRemainingCoverage(t *testing.T) {
	cases := []struct {
		name string
		json string
		err  bool
	}{
		{"resource array", `{"result":{"uploadId":"u","resourceUrls":[{"url":"https://upload.invalid","headers":{"X-Test":"yes","skip":1}}]}}`, false},
		{"flat resource and headers", `{"uploadId":"u","resourceUrl":"https://upload.invalid","headers":{"X-Test":"yes","skip":1}}`, false},
		{"upload URL fallback", `{"uploadId":"u","uploadUrl":"https://upload.invalid"}`, false},
		{"non-map first URL", `{"uploadId":"u","resourceUrls":[1]}`, true},
		{"missing upload id", `{"resourceUrl":"https://upload.invalid"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url, id, headers, err := parseDriveUploadInfo(tc.json)
			if (err != nil) != tc.err {
				t.Fatalf("parse error=%v, wantErr=%v", err, tc.err)
			}
			if !tc.err && (url == "" || id == "" || headers == nil) {
				t.Fatalf("parse result url=%q id=%q headers=%v", url, id, headers)
			}
		})
	}
}

func TestCrossPlatformCoverageDriveUploadValidationAndDryRunCoverage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "fixture.txt")
	if err := os.WriteFile(file, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		args []string
		err  bool
	}{
		{"missing file", []string{"upload"}, true},
		{"unreadable file", []string{"upload", "--file", file + ".missing"}, true},
		{"directory", []string{"upload", "--file", t.TempDir()}, true},
		{"numeric drive folder", []string{"upload", "--file", file, "--folder", "123"}, true},
		{"drive dry run", []string{"upload", "--file", file, "--file-name", "named.txt", "--dry-run"}, false},
		{"numeric doc folder", []string{"upload", "--file", file, "--workspace", "space", "--folder", "123"}, true},
		{"doc dry run extension", []string{"upload", "--file", file, "--file-name", "named", "--workspace", "space", "--dry-run"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := executeDriveEdge(t, &scriptedToolCaller{dry: strings.Contains(strings.Join(tc.args, " "), "--dry-run")}, tc.args...)
			if (err != nil) != tc.err {
				t.Fatalf("Execute(%v) error=%v, wantErr=%v", tc.args, err, tc.err)
			}
		})
	}
}

func TestCrossPlatformCoverageDriveUploadTransportCoverage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "fixture.txt")
	if err := os.WriteFile(file, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	t.Run("drive credentials error", func(t *testing.T) {
		if err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{{err: boom}}}, "upload", "--file", file); !errors.Is(err, boom) {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("drive credentials parse error", func(t *testing.T) {
		if err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}}, "upload", "--file", file); err == nil {
			t.Fatal("parse error returned nil")
		}
	})
	t.Run("drive put error", func(t *testing.T) {
		old := httpPutFile
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return boom }
		t.Cleanup(func() { httpPutFile = old })
		payload := `{"uploadId":"u","resourceUrl":"https://upload.invalid"}`
		if err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: payload}}}, "upload", "--file", file); !errors.Is(err, boom) {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("drive commit", func(t *testing.T) {
		old := httpPutFile
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
		t.Cleanup(func() { httpPutFile = old })
		payload := `{"uploadId":"u","resourceUrl":"https://upload.invalid"}`
		if err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: payload}, {text: `{}`}}}, "upload", "--file", file, "--space-id", "space", "--folder", "uuid"); err != nil {
			t.Fatalf("error=%v", err)
		}
	})

	docArgs := []string{"upload", "--file", file, "--workspace", "space", "--folder", "uuid", "--convert"}
	t.Run("doc credentials error", func(t *testing.T) {
		if err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{{err: boom}}}, docArgs...); !errors.Is(err, boom) {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("doc credentials parse error", func(t *testing.T) {
		if err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}}, docArgs...); err == nil {
			t.Fatal("parse error returned nil")
		}
	})
	t.Run("doc put error", func(t *testing.T) {
		old := httpPutFile
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return boom }
		t.Cleanup(func() { httpPutFile = old })
		payload := `{"resourceUrl":"https://upload.invalid","uploadKey":"key"}`
		if err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: payload}}}, docArgs...); !errors.Is(err, boom) {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("doc commit", func(t *testing.T) {
		old := httpPutFile
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
		t.Cleanup(func() { httpPutFile = old })
		payload := `{"resourceUrl":"https://upload.invalid","uploadKey":"key"}`
		if err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: payload}, {text: `{}`}}}, docArgs...); err != nil {
			t.Fatalf("error=%v", err)
		}
	})
}

func TestCrossPlatformCoverageUploadDriveFileDataStrictTransaction(t *testing.T) {
	credential := `{"result":{"uploadType":"httpToCenterWithToken","resourceUrl":"https://c.example.com/u?upload_key=u1","uploadId":"u1","headers":{"dentry-token":"token"}}}`
	request := DriveUploadRequest{FilePath: "fixture.bin", FileName: "fixture.bin", FileSize: 7, SpaceID: "space", ParentID: "folder", MIMEType: "application/octet-stream"}

	t.Run("success with parent", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: credential}, {text: `{"success":true,"result":{"fileId":"n1"}}`}}}
		installScriptedCaller(t, caller)
		SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
		t.Cleanup(func() { SetHTTPPutFile(nil) })
		result, err := UploadDriveFileData(context.Background(), request)
		if err != nil || result["success"] != true || caller.calls != 2 {
			t.Fatalf("result=%v calls=%d error=%v", result, caller.calls, err)
		}
		if caller.args["spaceId"] != "space" || caller.args["parentId"] != "folder" {
			t.Fatalf("commit args=%v", caller.args)
		}
	})

	t.Run("success with overwrite", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: credential}, {text: `{"success":true}`}}}
		installScriptedCaller(t, caller)
		SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
		t.Cleanup(func() { SetHTTPPutFile(nil) })
		overwrite := request
		overwrite.ParentID = "ignored"
		overwrite.OverwriteFile = "existing"
		if _, err := UploadDriveFileData(context.Background(), overwrite); err != nil {
			t.Fatal(err)
		}
		if caller.args["overwriteFileId"] != "existing" {
			t.Fatalf("commit args=%v", caller.args)
		}
	})

	t.Run("credential refresh callback", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: credential}, {text: credential}, {text: `{"success":true}`}}}
		installScriptedCaller(t, caller)
		putCalls := 0
		SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error {
			putCalls++
			if putCalls == 1 {
				return &httpStatusError{StatusCode: 401, Body: "expired"}
			}
			return nil
		})
		t.Cleanup(func() { SetHTTPPutFile(nil) })
		if _, err := UploadDriveFileData(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		if caller.calls != 3 || putCalls != 2 {
			t.Fatalf("caller calls=%d put calls=%d", caller.calls, putCalls)
		}
	})

	for _, tc := range []struct {
		name    string
		request DriveUploadRequest
		steps   []scriptedToolStep
		putErr  error
		want    string
	}{
		{name: "invalid request", request: DriveUploadRequest{}, want: "invalid drive upload request"},
		{name: "credential failure", request: request, steps: []scriptedToolStep{{err: errors.New("credentials")}}, want: "credentials"},
		{name: "put failure", request: request, steps: []scriptedToolStep{{text: credential}}, putErr: errors.New("put failed"), want: "put failed"},
		{name: "commit failure", request: request, steps: []scriptedToolStep{{text: credential}, {err: errors.New("commit failed")}}, want: "commit failed"},
		{name: "empty commit", request: request, steps: []scriptedToolStep{{text: credential}, {text: "  "}}, want: "no business result"},
		{name: "malformed commit", request: request, steps: []scriptedToolStep{{text: credential}, {text: "{"}}, want: "parse commit_upload response"},
		{name: "empty object commit", request: request, steps: []scriptedToolStep{{text: credential}, {text: `{}`}}, want: "empty JSON object"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: tc.steps}
			installScriptedCaller(t, caller)
			SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return tc.putErr })
			t.Cleanup(func() { SetHTTPPutFile(nil) })
			_, err := UploadDriveFileData(context.Background(), tc.request)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageUploadDocSpaceFileDataStrictTransaction(t *testing.T) {
	credential := `{"resourceUrl":"https://upload.invalid/resource","uploadKey":"upload-key","headers":{"x-token":"token"}}`
	request := DocSpaceUploadRequest{FilePath: "fixture.txt", FileName: "fixture.txt", FileSize: 7, WorkspaceID: "wiki-1", FolderID: "folder-1"}

	t.Run("success preserves doc-space routing and identifiers", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: credential}, {text: `{"success":true,"result":{"nodeId":"n1"}}`}}}
		installScriptedCaller(t, caller)
		SetHTTPPutFile(func(_ context.Context, resourceURL string, headers map[string]string, path string, size int64) error {
			if resourceURL != "https://upload.invalid/resource" || headers["x-token"] != "token" || path != "fixture.txt" || size != 7 {
				t.Fatalf("PUT args url=%q headers=%v path=%q size=%d", resourceURL, headers, path, size)
			}
			return nil
		})
		t.Cleanup(func() { SetHTTPPutFile(nil) })
		result, err := UploadDocSpaceFileData(context.Background(), request)
		if err != nil || result["success"] != true || caller.calls != 2 {
			t.Fatalf("result=%v calls=%d error=%v", result, caller.calls, err)
		}
		if strings.Join(caller.serverLog, ",") != "doc,doc" || strings.Join(caller.toolLog, ",") != "get_file_upload_info,commit_uploaded_file" {
			t.Fatalf("route servers=%v tools=%v", caller.serverLog, caller.toolLog)
		}
		if caller.argsLog[0]["workspaceId"] != "wiki-1" || caller.argsLog[0]["folderId"] != "folder-1" {
			t.Fatalf("credential args=%v", caller.argsLog[0])
		}
		if caller.argsLog[1]["workspaceId"] != "wiki-1" || caller.argsLog[1]["folderId"] != "folder-1" || caller.argsLog[1]["uploadKey"] != "upload-key" {
			t.Fatalf("commit args=%v", caller.argsLog[1])
		}
	})

	t.Run("overwrite excludes folder and supports conversion", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: credential}, {text: `{"success":true}`}}}
		installScriptedCaller(t, caller)
		SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return nil })
		t.Cleanup(func() { SetHTTPPutFile(nil) })
		overwrite := request
		overwrite.FolderID = ""
		overwrite.OverwriteNode = "existing"
		overwrite.Convert = true
		if _, err := UploadDocSpaceFileData(context.Background(), overwrite); err != nil {
			t.Fatal(err)
		}
		for index, args := range caller.argsLog {
			if args["overwriteNodeId"] != "existing" {
				t.Fatalf("call[%d] overwrite args=%v", index, args)
			}
			if _, exists := args["folderId"]; exists {
				t.Fatalf("call[%d] leaked folderId in overwrite mode: %v", index, args)
			}
		}
		if caller.argsLog[1]["convertToOnlineDoc"] != true {
			t.Fatalf("commit conversion args=%v", caller.argsLog[1])
		}
	})

	for _, tc := range []struct {
		name    string
		request DocSpaceUploadRequest
		steps   []scriptedToolStep
		putErr  error
		want    string
	}{
		{name: "invalid request", request: DocSpaceUploadRequest{}, want: "invalid document-space upload request"},
		{name: "folder overwrite conflict", request: DocSpaceUploadRequest{FilePath: "f", FileName: "f", FileSize: 1, WorkspaceID: "w", FolderID: "folder", OverwriteNode: "node"}, want: "mutually exclusive"},
		{name: "credential failure", request: request, steps: []scriptedToolStep{{err: errors.New("credentials")}}, want: "credentials"},
		{name: "credential parse failure", request: request, steps: []scriptedToolStep{{text: `{}`}}, want: "incomplete upload credentials"},
		{name: "put failure", request: request, steps: []scriptedToolStep{{text: credential}}, putErr: errors.New("put failed"), want: "put failed"},
		{name: "commit failure", request: request, steps: []scriptedToolStep{{text: credential}, {err: errors.New("commit failed")}}, want: "commit failed"},
		{name: "empty commit", request: request, steps: []scriptedToolStep{{text: credential}, {text: " "}}, want: "no business result"},
		{name: "malformed commit", request: request, steps: []scriptedToolStep{{text: credential}, {text: "{"}}, want: "parse commit_uploaded_file response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: tc.steps}
			installScriptedCaller(t, caller)
			SetHTTPPutFile(func(context.Context, string, map[string]string, string, int64) error { return tc.putErr })
			t.Cleanup(func() { SetHTTPPutFile(nil) })
			_, err := UploadDocSpaceFileData(context.Background(), tc.request)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageDriveCommandRemainingEdges(t *testing.T) {
	file := filepath.Join(t.TempDir(), "fixture.txt")
	_ = os.WriteFile(file, []byte("fixture"), 0o600)
	cases := [][]string{
		{"list", "--workspace", "space", "--folder", "123"},
		{"mkdir", "--name", "folder", "--folder", "123"},
		{"upload-info", "--file-name", "x", "--file-size", "1", "--folder", "123"},
		{"commit", "--file-name", "x", "--file-size", "1", "--upload-id", "u", "--folder", "123"},
		{"copy", "--node", "node", "--folder", "123"},
		{"move", "--node", "node", "--folder", "123"},
	}
	for _, args := range cases {
		if err := executeDriveEdge(t, &scriptedToolCaller{}, args...); err == nil {
			t.Fatalf("Execute(%v) returned nil", args)
		}
	}
	if err := executeDriveEdge(t, &scriptedToolCaller{dry: true}, "download", "--node", "node", "--output", file, "--dry-run"); err != nil {
		t.Fatalf("dry download: %v", err)
	}
}

func TestCrossPlatformCoverageDriveDownloadDirectoryCoverage(t *testing.T) {
	oldGet := httpGetFile
	// 下载引擎现在先写 <dest>.dwspart 再原子发布；stub 必须履约写入目标路径。
	httpGetFile = func(_ context.Context, _ string, _ map[string]string, destPath string) error {
		return os.WriteFile(destPath, []byte("payload"), 0o644)
	}
	t.Cleanup(func() { httpGetFile = oldGet })
	dir := t.TempDir()
	for _, payload := range []string{
		`{"resourceUrl":"https://download.invalid/path/from-url.txt","fileName":""}`,
		`{"resourceUrl":"https://download.invalid/path/from-url.txt","fileName":"folder/name.txt"}`,
	} {
		if err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: payload}}}, "download", "--node", "node", "--output", dir); err != nil {
			t.Fatalf("download: %v", err)
		}
	}
}

func TestCrossPlatformCoverageDriveConfirmationCancellationCoverage(t *testing.T) {
	oldStdin := os.Stdin
	t.Cleanup(func() { os.Stdin = oldStdin })
	for _, test := range []struct {
		args                 []string
		wantObservableCancel bool
	}{
		{args: []string{"delete", "--node", "node"}, wantObservableCancel: true},
		{args: []string{"publish", "set", "--node", "node"}},
		{args: []string{"publish", "unset", "--node", "node"}},
	} {
		file, err := os.CreateTemp(t.TempDir(), "stdin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteString("no\n"); err != nil {
			t.Fatal(err)
		}
		_, _ = file.Seek(0, 0)
		os.Stdin = file
		err = executeDriveEdge(t, &scriptedToolCaller{}, test.args...)
		if err == nil || !strings.Contains(err.Error(), "用户取消了操作") {
			t.Fatalf("cancel %v = %v, want 用户取消了操作", test.args, err)
		}
		_ = file.Close()
	}
}

func TestCrossPlatformCoverageDriveInfoDocFallbackCoverage(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "drive", "info"}
	t.Cleanup(func() { os.Args = oldArgs })
	boom := errors.New("boom")
	cases := []struct {
		name  string
		steps []scriptedToolStep
		err   bool
	}{
		{"drive error", []scriptedToolStep{{err: boom}}, true},
		{"invalid drive JSON", []scriptedToolStep{{text: `{`}}, false},
		{"no result", []scriptedToolStep{{text: `{}`}}, false},
		{"ordinary file", []scriptedToolStep{{text: `{"result":{"extension":"pdf"}}`}}, false},
		{"doc lookup error", []scriptedToolStep{{text: `{"result":{"message":"钉钉文档","fileId":""}}`}, {err: boom}}, false},
		{"invalid doc JSON", []scriptedToolStep{{text: `{"result":{"extension":"adoc","fileId":"node"}}`}, {text: `{`}}, false},
		{"empty flat doc", []scriptedToolStep{{text: `{"result":{"extension":"axls","fileId":"node"}}`}, {text: `{}`}}, false},
		{"flat doc merge", []scriptedToolStep{{text: `{"result":{"extension":"amind","fileId":"node","path":"/drive","fileSize":3,"type":"doc"}}`}, {text: `{"title":"Doc","path":"existing"}`}}, false},
		{"wrapped doc", []scriptedToolStep{{text: `{"result":{"extension":"adraw","fileId":"node","dentryId":"d"}}`}, {text: `{"result":{"title":"Doc"}}`}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: tc.steps}
			old := deps
			InitDeps(caller)
			deps.Out.w = &bytes.Buffer{}
			deps.Out.errW = io.Discard
			t.Cleanup(func() { deps = old })
			err := driveInfoWithDocFallback("fallback-node", map[string]any{"fileId": "fallback-node"})
			if (err != nil) != tc.err {
				t.Fatalf("error=%v, wantErr=%v", err, tc.err)
			}
		})
	}
}

func TestCrossPlatformCoverageDriveSmallHelperEdges(t *testing.T) {
	for _, ext := range []string{"adoc", "AXLS", "amind", "adraw", "pdf"} {
		_ = isDingTalkDocExtension(ext)
	}
	cmd := &cobra.Command{Use: "upload"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("file-name", "", "")
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("workspace", "", "")
	cmd.Flags().String("workspace-id", "", "")
	cmd.Flags().String("folder", "", "")
	cmd.Flags().String("parent-id", "", "")
	cmd.Flags().String("space-id", "", "")
	cmd.Flags().String("mime-type", "", "")
	cmd.Flags().Bool("convert", false, "")
	if err := cmd.Flags().Set("file", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := runDriveUpload(cmd, nil); err == nil {
		t.Fatal("directory upload returned nil")
	}
}

// TestCrossPlatformCoverageUploadToDocSpaceStep1Args asserts that
// uploadToDocSpace unconditionally populates name and fileSize in the
// get_file_upload_info step1 args, aligning with uploadToDrive and
// pushUploadWithTransport. This enables delegation-auth strong validation
// of fileName on the server side.
func TestCrossPlatformCoverageUploadToDocSpaceStep1Args(t *testing.T) {
	file := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	uploadPayload := `{"resourceUrl":"https://upload.test/obj","uploadKey":"k1"}`

	t.Run("folder mode includes name and fileSize", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{
			{text: uploadPayload},
			{text: `{"created":true}`},
		}}
		old := httpPutFile
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
		t.Cleanup(func() { httpPutFile = old })
		if err := executeDriveEdge(t, caller, "upload", "--file", file, "--workspace", "ws-1", "--folder", "f1"); err != nil {
			t.Fatal(err)
		}
		if len(caller.argsLog) < 2 {
			t.Fatalf("expected 2 calls, got %d", len(caller.argsLog))
		}
		wantStep1 := map[string]any{
			"name":        "note.md",
			"fileSize":    float64(5),
			"workspaceId": "ws-1",
			"folderId":    "f1",
		}
		if !reflect.DeepEqual(caller.argsLog[0], wantStep1) {
			t.Fatalf("folder mode step1 args = %#v, want %#v", caller.argsLog[0], wantStep1)
		}
	})

	t.Run("overwrite mode includes name and fileSize", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{
			{text: uploadPayload},
			{text: `{"updated":true}`},
		}}
		old := httpPutFile
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
		t.Cleanup(func() { httpPutFile = old })
		if err := executeDriveEdge(t, caller, "upload", "--file", file, "--workspace", "ws-1", "--node", "n1", "--yes"); err != nil {
			t.Fatal(err)
		}
		if len(caller.argsLog) < 2 {
			t.Fatalf("expected 2 calls, got %d", len(caller.argsLog))
		}
		wantStep1 := map[string]any{
			"name":            "note.md",
			"fileSize":        float64(5),
			"workspaceId":     "ws-1",
			"overwriteNodeId": "n1",
		}
		if !reflect.DeepEqual(caller.argsLog[0], wantStep1) {
			t.Fatalf("overwrite mode step1 args = %#v, want %#v", caller.argsLog[0], wantStep1)
		}
	})
}

// ---------------------------------------------------------------------------
// Upload dry-run delegation-precheck coverage — merged from the former
// drive_upload_delegation_dry_run_test.go (kept here per single-file rule).
// ---------------------------------------------------------------------------

// The upload leaf commands (drive upload, drive upload --workspace, doc upload)
// fast-return a dry-run preview before reaching deps.Caller.CallTool, so their
// dry-run branches used to bypass the delegation-auth decorator — diverging
// from the real execution which gates every business call through
// check_capability. These tests verify markdownDryRunDelegationPrecheck now
// gates every upload dry-run preview on check_capability when
// --principal-user-id is set, targets the command's real first delegated call
// carrying uploadActionParam{fileName,fileSize}, and stays a no-op otherwise.

// runUploadDelegation installs a raw dry-run caller then executes the given
// product group (drive/doc) through cobra so the group's PersistentPreRunE
// wraps deps.Caller in the delegation-auth decorator exactly as the real CLI.
func runUploadDelegation(t *testing.T, inner *docDelegationHelpersTestCaller, product *cobra.Command, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	out, _ := installHelpersCoreDeps(t, inner)
	err := executeMarkdownDriveCommand(t, product, nil, args...)
	return out, err
}

// assertUploadDelegationCheck verifies check_capability fired once through the
// dry-run read channel with the expected mcpToolKey and nodeId, carried the
// uploadActionParam{fileName,fileSize}, and that no business CallTool
// passthrough happened during dry-run.
func assertUploadDelegationCheck(t *testing.T, inner *docDelegationHelpersTestCaller, wantToolKey, wantNodeID, wantFileName string, wantFileSize int64) {
	t.Helper()
	if len(inner.readCalls) != 1 {
		t.Fatalf("readCalls = %d, want 1 check_capability via ReadTool", len(inner.readCalls))
	}
	rc := inner.readCalls[0]
	if rc.server != capabilityServerID || rc.tool != checkCapTool {
		t.Fatalf("check routed to %s.%s, want %s.%s", rc.server, rc.tool, capabilityServerID, checkCapTool)
	}
	if rc.args["mcpToolKey"] != wantToolKey {
		t.Fatalf("mcpToolKey = %v, want %s", rc.args["mcpToolKey"], wantToolKey)
	}
	if rc.args["nodeId"] != wantNodeID {
		t.Fatalf("nodeId = %v, want %s", rc.args["nodeId"], wantNodeID)
	}
	options, ok := rc.args["options"].(map[string]any)
	if !ok {
		t.Fatalf("options = %#v, want map with uploadActionParam", rc.args["options"])
	}
	up, ok := options["uploadActionParam"].(map[string]any)
	if !ok {
		t.Fatalf("uploadActionParam = %#v, want map", options["uploadActionParam"])
	}
	if up["fileName"] != wantFileName {
		t.Fatalf("uploadActionParam.fileName = %v, want %s", up["fileName"], wantFileName)
	}
	if up["fileSize"] != wantFileSize {
		t.Fatalf("uploadActionParam.fileSize = %v (%T), want %d", up["fileSize"], up["fileSize"], wantFileSize)
	}
	if len(inner.calls) != 0 {
		t.Fatalf("calls = %d, want 0 (dry-run must not passthrough to CallTool)", len(inner.calls))
	}
}

func newUploadDelegationInner(readResult string) *docDelegationHelpersTestCaller {
	return &docDelegationHelpersTestCaller{
		checkRes: textToolResult(readResult),
		readRes:  textToolResult(readResult),
	}
}

// TestCrossPlatformCoverageUploadDryRunDelegationAllowed covers both upload
// paths: drive.get_upload_info and doc.get_file_upload_info (via drive
// --workspace and via the doc upload compat command). Each should render the
// preview and fire check_capability carrying uploadActionParam.
func TestCrossPlatformCoverageUploadDryRunDelegationAllowed(t *testing.T) {
	const content = "hello" // 5 bytes → fileSize int64(5)
	file := writeMarkdownDriveFixture(t, "upload_fixture.md", content)

	cases := []struct {
		name        string
		product     func() *cobra.Command
		args        []string
		toolKey     string
		wantNode    string
		wantPreview string
	}{
		{
			name:        "drive upload to folder probes drive.get_upload_info",
			product:     newDriveCommand,
			args:        []string{"drive", "upload", "--file", file, "--folder", "f1", "--principal-user-id", "u1"},
			toolKey:     "drive.get_upload_info",
			wantNode:    "f1",
			wantPreview: "dry_run",
		},
		{
			name:        "drive upload --workspace probes doc.get_file_upload_info",
			product:     newDriveCommand,
			args:        []string{"drive", "upload", "--file", file, "--workspace", "w1", "--folder", "f1", "--principal-user-id", "u1"},
			toolKey:     "doc.get_file_upload_info",
			wantNode:    "f1",
			wantPreview: "dry_run",
		},
		{
			name:        "doc upload compat command probes doc.get_file_upload_info",
			product:     newDocCommand,
			args:        []string{"doc", "upload", "--file", file, "--folder", "f1", "--principal-user-id", "u1"},
			toolKey:     "doc.get_file_upload_info",
			wantNode:    "f1",
			wantPreview: "上传文件到钉钉文档",
		},
		{
			name:        "drive upload with --mime-type still probes drive.get_upload_info",
			product:     newDriveCommand,
			args:        []string{"drive", "upload", "--file", file, "--folder", "f1", "--mime-type", "application/pdf", "--principal-user-id", "u1"},
			toolKey:     "drive.get_upload_info",
			wantNode:    "f1",
			wantPreview: "dry_run",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner := newUploadDelegationInner(markdownDelegationAllowed)
			out, err := runUploadDelegation(t, inner, tc.product(), tc.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			assertUploadDelegationCheck(t, inner, tc.toolKey, tc.wantNode, "upload_fixture.md", int64(len(content)))
			if !strings.Contains(out.String(), tc.wantPreview) {
				t.Fatalf("output = %q, want preview containing %q", out.String(), tc.wantPreview)
			}
		})
	}
}

// TestCrossPlatformCoverageUploadDryRunDelegationDenied verifies a denied
// principal suppresses the preview and surfaces the DELEGATION_AUTH_DENIED
// error (exit code 1) for both upload paths.
func TestCrossPlatformCoverageUploadDryRunDelegationDenied(t *testing.T) {
	file := writeMarkdownDriveFixture(t, "upload_fixture.md", "hello")

	cases := []struct {
		name        string
		product     func() *cobra.Command
		args        []string
		wantPreview string
	}{
		{
			name:        "drive upload denied",
			product:     newDriveCommand,
			args:        []string{"drive", "upload", "--file", file, "--folder", "f1", "--principal-user-id", "u1"},
			wantPreview: "dry_run",
		},
		{
			name:        "doc upload denied",
			product:     newDocCommand,
			args:        []string{"doc", "upload", "--file", file, "--folder", "f1", "--principal-user-id", "u1"},
			wantPreview: "上传文件到钉钉文档",
		},
		{
			name:        "drive upload --workspace denied",
			product:     newDriveCommand,
			args:        []string{"drive", "upload", "--file", file, "--workspace", "w1", "--folder", "f1", "--principal-user-id", "u1"},
			wantPreview: "上传文件到文档空间",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner := newUploadDelegationInner(markdownDelegationDenied)
			out, err := runUploadDelegation(t, inner, tc.product(), tc.args...)
			if err == nil || !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_DENIED]") {
				t.Fatalf("error = %v, want [DELEGATION_AUTH_DENIED] prefix", err)
			}
			if strings.Contains(out.String(), tc.wantPreview) {
				t.Fatalf("output = %q, must not render preview on denial", out.String())
			}
		})
	}
}

// TestCrossPlatformCoverageUploadDryRunNoPrincipal verifies that without
// --principal-user-id the caller is never decorated, so no check_capability
// fires and the preview renders normally for both upload paths.
func TestCrossPlatformCoverageUploadDryRunNoPrincipal(t *testing.T) {
	file := writeMarkdownDriveFixture(t, "upload_fixture.md", "hello")

	cases := []struct {
		name        string
		product     func() *cobra.Command
		args        []string
		wantPreview string
	}{
		{
			name:        "drive upload without principal",
			product:     newDriveCommand,
			args:        []string{"drive", "upload", "--file", file, "--folder", "f1"},
			wantPreview: "dry_run",
		},
		{
			name:        "doc upload without principal",
			product:     newDocCommand,
			args:        []string{"doc", "upload", "--file", file, "--folder", "f1"},
			wantPreview: "上传文件到钉钉文档",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner := newUploadDelegationInner(markdownDelegationAllowed)
			out, err := runUploadDelegation(t, inner, tc.product(), tc.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if len(inner.readCalls) != 0 || len(inner.calls) != 0 {
				t.Fatalf("readCalls=%d calls=%d, want 0/0 (no delegation without principal)", len(inner.readCalls), len(inner.calls))
			}
			if !strings.Contains(out.String(), tc.wantPreview) {
				t.Fatalf("output = %q, want preview containing %q", out.String(), tc.wantPreview)
			}
		})
	}
}
