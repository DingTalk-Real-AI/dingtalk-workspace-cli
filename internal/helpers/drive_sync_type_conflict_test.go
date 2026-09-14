package helpers

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

type driveSyncTypeConflictFixture struct {
	name     string
	setup    func(t *testing.T, root string)
	listings map[string]string
}

func driveSyncTypeConflictFixtures() []driveSyncTypeConflictFixture {
	return []driveSyncTypeConflictFixture{
		{
			name: "local folder conflicts with remote file",
			setup: func(t *testing.T, root string) {
				t.Helper()
				mustWrite(t, filepath.Join(root, "conflict", "local.txt"), "local")
			},
			listings: map[string]string{
				"ROOT": `{"result":{"items":[{"name":"conflict","type":"file","fileId":"REMOTE_FILE","modifyTime":1}],"nextToken":""}}`,
			},
		},
		{
			name: "local file conflicts with remote folder",
			setup: func(t *testing.T, root string) {
				t.Helper()
				mustWrite(t, filepath.Join(root, "conflict"), "local")
			},
			listings: map[string]string{
				"ROOT":          `{"result":{"items":[{"name":"conflict","type":"folder","fileId":"REMOTE_FOLDER"}],"nextToken":""}}`,
				"REMOTE_FOLDER": `{"result":{"items":[{"name":"remote.txt","type":"file","fileId":"REMOTE_CHILD","modifyTime":1}],"nextToken":""}}`,
			},
		},
	}
}

func TestCrossPlatformCoverageDriveSync_typeConflictsFailClosedBeforeWrites(t *testing.T) {
	for _, tc := range driveSyncTypeConflictFixtures() {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.setup(t, root)
			caller := syncCaller(tc.listings)

			transferCalls := captureDriveSyncTransfers(t)

			err := runDriveCmd(t, caller, "sync", "--local-folder", root,
				"--remote-folder", "ROOT", "--quick")
			var syncFailure *driveSyncFailure
			if !errors.As(err, &syncFailure) || syncFailure.failed != 1 {
				t.Fatalf("type conflict error = %v, want one failed item", err)
			}
			assertDriveSyncNoWriteCalls(t, caller, transferCalls)
		})
	}
}

func TestCrossPlatformCoverageDriveSync_dryRunReportsTypeConflictsWithoutWrites(t *testing.T) {
	for _, tc := range driveSyncTypeConflictFixtures() {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.setup(t, root)
			caller := syncCaller(tc.listings)
			caller.dryRun = true

			transferCalls := captureDriveSyncTransfers(t)

			var out bytes.Buffer
			testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: &out}})
			testseam.Swap(t, &os.Args, []string{"dws", "drive", "sync"})

			cmd := findDriveSubcommand(t, "sync")
			mustSetFlags(t, cmd, map[string]string{
				"local-folder":  root,
				"remote-folder": "ROOT",
				"quick":         "true",
			})
			if err := cmd.RunE(cmd, nil); err != nil {
				t.Fatalf("dry-run type conflict: %v", err)
			}

			text := out.String()
			if !strings.Contains(text, `"failed": 1`) ||
				!strings.Contains(text, `"action": "failed"`) ||
				!strings.Contains(text, `"rel_path": "conflict"`) {
				t.Fatalf("dry-run must report the type conflict as failed: %s", text)
			}
			assertDriveSyncNoWriteCalls(t, caller, transferCalls)
		})
	}
}

type driveSyncTransferCalls struct {
	get int
	put int
}

func captureDriveSyncTransfers(t *testing.T) *driveSyncTransferCalls {
	t.Helper()
	calls := &driveSyncTransferCalls{}
	swapPullDownloadPath(t, func(context.Context, string, map[string]string, string) error {
		calls.get++
		return nil
	})
	testseam.Swap(t, &pushPutOpenedFile, func(context.Context, string, map[string]string, *os.File, int64) error {
		calls.put++
		return nil
	})
	return calls
}

func assertDriveSyncNoWriteCalls(t *testing.T, caller *driveScriptCaller, transferCalls *driveSyncTransferCalls) {
	t.Helper()
	for _, tool := range []string{"create_folder", "download_file", "get_upload_info", "commit_upload"} {
		if calls := caller.callsFor(tool); len(calls) != 0 {
			t.Errorf("type conflict must not call %s, got %v", tool, calls)
		}
	}
	if transferCalls.get != 0 || transferCalls.put != 0 {
		t.Errorf("type conflict must not transfer HTTP content, GET=%d PUT=%d", transferCalls.get, transferCalls.put)
	}
}
