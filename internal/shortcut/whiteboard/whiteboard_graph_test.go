package whiteboard

import (
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageWhiteboardInvalidGraphStopsShortcutBeforeMCP(t *testing.T) {
	for _, tc := range []struct{ nodes, issue string }{
		{`[{"id":"a","type":"shape"},{"id":"a","type":"shape"}]`, "重复节点 id"},
		{`[{"id":"a","type":"shape","parentId":"absent"}]`, "referenceNotFound"},
		{`[{"id":"a","type":"group","parentId":"b"},{"id":"b","type":"group","parentId":"a"}]`, "parentCycle"},
		{`[{"id":"a","type":"shape","width":"100"}]`, "invalidValue"},
		{`[{"id":"title","type":"shape","text":{"blocks":[{"type":"paragraph","runs":[{"text":"redacted","marks":{"fontColor":"#FFFFFF"}}]}]}}]`, "/marks/fontColor"},
		{`[{"id":"root","type":"shape","text":{"blocks":[{"type":"paragraph","runs":[{"text":"redacted","fontSize":16}]}]}}]`, "/runs/0/fontSize"},
		{`[{"id":"n","type":"shape","text":{"blocks":[{"type":"paragraph","runs":[{"text":""}]}],"padding":[1,2,3,4]}}]`, "/text/padding"},
	} {
		for _, declaration := range []shortcut.Shortcut{Update, Diff} {
			t.Run(declaration.Command+tc.issue, func(t *testing.T) {
				caller := &whiteboardCoverageCaller{responses: map[string][]string{}}
				source := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":` + tc.nodes + `}}`
				err := runWhiteboardCoverage(t, declaration, caller, "", "--node", "doc", "--part-id", "part", "--source", source, "--yes")
				if err == nil || apperrors.ExitCode(err) != 3 || !strings.Contains(err.Error(), tc.issue) || len(caller.calls) != 0 {
					t.Fatalf("invalid graph was not rejected locally: err=%v calls=%#v", err, caller.calls)
				}
			})
		}
	}
}
