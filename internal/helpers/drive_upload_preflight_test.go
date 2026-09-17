package helpers

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageUploadDocSpaceFileDataFirstCallMetadata(t *testing.T) {
	for _, target := range []struct {
		name, folder, overwrite, targetKey, targetID string
	}{
		{name: "root"},
		{name: "folder", folder: "folder-test", targetKey: "folderId", targetID: "folder-test"},
		{name: "overwrite", overwrite: "node-test", targetKey: "overwriteNodeId", targetID: "node-test"},
	} {
		t.Run(target.name, func(t *testing.T) {
			request := DocSpaceUploadRequest{
				FilePath: "local.txt", FileName: "renamed.txt", FileSize: 7,
				WorkspaceID: "workspace-test", FolderID: target.folder, OverwriteNode: target.overwrite,
			}
			// Deliberately independent of docFileUploadInfoArgs: a regression in
			// the shared builder must not change both actual and expected values.
			wantArgs := map[string]any{
				"name": "renamed.txt", "fileSize": float64(7), "workspaceId": "workspace-test",
			}
			if target.targetKey != "" {
				wantArgs[target.targetKey] = target.targetID
			}
			caller := &scriptedToolCaller{steps: []scriptedToolStep{
				{text: `{"resourceUrl":"https://upload.example.test/object","uploadKey":"key-test"}`},
				{text: `{"dentryUuid":"node-created"}`},
			}}
			installScriptedCaller(t, caller)
			putCalls := 0
			testseam.Swap(t, &httpPutFile, func(_ context.Context, url string, _ map[string]string, path string, size int64) error {
				putCalls++
				if caller.calls != 1 || caller.serverLog[0] != "doc" || caller.toolLog[0] != "get_file_upload_info" {
					t.Fatalf("calls before PUT = %v / %v, want only doc/get_file_upload_info", caller.serverLog, caller.toolLog)
				}
				if !reflect.DeepEqual(caller.argsLog[0], wantArgs) {
					t.Fatalf("credential args before PUT = %#v, want %#v", caller.argsLog[0], wantArgs)
				}
				if url != "https://upload.example.test/object" || path != request.FilePath || size != request.FileSize {
					t.Fatalf("PUT arguments = %q %q %d", url, path, size)
				}
				return nil
			})
			result, err := UploadDocSpaceFileData(context.Background(), request)
			if err != nil || result["dentryUuid"] != "node-created" || putCalls != 1 || caller.calls != 2 {
				t.Fatalf("result=%v error=%v PUTs=%d calls=%v", result, err, putCalls, caller.toolLog)
			}
			if caller.serverLog[1] != "doc" || caller.toolLog[1] != "commit_uploaded_file" {
				t.Fatalf("commit route = %v / %v", caller.serverLog, caller.toolLog)
			}
			wantArgs["uploadKey"] = "key-test"
			if !reflect.DeepEqual(caller.argsLog[1], wantArgs) {
				t.Fatalf("commit metadata drifted from credential request: got %#v, want %#v", caller.argsLog[1], wantArgs)
			}
		})
	}
}

func TestCrossPlatformCoverageUploadDocSpaceFileDataDelegationBeforePut(t *testing.T) {
	for _, target := range []struct {
		name, folder, overwrite, nodeID string
	}{
		{name: "root", nodeID: "workspace-test"},
		{name: "folder", folder: "folder-test", nodeID: "folder-test"},
		{name: "overwrite", overwrite: "node-test", nodeID: "node-test"},
	} {
		for _, decision := range []struct {
			name, response string
			allowed        bool
		}{
			{name: "denied", response: `{"allowed":false,"denialReason":"NO_PERM","denialMessage":"test upload denied"}`},
			{name: "allowed", response: `{"allowed":true}`, allowed: true},
		} {
			t.Run(target.name+"/"+decision.name, func(t *testing.T) {
				inner := &docUploadParityInner{checkResText: decision.response}
				testseam.Protect(t, &deps)
				InitDeps(newDocDelegationAuthDecorator(inner))
				request := DocSpaceUploadRequest{
					FilePath: "local.txt", FileName: "renamed.txt", FileSize: 7,
					WorkspaceID: "workspace-test", FolderID: target.folder, OverwriteNode: target.overwrite,
				}
				wantCheck := map[string]any{
					"userId": "u-principal", "mcpToolKey": "doc.get_file_upload_info", "nodeId": target.nodeID,
					"options": map[string]any{"uploadActionParam": map[string]any{"fileName": "renamed.txt", "fileSize": int64(7)}},
				}
				assertFirstCheck := func() {
					t.Helper()
					if len(inner.calls) == 0 || inner.calls[0].server != capabilityServerID || inner.calls[0].tool != checkCapTool || !reflect.DeepEqual(inner.calls[0].args, wantCheck) {
						t.Fatalf("first capability check = %#v, want %#v", inner.calls, wantCheck)
					}
				}
				putCalls := 0
				testseam.Swap(t, &httpPutFile, func(context.Context, string, map[string]string, string, int64) error {
					putCalls++
					if !decision.allowed {
						t.Fatal("HTTP PUT reached after delegation denial")
					}
					assertFirstCheck()
					if len(inner.calls) != 2 || inner.calls[1].server != "doc" || inner.calls[1].tool != "get_file_upload_info" {
						t.Fatalf("calls before PUT = %#v, want check then credentials only", inner.calls)
					}
					return nil
				})
				result, err := UploadDocSpaceFileData(context.Background(), request)
				assertFirstCheck()
				if !decision.allowed {
					if err == nil || !strings.HasPrefix(err.Error(), "[DELEGATION_AUTH_DENIED]") || result != nil || putCalls != 0 || len(inner.calls) != 1 {
						t.Fatalf("denial must stop before credentials/PUT/commit: result=%v err=%v PUTs=%d calls=%#v", result, err, putCalls, inner.calls)
					}
					return
				}
				if err != nil || result["dentryUuid"] != "node-1" || putCalls != 1 || len(inner.calls) != 4 || inner.calls[2].tool != checkCapTool || inner.calls[3].server != "doc" || inner.calls[3].tool != "commit_uploaded_file" {
					t.Fatalf("allowed upload must execute once: result=%v err=%v PUTs=%d calls=%#v", result, err, putCalls, inner.calls)
				}
			})
		}
	}
}
