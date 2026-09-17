// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeTempJSON writes a JSON file to a temp dir and returns its path.
func writeTempJSON(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestCrossPlatformCoverageContractRecordCommands covers all record RunE branches.
func TestCrossPlatformCoverageContractRecordCommands(t *testing.T) {
	// record get: missing --contract-id
	caller := &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"record", "get"); err == nil {
		t.Fatal("record get without --contract-id should fail")
	}
	if len(caller.calls) != 0 {
		t.Fatalf("expected zero calls, got %d", len(caller.calls))
	}

	// record get: success
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"record", "get", "--contract-id", "c_xxx"); err != nil {
		t.Fatalf("record get: %v", err)
	}
	call := onlyContractCall(t, caller)
	if call.toolName != "queryContractDetails" {
		t.Fatalf("tool = %q, want queryContractDetails", call.toolName)
	}
	if call.args["contractId"] != "c_xxx" {
		t.Fatalf("contractId = %#v", call.args["contractId"])
	}

	// record quantity-by-type: invalid type
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"record", "quantity-by-type", "--type", "bogus"); err == nil {
		t.Fatal("record quantity-by-type with invalid type should fail")
	}

	// record quantity-by-type: success
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"record", "quantity-by-type", "--type", "department"); err != nil {
		t.Fatalf("record quantity-by-type: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "queryContractQuantityByType" {
		t.Fatalf("tool = %q, want queryContractQuantityByType", call.toolName)
	}
	if call.args["type"] != "department" {
		t.Fatalf("type = %#v", call.args["type"])
	}

	// record create: success
	path := writeTempJSON(t, "contract.json", `{"name":"test","effectiveStatus":"effective"}`)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"record", "create", "--file", path); err != nil {
		t.Fatalf("record create: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "createContract" {
		t.Fatalf("tool = %q, want createContract", call.toolName)
	}
	req, ok := call.args["ImportContractInfoRequest"].(map[string]any)
	if !ok || req["name"] != "test" {
		t.Fatalf("ImportContractInfoRequest = %#v", call.args["ImportContractInfoRequest"])
	}

	// record create: readContractJSONPayload error (missing --file)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"record", "create"); err == nil {
		t.Fatal("record create without --file should fail")
	}
}

// TestCrossPlatformCoverageContractImportCommands covers import batch + batch-result.
func TestCrossPlatformCoverageContractImportCommands(t *testing.T) {
	// import batch: missing --file-id
	caller := &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"import", "batch", "--space-id", "7890"); err == nil {
		t.Fatal("import batch without --file-id should fail")
	}

	// import batch: missing --space-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"import", "batch", "--file-id", "123456"); err == nil {
		t.Fatal("import batch without --space-id should fail")
	}

	// import batch: success
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"import", "batch", "--file-id", "123456", "-s", "7890"); err != nil {
		t.Fatalf("import batch: %v", err)
	}
	call := onlyContractCall(t, caller)
	if call.toolName != "batchImportContractAsync" {
		t.Fatalf("tool = %q, want batchImportContractAsync", call.toolName)
	}
	if call.args["fileId"] != "123456" || call.args["spaceId"] != "7890" {
		t.Fatalf("args = %#v", call.args)
	}

	// import batch-result: missing --task-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"import", "batch-result"); err == nil {
		t.Fatal("import batch-result without --task-id should fail")
	}

	// import batch-result: success
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"import", "batch-result", "--task-id", "task_xxx"); err != nil {
		t.Fatalf("import batch-result: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "getBatchImportContractResult" {
		t.Fatalf("tool = %q, want getBatchImportContractResult", call.toolName)
	}
	if call.args["taskId"] != "task_xxx" {
		t.Fatalf("taskId = %#v", call.args["taskId"])
	}
}

// TestCrossPlatformCoverageContractDraftCommand covers draft error and success branches.
func TestCrossPlatformCoverageContractDraftCommand(t *testing.T) {
	// draft: missing --task-uuids
	caller := &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"draft"); err == nil {
		t.Fatal("draft without --task-uuids should fail")
	}

	// draft: empty CSV (only commas)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"draft", "--task-uuids", ", ,"); err == nil {
		t.Fatal("draft with empty CSV should fail")
	}

	// draft: missing template (neither --template-url nor --template-content)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"draft", "--task-uuids", "uuid1"); err == nil {
		t.Fatal("draft without template should fail")
	}

	// draft: success with --template-url
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"draft", "--task-uuids", "uuid1,uuid2", "--template-url", "https://example.com/tpl.docx"); err != nil {
		t.Fatalf("draft with template-url: %v", err)
	}
	call := onlyContractCall(t, caller)
	if call.toolName != "draft_contract_by_minutes" {
		t.Fatalf("tool = %q, want draft_contract_by_minutes", call.toolName)
	}
	uuids, _ := call.args["taskUuids"].([]string)
	if !reflect.DeepEqual(uuids, []string{"uuid1", "uuid2"}) {
		t.Fatalf("taskUuids = %#v", call.args["taskUuids"])
	}
	if call.args["templateUrl"] != "https://example.com/tpl.docx" {
		t.Fatalf("templateUrl = %#v", call.args["templateUrl"])
	}

	// draft: success with --template-content
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"draft", "--task-uuids", "uuid1", "--template-content", "合同正文"); err != nil {
		t.Fatalf("draft with template-content: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.args["templateContent"] != "合同正文" {
		t.Fatalf("templateContent = %#v", call.args["templateContent"])
	}
}

// TestCrossPlatformCoverageContractReviewCommands covers review create/analysis/result.
func TestCrossPlatformCoverageContractReviewCommands(t *testing.T) {
	// review benefit: success
	caller := &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"review", "benefit"); err != nil {
		t.Fatalf("review benefit: %v", err)
	}
	call := onlyContractCall(t, caller)
	if call.toolName != "queryContractReviewBenefit" {
		t.Fatalf("tool = %q, want queryContractReviewBenefit", call.toolName)
	}

	// review create: missing --file
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"review", "create"); err == nil {
		t.Fatal("review create without --file should fail")
	}

	// review create: file open error (nonexistent file)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"review", "create", "--file", "/nonexistent/path/file.json"); err == nil {
		t.Fatal("review create with nonexistent file should fail")
	}

	// review create: JSON parse error
	badPath := writeTempJSON(t, "bad.json", `{invalid`)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"review", "create", "--file", badPath); err == nil {
		t.Fatal("review create with invalid JSON should fail")
	}

	// review create: success
	goodPath := writeTempJSON(t, "review.json", `{"source":"OPEN_CLAW","reviewType":"AI_REVIEW"}`)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"review", "create", "--file", goodPath); err != nil {
		t.Fatalf("review create: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "createContractReviewTask" {
		t.Fatalf("tool = %q, want createContractReviewTask", call.toolName)
	}
	req, ok := call.args["IntelligentContractReviewClientRequest"].(map[string]any)
	if !ok || req["source"] != "OPEN_CLAW" {
		t.Fatalf("request = %#v", call.args["IntelligentContractReviewClientRequest"])
	}

	// review analysis: missing --file
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"review", "analysis"); err == nil {
		t.Fatal("review analysis without --file should fail")
	}

	// review analysis: file open error
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"review", "analysis", "--file", "/nonexistent/file.json"); err == nil {
		t.Fatal("review analysis with nonexistent file should fail")
	}

	// review analysis: JSON parse error
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"review", "analysis", "--file", badPath); err == nil {
		t.Fatal("review analysis with invalid JSON should fail")
	}

	// review analysis: success
	analysisPath := writeTempJSON(t, "analysis.json", `{"fileInfo":{"fileId":"xxx"}}`)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"review", "analysis", "--file", analysisPath); err != nil {
		t.Fatalf("review analysis: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "contractAnalysis" {
		t.Fatalf("tool = %q, want contractAnalysis", call.toolName)
	}

	// review result: missing --task-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"review", "result", "--review-type", "AI_REVIEW"); err == nil {
		t.Fatal("review result without --task-id should fail")
	}

	// review result: missing --review-type
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"review", "result", "--task-id", "task_xxx"); err == nil {
		t.Fatal("review result without --review-type should fail")
	}

	// review result: success
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"review", "result", "--task-id", "task_xxx", "--review-type", "AI_REVIEW"); err != nil {
		t.Fatalf("review result: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "queryContractReviewResult" {
		t.Fatalf("tool = %q, want queryContractReviewResult", call.toolName)
	}
}

// TestCrossPlatformCoverageContractMiscCommands covers process-templates, file-directories, archive.
func TestCrossPlatformCoverageContractMiscCommands(t *testing.T) {
	// process-templates: success
	caller := &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"process-templates"); err != nil {
		t.Fatalf("process-templates: %v", err)
	}
	call := onlyContractCall(t, caller)
	if call.toolName != "queryContractProcessContent" {
		t.Fatalf("tool = %q, want queryContractProcessContent", call.toolName)
	}

	// file-directories: success
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"file-directories"); err != nil {
		t.Fatalf("file-directories: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "getAllFileDirectory" {
		t.Fatalf("tool = %q, want getAllFileDirectory", call.toolName)
	}

	// archive: success
	archivePath := writeTempJSON(t, "archive.json", `{"bizId":"abc","archiveTime":1700000000000}`)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"archive", "--file", archivePath, "--yes"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "contractOpenArchive" {
		t.Fatalf("tool = %q, want contractOpenArchive", call.toolName)
	}
	req, ok := call.args["ContractOpenArchiveRequest"].(map[string]any)
	if !ok || req["bizId"] != "abc" {
		t.Fatalf("ContractOpenArchiveRequest = %#v", call.args["ContractOpenArchiveRequest"])
	}

	// archive: readContractJSONPayload error (missing --file)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"archive", "--yes"); err == nil {
		t.Fatal("archive without --file should fail")
	}
}

// TestCrossPlatformCoverageContractAccountCommands covers account create/update/get/list.
func TestCrossPlatformCoverageContractAccountCommands(t *testing.T) {
	// account create: success
	path := writeTempJSON(t, "acct.json", `{"contractId":123,"amount":"100.00","transactionNo":"tx1","executionDate":1700000000000,"status":"finished"}`)
	caller := &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"account", "create", "--file", path); err != nil {
		t.Fatalf("account create: %v", err)
	}
	call := onlyContractCall(t, caller)
	if call.toolName != "createAccountInfo" {
		t.Fatalf("tool = %q, want createAccountInfo", call.toolName)
	}

	// account create: missing --file
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"account", "create"); err == nil {
		t.Fatal("account create without --file should fail")
	}

	// account update: success
	updatePath := writeTempJSON(t, "acct_upd.json", `{"accountEntryId":456,"contractId":123,"amount":"200.00","transactionNo":"tx2","executionDate":1700000000000,"status":"finished"}`)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"account", "update", "--file", updatePath); err != nil {
		t.Fatalf("account update: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "updateAccountInfo" {
		t.Fatalf("tool = %q, want updateAccountInfo", call.toolName)
	}

	// account update: missing --file
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"account", "update"); err == nil {
		t.Fatal("account update without --file should fail")
	}

	// account get: missing --account-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"account", "get"); err == nil {
		t.Fatal("account get without --account-id should fail")
	}

	// account get: success
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"account", "get", "--account-id", "12345"); err != nil {
		t.Fatalf("account get: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "getAccountEntryInfo" {
		t.Fatalf("tool = %q, want getAccountEntryInfo", call.toolName)
	}

	// account list: success with all optional flags
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"account", "list",
		"--scope", "self",
		"--query-status", "pay",
		"--amount-type", "payment_party_other",
		"--status", "finished",
		"--source", "contract",
		"--contract-code", "C001",
		"--contract-name", "采购合同",
		"--transaction-no", "tx001",
		"--exec-start", "2026-01-01T00:00:00+08:00",
		"--exec-end", "2026-12-31T23:59:59+08:00",
		"--page", "2",
		"--page-size", "50"); err != nil {
		t.Fatalf("account list: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "listAccountInfo" {
		t.Fatalf("tool = %q, want listAccountInfo", call.toolName)
	}
	req, ok := call.args["QueryContractAccountListRequest"].(map[string]any)
	if !ok {
		t.Fatalf("QueryContractAccountListRequest = %#v", call.args["QueryContractAccountListRequest"])
	}
	if req["scope"] != "self" || req["queryStatus"] != "pay" || req["amountType"] != "payment_party_other" {
		t.Fatalf("account list req missing optional fields: %#v", req)
	}
	if req["status"] != "finished" || req["source"] != "contract" {
		t.Fatalf("account list req missing optional fields: %#v", req)
	}
	if req["contractCode"] != "C001" || req["contractName"] != "采购合同" || req["transactionNo"] != "tx001" {
		t.Fatalf("account list req missing optional fields: %#v", req)
	}
	if req["currentPage"] != 2 || req["pageSize"] != 50 {
		t.Fatalf("account list pagination: %#v", req)
	}

	// account list: success with no optional flags
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"account", "list"); err != nil {
		t.Fatalf("account list minimal: %v", err)
	}
}

// TestCrossPlatformCoverageContractProjectCommands covers all project commands.
func TestCrossPlatformCoverageContractProjectCommands(t *testing.T) {
	// project add: missing --name
	caller := &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "add"); err == nil {
		t.Fatal("project add without --name should fail")
	}

	// project add: success with all optional flags
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "add",
		"--name", "2024采购项目",
		"--code", "PRJ-001",
		"--owners", "staff1,staff2",
		"--start-date", "2026-01-01T00:00:00+08:00",
		"--end-date", "2026-12-31T23:59:59+08:00",
		"--remark", "备注",
		"--contract-ids", "100,200",
		"--source", "contract"); err != nil {
		t.Fatalf("project add: %v", err)
	}
	call := onlyContractCall(t, caller)
	if call.toolName != "addProject" {
		t.Fatalf("tool = %q, want addProject", call.toolName)
	}
	req, ok := call.args["AddProjectOpenRequest"].(map[string]any)
	if !ok || req["name"] != "2024采购项目" || req["code"] != "PRJ-001" {
		t.Fatalf("project add req: %#v", req)
	}
	if owners, _ := req["ownerList"].([]string); !reflect.DeepEqual(owners, []string{"staff1", "staff2"}) {
		t.Fatalf("ownerList = %#v", req["ownerList"])
	}
	if ids, _ := req["contractIds"].([]int64); !reflect.DeepEqual(ids, []int64{100, 200}) {
		t.Fatalf("contractIds = %#v", req["contractIds"])
	}
	if req["source"] != "contract" {
		t.Fatalf("source = %#v", req["source"])
	}

	// project add: success minimal
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "add", "--name", "minimal"); err != nil {
		t.Fatalf("project add minimal: %v", err)
	}

	// project delete: missing --project-ids
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "delete"); err == nil {
		t.Fatal("project delete without --project-ids should fail")
	}

	// project delete: invalid integer CSV
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "delete", "--project-ids", "abc"); err == nil {
		t.Fatal("project delete with non-integer should fail")
	}

	// project update: missing --project-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "update", "--name", "test"); err == nil {
		t.Fatal("project update without --project-id should fail")
	}

	// project update: missing --name
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "update", "--project-id", "1001"); err == nil {
		t.Fatal("project update without --name should fail")
	}

	// project update: success with all optional flags
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "update",
		"--project-id", "1001",
		"--name", "更新名称",
		"--code", "C002",
		"--owners", "s1,s2",
		"--start-date", "2026-01-01T00:00:00+08:00",
		"--end-date", "2026-12-31T23:59:59+08:00",
		"--remark", "备注",
		"--contract-ids", "10,20"); err != nil {
		t.Fatalf("project update: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "updateProject" {
		t.Fatalf("tool = %q, want updateProject", call.toolName)
	}
	req, ok = call.args["UpdateProjectOpenRequest"].(map[string]any)
	if !ok || req["projectId"] != int64(1001) || req["name"] != "更新名称" {
		t.Fatalf("project update req: %#v", req)
	}
	if ids, _ := req["contractIds"].([]int64); !reflect.DeepEqual(ids, []int64{10, 20}) {
		t.Fatalf("contractIds = %#v", req["contractIds"])
	}

	// project update: success minimal
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "update", "--project-id", "1001", "--name", "minimal"); err != nil {
		t.Fatalf("project update minimal: %v", err)
	}

	// project set-status: missing --project-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "set-status", "--status", "active"); err == nil {
		t.Fatal("project set-status without --project-id should fail")
	}

	// project set-status: missing --status
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "set-status", "--project-id", "1001"); err == nil {
		t.Fatal("project set-status without --status should fail")
	}

	// project set-status: success
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "set-status", "--project-id", "1001", "--status", "active"); err != nil {
		t.Fatalf("project set-status: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "setProjectStatus" {
		t.Fatalf("tool = %q, want setProjectStatus", call.toolName)
	}

	// project list: missing --current-page
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "list", "--page-size", "20", "--scope", "all"); err == nil {
		t.Fatal("project list without --current-page should fail")
	}

	// project list: missing --page-size
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "list", "--current-page", "1", "--scope", "all"); err == nil {
		t.Fatal("project list without --page-size should fail")
	}

	// project list: missing --scope
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "list", "--current-page", "1", "--page-size", "20"); err == nil {
		t.Fatal("project list without --scope should fail")
	}

	// project list: success with all optional flags
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "list",
		"--current-page", "1", "--page-size", "20", "--scope", "all",
		"--name", "采购", "--code", "PRJ", "--owners", "s1",
		"--status", "active",
		"--start-date-left", "2026-01-01T00:00:00+08:00",
		"--start-date-right", "2026-06-30T23:59:59+08:00",
		"--end-date-left", "2026-01-01T00:00:00+08:00",
		"--end-date-right", "2026-12-31T23:59:59+08:00"); err != nil {
		t.Fatalf("project list: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "queryProjects" {
		t.Fatalf("tool = %q, want queryProjects", call.toolName)
	}

	// project list: success minimal
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "list", "--current-page", "1", "--page-size", "10", "--scope", "self"); err != nil {
		t.Fatalf("project list minimal: %v", err)
	}

	// project digests: missing --current-page
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "digests", "--page-size", "20", "--scope", "all"); err == nil {
		t.Fatal("project digests without --current-page should fail")
	}

	// project digests: missing --page-size
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "digests", "--current-page", "1", "--scope", "all"); err == nil {
		t.Fatal("project digests without --page-size should fail")
	}

	// project digests: missing --scope
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "digests", "--current-page", "1", "--page-size", "20"); err == nil {
		t.Fatal("project digests without --scope should fail")
	}

	// project digests: success with all optional flags
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "digests",
		"--current-page", "1", "--page-size", "20", "--scope", "all",
		"--name", "采购", "--code", "PRJ", "--owners", "s1",
		"--status", "active",
		"--start-date-left", "2026-01-01T00:00:00+08:00",
		"--start-date-right", "2026-06-30T23:59:59+08:00",
		"--end-date-left", "2026-01-01T00:00:00+08:00",
		"--end-date-right", "2026-12-31T23:59:59+08:00"); err != nil {
		t.Fatalf("project digests: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "queryProjectDigests" {
		t.Fatalf("tool = %q, want queryProjectDigests", call.toolName)
	}

	// project digests: success minimal
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "digests", "--current-page", "1", "--page-size", "10", "--scope", "self"); err != nil {
		t.Fatalf("project digests minimal: %v", err)
	}

	// project detail: missing --project-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "detail"); err == nil {
		t.Fatal("project detail without --project-id should fail")
	}

	// project detail: success
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "detail", "--project-id", "1001"); err != nil {
		t.Fatalf("project detail: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "queryProjectDetail" {
		t.Fatalf("tool = %q, want queryProjectDetail", call.toolName)
	}

	// project export: missing --project-ids
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "export"); err == nil {
		t.Fatal("project export without --project-ids should fail")
	}

	// project export: invalid integer
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "export", "--project-ids", "abc"); err == nil {
		t.Fatal("project export with non-integer should fail")
	}

	// project export: success with --process-code
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "export", "--project-ids", "1001,1002", "--process-code", "PC001"); err != nil {
		t.Fatalf("project export: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "exportProject" {
		t.Fatalf("tool = %q, want exportProject", call.toolName)
	}
	req, ok = call.args["ExportProjectOpenRequest"].(map[string]any)
	if !ok || req["processCode"] != "PC001" {
		t.Fatalf("exportProject req: %#v", req)
	}

	// project export: success without --process-code
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "export", "--project-ids", "1001"); err != nil {
		t.Fatalf("project export minimal: %v", err)
	}

	// project import-template: success
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "import-template"); err != nil {
		t.Fatalf("project import-template: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "getImportProjectTemplate" {
		t.Fatalf("tool = %q, want getImportProjectTemplate", call.toolName)
	}

	// project import: missing --file-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "import"); err == nil {
		t.Fatal("project import without --file-id should fail")
	}

	// project import: success with all optional flags
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "import",
		"--file-id", "abc123",
		"--space-id", "7890",
		"--file-name", "data.xlsx",
		"--file-type", "xlsx",
		"--file-size", "102400"); err != nil {
		t.Fatalf("project import: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "importProject" {
		t.Fatalf("tool = %q, want importProject", call.toolName)
	}
	req, ok = call.args["ImportProjectOpenRequest"].(map[string]any)
	if !ok || req["fileId"] != "abc123" || req["spaceId"] != int64(7890) {
		t.Fatalf("importProject req: %#v", req)
	}
	if req["fileName"] != "data.xlsx" || req["fileType"] != "xlsx" || req["fileSize"] != int64(102400) {
		t.Fatalf("importProject optional fields: %#v", req)
	}

	// project import: success minimal
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "import", "--file-id", "abc"); err != nil {
		t.Fatalf("project import minimal: %v", err)
	}

	// project import-result: missing --task-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "import-result"); err == nil {
		t.Fatal("project import-result without --task-id should fail")
	}

	// project import-result: success
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "import-result", "--task-id", "task_xxx"); err != nil {
		t.Fatalf("project import-result: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "getImportProjectResult" {
		t.Fatalf("tool = %q, want getImportProjectResult", call.toolName)
	}
}

// TestCrossPlatformCoverageContractSubjectCommands covers all subject commands.
func TestCrossPlatformCoverageContractSubjectCommands(t *testing.T) {
	// subject add: success (already covered by TestCrossPlatformCoverageContractSubjectAddWrapsJSONPayload)
	// subject add: missing --file
	caller := &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "add"); err == nil {
		t.Fatal("subject add without --file should fail")
	}

	// subject list: missing --current-page
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "list", "--page-size", "20"); err == nil {
		t.Fatal("subject list without --current-page should fail")
	}

	// subject list: missing --page-size
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "list", "--current-page", "1"); err == nil {
		t.Fatal("subject list without --page-size should fail")
	}

	// subject list: success with all optional flags
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "list",
		"--current-page", "1", "--page-size", "20",
		"--party-type", "other", "--name", "科技", "--code", "C001", "--source", "contract"); err != nil {
		t.Fatalf("subject list: %v", err)
	}
	call := onlyContractCall(t, caller)
	if call.toolName != "querySubjects" {
		t.Fatalf("tool = %q, want querySubjects", call.toolName)
	}
	req, ok := call.args["QuerySubjectOpenRequest"].(map[string]any)
	if !ok || req["partyType"] != "other" || req["name"] != "科技" {
		t.Fatalf("subject list req: %#v", req)
	}
	if req["code"] != "C001" || req["source"] != "contract" {
		t.Fatalf("subject list optional fields: %#v", req)
	}

	// subject list: success minimal
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "list", "--current-page", "1", "--page-size", "10"); err != nil {
		t.Fatalf("subject list minimal: %v", err)
	}

	// subject detail: missing --subject-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "detail"); err == nil {
		t.Fatal("subject detail without --subject-id should fail")
	}

	// subject detail: success
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "detail", "--subject-id", "2001"); err != nil {
		t.Fatalf("subject detail: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "querySubjectDetail" {
		t.Fatalf("tool = %q, want querySubjectDetail", call.toolName)
	}

	// subject update: success
	updPath := writeTempJSON(t, "subj_upd.json", `{"subjectId":2001,"partyType":"other","name":"updated"}`)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "update", "--file", updPath); err != nil {
		t.Fatalf("subject update: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "updateSubject" {
		t.Fatalf("tool = %q, want updateSubject", call.toolName)
	}

	// subject update: missing --file
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "update"); err == nil {
		t.Fatal("subject update without --file should fail")
	}

	// subject delete: missing --subject-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "delete"); err == nil {
		t.Fatal("subject delete without --subject-id should fail")
	}

	// subject batch-delete: missing --subject-ids
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "batch-delete"); err == nil {
		t.Fatal("subject batch-delete without --subject-ids should fail")
	}

	// subject batch-delete: invalid integer
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "batch-delete", "--subject-ids", "abc"); err == nil {
		t.Fatal("subject batch-delete with non-integer should fail")
	}

	// subject sort: missing --subject-ids
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "sort"); err == nil {
		t.Fatal("subject sort without --subject-ids should fail")
	}

	// subject sort: invalid integer
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "sort", "--subject-ids", "xyz"); err == nil {
		t.Fatal("subject sort with non-integer should fail")
	}

	// subject sort: success
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "sort", "--subject-ids", "2001,2003,2002"); err != nil {
		t.Fatalf("subject sort: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "sortSubjects" {
		t.Fatalf("tool = %q, want sortSubjects", call.toolName)
	}

	// subject detect-risk: missing --subject-name
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "detect-risk"); err == nil {
		t.Fatal("subject detect-risk without --subject-name should fail")
	}

	// subject detect-risk: success with --subject-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "detect-risk", "--subject-name", "北京示例科技有限公司", "--subject-id", "2001"); err != nil {
		t.Fatalf("subject detect-risk: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "detectSubjectRisk" {
		t.Fatalf("tool = %q, want detectSubjectRisk", call.toolName)
	}
	req, ok = call.args["SubjectRiskOpenRequest"].(map[string]any)
	if !ok || req["subjectId"] != int64(2001) {
		t.Fatalf("detect-risk req: %#v", req)
	}

	// subject detect-risk: success without --subject-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "detect-risk", "--subject-name", "北京示例科技有限公司"); err != nil {
		t.Fatalf("subject detect-risk minimal: %v", err)
	}

	// subject base-info: missing --subject-name
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "base-info"); err == nil {
		t.Fatal("subject base-info without --subject-name should fail")
	}

	// subject base-info: success with --subject-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "base-info", "--subject-name", "北京示例科技有限公司", "--subject-id", "2001"); err != nil {
		t.Fatalf("subject base-info: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "querySubjectBaseInfo" {
		t.Fatalf("tool = %q, want querySubjectBaseInfo", call.toolName)
	}

	// subject base-info: success without --subject-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "base-info", "--subject-name", "北京示例科技有限公司"); err != nil {
		t.Fatalf("subject base-info minimal: %v", err)
	}

	// subject auto-fill: missing --subject-name
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "auto-fill"); err == nil {
		t.Fatal("subject auto-fill without --subject-name should fail")
	}

	// subject auto-fill: success with --subject-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "auto-fill", "--subject-name", "北京示例科技有限公司", "--subject-id", "2001"); err != nil {
		t.Fatalf("subject auto-fill: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "autoFillSubjectInfo" {
		t.Fatalf("tool = %q, want autoFillSubjectInfo", call.toolName)
	}

	// subject auto-fill: success without --subject-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "auto-fill", "--subject-name", "北京示例科技有限公司"); err != nil {
		t.Fatalf("subject auto-fill minimal: %v", err)
	}

	// subject export: missing --subject-ids
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "export"); err == nil {
		t.Fatal("subject export without --subject-ids should fail")
	}

	// subject export: invalid integer
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "export", "--subject-ids", "xyz"); err == nil {
		t.Fatal("subject export with non-integer should fail")
	}

	// subject export: success with --process-code
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "export", "--subject-ids", "2001,2002", "--process-code", "PC001"); err != nil {
		t.Fatalf("subject export: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "exportSubject" {
		t.Fatalf("tool = %q, want exportSubject", call.toolName)
	}
	req, ok = call.args["ExportSubjectOpenRequest"].(map[string]any)
	if !ok || req["processCode"] != "PC001" {
		t.Fatalf("exportSubject req: %#v", req)
	}

	// subject export: success without --process-code
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "export", "--subject-ids", "2001"); err != nil {
		t.Fatalf("subject export minimal: %v", err)
	}

	// subject import-template: success with --type
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "import-template", "--type", "other"); err != nil {
		t.Fatalf("subject import-template: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "getImportSubjectTemplate" {
		t.Fatalf("tool = %q, want getImportSubjectTemplate", call.toolName)
	}
	req, ok = call.args["GetImportSubjectTemplateOpenRequest"].(map[string]any)
	if !ok || req["type"] != "other" {
		t.Fatalf("import-template req: %#v", req)
	}

	// subject import-template: success without --type
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "import-template"); err != nil {
		t.Fatalf("subject import-template minimal: %v", err)
	}

	// subject import: missing --file-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "import"); err == nil {
		t.Fatal("subject import without --file-id should fail")
	}

	// subject import: success with all optional flags
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "import",
		"--file-id", "abc123",
		"--space-id", "7890",
		"--file-name", "subjects.xlsx",
		"--file-type", "xlsx",
		"--file-size", "51200"); err != nil {
		t.Fatalf("subject import: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "importSubject" {
		t.Fatalf("tool = %q, want importSubject", call.toolName)
	}
	req, ok = call.args["ImportSubjectOpenRequest"].(map[string]any)
	if !ok || req["fileId"] != "abc123" || req["spaceId"] != int64(7890) {
		t.Fatalf("importSubject req: %#v", req)
	}
	if req["fileName"] != "subjects.xlsx" || req["fileType"] != "xlsx" || req["fileSize"] != int64(51200) {
		t.Fatalf("importSubject optional fields: %#v", req)
	}

	// subject import: success minimal
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "import", "--file-id", "abc"); err != nil {
		t.Fatalf("subject import minimal: %v", err)
	}

	// subject import-result: missing --task-id
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "import-result"); err == nil {
		t.Fatal("subject import-result without --task-id should fail")
	}

	// subject import-result: success
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"subject", "import-result", "--task-id", "task_xxx"); err != nil {
		t.Fatalf("subject import-result: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "getImportSubjectResult" {
		t.Fatalf("tool = %q, want getImportSubjectResult", call.toolName)
	}
}

// TestCrossPlatformCoverageContractHelperEdges covers readContractJSONPayload stdin path,
// parseContractRecordQueryScope invalid value, and parseContractInt64CSV parse error.
func TestCrossPlatformCoverageContractHelperEdges(t *testing.T) {
	// readContractJSONPayload: stdin path (file == "-")
	// Test via a command that uses readContractJSONPayload, with stdin
	caller := &contractDefectCaller{}
	root := newContractCommand()
	root.PersistentFlags().Bool("yes", false, "confirm")
	root.PersistentFlags().Bool("dry-run", false, "preview")
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetIn(strings.NewReader(`{"name":"stdin-test"}`))
	root.SetArgs([]string{"record", "create", "--file", "-"})
	InitDeps(caller)
	var stdout bytes.Buffer
	deps.Out.w = &stdout
	deps.Out.errW = io.Discard
	if err := root.Execute(); err != nil {
		t.Fatalf("record create from stdin: %v", err)
	}
	call := onlyContractCall(t, caller)
	if call.toolName != "createContract" {
		t.Fatalf("tool = %q, want createContract", call.toolName)
	}
	req, ok := call.args["ImportContractInfoRequest"].(map[string]any)
	if !ok || req["name"] != "stdin-test" {
		t.Fatalf("ImportContractInfoRequest = %#v", call.args["ImportContractInfoRequest"])
	}

	// readContractJSONPayload: file open error (nonexistent file)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"record", "create", "--file", "/nonexistent/path/contract.json"); err == nil {
		t.Fatal("record create with nonexistent file should fail")
	}

	// readContractJSONPayload: JSON parse error
	badPath := writeTempJSON(t, "bad.json", `{broken`)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"record", "create", "--file", badPath); err == nil {
		t.Fatal("record create with broken JSON should fail")
	}

	// parseContractRecordQueryScope: invalid type
	if _, err := parseContractRecordQueryScope("totally-bogus"); err == nil {
		t.Fatal("parseContractRecordQueryScope should reject invalid type")
	}

	// parseContractInt64CSV: parse error
	if _, err := parseContractInt64CSV("123,not_a_number,456"); err == nil {
		t.Fatal("parseContractInt64CSV should reject non-integer")
	}
}

// TestCrossPlatformCoverageContractRemainingEdges covers the remaining uncovered branches:
// - io.ReadAll error paths (directory as --file)
// - appendContractCreateTimeFromISO error paths (invalid ISO time)
// - delete command validation with --yes but missing required flag
// - parseContractInt64CSV empty element skip
// - parseContractRecordQueryScope empty string
func TestCrossPlatformCoverageContractRemainingEdges(t *testing.T) {
	dir := t.TempDir() // a directory, to trigger io.ReadAll "is a directory" error

	// review create: io.ReadAll error (directory as --file)
	caller := &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"review", "create", "--file", dir); err == nil {
		t.Fatal("review create with directory as --file should fail (io.ReadAll error)")
	}

	// review analysis: io.ReadAll error (directory as --file)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"review", "analysis", "--file", dir); err == nil {
		t.Fatal("review analysis with directory as --file should fail (io.ReadAll error)")
	}

	// readContractJSONPayload: io.ReadAll error (directory as --file)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"record", "create", "--file", dir); err == nil {
		t.Fatal("record create with directory as --file should fail (io.ReadAll error)")
	}

	// account list: --exec-end invalid ISO time
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"account", "list", "--exec-start", "2026-01-01T00:00:00+08:00",
		"--exec-end", "not-a-date"); err == nil {
		t.Fatal("account list with invalid --exec-end should fail")
	}

	// account delete: --yes but missing --account-id (hits validation in RunE)
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"account", "delete", "--yes"); err == nil {
		t.Fatal("account delete with --yes but no --account-id should fail")
	}

	// project add: --contract-ids invalid integer
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "add", "--name", "test", "--contract-ids", "abc"); err == nil {
		t.Fatal("project add with invalid --contract-ids should fail")
	}

	// project update: --start-date invalid ISO time
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "update", "--project-id", "1", "--name", "test",
		"--start-date", "not-a-date"); err == nil {
		t.Fatal("project update with invalid --start-date should fail")
	}

	// project update: --end-date invalid ISO time
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "update", "--project-id", "1", "--name", "test",
		"--end-date", "not-a-date"); err == nil {
		t.Fatal("project update with invalid --end-date should fail")
	}

	// project update: --contract-ids invalid integer
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "update", "--project-id", "1", "--name", "test",
		"--contract-ids", "xyz"); err == nil {
		t.Fatal("project update with invalid --contract-ids should fail")
	}

	// project list: invalid date flags
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "list", "--current-page", "1", "--page-size", "20", "--scope", "all",
		"--start-date-left", "bad"); err == nil {
		t.Fatal("project list with invalid --start-date-left should fail")
	}
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "list", "--current-page", "1", "--page-size", "20", "--scope", "all",
		"--start-date-right", "bad"); err == nil {
		t.Fatal("project list with invalid --start-date-right should fail")
	}
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "list", "--current-page", "1", "--page-size", "20", "--scope", "all",
		"--end-date-left", "bad"); err == nil {
		t.Fatal("project list with invalid --end-date-left should fail")
	}
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "list", "--current-page", "1", "--page-size", "20", "--scope", "all",
		"--end-date-right", "bad"); err == nil {
		t.Fatal("project list with invalid --end-date-right should fail")
	}

	// project digests: invalid date flags
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "digests", "--current-page", "1", "--page-size", "20", "--scope", "all",
		"--start-date-left", "bad"); err == nil {
		t.Fatal("project digests with invalid --start-date-left should fail")
	}
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "digests", "--current-page", "1", "--page-size", "20", "--scope", "all",
		"--start-date-right", "bad"); err == nil {
		t.Fatal("project digests with invalid --start-date-right should fail")
	}
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "digests", "--current-page", "1", "--page-size", "20", "--scope", "all",
		"--end-date-left", "bad"); err == nil {
		t.Fatal("project digests with invalid --end-date-left should fail")
	}
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "digests", "--current-page", "1", "--page-size", "20", "--scope", "all",
		"--end-date-right", "bad"); err == nil {
		t.Fatal("project digests with invalid --end-date-right should fail")
	}

	// parseContractRecordQueryScope: empty string returns "all"
	scope, err := parseContractRecordQueryScope("")
	if err != nil || scope != "all" {
		t.Fatalf("parseContractRecordQueryScope(\"\") = %q, %v; want \"all\", nil", scope, err)
	}

	// parseContractInt64CSV: empty element skip ("1,,2")
	ids, err := parseContractInt64CSV("1,,2")
	if err != nil || !reflect.DeepEqual(ids, []int64{1, 2}) {
		t.Fatalf("parseContractInt64CSV(\"1,,2\") = %#v, %v; want [1 2], nil", ids, err)
	}
}

// TestCrossPlatformCoverageContractFinalEdges covers the last remaining branches.
func TestCrossPlatformCoverageContractFinalEdges(t *testing.T) {
	// record list: invalid --start time
	caller := &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"record", "list", "--start", "bad-date"); err == nil {
		t.Fatal("record list with invalid --start should fail")
	}

	// record list: invalid --end time
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"record", "list", "--end", "not-a-date"); err == nil {
		t.Fatal("record list with invalid --end should fail")
	}

	// review create: stdin path (--file -)
	caller = &contractDefectCaller{}
	root := newContractCommand()
	root.PersistentFlags().Bool("yes", false, "confirm")
	root.PersistentFlags().Bool("dry-run", false, "preview")
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetIn(strings.NewReader(`{"source":"test"}`))
	root.SetArgs([]string{"review", "create", "--file", "-"})
	InitDeps(caller)
	var stdout bytes.Buffer
	deps.Out.w = &stdout
	deps.Out.errW = io.Discard
	if err := root.Execute(); err != nil {
		t.Fatalf("review create from stdin: %v", err)
	}
	call := onlyContractCall(t, caller)
	if call.toolName != "createContractReviewTask" {
		t.Fatalf("tool = %q, want createContractReviewTask", call.toolName)
	}

	// review analysis: stdin path (--file -)
	caller = &contractDefectCaller{}
	root2 := newContractCommand()
	root2.PersistentFlags().Bool("yes", false, "confirm")
	root2.PersistentFlags().Bool("dry-run", false, "preview")
	root2.SilenceErrors = true
	root2.SilenceUsage = true
	root2.SetIn(strings.NewReader(`{"fileInfo":{"fileId":"xxx"}}`))
	root2.SetArgs([]string{"review", "analysis", "--file", "-"})
	InitDeps(caller)
	deps.Out.w = &stdout
	deps.Out.errW = io.Discard
	if err := root2.Execute(); err != nil {
		t.Fatalf("review analysis from stdin: %v", err)
	}
	call = onlyContractCall(t, caller)
	if call.toolName != "contractAnalysis" {
		t.Fatalf("tool = %q, want contractAnalysis", call.toolName)
	}

	// account list: invalid --exec-start
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"account", "list", "--exec-start", "bad-date"); err == nil {
		t.Fatal("account list with invalid --exec-start should fail")
	}

	// project add: invalid --start-date
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "add", "--name", "test", "--start-date", "bad"); err == nil {
		t.Fatal("project add with invalid --start-date should fail")
	}

	// project add: invalid --end-date
	caller = &contractDefectCaller{}
	if _, err := executeContractDefectCommand(t, caller, newContractCommand,
		"project", "add", "--name", "test", "--end-date", "bad"); err == nil {
		t.Fatal("project add with invalid --end-date should fail")
	}
}
