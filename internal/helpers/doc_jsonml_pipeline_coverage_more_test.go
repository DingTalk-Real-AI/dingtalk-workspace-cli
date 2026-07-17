package helpers

import (
	"errors"
	"strings"
	"testing"
)

func TestCrossPlatformCoveragePrepareJSONMLRemainingErrorPaths(t *testing.T) {
	strict := jsonMLTestCommand(t, false)
	fixing := jsonMLTestCommand(t, true)

	if _, err := prepareJsonMLNode(strict, ""); err == nil {
		t.Fatal("empty node returned nil")
	}

	origRepair := repairJSONML
	repairJSONML = func(string) (string, error) { return "", errors.New("repair failed") }
	if _, err := prepareJsonMLBody(fixing, "{"); err == nil || !strings.Contains(err.Error(), "repair failed") {
		t.Fatalf("body repair err=%v", err)
	}
	if _, err := prepareJsonMLNode(fixing, "{"); err == nil || !strings.Contains(err.Error(), "repair failed") {
		t.Fatalf("node repair err=%v", err)
	}
	repairJSONML = origRepair
	t.Cleanup(func() { repairJSONML = origRepair })
	repairJSONML = func(string) (string, error) { return "{", nil }
	if _, err := prepareJsonMLNode(fixing, "broken"); err == nil || !strings.Contains(err.Error(), "JSON 语法错误") {
		t.Fatalf("invalid repaired node err=%v", err)
	}
	repairJSONML = origRepair

	// Exercise the concise syntax-error diagnostic when the detailed parser has
	// no additional location information.
	origDiagnose := diagnoseJSONMLError
	diagnoseJSONMLError = func([]byte) string { return "" }
	if _, err := prepareJsonMLBody(strict, "{"); err == nil || !strings.Contains(err.Error(), "输入不是有效") {
		t.Fatalf("body concise diagnostic err=%v", err)
	}
	if _, err := prepareJsonMLNode(strict, "{"); err == nil || !strings.Contains(err.Error(), "输入不是有效") {
		t.Fatalf("node concise diagnostic err=%v", err)
	}
	diagnoseJSONMLError = func([]byte) string { return "line 1" }
	if _, err := prepareJsonMLBody(strict, "{"); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("body detailed diagnostic err=%v", err)
	}
	if _, err := prepareJsonMLNode(strict, "{"); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("node detailed diagnostic err=%v", err)
	}
	diagnoseJSONMLError = origDiagnose

	// Valid JSON with the wrong top-level shape is not a syntax-repair case.
	if _, err := prepareJsonMLBody(fixing, `"scalar"`); err == nil || !strings.Contains(err.Error(), "修复后仍无法解析") {
		t.Fatalf("fixed scalar body err=%v", err)
	}

	if _, err := prepareJsonMLBody(strict, `{"jsonml":["root",{},["p",{"jc":1}]]}`); err == nil || !strings.Contains(err.Error(), "格式校验失败") {
		t.Fatalf("body validation err=%v", err)
	}
	if _, err := prepareJsonMLNode(strict, `["p",{"jc":1}]`); err == nil || !strings.Contains(err.Error(), "格式校验失败") {
		t.Fatalf("node validation err=%v", err)
	}
}
