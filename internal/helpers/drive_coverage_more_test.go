package helpers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	return root.Execute()
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
	httpGetFile = func(context.Context, string, map[string]string, string) error { return nil }
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
	for _, args := range [][]string{
		{"delete", "--node", "node"},
		{"publish", "set", "--node", "node"},
		{"publish", "unset", "--node", "node"},
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
		if err := executeDriveEdge(t, &scriptedToolCaller{}, args...); err != nil {
			t.Fatalf("cancel %v: %v", args, err)
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
