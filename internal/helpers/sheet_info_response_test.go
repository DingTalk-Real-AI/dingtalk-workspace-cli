package helpers

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageSheetInfoRejectsInvalidResponses(t *testing.T) {
	for _, format := range []string{"json", "raw"} {
		t.Run(format, func(t *testing.T) {
			for _, tc := range []struct {
				name string
				text string
			}{
				{"empty", ""},
				{"whitespace", " \n\t"},
				{"array", `[]`},
				{"string", `"service busy"`},
				{"boolean", `true`},
				{"number", `42`},
				{"plain text", "service busy"},
				{"malformed JSON", `{"sheetId":`},
				{"null", `null`},
			} {
				t.Run(tc.name, func(t *testing.T) {
					caller := &scriptedToolCaller{format: format, steps: []scriptedToolStep{{text: tc.text}}}
					stdout, err := executeSheetReadCompletionCommand(t, caller, "info", "--node", "NODE")
					var cliErr *CLIError
					if !errors.As(err, &cliErr) || cliErr.Code != CodeMCPToolError || cliErr.ExitCode() != ExitAPI {
						t.Fatalf("error = %T %v, want MCP tool error with API exit code", err, err)
					}
					if cliErr.Operation != "sheet/get_sheet" || !strings.Contains(cliErr.Message, "get_sheet") {
						t.Errorf("error does not identify the failed operation: %#v", cliErr)
					}
					if stdout.Len() != 0 {
						t.Errorf("invalid response wrote a success body: %q", stdout.String())
					}
					if caller.calls != 1 || caller.tool != "get_sheet" {
						t.Errorf("calls/tool = %d/%q, want one get_sheet call", caller.calls, caller.tool)
					}
				})
			}
		})
	}
}

func TestCrossPlatformCoverageSheetInfoPreservesObjectResponses(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want map[string]any
	}{
		{"empty workbook", `{}`, map[string]any{"nonEmptyRange": nil}},
		{"legacy coordinates", `{"sheetId":"Sheet1","lastNonEmptyRow":1,"lastNonEmptyColumn":2}`, map[string]any{
			"sheetId":       "Sheet1",
			"nonEmptyRange": map[string]any{"range": "A1:C2", "lastCell": "C2", "lastRow": float64(2), "lastColumn": "C"},
		}},
		{"normalized coordinates", `{"sheetId":"Sheet1","nonEmptyRange":{"range":"A1:B3","lastCell":"B3","lastRow":3,"lastColumn":"B"}}`, map[string]any{
			"sheetId":       "Sheet1",
			"nonEmptyRange": map[string]any{"range": "A1:B3", "lastCell": "B3", "lastRow": float64(3), "lastColumn": "B"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: tc.text}}}
			stdout, err := executeSheetReadCompletionCommand(t, caller, "info", "--node", "NODE")
			if err != nil {
				t.Fatalf("sheet info: %v", err)
			}
			if got := decodeSheetReadOutput(t, stdout); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("sheet info = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageSheetInfoPreservesUpstreamErrors(t *testing.T) {
	want := &CLIError{Code: CodeAuthPermission, Message: "synthetic permission denial"}
	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{err: want}}}
	stdout, err := executeSheetReadCompletionCommand(t, caller, "info", "--node", "NODE")
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want original permission error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("failure wrote a success body: %q", stdout.String())
	}
}

func TestCrossPlatformCoverageSheetInfoPreservesDryRun(t *testing.T) {
	caller := &scriptedToolCaller{format: "json", dry: true, steps: []scriptedToolStep{{text: `{"dry_run":true}`}}}
	stdout, err := executeSheetReadCompletionCommand(t, caller, "info", "--node", "NODE")
	if err != nil {
		t.Fatalf("sheet info dry-run: %v", err)
	}
	got := decodeSheetReadOutput(t, stdout)
	if got["dry_run"] != true {
		t.Fatalf("dry-run preview = %#v", got)
	}
	if _, exists := got["nonEmptyRange"]; exists {
		t.Fatalf("dry-run preview was normalized as sheet data: %#v", got)
	}
}
