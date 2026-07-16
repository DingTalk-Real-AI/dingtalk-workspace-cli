package helpers

import "testing"

func TestCrossPlatformCoverageCondFormatRemainingCoverage(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	base := []string{"create", "--node", "node", "--sheet-id", "sheet"}
	for _, extra := range [][]string{
		{"--ranges", "[]", "--condition", `{}`},
		{"--ranges", `["A1"]`, "--condition", `{`},
		{"--ranges", `["A1"]`, "--condition", `{}`, "--cell-style", `{`},
		{"--ranges", `["A1"]`, "--condition", `{}`, "--cell-style", `{}`, "--data-bar-style", `{`},
	} {
		if err := executeFilterCoverage(t, newCondFormatCmd(), append(base, extra...)...); err == nil {
			t.Fatalf("invalid create arguments %v returned nil", extra)
		}
	}
	if err := executeFilterCoverage(t, newCondFormatCmd(), append(base,
		"--ranges", `["A1:B2"]`,
		"--condition", `{"numberCondition":{"operator":"greater","value1":"1"}}`,
		"--cell-style", `{"bold":true}`,
		"--data-bar-style", `{"isGradient":true}`,
	)...); err != nil {
		t.Fatalf("create all fields: %v", err)
	}

	if err := executeFilterCoverage(t, newCondFormatCmd(),
		"update", "--node", "node", "--sheet-id", "sheet", "--rule-id", "rule",
		"--ranges", `["A1:B2"]`,
		"--condition", `{"formulaCondition":{"formula":"=A1>1"}}`,
		"--cell-style", `{"bold":true}`,
		"--data-bar-style", `{"isGradient":false}`,
	); err != nil {
		t.Fatalf("update all fields: %v", err)
	}
}
