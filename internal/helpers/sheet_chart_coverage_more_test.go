package helpers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validChartPropertiesCoverage = `{"position":{"row":0,"col":"A"},"dimensions":{"width":100,"height":100},"chart":{"type":"line","series":[{"value":["A1:A2"]}]}}`

func TestCrossPlatformCoverageSheetChartCommandRemainingCoverage(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	base := []string{"--node", "node", "--sheet-id", "sheet"}
	for _, args := range [][]string{
		append([]string{"create"}, base...),
		append(append([]string{"create"}, base...), "--properties", "@"+filepath.Join(t.TempDir(), "missing")),
		append(append([]string{"create"}, base...), "--properties", `{}`),
		append(append([]string{"update"}, base...), "--chart-id", "chart"),
		append(append([]string{"update"}, base...), "--chart-id", "chart", "--properties", "@"+filepath.Join(t.TempDir(), "missing")),
		append(append([]string{"update"}, base...), "--chart-id", "chart", "--properties", `{}`),
	} {
		if err := executeFilterCoverage(t, newChartCmd(), args...); err == nil {
			t.Fatalf("args=%v returned nil", args)
		}
	}

	propsFile := filepath.Join(t.TempDir(), "chart.json")
	if err := os.WriteFile(propsFile, []byte(validChartPropertiesCoverage), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"create", "update"} {
		args := append([]string{name}, base...)
		if name == "update" {
			args = append(args, "--chart-id", "chart")
		}
		args = append(args, "--properties", "@"+propsFile)
		if err := executeFilterCoverage(t, newChartCmd(), args...); err != nil {
			t.Fatalf("%s from file: %v", name, err)
		}
	}

	root := newChartCmd()
	installExampleGlobalFlags(root)
	deleteCmd := findCoverageSubcommand(t, root, "delete")
	_ = deleteCmd.Flags().Set("node", "node")
	_ = deleteCmd.Flags().Set("sheet-id", "sheet")
	_ = deleteCmd.Flags().Set("chart-id", "chart")
	_ = root.PersistentFlags().Set("yes", "true")
	if err := deleteCmd.RunE(deleteCmd, nil); err != nil {
		t.Fatalf("confirmed delete: %v", err)
	}
	_ = root.PersistentFlags().Set("yes", "false")
	// Cancellation must exercise ConfirmSafety; a dry-run caller bypasses it.
	installScriptedCaller(t, &scriptedToolCaller{})
	noFile := filepath.Join(t.TempDir(), "no")
	if err := os.WriteFile(noFile, []byte("no\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdin, err := os.Open(noFile)
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = stdin
	deleteCmd.SetIn(stdin)
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = stdin.Close()
	})
	if err := deleteCmd.RunE(deleteCmd, nil); err == nil || !strings.Contains(err.Error(), "用户取消了操作") {
		t.Fatalf("cancelled delete: error = %v, want 用户取消了操作", err)
	}
}
