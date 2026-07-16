package helpers

import (
	"os"
	"testing"
)

func TestCrossPlatformCoverageSheetCellInfoCleanupRemainingCoverage(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "sheet"}
	t.Cleanup(func() { os.Args = oldArgs })
	payload := `{"cells":["not-a-row",[1,{"value":"plain"},{"dataValidation":{"type":null},"hyperlink":{"url":null}},{"dataValidation":{"type":"list"},"hyperlink":{"url":"https://example.test"}}]]}`
	installScriptedCaller(t, &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: payload}}})
	if err := callMCPToolCellInfos(map[string]any{"nodeId": "node"}); err != nil {
		t.Fatalf("cell info cleanup: %v", err)
	}
}
