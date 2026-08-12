// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type driveCoverageCaller struct {
	mu        sync.Mutex
	responses map[string][]string
	history   []string
}

func (caller *driveCoverageCaller) CallTool(_ context.Context, _, tool string, _ map[string]any) (*edition.ToolResult, error) {
	caller.mu.Lock()
	defer caller.mu.Unlock()
	caller.history = append(caller.history, tool)
	queue := caller.responses[tool]
	if len(queue) == 0 {
		return nil, errors.New("missing fake response for " + tool)
	}
	caller.responses[tool] = queue[1:]
	if queue[0] == "__ERROR__" {
		return nil, errors.New("injected drive failure")
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: queue[0]}}}, nil
}

func (*driveCoverageCaller) Format() string { return "json" }
func (*driveCoverageCaller) DryRun() bool   { return false }
func (*driveCoverageCaller) Fields() string { return "" }
func (*driveCoverageCaller) JQ() string     { return "" }

func driveJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func runDriveCoverage(t *testing.T, declaration shortcut.Shortcut, caller *driveCoverageCaller, args ...string) error {
	t.Helper()
	helpers.InitDeps(caller)
	declaration.OutputRollout = output.RolloutLegacyOnly
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	service := &cobra.Command{Use: "drive"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(declaration)))
	root.AddCommand(service)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"drive", declaration.Command}, args...))
	return root.Execute()
}

func TestCrossPlatformCoverageDriveCollectionsRejectFalseEmptySuccess(t *testing.T) {
	tests := []struct {
		name     string
		shortcut shortcut.Shortcut
		tool     string
		payload  string
		args     []string
		wantErr  string
	}{
		{"list explicit empty", List, "list_files", `{"success":true,"result":{"items":[]}}`, nil, ""},
		{"list missing collection", List, "list_files", `{"success":true,"result":{"count":0}}`, nil, "缺少声明的业务数组"},
		{"list malformed collection", List, "list_files", `{"success":true,"result":{"items":{}}}`, nil, "不是数组"},
		{"list malformed item", List, "list_files", `{"success":true,"result":{"items":["bad"]}}`, nil, "不是对象"},
		{"list empty tool response", List, "list_files", ``, nil, "空响应"},
		{"list remote failure", List, "list_files", `{"success":false,"errorMsg":"denied"}`, nil, "denied"},
		{"search explicit empty", Search, "search_files", `{"success":true,"items":[],"hasMore":false}`, []string{"--query", "none"}, ""},
		{"search missing collection", Search, "search_files", `{"success":true,"hasMore":false}`, []string{"--query", "none"}, "缺少声明的业务数组"},
		{"recent nested data", Recent, "get_recent_list", `{"success":true,"result":{"recentItems":[],"hasMore":false}}`, nil, ""},
		{"recent missing collection", Recent, "get_recent_list", `{"success":true,"result":{"totalCount":0}}`, nil, "缺少声明的业务数组"},
		{"star explicit empty", StarList, "get_star_list", `{"success":true,"starList":[]}`, nil, ""},
		{"recycle explicit empty", RecycleList, "list_recycle_items", `{"success":true,"recycleItems":[]}`, nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &driveCoverageCaller{responses: map[string][]string{tc.tool: {tc.payload}}}
			err := runDriveCoverage(t, tc.shortcut, caller, tc.args...)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageDriveCreateAndInspectReadback(t *testing.T) {
	caller := &driveCoverageCaller{responses: map[string][]string{
		"create_folder": {driveJSON(map[string]any{"success": true, "result": map[string]any{"fileId": "folder-1"}})},
		"get_file_info": {driveJSON(map[string]any{"success": true, "result": map[string]any{"fileId": "folder-1", "name": "项目资料"}})},
	}}
	if err := runDriveCoverage(t, CreateFolder, caller, "--name", "项目资料"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(caller.history, ",") != "create_folder,get_file_info" {
		t.Fatalf("history = %v", caller.history)
	}

	missingID := &driveCoverageCaller{responses: map[string][]string{
		"create_folder": {`{"success":true,"result":{"name":"x"}}`},
	}}
	if err := runDriveCoverage(t, CreateFolder, missingID, "--name", "x"); err == nil || !strings.Contains(err.Error(), "没有文件夹 fileId") {
		t.Fatalf("missing ID error = %v", err)
	}

	mismatch := &driveCoverageCaller{responses: map[string][]string{
		"create_folder": {`{"success":true,"result":{"fileId":"folder-2"}}`},
		"get_file_info": {`{"success":true,"result":{"fileId":"folder-2","name":"other"}}`},
	}}
	if err := runDriveCoverage(t, CreateFolder, mismatch, "--name", "wanted"); err == nil || !strings.Contains(err.Error(), "读回名称") {
		t.Fatalf("readback mismatch error = %v", err)
	}

	partial := &driveCoverageCaller{responses: map[string][]string{
		"get_file_info":           {`{"success":true,"result":{"fileId":"n1","name":"x"}}`},
		"get_node_stats":          {`{"success":true,"views":1}`},
		"get_file_publish_status": {`__ERROR__`},
	}}
	err := runDriveCoverage(t, Inspect, partial, "--node", "n1", "--include-stats", "--include-publish")
	if err == nil || !strings.Contains(err.Error(), "部分读取") {
		t.Fatalf("inspect partial error = %v", err)
	}
}

func TestCrossPlatformCoverageDriveDownloadAndUploadRequireArtifactsAndReadback(t *testing.T) {
	var outputFlag *shortcut.Flag
	for i := range Download.Flags {
		if Download.Flags[i].Name == "output" {
			outputFlag = &Download.Flags[i]
			break
		}
	}
	if outputFlag == nil || outputFlag.Shorthand != "o" {
		t.Fatalf("download output shorthand = %#v, want -o compatibility", outputFlag)
	}

	t.Chdir(t.TempDir())
	if err := os.WriteFile("input.bin", []byte("actual-drive-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &uploadDriveFile, func(context.Context, helpers.DriveUploadRequest) (map[string]any, error) {
		return map[string]any{"success": true, "result": map[string]any{"fileId": "uploaded-1"}}, nil
	})
	uploadCaller := &driveCoverageCaller{responses: map[string][]string{
		"get_file_info": {`{"success":true,"result":{"fileId":"uploaded-1","name":"input.bin","fileSize":18}}`},
	}}
	if err := runDriveCoverage(t, Upload, uploadCaller, "--file", "input.bin", "--yes"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveDriveUploadInput("../escape.bin"); err == nil {
		t.Fatal("upload path escape was accepted")
	}

	testseam.Swap(t, &driveDownload, func(_ context.Context, _ string, options localio.DownloadOptions) (localio.DownloadResult, error) {
		if options.Output != "downloads/file.bin" || options.Headers["x-token"] != "secret" {
			t.Fatalf("download options = %#v", options)
		}
		return localio.DownloadResult{RelativePath: options.Output, AbsolutePath: filepath.Join(t.TempDir(), "file.bin"), SizeBytes: 18}, nil
	})
	downloadCaller := &driveCoverageCaller{responses: map[string][]string{
		"download_file": {`{"success":true,"result":{"downloadUrl":"https://download.dingtalk.com/file.bin","fileName":"file.bin","headers":{"x-token":"secret"}}}`},
	}}
	if err := runDriveCoverage(t, Download, downloadCaller, "--node", "uploaded-1", "--output", "downloads/file.bin"); err != nil {
		t.Fatal(err)
	}

	testseam.Swap(t, &driveDownload, func(context.Context, string, localio.DownloadOptions) (localio.DownloadResult, error) {
		return localio.DownloadResult{}, nil
	})
	emptyArtifact := &driveCoverageCaller{responses: map[string][]string{
		"download_file": {`{"success":true,"result":{"downloadUrl":"https://download.dingtalk.com/file.bin"}}`},
	}}
	if err := runDriveCoverage(t, Download, emptyArtifact, "--node", "uploaded-1", "--output", "empty.bin"); err == nil || !strings.Contains(err.Error(), "0 字节") {
		t.Fatalf("empty artifact error = %v", err)
	}
}

func TestCrossPlatformCoverageDriveCopyPreservesSchemaProperties(t *testing.T) {
	want := map[string]string{
		"folder":    "folder",
		"node":      "node",
		"workspace": "workspace",
	}
	got := make(map[string]string, len(Copy.Contract.Parameters))
	for _, parameter := range Copy.Contract.Parameters {
		got[parameter.Name] = parameter.Property
	}
	for name, property := range want {
		if got[name] != property {
			t.Errorf("copy parameter %q property = %q, want %q", name, got[name], property)
		}
	}
}

func TestCrossPlatformCoverageDriveVersionAndPublishContracts(t *testing.T) {
	versionPayload := `{"success":true,"versions":[{"version":1,"fileSize":3},{"versionNumber":"2","fileSize":4}],"hasMore":false}`
	caller := &driveCoverageCaller{responses: map[string][]string{"list_file_versions": {versionPayload}}}
	if err := runDriveCoverage(t, VersionGet, caller, "--node", "n1", "--version", "2"); err != nil {
		t.Fatal(err)
	}
	notFound := &driveCoverageCaller{responses: map[string][]string{"list_file_versions": {versionPayload}}}
	if err := runDriveCoverage(t, VersionGet, notFound, "--node", "n1", "--version", "9"); err == nil || !strings.Contains(err.Error(), "不存在版本") {
		t.Fatalf("version not found = %v", err)
	}

	publish := &driveCoverageCaller{responses: map[string][]string{
		"set_file_publish":        {`{"success":true,"message":"ok"}`},
		"get_file_publish_status": {`{"success":true,"result":{"published":true,"permission":"READER"}}`},
	}}
	if err := runDriveCoverage(t, PublishSet, publish, "--node", "n1", "--permission", "READER", "--yes"); err != nil {
		t.Fatal(err)
	}
	mismatch := &driveCoverageCaller{responses: map[string][]string{
		"set_file_publish":        {`{"success":true}`},
		"get_file_publish_status": {`{"success":true,"result":{"published":false}}`},
	}}
	if err := runDriveCoverage(t, PublishSet, mismatch, "--node", "n1", "--yes"); err == nil || !strings.Contains(err.Error(), "读回不一致") {
		t.Fatalf("publish mismatch = %v", err)
	}
}

func TestCrossPlatformCoverageDriveRealFieldSemanticsAndRenameNormalization(t *testing.T) {
	items := []any{map[string]any{
		"recycleItemId": "123456789012",
		"originalName":  "deleted.md",
		"originalPath":  "/folder/deleted.md",
		"operatorTime":  "2026-08-11T00:00:00Z",
		"fileSize":      9,
	}}
	rows := projectDriveRows(items, map[string][]string{
		"recycleItemId": {"recycleItemId", "id"},
		"originalName":  {"originalName", "name"},
		"originalPath":  {"originalPath", "path"},
	})
	if len(rows) != 1 || rows[0]["recycleItemId"] != "123456789012" || rows[0]["originalName"] != "deleted.md" {
		t.Fatalf("recycle projection = %#v", rows)
	}

	request, expected := normalizedDriveRename("report.md", map[string]any{"type": "FILE", "extension": "md"})
	if request != "report" || !expected["report.md"] || expected["report.md.md"] {
		t.Fatalf("normalized rename = %q %#v", request, expected)
	}
	request, expected = normalizedDriveRename("report", map[string]any{"type": "FILE", "extension": "md"})
	if request != "report" || !expected["report"] || !expected["report.md"] {
		t.Fatalf("extension-less rename = %q %#v", request, expected)
	}
	request, expected = normalizedDriveRename("folder.md", map[string]any{"type": "FOLDER", "extension": "md"})
	if request != "folder.md" || !expected["folder.md"] {
		t.Fatalf("folder rename = %q %#v", request, expected)
	}

	ordinary := &driveCoverageCaller{responses: map[string][]string{
		"get_document_info": {`{"success":true,"extension":"md","nodeType":"file"}`},
	}}
	err := runDriveCoverage(t, Copy, ordinary, "--node", "plain-file", "--folder", "target", "--yes")
	if err == nil || !strings.Contains(err.Error(), ".dlink") {
		t.Fatalf("ordinary copy preflight error = %v", err)
	}
	if strings.Join(ordinary.history, ",") != "get_document_info" {
		t.Fatalf("ordinary copy performed a write: %v", ordinary.history)
	}
}

func TestCrossPlatformCoverageDriveStrictResponseHelpers(t *testing.T) {
	valid := map[string]any{"success": true, "result": map[string]any{"id": "n1"}}
	if object, err := requireDriveObject(valid, "op"); err != nil || object["id"] != "n1" {
		t.Fatalf("valid result=%v error=%v", object, err)
	}
	if object, err := requireDriveObject(map[string]any{"success": true, "data": map[string]any{"id": "n2"}}, "op"); err != nil || object["id"] != "n2" {
		t.Fatalf("valid data=%v error=%v", object, err)
	}
	if object, err := requireDriveObject(map[string]any{"success": true, "id": "n3"}, "op"); err != nil || object["id"] != "n3" {
		t.Fatalf("valid flat=%v error=%v", object, err)
	}
	if value := nestedString(map[string]any{"data": map[string]any{"id": "nested"}}, "id"); value != "nested" {
		t.Fatalf("nestedString=%q", value)
	}
	if value := nestedString(map[string]any{"id": "direct"}, "id"); value != "direct" {
		t.Fatalf("direct nestedString=%q", value)
	}
	if value := nestedString(map[string]any{"result": "bad"}, "id"); value != "" {
		t.Fatalf("unexpected nestedString=%q", value)
	}

	cases := []struct {
		name string
		call func() error
	}{
		{"malformed success", func() error { _, err := requireDriveResponse(map[string]any{"success": "yes"}, "op"); return err }},
		{"object response failure", func() error { _, err := requireDriveObject(map[string]any{}, "op"); return err }},
		{"false no message", func() error { _, err := requireDriveResponse(map[string]any{"success": false}, "op"); return err }},
		{"malformed result", func() error {
			_, err := requireDriveObject(map[string]any{"success": true, "result": []any{}}, "op")
			return err
		}},
		{"empty result", func() error {
			_, err := requireDriveObject(map[string]any{"success": true, "result": map[string]any{}}, "op")
			return err
		}},
		{"malformed data", func() error {
			_, err := requireDriveObject(map[string]any{"success": true, "data": "bad"}, "op")
			return err
		}},
		{"empty data", func() error {
			_, err := requireDriveObject(map[string]any{"success": true, "data": map[string]any{}}, "op")
			return err
		}},
		{"missing object", func() error { _, err := requireDriveObject(map[string]any{"success": true}, "op"); return err }},
		{"write response error", func() error { _, err := requireDriveWrite(map[string]any{}, "op"); return err }},
		{"write no terminal success", func() error { _, err := requireDriveWrite(map[string]any{"id": "n1"}, "op"); return err }},
		{"collection malformed result envelope", func() error {
			_, _, err := requireDriveCollection(map[string]any{"success": true, "result": "bad"}, "op", "items")
			return err
		}},
		{"collection malformed data envelope", func() error {
			_, _, err := requireDriveCollection(map[string]any{"success": true, "data": "bad"}, "op", "items")
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Fatal("expected strict response error")
			}
		})
	}
}

func TestCrossPlatformCoverageDriveObjectAndMutationShortcuts(t *testing.T) {
	for _, tc := range []struct {
		name        string
		declaration shortcut.Shortcut
		responses   map[string][]string
		args        []string
		wantErr     string
	}{
		{"info", Info, map[string][]string{"get_file_info": {`{"success":true,"result":{"fileId":"n1"}}`}}, []string{"--node", "n1"}, ""},
		{"stats", Stats, map[string][]string{"get_node_stats": {`{"success":true,"result":{"views":1}}`}}, []string{"--node", "n1"}, ""},
		{"cover", Cover, map[string][]string{"get_cover": {`{"success":true,"result":{"url":"https://x"}}`}}, []string{"--node", "n1"}, ""},
		{"stats call failure", Stats, map[string][]string{"get_node_stats": {`__ERROR__`}}, []string{"--node", "n1"}, "injected"},
		{"stats malformed object", Stats, map[string][]string{"get_node_stats": {`{"success":true}`}}, []string{"--node", "n1"}, "业务对象"},
		{"star add", StarAdd, map[string][]string{"mark_star": {`{"success":true}`}}, []string{"--node", "n1"}, ""},
		{"star remove", StarRemove, map[string][]string{"unmark_star": {`{"success":true}`}}, []string{"--node", "n1"}, ""},
		{"star mutation failure", StarAdd, map[string][]string{"mark_star": {`__ERROR__`}}, []string{"--node", "n1"}, "injected"},
		{"star terminal evidence failure", StarAdd, map[string][]string{"mark_star": {`{"nodeId":"n1"}`}}, []string{"--node", "n1"}, "success=true"},
		{"delete", Delete, map[string][]string{"delete_document": {`{"success":true}`}}, []string{"--node", "n1", "--yes"}, ""},
		{"publish get", PublishGet, map[string][]string{"get_file_publish_status": {`{"success":true,"result":{"published":false}}`}}, []string{"--node", "n1"}, ""},
		{"publish get call failure", PublishGet, map[string][]string{"get_file_publish_status": {`__ERROR__`}}, []string{"--node", "n1"}, "injected"},
		{"publish get malformed object", PublishGet, map[string][]string{"get_file_publish_status": {`{"success":true}`}}, []string{"--node", "n1"}, "业务对象"},
		{"publish unset", PublishUnset, map[string][]string{"set_file_publish": {`{"success":true}`}, "get_file_publish_status": {`{"success":true,"result":{"isPublished":false}}`}}, []string{"--node", "n1", "--yes"}, ""},
		{"publish write call failure", PublishUnset, map[string][]string{"set_file_publish": {`__ERROR__`}}, []string{"--node", "n1", "--yes"}, "injected"},
		{"publish terminal evidence failure", PublishUnset, map[string][]string{"set_file_publish": {`{"nodeId":"n1"}`}}, []string{"--node", "n1", "--yes"}, "success=true"},
		{"publish readback call failure", PublishUnset, map[string][]string{"set_file_publish": {`{"success":true}`}, "get_file_publish_status": {`__ERROR__`}}, []string{"--node", "n1", "--yes"}, "injected"},
		{"publish readback malformed object", PublishUnset, map[string][]string{"set_file_publish": {`{"success":true}`}, "get_file_publish_status": {`{"success":true}`}}, []string{"--node", "n1", "--yes"}, "业务对象"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := runDriveCoverage(t, tc.declaration, &driveCoverageCaller{responses: tc.responses}, tc.args...)
			if tc.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("error=%v want=%q", err, tc.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageDriveCreateRestoreCopyMoveRename(t *testing.T) {
	testseam.Swap(t, &driveRestoreWait, func(time.Duration) {})
	createShortcut := &driveCoverageCaller{responses: map[string][]string{
		"create_shortcut": {`{"success":true,"result":{"nodeId":"shortcut-1"}}`},
		"get_file_info":   {`{"success":true,"result":{"fileId":"shortcut-1","name":"link"}}`},
	}}
	if err := runDriveCoverage(t, CreateShortcut, createShortcut, "--node", "source", "--folder", "target", "--workspace", "space"); err != nil {
		t.Fatal(err)
	}
	missingShortcut := &driveCoverageCaller{responses: map[string][]string{"create_shortcut": {`{"success":true,"result":{"name":"link"}}`}}}
	if err := runDriveCoverage(t, CreateShortcut, missingShortcut, "--node", "source"); err == nil {
		t.Fatal("missing shortcut id accepted")
	}

	restore := &driveCoverageCaller{responses: map[string][]string{
		"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"recycle-1","originalName":"restored.txt"}]}`},
		"restore_recycle_item": {`{"success":true,"result":{"fileId":"restored-1"}}`},
		"get_file_info":        {`{"success":true,"result":{"fileId":"restored-1"}}`},
	}}
	if err := runDriveCoverage(t, RecycleRestore, restore, "--id", "recycle-1", "--yes"); err != nil {
		t.Fatal(err)
	}
	restoreWithoutID := &driveCoverageCaller{responses: map[string][]string{
		"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"recycle-1","originalName":"restored.txt","originalPath":"/folder/restored.txt"}]}`},
		"restore_recycle_item": {`{"success":true}`},
		"search_files":         {`{"success":true,"items":[{"fileId":"folder-1","name":"folder","type":"FOLDER"}]}`},
		"list_files":           {`{"success":true,"items":[{"fileId":"restored-1","name":"restored.txt"}]}`},
		"get_file_info":        {`{"success":true,"result":{"fileId":"restored-1","name":"restored.txt"}}`},
	}}
	if err := runDriveCoverage(t, RecycleRestore, restoreWithoutID, "--id", "recycle-1", "--yes"); err != nil {
		t.Fatal(err)
	}
	missingRestore := &driveCoverageCaller{responses: map[string][]string{
		"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"recycle-1","originalName":"restored.txt","originalPath":"/folder/restored.txt"}]}`},
		"restore_recycle_item": {`{"success":true}`},
		"search_files":         {`{"success":true,"items":[]}`, `{"success":true,"items":[]}`, `{"success":true,"items":[]}`, `{"success":true,"items":[]}`, `{"success":true,"items":[]}`, `{"success":true,"items":[]}`, `{"success":true,"items":[]}`, `{"success":true,"items":[]}`},
	}}
	if err := runDriveCoverage(t, RecycleRestore, missingRestore, "--id", "recycle-1", "--yes"); err == nil {
		t.Fatal("restore without read-back evidence accepted")
	}

	copyCaller := &driveCoverageCaller{responses: map[string][]string{
		"get_document_info": {`{"success":true,"result":{"extension":"adoc"}}`, `{"success":true,"result":{"nodeId":"copy-1","name":"copy"}}`},
		"copy_document":     {`{"success":true,"result":{"nodeId":"copy-1"}}`},
	}}
	if err := runDriveCoverage(t, Copy, copyCaller, "--node", "source", "--folder", "target", "--workspace", "space", "--yes"); err != nil {
		t.Fatal(err)
	}
	missingCopyID := &driveCoverageCaller{responses: map[string][]string{
		"get_document_info": {`{"success":true,"result":{"contentType":"DOC"}}`},
		"copy_document":     {`{"success":true}`},
	}}
	if err := runDriveCoverage(t, Copy, missingCopyID, "--node", "source", "--folder", "target", "--yes"); err == nil {
		t.Fatal("missing copy id accepted")
	}

	move := &driveCoverageCaller{responses: map[string][]string{
		"move_document":     {`{"success":true}`},
		"get_document_info": {`{"success":true,"result":{"nodeId":"n1"}}`},
	}}
	if err := runDriveCoverage(t, Move, move, "--node", "n1", "--folder", "target", "--workspace", "space", "--yes"); err != nil {
		t.Fatal(err)
	}

	rename := &driveCoverageCaller{responses: map[string][]string{
		"get_file_info":   {`{"success":true,"result":{"fileId":"n1","type":"FILE","extension":"md","name":"old.md"}}`, `{"success":true,"result":{"fileId":"n1","name":"new.md"}}`},
		"rename_document": {`{"success":true}`},
	}}
	if err := runDriveCoverage(t, Rename, rename, "--node", "n1", "--name", "new.md", "--yes"); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageDriveRestoreReadbackFallbackBranches(t *testing.T) {
	testseam.Swap(t, &driveRestoreWait, func(time.Duration) {})
	successInfo := `{"success":true,"result":{"fileId":"restored-1","name":"restored.txt"}}`
	cases := []struct {
		name      string
		responses map[string][]string
		wantOK    bool
	}{
		{
			name: "restore call after preflight",
			responses: map[string][]string{
				"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"r1","originalName":"restored.txt"}]}`},
				"restore_recycle_item": {`__ERROR__`},
			},
		},
		{
			name: "restore terminal after preflight",
			responses: map[string][]string{
				"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"r1","originalName":"restored.txt"}]}`},
				"restore_recycle_item": {`{"nodeId":"restored-1"}`},
			},
		},
		{
			name: "restore read call after preflight",
			responses: map[string][]string{
				"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"r1","originalName":"restored.txt"}]}`},
				"restore_recycle_item": {`{"success":true,"nodeId":"restored-1"}`},
				"get_file_info":        {`__ERROR__`},
			},
		},
		{
			name: "restore read schema after preflight",
			responses: map[string][]string{
				"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"r1","originalName":"restored.txt"}]}`},
				"restore_recycle_item": {`{"success":true,"nodeId":"restored-1"}`},
				"get_file_info":        {`{"success":true}`},
			},
		},
		{
			name: "preflight pagination",
			responses: map[string][]string{
				"list_recycle_items":   {`{"success":true,"recycleItems":[],"hasMore":true,"nextCursor":"next"}`, `{"success":true,"recycleItems":[{"recycleItemId":"r1","originalName":"restored.txt"}]}`},
				"restore_recycle_item": {`{"success":true,"fileId":"restored-1"}`},
				"get_file_info":        {successInfo},
			},
			wantOK: true,
		},
		{name: "preflight malformed", responses: map[string][]string{"list_recycle_items": {`{"success":true}`}}},
		{name: "preflight not found", responses: map[string][]string{"list_recycle_items": {`{"success":true,"recycleItems":[],"hasMore":false,"nextCursor":"ignored"}`}}},
		{
			name: "missing restore identity",
			responses: map[string][]string{
				"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"r1"}]}`},
				"restore_recycle_item": {`{"success":true}`},
			},
		},
		{
			name: "parent search call",
			responses: map[string][]string{
				"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"r1","originalName":"restored.txt","originalPath":"/folder/restored.txt"}]}`},
				"restore_recycle_item": {`{"success":true}`},
				"search_files":         {`__ERROR__`},
			},
		},
		{
			name: "parent search schema",
			responses: map[string][]string{
				"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"r1","originalName":"restored.txt","originalPath":"/folder/restored.txt"}]}`},
				"restore_recycle_item": {`{"success":true}`},
				"search_files":         {`{"success":true}`},
			},
		},
		{
			name: "parent list call",
			responses: map[string][]string{
				"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"r1","originalName":"restored.txt","originalPath":"/folder/restored.txt"}]}`},
				"restore_recycle_item": {`{"success":true}`},
				"search_files":         {`{"success":true,"items":[{"fileId":"folder-1","name":"folder"}]}`},
				"list_files":           {`__ERROR__`},
			},
		},
		{
			name: "parent list schema",
			responses: map[string][]string{
				"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"r1","originalName":"restored.txt","originalPath":"/folder/restored.txt"}]}`},
				"restore_recycle_item": {`{"success":true}`},
				"search_files":         {`{"success":true,"items":[{"fileId":"folder-1","name":"folder"}]}`},
				"list_files":           {`{"success":true}`},
			},
		},
		{
			name: "parent list duplicate",
			responses: map[string][]string{
				"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"r1","originalName":"restored.txt","originalPath":"/folder/restored.txt"}]}`},
				"restore_recycle_item": {`{"success":true}`},
				"search_files":         {`{"success":true,"items":[{"fileId":"folder-1","name":"folder"}]}`},
				"list_files":           {`{"success":true,"items":[{"fileId":"skip","name":"other.txt"},{"fileId":"restored-1","name":"restored.txt"},{"fileId":"restored-2","name":"restored.txt"}]}`},
			},
		},
		{
			name: "fallback search unique with extension",
			responses: map[string][]string{
				"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"r1","originalName":"restored.txt","originalPath":"/folder/restored.txt"}]}`},
				"restore_recycle_item": {`{"success":true}`},
				"search_files": {`{"success":true,"items":[{"name":"other"},{"name":"folder"}]}`,
					`{"success":true,"items":[{"fileId":"skip-1","name":"other"},{"fileId":"skip-2","name":"restored","path":"/other/restored.txt"},{"name":"restored","extension":"txt","path":"/folder/restored.txt"},{"fileId":"restored-1","name":"restored","extension":"txt","path":"/folder/restored.txt"}]}`},
				"get_file_info": {successInfo},
			},
			wantOK: true,
		},
		{
			name: "fallback search direct name",
			responses: map[string][]string{
				"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"r1","originalName":"restored"}]}`},
				"restore_recycle_item": {`{"success":true}`},
				"search_files":         {`{"success":true,"items":[{"fileId":"restored-1","name":"restored"}]}`},
				"get_file_info":        {successInfo},
			},
			wantOK: true,
		},
		{
			name: "fallback search ambiguous",
			responses: map[string][]string{
				"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"r1","originalName":"restored"}]}`},
				"restore_recycle_item": {`{"success":true}`},
				"search_files":         {`{"success":true,"items":[{"fileId":"restored-1","name":"restored"},{"fileId":"restored-2","name":"restored"}]}`},
			},
		},
		{
			name: "fallback search call",
			responses: map[string][]string{
				"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"r1","originalName":"restored"}]}`},
				"restore_recycle_item": {`{"success":true}`},
				"search_files":         {`__ERROR__`},
			},
		},
		{
			name: "fallback search schema",
			responses: map[string][]string{
				"list_recycle_items":   {`{"success":true,"recycleItems":[{"recycleItemId":"r1","originalName":"restored"}]}`},
				"restore_recycle_item": {`{"success":true}`},
				"search_files":         {`{"success":true}`},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runDriveCoverage(t, RecycleRestore, &driveCoverageCaller{responses: tc.responses}, "--id", "r1", "--yes")
			if tc.wantOK && err != nil {
				t.Fatal(err)
			}
			if !tc.wantOK && err == nil {
				t.Fatal("expected restore fallback error")
			}
		})
	}

	for _, path := range []string{"restored.txt", "/restored.txt"} {
		caller := &driveCoverageCaller{responses: map[string][]string{
			"search_files": {`{"success":true,"items":[{"fileId":"restored-1","name":"restored","extension":"txt"}]}`},
		}}
		helpers.InitDeps(caller)
		rt := shortcut.RuntimeContextForTest(&cobra.Command{}, RecycleRestore)
		if _, err := findRestoredDriveNode(rt, map[string]any{"originalName": "restored.txt", "originalPath": path}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCrossPlatformCoverageDriveInspectAndCollectionOptions(t *testing.T) {
	inspect := &driveCoverageCaller{responses: map[string][]string{
		"get_file_info":           {`{"success":true,"result":{"fileId":"n1"}}`},
		"get_node_stats":          {`{"success":true,"result":{"views":1}}`},
		"get_file_publish_status": {`{"success":true,"result":{"published":false}}`},
		"get_cover":               {`{"success":true,"result":{"url":"https://x"}}`},
	}}
	if err := runDriveCoverage(t, Inspect, inspect, "--node", "n1", "--space-id", "space", "--include-stats", "--include-publish", "--include-cover"); err != nil {
		t.Fatal(err)
	}
	list := &driveCoverageCaller{responses: map[string][]string{"list_files": {`{"success":true,"result":{"items":[{"fileId":"n1","dentryId":"d1","name":"x"}],"nextToken":"c2","hasMore":true}}`}}}
	if err := runDriveCoverage(t, List, list, "--space-id", "space", "--folder", "folder", "--limit", "1", "--cursor", "c1", "--order-by", "name", "--order", "asc", "--thumbnail"); err != nil {
		t.Fatal(err)
	}
	search := &driveCoverageCaller{responses: map[string][]string{"search_files": {`{"success":true,"items":[{"fileId":"n1","name":"x"}],"nextPageToken":"c2"}`}}}
	if err := runDriveCoverage(t, Search, search, "--query", "x", "--target", "file", "--file-types", "document", "--extensions", "pdf", "--creator-uids", "u1", "--created-from", "1", "--created-to", "2", "--modified-from", "3", "--modified-to", "4", "--limit", "1", "--cursor", "c1"); err != nil {
		t.Fatal(err)
	}
	searchDocs := &driveCoverageCaller{responses: map[string][]string{"search_documents": {`{"success":true,"result":{"documents":[{"nodeId":"n1","name":"x"}]}}`}}}
	if err := runDriveCoverage(t, SearchDocs, searchDocs, "--query", "x", "--limit", "1"); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageDriveUploadInputAndExecutionEdges(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("empty.bin", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveDriveUploadInput("empty.bin"); err == nil {
		t.Fatal("empty file accepted")
	}
	if _, _, err := resolveDriveUploadInput("."); err == nil {
		t.Fatal("directory accepted")
	}
	if _, _, err := resolveDriveUploadInput("/absolute"); err == nil {
		t.Fatal("absolute path accepted")
	}

	t.Run("cwd error", func(t *testing.T) {
		testseam.Swap(t, &driveGetwd, func() (string, error) { return "", errors.New("cwd") })
		if _, _, err := resolveDriveUploadInput("x"); err == nil {
			t.Fatal("cwd error ignored")
		}
	})
	t.Run("base symlink error", func(t *testing.T) {
		testseam.Swap(t, &driveEvalSymlinks, func(string) (string, error) { return "", errors.New("base") })
		if _, _, err := resolveDriveUploadInput("x"); err == nil {
			t.Fatal("base symlink error ignored")
		}
	})
	t.Run("path symlink error", func(t *testing.T) {
		calls := 0
		testseam.Swap(t, &driveEvalSymlinks, func(path string) (string, error) {
			calls++
			if calls == 1 {
				return path, nil
			}
			return "", errors.New("path")
		})
		if _, _, err := resolveDriveUploadInput("x"); err == nil || !strings.Contains(err.Error(), "读取上传文件失败") {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("rel error", func(t *testing.T) {
		testseam.Swap(t, &driveEvalSymlinks, func(path string) (string, error) { return path, nil })
		testseam.Swap(t, &driveRel, func(string, string) (string, error) { return "", errors.New("rel") })
		if _, _, err := resolveDriveUploadInput("x"); err == nil {
			t.Fatal("rel error ignored")
		}
	})
	t.Run("stat error", func(t *testing.T) {
		testseam.Swap(t, &driveEvalSymlinks, func(path string) (string, error) { return path, nil })
		testseam.Swap(t, &driveStat, func(string) (os.FileInfo, error) { return nil, errors.New("stat") })
		if _, _, err := resolveDriveUploadInput("x"); err == nil {
			t.Fatal("stat error ignored")
		}
	})

	if err := os.WriteFile("input.bin", []byte("bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	dry := &driveCoverageCaller{responses: map[string][]string{}}
	if err := runDriveCoverage(t, Upload, dry, "--file", "input.bin", "--file-name", "remote.bin", "--dry-run", "--yes"); err != nil {
		t.Fatal(err)
	}
	t.Run("missing committed id", func(t *testing.T) {
		testseam.Swap(t, &uploadDriveFile, func(context.Context, helpers.DriveUploadRequest) (map[string]any, error) {
			return map[string]any{"success": true}, nil
		})
		err := runDriveCoverage(t, Upload, &driveCoverageCaller{responses: map[string][]string{}}, "--file", "input.bin", "--yes")
		if err == nil {
			t.Fatal("missing upload id accepted")
		}
	})
	t.Run("upload helper failure", func(t *testing.T) {
		testseam.Swap(t, &uploadDriveFile, func(context.Context, helpers.DriveUploadRequest) (map[string]any, error) {
			return nil, errors.New("upload helper")
		})
		err := runDriveCoverage(t, Upload, &driveCoverageCaller{responses: map[string][]string{}}, "--file", "input.bin", "--yes")
		if err == nil || !strings.Contains(err.Error(), "upload helper") {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("upload terminal evidence failure", func(t *testing.T) {
		testseam.Swap(t, &uploadDriveFile, func(context.Context, helpers.DriveUploadRequest) (map[string]any, error) {
			return map[string]any{"nodeId": "n1"}, nil
		})
		err := runDriveCoverage(t, Upload, &driveCoverageCaller{responses: map[string][]string{}}, "--file", "input.bin", "--yes")
		if err == nil || !strings.Contains(err.Error(), "success=true") {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("upload readback call failure", func(t *testing.T) {
		testseam.Swap(t, &uploadDriveFile, func(context.Context, helpers.DriveUploadRequest) (map[string]any, error) {
			return map[string]any{"success": true, "nodeId": "n1"}, nil
		})
		caller := &driveCoverageCaller{responses: map[string][]string{"get_file_info": {`__ERROR__`}}}
		if err := runDriveCoverage(t, Upload, caller, "--file", "input.bin", "--yes"); err == nil {
			t.Fatal("readback call failure ignored")
		}
	})
	t.Run("upload readback malformed object", func(t *testing.T) {
		testseam.Swap(t, &uploadDriveFile, func(context.Context, helpers.DriveUploadRequest) (map[string]any, error) {
			return map[string]any{"success": true, "nodeId": "n1"}, nil
		})
		caller := &driveCoverageCaller{responses: map[string][]string{"get_file_info": {`{"success":true}`}}}
		if err := runDriveCoverage(t, Upload, caller, "--file", "input.bin", "--yes"); err == nil {
			t.Fatal("malformed readback ignored")
		}
	})
	t.Run("overwrite readback", func(t *testing.T) {
		testseam.Swap(t, &uploadDriveFile, func(context.Context, helpers.DriveUploadRequest) (map[string]any, error) {
			return map[string]any{"success": true}, nil
		})
		caller := &driveCoverageCaller{responses: map[string][]string{"get_file_info": {`{"success":true,"result":{"fileId":"existing","name":"wrong"}}`}}}
		err := runDriveCoverage(t, Upload, caller, "--file", "input.bin", "--file-name", "wanted.bin", "--node", "existing", "--yes")
		if err == nil || !strings.Contains(err.Error(), "读回名称") {
			t.Fatalf("error=%v", err)
		}
	})
}

func TestCrossPlatformCoverageDriveVersionExecutionAndPayloadEdges(t *testing.T) {
	versionPayload := `{"success":true,"versions":[{"version":1,"fileName":"v1"}],"nextCursor":"c2"}`
	history := &driveCoverageCaller{responses: map[string][]string{"list_file_versions": {versionPayload}}}
	if err := runDriveCoverage(t, VersionHistory, history, "--node", "n1", "--limit", "1", "--cursor", "c1"); err != nil {
		t.Fatal(err)
	}
	if err := runDriveCoverage(t, VersionHistory, &driveCoverageCaller{responses: map[string][]string{"list_file_versions": {`__ERROR__`}}}, "--node", "n1"); err == nil {
		t.Fatal("version call failure ignored")
	}
	if err := runDriveCoverage(t, VersionHistory, &driveCoverageCaller{responses: map[string][]string{"list_file_versions": {`{"success":true}`}}}, "--node", "n1"); err == nil {
		t.Fatal("missing versions accepted")
	}

	t.Run("download success", func(t *testing.T) {
		testseam.Swap(t, &driveDownload, func(context.Context, string, localio.DownloadOptions) (localio.DownloadResult, error) {
			return localio.DownloadResult{RelativePath: "v1.bin", SizeBytes: 3}, nil
		})
		caller := &driveCoverageCaller{responses: map[string][]string{
			"list_file_versions":    {versionPayload},
			"download_file_version": {`{"success":true,"result":{"resourceUrls":[{"url":"https://x","fileName":"v1.bin","headers":{"X":"y","skip":1}}]}}`},
		}}
		if err := runDriveCoverage(t, VersionDownload, caller, "--node", "n1", "--version", "1", "--output", "v1.bin"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("download empty artifact", func(t *testing.T) {
		testseam.Swap(t, &driveDownload, func(context.Context, string, localio.DownloadOptions) (localio.DownloadResult, error) {
			return localio.DownloadResult{}, nil
		})
		caller := &driveCoverageCaller{responses: map[string][]string{"list_file_versions": {versionPayload}, "download_file_version": {`{"success":true,"result":{"downloadUrl":"https://x"}}`}}}
		if err := runDriveCoverage(t, VersionDownload, caller, "--node", "n1", "--version", "1", "--output", "v1.bin"); err == nil {
			t.Fatal("empty version artifact accepted")
		}
	})
	t.Run("find version call failure", func(t *testing.T) {
		caller := &driveCoverageCaller{responses: map[string][]string{"list_file_versions": {`__ERROR__`}}}
		if err := runDriveCoverage(t, VersionDownload, caller, "--node", "n1", "--version", "1", "--output", "v1.bin"); err == nil {
			t.Fatal("findVersion call error ignored")
		}
	})
	t.Run("find version not found", func(t *testing.T) {
		caller := &driveCoverageCaller{responses: map[string][]string{"list_file_versions": {versionPayload}}}
		if err := runDriveCoverage(t, VersionDownload, caller, "--node", "n1", "--version", "9", "--output", "v9.bin"); err == nil {
			t.Fatal("missing version accepted")
		}
	})
	revert := &driveCoverageCaller{responses: map[string][]string{
		"list_file_versions": {versionPayload}, "revert_file_version": {`{"success":true}`}, "get_file_info": {`{"success":true,"result":{"fileId":"n1"}}`},
	}}
	if err := runDriveCoverage(t, VersionRevert, revert, "--node", "n1", "--version", "1", "--yes"); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		value  any
		number int
		ok     bool
	}{
		{float64(2), 2, true}, {float64(2.5), 2, false}, {3, 3, true}, {"4", 4, true}, {"bad", 0, false}, {nil, 0, false},
	} {
		number, ok := versionNumber(map[string]any{"version": tc.value})
		if number != tc.number || ok != tc.ok {
			t.Fatalf("versionNumber(%v)=(%d,%v)", tc.value, number, ok)
		}
	}
	if _, _, _, err := driveDownloadPayload(map[string]any{}, "op"); err == nil {
		t.Fatal("missing URL accepted")
	}
	if url, _, headers, err := driveDownloadPayload(map[string]any{"resourceUrls": []any{"bad"}, "url": "https://fallback", "headers": map[string]any{"X": "y", "skip": 1}}, "op"); err != nil || url == "" || headers["X"] != "y" {
		t.Fatalf("payload=(%q,%v,%v)", url, headers, err)
	}
	if _, _, _, err := driveDownloadPayload(map[string]any{"resourceUrls": []any{}}, "op"); err == nil {
		t.Fatal("empty resources accepted")
	}
}

func TestCrossPlatformCoverageDriveVersionOperationsFindTargetOnSecondPage(t *testing.T) {
	firstPage := `{"success":true,"versions":[{"version":7,"fileName":"v7"}],"hasMore":true,"nextCursor":"page-2"}`
	secondPage := `{"success":true,"versions":[{"version":3,"fileName":"v3"}],"hasMore":false}`

	getCaller := &driveCoverageCaller{responses: map[string][]string{
		"list_file_versions": {firstPage, secondPage},
	}}
	if err := runDriveCoverage(t, VersionGet, getCaller, "--node", "n1", "--version", "3"); err != nil {
		t.Fatalf("version get second page: %v", err)
	}
	if got := strings.Join(getCaller.history, ","); got != "list_file_versions,list_file_versions" {
		t.Fatalf("version get history = %q", got)
	}

	t.Run("download", func(t *testing.T) {
		testseam.Swap(t, &driveDownload, func(context.Context, string, localio.DownloadOptions) (localio.DownloadResult, error) {
			return localio.DownloadResult{RelativePath: "v3.bin", SizeBytes: 3}, nil
		})
		caller := &driveCoverageCaller{responses: map[string][]string{
			"list_file_versions":    {firstPage, secondPage},
			"download_file_version": {`{"success":true,"result":{"downloadUrl":"https://download.dingtalk.com/v3.bin"}}`},
		}}
		if err := runDriveCoverage(t, VersionDownload, caller, "--node", "n1", "--version", "3", "--output", "v3.bin"); err != nil {
			t.Fatalf("version download second page: %v", err)
		}
		if got := strings.Join(caller.history, ","); got != "list_file_versions,list_file_versions,download_file_version" {
			t.Fatalf("version download history = %q", got)
		}
	})

	revertCaller := &driveCoverageCaller{responses: map[string][]string{
		"list_file_versions":  {firstPage, secondPage},
		"revert_file_version": {`{"success":true}`},
		"get_file_info":       {`{"success":true,"result":{"fileId":"n1"}}`},
	}}
	if err := runDriveCoverage(t, VersionRevert, revertCaller, "--node", "n1", "--version", "3", "--yes"); err != nil {
		t.Fatalf("version revert second page: %v", err)
	}
	if got := strings.Join(revertCaller.history, ","); got != "list_file_versions,list_file_versions,revert_file_version,get_file_info" {
		t.Fatalf("version revert history = %q", got)
	}
}

func TestCrossPlatformCoverageDriveVersionLookupFailsClosedOnIncompletePagination(t *testing.T) {
	t.Run("complete without pagination metadata", func(t *testing.T) {
		caller := &driveCoverageCaller{responses: map[string][]string{
			"list_file_versions": {`{"success":true,"versions":[]}`},
		}}
		err := runDriveCoverage(t, VersionGet, caller, "--node", "n1", "--version", "3")
		if err == nil || !strings.Contains(err.Error(), "不存在版本 3") || len(caller.history) != 1 {
			t.Fatalf("complete page error = %v", err)
		}
	})

	t.Run("missing cursor", func(t *testing.T) {
		caller := &driveCoverageCaller{responses: map[string][]string{
			"list_file_versions": {`{"success":true,"versions":[],"hasMore":true}`},
		}}
		err := runDriveCoverage(t, VersionGet, caller, "--node", "n1", "--version", "3")
		if err == nil || !strings.Contains(err.Error(), "无法证明分页已经完整") || len(caller.history) != 1 {
			t.Fatalf("missing cursor error = %v", err)
		}
	})

	t.Run("stalled cursor", func(t *testing.T) {
		caller := &driveCoverageCaller{responses: map[string][]string{
			"list_file_versions": {
				`{"success":true,"versions":[],"hasMore":true,"nextCursor":"same"}`,
				`{"success":true,"versions":[],"hasMore":true,"nextCursor":"same"}`,
			},
		}}
		err := runDriveCoverage(t, VersionGet, caller, "--node", "n1", "--version", "3")
		if err == nil || !strings.Contains(err.Error(), "无法证明分页已经完整") || len(caller.history) != 2 {
			t.Fatalf("stalled cursor error = %v", err)
		}
	})

	t.Run("page bound", func(t *testing.T) {
		pages := make([]string, 20)
		for i := range pages {
			pages[i] = fmt.Sprintf(`{"success":true,"versions":[],"hasMore":true,"nextCursor":"page-%d"}`, i+1)
		}
		caller := &driveCoverageCaller{responses: map[string][]string{"list_file_versions": pages}}
		err := runDriveCoverage(t, VersionGet, caller, "--node", "n1", "--version", "3")
		if err == nil || !strings.Contains(err.Error(), "无法证明分页已经完整") {
			t.Fatalf("page bound error = %v", err)
		}
		if len(caller.history) != 20 {
			t.Fatalf("page calls = %d, want 20", len(caller.history))
		}
	})
}

func TestCrossPlatformCoverageDriveSmallSemanticBranches(t *testing.T) {
	for _, info := range []map[string]any{{"extension": "adoc"}, {"contentType": "SHEET"}, {"contentType": "FILE"}} {
		_ = isOnlineDriveObject(info)
	}
	request, expected := normalizedDriveRename("plain", map[string]any{"type": "FILE"})
	if request != "plain" || !expected["plain"] {
		t.Fatalf("rename=%q %v", request, expected)
	}
	if value, ok := boolField(map[string]any{"publish": true}, "published", "publish"); !ok || !value {
		t.Fatalf("boolField=(%v,%v)", value, ok)
	}
	if _, ok := boolField(map[string]any{}, "published"); ok {
		t.Fatal("missing bool field accepted")
	}
}

func TestCrossPlatformCoverageDriveEveryTerminalErrorPath(t *testing.T) {
	versionOK := `{"success":true,"versions":[{"version":1}]}`
	cases := []struct {
		name        string
		declaration shortcut.Shortcut
		responses   map[string][]string
		args        []string
	}{
		{"recycle list options", RecycleList, map[string][]string{"list_recycle_items": {`{"success":true,"recycleItems":[]}`}}, []string{"--space-id", "space", "--cursor", "c1"}},
		{"recycle list call", RecycleList, map[string][]string{"list_recycle_items": {`__ERROR__`}}, nil},
		{"recycle list schema", RecycleList, map[string][]string{"list_recycle_items": {`{"success":true}`}}, nil},
		{"search call", Search, map[string][]string{"search_files": {`__ERROR__`}}, []string{"--query", "x"}},
		{"restore call", RecycleRestore, map[string][]string{"restore_recycle_item": {`__ERROR__`}}, []string{"--id", "r1", "--yes"}},
		{"restore terminal", RecycleRestore, map[string][]string{"restore_recycle_item": {`{"nodeId":"n1"}`}}, []string{"--id", "r1", "--yes"}},
		{"restore read call", RecycleRestore, map[string][]string{"restore_recycle_item": {`{"success":true,"nodeId":"n1"}`}, "get_file_info": {`__ERROR__`}}, []string{"--id", "r1", "--yes"}},
		{"restore read schema", RecycleRestore, map[string][]string{"restore_recycle_item": {`{"success":true,"nodeId":"n1"}`}, "get_file_info": {`{"success":true}`}}, []string{"--id", "r1", "--yes"}},
		{"star list options", StarList, map[string][]string{"get_star_list": {`{"success":true,"starList":[]}`}}, []string{"--cursor", "c1", "--content-types", "DOC"}},
		{"star list call", StarList, map[string][]string{"get_star_list": {`__ERROR__`}}, nil},
		{"star list schema", StarList, map[string][]string{"get_star_list": {`{"success":true}`}}, nil},
		{"inspect base call", Inspect, map[string][]string{"get_file_info": {`__ERROR__`}}, []string{"--node", "n1"}},
		{"inspect base schema", Inspect, map[string][]string{"get_file_info": {`{"success":true}`}}, []string{"--node", "n1"}},
		{"folder call", CreateFolder, map[string][]string{"create_folder": {`__ERROR__`}}, []string{"--name", "x"}},
		{"folder terminal", CreateFolder, map[string][]string{"create_folder": {`{"fileId":"n1"}`}}, []string{"--name", "x"}},
		{"folder read call", CreateFolder, map[string][]string{"create_folder": {`{"success":true,"fileId":"n1"}`}, "get_file_info": {`__ERROR__`}}, []string{"--name", "x"}},
		{"folder read schema", CreateFolder, map[string][]string{"create_folder": {`{"success":true,"fileId":"n1"}`}, "get_file_info": {`{"success":true}`}}, []string{"--name", "x"}},
		{"shortcut call", CreateShortcut, map[string][]string{"create_shortcut": {`__ERROR__`}}, []string{"--node", "n1"}},
		{"shortcut terminal", CreateShortcut, map[string][]string{"create_shortcut": {`{"nodeId":"n2"}`}}, []string{"--node", "n1"}},
		{"shortcut read call", CreateShortcut, map[string][]string{"create_shortcut": {`{"success":true,"nodeId":"n2"}`}, "get_file_info": {`__ERROR__`}}, []string{"--node", "n1"}},
		{"shortcut read schema", CreateShortcut, map[string][]string{"create_shortcut": {`{"success":true,"nodeId":"n2"}`}, "get_file_info": {`{"success":true}`}}, []string{"--node", "n1"}},
		{"rename preflight call", Rename, map[string][]string{"get_file_info": {`__ERROR__`}}, []string{"--node", "n1", "--name", "x", "--yes"}},
		{"rename preflight schema", Rename, map[string][]string{"get_file_info": {`{"success":true}`}}, []string{"--node", "n1", "--name", "x", "--yes"}},
		{"rename write call", Rename, map[string][]string{"get_file_info": {`{"success":true,"result":{"fileId":"n1"}}`}, "rename_document": {`__ERROR__`}}, []string{"--node", "n1", "--name", "x", "--yes"}},
		{"rename terminal", Rename, map[string][]string{"get_file_info": {`{"success":true,"result":{"fileId":"n1"}}`}, "rename_document": {`{"nodeId":"n1"}`}}, []string{"--node", "n1", "--name", "x", "--yes"}},
		{"rename read call", Rename, map[string][]string{"get_file_info": {`{"success":true,"result":{"fileId":"n1"}}`, `__ERROR__`}, "rename_document": {`{"success":true}`}}, []string{"--node", "n1", "--name", "x", "--yes"}},
		{"rename read schema", Rename, map[string][]string{"get_file_info": {`{"success":true,"result":{"fileId":"n1"}}`, `{"success":true}`}, "rename_document": {`{"success":true}`}}, []string{"--node", "n1", "--name", "x", "--yes"}},
		{"delete call", Delete, map[string][]string{"delete_document": {`__ERROR__`}}, []string{"--node", "n1", "--yes"}},
		{"delete terminal", Delete, map[string][]string{"delete_document": {`{"nodeId":"n1"}`}}, []string{"--node", "n1", "--yes"}},
		{"version get validate", VersionGet, map[string][]string{}, []string{"--node", "n1", "--version", "0"}},
		{"version get page", VersionGet, map[string][]string{"list_file_versions": {`__ERROR__`}}, []string{"--node", "n1", "--version", "1"}},
		{"version download validate", VersionDownload, map[string][]string{}, []string{"--node", "n1", "--version", "0", "--output", "x"}},
		{"version download call", VersionDownload, map[string][]string{"list_file_versions": {versionOK}, "download_file_version": {`__ERROR__`}}, []string{"--node", "n1", "--version", "1", "--output", "x"}},
		{"version download schema", VersionDownload, map[string][]string{"list_file_versions": {versionOK}, "download_file_version": {`{"success":true}`}}, []string{"--node", "n1", "--version", "1", "--output", "x"}},
		{"version download URL", VersionDownload, map[string][]string{"list_file_versions": {versionOK}, "download_file_version": {`{"success":true,"result":{"name":"x"}}`}}, []string{"--node", "n1", "--version", "1", "--output", "x"}},
		{"version revert validate", VersionRevert, map[string][]string{}, []string{"--node", "n1", "--version", "0", "--yes"}},
		{"version revert find", VersionRevert, map[string][]string{"list_file_versions": {`__ERROR__`}}, []string{"--node", "n1", "--version", "1", "--yes"}},
		{"version revert write", VersionRevert, map[string][]string{"list_file_versions": {versionOK}, "revert_file_version": {`__ERROR__`}}, []string{"--node", "n1", "--version", "1", "--yes"}},
		{"version revert terminal", VersionRevert, map[string][]string{"list_file_versions": {versionOK}, "revert_file_version": {`{"nodeId":"n1"}`}}, []string{"--node", "n1", "--version", "1", "--yes"}},
		{"version revert read call", VersionRevert, map[string][]string{"list_file_versions": {versionOK}, "revert_file_version": {`{"success":true}`}, "get_file_info": {`__ERROR__`}}, []string{"--node", "n1", "--version", "1", "--yes"}},
		{"version revert read schema", VersionRevert, map[string][]string{"list_file_versions": {versionOK}, "revert_file_version": {`{"success":true}`}, "get_file_info": {`{"success":true}`}}, []string{"--node", "n1", "--version", "1", "--yes"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runDriveCoverage(t, tc.declaration, &driveCoverageCaller{responses: tc.responses}, tc.args...)
			if strings.Contains(tc.name, "options") {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected terminal error")
			}
		})
	}

	t.Run("create folder option branches", func(t *testing.T) {
		caller := &driveCoverageCaller{responses: map[string][]string{
			"create_folder": {`{"success":true,"fileId":"n1"}`},
			"get_file_info": {`{"success":true,"result":{"fileId":"n1","name":"x"}}`},
		}}
		if err := runDriveCoverage(t, CreateFolder, caller, "--name", "x", "--space-id", "space", "--folder", "folder"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("version download cwd", func(t *testing.T) {
		testseam.Swap(t, &driveGetwd, func() (string, error) { return "", errors.New("cwd") })
		caller := &driveCoverageCaller{responses: map[string][]string{"list_file_versions": {versionOK}, "download_file_version": {`{"success":true,"result":{"url":"https://x"}}`}}}
		if err := runDriveCoverage(t, VersionDownload, caller, "--node", "n1", "--version", "1", "--output", "x"); err == nil {
			t.Fatal("cwd failure ignored")
		}
	})
	t.Run("version download transport", func(t *testing.T) {
		testseam.Swap(t, &driveDownload, func(context.Context, string, localio.DownloadOptions) (localio.DownloadResult, error) {
			return localio.DownloadResult{}, errors.New("download")
		})
		caller := &driveCoverageCaller{responses: map[string][]string{"list_file_versions": {versionOK}, "download_file_version": {`{"success":true,"result":{"url":"https://x"}}`}}}
		if err := runDriveCoverage(t, VersionDownload, caller, "--node", "n1", "--version", "1", "--output", "x"); err == nil {
			t.Fatal("download failure ignored")
		}
	})
}

func TestCrossPlatformCoverageDriveLegacyLeavesStrictErrorClosure(t *testing.T) {
	cases := []struct {
		name        string
		declaration shortcut.Shortcut
		responses   map[string][]string
		args        []string
		wantSuccess bool
	}{
		{"info space option", Info, map[string][]string{"get_file_info": {`{"success":true,"result":{"fileId":"n1"}}`}}, []string{"--node", "n1", "--space-id", "space"}, true},
		{"info call", Info, map[string][]string{"get_file_info": {`__ERROR__`}}, []string{"--node", "n1"}, false},
		{"info schema", Info, map[string][]string{"get_file_info": {`{"success":true}`}}, []string{"--node", "n1"}, false},
		{"download call", Download, map[string][]string{"download_file": {`__ERROR__`}}, []string{"--node", "n1", "--output", "x"}, false},
		{"download schema", Download, map[string][]string{"download_file": {`{"success":true}`}}, []string{"--node", "n1", "--output", "x"}, false},
		{"download URL", Download, map[string][]string{"download_file": {`{"success":true,"result":{"name":"x"}}`}}, []string{"--node", "n1", "--output", "x"}, false},
		{"search docs call", SearchDocs, map[string][]string{"search_documents": {`__ERROR__`}}, []string{"--query", "x"}, false},
		{"search docs schema", SearchDocs, map[string][]string{"search_documents": {`{"success":true}`}}, []string{"--query", "x"}, false},
		{"copy preflight call", Copy, map[string][]string{"get_document_info": {`__ERROR__`}}, []string{"--node", "n1", "--yes"}, false},
		{"copy preflight schema", Copy, map[string][]string{"get_document_info": {`{"success":true}`}}, []string{"--node", "n1", "--yes"}, false},
		{"copy write call", Copy, map[string][]string{"get_document_info": {`{"success":true,"result":{"extension":"adoc"}}`}, "copy_document": {`__ERROR__`}}, []string{"--node", "n1", "--yes"}, false},
		{"copy terminal", Copy, map[string][]string{"get_document_info": {`{"success":true,"result":{"extension":"adoc"}}`}, "copy_document": {`{"nodeId":"n2"}`}}, []string{"--node", "n1", "--yes"}, false},
		{"copy read call", Copy, map[string][]string{"get_document_info": {`{"success":true,"result":{"extension":"adoc"}}`, `__ERROR__`}, "copy_document": {`{"success":true,"nodeId":"n2"}`}}, []string{"--node", "n1", "--yes"}, false},
		{"copy read schema", Copy, map[string][]string{"get_document_info": {`{"success":true,"result":{"extension":"adoc"}}`, `{"success":true}`}, "copy_document": {`{"success":true,"nodeId":"n2"}`}}, []string{"--node", "n1", "--yes"}, false},
		{"move call", Move, map[string][]string{"move_document": {`__ERROR__`}}, []string{"--node", "n1", "--yes"}, false},
		{"move terminal", Move, map[string][]string{"move_document": {`{"nodeId":"n1"}`}}, []string{"--node", "n1", "--yes"}, false},
		{"move read call", Move, map[string][]string{"move_document": {`{"success":true}`}, "get_document_info": {`__ERROR__`}}, []string{"--node", "n1", "--yes"}, false},
		{"move read schema", Move, map[string][]string{"move_document": {`{"success":true}`}, "get_document_info": {`{"success":true}`}}, []string{"--node", "n1", "--yes"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runDriveCoverage(t, tc.declaration, &driveCoverageCaller{responses: tc.responses}, tc.args...)
			if tc.wantSuccess && err != nil {
				t.Fatal(err)
			}
			if !tc.wantSuccess && err == nil {
				t.Fatal("expected strict error")
			}
		})
	}

	t.Run("download space option", func(t *testing.T) {
		testseam.Swap(t, &driveDownload, func(context.Context, string, localio.DownloadOptions) (localio.DownloadResult, error) {
			return localio.DownloadResult{RelativePath: "x", SizeBytes: 1}, nil
		})
		caller := &driveCoverageCaller{responses: map[string][]string{"download_file": {`{"success":true,"result":{"url":"https://x"}}`}}}
		if err := runDriveCoverage(t, Download, caller, "--node", "n1", "--space-id", "space", "--output", "x"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("download cwd", func(t *testing.T) {
		testseam.Swap(t, &driveGetwd, func() (string, error) { return "", errors.New("cwd") })
		caller := &driveCoverageCaller{responses: map[string][]string{"download_file": {`{"success":true,"result":{"url":"https://x"}}`}}}
		if err := runDriveCoverage(t, Download, caller, "--node", "n1", "--output", "x"); err == nil {
			t.Fatal("cwd error ignored")
		}
	})
	t.Run("download transport", func(t *testing.T) {
		testseam.Swap(t, &driveDownload, func(context.Context, string, localio.DownloadOptions) (localio.DownloadResult, error) {
			return localio.DownloadResult{}, errors.New("download")
		})
		caller := &driveCoverageCaller{responses: map[string][]string{"download_file": {`{"success":true,"result":{"url":"https://x"}}`}}}
		if err := runDriveCoverage(t, Download, caller, "--node", "n1", "--output", "x"); err == nil {
			t.Fatal("download error ignored")
		}
	})
	t.Run("rename mismatch", func(t *testing.T) {
		caller := &driveCoverageCaller{responses: map[string][]string{
			"get_file_info":   {`{"success":true,"result":{"fileId":"n1"}}`, `{"success":true,"result":{"fileId":"n1","name":"wrong"}}`},
			"rename_document": {`{"success":true}`},
		}}
		if err := runDriveCoverage(t, Rename, caller, "--node", "n1", "--name", "wanted", "--yes"); err == nil {
			t.Fatal("rename mismatch accepted")
		}
	})
	t.Run("upload input failure", func(t *testing.T) {
		if err := runDriveCoverage(t, Upload, &driveCoverageCaller{responses: map[string][]string{}}, "--file", "missing.bin", "--yes"); err == nil {
			t.Fatal("missing upload input accepted")
		}
	})
}
