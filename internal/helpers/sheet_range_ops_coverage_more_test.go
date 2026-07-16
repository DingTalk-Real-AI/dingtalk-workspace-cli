package helpers

import "testing"

func TestCrossPlatformCoverageSheetRangeUpdateAndSortRemainingCoverage(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	base := []string{"update", "--node", "node", "--sheet-id", "sheet", "--range", "A1", "--values"}
	for _, raw := range []string{
		`{`,
		`[[null]]`,
		`[[1]]`,
		`[[{"type":"unsupported"}]]`,
	} {
		if err := executeFilterCoverage(t, newRangeCmd(), append(base, raw)...); err == nil {
			t.Fatalf("invalid values %s returned nil", raw)
		}
	}
	for _, raw := range []string{
		`[[{}]]`,
		`[[{"type":"text","text":"ok"}]]`,
	} {
		if err := executeFilterCoverage(t, newRangeCmd(), append(base, raw)...); err != nil {
			t.Fatalf("valid values %s: %v", raw, err)
		}
	}

	sortBase := []string{"sort", "--node", "node", "--sheet-id", "sheet", "--range", "A1:B2", "--sort-keys"}
	if err := executeFilterCoverage(t, newRangeCmd(), append(sortBase, "{")...); err == nil {
		t.Fatal("invalid sort keys returned nil")
	}
	if err := executeFilterCoverage(t, newRangeCmd(), append(sortBase, `[{"column":"A","ascending":true}]`, "--has-header")...); err != nil {
		t.Fatalf("sort with header: %v", err)
	}
}
