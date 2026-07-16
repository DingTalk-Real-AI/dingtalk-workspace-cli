package helpers

import (
	"os"
	"path/filepath"
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
	deleteCmd := findCoverageSubcommand(t, root, "delete")
	_ = deleteCmd.Flags().Set("node", "node")
	_ = deleteCmd.Flags().Set("sheet-id", "sheet")
	_ = deleteCmd.Flags().Set("chart-id", "chart")
	oldArgs := os.Args
	os.Args = []string{"dws", "--yes"}
	if err := deleteCmd.RunE(deleteCmd, nil); err != nil {
		t.Fatalf("confirmed delete: %v", err)
	}
	os.Args = []string{"dws"}
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
	t.Cleanup(func() {
		os.Args = oldArgs
		os.Stdin = oldStdin
		_ = stdin.Close()
	})
	if err := deleteCmd.RunE(deleteCmd, nil); err != nil {
		t.Fatalf("cancelled delete: %v", err)
	}
}
