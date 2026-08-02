package helpers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func executeFormulaVerify(t *testing.T, caller *scriptedToolCaller, stdin *strings.Reader, args ...string) error {
	t.Helper()
	installScriptedCaller(t, caller)
	cmd := newSheetFormulaVerifyCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	if stdin != nil {
		cmd.SetIn(stdin)
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestCrossPlatformCoverageSheetFormulaVerifyRejectsNonPositiveLimits(t *testing.T) {
	if err := executeFormulaVerify(t, &scriptedToolCaller{}, nil,
		"--node", "n1", "--max-locations-per-error", "0"); err == nil {
		t.Fatal("max-locations-per-error 0 returned nil")
	}
	if err := executeFormulaVerify(t, &scriptedToolCaller{}, nil,
		"--node", "n1", "--max-cells", "-1"); err == nil {
		t.Fatal("max-cells -1 returned nil")
	}
}

func TestCrossPlatformCoverageSheetFormulaVerifyLimitsAndInlineTargets(t *testing.T) {
	caller := &scriptedToolCaller{}
	err := executeFormulaVerify(t, caller, nil,
		"--node", "n1", "--max-locations-per-error", "3", "--max-cells", "100",
		"--targets", `[{"sheetId":"Sheet1","range":"A1:D10"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls != 1 {
		t.Fatalf("calls = %d, want 1", caller.calls)
	}
}

func TestCrossPlatformCoverageSheetFormulaVerifyTargetsFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "targets.json")
	if err := os.WriteFile(path, []byte(`[{"sheetId":"Sheet1"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	caller := &scriptedToolCaller{}
	if err := executeFormulaVerify(t, caller, nil, "--node", "n1", "--targets", "@"+path); err != nil {
		t.Fatal(err)
	}
	if caller.calls != 1 {
		t.Fatalf("calls = %d, want 1", caller.calls)
	}
}

func TestCrossPlatformCoverageSheetFormulaVerifyTargetsFileMissing(t *testing.T) {
	err := executeFormulaVerify(t, &scriptedToolCaller{}, nil,
		"--node", "n1", "--targets", "@/nonexistent/targets.json")
	if err == nil || !strings.Contains(err.Error(), "读取 --targets 文件失败") {
		t.Fatalf("err = %v, want file read failure", err)
	}
}

func TestCrossPlatformCoverageSheetFormulaVerifyTargetsFromStdin(t *testing.T) {
	caller := &scriptedToolCaller{}
	if err := executeFormulaVerify(t, caller, strings.NewReader(`[{"sheetId":"S1"}]`),
		"--node", "n1", "--targets", "-"); err != nil {
		t.Fatal(err)
	}
	if caller.calls != 1 {
		t.Fatalf("calls = %d, want 1", caller.calls)
	}
}

func TestCrossPlatformCoverageSheetFormulaVerifyTargetsStdinFailure(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{})
	cmd := newSheetFormulaVerifyCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetIn(failingReader{})
	cmd.SetArgs([]string{"--node", "n1", "--targets", "-"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "读取 stdin 失败") {
		t.Fatalf("err = %v, want stdin failure", err)
	}
}

func TestCrossPlatformCoverageSheetFormulaVerifyTargetsConflict(t *testing.T) {
	for _, extra := range [][]string{
		{"--sheet-id", "Sheet1"},
		{"--range", "A1:B2"},
	} {
		args := append([]string{"--node", "n1", "--targets", `[{"sheetId":"S1"}]`}, extra...)
		err := executeFormulaVerify(t, &scriptedToolCaller{}, nil, args...)
		if err == nil || !strings.Contains(err.Error(), "--targets 不能与 --sheet-id 或 --range 同时使用") {
			t.Fatalf("args %v err = %v, want conflict error", extra, err)
		}
	}
}

func TestCrossPlatformCoverageSheetFormulaVerifyExitOnError(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"status":"ERRORS_FOUND","totalErrors":2}`},
	}}
	err := executeFormulaVerify(t, caller, nil, "--node", "n1", "--exit-on-error")
	if err == nil || !strings.Contains(err.Error(), "formula errors found") {
		t.Fatalf("err = %v, want formula errors found", err)
	}

	caller = &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"result":{"status":"partial","totalErrors":1}}`},
	}}
	err = executeFormulaVerify(t, caller, nil, "--node", "n1", "--exit-on-error")
	if err == nil || !strings.Contains(err.Error(), "formula errors found") {
		t.Fatalf("nested totalErrors err = %v, want formula errors found", err)
	}

	caller = &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"status":"clean","totalErrors":0}`},
	}}
	if err := executeFormulaVerify(t, caller, nil, "--node", "n1", "--exit-on-error"); err != nil {
		t.Fatalf("clean result err = %v", err)
	}

	caller = &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"status":"errors_found"}`},
	}}
	if err := executeFormulaVerify(t, caller, nil, "--node", "n1"); err != nil {
		t.Fatalf("errors without --exit-on-error err = %v", err)
	}
}

func TestCrossPlatformCoverageSheetFormulaVerifyPayloadEdges(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `plain text result`}}}
	if err := executeFormulaVerify(t, caller, nil, "--node", "n1", "--exit-on-error"); err != nil {
		t.Fatalf("non-JSON payload err = %v", err)
	}

	caller = &scriptedToolCaller{steps: []scriptedToolStep{{text: `null`}}}
	err := executeFormulaVerify(t, caller, nil, "--node", "n1")
	if err == nil || !strings.Contains(err.Error(), "empty result") {
		t.Fatalf("null payload err = %v, want empty result", err)
	}

	caller = &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"errorCode":"forbidden.x","errorMsg":"denied"}`}}}
	if err := executeFormulaVerify(t, caller, nil, "--node", "n1"); err == nil {
		t.Fatal("business error returned nil")
	}

	caller = &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"status":"clean"}`}}}
	installScriptedCaller(t, caller)
	deps.Out.w = failingWriter{}
	cmd := newSheetFormulaVerifyCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--node", "n1"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("print failure err = %v, want write failed", err)
	}
}

func TestCrossPlatformCoverageSheetFormulaVerifyDryRunShortCircuit(t *testing.T) {
	caller := &scriptedToolCaller{dry: true}
	if err := executeFormulaVerify(t, caller, nil, "--node", "n1", "--exit-on-error"); err != nil {
		t.Fatal(err)
	}
	if caller.calls != 0 {
		t.Fatalf("dry-run calls = %d, want 0 (preview only)", caller.calls)
	}
}
