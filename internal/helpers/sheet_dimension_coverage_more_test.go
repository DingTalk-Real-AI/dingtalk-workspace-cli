package helpers

import (
	"io"
	"testing"

	"github.com/spf13/cobra"
)

func dimensionCoverageCommand(t *testing.T, name string) *cobra.Command {
	t.Helper()
	for _, cmd := range newDimensionCmds() {
		if cmd.Name() == name {
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			return cmd
		}
	}
	t.Fatalf("dimension command %q not found", name)
	return nil
}

func executeDimensionCoverage(t *testing.T, name string, args ...string) error {
	t.Helper()
	cmd := dimensionCoverageCommand(t, name)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestCrossPlatformCoverageDimensionValidationRemainingCoverage(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	common := []string{"--node", "node", "--sheet-id", "sheet"}
	if err := executeDimensionCoverage(t, "insert-dimension", append(common, "--dimension", "ROWS", "--position", "1", "--length", "5001")...); err == nil {
		t.Fatal("oversized insert returned nil")
	}
	for _, tc := range []struct {
		dimension string
		extra     []string
		wantErr   bool
	}{
		{"ROW", []string{"--start-index", "1", "--end-index", "2", "--destination-index", "3"}, false},
		{"COLUMN", []string{"--start-index", "A", "--end-index", "B", "--destination-index", "C"}, false},
		{"ROWS", []string{"--end-index", "2", "--destination-index", "3"}, true},
		{"ROWS", []string{"--start-index", "1", "--destination-index", "3"}, true},
		{"ROWS", []string{"--start-index", "1", "--end-index", "2"}, true},
	} {
		args := append(append([]string{}, common...), "--dimension", tc.dimension)
		args = append(args, tc.extra...)
		err := executeDimensionCoverage(t, "move-dimension", args...)
		if (err != nil) != tc.wantErr {
			t.Errorf("move %s %v error=%v", tc.dimension, tc.extra, err)
		}
	}

	add := dimensionCoverageCommand(t, "add-dimension")
	_ = add.Flags().Set("dimension", "ROWS")
	add.Flags().Lookup("length").Value = invalidIntFlagValue{}
	if err := add.RunE(add, nil); err == nil {
		t.Fatal("invalid add length returned nil")
	}
	if err := executeDimensionCoverage(t, "add-dimension", append(common, "--dimension", "ROWS", "--length", "5001")...); err == nil {
		t.Fatal("oversized add returned nil")
	}
	if err := executeDimensionCoverage(t, "delete-dimension", append(common, "--dimension", "ROWS", "--position", "1", "--length", "5001")...); err == nil {
		t.Fatal("oversized delete returned nil")
	}
	if err := executeDimensionCoverage(t, "update-dimension", append(common, "--dimension", "ROWS", "--start-index", "1", "--length", "5001", "--hidden")...); err == nil {
		t.Fatal("oversized update returned nil")
	}
	if err := executeDimensionCoverage(t, "update-dimension", append(common, "--dimension", "ROWS", "--start-index", "1", "--length", "1")...); err == nil {
		t.Fatal("update without property returned nil")
	}
}

func TestCrossPlatformCoverageDropdownValidationRemainingCoverage(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	base := []string{"--node", "node", "--sheet-id", "sheet", "--range", "A1"}
	for _, options := range []string{"[]", `[{"value":"a,b"}]`} {
		if err := executeDimensionCoverage(t, "set-dropdown", append(base, "--options", options)...); err == nil {
			t.Fatalf("options %s returned nil", options)
		}
	}
	if err := executeDimensionCoverage(t, "set-dropdown", append(base, "--options", `[{"value":"a"}]`, "--multi-select")...); err != nil {
		t.Fatalf("multi-select dropdown: %v", err)
	}
}
