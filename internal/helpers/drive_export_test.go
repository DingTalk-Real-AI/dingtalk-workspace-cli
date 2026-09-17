package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// ── inferExportFilename：URL 推断本地文件名 ──

func TestCrossPlatformCoverageInferExportFilename(t *testing.T) {
	cases := []struct {
		name     string
		rawURL   string
		fallback string
		want     string
	}{
		{"query string stripped", "https://x.test/files/report.docx?sig=1", "fallback.docx", "report.docx"},
		{"percent-decoded path", "https://x.test/files/a%2Fb.xlsx", "fallback.xlsx", "b.xlsx"},
		{"backslash converted", `https://x.test/files\a\report.pdf`, "fallback.pdf", "report.pdf"},
		{"empty url uses fallback", "", "fallback.docx", "fallback.docx"},
		{"root only url uses fallback", "https://x.test/", "fallback.docx", "fallback.docx"},
		{"invalid escape keeps raw name", "https://x.test/%", "fallback.docx", "%"},
	}
	for _, tc := range cases {
		if got := inferExportFilename(tc.rawURL, tc.fallback); got != tc.want {
			t.Errorf("%s: inferExportFilename(%q) = %q, want %q", tc.name, tc.rawURL, got, tc.want)
		}
	}
}

// ── resolveDriveExportOutputPath：目录/文件路径与扩展名对齐 ──

func TestCrossPlatformCoverageResolveDriveExportOutputPath(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name        string
		outputPath  string
		downloadURL string
		fileName    string
		fileExt     string
		jobID       string
		want        string
	}{
		{
			name:       "non-directory path unchanged",
			outputPath: filepath.Join(dir, "custom.bin"),
			fileExt:    ".docx",
			want:       filepath.Join(dir, "custom.bin"),
		},
		{
			name:       "nonexistent path unchanged",
			outputPath: filepath.Join(dir, "missing", "target.docx"),
			fileExt:    ".docx",
			want:       filepath.Join(dir, "missing", "target.docx"),
		},
		{
			name:       "filename keeps matching extension",
			outputPath: dir, fileName: "report.docx", fileExt: ".docx",
			want: filepath.Join(dir, "report.docx"),
		},
		{
			name:       "mismatched extension realigned",
			outputPath: dir, fileName: "report.pdf", fileExt: ".docx",
			want: filepath.Join(dir, "report.docx"),
		},
		{
			name:       "missing extension appended",
			outputPath: dir, fileName: "report", fileExt: ".docx",
			want: filepath.Join(dir, "report.docx"),
		},
		{
			name:        "empty filename falls back to url",
			outputPath:  dir,
			downloadURL: "https://x.test/files/result.xlsx?sig=1",
			fileExt:     ".docx",
			want:        filepath.Join(dir, "result.docx"),
		},
		{
			name:       "unnamed filename falls back to url",
			outputPath: dir, fileName: "unnamed",
			downloadURL: "https://x.test/files/result.xlsx",
			fileExt:     ".docx",
			want:        filepath.Join(dir, "result.docx"),
		},
		{
			name:        "no filename and bare url use job id",
			outputPath:  dir,
			downloadURL: "https://x.test/",
			fileExt:     ".docx",
			jobID:       "job-42",
			want:        filepath.Join(dir, "drive-export-job-42.docx"),
		},
	}
	for _, tc := range cases {
		got := resolveDriveExportOutputPath(tc.outputPath, tc.downloadURL, tc.fileName, tc.fileExt, tc.jobID)
		if got != tc.want {
			t.Errorf("%s: resolveDriveExportOutputPath = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// ── inferExportFormatFromDocInfo：扩展名映射与 docx 兜底 ──

func TestCrossPlatformCoverageInferExportFormatFromDocInfo(t *testing.T) {
	cases := []struct {
		name  string
		steps []scriptedToolStep
		want  string
	}{
		{"call error falls back", []scriptedToolStep{{err: errors.New("boom")}}, "docx"},
		{"parse error falls back", []scriptedToolStep{{text: `{`}}, "docx"},
		{"flat adoc", []scriptedToolStep{{text: `{"extension":"adoc"}`}}, "docx"},
		{"flat axls", []scriptedToolStep{{text: `{"extension":"axls"}`}}, "xlsx"},
		{"flat appt", []scriptedToolStep{{text: `{"extension":"appt"}`}}, "pptx"},
		{"uppercase extension normalized", []scriptedToolStep{{text: `{"extension":" AXLS "}`}}, "xlsx"},
		{"unknown extension falls back", []scriptedToolStep{{text: `{"extension":"xyzw"}`}}, "docx"},
		{"wrapped extension", []scriptedToolStep{{text: `{"result":{"extension":"appt"}}`}}, "pptx"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installScriptedCaller(t, &scriptedToolCaller{steps: tc.steps})
			if got := inferExportFormatFromDocInfo(context.Background(), "node-1"); got != tc.want {
				t.Fatalf("format = %q, want %q", got, tc.want)
			}
		})
	}
}

// ── pollDriveExportJob：取消 / 各终态 / 查询失败重试 / 轮询上限 ──

func TestCrossPlatformCoveragePollDriveExportJobTerminalStates(t *testing.T) {
	installImmediateTiming(t)
	installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"status":"PROCESSING"}`}}})

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := pollDriveExportJob(cancelled, "job-1"); err == nil || !strings.Contains(err.Error(), "导出轮询被取消") {
		t.Fatalf("cancelled error = %v", err)
	}

	steps := []struct {
		name  string
		steps []scriptedToolStep
		check func(t *testing.T, url, name string, err error)
	}{
		{"success", []scriptedToolStep{{text: `{"status":"SUCCESS","resultUrl":"https://x.test/f.docx","resultName":"f.docx"}`}},
			func(t *testing.T, url, name string, err error) {
				if err != nil || url != "https://x.test/f.docx" || name != "f.docx" {
					t.Fatalf("url=%q name=%q err=%v", url, name, err)
				}
			}},
		// 网络类错误（NETWORK_TIMEOUT）属临时性故障：继续轮询直到成功。
		{"success after query failure", []scriptedToolStep{{err: &CLIError{Code: CodeNetworkTimeout, Message: "request timed out"}}, {text: `{"status":"SUCCESS","resultUrl":"https://x.test/f.docx"}`}},
			func(t *testing.T, url, name string, err error) {
				if err != nil || url != "https://x.test/f.docx" {
					t.Fatalf("url=%q err=%v", url, err)
				}
			}},
		// 响应解析失败（网关临时返回非 JSON 错误页）同样归入可重试。
		{"parse failure keeps polling", []scriptedToolStep{{text: `{`}, {text: `{"status":"SUCCESS","resultUrl":"https://x.test/f.docx"}`}},
			func(t *testing.T, url, name string, err error) {
				if err != nil || url != "https://x.test/f.docx" {
					t.Fatalf("url=%q err=%v", url, err)
				}
			}},
		{"success without url fails", []scriptedToolStep{{text: `{"status":"SUCCESS"}`}},
			func(t *testing.T, url, name string, err error) {
				if err == nil || !strings.Contains(err.Error(), "导出成功但 downloadUrl 为空") {
					t.Fatalf("error = %v", err)
				}
			}},
		{"failed with message", []scriptedToolStep{{text: `{"status":"FAILED","message":"export denied"}`}},
			func(t *testing.T, url, name string, err error) {
				if err == nil || err.Error() != "export denied" {
					t.Fatalf("error = %v", err)
				}
			}},
		{"failed without message", []scriptedToolStep{{text: `{"status":"FAILED"}`}},
			func(t *testing.T, url, name string, err error) {
				if err == nil || !strings.Contains(err.Error(), "导出任务失败 (taskId=job-1, status=FAILED)") {
					t.Fatalf("error = %v", err)
				}
			}},
		{"partial failed with message", []scriptedToolStep{{text: `{"status":"PARTIAL_FAILED","message":"partial"}`}},
			func(t *testing.T, url, name string, err error) {
				if err == nil || err.Error() != "partial" {
					t.Fatalf("error = %v", err)
				}
			}},
		{"partial failed without message", []scriptedToolStep{{text: `{"status":"PARTIAL_FAILED"}`}},
			func(t *testing.T, url, name string, err error) {
				if err == nil || !strings.Contains(err.Error(), "导出任务失败 (taskId=job-1, status=PARTIAL_FAILED)") {
					t.Fatalf("error = %v", err)
				}
			}},
		{"timeout", []scriptedToolStep{{text: `{"status":"TIMEOUT"}`}},
			func(t *testing.T, url, name string, err error) {
				if err == nil || !strings.Contains(err.Error(), "导出任务超时 (taskId=job-1)") {
					t.Fatalf("error = %v", err)
				}
			}},
		{"processing keeps polling", []scriptedToolStep{{text: `{"status":"PROCESSING"}`}, {text: `{"status":"SUCCESS","resultUrl":"https://x.test/f.docx"}`}},
			func(t *testing.T, url, name string, err error) {
				if err != nil || url != "https://x.test/f.docx" {
					t.Fatalf("url=%q err=%v", url, err)
				}
			}},
		{"poll cap still processing", []scriptedToolStep{{text: `{"status":"PENDING"}`}},
			func(t *testing.T, url, name string, err error) {
				if err == nil || !strings.Contains(err.Error(), "导出任务超时：已轮询 30 次仍在处理中") {
					t.Fatalf("error = %v", err)
				}
			}},
		// 30 次查询全部遭遇网络错误：超时兜底消息必须带上最后一次查询错误，
		// 而不是只报超时掩盖真实故障。
		{"poll cap retains last query error", []scriptedToolStep{{err: &CLIError{Code: CodeNetworkTimeout, Message: "request timed out"}}},
			func(t *testing.T, url, name string, err error) {
				if err == nil || !strings.Contains(err.Error(), "导出任务超时：已轮询 30 次仍在处理中") {
					t.Fatalf("error = %v", err)
				}
				if !strings.Contains(err.Error(), "最后一次查询错误") || !strings.Contains(err.Error(), "request timed out") {
					t.Fatalf("timeout error must retain the last query error: %v", err)
				}
			}},
	}
	for _, tc := range steps {
		t.Run(tc.name, func(t *testing.T) {
			installScriptedCaller(t, &scriptedToolCaller{steps: tc.steps})
			url, name, err := pollDriveExportJob(context.Background(), "job-1")
			tc.check(t, url, name, err)
		})
	}
}

// ── runDriveExport 命令级：格式解析 / dry-run / 三步流程 ──

// executeDriveExportFailingWriter 以永远写失败的 stdout writer 执行 drive 命令，
// 用于断言 PrintJSON 的写错误被上抛而非静默吞掉（对齐 drive pull/push/sync 契约）。
func executeDriveExportFailingWriter(t *testing.T, caller *scriptedToolCaller, args ...string) error {
	t.Helper()
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	InitDeps(caller)
	deps.Out.w = failingWriter{}
	deps.Out.errW = io.Discard
	root := newDriveCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.Execute()
}

func TestCrossPlatformCoverageRunDriveExportFlow(t *testing.T) {
	installImmediateTiming(t)

	submitOK := scriptedToolStep{text: `{"jobId":"job-9"}`}
	queryOK := scriptedToolStep{text: `{"status":"SUCCESS","resultUrl":"https://x.test/report.docx","resultName":"report.docx"}`}

	t.Run("missing node flag", func(t *testing.T) {
		if err := executeDriveEdge(t, &scriptedToolCaller{}, "export"); err == nil {
			t.Fatal("missing node returned nil")
		}
	})

	t.Run("unsupported format", func(t *testing.T) {
		err := executeDriveEdge(t, &scriptedToolCaller{}, "export", "--node", "n1", "--export-format", "txt")
		if err == nil || !strings.Contains(err.Error(), "不支持的导出格式") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("md alias normalizes to markdown", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{submitOK, queryOK}}
		if err := executeDriveEdge(t, caller, "export", "--node", "n1", "--export-format", "md"); err != nil {
			t.Fatal(err)
		}
		if caller.argsLog[0]["exportFormat"] != "markdown" {
			t.Fatalf("exportFormat = %v, want markdown", caller.argsLog[0]["exportFormat"])
		}
	})

	t.Run("legacy format flag as export format", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{submitOK, queryOK}}
		if err := executeDriveEdge(t, caller, "export", "--node", "n1", "--format", "pdf"); err != nil {
			t.Fatal(err)
		}
		if caller.argsLog[0]["exportFormat"] != "pdf" {
			t.Fatalf("exportFormat = %v, want pdf", caller.argsLog[0]["exportFormat"])
		}
	})

	t.Run("output format value ignored for detection", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"extension":"axls"}`}, // get_document_info auto-detect
			submitOK,
			queryOK,
		}}
		if err := executeDriveEdge(t, caller, "export", "--node", "n1", "--format", "json"); err != nil {
			t.Fatal(err)
		}
		if caller.toolLog[0] != "get_document_info" {
			t.Fatalf("first tool = %q, want get_document_info", caller.toolLog[0])
		}
		if caller.argsLog[1]["exportFormat"] != "xlsx" {
			t.Fatalf("exportFormat = %v, want xlsx", caller.argsLog[1]["exportFormat"])
		}
	})

	t.Run("dry run prints preview", func(t *testing.T) {
		for _, extra := range [][]string{{}, {"--output", "out.docx"}, {"--async"}, {"--output", "out.docx", "--async"}} {
			caller := &scriptedToolCaller{dry: true}
			args := append([]string{"export", "--node", "n1", "--export-format", "docx", "--dry-run"}, extra...)
			if err := executeDriveEdge(t, caller, args...); err != nil {
				t.Fatalf("dry-run %v: %v", extra, err)
			}
			if caller.calls != 0 {
				t.Fatalf("dry-run %v: tool calls = %d, want 0", extra, caller.calls)
			}
		}
	})

	t.Run("dry run without explicit format skips detection", func(t *testing.T) {
		// dry-run 早退必须先于格式自动探测：探测会真实调用远端
		// get_document_info，而 dry-run 不允许任何远端调用。
		caller := &scriptedToolCaller{dry: true}
		if err := executeDriveEdge(t, caller, "export", "--node", "n1", "--dry-run"); err != nil {
			t.Fatal(err)
		}
		if caller.calls != 0 || len(caller.argsLog) != 0 {
			t.Fatalf("dry-run made remote calls: calls=%d argsLog=%v", caller.calls, caller.argsLog)
		}
	})

	t.Run("dry run with unsupported explicit format fails fast", func(t *testing.T) {
		// 显式格式的合法性校验先于 dry-run 预览：非法输入在预览阶段同样报错。
		caller := &scriptedToolCaller{dry: true}
		err := executeDriveEdge(t, caller, "export", "--node", "n1", "--export-format", "txt", "--dry-run")
		if err == nil || !strings.Contains(err.Error(), "不支持的导出格式") {
			t.Fatalf("error = %v, want unsupported format", err)
		}
		if caller.calls != 0 {
			t.Fatalf("dry-run calls = %d, want 0", caller.calls)
		}
	})

	t.Run("submit error", func(t *testing.T) {
		err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("boom")}}},
			"export", "--node", "n1", "--export-format", "docx")
		if err == nil || !strings.Contains(err.Error(), "提交导出任务失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("submit response without job id", func(t *testing.T) {
		err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}},
			"export", "--node", "n1", "--export-format", "docx")
		if err == nil || !strings.Contains(err.Error(), "jobId") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("async mode returns task id immediately", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{submitOK}}
		if err := executeDriveEdge(t, caller, "export", "--node", "n1", "--export-format", "docx", "--async"); err != nil {
			t.Fatal(err)
		}
		if len(caller.toolLog) != 1 || caller.calls != 1 {
			t.Fatalf("calls = %v, want exactly the submit call", caller.toolLog)
		}
	})

	t.Run("poll failure surfaces", func(t *testing.T) {
		err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{submitOK, {text: `{"status":"FAILED","message":"denied"}`}}},
			"export", "--node", "n1", "--export-format", "docx")
		if err == nil || !strings.Contains(err.Error(), "denied") {
			t.Fatalf("error = %v", err)
		}
	})

	// 确定性业务错误（MCP_TOOL_ERROR）：立即上抛，不盲等轮询上限。
	t.Run("deterministic query error aborts polling", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{
			submitOK,
			{err: &CLIError{Code: CodeMCPToolError, Message: "task not found"}},
		}}
		err := executeDriveEdge(t, caller, "export", "--node", "n1", "--export-format", "docx")
		if err == nil || !strings.Contains(err.Error(), "查询导出任务失败") || !strings.Contains(err.Error(), "task not found") {
			t.Fatalf("error = %v, want 查询导出任务失败 wrapping task not found", err)
		}
		if caller.calls != 2 {
			t.Fatalf("tool calls = %d, want 2 (submit + single query, no blind retry)", caller.calls)
		}
	})

	// 鉴权类错误（AUTH_TOKEN_EXPIRED）：同样立即上抛。
	t.Run("auth query error aborts polling", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{
			submitOK,
			{err: &CLIError{Code: CodeAuthTokenExpired, Message: "Token 已过期或验证失败"}},
		}}
		err := executeDriveEdge(t, caller, "export", "--node", "n1", "--export-format", "docx")
		if err == nil || !strings.Contains(err.Error(), "查询导出任务失败") || !strings.Contains(err.Error(), "AUTH_TOKEN_EXPIRED") {
			t.Fatalf("error = %v, want 查询导出任务失败 wrapping AUTH_TOKEN_EXPIRED", err)
		}
		if caller.calls != 2 {
			t.Fatalf("tool calls = %d, want 2 (submit + single query, no blind retry)", caller.calls)
		}
	})

	t.Run("no output prints single json result", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{submitOK, queryOK}}
		out, err := executeDriveCommandCapture(t, caller, "export", "--node", "n1", "--export-format", "docx")
		if err != nil {
			t.Fatal(err)
		}
		if len(caller.toolLog) != 2 {
			t.Fatalf("calls = %v", caller.toolLog)
		}
		// json 模式：stdout 必须是单一可解析 JSON 结果对象（taskId/downloadUrl，
		// camelCase），不能混入纯文本行。
		var payload map[string]any
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("json-mode output is not parseable JSON: %v\n%s", err, out.String())
		}
		if payload["success"] != true || payload["taskId"] != "job-9" || payload["downloadUrl"] != "https://x.test/report.docx" {
			t.Fatalf("payload = %#v", payload)
		}
	})

	t.Run("no output non json keeps key value lines", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "table", steps: []scriptedToolStep{submitOK, queryOK}}
		out, err := executeDriveCommandCapture(t, caller, "export", "--node", "n1", "--export-format", "docx")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "taskId:") || !strings.Contains(out.String(), "downloadUrl:") {
			t.Fatalf("table output = %q, want taskId/downloadUrl key-value lines", out.String())
		}
	})

	t.Run("file output downloads", func(t *testing.T) {
		oldGet := httpGetFile
		httpGetFile = func(context.Context, string, map[string]string, string) error { return nil }
		t.Cleanup(func() { httpGetFile = oldGet })
		target := filepath.Join(t.TempDir(), "target.docx")
		if err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{submitOK, queryOK}},
			"export", "--node", "n1", "--export-format", "docx", "--output", target); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("directory output infers filename", func(t *testing.T) {
		oldGet := httpGetFile
		var savedPath string
		httpGetFile = func(_ context.Context, _ string, _ map[string]string, destination string) error {
			savedPath = destination
			return nil
		}
		t.Cleanup(func() { httpGetFile = oldGet })
		dir := t.TempDir()
		if err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{submitOK, queryOK}},
			"export", "--node", "n1", "--export-format", "docx", "--output", dir); err != nil {
			t.Fatal(err)
		}
		if filepath.Base(savedPath) != "report.docx" {
			t.Fatalf("savedPath = %q, want report.docx", savedPath)
		}
	})

	t.Run("download failure surfaces", func(t *testing.T) {
		oldGet := httpGetFile
		httpGetFile = func(context.Context, string, map[string]string, string) error { return errors.New("disk full") }
		t.Cleanup(func() { httpGetFile = oldGet })
		target := filepath.Join(t.TempDir(), "target.docx")
		err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{submitOK, queryOK}},
			"export", "--node", "n1", "--export-format", "docx", "--output", target)
		if err == nil || !strings.Contains(err.Error(), "文件下载失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("file output prints single json result", func(t *testing.T) {
		oldGet := httpGetFile
		httpGetFile = func(context.Context, string, map[string]string, string) error { return nil }
		t.Cleanup(func() { httpGetFile = oldGet })
		target := filepath.Join(t.TempDir(), "target.docx")
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{submitOK, queryOK}}
		out, err := executeDriveCommandCapture(t, caller, "export", "--node", "n1", "--export-format", "docx", "--output", target)
		if err != nil {
			t.Fatal(err)
		}
		// json 模式：下载完成后 stdout 必须是单一可解析结果对象
		// （success/taskId/outputPath/downloadUrl，camelCase）。
		var payload map[string]any
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("json-mode output is not parseable JSON: %v\n%s", err, out.String())
		}
		if payload["success"] != true || payload["taskId"] != "job-9" ||
			payload["outputPath"] != target || payload["downloadUrl"] != "https://x.test/report.docx" {
			t.Fatalf("payload = %#v", payload)
		}
	})

	t.Run("file output table mode omits json", func(t *testing.T) {
		oldGet := httpGetFile
		httpGetFile = func(context.Context, string, map[string]string, string) error { return nil }
		t.Cleanup(func() { httpGetFile = oldGet })
		target := filepath.Join(t.TempDir(), "target.docx")
		caller := &scriptedToolCaller{format: "table", steps: []scriptedToolStep{submitOK, queryOK}}
		out, err := executeDriveCommandCapture(t, caller, "export", "--node", "n1", "--export-format", "docx", "--output", target)
		if err != nil {
			t.Fatal(err)
		}
		// table 模式：下载进度走 stderr，stdout 不应混入结果 JSON。
		if strings.TrimSpace(out.String()) != "" {
			t.Fatalf("table-mode stdout = %q, want empty", out.String())
		}
	})

	t.Run("file output json print failure propagates", func(t *testing.T) {
		oldGet := httpGetFile
		httpGetFile = func(context.Context, string, map[string]string, string) error { return nil }
		t.Cleanup(func() { httpGetFile = oldGet })
		target := filepath.Join(t.TempDir(), "target.docx")
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{submitOK, queryOK}}
		if err := executeDriveExportFailingWriter(t, caller, "export", "--node", "n1", "--export-format", "docx", "--output", target); err == nil {
			t.Fatal("expected the PrintJSON writer failure to propagate")
		}
	})

	t.Run("async print failure propagates", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{submitOK}}
		if err := executeDriveExportFailingWriter(t, caller, "export", "--node", "n1", "--export-format", "docx", "--async"); err == nil {
			t.Fatal("expected the PrintJSON writer failure to propagate")
		}
	})
}

// ── runDriveExportGet：参数校验 / dry-run / 查询链路 ──

func TestCrossPlatformCoverageDriveExportGetCommand(t *testing.T) {
	t.Run("missing task id", func(t *testing.T) {
		if err := executeDriveEdge(t, &scriptedToolCaller{}, "export", "get"); err == nil {
			t.Fatal("missing task-id returned nil")
		}
	})

	t.Run("dry run prints preview", func(t *testing.T) {
		err := executeDriveEdge(t, &scriptedToolCaller{dry: true}, "export", "get", "--task-id", "job-1", "--dry-run")
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("query success", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"status":"SUCCESS","resultUrl":"https://x.test/f.docx"}`}}}
		if err := executeDriveEdge(t, caller, "export", "get", "--task-id", "job-1", "--format", "json"); err != nil {
			t.Fatal(err)
		}
		if caller.server != "drive" || caller.tool != "query_task" {
			t.Fatalf("routed to %s/%s", caller.server, caller.tool)
		}
	})

	t.Run("query error surfaces", func(t *testing.T) {
		err := executeDriveEdge(t, &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("boom")}}},
			"export", "get", "--task-id", "job-1")
		if err == nil || !strings.Contains(err.Error(), "查询任务失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("hidden job id alias works", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"status":"PROCESSING"}`}}}
		if err := executeDriveEdge(t, caller, "export", "get", "--job-id", "job-2"); err != nil {
			t.Fatal(err)
		}
		if caller.args["taskId"] != "job-2" {
			t.Fatalf("taskId arg = %v", caller.args["taskId"])
		}
	})

	t.Run("query print failure propagates", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"status":"SUCCESS","resultUrl":"https://x.test/f.docx"}`}}}
		if err := executeDriveExportFailingWriter(t, caller, "export", "get", "--task-id", "job-1"); err == nil {
			t.Fatal("expected the PrintJSON writer failure to propagate")
		}
	})
}
