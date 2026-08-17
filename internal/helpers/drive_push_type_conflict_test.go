package helpers

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDrivePushDryRun_typeConflictsFail(t *testing.T) {
	var out bytes.Buffer
	testseam.Swap(t, &deps, &Deps{Out: &Formatter{w: &out}})

	tests := []struct {
		name          string
		remoteFiles   map[string]*remoteFile
		remoteFolders map[string]string
		localDirs     []string
		localFiles    []localPushFile
	}{
		{
			name:          "remote file conflicts with local folder",
			remoteFiles:   map[string]*remoteFile{"conflict": {RelPath: "conflict", FileID: "REMOTE_FILE"}},
			remoteFolders: map[string]string{"": "ROOT"},
			localDirs:     []string{"conflict"},
		},
		{
			name:          "remote folder conflicts with local file",
			remoteFiles:   map[string]*remoteFile{},
			remoteFolders: map[string]string{"": "ROOT", "conflict": "REMOTE_FOLDER"},
			localFiles:    []localPushFile{{RelPath: "conflict", Size: 1}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out.Reset()
			if err := printDrivePushDryRun(ifExistsSkip, tt.remoteFiles, tt.remoteFolders, tt.localDirs, tt.localFiles); err != nil {
				t.Fatalf("printDrivePushDryRun: %v", err)
			}

			var got drivePushDryRunResult
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("decode dry-run result: %v\n%s", err, out.String())
			}
			if got.Plan.Summary.Failed != 1 || got.Plan.Summary.Uploaded != 0 {
				t.Fatalf("summary = %+v, want failed=1 uploaded=0", got.Plan.Summary)
			}
			if len(got.Plan.Items) != 1 || got.Plan.Items[0].Action != pushActionFailed || got.Plan.Items[0].Error == "" {
				t.Fatalf("items = %+v, want one failed item with an error", got.Plan.Items)
			}
		})
	}
}

func TestCrossPlatformCoverageDrivePush_typeConflictsFailBeforeWrites(t *testing.T) {
	tests := []struct {
		name       string
		prepare    func(*testing.T, string)
		remoteItem string
	}{
		{
			name: "remote file conflicts with local folder",
			prepare: func(t *testing.T, root string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(root, "conflict"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			remoteItem: `{"name":"conflict","type":"file","fileId":"REMOTE_FILE"}`,
		},
		{
			name: "remote folder conflicts with local file",
			prepare: func(t *testing.T, root string) {
				t.Helper()
				mustWrite(t, filepath.Join(root, "conflict"), "local")
			},
			remoteItem: `{"name":"conflict","type":"folder","fileId":"REMOTE_FOLDER"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.prepare(t, root)
			withNoopPut(t)

			caller := pushOKCaller(map[string]string{
				"ROOT": `{"result":{"items":[` + tt.remoteItem + `],"nextToken":""}}`,
			})
			err := runDriveCmd(t, caller, "push", "--local-folder", root, "--remote-folder", "ROOT")
			var failure *drivePushFailure
			if !errors.As(err, &failure) || failure.failed != 1 {
				t.Fatalf("error = %T %v, want drivePushFailure(failed=1)", err, err)
			}
			if calls := caller.callsFor("create_folder"); len(calls) != 0 {
				t.Fatalf("type conflict must not create folders, got %v", calls)
			}
			if calls := caller.callsFor("get_upload_info"); len(calls) != 0 {
				t.Fatalf("type conflict must not upload, got %v", calls)
			}
		})
	}
}
