package helpers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// These are minimized, redacted structures recovered from historical requests,
// not complete production replays. The padding value is a protocol boundary case.
func TestCrossPlatformCoverageWhiteboardHistoricalTextStopsNativeCommands(t *testing.T) {
	for _, tc := range []struct{ run, padding, path string }{
		{`{"text":"redacted","marks":{"fontColor":"#FFFFFF"}}`, `0`, "/marks/fontColor"},
		{`{"text":"redacted","marks":{"bold":true},"fontSize":16}`, `0`, "/runs/0/fontSize"},
		{`{"text":"redacted"}`, `[1,2,3,4]`, "/text/padding"},
	} {
		for _, operation := range []string{"render", "create-with-content", "update"} {
			for _, dry := range []bool{false, true} {
				t.Run(operation+tc.path+map[bool]string{false: "/write", true: "/dry"}[dry], func(t *testing.T) {
					caller := &whiteboardTestCaller{format: "json", dry: dry}
					installWhiteboardTestCaller(t, caller)
					source := `{"source":{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[{"id":"title","type":"shape","x":0,"y":0,"width":100,"height":50,"geometry":"dml:roundRect","text":{"blocks":[{"type":"paragraph","runs":[` + tc.run + `]}],"padding":` + tc.padding + `}}]}}`
					cmd := newWhiteboardCommand()
					cmd.PersistentFlags().Bool("yes", false, "")
					cmd.PersistentFlags().Bool("dry-run", false, "")
					artifact := filepath.Join(t.TempDir(), "preview.svg")
					args := []string{operation, "--source", writeWhiteboardFixture(t, source), "--yes"}
					switch operation {
					case "render":
						args = append(args, "--output", artifact)
					case "create-with-content":
						args = append(args, "--name", "Regression", "--request-id", "wb-regression")
					case "update":
						args = append(args, "--node", "doc", "--part-id", "part")
					}
					if dry {
						args = append(args, "--dry-run")
					}
					cmd.SetArgs(args)
					err := corecmd.ExecuteForTest(cmd)
					if err == nil || apperrors.ExitCode(err) != 3 || !strings.Contains(err.Error(), tc.path) || len(caller.calls) != 0 {
						t.Fatalf("invalid source escaped: err=%v calls=%#v", err, caller.calls)
					}
					if _, err := os.Stat(artifact); !os.IsNotExist(err) {
						t.Fatalf("invalid source wrote preview: %v", err)
					}
				})
			}
		}
	}
}
