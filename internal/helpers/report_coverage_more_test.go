package helpers

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func reportCreateCoverageCommand(t *testing.T, values map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "create"}
	addReportCreateFlags(cmd)
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}
	return cmd
}

func reportListCoverageCommand(t *testing.T, sent bool, values map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "list"}
	if sent {
		addReportSentFlags(cmd)
	} else {
		addReportListFlags(cmd)
	}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}
	return cmd
}

func TestCrossPlatformCoverageReportHandlerRemainingCoverage(t *testing.T) {
	caller := &reportTestCaller{dry: true, format: "json"}
	installReportTestDeps(t, caller)
	validContents := `[{"key":"Done","sort":"0","content":"work","contentType":"markdown","type":"1"}]`

	if err := runReportCreate(reportCreateCoverageCommand(t, map[string]string{
		"template-id": "template", "contents": "[]",
	}), nil); err == nil {
		t.Fatal("empty contents array returned nil")
	}
	if err := runReportCreate(reportCreateCoverageCommand(t, map[string]string{
		"template-id": "template", "contents": validContents, "dd-from": "", "to-chat": "true", "to-user-ids": "u1,u2",
	}), nil); err != nil {
		t.Fatalf("valid report create: %v", err)
	}

	start := "2026-01-01T00:00:00+08:00"
	end := "2026-01-02T00:00:00+08:00"
	if err := runReportList(reportListCoverageCommand(t, false, map[string]string{
		"start": end, "end": start,
	}), nil); err == nil {
		t.Fatal("reversed inbox range returned nil")
	}
	if err := runReportList(reportListCoverageCommand(t, false, map[string]string{
		"start": start, "end": end, "limit": "3", "sender-user-ids": "u1,u2",
	}), nil); err != nil {
		t.Fatalf("inbox filters: %v", err)
	}

	if err := runReportSent(reportListCoverageCommand(t, true, map[string]string{
		"start": end, "end": start,
	}), nil); err == nil {
		t.Fatal("reversed outbox range returned nil")
	}
	if err := runReportSent(reportListCoverageCommand(t, true, map[string]string{
		"start": start, "end": end, "limit": "4", "modified-start": start, "modified-end": end, "template-name": "daily",
	}), nil); err != nil {
		t.Fatalf("outbox filters: %v", err)
	}
}

func TestCrossPlatformCoverageReportReadableRemainingCoverage(t *testing.T) {
	caller := &reportTestCaller{format: "json", response: "{"}
	installReportTestDeps(t, caller)
	if got := reportDetailForListRow(context.Background(), "report", true); got.markdownLink != "查看详情" {
		t.Fatalf("invalid detail fallback = %#v", got)
	}
	if got := reportMarkdownLinkFromResponse(map[string]any{"dingtalkOpenMarkdownLink": " [open](url) "}); got != "[open](url)" {
		t.Fatalf("direct markdown link = %q", got)
	}
	if maps := reportResponseMaps("not an object"); maps != nil {
		t.Fatalf("non-object response maps = %#v", maps)
	}
	if got := reportTimeValueToString("2026-01-02T03:04:05Z"); got == "" || got == "2026-01-02T03:04:05Z" {
		t.Fatalf("RFC3339 time = %q", got)
	}
	if got := reportTimeValueToString("1970-01-01T00:00:00Z"); got == "" || got == "1970-01-01T00:00:00Z" {
		t.Fatalf("epoch RFC3339 time = %q", got)
	}
	if _, err := readReportContentsStdin(failingReader{}); err == nil {
		t.Fatal("stdin read error returned nil")
	}
	if _, err := normalizeReportContentType(true, "1", 0); err == nil {
		t.Fatal("non-scalar content type returned nil")
	}
}

func TestCrossPlatformCoverageReportFilesystemFailureCoverage(t *testing.T) {
	originalOpen := reportOpenFile
	originalAbs := reportAbsPath
	originalGetwd := reportGetwd
	originalEval := reportEvalSymlinks
	originalStat := reportStat
	originalRel := reportRelPath
	t.Cleanup(func() {
		reportOpenFile = originalOpen
		reportAbsPath = originalAbs
		reportGetwd = originalGetwd
		reportEvalSymlinks = originalEval
		reportStat = originalStat
		reportRelPath = originalRel
	})
	boom := errors.New("filesystem failure")

	reportOpenFile = func(string) (*os.File, error) { return nil, boom }
	if _, err := readReportContentsFile("blocked.json"); err == nil {
		t.Fatal("generic open error returned nil")
	}
	reportOpenFile = originalOpen

	reportAbsPath = func(string) (string, error) { return "", boom }
	if _, err := resolveSafeReportContentsFilePath("report.json"); err == nil {
		t.Fatal("absolute path error returned nil")
	}
	reportAbsPath = originalAbs

	root := t.TempDir()
	reportGetwd = func() (string, error) { return "", boom }
	if _, err := resolveSafeReportContentsFilePath(filepath.Join(root, "report.json")); err == nil {
		t.Fatal("getwd error returned nil")
	}
	reportGetwd = func() (string, error) { return root, nil }

	absCalls := 0
	reportAbsPath = func(path string) (string, error) {
		absCalls++
		if absCalls == 2 {
			return "", boom
		}
		return originalAbs(path)
	}
	if _, err := resolveSafeReportContentsFilePath(filepath.Join(root, "report.json")); err == nil {
		t.Fatal("cwd absolute path error returned nil")
	}
	reportAbsPath = originalAbs

	reportEvalSymlinks = func(string) (string, error) { return "", boom }
	if _, err := resolveSafeReportContentsFilePath(filepath.Join(root, "report.json")); err == nil {
		t.Fatal("root symlink resolution error returned nil")
	}
	reportEvalSymlinks = originalEval

	reportStat = func(string) (os.FileInfo, error) { return nil, boom }
	if _, err := resolveSafeReportContentsFilePath(filepath.Join(root, "report.json")); err == nil {
		t.Fatal("generic stat error returned nil")
	}
	reportStat = originalStat

	directory := filepath.Join(root, "directory")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveSafeReportContentsFilePath(directory); err == nil {
		t.Fatal("directory path returned nil")
	}

	file := filepath.Join(root, "report.json")
	if err := os.WriteFile(file, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	evalCalls := 0
	reportEvalSymlinks = func(path string) (string, error) {
		evalCalls++
		if evalCalls == 2 {
			return "", boom
		}
		return originalEval(path)
	}
	if _, err := resolveSafeReportContentsFilePath(file); err == nil {
		t.Fatal("file symlink resolution error returned nil")
	}
	reportEvalSymlinks = originalEval

	reportRelPath = func(string, string) (string, error) { return "", boom }
	if _, err := resolveSafeReportContentsFilePath(file); err == nil {
		t.Fatal("relative path error returned nil")
	}
}
