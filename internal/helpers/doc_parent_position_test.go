package helpers

import (
	"errors"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// A parent flag that the public MCP cannot represent must never become a
// successful root/sibling insertion, regardless of the content carrier or mode.
func TestCrossPlatformCoverageDocInsertRejectsUnsupportedParentBeforeWrite(t *testing.T) {
	for _, input := range []struct {
		name string
		args []string
	}{
		{"paragraph", []string{"--content", "body"}},
		{"element", []string{"--element", `{"blockType":"paragraph","paragraph":{"text":"body"}}`}},
		{"single jsonml", []string{"--content-format", "jsonml", "--element", `["p",{},"body"]`}},
		{"jsonml sequence", []string{"--content-format", "jsonml", "--jsonml-nodes", `[["p",{},"body"]]`}},
		{"markdown", []string{"--content-format", "markdown", "--content", "body"}},
	} {
		for _, selector := range []string{"", "--suggestion", "--suggestion=false"} {
			t.Run(input.name+selector, func(t *testing.T) {
				caller := &docReadScopeCaller{text: `{"success":true,"newBlockIds":["new"],"suggestionId":"pending","applied":false}`}
				args := append([]string{"block", "insert", "--node", "n", "--parent-block", "cell", "--index", "1"}, input.args...)
				if selector != "" {
					args = append(args, selector)
				}
				err := executeDocSuggestionCommand(t, caller, args...)
				var classified *apperrors.Error
				if !errors.As(err, &classified) || classified.Category != apperrors.CategoryValidation || apperrors.ExitCode(err) != 3 {
					t.Errorf("error=%#v; want validation/exit3", err)
				}
				if err == nil || !strings.Contains(err.Error(), "--parent-block") || len(caller.calls) != 0 {
					t.Fatalf("error=%v calls=%#v; want unsupported parent rejected before any RPC", err, caller.calls)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageDocInsertParentCannotFallBackToReference(t *testing.T) {
	for _, parent := range []string{"cell", ""} {
		caller := &docReadScopeCaller{text: `{"success":true}`}
		err := executeDocSuggestionCommand(t, caller, "block", "insert", "--node", "n", "--content", "body",
			"--parent-block", parent, "--ref-block", "sibling", "--where", "before")
		if apperrors.ExitCode(err) != 3 || len(caller.calls) != 0 {
			t.Fatalf("parent=%q error=%v calls=%#v; want explicit unsupported flag rejected without fallback", parent, err, caller.calls)
		}
	}
}
