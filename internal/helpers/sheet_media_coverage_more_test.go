package helpers

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func executeSheetMediaEdge(t *testing.T, index int, caller *scriptedToolCaller, args ...string) error {
	t.Helper()
	old := deps
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	t.Cleanup(func() { deps = old })
	commands := newMediaCmds()
	cmd := commands[index]
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestCrossPlatformCoverageSheetMediaUploadRemainingCoverage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(file, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		args []string
	}{
		{"node", nil},
		{"file", []string{"--node", "node"}},
		{"stat", []string{"--node", "node", "--file", file + ".missing"}},
		{"directory", []string{"--node", "node", "--file", t.TempDir()}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := executeSheetMediaEdge(t, 0, &scriptedToolCaller{}, tc.args...); err == nil {
				t.Fatal("validation returned nil")
			}
		})
	}
	if err := executeSheetMediaEdge(t, 0, &scriptedToolCaller{dry: true}, "--node", "node", "--file", file, "--name", "renamed"); err != nil {
		t.Fatalf("dry run: %v", err)
	}

	boom := errors.New("boom")
	if err := executeSheetMediaEdge(t, 0, &scriptedToolCaller{steps: []scriptedToolStep{{err: boom}}}, "--node", "node", "--file", file); !errors.Is(err, boom) {
		t.Fatalf("tool error=%v", err)
	}
	if err := executeSheetMediaEdge(t, 0, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}}, "--node", "node", "--file", file); err == nil {
		t.Fatal("credential parse error returned nil")
	}

	origPut := httpPutFile
	t.Cleanup(func() { httpPutFile = origPut })
	credential := `{"result":{"uploadUrl":"https://upload.invalid","resourceId":"resource","resourceUrl":"https://resource.invalid"}}`
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return boom }
	if err := executeSheetMediaEdge(t, 0, &scriptedToolCaller{steps: []scriptedToolStep{{text: credential}}}, "--node", "node", "--file", file); !errors.Is(err, boom) {
		t.Fatalf("put error=%v", err)
	}
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }
	for _, format := range []string{"json", "pretty"} {
		caller := &scriptedToolCaller{format: format, steps: []scriptedToolStep{{text: credential}}}
		if err := executeSheetMediaEdge(t, 0, caller, "--node", "node", "--file", file, "--mime-type", "image/png"); err != nil {
			t.Fatalf("format %s: %v", format, err)
		}
	}
}

func TestCrossPlatformCoverageSheetWriteImageRemainingCoverage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(file, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := [][]string{
		nil,
		{"--node", "node"},
		{"--node", "node", "--sheet-id", "sheet"},
		{"--node", "node", "--sheet-id", "sheet", "--range", "A1"},
		{"--node", "node", "--sheet-id", "sheet", "--range", "A1", "--file", file + ".missing"},
		{"--node", "node", "--sheet-id", "sheet", "--range", "A1", "--file", t.TempDir()},
	}
	for _, args := range cases {
		if err := executeSheetMediaEdge(t, 1, &scriptedToolCaller{}, args...); err == nil {
			t.Fatalf("validation %v returned nil", args)
		}
	}
	base := []string{"--node", "node", "--sheet-id", "sheet", "--range", "A1", "--file", file, "--name", "renamed"}
	defaultName := []string{"--node", "node", "--sheet-id", "sheet", "--range", "A1", "--file", file}
	if err := executeSheetMediaEdge(t, 1, &scriptedToolCaller{dry: true}, defaultName...); err != nil {
		t.Fatalf("default name dry run: %v", err)
	}
	if err := executeSheetMediaEdge(t, 1, &scriptedToolCaller{dry: true}, base...); err != nil {
		t.Fatalf("dry run: %v", err)
	}

	boom := errors.New("boom")
	if err := executeSheetMediaEdge(t, 1, &scriptedToolCaller{steps: []scriptedToolStep{{err: boom}}}, base...); !errors.Is(err, boom) {
		t.Fatalf("tool error=%v", err)
	}
	if err := executeSheetMediaEdge(t, 1, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}}, base...); err == nil {
		t.Fatal("credential parse error returned nil")
	}

	origPut := httpPutFile
	t.Cleanup(func() { httpPutFile = origPut })
	credential := `{"result":{"uploadUrl":"https://upload.invalid","resourceId":"resource","resourceUrl":"https://resource.invalid"}}`
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return boom }
	if err := executeSheetMediaEdge(t, 1, &scriptedToolCaller{steps: []scriptedToolStep{{text: credential}}}, base...); !errors.Is(err, boom) {
		t.Fatalf("put error=%v", err)
	}
	httpPutFile = func(context.Context, string, map[string]string, string, int64) error { return nil }

	for _, format := range []string{"json", "pretty"} {
		steps := []scriptedToolStep{{text: credential}, {text: `{}`}}
		caller := &scriptedToolCaller{format: format, steps: steps}
		args := append(append([]string(nil), base...), "--width", "20", "--height", "10")
		if err := executeSheetMediaEdge(t, 1, caller, args...); err != nil {
			t.Fatalf("format %s: %v", format, err)
		}
	}
	caller := &scriptedToolCaller{format: "pretty", steps: []scriptedToolStep{{text: credential}, {err: boom}}}
	if err := executeSheetMediaEdge(t, 1, caller, base...); !errors.Is(err, boom) && (err == nil || !strings.Contains(err.Error(), "boom")) {
		t.Fatalf("write error=%v", err)
	}
}

func TestCrossPlatformCoverageSheetMediaCommandDefinitionsCoverage(t *testing.T) {
	for _, cmd := range newMediaCmds() {
		if cmd.RunE == nil || cmd.Name() == "" {
			t.Fatalf("invalid media command %#v", cmd)
		}
	}
	_ = (&cobra.Command{}).Name()
}
