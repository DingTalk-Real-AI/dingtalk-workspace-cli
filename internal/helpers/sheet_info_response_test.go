package helpers

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageSheetInfoRejectsMissingOrNonObjectResponse(t *testing.T) {
	for _, tc := range []struct {
		name, text, message string
		content             []edition.ContentBlock
	}{
		{name: "null", text: "null", message: "get_sheet returned a null response"},
		{name: "whitespace null", text: " \nnull\t", message: "get_sheet returned a null response"},
		{name: "array", text: "[]"},
		{name: "nonempty array", text: `[{}]`},
		{name: "string", text: `"text"`},
		{name: "number", text: "42"},
		{name: "boolean", text: "true"},
		{name: "plain text", text: "service busy"},
		{name: "malformed JSON", text: `{"sheetId":`},
		{name: "empty text", message: "get_sheet returned an empty response"},
		{name: "whitespace text", text: " \n\t", message: "get_sheet returned an empty response"},
		{name: "empty content", content: []edition.ContentBlock{}, message: "get_sheet returned an empty response"},
		{name: "no text", content: []edition.ContentBlock{{Type: "image"}}, message: "get_sheet returned an empty response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testseam.Swap(t, &os.Args, []string{"dws", "sheet"})
			result := textToolResult(tc.text)
			if tc.content != nil {
				result.Content = tc.content
			}
			caller := &helpersCoreCaller{format: "json", result: result}
			InitDepsForTest(t, caller)
			var stdout, stderr bytes.Buffer
			deps.Out.SetWriters(&stdout, &stderr)
			err := callMCPToolSheetInfo(map[string]any{"nodeId": "node"})
			var failure *apperrors.Error
			if !errors.As(err, &failure) || failure.Category != apperrors.CategoryAPI || failure.Reason != CodeMCPToolError || failure.ExitCode() != ExitAPI || failure.Operation != "sheet/get_sheet" || failure.ServerKey != "sheet" || failure.Hint == "" {
				t.Fatalf("missing classified response error: %#v", err)
			}
			message := tc.message
			if message == "" {
				message = "get_sheet returned a non-object response"
			}
			if failure.Message != message || stdout.String() != "" || stderr.String() != "" || caller.calls != 1 {
				t.Fatalf("error=%v stdout=%q stderr=%q calls=%d", failure, stdout.String(), stderr.String(), caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageSheetInfoPreservesObjectCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       map[string]any
	}{
		{"empty object", `{}`, map[string]any{"nonEmptyRange": nil}},
		{"modern coordinates", `{"sheetId":"s1","nonEmptyRange":{"range":"A1:C2","lastCell":"C2","lastRow":2,"lastColumn":"C"}}`, map[string]any{
			"sheetId": "s1", "nonEmptyRange": map[string]any{"range": "A1:C2", "lastCell": "C2", "lastRow": float64(2), "lastColumn": "C"},
		}},
		{"legacy coordinates", `{"sheetId":"s1","lastNonEmptyRow":1,"lastNonEmptyColumn":2,"lastNonEmptyIndexBase":0,"lastNonEmptyRowNumber":2,"lastNonEmptyColumnLetter":"C","nonEmptyRangeA1":"A1:C2"}`, map[string]any{
			"sheetId": "s1", "nonEmptyRange": map[string]any{"range": "A1:C2", "lastCell": "C2", "lastRow": float64(2), "lastColumn": "C"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testseam.Swap(t, &os.Args, []string{"dws", "sheet"})
			caller := &helpersCoreCaller{format: "json", result: textToolResult(tc.text)}
			InitDepsForTest(t, caller)
			var stdout bytes.Buffer
			deps.Out.SetWriters(&stdout, nil)
			if err := callMCPToolSheetInfo(map[string]any{"nodeId": "node"}); err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) || caller.calls != 1 {
				t.Fatalf("got=%#v want=%#v calls=%d", got, tc.want, caller.calls)
			}
		})
	}
	normalizeSheetInfoCoordinatesForAgent(nil)
}

func TestCrossPlatformCoverageSheetInfoPreservesUpstreamErrorsAndDryRun(t *testing.T) {
	for _, code := range []string{CodeAuthTokenExpired, CodeAuthPermission, CodeMCPToolError} {
		t.Run(code, func(t *testing.T) {
			testseam.Swap(t, &os.Args, []string{"dws", "sheet"})
			want := &CLIError{Code: code, Message: "upstream failure", ServerCode: "upstream-code"}
			caller := &helpersCoreCaller{format: "json", err: want}
			InitDepsForTest(t, caller)
			if got := callMCPToolSheetInfo(map[string]any{"nodeId": "node"}); got != want || caller.calls != 1 {
				t.Fatalf("got=%v want same error=%v calls=%d", got, want, caller.calls)
			}
		})
	}
	t.Run("dry-run", func(t *testing.T) {
		testseam.Swap(t, &os.Args, []string{"dws", "sheet"})
		caller := &helpersCoreCaller{format: "json", dry: true, result: textToolResult(`{"dry_run":true}`)}
		InitDepsForTest(t, caller)
		var stdout bytes.Buffer
		deps.Out.SetWriters(&stdout, nil)
		if err := callMCPToolSheetInfo(map[string]any{"nodeId": "node"}); err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || got["dry_run"] != true || got["executed"] != false || got["tool"] != "get_sheet" || caller.calls != 0 {
			t.Fatalf("dry-run changed: output=%q error=%v calls=%d", stdout.String(), err, caller.calls)
		}
	})
}
