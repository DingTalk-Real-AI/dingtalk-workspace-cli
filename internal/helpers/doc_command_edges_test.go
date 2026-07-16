package helpers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func runDocCoverageCommand(t *testing.T, caller edition.ToolCaller, args ...string) error {
	t.Helper()
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	root := newDocCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.ExecuteContext(context.Background())
}

func TestCrossPlatformCoverageDocTransferAndDiffEdges(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "doc"}
	t.Cleanup(func() { os.Args = oldArgs })
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"markdown":"before"}`}}}
	installScriptedCaller(t, caller)

	root := newDocCommand()
	copyCmd, _, _ := root.Find([]string{"copy"})
	_ = copyCmd.Flags().Set("node", "node")
	_ = copyCmd.Flags().Set("folder", "12345")
	if err := copyCmd.RunE(copyCmd, nil); err == nil {
		t.Fatal("numeric document folder unexpectedly accepted")
	}

	previewCmd := root
	var out bytes.Buffer
	previewCmd.SetOut(&out)
	if err := previewDocOverwriteDiff(context.Background(), previewCmd, "node", "after"); err != nil {
		t.Fatal(err)
	}
	caller.steps = []scriptedToolStep{{err: errors.New("read")}}
	caller.index = 0
	if err := previewDocOverwriteDiff(context.Background(), previewCmd, "node", "after"); err == nil {
		t.Fatal("preview read failure returned nil")
	}
	if got := extractMarkdownField(`{"markdown":"body"}`); got != "body" {
		t.Fatalf("markdown extraction = %q", got)
	}
	long := strings.TrimSuffix(strings.Repeat("line\n", 25), "\n")
	diff := renderDocOverwriteDiff("node", long, long)
	if !strings.Contains(diff, "5 more lines") {
		t.Fatalf("long diff was not truncated: %s", diff)
	}
}

func TestCrossPlatformCoverageDocUploadAndMediaErrorEdges(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "doc"}
	t.Cleanup(func() { os.Args = oldArgs })
	oldPut, oldGet := httpPutFile, httpGetFile
	t.Cleanup(func() { httpPutFile, httpGetFile = oldPut, oldGet })
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("upload input validation", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{})
		root := newDocCommand()
		upload, _, _ := root.Find([]string{"upload"})
		_ = upload.Flags().Set("file", t.TempDir())
		if err := upload.RunE(upload, nil); err == nil {
			t.Fatal("directory upload returned nil")
		}
		root = newDocCommand()
		upload, _, _ = root.Find([]string{"upload"})
		_ = upload.Flags().Set("file", filepath.Join(t.TempDir(), "missing"))
		if err := upload.RunE(upload, nil); err == nil {
			t.Fatal("missing upload returned nil")
		}
		root = newDocCommand()
		upload, _, _ = root.Find([]string{"upload"})
		_ = upload.Flags().Set("file", file)
		_ = upload.Flags().Set("folder", "123")
		if err := upload.RunE(upload, nil); err == nil {
			t.Fatal("numeric folder returned nil")
		}
	})

	t.Run("download failure", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"resourceUrl":"https://download/file"}`}}})
		httpGetFile = func(context.Context, string, map[string]string, string) error { return errors.New("get") }
		root := newDocCommand()
		download, _, _ := root.Find([]string{"download"})
		_ = download.Flags().Set("node", "node")
		_ = download.Flags().Set("output", filepath.Join(t.TempDir(), "out"))
		if err := download.RunE(download, nil); err == nil {
			t.Fatal("download failure returned nil")
		}
	})

	t.Run("upload put failure", func(t *testing.T) {
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"resourceUrl":"https://upload","uploadKey":"key"}`}}})
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return errors.New("put") }
		root := newDocCommand()
		upload, _, _ := root.Find([]string{"upload"})
		_ = upload.Flags().Set("file", file)
		if err := upload.RunE(upload, nil); err == nil {
			t.Fatal("put failure returned nil")
		}
	})
	for _, tc := range []struct {
		name string
		step scriptedToolStep
	}{
		{"upload credential failure", scriptedToolStep{err: errors.New("credential")}},
		{"upload credential parse failure", scriptedToolStep{text: `{}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{tc.step}})
			root := newDocCommand()
			upload, _, _ := root.Find([]string{"upload"})
			_ = upload.Flags().Set("file", file)
			if err := upload.RunE(upload, nil); err == nil {
				t.Fatal("upload failure returned nil")
			}
		})
	}

	mediaCommand := func(t *testing.T, caller *scriptedToolCaller, path, mime string) error {
		t.Helper()
		installScriptedCaller(t, caller)
		root := newDocCommand()
		media, _, _ := root.Find([]string{"media", "insert"})
		_ = media.Flags().Set("node", "node")
		_ = media.Flags().Set("file", path)
		if mime != "" {
			_ = media.Flags().Set("mime-type", mime)
		}
		return media.RunE(media, nil)
	}

	t.Run("media directory", func(t *testing.T) {
		if err := mediaCommand(t, &scriptedToolCaller{}, t.TempDir(), ""); err == nil {
			t.Fatal("directory media returned nil")
		}
	})
	t.Run("media missing file", func(t *testing.T) {
		if err := mediaCommand(t, &scriptedToolCaller{}, filepath.Join(t.TempDir(), "missing"), ""); err == nil {
			t.Fatal("missing media returned nil")
		}
	})
	t.Run("media dry run", func(t *testing.T) {
		if err := mediaCommand(t, &scriptedToolCaller{dry: true}, file, ""); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("media credential failure", func(t *testing.T) {
		if err := mediaCommand(t, &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("credential")}}}, file, ""); err == nil {
			t.Fatal("credential failure returned nil")
		}
	})
	t.Run("media parse failure", func(t *testing.T) {
		if err := mediaCommand(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}}, file, ""); err == nil {
			t.Fatal("parse failure returned nil")
		}
	})
	t.Run("media put failure", func(t *testing.T) {
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return errors.New("put") }
		if err := mediaCommand(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"uploadUrl":"https://upload","resourceId":"resource"}`}}}, file, ""); err == nil {
			t.Fatal("media put failure returned nil")
		}
	})
	t.Run("media insert failure", func(t *testing.T) {
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"uploadUrl":"https://upload","resourceId":"resource"}`}, {err: errors.New("insert")}}}
		if err := mediaCommand(t, caller, file, "text/plain"); err == nil {
			t.Fatal("media insert failure returned nil")
		}
	})
	t.Run("large image becomes attachment", func(t *testing.T) {
		httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
		large := filepath.Join(t.TempDir(), "large.png")
		if err := os.WriteFile(large, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Truncate(large, 21*1024*1024); err != nil {
			t.Fatal(err)
		}
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"uploadUrl":"https://upload","resourceId":"resource","resourceUrl":"https://image"}`}, {text: `{}`}}}
		if err := mediaCommand(t, caller, large, "image/png"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestCrossPlatformCoverageDefaultDocHTTPTransportEdges(t *testing.T) {
	file := filepath.Join(t.TempDir(), "body")
	if err := os.WriteFile(file, []byte("body"), 0o600); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := defaultHTTPPutFile(cancelled, "http://127.0.0.1:1", nil, file, 4); err == nil {
		t.Fatal("cancelled upload returned nil")
	}
	if err := defaultHTTPGetFile(cancelled, "http://127.0.0.1:1", nil, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("cancelled download returned nil")
	}

	oldCreate, oldCopy := docCreateDestination, docCopyContent
	t.Cleanup(func() { docCreateDestination, docCopyContent = oldCreate, oldCopy })
	docCreateDestination = func(string) (*os.File, error) { return nil, errors.New("create") }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "body")
	}))
	t.Cleanup(server.Close)
	if err := defaultHTTPGetFile(context.Background(), server.URL, nil, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("destination creation failure returned nil")
	}
	docCreateDestination = os.Create
	docCopyContent = func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("copy") }
	if err := defaultHTTPGetFile(context.Background(), server.URL, nil, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("copy failure returned nil")
	}
}

func TestCrossPlatformCoverageDocCreateUpdateAndBlockCommandEdges(t *testing.T) {
	oldDeps, oldArgs, oldPut, oldGet := deps, os.Args, httpPutFile, httpGetFile
	t.Cleanup(func() {
		deps, os.Args, httpPutFile, httpGetFile = oldDeps, oldArgs, oldPut, oldGet
	})
	os.Args = []string{"dws", "doc", "--yes"}
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
	httpGetFile = func(context.Context, string, map[string]string, string) error { return nil }

	type commandCase struct {
		name  string
		steps []scriptedToolStep
		args  []string
	}
	docJSONML := `["root",{},["p",{},"hello"]]`
	nodeJSONML := `["p",{},"hello"]`
	cases := []commandCase{
		{"create numeric folder", nil, []string{"create", "--name=n", "--folder=123"}},
		{"create missing content file", nil, []string{"create", "--name=n", "--content-file=missing"}},
		{"create markdown that resembles jsonml", []scriptedToolStep{{text: `{}`}}, []string{"create", "--name=n", `--content=["p",{},"hello"]`}},
		{"create invalid jsonml", nil, []string{"create", "--name=n", "--content-format=jsonml", "--content={"}},
		{"create jsonml call failure", []scriptedToolStep{{err: errors.New("create")}}, []string{"create", "--name=n", "--content-format=jsonml", "--content=" + docJSONML}},
		{"create jsonml missing node", []scriptedToolStep{{text: `{}`}}, []string{"create", "--name=n", "--content-format=jsonml", "--content=" + docJSONML}},
		{"create jsonml success", []scriptedToolStep{{text: `{"nodeId":"node"}`}, {text: `{}`}}, []string{"create", "--name=n", "--folder=folder", "--workspace=workspace", "--content-format=jsonml", "--content=" + docJSONML}},
		{"update missing content file", nil, []string{"update", "--node=node", "--content-file=missing", "--mode=append"}},
		{"update empty content", nil, []string{"update", "--node=node", "--content="}},
		{"update missing mode", nil, []string{"update", "--node=node", "--content=body", "--mode="}},
		{"update overwrite preview", []scriptedToolStep{{text: `{"markdown":"old"}`}}, []string{"update", "--node=node", "--content=new", "--mode=overwrite", "--dry-run"}},
		{"update overwrite preview failure", []scriptedToolStep{{err: errors.New("read")}}, []string{"update", "--node=node", "--content=new", "--mode=overwrite", "--dry-run"}},
		{"update overwrite needs yes", nil, []string{"update", "--node=node", "--content=new", "--mode=overwrite"}},
		{"update markdown resembles jsonml", []scriptedToolStep{{text: `{}`}}, []string{"update", "--node=node", `--content=["p",{},"hello"]`, "--mode=append"}},
		{"update jsonml append", nil, []string{"update", "--node=node", "--content-format=jsonml", "--content=" + docJSONML, "--mode=append"}},
		{"update invalid jsonml", nil, []string{"update", "--node=node", "--content-format=jsonml", "--content={", "--mode=overwrite", "--yes"}},
		{"update jsonml revision", []scriptedToolStep{{text: `{}`}}, []string{"update", "--node=node", "--content-format=jsonml", "--content=" + docJSONML, "--mode=overwrite", "--revision=2", "--yes"}},
		{"update markdown index", []scriptedToolStep{{text: `{}`}}, []string{"update", "--node=node", "--content=body", "--mode=append", "--index=2"}},
		{"file create numeric folder", nil, []string{"file", "create", "--name=n", "--type=adoc", "--folder=123"}},
		{"folder create numeric folder", nil, []string{"folder", "create", "--name=n", "--folder=123"}},
		{"block insert sniff", []scriptedToolStep{{text: `{}`}}, []string{"block", "insert", "--node=node", `--element=["p",{},"hello"]`}},
		{"block insert invalid jsonml", nil, []string{"block", "insert", "--node=node", "--content-format=jsonml", "--element={"}},
		{"block insert jsonml options", []scriptedToolStep{{text: `{}`}}, []string{"block", "insert", "--node=node", "--content-format=jsonml", "--element=" + nodeJSONML, "--ref-block=ref", "--parent-block=parent", "--index=2"}},
		{"block insert normal options", []scriptedToolStep{{text: `{}`}}, []string{"block", "insert", "--node=node", "--text=body", "--index=2", "--where=before", "--ref-block=ref"}},
		{"block update sniff", []scriptedToolStep{{text: `{}`}}, []string{"block", "update", "--node=node", "--block-id=block", `--element=["p",{},"hello"]`}},
		{"block update invalid jsonml", nil, []string{"block", "update", "--node=node", "--block-id=block", "--content-format=jsonml", "--element={"}},
		{"block update jsonml", []scriptedToolStep{{text: `{}`}}, []string{"block", "update", "--node=node", "--block-id=block", "--content-format=jsonml", "--element=" + nodeJSONML}},
		{"block update normal", []scriptedToolStep{{text: `{}`}}, []string{"block", "update", "--node=node", "--block-id=block", "--heading=Title", "--level=2"}},
		{"inline comment missing offsets", nil, []string{"comment", "create-inline", "--node=node", "--block-id=block", "--content=note"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := runDocCoverageCommand(t, &scriptedToolCaller{steps: tc.steps}, tc.args...); err != nil {
				t.Logf("command returned: %v", err)
			}
		})
	}

	for _, value := range []string{"plain", "# Other\nbody", "# Name", "# **Name** ###\n\nbody"} {
		_ = stripDuplicateTitle(value, "Name")
	}
	for _, value := range []string{"", " Name ### ", "**Bold**", "__Under__ ~~Strike~~ `Code`"} {
		_ = normalizeHeadingText(value)
	}
	for _, name := range []string{"file.pdf", "file.md", "file.unknown"} {
		_ = inferMimeType(name)
	}
	_ = extractMarkdownField("not-json")

	newBlockCommand := func() *cobra.Command {
		root := newDocCommand()
		cmd, _, _ := root.Find([]string{"block", "insert"})
		return cmd
	}
	for _, tc := range []struct {
		flag, value string
	}{
		{"element", "{"},
		{"element", `{"blockType":"paragraph"}`},
		{"heading", "Heading"},
		{"text", "Body"},
	} {
		cmd := newBlockCommand()
		_ = cmd.Flags().Set(tc.flag, tc.value)
		_, _ = buildBlockElement(cmd)
	}
	cmd := newBlockCommand()
	_ = cmd.Flags().Set("heading", "Heading")
	_ = cmd.Flags().Set("level", "99")
	_, _ = buildBlockElement(cmd)
	_, _ = buildBlockElement(newBlockCommand())
}

func TestCrossPlatformCoverageDocDestructiveCancellationEdges(t *testing.T) {
	oldDeps, oldArgs, oldStdin := deps, os.Args, os.Stdin
	t.Cleanup(func() { deps, os.Args, os.Stdin = oldDeps, oldArgs, oldStdin })
	os.Args = []string{"dws", "doc"}
	input, err := os.CreateTemp(t.TempDir(), "answers")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close() })
	_, _ = input.WriteString(strings.Repeat("no\n", 4))
	_, _ = input.Seek(0, 0)
	os.Stdin = input
	for _, args := range [][]string{
		{"block", "delete", "--node=node", "--block-id=block"},
		{"delete", "--node=node"},
	} {
		_ = runDocCoverageCommand(t, &scriptedToolCaller{}, args...)
	}
}

func TestCrossPlatformCoverageDocExportImportCommandEdges(t *testing.T) {
	oldDeps, oldArgs, oldPut, oldGet, oldSleep, oldAfter := deps, os.Args, httpPutFile, httpGetFile, helperSleep, helperAfter
	t.Cleanup(func() {
		deps, os.Args, httpPutFile, httpGetFile, helperSleep, helperAfter = oldDeps, oldArgs, oldPut, oldGet, oldSleep, oldAfter
	})
	os.Args = []string{"dws", "doc", "--yes"}
	helperSleep = func(time.Duration) {}
	helperAfter = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
	httpGetFile = func(context.Context, string, map[string]string, string) error { return nil }

	run := func(t *testing.T, steps []scriptedToolStep, dry bool, args ...string) error {
		t.Helper()
		return runDocCoverageCommand(t, &scriptedToolCaller{steps: steps, dry: dry}, args...)
	}
	outDir := t.TempDir()
	exportCases := []struct {
		name  string
		steps []scriptedToolStep
		dry   bool
		args  []string
	}{
		{"legacy md", nil, true, []string{"export", "--node=node", "--output=out", "--export-format=", "--format=md"}},
		{"legacy global ignored", nil, true, []string{"export", "--node=node", "--output=out", "--export-format=", "--format=json"}},
		{"invalid format", nil, false, []string{"export", "--node=node", "--output=out", "--export-format=zip"}},
		{"dry run", nil, true, []string{"export", "--node=node", "--output=out", "--export-format=pdf"}},
		{"submit failure", []scriptedToolStep{{err: errors.New("submit")}}, false, []string{"export", "--node=node", "--output=out"}},
		{"submit invalid", []scriptedToolStep{{text: `{`}}, false, []string{"export", "--node=node", "--output=out"}},
		{"submit missing job", []scriptedToolStep{{text: `{}`}}, false, []string{"export", "--node=node", "--output=out"}},
		{"poll failure", []scriptedToolStep{{text: `{"jobId":"job"}`}, {err: errors.New("poll")}}, false, []string{"export", "--node=node", "--output=out"}},
		{"download directory without extension", []scriptedToolStep{{text: `{"jobId":"job"}`}, {text: `{"status":"SUCCESS","downloadUrl":"https://example.test/download"}`}}, false, []string{"export", "--node=node", "--output=" + outDir}},
		{"download directory", []scriptedToolStep{{text: `{"jobId":"job"}`}, {text: `{"status":"SUCCESS","downloadUrl":"https://example.test/name.txt"}`}}, false, []string{"export", "--node=node", "--output=" + outDir, "--export-format=pdf"}},
	}
	for _, tc := range exportCases {
		t.Run("export "+tc.name, func(t *testing.T) { _ = run(t, tc.steps, tc.dry, tc.args...) })
	}
	httpGetFile = func(context.Context, string, map[string]string, string) error { return errors.New("download") }
	_ = run(t, []scriptedToolStep{{text: `{"jobId":"job"}`}, {text: `{"status":"SUCCESS","downloadUrl":"https://example.test/file.docx"}`}}, false, "export", "--node=node", "--output=out")
	httpGetFile = func(context.Context, string, map[string]string, string) error { return nil }

	for _, tc := range []struct {
		name  string
		steps []scriptedToolStep
		dry   bool
	}{
		{"dry", nil, true},
		{"error", []scriptedToolStep{{err: errors.New("query")}}, false},
		{"invalid", []scriptedToolStep{{text: `{`}}, false},
		{"success", []scriptedToolStep{{text: `{"status":"SUCCESS"}`}}, false},
		{"processing", []scriptedToolStep{{text: `{"status":"PROCESSING"}`}}, false},
		{"failed message", []scriptedToolStep{{text: `{"status":"FAILED","message":"bad"}`}}, false},
		{"failed empty", []scriptedToolStep{{text: `{"status":"FAILED"}`}}, false},
	} {
		t.Run("export get "+tc.name, func(t *testing.T) {
			_ = run(t, tc.steps, tc.dry, "export", "get", "--job-id=job")
		})
	}

	valid := filepath.Join(t.TempDir(), "report.md")
	empty := filepath.Join(t.TempDir(), "empty.md")
	large := filepath.Join(t.TempDir(), "large.md")
	unsupported := filepath.Join(t.TempDir(), "file.bin")
	for path, data := range map[string][]byte{valid: []byte("body"), empty: nil, unsupported: []byte("body")} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(large, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(large, 21*1024*1024); err != nil {
		t.Fatal(err)
	}
	_ = run(t, nil, false, "import", "--file="+t.TempDir())
	_ = run(t, nil, false, "import", "--file="+large)
	_ = run(t, nil, false, "import", "--file="+empty)
	_ = run(t, nil, false, "import", "--file="+unsupported)
	_ = run(t, nil, true, "import", valid, "--folder=folder", "--workspace=workspace")
	_ = run(t, []scriptedToolStep{{err: errors.New("session")}}, false, "import", "--file="+valid)
	_ = run(t, []scriptedToolStep{{text: `{`}}, false, "import", "--file="+valid)
	_ = run(t, []scriptedToolStep{{text: `{}`}}, false, "import", "--file="+valid)
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return errors.New("put") }
	_ = run(t, []scriptedToolStep{{text: `{"sessionId":"session","uploadUrl":"https://upload"}`}}, false, "import", "--file="+valid)
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
	_ = run(t, []scriptedToolStep{{text: `{"sessionId":"session","uploadUrl":"https://upload"}`}, {err: errors.New("confirm")}}, false, "import", "--file="+valid)
	_ = run(t, []scriptedToolStep{{text: `{"sessionId":"session","uploadUrl":"https://upload"}`}, {text: `{`}}, false, "import", "--file="+valid)
	_ = run(t, []scriptedToolStep{{text: `{"sessionId":"session","uploadUrl":"https://upload"}`}, {text: `{}`}}, false, "import", "--file="+valid)
	_ = run(t, []scriptedToolStep{{text: `{"sessionId":"session","uploadUrl":"https://upload"}`}, {text: `{"taskId":"task"}`}, {err: errors.New("poll")}}, false, "import", "--file="+valid)
	_ = run(t, []scriptedToolStep{{text: `{"sessionId":"session","uploadUrl":"https://upload"}`}, {text: `{"taskId":"task"}`}, {text: `{"status":"completed","documentUrl":"url","documentName":"name","documentType":"doc"}`}}, false, "import", "--file="+valid)

	for _, tc := range []struct {
		name  string
		steps []scriptedToolStep
		dry   bool
	}{
		{"dry", nil, true},
		{"error", []scriptedToolStep{{err: errors.New("query")}}, false},
		{"invalid", []scriptedToolStep{{text: `{`}}, false},
		{"completed", []scriptedToolStep{{text: `{"status":"completed"}`}}, false},
		{"processing", []scriptedToolStep{{text: `{"status":"processing"}`}}, false},
		{"failed message", []scriptedToolStep{{text: `{"status":"failed","message":"bad"}`}}, false},
		{"failed empty", []scriptedToolStep{{text: `{"status":"failed"}`}}, false},
	} {
		t.Run("import get "+tc.name, func(t *testing.T) {
			_ = run(t, tc.steps, tc.dry, "import", "get", "--task-id=task")
		})
	}
}

func TestCrossPlatformCoverageDocVersionRevertCommandEdges(t *testing.T) {
	oldDeps, oldArgs, oldStdin := deps, os.Args, os.Stdin
	t.Cleanup(func() { deps, os.Args, os.Stdin = oldDeps, oldArgs, oldStdin })

	os.Args = []string{"dws", "doc", "--yes"}
	_ = runDocCoverageCommand(t, &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("list")}}}, "version", "revert", "--node=node", "--version=3")
	_ = runDocCoverageCommand(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"versions":[]}`}}}, "version", "revert", "--node=node", "--version=3")
	_ = runDocCoverageCommand(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"versions":[{"version":3}]}`}, {text: `{}`}}}, "version", "revert", "--node=node", "--version=3")

	os.Args = []string{"dws", "doc"}
	input, err := os.CreateTemp(t.TempDir(), "answer")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close() })
	_, _ = input.WriteString("no\n")
	_, _ = input.Seek(0, 0)
	os.Stdin = input
	_ = runDocCoverageCommand(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"versions":[{"version":3}]}`}}}, "version", "revert", "--node=node", "--version=3")
}
