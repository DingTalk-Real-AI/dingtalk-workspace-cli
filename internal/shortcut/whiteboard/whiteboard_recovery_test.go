package whiteboard

import (
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageWhiteboardRecoveryAtShortcutBoundaries(t *testing.T) {
	for _, tc := range []struct {
		declaration shortcut.Shortcut
		tool        string
		args        []string
	}{
		{Query, toolQuery, []string{"--node", "doc", "--part-id", "part"}},
		{Diff, toolQuery, []string{"--node", "doc", "--part-id", "part", "--source", `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n","type":"shape","x":0,"y":0,"width":10,"height":10,"geometry":"dml:rect"}]}}`}},
		{Update, toolUpdate, []string{"--node", "doc", "--part-id", "part", "--source", `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"n","type":"shape","x":0,"y":0,"width":10,"height":10,"geometry":"dml:rect"}]}}`, "--yes"}},
	} {
		t.Run(tc.declaration.Command, func(t *testing.T) {
			caller := &whiteboardCoverageCaller{responses: map[string][]string{tc.tool: {`{"success":false,"errorCode":"SCHEMA_INVALID","errorMsg":"invalid"}`}}}
			err := runWhiteboardCoverage(t, tc.declaration, caller, "", tc.args...)
			var failure *helpers.CLIError
			if !errors.As(err, &failure) || failure.ServerCode != "SCHEMA_INVALID" || !strings.Contains(failure.Suggestion, "OpenNodes") || len(caller.calls) != 1 {
				t.Fatalf("lost domain recovery: %v calls=%#v", err, caller.calls)
			}
		})
	}
}
