package helpers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func executeSheetExportCoverage(t *testing.T, caller *scriptedToolCaller, args ...string) error {
	t.Helper()
	installScriptedCaller(t, caller)
	oldArgs := os.Args
	os.Args = []string{"dws", "sheet"}
	t.Cleanup(func() { os.Args = oldArgs })
	cmd := newExportCmd()
	for index := 0; index < len(args); index += 2 {
		if err := cmd.Flags().Set(args[index], args[index+1]); err != nil {
			t.Fatalf("set %s: %v", args[index], err)
		}
	}
	return runSheetExport(cmd, nil)
}

func TestCrossPlatformCoverageSheetExportCommandRemainingCoverage(t *testing.T) {
	installImmediateTiming(t)
	if err := executeSheetExportCoverage(t, &scriptedToolCaller{}); err == nil {
		t.Fatal("missing node returned nil")
	}
	if err := executeSheetExportCoverage(t, &scriptedToolCaller{dry: true}, "node", "node", "output", "file.xlsx"); err != nil {
		t.Fatalf("dry run with output: %v", err)
	}
	if err := executeSheetExportCoverage(t, &scriptedToolCaller{dry: true}, "node", "node"); err != nil {
		t.Fatalf("dry run without output: %v", err)
	}

	successSteps := func(url string) []scriptedToolStep {
		return []scriptedToolStep{{text: `{"jobId":"job"}`}, {text: `{"status":"SUCCESS","downloadUrl":"` + url + `"}`}}
	}
	if err := executeSheetExportCoverage(t, &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("submit")}}}, "node", "node"); err == nil {
		t.Fatal("submit error returned nil")
	}
	if err := executeSheetExportCoverage(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{`}}}, "node", "node"); err == nil {
		t.Fatal("invalid submit response returned nil")
	}
	if err := executeSheetExportCoverage(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"jobId":"job"}`}, {text: `{`}}}, "node", "node"); err == nil {
		t.Fatal("poll parse error returned nil")
	}
	if err := executeSheetExportCoverage(t, &scriptedToolCaller{format: "table", steps: successSteps("https://example.test/file.xlsx")}, "node", "node"); err != nil {
		t.Fatalf("table URL result: %v", err)
	}
	if err := executeSheetExportCoverage(t, &scriptedToolCaller{format: "json", steps: successSteps("https://example.test/file.xlsx")}, "node", "node"); err != nil {
		t.Fatalf("JSON URL result: %v", err)
	}

	oldGet := httpGetFile
	t.Cleanup(func() { httpGetFile = oldGet })
	var destination string
	httpGetFile = func(_ context.Context, _ string, _ map[string]string, output string) error {
		destination = output
		return nil
	}
	directory := t.TempDir()
	if err := executeSheetExportCoverage(t, &scriptedToolCaller{format: "table", steps: successSteps("https://example.test/")}, "node", "node", "output", directory); err != nil {
		t.Fatalf("directory output: %v", err)
	}
	if filepath.Base(destination) != "sheet-export-job.xlsx" {
		t.Fatalf("fallback destination = %q", destination)
	}
	file := filepath.Join(directory, "explicit.xlsx")
	if err := executeSheetExportCoverage(t, &scriptedToolCaller{format: "json", steps: successSteps("https://example.test/file.xlsx")}, "node", "node", "output", file); err != nil {
		t.Fatalf("JSON file output: %v", err)
	}

	boom := errors.New("download failed")
	httpGetFile = func(context.Context, string, map[string]string, string) error { return boom }
	if err := executeSheetExportCoverage(t, &scriptedToolCaller{format: "table", steps: successSteps("https://example.test/file.xlsx")}, "node", "node", "output", file); err == nil {
		t.Fatal("download error returned nil")
	}
}

func TestCrossPlatformCoverageSheetExportFilenameDotCoverage(t *testing.T) {
	if got := inferSheetExportFilename("https://example.test/."); got != "" {
		t.Fatalf("dot filename = %q", got)
	}
}
