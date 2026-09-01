// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageContractCommandTreeIncludesWukongCommands(t *testing.T) {
	root := newContractCommand()
	if root.Hidden {
		t.Fatal("contract root must be visible after Agent Schema exposure")
	}
	if err := cobracmd.ValidateGroupTree(root); err != nil {
		t.Fatalf("contract group declarations: %v", err)
	}

	paths := []string{
		"record list", "record get", "record quantity-by-type", "record create",
		"import batch", "import batch-result", "process-templates", "file-directories",
		"draft", "review benefit", "review create", "review analysis", "review result",
		"account create", "account update", "account get", "account list", "account delete",
		"archive",
		"project add", "project delete", "project update", "project set-status", "project list",
		"project digests", "project detail", "project export", "project import-template",
		"project import", "project import-result",
		"subject add", "subject list", "subject detail", "subject update", "subject delete",
		"subject batch-delete", "subject sort", "subject detect-risk", "subject base-info",
		"subject auto-fill", "subject export", "subject import-template", "subject import",
		"subject import-result",
	}
	for _, path := range paths {
		cmd, remaining, err := root.Find(splitCommandPath(path))
		if err != nil || len(remaining) != 0 || cmd == root || !cmd.Runnable() {
			t.Errorf("find %q: command=%v remaining=%v runnable=%v err=%v", path, cmd, remaining, cmd != nil && cmd.Runnable(), err)
		}
	}

	detail, remaining, err := root.Find([]string{"record", "detail"})
	if err != nil || len(remaining) != 0 || detail.Name() != "get" {
		t.Fatalf("record detail alias: command=%v remaining=%v err=%v", detail, remaining, err)
	}
	directories, remaining, err := root.Find([]string{"directories"})
	if err != nil || len(remaining) != 0 || directories.Name() != "file-directories" {
		t.Fatalf("directories alias: command=%v remaining=%v err=%v", directories, remaining, err)
	}
}

func TestCrossPlatformCoverageContractRecordListMapsISOTimeAndScope(t *testing.T) {
	caller := &contractDefectCaller{}
	_, err := executeContractDefectCommand(t, caller, newContractCommand,
		"record", "list",
		"--start", "2026-03-10T00:00:00+08:00",
		"--end", "2026-03-11T00:00:00+08:00",
		"--status", "approving, signing",
		"--type", "participation")
	if err != nil {
		t.Fatalf("record list: %v", err)
	}
	call := onlyContractCall(t, caller)
	if call.toolName != "queryContracts" {
		t.Fatalf("tool = %q, want queryContracts", call.toolName)
	}
	if got := call.args["type"]; got != "participation" {
		t.Fatalf("type = %#v, want participation", got)
	}
	if got := call.args["contractStatusList"]; !reflect.DeepEqual(got, []string{"approving", "signing"}) {
		t.Fatalf("contractStatusList = %#v", got)
	}
	if got := call.args["createStartTime"]; got != int64(1773072000000) {
		t.Fatalf("createStartTime = %#v", got)
	}
	if got := call.args["createEndTime"]; got != int64(1773158400000) {
		t.Fatalf("createEndTime = %#v", got)
	}
}

func TestCrossPlatformCoverageContractRecordListRejectsInvalidInputBeforeCall(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "scope", args: []string{"record", "list", "--type", "mine"}},
		{name: "range", args: []string{"record", "list", "--start", "2026-03-11", "--end", "2026-03-10"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &contractDefectCaller{}
			if _, err := executeContractDefectCommand(t, caller, newContractCommand, test.args...); err == nil {
				t.Fatal("expected validation error")
			}
			if len(caller.calls)+len(caller.readCalls) != 0 {
				t.Fatalf("calls after validation failure: mutation=%#v read=%#v", caller.calls, caller.readCalls)
			}
		})
	}
}

func TestCrossPlatformCoverageContractDeleteCommandsRequireConfirmation(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		tool       string
		requestKey string
		wantArgs   map[string]any
	}{
		{
			name:       "project delete",
			args:       []string{"project", "delete", "--project-ids", "1001, 1002"},
			tool:       "deleteProject",
			requestKey: "DeleteProjectOpenRequest",
			wantArgs:   map[string]any{"projectIds": []int64{1001, 1002}},
		},
		{
			name:       "account delete",
			args:       []string{"account", "delete", "--account-id", "12345"},
			tool:       "deleteAccountEntryInfo",
			requestKey: "DeleteContractAccountEntryRequest",
			wantArgs:   map[string]any{"accountEntryId": int64(12345)},
		},
		{
			name:       "subject delete",
			args:       []string{"subject", "delete", "--subject-id", "2001"},
			tool:       "deleteSubject",
			requestKey: "DeleteSubjectOpenRequest",
			wantArgs:   map[string]any{"subjectId": int64(2001)},
		},
		{
			name:       "subject batch-delete",
			args:       []string{"subject", "batch-delete", "--subject-ids", "2001, 2002, 2003"},
			tool:       "batchDeleteSubject",
			requestKey: "BatchDeleteSubjectOpenRequest",
			wantArgs:   map[string]any{"subjectIdList": []int64{2001, 2002, 2003}},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// 1) Without --yes: typed confirmation_required error and zero MCP calls.
			caller := &contractDefectCaller{}
			_, err := executeContractDefectCommand(t, caller, newContractCommand, tc.args...)
			if err == nil {
				t.Fatal("delete without --yes should fail")
			}
			var appErr *apperrors.Error
			if !errors.As(err, &appErr) || appErr.Reason != "confirmation_required" {
				t.Fatalf("expected typed confirmation_required error, got: %v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("expected zero MCP calls without confirmation, got %d: %+v", len(caller.calls), caller.calls)
			}

			// 2) With --yes: exactly one call with correct tool and precise arguments.
			confirmed := &contractDefectCaller{}
			_, err = executeContractDefectCommand(t, confirmed, newContractCommand, append(append([]string(nil), tc.args...), "--yes")...)
			if err != nil {
				t.Fatalf("delete with --yes: %v", err)
			}
			call := onlyContractCall(t, confirmed)
			if call.toolName != tc.tool {
				t.Fatalf("tool = %q, want %q", call.toolName, tc.tool)
			}
			request, ok := call.args[tc.requestKey].(map[string]any)
			if !ok {
				t.Fatalf("%s = %#v", tc.requestKey, call.args[tc.requestKey])
			}
			if !reflect.DeepEqual(request, tc.wantArgs) {
				t.Fatalf("request = %#v, want %#v", request, tc.wantArgs)
			}
		})
	}
}

func TestCrossPlatformCoverageContractSubjectAddWrapsJSONPayload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subject.json")
	if err := os.WriteFile(path, []byte(`{"partyType":"other","name":"示例公司"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	caller := &contractDefectCaller{}
	_, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "add", "--file", path)
	if err != nil {
		t.Fatalf("subject add: %v", err)
	}
	call := onlyContractCall(t, caller)
	request, ok := call.args["AddSubjectOpenRequest"].(map[string]any)
	if !ok || request["partyType"] != "other" || request["name"] != "示例公司" {
		t.Fatalf("AddSubjectOpenRequest = %#v", call.args["AddSubjectOpenRequest"])
	}
}

func splitCommandPath(path string) []string {
	return strings.Fields(path)
}

func onlyContractCall(t *testing.T, caller *contractDefectCaller) guardedMutationCall {
	t.Helper()
	all := append(append([]guardedMutationCall(nil), caller.calls...), caller.readCalls...)
	if len(all) != 1 {
		t.Fatalf("contract calls = %#v, read calls = %#v; want exactly one", caller.calls, caller.readCalls)
	}
	if all[0].productID != "contract" {
		t.Fatalf("product = %q, want contract", all[0].productID)
	}
	return all[0]
}
